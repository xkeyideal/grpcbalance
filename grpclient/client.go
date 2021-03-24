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
	"github.com/xkeyideal/grpcbalance/grpclient/resolver"

	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

func init() {
	//balancer.RegisterWRRBalance(true)
}

// Client provides and manages an etcd v3 client session.
type Client struct {
	conn *grpc.ClientConn

	cfg      Config
	resolver *resolver.CustomizeResolver
	mu       *sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc

	callOpts []grpc.CallOption
}

// Close shuts down the client's etcd connections.
func (c *Client) Close() error {
	c.cancel()
	if c.conn != nil {
		return toErr(c.ctx, c.conn.Close())
	}
	return c.ctx.Err()
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
	opts = append(opts, grpc.WithInsecure(), grpc.WithInitialWindowSize(65536*100)) // 100*64K

	// 设置拦截器
	retryOpts := []grpc_retry.CallOption{
		grpc_retry.WithMax(3),
		grpc_retry.WithBackoff(grpc_retry.BackoffExponential(100 * time.Millisecond)),
		grpc_retry.WithCodes(codes.Canceled, codes.Internal, codes.Unavailable),
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

	dctx := c.ctx
	if c.cfg.DialTimeout > 0 {
		var cancel context.CancelFunc
		dctx, cancel = context.WithTimeout(c.ctx, c.cfg.DialTimeout)
		defer cancel()

		// Is this right for cases where grpc.WithBlock() is not set on the dial options
		// 设置连接超时的时候，需要明确等连接创建成功之后
		opts = append(opts, grpc.WithBlock())
	}

	initialEndpoints := strings.Join(c.cfg.Endpoints, ";")
	target := fmt.Sprintf("%s://%p/#initially=[%s]", resolver.Scheme, c, initialEndpoints)
	conn, err := grpc.DialContext(dctx, target, opts...)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func NewClient(cfg *Config) (*Client, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}

	// default load balance algorithm round robin
	switch cfg.BalanceName {
	case balancer.WeightedRobinBalanceName:
		balancer.RegisterWRRBalance(true)
	case balancer.RandomWeightedRobinBalanceName:
		balancer.RegisterRWRRBalance(true)
	default:
		cfg.BalanceName = balancer.RoundRobinBalanceName
		balancer.RegisterRRBalance(true)
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

	if cfg.MaxCallSendMsgSize > 0 || cfg.MaxCallRecvMsgSize > 0 {
		if cfg.MaxCallRecvMsgSize > 0 && cfg.MaxCallSendMsgSize > cfg.MaxCallRecvMsgSize {
			return nil, fmt.Errorf("gRPC message recv limit (%d bytes) must be greater than send limit (%d bytes)", cfg.MaxCallRecvMsgSize, cfg.MaxCallSendMsgSize)
		}
		callOpts := []grpc.CallOption{
			defaultFailFast,
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

	client.resolver = resolver.NewCustomizeResolver(cfg.Endpoints, cfg.Attributes)

	if len(cfg.Endpoints) < 1 {
		client.cancel()
		return nil, fmt.Errorf("at least one Endpoint is required in client config")
	}
	// Use a provided endpoint target so that for https:// without any tls config given, then
	// grpc will assume the certificate server name is the endpoint host.
	conn, err := client.dialWithBalancer(grpc.WithBalancerName(cfg.BalanceName))
	if err != nil {
		client.cancel()
		client.resolver.Close()
		// TODO: Error like `fmt.Errorf(dialing [%s] failed: %v, strings.Join(cfg.Endpoints, ";"), err)` would help with debugging a lot.
		return nil, err
	}
	client.conn = conn

	return client, nil
}

// ActiveConnection returns the current in-use connection
func (c *Client) ActiveConnection() *grpc.ClientConn { return c.conn }

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
