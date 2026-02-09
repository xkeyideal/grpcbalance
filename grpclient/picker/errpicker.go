package picker

import (
	"github.com/xkeyideal/grpcbalance/grpclient/logger"
	"google.golang.org/grpc/balancer"
)

// NewErrPicker returns a Picker that always returns err on Pick().
func NewErrPicker(err error) balancer.Picker {
	return &errPicker{err: err, logger: logger.GetDefaultLogger()}
}

type errPicker struct {
	err    error // Pick() always returns this err.
	logger logger.Logger
}

func (p *errPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	if p.logger != nil {
		p.logger.Debugf("ErrPicker: pick info %s, err: %v", formatPickInfo(info), p.err)
	}
	return balancer.PickResult{}, p.err
}
