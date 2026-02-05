// Copyright 2016 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package grpclient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/xkeyideal/grpcbalance/grpclient/balancer"
	"github.com/xkeyideal/grpcbalance/grpclient/circuitbreaker"
	"github.com/xkeyideal/grpcbalance/grpclient/discovery"
	"github.com/xkeyideal/grpcbalance/grpclient/logger"
	"github.com/xkeyideal/grpcbalance/grpclient/resolver"

	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

// Client provides and manages an etcd v3 client session.
type Client struct {
	conn *grpc.ClientConn

	cfg      Config
	resolver *resolver.CustomizeResolver
	mu       *sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc

	callOpts []grpc.CallOption

	// discovery related fields
	discovery discovery.Discovery
	watchDone chan struct{}

	// logger for client operations
	logger logger.Logger
}

func (c *Client) GetCallOpts() []grpc.CallOption {
	return c.callOpts
}

// Ctx is a context for "out of band" messages (e.g., for sending
// "clean up" message when another context is canceled). It is
// canceled on client Close().
func (c *Client) Ctx() context.Context { return c.ctx }

// Endpoints lists the registered endpoints for the client.
func (c *Client) Endpoints() []string {
	// copy the slice; protect original endpoints from being changed
	c.mu.RLock()
	defer c.mu.RUnlock()
	eps := make([]string, len(c.cfg.Endpoints))
	copy(eps, c.cfg.Endpoints)
	return eps
}

// SetEndpoints updates client's endpoints.
func (c *Client) SetEndpoints(eps []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg.Endpoints = eps

	c.resolver.SetEndpoints(eps, c.cfg.Attributes)
}

// dialSetupOpts gives the dial opts prior to any authentication.
func (c *Client) dialSetupOpts(dopts ...grpc.DialOption) (opts []grpc.DialOption, err error) {
	if c.cfg.DialKeepAliveTime > 0 {
		params := keepalive.ClientParameters{
			Time:                c.cfg.DialKeepAliveTime,
			Timeout:             c.cfg.DialKeepAliveTimeout,
			PermitWithoutStream: c.cfg.PermitWithoutStream,
		}
		opts = append(opts, grpc.WithKeepaliveParams(params))
	}
	opts = append(opts, dopts...)
	// grpc.WithInsecure() instead of WithTransportCredentials(insecure.NewCredentials())
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithInitialWindowSize(65536*100)) // 100*64K

	// 设置拦截器
	retryOpts := []grpc_retry.CallOption{
		grpc_retry.WithMax(3),
		grpc_retry.WithBackoff(grpc_retry.BackoffExponential(100 * time.Millisecond)),
		// Unavailable: 服务暂时不可用，可重试
		// ResourceExhausted: 资源耗尽（如限流），可重试
		// Aborted: 操作被中止（如并发冲突），可重试
		grpc_retry.WithCodes(codes.Unavailable, codes.ResourceExhausted, codes.Aborted),
	}

	opts = append(opts,
		// Disable stream retry by default since go-grpc-middleware/retry does not support client streams.
		// Streams that are safe to retry are enabled individually.
		//grpc.WithStreamInterceptor(c.streamClientInterceptor(withMax(0), rrBackoff)),
		grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor(retryOpts...)),
	)

	return opts, nil
}

// Dial connects to a single endpoint using the client's config.
func (c *Client) Dial(ep string) (*grpc.ClientConn, error) {
	// Using ad-hoc created resolver, to guarantee only explicitly given
	// endpoint is used.
	return c.dial(grpc.WithResolvers(resolver.NewCustomizeResolver([]string{ep}, c.cfg.Attributes)))
}

// dialWithBalancer dials the client's current load balanced resolver group.  The scheme of the host
// of the provided endpoint determines the scheme used for all endpoints of the client connection.
func (c *Client) dialWithBalancer(dopts ...grpc.DialOption) (*grpc.ClientConn, error) {
	opts := append(dopts, grpc.WithResolvers(c.resolver))
	return c.dial(opts...)
}

