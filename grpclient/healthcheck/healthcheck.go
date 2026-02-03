// Package healthcheck provides active health checking for gRPC endpoints.
package healthcheck

import (
	"context"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// Status represents the health status of an endpoint
type Status int

const (
	StatusUnknown Status = iota
	StatusHealthy
	StatusUnhealthy
)

func (s Status) String() string {
	switch s {
	case StatusHealthy:
		return "healthy"
	case StatusUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// Config holds the configuration for health checker
type Config struct {
	// CheckInterval is how often to check endpoint health
	CheckInterval time.Duration
	// CheckTimeout is the timeout for each health check
	CheckTimeout time.Duration
	// UnhealthyThreshold is the number of consecutive failures before marking unhealthy
	UnhealthyThreshold int
	// HealthyThreshold is the number of consecutive successes before marking healthy
	HealthyThreshold int
}

// DefaultConfig returns default health check configuration
func DefaultConfig() Config {
	return Config{
		CheckInterval:      10 * time.Second,
		CheckTimeout:       3 * time.Second,
		UnhealthyThreshold: 3,
		HealthyThreshold:   2,
	}
}

// EndpointHealth tracks the health of a single endpoint
type EndpointHealth struct {
	mu               sync.RWMutex
	addr             string
	status           Status
	consecutiveFails int
	consecutiveOK    int
	lastCheck        time.Time
	lastError        error
}

// Status returns the current health status
func (eh *EndpointHealth) Status() Status {
	eh.mu.RLock()
	defer eh.mu.RUnlock()
	return eh.status
}

// LastError returns the last error encountered
func (eh *EndpointHealth) LastError() error {
	eh.mu.RLock()
	defer eh.mu.RUnlock()
	return eh.lastError
}

// Checker performs active health checks on endpoints
type Checker struct {
	mu        sync.RWMutex
	config    Config
	endpoints map[string]*EndpointHealth
	stopCh    chan struct{}
	wg        sync.WaitGroup
	running   bool

	// OnStatusChange is called when an endpoint's health status changes
	OnStatusChange func(addr string, oldStatus, newStatus Status)
}

// NewChecker creates a new health checker with the given configuration
func NewChecker(config Config) *Checker {
	return &Checker{
		config:    config,
		endpoints: make(map[string]*EndpointHealth),
		stopCh:    make(chan struct{}),
	}
}

// NewCheckerWithDefaults creates a new health checker with default configuration
func NewCheckerWithDefaults() *Checker {
	return NewChecker(DefaultConfig())
}

// AddEndpoint adds an endpoint to be health checked
func (c *Checker) AddEndpoint(addr string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.endpoints[addr]; !exists {
		c.endpoints[addr] = &EndpointHealth{
			addr:   addr,
			status: StatusUnknown,
		}
	}
}

// RemoveEndpoint removes an endpoint from health checking
func (c *Checker) RemoveEndpoint(addr string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.endpoints, addr)
}

// SetEndpoints sets the full list of endpoints to check
func (c *Checker) SetEndpoints(addrs []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create a set of new addresses
	newAddrs := make(map[string]bool)
	for _, addr := range addrs {
		newAddrs[addr] = true
	}

	// Remove endpoints not in new list
	for addr := range c.endpoints {
		if !newAddrs[addr] {
			delete(c.endpoints, addr)
		}
	}

	// Add new endpoints
	for _, addr := range addrs {
		if _, exists := c.endpoints[addr]; !exists {
			c.endpoints[addr] = &EndpointHealth{
				addr:   addr,
				status: StatusUnknown,
			}
		}
	}
}

// GetHealthyEndpoints returns a list of healthy endpoint addresses
func (c *Checker) GetHealthyEndpoints() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []string
	for addr, health := range c.endpoints {
		if health.Status() == StatusHealthy || health.Status() == StatusUnknown {
			result = append(result, addr)
		}
	}
	return result
}

// GetUnhealthyEndpoints returns a list of unhealthy endpoint addresses
func (c *Checker) GetUnhealthyEndpoints() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []string
	for addr, health := range c.endpoints {
		if health.Status() == StatusUnhealthy {
			result = append(result, addr)
		}
	}
	return result
}

// IsHealthy returns whether an endpoint is healthy
func (c *Checker) IsHealthy(addr string) bool {
	c.mu.RLock()
	health, exists := c.endpoints[addr]
	c.mu.RUnlock()

	if !exists {
		return true // Unknown endpoints are considered healthy
	}

	status := health.Status()
	return status == StatusHealthy || status == StatusUnknown
}

// Start begins the health check loop
func (c *Checker) Start() {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.stopCh = make(chan struct{})
	c.mu.Unlock()

	c.wg.Add(1)
	go c.loop()
}

// Stop stops the health check loop
func (c *Checker) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	c.running = false
	close(c.stopCh)
	c.mu.Unlock()

	c.wg.Wait()
}

func (c *Checker) loop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.CheckInterval)
	defer ticker.Stop()

	// Initial check
	c.checkAll()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.checkAll()
		}
	}
}

func (c *Checker) checkAll() {
	c.mu.RLock()
	endpoints := make([]*EndpointHealth, 0, len(c.endpoints))
	for _, health := range c.endpoints {
		endpoints = append(endpoints, health)
	}
	c.mu.RUnlock()

	var wg sync.WaitGroup
	for _, health := range endpoints {
		wg.Add(1)
		go func(h *EndpointHealth) {
			defer wg.Done()
			c.checkEndpoint(h)
		}(health)
	}
	wg.Wait()
}

func (c *Checker) checkEndpoint(health *EndpointHealth) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CheckTimeout)
	defer cancel()

	err := c.doHealthCheck(ctx, health.addr)

	health.mu.Lock()
	oldStatus := health.status
	health.lastCheck = time.Now()

	if err != nil {
		health.lastError = err
		health.consecutiveOK = 0
		health.consecutiveFails++
		if health.consecutiveFails >= c.config.UnhealthyThreshold {
			health.status = StatusUnhealthy
		}
	} else {
		health.lastError = nil
		health.consecutiveFails = 0
		health.consecutiveOK++
		if health.consecutiveOK >= c.config.HealthyThreshold {
			health.status = StatusHealthy
		}
	}
	newStatus := health.status
	health.mu.Unlock()

	// Notify status change
	if oldStatus != newStatus && c.OnStatusChange != nil {
		c.OnStatusChange(health.addr, oldStatus, newStatus)
	}
}

func (c *Checker) doHealthCheck(ctx context.Context, addr string) error {
	// Use grpc.NewClient instead of deprecated grpc.DialContext
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Trigger connection and wait for it to be ready
	conn.Connect()
	if !conn.WaitForStateChange(ctx, connectivity.Idle) {
		state := conn.GetState()
		if state != connectivity.Ready {
			return &UnhealthyError{State: state}
		}
	}

	// Check connection state
	state := conn.GetState()
	if state != connectivity.Ready && state != connectivity.Idle {
		return &UnhealthyError{State: state}
	}

	return nil
}

// UnhealthyError represents an unhealthy connection state
type UnhealthyError struct {
	State connectivity.State
}

func (e *UnhealthyError) Error() string {
	return "unhealthy connection state: " + e.State.String()
}
