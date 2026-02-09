package picker

import (
	"sync"

	"github.com/xkeyideal/grpcbalance/grpclient/logger"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/resolver"
)

type WRRPickerBuilder struct {
	logger logger.Logger
}

func (pb *WRRPickerBuilder) SetLogger(log logger.Logger) {
	pb.logger = log
}

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
			if val := scInfo.Address.Attributes.Value(WeightAttributeKey); val != nil {
				if w, ok := val.(int32); ok {
					weight = w
				}
			}
		}
		wrr.add(weight)
	}

	log := pb.logger
	if log == nil {
		log = logger.GetDefaultLogger()
	}
	log.Debugf("WRRPicker built with %d SubConns", len(scs))

	return &wrrPicker{
		subConns: scs,
		scToAddr: scToAddr,
		logger:   log,
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
	logger   logger.Logger

	mu sync.Mutex

	wrr  *wrr
	next int
}

func (p *wrrPicker) Pick(opts balancer.PickInfo) (balancer.PickResult, error) {
	p.mu.Lock()
	n := len(p.subConns)
	if n == 0 {
		p.mu.Unlock()
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}
	sc := p.subConns[p.next]
	picked := p.scToAddr[sc]
	currentIndex := p.next
	p.next = p.wrr.next()
	p.mu.Unlock()

	p.logger.Debugf("WRRPicker: picked %s (index: %d, weight: %d)", picked.Addr, currentIndex, p.wrr.weights[currentIndex])
	p.logger.Debugf("WRRPicker: pick info %s", formatPickInfo(opts))

	done := func(info balancer.DoneInfo) {
		if info.Err != nil {
			p.logger.Debugf("WRRPicker: done %s, error: %v, info: %s", picked.Addr, info.Err, formatDoneInfo(info))
		} else {
			p.logger.Debugf("WRRPicker: done %s, info: %s", picked.Addr, formatDoneInfo(info))
		}
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
