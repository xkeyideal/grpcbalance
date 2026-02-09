package picker

import (
	"math/rand"
	"sync"

	"github.com/xkeyideal/grpcbalance/grpclient/logger"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/resolver"
)

type RRPickerBuilder struct {
	logger logger.Logger
}

func (pb *RRPickerBuilder) SetLogger(log logger.Logger) {
	pb.logger = log
}

func (pb *RRPickerBuilder) Build(info PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}
	scs := []balancer.SubConn{}
	scToAddr := make(map[balancer.SubConn]resolver.Address)
	for sc, scInfo := range info.ReadySCs {
		scs = append(scs, sc)
		scToAddr[sc] = scInfo.Address
	}

	log := pb.logger
	if log == nil {
		log = logger.GetDefaultLogger()
	}
	log.Debugf("RRPicker built with %d SubConns", len(scs))

	return &rrPicker{
		subConns: scs,
		scToAddr: scToAddr,
		logger:   log,
		// Start at a random index, as the same RR balancer rebuilds a new
		// picker when SubConn states change, and we don't want to apply excess
		// load to the first server in the list.
		next: rand.Intn(len(scs)),
	}
}

type rrPicker struct {
	// subConns is the snapshot of the roundrobin balancer when this picker was
	// created. The slice is immutable. Each Get() will do a round robin
	// selection from it and return the selected SubConn.
	subConns []balancer.SubConn

	scToAddr map[balancer.SubConn]resolver.Address
	logger   logger.Logger

	mu   sync.Mutex
	next int
}

func (p *rrPicker) Pick(opts balancer.PickInfo) (balancer.PickResult, error) {
	p.mu.Lock()
	n := len(p.subConns)
	if n == 0 {
		p.mu.Unlock()
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}
	sc := p.subConns[p.next]
	picked := p.scToAddr[sc]
	p.next = (p.next + 1) % n
	p.mu.Unlock()

	p.logger.Debugf("RRPicker: picked %s (index: %d/%d)", picked.Addr, (p.next-1+n)%n, n)
	p.logger.Debugf("RRPicker: pick info %s", formatPickInfo(opts))

	done := func(info balancer.DoneInfo) {
		if info.Err != nil {
			p.logger.Debugf("RRPicker: done %s, error: %v, info: %s", picked.Addr, info.Err, formatDoneInfo(info))
		} else {
			p.logger.Debugf("RRPicker: done %s, info: %s", picked.Addr, formatDoneInfo(info))
		}
	}

	return balancer.PickResult{SubConn: sc, Done: done}, nil
}
