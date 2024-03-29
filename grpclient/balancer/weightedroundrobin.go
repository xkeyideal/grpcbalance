package balancer

import (
	"github.com/xkeyideal/grpcbalance/grpclient/picker"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/resolver"
)

const WeightedRobinBalanceName = "customize_weighted_round_robin"

type wrrBalance struct {
	pickerBuilder picker.PickerBuilder
	config        Config
}

func RegisterWRRBalance(healthCheck bool) {
	rb := &wrrBalance{
		pickerBuilder: &picker.WRRPickerBuilder{},
		config: Config{
			HealthCheck:     healthCheck,
			StripAttributes: false,
		},
	}

	balancer.Register(rb)
}

func (b *wrrBalance) Build(cc balancer.ClientConn, opt balancer.BuildOptions) balancer.Balancer {
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

func (b *wrrBalance) Name() string {
	return WeightedRobinBalanceName
}
