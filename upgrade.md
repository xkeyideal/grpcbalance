# grpcbalance 升级与优化记录

## 版本信息

- **升级日期**: 2026年2月3日
- **gRPC 版本**: v1.78.0
- **Go 版本**: 1.21+

---

## 最新变更 (2026年2月3日)

### 1. 移除 StripAttributes 配置项

**背景**: 对比 grpc-go 官方 `balancer/base` 实现，发现官方 `base.Config` 仅包含 `HealthCheck` 字段，而本项目的 `StripAttributes` 是自定义添加的。

**问题分析**:
- `resolver.Address` 的 `Attributes` 字段使用 `Attributes.Equal()` 进行比较
- 当 Attributes 包含不可比较类型（如 `map[string]string`）时会导致 panic
- 这也是原本添加 `StripAttributes` 的原因

**解决方案**:
- 从 `balancer.Config` 中移除 `StripAttributes` 配置项
- 在 `UpdateClientConnState` 中**始终**将 Attributes 从 map key 中剥离（与 grpc-go 的 grpclb 实现一致）
- 使用 `scAddrs map[balancer.SubConn]resolver.Address` 存储原始地址（包含 Attributes），供 picker 使用

**修改文件**:
- `grpclient/balancer/balancer.go` - Config 结构体简化，UpdateClientConnState 逻辑优化
- `grpclient/balancer/roundrobin.go` - 移除 `StripAttributes: true`
- `grpclient/balancer/weightedroundrobin.go` - 移除 `StripAttributes: true`
- `grpclient/balancer/randomweightedroundrobin.go` - 移除 `StripAttributes: true`
- `grpclient/balancer/minconnect.go` - 移除 `StripAttributes: true`
- `grpclient/balancer/minresptime.go` - 移除 `StripAttributes: true`

**Config 结构体变更**:
```go
// 修改前
type Config struct {
    HealthCheck     bool
    StripAttributes bool // 自定义添加
}

// 修改后（与官方一致）
type Config struct {
    HealthCheck bool
}
```

### 2. 节点过滤功能改为可选

**背景**: 节点过滤会增加一定的性能开销，不需要时应该能够关闭。

**解决方案**:
- 在 `grpclient.Config` 中添加 `EnableNodeFilter` 配置项
- 默认值为 `false`，只有显式设置为 `true` 时才启用节点过滤
- 未启用时，`picker.WithNodeFilter()` 设置的过滤器将被忽略

**使用方式**:
```go
cfg := &grpclient.Config{
    Endpoints:        []string{"127.0.0.1:8081", "127.0.0.1:8082"},
    BalanceName:      balancer.RoundRobinBalanceName,
    EnableNodeFilter: true, // 启用节点过滤
}
```

### 3. etcd Watch 断线重连

**背景**: etcd Watch 可能因网络问题断开，需要自动重连。

**解决方案**:
- 实现指数退避重连机制
- 重连间隔从 1s 开始，最大 30s
- 每次重连成功后重置退避时间

### 4. 修复 filteredPicker.Pick 未使用过滤后节点的 bug

**问题**: `filteredPicker.Pick()` 方法过滤了节点后，未将过滤后的节点传递给内部 picker。

**修复**: 将过滤后的节点通过 context 传递给内部 picker。

### 5. 优化节点过滤的 Picker 缓存机制

**问题**: 原实现每次 Pick 都重新构建临时 picker，导致：
- 轮询状态被重置（RR/WRR 无法实现真正轮询）
- 统计信息丢失（P2C、MinConnect、MinRespTime 等算法失效）

**解决方案**:
- 预构建 `defaultPicker` 用于无过滤器场景
- 使用 `cachedPickers` 缓存相同过滤条件的 picker
- 相同节点集合复用同一个 picker，保持轮询状态
- 当节点状态变化时，balancer 会重建整个 picker，缓存自然清除

**设计决策**:
对于有状态的负载均衡算法（P2C、MinConnect、MinRespTime），过滤后的子集使用独立的统计数据，与全量节点不共享。这在大多数场景下是合理的：
- 过滤通常用于灰度发布、版本控制等场景
- 不同版本的节点本身就应该独立统计负载
- 例如：v1 节点的 inflight 不应影响 v2 节点的选择

### 6. 示例代码修复

修复了以下示例中缺少 `EnableNodeFilter: true` 配置的问题：

- `examples/comprehensive/main.go` - 3 处配置修复
- `examples/filter/main.go` - 1 处配置修复

## 一、新增功能

### 1. P2C (Power of Two Choices) 负载均衡器

**文件**:
- `grpclient/picker/p2cpicker.go`
- `grpclient/balancer/p2c.go`

