package picker

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/xkeyideal/grpcbalance/grpclient/logger"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"
)

// GracefulSwitchPicker 实现了平滑切换 picker 的功能
// 当 picker 更新时，不会立即替换，而是等待当前 picker 的所有请求完成后再切换
// 这样可以避免 picker 切换时的请求失败
type GracefulSwitchPicker struct {
	mu              sync.RWMutex
	currentPicker   balancer.Picker
	pendingPicker   balancer.Picker
	currentState    connectivity.State
	pendingState    connectivity.State
	inflightCount   int64 // 当前 picker 正在处理的请求数
	drainTimeout    time.Duration
	drainInProgress bool
	logger          logger.Logger
}

// NewGracefulSwitchPicker 创建一个平滑切换的 picker
func NewGracefulSwitchPicker(drainTimeout time.Duration) *GracefulSwitchPicker {
	if drainTimeout <= 0 {
		drainTimeout = 5 * time.Second
	}
	return &GracefulSwitchPicker{
		drainTimeout: drainTimeout,
	}
}

// UpdatePicker 更新 picker，如果有正在进行的请求，会延迟切换
func (g *GracefulSwitchPicker) UpdatePicker(picker balancer.Picker, state connectivity.State) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 如果当前没有 picker，直接设置
	if g.currentPicker == nil {
		g.currentPicker = picker
		g.currentState = state
		return
	}

	// 如果新状态不是 Ready，立即切换
	if state != connectivity.Ready && g.currentState != connectivity.Ready {
		g.currentPicker = picker
		g.currentState = state
		return
	}

	// 如果当前状态是 Ready，新状态不是 Ready，继续使用当前 picker
	if g.currentState == connectivity.Ready && state != connectivity.Ready {
		g.pendingPicker = picker
		g.pendingState = state
		// 当前 picker 没有正在进行的请求时切换
		if atomic.LoadInt64(&g.inflightCount) == 0 {
			g.swapPickersLocked()
		} else if !g.drainInProgress {
			g.drainInProgress = true
			go g.drainAndSwap()
		}
		return
	}

	// 如果新状态是 Ready，立即切换到新的 picker
	if state == connectivity.Ready {
		g.currentPicker = picker
		g.currentState = state
		g.pendingPicker = nil
		g.drainInProgress = false
		return
	}

	// 其他情况，设置为 pending
	g.pendingPicker = picker
	g.pendingState = state
}

// swapPickersLocked 在持有锁的情况下交换 picker
func (g *GracefulSwitchPicker) swapPickersLocked() {
	if g.pendingPicker != nil {
		g.currentPicker = g.pendingPicker
		g.currentState = g.pendingState
		g.pendingPicker = nil
		g.drainInProgress = false
	}
}

// drainAndSwap 等待当前请求完成后切换 picker
func (g *GracefulSwitchPicker) drainAndSwap() {
	deadline := time.Now().Add(g.drainTimeout)

	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&g.inflightCount) == 0 {
			g.mu.Lock()
			g.swapPickersLocked()
			g.mu.Unlock()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 超时后强制切换
	g.mu.Lock()
	g.swapPickersLocked()
	g.mu.Unlock()
}

// Pick 选择一个 SubConn
func (g *GracefulSwitchPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	g.mu.RLock()
	picker := g.currentPicker
	log := g.logger
	g.mu.RUnlock()

	if picker == nil {
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}

	if log != nil {
		log.Debugf("GracefulPicker: pick info %s", formatPickInfo(info))
	}

	atomic.AddInt64(&g.inflightCount, 1)

	result, err := picker.Pick(info)
	if err != nil {
		atomic.AddInt64(&g.inflightCount, -1)
		return result, err
	}

	// 包装 Done 回调
	originalDone := result.Done
	result.Done = func(doneInfo balancer.DoneInfo) {
		atomic.AddInt64(&g.inflightCount, -1)
		if log != nil {
			log.Debugf("GracefulPicker: done info %s", formatDoneInfo(doneInfo))
		}
		if originalDone != nil {
			originalDone(doneInfo)
		}
	}

	return result, nil
}

// CurrentState 返回当前的连接状态
func (g *GracefulSwitchPicker) CurrentState() connectivity.State {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.currentState
}

// InflightCount 返回当前正在进行的请求数
func (g *GracefulSwitchPicker) InflightCount() int64 {
	return atomic.LoadInt64(&g.inflightCount)
}

// GracefulPickerBuilder 包装一个 PickerBuilder，提供平滑切换功能
type GracefulPickerBuilder struct {
	inner        PickerBuilder
	drainTimeout time.Duration
	current      *GracefulSwitchPicker
	mu           sync.Mutex
	logger       logger.Logger
}

// NewGracefulPickerBuilder 创建一个支持平滑切换的 PickerBuilder
func NewGracefulPickerBuilder(pb PickerBuilder, drainTimeout time.Duration) *GracefulPickerBuilder {
	return &GracefulPickerBuilder{
		inner:        pb,
		drainTimeout: drainTimeout,
	}
}

func (gpb *GracefulPickerBuilder) SetLogger(log logger.Logger) {
	gpb.logger = log
	if gpb.inner != nil {
		gpb.inner.SetLogger(log)
	}
}

func (gpb *GracefulPickerBuilder) Build(info PickerBuildInfo) balancer.Picker {
	gpb.mu.Lock()
	defer gpb.mu.Unlock()

	innerPicker := gpb.inner.Build(info)

	if gpb.current == nil {
		gpb.current = NewGracefulSwitchPicker(gpb.drainTimeout)
	}
	log := gpb.logger
	if log == nil {
		log = logger.GetDefaultLogger()
	}
	gpb.current.logger = log

	// 根据节点数量确定状态
	state := connectivity.Ready
	if len(info.ReadySCs) == 0 {
		state = connectivity.TransientFailure
	}

	gpb.current.UpdatePicker(innerPicker, state)
	return gpb.current
}