// dial configures and dials any grpc balancer target.
func (c *Client) dial(dopts ...grpc.DialOption) (*grpc.ClientConn, error) {
	opts, err := c.dialSetupOpts(dopts...)
	if err != nil {
		return nil, fmt.Errorf("failed to configure dialer: %v", err)
	}

	opts = append(opts, c.cfg.DialOptions...)

	// initialEndpoints := strings.Join(c.cfg.Endpoints, ";")
	// target := fmt.Sprintf("%s://%p/#initially=[%s]", resolver.Scheme, c, initialEndpoints)
	target := fmt.Sprintf("%s://%p/%s", resolver.Scheme, c, authority(c.Endpoints()[0]))

	// Use grpc.NewClient instead of deprecated grpc.DialContext
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, err
	}

	// If dial timeout is set, wait for connection to be ready
	if c.cfg.DialTimeout > 0 {
		ctx, cancel := context.WithTimeout(c.ctx, c.cfg.DialTimeout)
		defer cancel()
		conn.Connect()
		for {
			state := conn.GetState()
			if state == connectivity.Ready {
				break
			}
			if state == connectivity.Shutdown {
				conn.Close()
				return nil, fmt.Errorf("connection shutdown while dialing")
			}
			if !conn.WaitForStateChange(ctx, state) {
				// Timed out.
				finalState := conn.GetState()
				conn.Close()
				return nil, fmt.Errorf("connection timeout: state=%s", finalState)
			}
		}
	}

	return conn, nil
}

