package balancer

import (
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
			HealthCheck:     healthCheck,
			StripAttributes: true,
		},
	}

	balancer.Register(rb)
}

func (b *rrBalance) Build(cc balancer.ClientConn, opt balancer.BuildOptions) balancer.Balancer {
	bal := &baseBalancer{
		cc:            cc,
		pickerBuilder: b.pickerBuilder,

		subConns: resolver.NewAddressMap(),
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

func (b *rrBalance) Name() string {
	return RoundRobinBalanceName
}
