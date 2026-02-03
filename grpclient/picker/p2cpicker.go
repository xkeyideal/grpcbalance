package picker

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/resolver"
)

const (
	// forcePick 如果某个节点在此时间内未被选中过，强制选中一次以更新其状态
	forcePick = time.Second * 3
)

// p2cNode 包装 SubConn 并记录统计信息
type p2cNode struct {
	sc       balancer.SubConn
	addr     resolver.Address
	ewma     float64   // EWMA 响应时间
	inflight int64     // 当前正在进行的请求数
	success  int64     // 成功请求数
	requests int64     // 总请求数
	lastPick time.Time // 最后一次被选中的时间
	mu       sync.RWMutex
}

// load 计算节点的当前负载 = ewma * (inflight + 1)
// inflight+1 是为了避免新节点一开始就承担过多流量
func (n *p2cNode) load() float64 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	inflight := atomic.LoadInt64(&n.inflight)
	ewma := n.ewma
	if ewma == 0 {
		ewma = 1 // 默认值，避免新节点load为0
	}

	// 负载 = 响应时间 * (正在进行的请求数 + 1)
	return ewma * float64(inflight+1)
}

// pickElapsed 返回距离上次被选中的时间
func (n *p2cNode) pickElapsed() time.Duration {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return time.Since(n.lastPick)
}

// P2CPickerBuilder P2C选择器构造器
type P2CPickerBuilder struct{}

func (pb *P2CPickerBuilder) Build(info PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}

	nodes := make([]*p2cNode, 0, len(info.ReadySCs))
	for sc, scInfo := range info.ReadySCs {
		nodes = append(nodes, &p2cNode{
			sc:       sc,
			addr:     scInfo.Address,
			ewma:     0,
			inflight: 0,
			lastPick: time.Now(),
		})
	}

	return &p2cPicker{
		nodes:  nodes,
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
		picked: atomic.Bool{},
	}
}

type p2cPicker struct {
	nodes  []*p2cNode
	mu     sync.Mutex
	rand   *rand.Rand
	picked atomic.Bool // 用于 forcePick 逻辑
}

// prePick 随机选择两个不同的节点
func (p *p2cPicker) prePick() (nodeA, nodeB *p2cNode) {
	p.mu.Lock()
	a := p.rand.Intn(len(p.nodes))
	b := p.rand.Intn(len(p.nodes) - 1)
	p.mu.Unlock()

	if b >= a {
		b = b + 1
	}
	return p.nodes[a], p.nodes[b]
}

func (p *p2cPicker) Pick(opts balancer.PickInfo) (balancer.PickResult, error) {
	if len(p.nodes) == 0 {
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}

	// 只有一个节点时直接选择
	if len(p.nodes) == 1 {
		return p.pickNode(p.nodes[0])
	}

	nodeA, nodeB := p.prePick()

	// 选择负载较低的节点
	var picked, unpicked *p2cNode
	if nodeB.load() > nodeA.load() {
		picked, unpicked = nodeA, nodeB
	} else {
		picked, unpicked = nodeB, nodeA
	}

	// 如果 unpicked 节点长时间未被选中，强制选中一次
	// 这样可以获取其最新的响应时间数据
	if unpicked.pickElapsed() > forcePick && p.picked.CompareAndSwap(false, true) {
		picked = unpicked
		defer p.picked.Store(false)
	}

	return p.pickNode(picked)
}

func (p *p2cPicker) pickNode(node *p2cNode) (balancer.PickResult, error) {
	// 更新最后选中时间
	node.mu.Lock()
	node.lastPick = time.Now()
	node.mu.Unlock()

	// 增加 inflight 计数
	atomic.AddInt64(&node.inflight, 1)
	atomic.AddInt64(&node.requests, 1)

	start := time.Now()
	done := func(info balancer.DoneInfo) {
		// 减少 inflight 计数
		atomic.AddInt64(&node.inflight, -1)

		if info.Err == nil {
			atomic.AddInt64(&node.success, 1)
		}

		// 使用 EWMA 更新响应时间
		latency := float64(time.Since(start).Microseconds())
		node.mu.Lock()
		if node.ewma == 0 {
			node.ewma = latency
		} else {
			node.ewma = ewmaAlpha*latency + (1-ewmaAlpha)*node.ewma
		}
		node.mu.Unlock()
	}

	return balancer.PickResult{SubConn: node.sc, Done: done}, nil
}
