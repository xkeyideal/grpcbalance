package balancer

import (
	"github.com/xkeyideal/grpcbalance/grpclient/circuitbreaker"
	"github.com/xkeyideal/grpcbalance/grpclient/picker"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/resolver"
)

const P2CBalancerName = "p2c_x"

func RegisterP2CBalance() {
	builder := &p2cBalanceBuilder{
		name:       P2CBalancerName,
		withFilter: false,
	}
	balancer.Register(builder)
}

// RegisterP2CBalanceWithFilter registers P2C balancer with node filter support
func RegisterP2CBalanceWithFilter() {
	builder := &p2cBalanceBuilder{
		name:       P2CBalancerName,
		withFilter: true,
	}
	balancer.Register(builder)
}

func RegisterP2CBalanceWithCircuitBreaker(cbConfig circuitbreaker.Config) {
	builder := &p2cBalanceBuilder{
		name:           P2CBalancerName,
		circuitBreaker: &cbConfig,
		withFilter:     false,
	}
	balancer.Register(builder)
}

// RegisterP2CBalanceWithFilterAndCircuitBreaker registers P2C balancer with both filter and circuit breaker
func RegisterP2CBalanceWithFilterAndCircuitBreaker(cbConfig circuitbreaker.Config) {
	builder := &p2cBalanceBuilder{
		name:           P2CBalancerName,
		circuitBreaker: &cbConfig,
		withFilter:     true,
	}
	balancer.Register(builder)
}

type p2cBalanceBuilder struct {
	name           string
	circuitBreaker *circuitbreaker.Config
	withFilter     bool
}

func (b *p2cBalanceBuilder) Build(cc balancer.ClientConn, opts balancer.BuildOptions) balancer.Balancer {
	var pb picker.PickerBuilder = &picker.P2CPickerBuilder{}
	if b.circuitBreaker != nil {
		pb = picker.NewCircuitBreakerPickerBuilder(pb, *b.circuitBreaker)
	}
	// Wrap with FilteredPickerBuilder only if filter is enabled
	if b.withFilter {
		pb = picker.NewFilteredPickerBuilder(pb)
	}

	bal := &baseBalancer{
		cc:            cc,
		pickerBuilder: pb,

		subConns: resolver.NewAddressMapV2[balancer.SubConn](),
		scStates: make(map[balancer.SubConn]connectivity.State),
		scAddrs:  make(map[balancer.SubConn]resolver.Address),
		csEvltr:  &balancer.ConnectivityStateEvaluator{},
		config:   Config{HealthCheck: true},
		state:    connectivity.Connecting,
	}
	bal.picker = picker.NewErrPicker(balancer.ErrNoSubConnAvailable)
	return bal
}

func (b *p2cBalanceBuilder) Name() string {
	return b.name
}
