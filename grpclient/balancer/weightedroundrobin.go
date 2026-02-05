package balancer

import (
	"github.com/xkeyideal/grpcbalance/grpclient/circuitbreaker"
	"github.com/xkeyideal/grpcbalance/grpclient/logger"
	"github.com/xkeyideal/grpcbalance/grpclient/picker"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/resolver"
)

const WeightedRobinBalanceName = "x_customize_weighted_round_robin"

type wrrBalance struct {
	pickerBuilder picker.PickerBuilder
	config        Config

	// Logger is the logger for this balancer. If nil, the default logger will be used.
	Logger logger.Logger
}

func RegisterWRRBalance(healthCheck bool, log logger.Logger) {
	rb := &wrrBalance{
		pickerBuilder: &picker.WRRPickerBuilder{},
		config: Config{
			HealthCheck: healthCheck,
		},
		Logger: log,
	}

	balancer.Register(rb)
}

// RegisterWRRBalanceWithFilter registers weighted round robin balancer with node filter support
func RegisterWRRBalanceWithFilter(healthCheck bool, log logger.Logger) {
	rb := &wrrBalance{
		pickerBuilder: picker.NewFilteredPickerBuilder(&picker.WRRPickerBuilder{}),
		config: Config{
			HealthCheck: healthCheck,
		},
		Logger: log,
	}

	balancer.Register(rb)
}

// RegisterWRRBalanceWithCircuitBreaker registers weighted round robin balancer with circuit breaker
func RegisterWRRBalanceWithCircuitBreaker(healthCheck bool, cbConfig circuitbreaker.Config, log logger.Logger) {
	rb := &wrrBalance{
		pickerBuilder: picker.NewCircuitBreakerPickerBuilder(&picker.WRRPickerBuilder{}, cbConfig),
		config: Config{
			HealthCheck: healthCheck,
		},
		Logger: log,
	}

	balancer.Register(rb)
}

// RegisterWRRBalanceWithFilterAndCircuitBreaker registers weighted round robin balancer with both filter and circuit breaker
func RegisterWRRBalanceWithFilterAndCircuitBreaker(healthCheck bool, cbConfig circuitbreaker.Config, log logger.Logger) {
	rb := &wrrBalance{
		pickerBuilder: picker.NewFilteredPickerBuilder(
			picker.NewCircuitBreakerPickerBuilder(&picker.WRRPickerBuilder{}, cbConfig),
		),
		config: Config{
			HealthCheck: healthCheck,
		},
		Logger: log,
	}

	balancer.Register(rb)
}

func (b *wrrBalance) Build(cc balancer.ClientConn, opt balancer.BuildOptions) balancer.Balancer {
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

func (b *wrrBalance) Name() string {
	return WeightedRobinBalanceName
}
