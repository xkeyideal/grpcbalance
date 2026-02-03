package balancer

import (
	"github.com/xkeyideal/grpcbalance/grpclient/circuitbreaker"
	"github.com/xkeyideal/grpcbalance/grpclient/picker"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/resolver"
)

const RoundRobinBalanceName = "customize_round_robin"

type rrBalance struct {
	pickerBuilder picker.PickerBuilder
	config        Config
}

func RegisterRRBalance(healthCheck bool) {
	rb := &rrBalance{
		pickerBuilder: &picker.RRPickerBuilder{},
		config: Config{
			HealthCheck: healthCheck,
		},
	}

	balancer.Register(rb)
}

// RegisterRRBalanceWithFilter registers round robin balancer with node filter support
func RegisterRRBalanceWithFilter(healthCheck bool) {
	rb := &rrBalance{
		pickerBuilder: picker.NewFilteredPickerBuilder(&picker.RRPickerBuilder{}),
		config: Config{
			HealthCheck: healthCheck,
		},
	}

	balancer.Register(rb)
}

// RegisterRRBalanceWithCircuitBreaker registers round robin balancer with circuit breaker
func RegisterRRBalanceWithCircuitBreaker(healthCheck bool, cbConfig circuitbreaker.Config) {
	rb := &rrBalance{
		pickerBuilder: picker.NewCircuitBreakerPickerBuilder(&picker.RRPickerBuilder{}, cbConfig),
		config: Config{
			HealthCheck: healthCheck,
		},
	}

	balancer.Register(rb)
}

// RegisterRRBalanceWithFilterAndCircuitBreaker registers round robin balancer with both filter and circuit breaker
func RegisterRRBalanceWithFilterAndCircuitBreaker(healthCheck bool, cbConfig circuitbreaker.Config) {
	rb := &rrBalance{
		pickerBuilder: picker.NewFilteredPickerBuilder(
			picker.NewCircuitBreakerPickerBuilder(&picker.RRPickerBuilder{}, cbConfig),
		),
		config: Config{
			HealthCheck: healthCheck,
		},
	}

	balancer.Register(rb)
}

func (b *rrBalance) Build(cc balancer.ClientConn, opt balancer.BuildOptions) balancer.Balancer {
	bal := &baseBalancer{
		cc:            cc,
		pickerBuilder: b.pickerBuilder,

		subConns: resolver.NewAddressMapV2[balancer.SubConn](),
		scStates: make(map[balancer.SubConn]connectivity.State),
		scAddrs:  make(map[balancer.SubConn]resolver.Address),
		csEvltr:  &balancer.ConnectivityStateEvaluator{},
		config:   b.config,
		state:    connectivity.Connecting,
	}

	// Initialize picker to a picker that always returns
	// ErrNoSubConnAvailable, because when state of a SubConn changes, we
	// may call UpdateState with this picker.
	bal.picker = picker.NewErrPicker(balancer.ErrNoSubConnAvailable)
	return bal
}

func (b *rrBalance) Name() string {
	return RoundRobinBalanceName
}
