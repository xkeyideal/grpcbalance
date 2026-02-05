package picker

import (
	"math/rand"
	"sync"

	"github.com/xkeyideal/grpcbalance/grpclient/logger"
	"github.com/xkeyideal/grpcbalance/grpclient/priorityqueue"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/resolver"
)

type McPickerBuilder struct {
	logger logger.Logger
}

func (pb *McPickerBuilder) SetLogger(log logger.Logger) {
	pb.logger = log
}

func (pb *McPickerBuilder) Build(info PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}
	scs := []balancer.SubConn{}
	scToAddr := make(map[balancer.SubConn]resolver.Address)
	scConnectNum := priorityqueue.NewPriorityQueue()
	i := 0
	for sc, scInfo := range info.ReadySCs {
		scs = append(scs, sc)
		scToAddr[sc] = scInfo.Address
		scConnectNum.PushItem(&priorityqueue.Item{
			Addr:  scInfo.Address.Addr,
			Val:   0,
			Index: i,
		})
		i++
	}

	log := pb.logger
	if log == nil {
		log = logger.GetDefaultLogger()
	}
	log.Debugf("McPicker built with %d SubConns", len(scs))

	return &mcPicker{
		subConns:     scs,
		scToAddr:     scToAddr,
		scConnectNum: scConnectNum,
		logger:       log,
		// Start at a random index, as the same RR balancer rebuilds a new
		// picker when SubConn states change, and we don't want to apply excess
		// load to the first server in the list.
		next: rand.Intn(len(scs)),
	}
}

type mcPicker struct {
	// subConns is the snapshot of the roundrobin balancer when this picker was
	// created. The slice is immutable. Each Get() will do a round robin
	// selection from it and return the selected SubConn.
	subConns []balancer.SubConn

	scToAddr map[balancer.SubConn]resolver.Address
	logger   logger.Logger

	// subConns connect number
	scConnectNum *priorityqueue.PriorityQueue

	mu   sync.Mutex
	next int
}

func (p *mcPicker) Pick(opts balancer.PickInfo) (balancer.PickResult, error) {
	p.mu.Lock()
	n := len(p.subConns)
	if n == 0 {
		p.mu.Unlock()
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}

	minItem := p.scConnectNum.Min()
	if minItem == nil {
		p.mu.Unlock()
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}
	item := minItem.(*priorityqueue.Item)
	p.next = item.Index
	sc := p.subConns[p.next]
	picked := p.scToAddr[sc]
	connCountBefore := item.Val
	item.Val++
	p.scConnectNum.UpdateItem(item)
	p.mu.Unlock()

	p.logger.Debugf("McPicker: picked %s (connections: %d -> %d)", picked.Addr, connCountBefore, connCountBefore+1)

	done := func(info balancer.DoneInfo) {
		p.mu.Lock()
		item.Val--
		p.scConnectNum.UpdateItem(item)
		p.mu.Unlock()
		if info.Err != nil {
			p.logger.Debugf("McPicker: done %s, error: %v, connections: %d", picked.Addr, info.Err, item.Val)
		} else {
			p.logger.Debugf("McPicker: done %s, success, connections: %d", picked.Addr, item.Val)
		}
	}

	return balancer.PickResult{SubConn: sc, Done: done}, nil
}
