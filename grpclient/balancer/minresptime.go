package balancer

import (
	"github.com/xkeyideal/grpcbalance/grpclient/circuitbreaker"
	"github.com/xkeyideal/grpcbalance/grpclient/logger"
	"github.com/xkeyideal/grpcbalance/grpclient/picker"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/resolver"
)

const MinRespTimeBalanceName = "x_customize_min_resp_time"

type mrtBalance struct {
	pickerBuilder picker.PickerBuilder
	config        Config

	// Logger is the logger for this balancer. If nil, the default logger will be used.
	Logger logger.Logger
}

func RegisterMrtBalance(healthCheck bool, log logger.Logger) {
	rb := &mrtBalance{
		pickerBuilder: &picker.MrtPickerBuilder{},
		config: Config{
			HealthCheck: healthCheck,
		},
		Logger: log,
	}

	balancer.Register(rb)
}

// RegisterMrtBalanceWithFilter registers min response time balancer with node filter support
func RegisterMrtBalanceWithFilter(healthCheck bool, log logger.Logger) {
	rb := &mrtBalance{
		pickerBuilder: picker.NewFilteredPickerBuilder(&picker.MrtPickerBuilder{}),
		config: Config{
			HealthCheck: healthCheck,
		},
		Logger: log,
	}

	balancer.Register(rb)
}

// RegisterMrtBalanceWithCircuitBreaker registers min response time balancer with circuit breaker
func RegisterMrtBalanceWithCircuitBreaker(healthCheck bool, cbConfig circuitbreaker.Config, log logger.Logger) {
	rb := &mrtBalance{
		pickerBuilder: picker.NewCircuitBreakerPickerBuilder(&picker.MrtPickerBuilder{}, cbConfig),
		config: Config{
			HealthCheck: healthCheck,
		},
		Logger: log,
	}

	balancer.Register(rb)
}

// RegisterMrtBalanceWithFilterAndCircuitBreaker registers min response time balancer with both filter and circuit breaker
func RegisterMrtBalanceWithFilterAndCircuitBreaker(healthCheck bool, cbConfig circuitbreaker.Config, log logger.Logger) {
	rb := &mrtBalance{
		pickerBuilder: picker.NewFilteredPickerBuilder(
			picker.NewCircuitBreakerPickerBuilder(&picker.MrtPickerBuilder{}, cbConfig),
		),
		config: Config{
			HealthCheck: healthCheck,
		},
		Logger: log,
	}

	balancer.Register(rb)
}

func (b *mrtBalance) Build(cc balancer.ClientConn, opt balancer.BuildOptions) balancer.Balancer {
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

func (b *mrtBalance) Name() string {
	return MinRespTimeBalanceName
}
