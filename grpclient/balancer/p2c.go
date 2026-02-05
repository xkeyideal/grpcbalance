package balancer

import (
	"github.com/xkeyideal/grpcbalance/grpclient/circuitbreaker"
	"github.com/xkeyideal/grpcbalance/grpclient/logger"
	"github.com/xkeyideal/grpcbalance/grpclient/picker"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/resolver"
)

const P2CBalancerName = "x_customize_p2c"

func RegisterP2CBalance(healthCheck bool, log logger.Logger) {
	builder := &p2cBalance{
		pickerBuilder: &picker.P2CPickerBuilder{},
		config: Config{
			HealthCheck: healthCheck,
		},
		Logger: log,
	}

	balancer.Register(builder)
}

// RegisterP2CBalanceWithFilter registers P2C balancer with node filter support
func RegisterP2CBalanceWithFilter(healthCheck bool, log logger.Logger) {
	builder := &p2cBalance{
		pickerBuilder: picker.NewFilteredPickerBuilder(&picker.P2CPickerBuilder{}),
		config: Config{
			HealthCheck: healthCheck,
		},
		Logger: log,
	}
	balancer.Register(builder)
}

func RegisterP2CBalanceWithCircuitBreaker(healthCheck bool, cbConfig circuitbreaker.Config, log logger.Logger) {
	builder := &p2cBalance{
		pickerBuilder: picker.NewCircuitBreakerPickerBuilder(&picker.P2CPickerBuilder{}, cbConfig),
		config: Config{
			HealthCheck: healthCheck,
		},
		Logger: log,
	}
	balancer.Register(builder)
}

// RegisterP2CBalanceWithFilterAndCircuitBreaker registers P2C balancer with both filter and circuit breaker
func RegisterP2CBalanceWithFilterAndCircuitBreaker(healthCheck bool, cbConfig circuitbreaker.Config, log logger.Logger) {
	builder := &p2cBalance{
		pickerBuilder: picker.NewFilteredPickerBuilder(
			picker.NewCircuitBreakerPickerBuilder(&picker.P2CPickerBuilder{}, cbConfig),
		),
		config: Config{
			HealthCheck: healthCheck,
		},
		Logger: log,
	}
	balancer.Register(builder)
}

type p2cBalance struct {
	pickerBuilder picker.PickerBuilder
	config        Config

	// Logger is the logger for this balancer. If nil, the default logger will be used.
	Logger logger.Logger
}

func (b *p2cBalance) Build(cc balancer.ClientConn, opts balancer.BuildOptions) balancer.Balancer {
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
		config:   Config{HealthCheck: b.config.HealthCheck},
		state:    connectivity.Connecting,
	}

	bal.picker = picker.NewErrPicker(balancer.ErrNoSubConnAvailable)
	return bal
}

func (b *p2cBalance) Name() string {
	return P2CBalancerName
}
