package picker

import (
	"log"
	"sync"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/resolver"
)

type WRRPickerBuilder struct{}

func (pb *WRRPickerBuilder) Build(info PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}

	scs := []balancer.SubConn{}
	scToAddr := make(map[balancer.SubConn]resolver.Address)
	wrr := newWrr()
	var weight int32
	for sc, scInfo := range info.ReadySCs {
		scs = append(scs, sc)
		scToAddr[sc] = scInfo.Address
		weight = 1
		if scInfo.Address.Attributes != nil {
			val := scInfo.Address.Attributes.Value(WeightAttributeKey)
			weight = val.(int32)
		}
		wrr.add(weight)
	}

	return &wrrPicker{
		subConns: scs,
		scToAddr: scToAddr,
		// Start at a random index, as the same WRR balancer rebuilds a new
		// picker when SubConn states change, and we don't want to apply excess
		// load to the first server in the list.
		next: wrr.next(),
		wrr:  wrr,
	}
}

type wrrPicker struct {
	// subConns is the snapshot of the weightedroundrobin balancer when this picker was
	// created. The slice is immutable. Each Get() will do a round robin
	// selection from it and return the selected SubConn.
	subConns []balancer.SubConn

	scToAddr map[balancer.SubConn]resolver.Address

	mu sync.Mutex

	wrr  *wrr
	next int
}

func (p *wrrPicker) Pick(opts balancer.PickInfo) (balancer.PickResult, error) {
	p.mu.Lock()
	n := len(p.subConns)
	p.mu.Unlock()
	if n == 0 {
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}

	p.mu.Lock()
	sc := p.subConns[p.next]
	picked := p.scToAddr[sc]
	p.next = p.wrr.next()
	p.mu.Unlock()

	done := func(info balancer.DoneInfo) {
		log.Println("wrrpicker done", picked.Addr)
	}

	return balancer.PickResult{SubConn: sc, Done: done}, nil
}

type wrr struct {
	weights []int32
	n       int
	gcd     int32
	maxW    int32
	i       int
	cw      int32
}

func newWrr() *wrr {
	return &wrr{}
}

func (w *wrr) add(weight int32) {
	if weight > 0 {
		if w.gcd == 0 {
			w.gcd = weight
			w.maxW = weight
			w.i = -1
			w.cw = 0
		} else {
			w.gcd = gcd(w.gcd, weight)
			if w.maxW < weight {
				w.maxW = weight
			}
		}
	}

	w.weights = append(w.weights, weight)
	w.n++
}

func (w *wrr) next() int {
	if w.n <= 1 {
		return 0
	}

	for {
		w.i = (w.i + 1) % w.n
		if w.i == 0 {
			w.cw = w.cw - w.gcd
			if w.cw <= 0 {
				w.cw = w.maxW
				if w.cw == 0 {
					return 0
				}
			}
		}

		if w.weights[w.i] >= w.cw {
			return w.i
		}
	}
}

func gcd(a, b int32) int32 {
	if b == 0 {
		return a
	}

	return gcd(b, a%b)
}
