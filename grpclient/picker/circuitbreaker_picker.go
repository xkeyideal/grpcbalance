package picker

import (
	"github.com/xkeyideal/grpcbalance/grpclient/circuitbreaker"
	"github.com/xkeyideal/grpcbalance/grpclient/logger"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/resolver"
)

// CircuitBreakerPickerBuilder wraps another PickerBuilder with circuit breaker functionality
type CircuitBreakerPickerBuilder struct {
	inner   PickerBuilder
	manager *circuitbreaker.Manager
	logger  logger.Logger
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

// SetLogger sets the logger for both the wrapper and inner builder
func (pb *CircuitBreakerPickerBuilder) SetLogger(log logger.Logger) {
	pb.logger = log
	if pb.inner != nil {
		pb.inner.SetLogger(log)
	}
}

// Build creates a picker with circuit breaker functionality
func (pb *CircuitBreakerPickerBuilder) Build(info PickerBuildInfo) balancer.Picker {
	log := pb.logger
	if log == nil {
		log = logger.GetDefaultLogger()
	}

	totalCount := len(info.ReadySCs)
	// Filter out endpoints with open circuit breakers
	filteredSCs := make(map[balancer.SubConn]SubConnInfo)
	for sc, scInfo := range info.ReadySCs {
		cb := pb.manager.Get(scInfo.Address.Addr)
		if cb.Allow() {
			filteredSCs[sc] = scInfo
		}
	}

	filteredCount := len(filteredSCs)
	// If all circuits are open, use original list to prevent complete outage
	if filteredCount == 0 {
		filteredSCs = info.ReadySCs
		log.Warnf("CircuitBreaker: all %d SubConns have open circuits, using all to prevent outage", totalCount)
	} else if filteredCount < totalCount {
		log.Infof("CircuitBreaker: filtered %d/%d SubConns with open circuits", totalCount-filteredCount, totalCount)
	} else {
		log.Debugf("CircuitBreaker: all %d SubConns are healthy", totalCount)
	}

	innerPicker := pb.inner.Build(PickerBuildInfo{ReadySCs: filteredSCs})

	return &circuitBreakerPicker{
		inner:   innerPicker,
		manager: pb.manager,
		logger:  log,
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
	logger   logger.Logger
	scToAddr map[balancer.SubConn]string
}

func (p *circuitBreakerPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	result, err := p.inner.Pick(info)
	if err != nil {
		return result, err
	}

	p.logger.Debugf("CircuitBreaker: pick info %s", formatPickInfo(info))

	// Wrap the done callback to track success/failure for circuit breaker
	addr, ok := p.scToAddr[result.SubConn]
	if !ok || addr == "" {
		p.logger.Warnf("CircuitBreaker: missing address for SubConn")
		return result, nil
	}
	cb := p.manager.Get(addr)

	originalDone := result.Done
	result.Done = func(doneInfo balancer.DoneInfo) {
		if doneInfo.Err != nil {
			cb.RecordFailure()
			p.logger.Debugf("CircuitBreaker: recorded failure for %s, info: %s", addr, formatDoneInfo(doneInfo))
		} else {
			cb.RecordSuccess()
			p.logger.Debugf("CircuitBreaker: recorded success for %s, info: %s", addr, formatDoneInfo(doneInfo))
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
	logger       logger.Logger
}

// NewHealthAwarePickerBuilder creates a health-aware picker builder
func NewHealthAwarePickerBuilder(inner PickerBuilder, healthyAddrs func() []string) *HealthAwarePickerBuilder {
	return &HealthAwarePickerBuilder{
		inner:        inner,
		healthyAddrs: healthyAddrs,
	}
}

// SetLogger sets the logger for both the wrapper and inner builder
func (pb *HealthAwarePickerBuilder) SetLogger(log logger.Logger) {
	pb.logger = log
	if pb.inner != nil {
		pb.inner.SetLogger(log)
	}
}

// Build creates a picker that only uses healthy endpoints
func (pb *HealthAwarePickerBuilder) Build(info PickerBuildInfo) balancer.Picker {
	log := pb.logger
	if log == nil {
		log = logger.GetDefaultLogger()
	}

	totalCount := len(info.ReadySCs)
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

	filteredCount := len(filteredSCs)
	// If no healthy endpoints, use all (fallback)
	if filteredCount == 0 {
		filteredSCs = info.ReadySCs
		log.Warnf("HealthAware: no healthy SubConns found, using all %d to prevent outage", totalCount)
	} else if filteredCount < totalCount {
		log.Infof("HealthAware: filtered %d/%d unhealthy SubConns", totalCount-filteredCount, totalCount)
	} else {
		log.Debugf("HealthAware: all %d SubConns are healthy", totalCount)
	}

	return pb.inner.Build(PickerBuildInfo{ReadySCs: filteredSCs})
}

// CombinedPickerBuilder combines circuit breaker and health check functionality
type CombinedPickerBuilder struct {
	inner        PickerBuilder
	cbManager    *circuitbreaker.Manager
	healthyAddrs func() []string
	logger       logger.Logger
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

// SetLogger sets the logger for both the wrapper and inner builder
func (pb *CombinedPickerBuilder) SetLogger(log logger.Logger) {
	pb.logger = log
	if pb.inner != nil {
		pb.inner.SetLogger(log)
	}
}

// Build creates a picker with combined functionality
func (pb *CombinedPickerBuilder) Build(info PickerBuildInfo) balancer.Picker {
	log := pb.logger
	if log == nil {
		log = logger.GetDefaultLogger()
	}

	totalCount := len(info.ReadySCs)
	// Get healthy addresses
	healthySet := make(map[string]bool)
	if pb.healthyAddrs != nil {
		for _, addr := range pb.healthyAddrs() {
			healthySet[addr] = true
		}
	}

	// Filter endpoints based on health and circuit breaker
	filteredSCs := make(map[balancer.SubConn]SubConnInfo)
	unhealthyCount := 0
	circuitOpenCount := 0
	for sc, scInfo := range info.ReadySCs {
		addr := scInfo.Address.Addr

		// Check health (if health check is configured)
		isHealthy := len(healthySet) == 0 || healthySet[addr]
		if !isHealthy {
			unhealthyCount++
		}

		// Check circuit breaker
		cb := pb.cbManager.Get(addr)
		cbAllows := cb.Allow()
		if !cbAllows {
			circuitOpenCount++
		}

		if isHealthy && cbAllows {
			filteredSCs[sc] = scInfo
		}
	}

	filteredCount := len(filteredSCs)
	// Fallback: if all filtered out, use original list
	if filteredCount == 0 {
		filteredSCs = info.ReadySCs
		log.Warnf("Combined: all %d SubConns filtered (unhealthy:%d, circuit-open:%d), using all to prevent outage",
			totalCount, unhealthyCount, circuitOpenCount)
	} else if filteredCount < totalCount {
		log.Infof("Combined: filtered %d/%d SubConns (unhealthy:%d, circuit-open:%d)",
			totalCount-filteredCount, totalCount, unhealthyCount, circuitOpenCount)
	} else {
		log.Debugf("Combined: all %d SubConns are healthy with closed circuits", totalCount)
	}

	innerPicker := pb.inner.Build(PickerBuildInfo{ReadySCs: filteredSCs})

	return &circuitBreakerPicker{
		inner:   innerPicker,
		manager: pb.cbManager,
		logger:  log,
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
