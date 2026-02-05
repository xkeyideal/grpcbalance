package balancer

import (
	"github.com/xkeyideal/grpcbalance/grpclient/circuitbreaker"
	"github.com/xkeyideal/grpcbalance/grpclient/logger"
	"github.com/xkeyideal/grpcbalance/grpclient/picker"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/resolver"
)

const MinConnectBalanceName = "x_customize_min_connect"

type mcBalance struct {
	pickerBuilder picker.PickerBuilder
	config        Config

	// Logger is the logger for this balancer. If nil, the default logger will be used.
	Logger logger.Logger
}

func RegisterMcBalance(healthCheck bool, log logger.Logger) {
	rb := &mcBalance{
		pickerBuilder: &picker.McPickerBuilder{},
		config: Config{
			HealthCheck: healthCheck,
		},
		Logger: log,
	}

	balancer.Register(rb)
}

// RegisterMcBalanceWithFilter registers min connect balancer with node filter support
func RegisterMcBalanceWithFilter(healthCheck bool, log logger.Logger) {
	rb := &mcBalance{
		pickerBuilder: picker.NewFilteredPickerBuilder(&picker.McPickerBuilder{}),
		config: Config{
			HealthCheck: healthCheck,
		},
		Logger: log,
	}

	balancer.Register(rb)
}

// RegisterMcBalanceWithCircuitBreaker registers min connect balancer with circuit breaker
func RegisterMcBalanceWithCircuitBreaker(healthCheck bool, cbConfig circuitbreaker.Config, log logger.Logger) {
	rb := &mcBalance{
		pickerBuilder: picker.NewCircuitBreakerPickerBuilder(&picker.McPickerBuilder{}, cbConfig),
		config: Config{
			HealthCheck: healthCheck,
		},
		Logger: log,
	}

	balancer.Register(rb)
}

// RegisterMcBalanceWithFilterAndCircuitBreaker registers min connect balancer with both filter and circuit breaker
func RegisterMcBalanceWithFilterAndCircuitBreaker(healthCheck bool, cbConfig circuitbreaker.Config, log logger.Logger) {
	rb := &mcBalance{
		pickerBuilder: picker.NewFilteredPickerBuilder(
			picker.NewCircuitBreakerPickerBuilder(&picker.McPickerBuilder{}, cbConfig),
		),
		config: Config{
			HealthCheck: healthCheck,
		},
		Logger: log,
	}

	balancer.Register(rb)
}

func (b *mcBalance) Build(cc balancer.ClientConn, opt balancer.BuildOptions) balancer.Balancer {
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

func (b *mcBalance) Name() string {
	return MinConnectBalanceName
}
