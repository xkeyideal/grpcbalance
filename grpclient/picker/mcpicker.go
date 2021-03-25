package picker

import (
	"math/rand"
	"sync"

	"github.com/xkeyideal/grpcbalance/grpclient/priorityqueue"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/resolver"
)

type McPickerBuilder struct {
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

	return &mcPicker{
		subConns:     scs,
		scToAddr:     scToAddr,
		scConnectNum: scConnectNum,
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

	// subConns connect number
	scConnectNum *priorityqueue.PriorityQueue

	mu   sync.Mutex
	next int
}

func (p *mcPicker) Pick(opts balancer.PickInfo) (balancer.PickResult, error) {
	p.mu.Lock()
	n := len(p.subConns)
	p.mu.Unlock()
	if n == 0 {
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}

	p.mu.Lock()
	sc := p.subConns[p.next]
	//picked := p.scToAddr[sc]
	item := p.scConnectNum.Min().(*priorityqueue.Item)
	p.next = item.Index
	item.Val++
	p.scConnectNum.UpdateItem(item)
	p.mu.Unlock()

	done := func(info balancer.DoneInfo) {
		p.mu.Lock()
		item.Val--
		p.scConnectNum.UpdateItem(item)
		p.mu.Unlock()
		//log.Println("mcpicker done", picked.Addr, p.next)
	}

	return balancer.PickResult{SubConn: sc, Done: done}, nil
}
