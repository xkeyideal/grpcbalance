package balancer

import (
	"github.com/xkeyideal/grpcbalance/grpclient/picker"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/resolver"
)

const MinConnectBalanceName = "customize_min_connect"

type mcBalance struct {
	pickerBuilder picker.PickerBuilder
	config        Config
}

func RegisterMcBalance(healthCheck bool) {
	rb := &mcBalance{
		pickerBuilder: &picker.McPickerBuilder{},
		config: Config{
			HealthCheck:     healthCheck,
			StripAttributes: true,
		},
	}

	balancer.Register(rb)
}

func (b *mcBalance) Build(cc balancer.ClientConn, opt balancer.BuildOptions) balancer.Balancer {
	bal := &baseBalancer{
		cc:            cc,
		pickerBuilder: b.pickerBuilder,

		subConns: resolver.NewAddressMap(),
		scStates: make(map[balancer.SubConn]connectivity.State),
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
