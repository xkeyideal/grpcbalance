package balancer

import (
	"github.com/xkeyideal/grpcbalance/grpclient/picker"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/resolver"
)

const MinRespTimeBalanceName = "customize_min_resp_time"

type mrtBalance struct {
	pickerBuilder picker.PickerBuilder
	config        Config
}

func RegisterMrtBalance(healthCheck bool) {
	rb := &mrtBalance{
		pickerBuilder: &picker.MrtPickerBuilder{},
		config: Config{
			HealthCheck:     healthCheck,
			StripAttributes: true,
		},
	}

	balancer.Register(rb)
}

func (b *mrtBalance) Build(cc balancer.ClientConn, opt balancer.BuildOptions) balancer.Balancer {
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

func (b *mrtBalance) Name() string {
	return MinRespTimeBalanceName
}