func NewClient(cfg *Config) (*Client, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}

	// default load balance algorithm round robin
	// Get circuit breaker config if enabled
	var cbConfig circuitbreaker.Config
	if cfg.EnableCircuitBreaker {
		if cfg.CircuitBreakerConfig != nil {
			cbConfig = *cfg.CircuitBreakerConfig
		} else {
			cbConfig = circuitbreaker.DefaultConfig()
		}
	}

	switch cfg.BalanceName {
	case balancer.WeightedRobinBalanceName:
		if cfg.EnableCircuitBreaker && cfg.EnableNodeFilter {
			balancer.RegisterWRRBalanceWithFilterAndCircuitBreaker(cfg.EnableHealthCheck, cbConfig, cfg.Logger)
		} else if cfg.EnableCircuitBreaker {
			balancer.RegisterWRRBalanceWithCircuitBreaker(cfg.EnableHealthCheck, cbConfig, cfg.Logger)
		} else if cfg.EnableNodeFilter {
			balancer.RegisterWRRBalanceWithFilter(cfg.EnableHealthCheck, cfg.Logger)
		} else {
			balancer.RegisterWRRBalance(cfg.EnableHealthCheck, cfg.Logger)
		}
	case balancer.RandomWeightedRobinBalanceName:
		if cfg.EnableCircuitBreaker && cfg.EnableNodeFilter {
			balancer.RegisterRwrrBalanceWithFilterAndCircuitBreaker(cfg.EnableHealthCheck, cbConfig, cfg.Logger)
		} else if cfg.EnableCircuitBreaker {
			balancer.RegisterRwrrBalanceWithCircuitBreaker(cfg.EnableHealthCheck, cbConfig, cfg.Logger)
		} else if cfg.EnableNodeFilter {
			balancer.RegisterRwrrBalanceWithFilter(cfg.EnableHealthCheck, cfg.Logger)
		} else {
			balancer.RegisterRwrrBalance(cfg.EnableHealthCheck, cfg.Logger)
		}
	case balancer.MinConnectBalanceName:
		if cfg.EnableCircuitBreaker && cfg.EnableNodeFilter {
			balancer.RegisterMcBalanceWithFilterAndCircuitBreaker(cfg.EnableHealthCheck, cbConfig, cfg.Logger)
		} else if cfg.EnableCircuitBreaker {
			balancer.RegisterMcBalanceWithCircuitBreaker(cfg.EnableHealthCheck, cbConfig, cfg.Logger)
		} else if cfg.EnableNodeFilter {
			balancer.RegisterMcBalanceWithFilter(cfg.EnableHealthCheck, cfg.Logger)
		} else {
			balancer.RegisterMcBalance(cfg.EnableHealthCheck, cfg.Logger)
		}
	case balancer.MinRespTimeBalanceName:
		if cfg.EnableCircuitBreaker && cfg.EnableNodeFilter {
			balancer.RegisterMrtBalanceWithFilterAndCircuitBreaker(cfg.EnableHealthCheck, cbConfig, cfg.Logger)
		} else if cfg.EnableCircuitBreaker {
			balancer.RegisterMrtBalanceWithCircuitBreaker(cfg.EnableHealthCheck, cbConfig, cfg.Logger)
		} else if cfg.EnableNodeFilter {
			balancer.RegisterMrtBalanceWithFilter(cfg.EnableHealthCheck, cfg.Logger)
		} else {
			balancer.RegisterMrtBalance(cfg.EnableHealthCheck, cfg.Logger)
		}
	case balancer.P2CBalancerName:
		if cfg.EnableCircuitBreaker && cfg.EnableNodeFilter {
			balancer.RegisterP2CBalanceWithFilterAndCircuitBreaker(cfg.EnableHealthCheck, cbConfig, cfg.Logger)
		} else if cfg.EnableCircuitBreaker {
			balancer.RegisterP2CBalanceWithCircuitBreaker(cfg.EnableHealthCheck, cbConfig, cfg.Logger)
		} else if cfg.EnableNodeFilter {
			balancer.RegisterP2CBalanceWithFilter(cfg.EnableHealthCheck, cfg.Logger)
		} else {
			balancer.RegisterP2CBalance(cfg.EnableHealthCheck, cfg.Logger)
		}
	default:
		cfg.BalanceName = balancer.RoundRobinBalanceName
		if cfg.EnableCircuitBreaker && cfg.EnableNodeFilter {
			balancer.RegisterRRBalanceWithFilterAndCircuitBreaker(cfg.EnableHealthCheck, cbConfig, cfg.Logger)
		} else if cfg.EnableCircuitBreaker {
			balancer.RegisterRRBalanceWithCircuitBreaker(cfg.EnableHealthCheck, cbConfig, cfg.Logger)
		} else if cfg.EnableNodeFilter {
			balancer.RegisterRRBalanceWithFilter(cfg.EnableHealthCheck, cfg.Logger)
		} else {
			balancer.RegisterRRBalance(cfg.EnableHealthCheck, cfg.Logger)
		}
	}

	// use a temporary skeleton client to bootstrap first connection
	baseCtx := context.TODO()
	if cfg.Context != nil {
		baseCtx = cfg.Context
	}

	ctx, cancel := context.WithCancel(baseCtx)
	client := &Client{
		conn:     nil,
		cfg:      *cfg,
		ctx:      ctx,
		cancel:   cancel,
		mu:       new(sync.RWMutex),
		callOpts: defaultCallOpts,
	}

	// Initialize logger
	if cfg.Logger != nil {
		client.logger = cfg.Logger
	} else {
		client.logger = logger.NewDefaultLogger(logger.LevelInfo)
	}

	if cfg.MaxCallSendMsgSize > 0 || cfg.MaxCallRecvMsgSize > 0 {
		if cfg.MaxCallRecvMsgSize > 0 && cfg.MaxCallSendMsgSize > cfg.MaxCallRecvMsgSize {
			return nil, fmt.Errorf("gRPC message recv limit (%d bytes) must be greater than send limit (%d bytes)", cfg.MaxCallRecvMsgSize, cfg.MaxCallSendMsgSize)
		}
		callOpts := []grpc.CallOption{
			defaultWaitForReady,
			defaultMaxCallSendMsgSize,
			defaultMaxCallRecvMsgSize,
		}
		if cfg.MaxCallSendMsgSize > 0 {
			callOpts[1] = grpc.MaxCallSendMsgSize(cfg.MaxCallSendMsgSize)
		}
		if cfg.MaxCallRecvMsgSize > 0 {
			callOpts[2] = grpc.MaxCallRecvMsgSize(cfg.MaxCallRecvMsgSize)
		}
		client.callOpts = callOpts
	}

	// Initialize endpoints from discovery or config
	endpoints := cfg.Endpoints
	attrs := cfg.Attributes

	// If Discovery is configured, fetch initial endpoints from it
	if cfg.Discovery != nil {
		client.discovery = cfg.Discovery

		discoveryEps, err := cfg.Discovery.GetEndpoints(ctx)
		if err != nil {
			client.cancel()
			return nil, fmt.Errorf("failed to get initial endpoints from discovery: %v", err)
		}

		if len(discoveryEps) == 0 {
			client.cancel()
			return nil, errors.New("discovery returned no endpoints")
		}

		endpoints = discovery.EndpointsToAddrs(discoveryEps)
		attrs = discovery.EndpointsToAttrsMap(discoveryEps)

		// Call the callback if set
		if cfg.OnEndpointsUpdate != nil {
			cfg.OnEndpointsUpdate(discoveryEps)
		}

		// Keep cfg in sync with the resolver's initial state.
		// This is important because dial() derives the target authority from c.Endpoints()[0].
		client.cfg.Endpoints = endpoints
		client.cfg.Attributes = attrs
	}

	client.resolver = resolver.NewCustomizeResolver(endpoints, attrs)

	if len(endpoints) < 1 {
		client.cancel()
		return nil, fmt.Errorf("at least one Endpoint is required in client config")
	}

	// Use a provided endpoint target so that for https:// without any tls config given, then
	// grpc will assume the certificate server name is the endpoint host.
	opt := grpc.WithDefaultServiceConfig(fmt.Sprintf(`{"loadBalancingConfig": [{"%s":{}}]}`, cfg.BalanceName))
	conn, err := client.dialWithBalancer(opt)
	if err != nil {
		client.cancel()
		client.resolver.Close()
		// TODO: Error like `fmt.Errorf(dialing [%s] failed: %v, strings.Join(cfg.Endpoints, ";"), err)` would help with debugging a lot.
		return nil, err
	}
	client.conn = conn

	// Start discovery watcher if Discovery is configured
	if cfg.Discovery != nil {
		if err := client.startDiscoveryWatcher(); err != nil {
			client.cancel()
			client.resolver.Close()
			conn.Close()
			return nil, fmt.Errorf("failed to start discovery watcher: %v", err)
		}
	}

	return client, nil
}

