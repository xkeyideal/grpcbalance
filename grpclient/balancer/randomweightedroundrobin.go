package balancer

import (
	"github.com/xkeyideal/grpcbalance/grpclient/circuitbreaker"
	"github.com/xkeyideal/grpcbalance/grpclient/picker"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/resolver"
)

const RandomWeightedRobinBalanceName = "customize_random_weighted_round_robin"

type rwrrBalance struct {
	pickerBuilder picker.PickerBuilder
	config        Config
}

func RegisterRWRRBalance(healthCheck bool) {
	rb := &rwrrBalance{
		pickerBuilder: &picker.RWRRPickerBuilder{},
		config: Config{
			HealthCheck: healthCheck,
		},
	}

	balancer.Register(rb)
}

// RegisterRWRRBalanceWithFilter registers random weighted round robin balancer with node filter support
func RegisterRWRRBalanceWithFilter(healthCheck bool) {
	rb := &rwrrBalance{
		pickerBuilder: picker.NewFilteredPickerBuilder(&picker.RWRRPickerBuilder{}),
		config: Config{
			HealthCheck: healthCheck,
		},
	}

	balancer.Register(rb)
}

// RegisterRWRRBalanceWithCircuitBreaker registers random weighted round robin balancer with circuit breaker
func RegisterRWRRBalanceWithCircuitBreaker(healthCheck bool, cbConfig circuitbreaker.Config) {
	rb := &rwrrBalance{
		pickerBuilder: picker.NewCircuitBreakerPickerBuilder(&picker.RWRRPickerBuilder{}, cbConfig),
		config: Config{
			HealthCheck: healthCheck,
		},
	}

	balancer.Register(rb)
}

// RegisterRWRRBalanceWithFilterAndCircuitBreaker registers random weighted round robin balancer with both filter and circuit breaker
func RegisterRWRRBalanceWithFilterAndCircuitBreaker(healthCheck bool, cbConfig circuitbreaker.Config) {
	rb := &rwrrBalance{
		pickerBuilder: picker.NewFilteredPickerBuilder(
			picker.NewCircuitBreakerPickerBuilder(&picker.RWRRPickerBuilder{}, cbConfig),
		),
		config: Config{
			HealthCheck: healthCheck,
		},
	}

	balancer.Register(rb)
}

func (b *rwrrBalance) Build(cc balancer.ClientConn, opt balancer.BuildOptions) balancer.Balancer {
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

func (b *rwrrBalance) Name() string {
	return RandomWeightedRobinBalanceName
}
