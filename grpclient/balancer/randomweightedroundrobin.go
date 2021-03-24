package balancer

import (
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
			HealthCheck:     healthCheck,
			StripAttributes: false,
		},
	}

	balancer.Register(rb)
}

func (b *rwrrBalance) Build(cc balancer.ClientConn, opt balancer.BuildOptions) balancer.Balancer {
	bal := &baseBalancer{
		cc:            cc,
		pickerBuilder: b.pickerBuilder,

		subConns: make(map[resolver.Address]balancer.SubConn),
		scStates: make(map[balancer.SubConn]connectivity.State),
		csEvltr:  &balancer.ConnectivityStateEvaluator{},
		config:   b.config,
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
