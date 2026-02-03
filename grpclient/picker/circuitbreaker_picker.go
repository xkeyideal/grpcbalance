package picker

import (
	"github.com/xkeyideal/grpcbalance/grpclient/circuitbreaker"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/resolver"
)

// CircuitBreakerPickerBuilder wraps another PickerBuilder with circuit breaker functionality
type CircuitBreakerPickerBuilder struct {
	inner   PickerBuilder
	manager *circuitbreaker.Manager
}

// NewCircuitBreakerPickerBuilder creates a new circuit breaker enabled picker builder
func NewCircuitBreakerPickerBuilder(inner PickerBuilder, config circuitbreaker.Config) *CircuitBreakerPickerBuilder {
	return &CircuitBreakerPickerBuilder{
		inner:   inner,
		manager: circuitbreaker.NewManager(config),
	}
}

// NewCircuitBreakerPickerBuilderWithDefaults creates a new circuit breaker enabled picker builder with default config
func NewCircuitBreakerPickerBuilderWithDefaults(inner PickerBuilder) *CircuitBreakerPickerBuilder {
	return &CircuitBreakerPickerBuilder{
		inner:   inner,
		manager: circuitbreaker.NewManagerWithDefaults(),
	}
}

// Build creates a picker with circuit breaker functionality
func (pb *CircuitBreakerPickerBuilder) Build(info PickerBuildInfo) balancer.Picker {
	// Filter out endpoints with open circuit breakers
	filteredSCs := make(map[balancer.SubConn]SubConnInfo)
	for sc, scInfo := range info.ReadySCs {
		cb := pb.manager.Get(scInfo.Address.Addr)
		if cb.Allow() {
			filteredSCs[sc] = scInfo
		}
	}

	// If all circuits are open, use original list to prevent complete outage
	if len(filteredSCs) == 0 {
		filteredSCs = info.ReadySCs
	}

	innerPicker := pb.inner.Build(PickerBuildInfo{ReadySCs: filteredSCs})

	return &circuitBreakerPicker{
		inner:   innerPicker,
		manager: pb.manager,
		scToAddr: func() map[balancer.SubConn]string {
			m := make(map[balancer.SubConn]string)
			for sc, scInfo := range info.ReadySCs {
				m[sc] = scInfo.Address.Addr
			}
			return m
		}(),
	}
}

// Manager returns the circuit breaker manager for external access
func (pb *CircuitBreakerPickerBuilder) Manager() *circuitbreaker.Manager {
	return pb.manager
}

type circuitBreakerPicker struct {
	inner    balancer.Picker
	manager  *circuitbreaker.Manager
	scToAddr map[balancer.SubConn]string
}

func (p *circuitBreakerPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	result, err := p.inner.Pick(info)
	if err != nil {
		return result, err
	}

	// Wrap the done callback to track success/failure for circuit breaker
	addr := p.scToAddr[result.SubConn]
	cb := p.manager.Get(addr)

	originalDone := result.Done
	result.Done = func(doneInfo balancer.DoneInfo) {
		if doneInfo.Err != nil {
			cb.RecordFailure()
		} else {
			cb.RecordSuccess()
		}

		if originalDone != nil {
			originalDone(doneInfo)
		}
	}

	return result, nil
}

// HealthAwarePickerBuilder wraps another PickerBuilder with health check awareness
type HealthAwarePickerBuilder struct {
	inner        PickerBuilder
	healthyAddrs func() []string // Function to get healthy addresses
}

// NewHealthAwarePickerBuilder creates a health-aware picker builder
func NewHealthAwarePickerBuilder(inner PickerBuilder, healthyAddrs func() []string) *HealthAwarePickerBuilder {
	return &HealthAwarePickerBuilder{
		inner:        inner,
		healthyAddrs: healthyAddrs,
	}
}

// Build creates a picker that only uses healthy endpoints
func (pb *HealthAwarePickerBuilder) Build(info PickerBuildInfo) balancer.Picker {
	healthySet := make(map[string]bool)
	for _, addr := range pb.healthyAddrs() {
		healthySet[addr] = true
	}

	// Filter to only healthy endpoints
	filteredSCs := make(map[balancer.SubConn]SubConnInfo)
	for sc, scInfo := range info.ReadySCs {
		if healthySet[scInfo.Address.Addr] || len(healthySet) == 0 {
			filteredSCs[sc] = scInfo
		}
	}

	// If no healthy endpoints, use all (fallback)
	if len(filteredSCs) == 0 {
		filteredSCs = info.ReadySCs
	}

	return pb.inner.Build(PickerBuildInfo{ReadySCs: filteredSCs})
}

// CombinedPickerBuilder combines circuit breaker and health check functionality
type CombinedPickerBuilder struct {
	inner        PickerBuilder
	cbManager    *circuitbreaker.Manager
	healthyAddrs func() []string
}

// NewCombinedPickerBuilder creates a picker builder with both circuit breaker and health check
func NewCombinedPickerBuilder(
	inner PickerBuilder,
	cbConfig circuitbreaker.Config,
	healthyAddrs func() []string,
) *CombinedPickerBuilder {
	return &CombinedPickerBuilder{
		inner:        inner,
		cbManager:    circuitbreaker.NewManager(cbConfig),
		healthyAddrs: healthyAddrs,
	}
}

// Build creates a picker with combined functionality
func (pb *CombinedPickerBuilder) Build(info PickerBuildInfo) balancer.Picker {
	// Get healthy addresses
	healthySet := make(map[string]bool)
	if pb.healthyAddrs != nil {
		for _, addr := range pb.healthyAddrs() {
			healthySet[addr] = true
		}
	}

	// Filter endpoints based on health and circuit breaker
	filteredSCs := make(map[balancer.SubConn]SubConnInfo)
	for sc, scInfo := range info.ReadySCs {
		addr := scInfo.Address.Addr

		// Check health (if health check is configured)
		isHealthy := len(healthySet) == 0 || healthySet[addr]

		// Check circuit breaker
		cb := pb.cbManager.Get(addr)
		cbAllows := cb.Allow()

		if isHealthy && cbAllows {
			filteredSCs[sc] = scInfo
		}
	}

	// Fallback: if all filtered out, use original list
	if len(filteredSCs) == 0 {
		filteredSCs = info.ReadySCs
	}

	innerPicker := pb.inner.Build(PickerBuildInfo{ReadySCs: filteredSCs})

	return &circuitBreakerPicker{
		inner:   innerPicker,
		manager: pb.cbManager,
		scToAddr: func() map[balancer.SubConn]string {
			m := make(map[balancer.SubConn]string)
			for sc, scInfo := range info.ReadySCs {
				m[sc] = scInfo.Address.Addr
			}
			return m
		}(),
	}
}

// CircuitBreakerManager returns the circuit breaker manager
func (pb *CombinedPickerBuilder) CircuitBreakerManager() *circuitbreaker.Manager {
	return pb.cbManager
}

// SubConnInfoWithAddr is a helper to extract address from SubConnInfo
func SubConnInfoWithAddr(info SubConnInfo) resolver.Address {
	return info.Address
}