// startDiscoveryWatcher starts watching for endpoint changes from discovery
func (c *Client) startDiscoveryWatcher() error {
	c.watchDone = make(chan struct{})

	// Try to use native Watch first
	eventCh, err := c.cfg.Discovery.Watch(c.ctx)
	if err != nil {
		return err
	}

	// If discovery doesn't support native watching, use polling
	if eventCh == nil {
		pollInterval := c.cfg.DiscoveryPollInterval
		if pollInterval <= 0 {
			pollInterval = 30 * time.Second
		}

		pollingDiscovery := discovery.NewPollingDiscovery(c.cfg.Discovery, pollInterval)
		eventCh, err = pollingDiscovery.Watch(c.ctx)
		if err != nil {
			return err
		}
	}

	go c.watchDiscovery(eventCh)
	return nil
}

// watchDiscovery watches for endpoint changes and updates the resolver
func (c *Client) watchDiscovery(eventCh <-chan discovery.Event) {
	defer close(c.watchDone)

	var consecutiveErrors int
	const maxConsecutiveErrors = 5

	for {
		select {
		case <-c.ctx.Done():
			return
		case event, ok := <-eventCh:
			if !ok {
				// Channel closed, discovery stopped
				c.logger.Warn("discovery watch channel closed")
				return
			}

			switch event.Type {
			case discovery.EventTypeUpdate:
				if len(event.Endpoints) > 0 {
					consecutiveErrors = 0 // Reset error counter on successful update

					c.mu.Lock()
					addrs := discovery.EndpointsToAddrs(event.Endpoints)
					attrs := discovery.EndpointsToAttrsMap(event.Endpoints)
					c.cfg.Endpoints = addrs
					c.cfg.Attributes = attrs
					c.resolver.SetEndpoints(addrs, attrs)
					c.mu.Unlock()

					c.logger.Infof("discovery endpoints updated, count=%d", len(event.Endpoints))

					// Call the callback if set
					if c.cfg.OnEndpointsUpdate != nil {
						c.cfg.OnEndpointsUpdate(event.Endpoints)
					}
				} else {
					// Empty endpoints update - this might indicate all endpoints are gone
					c.logger.Warn("discovery returned empty endpoints, keeping existing endpoints")
				}

			case discovery.EventTypeDelete:
				// Handle endpoint deletion
				// When endpoints are deleted, we receive an update with remaining endpoints
				// If all endpoints are deleted, the Endpoints slice will be empty or nil
				if len(event.Endpoints) == 0 {
					// All endpoints deleted - this is a critical state
					// Keep existing endpoints to maintain connectivity
					c.logger.Warn("all discovery endpoints deleted, keeping existing endpoints")
				} else {
					// Some endpoints deleted, update with remaining endpoints
					c.mu.Lock()
					addrs := discovery.EndpointsToAddrs(event.Endpoints)
					attrs := discovery.EndpointsToAttrsMap(event.Endpoints)
					c.cfg.Endpoints = addrs
					c.cfg.Attributes = attrs
					c.resolver.SetEndpoints(addrs, attrs)
					c.mu.Unlock()

					c.logger.Infof("discovery endpoints deleted, remaining count=%d", len(event.Endpoints))

					// Call the callback if set
					if c.cfg.OnEndpointsUpdate != nil {
						c.cfg.OnEndpointsUpdate(event.Endpoints)
					}
				}

			case discovery.EventTypeError:
				consecutiveErrors++
				errMsg := "unknown error"
				if event.Err != nil {
					errMsg = event.Err.Error()
				}
				c.logger.Errorf("discovery error occurred: %s, consecutive errors=%d", errMsg, consecutiveErrors)

				// If too many consecutive errors, try to re-fetch endpoints
				if consecutiveErrors >= maxConsecutiveErrors {
					c.logger.Warnf("too many consecutive discovery errors (%d), attempting to refresh endpoints", consecutiveErrors)

					// Try to get current endpoints directly
					ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
					eps, err := c.discovery.GetEndpoints(ctx)
					cancel()

					if err != nil {
						c.logger.Errorf("failed to refresh endpoints: %v", err)
					} else if len(eps) > 0 {
						c.mu.Lock()
						addrs := discovery.EndpointsToAddrs(eps)
						attrs := discovery.EndpointsToAttrsMap(eps)
						c.cfg.Endpoints = addrs
						c.cfg.Attributes = attrs
						c.resolver.SetEndpoints(addrs, attrs)
						c.mu.Unlock()

						c.logger.Infof("endpoints refreshed successfully, count=%d", len(eps))
						consecutiveErrors = 0

						if c.cfg.OnEndpointsUpdate != nil {
							c.cfg.OnEndpointsUpdate(eps)
						}
					}
				}
			}
		}
	}
}

