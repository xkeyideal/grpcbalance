package picker

import (
	"log"
	"math/rand"
	"sync"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/resolver"
)

type RRPickerBuilder struct {
}

func (pb *RRPickerBuilder) Build(info PickerBuildInfo) balancer.Picker {
	log.Println("rrbuilder: ", len(info.ReadySCs))

	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}
	scs := []balancer.SubConn{}
	scToAddr := make(map[balancer.SubConn]resolver.Address)
	for sc, scInfo := range info.ReadySCs {
		scs = append(scs, sc)
		scToAddr[sc] = scInfo.Address
	}

	return &rrPicker{
		subConns: scs,
		scToAddr: scToAddr,
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

	mu   sync.Mutex
	next int
}

func (p *rrPicker) Pick(opts balancer.PickInfo) (balancer.PickResult, error) {
	p.mu.Lock()
	n := len(p.subConns)
	p.mu.Unlock()
	if n == 0 {
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}

	log.Println("rrpicker: ", opts.FullMethodName, p.next, len(p.subConns))

	p.mu.Lock()
	sc := p.subConns[p.next]
	picked := p.scToAddr[sc]
	p.next = (p.next + 1) % len(p.subConns)
	p.mu.Unlock()

	done := func(info balancer.DoneInfo) {
		log.Println("rrpicker done", picked.Addr)
	}

	return balancer.PickResult{SubConn: sc, Done: done}, nil
}
