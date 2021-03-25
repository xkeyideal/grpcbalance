package picker

import (
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/xkeyideal/grpcbalance/grpclient/priorityqueue"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/resolver"
)

const recordTimes = 10

type MrtPickerBuilder struct {
}

func (pb *MrtPickerBuilder) Build(info PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}
	scs := []balancer.SubConn{}
	scToAddr := make(map[balancer.SubConn]resolver.Address)
	scCostTime := priorityqueue.NewPriorityQueue()
	scRecords := make([][]int64, len(info.ReadySCs))
	i := 0
	for sc, scInfo := range info.ReadySCs {
		scs = append(scs, sc)
		scToAddr[sc] = scInfo.Address
		scCostTime.PushItem(&priorityqueue.Item{
			Addr:  scInfo.Address.Addr,
			Val:   0,
			Index: i,
		})
		scRecords[i] = make([]int64, recordTimes)
		i++
	}

	return &mrtPicker{
		subConns:   scs,
		scToAddr:   scToAddr,
		scCostTime: scCostTime,
		scRecords:  scRecords,
		// Start at a random index, as the same RR balancer rebuilds a new
		// picker when SubConn states change, and we don't want to apply excess
		// load to the first server in the list.
		next: rand.Intn(len(scs)),
	}
}

type mrtPicker struct {
	// subConns is the snapshot of the roundrobin balancer when this picker was
	// created. The slice is immutable. Each Get() will do a round robin
	// selection from it and return the selected SubConn.
	subConns []balancer.SubConn

	scToAddr map[balancer.SubConn]resolver.Address

	// subConns response cost time
	scCostTime *priorityqueue.PriorityQueue

	// last ten times response cost time
	scRecords [][]int64

	mu   sync.Mutex
	next int
}

func (p *mrtPicker) Pick(opts balancer.PickInfo) (balancer.PickResult, error) {
	p.mu.Lock()
	n := len(p.subConns)
	p.mu.Unlock()
	if n == 0 {
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}

	p.mu.Lock()
	item := p.scCostTime.Min().(*priorityqueue.Item)
	p.next = item.Index
	//log.Println("next", item.Index, p.scToAddr[p.subConns[p.next]].Addr, item.Val, n, item.Addr)
	sc := p.subConns[p.next]
	//picked := p.scToAddr[sc]
	p.mu.Unlock()

	start := time.Now()
	done := func(info balancer.DoneInfo) {
		sub := time.Now().Sub(start).Microseconds()
		if info.Err == nil {
			p.mu.Lock()
			p.scRecords[item.Index] = append(p.scRecords[item.Index][1:], sub)
			item.Val = weightedAverage(p.scRecords[item.Index])
			p.scCostTime.UpdateItem(item)
			p.mu.Unlock()
		}
		//log.Println("mrtpicker done", picked.Addr, p.next, sub)
	}

	return balancer.PickResult{SubConn: sc, Done: done}, nil
}

func weightedAverage(vals []int64) int64 {
	var (
		min int64 = math.MaxInt64
		max int64 = math.MinInt64
		sum int64 = 0
		n   int64 = int64(len(vals))
	)

	for _, val := range vals {
		sum += val
		if min > val {
			min = val
		}
		if max < val {
			max = val
		}
	}

	if n <= 2 {
		return sum / n
	}

	sum = sum - min - max
	return sum / (n - 2)
}
