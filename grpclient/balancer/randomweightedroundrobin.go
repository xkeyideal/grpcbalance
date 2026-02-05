package balancer

import (
	"github.com/xkeyideal/grpcbalance/grpclient/circuitbreaker"
	"github.com/xkeyideal/grpcbalance/grpclient/logger"
	"github.com/xkeyideal/grpcbalance/grpclient/picker"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/resolver"
)

const RandomWeightedRobinBalanceName = "x_customize_random_weighted_round_robin"

type rwrrBalance struct {
	pickerBuilder picker.PickerBuilder
	config        Config

	// Logger is the logger for this balancer. If nil, the default logger will be used.
	Logger logger.Logger
}

func RegisterRwrrBalance(healthCheck bool, log logger.Logger) {
	rb := &rwrrBalance{
		pickerBuilder: &picker.RWRRPickerBuilder{},
		config: Config{
			HealthCheck: healthCheck,
		},
		Logger: log,
	}

	balancer.Register(rb)
}

// RegisterRWRRBalanceWithFilter registers random weighted round robin balancer with node filter support
func RegisterRwrrBalanceWithFilter(healthCheck bool, log logger.Logger) {
	rb := &rwrrBalance{
		pickerBuilder: picker.NewFilteredPickerBuilder(&picker.RWRRPickerBuilder{}),
		config: Config{
			HealthCheck: healthCheck,
		},
		Logger: log,
	}

	balancer.Register(rb)
}

// RegisterRWRRBalanceWithCircuitBreaker registers random weighted round robin balancer with circuit breaker
func RegisterRwrrBalanceWithCircuitBreaker(healthCheck bool, cbConfig circuitbreaker.Config, log logger.Logger) {
	rb := &rwrrBalance{
		pickerBuilder: picker.NewCircuitBreakerPickerBuilder(&picker.RWRRPickerBuilder{}, cbConfig),
		config: Config{
			HealthCheck: healthCheck,
		},
		Logger: log,
	}

	balancer.Register(rb)
}

// RegisterRWRRBalanceWithFilterAndCircuitBreaker registers random weighted round robin balancer with both filter and circuit breaker
func RegisterRwrrBalanceWithFilterAndCircuitBreaker(healthCheck bool, cbConfig circuitbreaker.Config, log logger.Logger) {
	rb := &rwrrBalance{
		pickerBuilder: picker.NewFilteredPickerBuilder(
			picker.NewCircuitBreakerPickerBuilder(&picker.RWRRPickerBuilder{}, cbConfig),
		),
		config: Config{
			HealthCheck: healthCheck,
		},
		Logger: log,
	}

	balancer.Register(rb)
}

func (b *rwrrBalance) Build(cc balancer.ClientConn, opt balancer.BuildOptions) balancer.Balancer {
	log := b.Logger
	if log == nil {
		log = logger.GetDefaultLogger()
	}

	bal := &baseBalancer{
		cc:            cc,
		pickerBuilder: b.pickerBuilder,
		logger:        log,

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
