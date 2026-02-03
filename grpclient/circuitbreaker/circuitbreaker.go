// Package circuitbreaker implements a circuit breaker pattern for gRPC connections.
// It helps prevent cascading failures by temporarily disabling unhealthy endpoints.
package circuitbreaker

import (
	"sync"
	"time"
)

// State represents the circuit breaker state
type State int

const (
	// StateClosed - circuit breaker is closed, requests are allowed
	StateClosed State = iota
	// StateOpen - circuit breaker is open, requests are blocked
	StateOpen
	// StateHalfOpen - circuit breaker is half-open, limited requests are allowed to test recovery
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Config holds the configuration for a circuit breaker
type Config struct {
	// FailureThreshold is the number of consecutive failures before opening the circuit
	FailureThreshold int
	// SuccessThreshold is the number of consecutive successes in half-open state to close the circuit
	SuccessThreshold int
	// OpenTimeout is how long the circuit stays open before transitioning to half-open
	OpenTimeout time.Duration
	// HalfOpenMaxRequests is the max number of requests allowed in half-open state
	HalfOpenMaxRequests int
}

// DefaultConfig returns default circuit breaker configuration
func DefaultConfig() Config {
	return Config{
		FailureThreshold:    5,
		SuccessThreshold:    2,
		OpenTimeout:         30 * time.Second,
		HalfOpenMaxRequests: 1,
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	mu               sync.RWMutex
	state            State
	failureCount     int
	successCount     int
	halfOpenRequests int
	lastStateChange  time.Time
	config           Config
}

// New creates a new circuit breaker with the given configuration
func New(config Config) *CircuitBreaker {
	return &CircuitBreaker{
		state:           StateClosed,
		lastStateChange: time.Now(),
		config:          config,
	}
}

// NewWithDefaults creates a new circuit breaker with default configuration
func NewWithDefaults() *CircuitBreaker {
	return New(DefaultConfig())
}

// State returns the current state of the circuit breaker
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Allow checks if a request is allowed to proceed
// Returns true if allowed, false if the circuit is open
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case StateClosed:
		return true

	case StateOpen:
		// Check if timeout has passed to transition to half-open
		if now.Sub(cb.lastStateChange) >= cb.config.OpenTimeout {
			cb.toHalfOpen()
			cb.halfOpenRequests++
			return true
		}
		return false

	case StateHalfOpen:
		// Allow limited requests in half-open state
		if cb.halfOpenRequests < cb.config.HalfOpenMaxRequests {
			cb.halfOpenRequests++
			return true
		}
		return false
	}

	return false
}

// RecordSuccess records a successful request
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount = 0

	switch cb.state {
	case StateHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.config.SuccessThreshold {
			cb.toClosed()
		}
	case StateClosed:
		// Already closed, reset success count
		cb.successCount = 0
	}
}

// RecordFailure records a failed request
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.successCount = 0

	switch cb.state {
	case StateClosed:
		cb.failureCount++
		if cb.failureCount >= cb.config.FailureThreshold {
			cb.toOpen()
		}
	case StateHalfOpen:
		// Any failure in half-open state opens the circuit again
		cb.toOpen()
	}
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.toClosed()
}

func (cb *CircuitBreaker) toClosed() {
	cb.state = StateClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.halfOpenRequests = 0
	cb.lastStateChange = time.Now()
}

func (cb *CircuitBreaker) toOpen() {
	cb.state = StateOpen
	cb.failureCount = 0
	cb.successCount = 0
	cb.halfOpenRequests = 0
	cb.lastStateChange = time.Now()
}

func (cb *CircuitBreaker) toHalfOpen() {
	cb.state = StateHalfOpen
	cb.failureCount = 0
	cb.successCount = 0
	cb.halfOpenRequests = 0
	cb.lastStateChange = time.Now()
}

// Manager manages circuit breakers for multiple endpoints
type Manager struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
	config   Config
}

// NewManager creates a new circuit breaker manager
func NewManager(config Config) *Manager {
	return &Manager{
		breakers: make(map[string]*CircuitBreaker),
		config:   config,
	}
}

// NewManagerWithDefaults creates a new circuit breaker manager with default config
func NewManagerWithDefaults() *Manager {
	return NewManager(DefaultConfig())
}

// Get returns the circuit breaker for the given address, creating one if necessary
func (m *Manager) Get(addr string) *CircuitBreaker {
	m.mu.RLock()
	cb, ok := m.breakers[addr]
	m.mu.RUnlock()

	if ok {
		return cb
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if cb, ok = m.breakers[addr]; ok {
		return cb
	}

	cb = New(m.config)
	m.breakers[addr] = cb
	return cb
}

// Remove removes the circuit breaker for the given address
func (m *Manager) Remove(addr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.breakers, addr)
}

// Reset resets all circuit breakers
func (m *Manager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, cb := range m.breakers {
		cb.Reset()
	}
}

// GetOpenCircuits returns a list of addresses with open circuits
func (m *Manager) GetOpenCircuits() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []string
	for addr, cb := range m.breakers {
		if cb.State() == StateOpen {
			result = append(result, addr)
		}
	}
	return result
}