**来源**: 参考 [go-kratos/kratos](https://github.com/go-kratos/kratos) 的实现

**特性**:
- 随机选择两个节点，选择负载更低的那个
- 使用 EWMA (指数加权移动平均) 计算节点延迟
- 跟踪 inflight 请求数和成功率
- 支持 forcePick 机制，避免节点长时间未被选中导致状态过时

**使用方式**:
```go
import "github.com/xkeyideal/grpcbalance/grpclient/balancer"

// 注册 P2C 负载均衡器
balancer.RegisterP2CBalance()

// 或带熔断器
balancer.RegisterP2CBalanceWithCircuitBreaker(cbConfig)

cfg := &grpclient.Config{
    Endpoints:   addrs,
    BalanceName: balancer.P2CBalancerName, // "p2c_x"
}
```

### 2. RPC 统计指标 (Metrics)

**文件**: `grpclient/picker/metrics.go`

**特性**:
- `DoneInfo` 结构体记录 RPC 完成信息（延迟、发送/接收字节数、服务器负载、错误）
- `Metrics` 收集器支持请求数、成功率、延迟等统计
- `MetricsCollector` 接口用于自定义指标收集

### 3. 节点过滤功能

**文件**: `grpclient/picker/filter.go`

**来源**: 参考 go-kratos 的节点过滤设计

**支持的过滤器**:
| 过滤器 | 说明 |
|--------|------|
| `VersionFilter(version)` | 精确版本匹配 |
| `VersionPrefixFilter(prefix)` | 版本前缀匹配 |
| `MetadataFilter(key, value)` | 元数据 key-value 匹配 |
| `MetadataExistsFilter(key)` | 元数据 key 存在性检查 |
| `AddressFilter(addrs...)` | 只保留指定地址 |
| `ExcludeAddressFilter(addrs...)` | 排除指定地址 |
| `HealthyFilter(checker)` | 健康状态过滤 |

**使用方式**:
```go
import "github.com/xkeyideal/grpcbalance/grpclient/picker"

// 只请求 v2.x 版本的生产环境节点
ctx := picker.WithNodeFilter(context.Background(),
    picker.VersionPrefixFilter("v2."),
    picker.MetadataFilter("env", "prod"),
)
resp, err := client.SomeRPC(ctx, req)
```

### 4. 子集选择功能

**文件**: `grpclient/resolver/subset.go`

**来源**: 参考 kratos aegis/subset

**特性**:
- 当后端节点数量过多时，限制客户端连接的节点数量
- 支持确定性选择（相同 ClientKey 选择相同子集）
- 支持随机选择
- 支持权重选择（优先选择高权重节点）
- 使用 CRC32 进行一致性哈希

**使用方式**:
```go
import "github.com/xkeyideal/grpcbalance/grpclient/resolver"

// 从 100 个节点中选择 10 个
selector := resolver.NewSubsetSelector(resolver.SubsetConfig{
    SubsetSize: 10,
    ClientKey:  "my-service-instance-01",
})
subset := selector.Select(allAddrs)
```

### 5. 平滑切换 Picker

**文件**: `grpclient/picker/graceful.go`

**来源**: 参考 grpc-go 的 gracefulswitch

**特性**:
- Picker 更新时不会立即替换
- 等待当前正在进行的请求完成后再切换
- 避免 Picker 切换时的请求失败
- 支持超时强制切换（默认 5 秒）

**使用方式**:
```go
import "github.com/xkeyideal/grpcbalance/grpclient/picker"

// 包装现有的 PickerBuilder
gracefulBuilder := picker.NewGracefulPickerBuilder(innerBuilder, 5*time.Second)
```

### 6. 服务发现支持

**文件**:
- `grpclient/discovery/discovery.go` - 核心接口和基础实现
- `grpclient/discovery/etcd.go` - etcd 服务发现（可选，需 build tag）
- `grpclient/discovery/consul.go` - Consul 服务发现（可选，需 build tag）

**特性**:
- 统一的 `Discovery` 接口，支持 Watch 和 GetEndpoints
- 自动监听端点变化并更新 resolver
- 支持静态发现、轮询发现、etcd 发现、Consul 发现
- 支持自定义发现实现（HTTP、数据库等）
- 端点更新回调函数

**Discovery 接口**:
```go
type Discovery interface {
    // Watch 监听端点变化
    Watch(ctx context.Context) (<-chan Event, error)
    // GetEndpoints 获取当前端点列表
    GetEndpoints(ctx context.Context) ([]Endpoint, error)
    // Close 关闭发现客户端
    Close() error
}
```

**使用方式**:
```go
import (
    "github.com/xkeyideal/grpcbalance/grpclient"
    "github.com/xkeyideal/grpcbalance/grpclient/discovery"
)

// 方式 1: 静态发现
staticDiscovery := discovery.NewStaticDiscovery([]string{
    "127.0.0.1:8081",
    "127.0.0.1:8082",
})

// 方式 2: 自定义发现函数 + 轮询
customDiscovery := discovery.DiscoveryFunc(func(ctx context.Context) ([]discovery.Endpoint, error) {
    // 从你的服务注册中心获取端点
    return fetchEndpointsFromRegistry()
})
pollingDiscovery := discovery.NewPollingDiscovery(customDiscovery, 30*time.Second)

// 创建客户端
cfg := &grpclient.Config{
    BalanceName:          balancer.RoundRobinBalanceName,
    Discovery:            pollingDiscovery,
    DiscoveryPollInterval: 30 * time.Second,
    OnEndpointsUpdate: func(endpoints []discovery.Endpoint) {
        log.Printf("端点已更新: %v", discovery.EndpointsToAddrs(endpoints))
    },
}

client, err := grpclient.NewClient(cfg)
```

**etcd 服务发现** (需要 build tag: `-tags etcd`):
```go
import "github.com/xkeyideal/grpcbalance/grpclient/discovery"

etcdDiscovery, err := discovery.NewEtcdDiscovery(discovery.EtcdDiscoveryConfig{
    Endpoints:   []string{"127.0.0.1:2379"},
    KeyPrefix:   "/services/myapp/",
    DialTimeout: 5 * time.Second,
})

cfg := &grpclient.Config{
    Discovery: etcdDiscovery,
}
```

**Consul 服务发现** (需要 build tag: `-tags consul`):
```go
consulDiscovery, err := discovery.NewConsulDiscovery(discovery.ConsulDiscoveryConfig{
    Address:     "127.0.0.1:8500",
    ServiceName: "myapp",
    Tags:        []string{"grpc"},
    PassingOnly: true,
})

cfg := &grpclient.Config{
    Discovery: consulDiscovery,
}
```

---

## 二、示例代码

新增了丰富的使用示例，位于 `examples/` 目录：

| 目录 | 说明 | 主要内容 |
|------|------|---------|
| `examples/basic/` | 基本负载均衡 | 轮询、加权轮询、随机加权轮询、最少连接、最小响应时间 |
| `examples/circuitbreaker/` | 熔断器功能 | 默认配置、自定义配置（FailureThreshold、OpenTimeout 等） |
| `examples/p2c/` | P2C 负载均衡 | 基本 P2C、P2C+熔断器、高并发场景 |
| `examples/filter/` | 节点过滤 | 版本过滤、元数据过滤、地址过滤、组合过滤 |
| `examples/subset/` | 子集选择 | 确定性选择、随机选择、权重选择、多客户端分布 |
| `examples/graceful/` | 平滑切换 | 基本切换、Picker 重建、并发请求场景 |
| `examples/dynamic/` | 动态端点 | 添加节点、删除节点、更新属性、替换端点 |
| `examples/discovery/` | 服务发现 | 静态发现、自定义发现、轮询发现、HTTP 发现 |
| `examples/comprehensive/` | 综合最佳实践 | 生产环境配置、多区域部署、灰度发布、大规模优化 |

---

## 三、文件变更清单

### 新增文件

| 文件路径 | 说明 |
|---------|------|
| `grpclient/picker/p2cpicker.go` | P2C 选择器实现 |
| `grpclient/balancer/p2c.go` | P2C 负载均衡器注册 |
| `grpclient/picker/metrics.go` | RPC 统计指标类型定义 |
| `grpclient/picker/filter.go` | 节点过滤器实现 |
| `grpclient/resolver/subset.go` | 子集选择器实现 |
| `grpclient/picker/graceful.go` | 平滑切换 Picker 实现 |
| `grpclient/discovery/discovery.go` | 服务发现核心接口 |
| `grpclient/discovery/etcd.go` | etcd 服务发现实现（可选） |
| `grpclient/discovery/consul.go` | Consul 服务发现实现（可选） |
| `examples/basic/main.go` | 基本负载均衡示例 |
| `examples/circuitbreaker/main.go` | 熔断器示例 |
| `examples/p2c/main.go` | P2C 示例 |
| `examples/filter/main.go` | 节点过滤示例 |
| `examples/subset/main.go` | 子集选择示例 |
| `examples/graceful/main.go` | 平滑切换示例 |
| `examples/dynamic/main.go` | 动态端点示例 |
| `examples/discovery/main.go` | 服务发现示例 |
| `examples/comprehensive/main.go` | 综合示例 |

### 修改文件

| 文件路径 | 修改内容 |
|---------|---------|
| `README.md` | 更新版本要求、添加功能特性说明、添加示例索引 |
| `grpclient/config.go` | 添加 Discovery、DiscoveryPollInterval、OnEndpointsUpdate、EnableNodeFilter 配置项 |
| `grpclient/client.go` | 添加服务发现支持、自动监听端点变化 |
| `grpclient/balancer/balancer.go` | 移除 StripAttributes，优化 UpdateClientConnState 实现 |
| `grpclient/balancer/roundrobin.go` | 移除 StripAttributes 字段 |
| `grpclient/balancer/weightedroundrobin.go` | 移除 StripAttributes 字段 |
| `grpclient/balancer/randomweightedroundrobin.go` | 移除 StripAttributes 字段 |
| `grpclient/balancer/minconnect.go` | 移除 StripAttributes 字段 |
| `grpclient/balancer/minresptime.go` | 移除 StripAttributes 字段 |
| `grpclient/picker/filter.go` | 修复 filteredPicker.Pick 未使用过滤后节点的 bug |
| `grpclient/discovery/etcd.go` | 添加 Watch 断线重连机制 |
| `examples/comprehensive/main.go` | 添加 EnableNodeFilter 配置 |
| `examples/filter/main.go` | 添加 EnableNodeFilter 配置 |

---

## 四、参考来源

本次优化参考了以下开源项目：

1. **[grpc-go](https://github.com/grpc/grpc-go)** - gRPC 官方 Go 实现
   - gracefulswitch 平滑切换机制

2. **[go-kratos/kratos](https://github.com/go-kratos/kratos)** - 微服务框架
   - P2C 负载均衡算法
   - 节点过滤设计
   - 子集选择 (aegis/subset)

3. **[etcd-io/etcd](https://github.com/etcd-io/etcd)** - 分布式 KV 存储
   - 客户端设计模式

---

## 五、快速开始

### 基本使用

```go
package main

import (
    "github.com/xkeyideal/grpcbalance/grpclient"
    "github.com/xkeyideal/grpcbalance/grpclient/balancer"
)

func main() {
    cfg := &grpclient.Config{
        Endpoints:   []string{"127.0.0.1:50051", "127.0.0.1:50052"},
        BalanceName: balancer.RoundRobinBalanceName,
        
        EnableCircuitBreaker: true, // 启用熔断器
    }
    
    client, err := grpclient.NewClient(cfg)
    if err != nil {
        panic(err)
    }
    defer client.Close()
    
    conn := client.ActiveConnection()
    // 使用 conn 创建 gRPC 客户端...
}
```

### 使用 P2C 负载均衡

```go
balancer.RegisterP2CBalance()

cfg := &grpclient.Config{
    Endpoints:   addrs,
    BalanceName: balancer.P2CBalancerName,
}
```

### 使用节点过滤

```go
ctx := picker.WithNodeFilter(context.Background(),
    picker.VersionPrefixFilter("v2."),
    picker.MetadataFilter("env", "prod"),
)
resp, err := client.SomeRPC(ctx, req)
```

### 使用子集选择

```go
selector := resolver.NewSubsetSelector(resolver.SubsetConfig{
    SubsetSize: 10,
    ClientKey:  "my-service",
})
subset := selector.Select(allAddrs)
```

---

## 六、注意事项

1. **P2C 负载均衡器需要手动注册**: 使用前需调用 `balancer.RegisterP2CBalance()` 或 `balancer.RegisterP2CBalanceWithCircuitBreaker()`

2. **节点过滤需要显式启用**: 需要在配置中设置 `EnableNodeFilter: true`，然后才能通过 `picker.WithNodeFilter()` 过滤节点

3. **子集选择器独立使用**: SubsetSelector 不与 grpclient 直接集成，需要手动过滤地址列表

4. **熔断器配置字段**:
   - `FailureThreshold` - 连续失败次数阈值
   - `SuccessThreshold` - 恢复所需成功次数
   - `OpenTimeout` - 熔断持续时间
   - `HalfOpenMaxRequests` - 半开状态最大请求数

5. **服务发现使用说明**:
   - 当配置 `Discovery` 时，`Endpoints` 配置会被忽略
   - 客户端会自动监听端点变化并更新 resolver
   - etcd/Consul 发现需要添加对应 build tag: `-tags etcd` 或 `-tags consul`
   - 需要先安装依赖: `go get go.etcd.io/etcd/client/v3` 或 `go get github.com/hashicorp/consul/api`