// ActiveConnection returns the current in-use connection
func (c *Client) ActiveConnection() *grpc.ClientConn { return c.conn }

// Close shuts down the client's etcd connections.
func (c *Client) Close() error {
	c.cancel()

	// Wait for discovery watcher to stop
	if c.watchDone != nil {
		<-c.watchDone
	}

	// Close discovery client
	if c.discovery != nil {
		c.discovery.Close()
	}

	if c.conn != nil {
		return toErr(c.ctx, c.conn.Close())
	}
	return c.ctx.Err()
}

// isHaltErr returns true if the given error and context indicate no forward
// progress can be made, even after reconnecting.
func isHaltErr(ctx context.Context, err error) bool {
	if ctx != nil && ctx.Err() != nil {
		return true
	}
	if err == nil {
		return false
	}
	ev, _ := status.FromError(err)
	// Unavailable codes mean the system will be right back.
	// (e.g., can't connect, lost leader)
	// Treat Internal codes as if something failed, leaving the
	// system in an inconsistent state, but retrying could make progress.
	// (e.g., failed in middle of send, corrupted frame)
	// TODO: are permanent Internal errors possible from grpc?
	return ev.Code() != codes.Unavailable && ev.Code() != codes.Internal
}

// isUnavailableErr returns true if the given error is an unavailable error
func isUnavailableErr(ctx context.Context, err error) bool {
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	if err == nil {
		return false
	}
	ev, ok := status.FromError(err)
	if ok {
		// Unavailable codes mean the system will be right back.
		// (e.g., can't connect, lost leader)
		return ev.Code() == codes.Unavailable
	}
	return false
}

func toErr(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	if ev, ok := status.FromError(err); ok {
		code := ev.Code()
		switch code {
		case codes.DeadlineExceeded:
			fallthrough
		case codes.Canceled:
			if ctx.Err() != nil {
				err = ctx.Err()
			}
		}
	}
	return err
}

func canceledByCaller(stopCtx context.Context, err error) bool {
	if stopCtx.Err() == nil || err == nil {
		return false
	}

	return err == context.Canceled || err == context.DeadlineExceeded
}

// IsConnCanceled returns true, if error is from a closed gRPC connection.
// ref. https://github.com/grpc/grpc-go/pull/1854
func IsConnCanceled(err error) bool {
	if err == nil {
		return false
	}

	// >= gRPC v1.23.x
	s, ok := status.FromError(err)
	if ok {
		// connection is canceled or server has already closed the connection
		return s.Code() == codes.Canceled || s.Message() == "transport is closing"
	}

	// >= gRPC v1.10.x
	if err == context.Canceled {
		return true
	}

	// <= gRPC v1.7.x returns 'errors.New("grpc: the client connection is closing")'
	return strings.Contains(err.Error(), "grpc: the client connection is closing")
}

func authority(endpoint string) string {
	spl := strings.SplitN(endpoint, "://", 2)
	if len(spl) < 2 {
		if strings.HasPrefix(endpoint, "unix:") {
			return endpoint[len("unix:"):]
		}
		if strings.HasPrefix(endpoint, "unixs:") {
			return endpoint[len("unixs:"):]
		}
		return endpoint
	}
	return spl[1]
}
