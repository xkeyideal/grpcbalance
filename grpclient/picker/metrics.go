package picker

import (
	"time"
)

// DoneInfo 包含 RPC 完成时的统计信息
type DoneInfo struct {
	// Err 是 RPC 的错误（如果有）
	Err error

	// BytesSent 表示是否有字节发送到服务器
	BytesSent bool

	// BytesReceived 表示是否从服务器收到响应
	BytesReceived bool

	// Latency 是请求的延迟时间
	Latency time.Duration

	// ServerLoad 是服务器返回的负载信息（可选）
	ServerLoad interface{}
}

// DoneFunc 是 RPC 完成后的回调函数类型
type DoneFunc func(info DoneInfo)

// Metrics 用于记录节点的统计指标
type Metrics struct {
	// Requests 是总请求数
	Requests int64

	// Success 是成功请求数
	Success int64

	// Failures 是失败请求数
	Failures int64

	// Latency 是 EWMA 平均延迟（微秒）
	Latency float64

	// Inflight 是当前正在进行的请求数
	Inflight int64

	// LastUpdate 是最后更新时间
	LastUpdate time.Time
}

// NodeMetrics 是节点级别的指标收集器
type NodeMetrics struct {
	Address     string
	Metrics     Metrics
	SuccessRate float64 // 成功率 (0-1)
}

// MetricsCollector 定义收集 RPC 指标的接口
type MetricsCollector interface {
	// Collect 收集一次 RPC 调用的指标
	Collect(addr string, info DoneInfo)

	// GetMetrics 获取指定节点的指标
	GetMetrics(addr string) *NodeMetrics

	// GetAllMetrics 获取所有节点的指标
	GetAllMetrics() []NodeMetrics
}
