package picker

import (
	"math/rand"
	"sync"
	"time"

	"github.com/xkeyideal/grpcbalance/grpclient/logger"
	"github.com/xkeyideal/grpcbalance/grpclient/priorityqueue"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/resolver"
)

const (
	// EWMA 衰减因子，值越大新数据权重越高
	// alpha = 0.3 意味着新值占 30%，历史值占 70%
	ewmaAlpha = 0.3
)

type MrtPickerBuilder struct {
	logger logger.Logger
}

func (pb *MrtPickerBuilder) SetLogger(log logger.Logger) {
	pb.logger = log
}

func (pb *MrtPickerBuilder) Build(info PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}
	scs := []balancer.SubConn{}
	scToAddr := make(map[balancer.SubConn]resolver.Address)
	scCostTime := priorityqueue.NewPriorityQueue()
	scEWMA := make([]float64, len(info.ReadySCs))
	i := 0
	for sc, scInfo := range info.ReadySCs {
		scs = append(scs, sc)
		scToAddr[sc] = scInfo.Address
		scCostTime.PushItem(&priorityqueue.Item{
			Addr:  scInfo.Address.Addr,
			Val:   0,
			Index: i,
		})
		scEWMA[i] = 0
		i++
	}

	log := pb.logger
	if log == nil {
		log = logger.GetDefaultLogger()
	}
	log.Debugf("MrtPicker built with %d SubConns", len(scs))

	return &mrtPicker{
		subConns:   scs,
		scToAddr:   scToAddr,
		scCostTime: scCostTime,
		scEWMA:     scEWMA,
		logger:     log,
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
	logger   logger.Logger

	// subConns response cost time (priority queue based on EWMA)
	scCostTime *priorityqueue.PriorityQueue

	// EWMA (Exponentially Weighted Moving Average) for each SubConn
	scEWMA []float64

	mu   sync.Mutex
	next int
}

func (p *mrtPicker) Pick(opts balancer.PickInfo) (balancer.PickResult, error) {
	p.mu.Lock()
	n := len(p.subConns)
	if n == 0 {
		p.mu.Unlock()
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}
	minItem := p.scCostTime.Min()
	if minItem == nil {
		p.mu.Unlock()
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}
	item := minItem.(*priorityqueue.Item)
	p.next = item.Index
	sc := p.subConns[p.next]
	picked := p.scToAddr[sc]
	ewmaBefore := p.scEWMA[item.Index]
	p.mu.Unlock()

	p.logger.Debugf("MrtPicker: picked %s (ewma: %.2fμs)", picked.Addr, ewmaBefore)

	start := time.Now()
	done := func(info balancer.DoneInfo) {
		latency := float64(time.Since(start).Microseconds())
		if info.Err == nil {
			p.mu.Lock()
			// 使用 EWMA 计算平均响应时间
			// newEWMA = alpha * newValue + (1 - alpha) * oldEWMA
			if p.scEWMA[item.Index] == 0 {
				// 首次记录，直接使用当前值
				p.scEWMA[item.Index] = latency
			} else {
				p.scEWMA[item.Index] = ewmaAlpha*latency + (1-ewmaAlpha)*p.scEWMA[item.Index]
			}
			item.Val = int64(p.scEWMA[item.Index])
			p.scCostTime.UpdateItem(item)
			newEwma := p.scEWMA[item.Index]
			p.mu.Unlock()
			p.logger.Debugf("MrtPicker: done %s, latency: %.2fμs, ewma: %.2fμs -> %.2fμs", picked.Addr, latency, ewmaBefore, newEwma)
		} else {
			p.logger.Debugf("MrtPicker: done %s, error: %v, latency: %.2fμs", picked.Addr, info.Err, latency)
		}
	}

	return balancer.PickResult{SubConn: sc, Done: done}, nil
}
