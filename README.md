## gRPC-go load balancing

The gRPC-go Require

* Go 1.21+
* gRPC 1.78.0+

### Version

Each version corresponds to the corresponding version of GRPC-go.

For example, tag: v1.78.0 -> grpc-go v1.78.0

### Features

- **多种负载均衡算法**: 轮询、加权轮询、随机加权轮询、最少连接、最小响应时间、P2C
- **熔断器**: 自动隔离故障节点
- **节点过滤**: 按版本、元数据、地址过滤节点（可选功能，需显式启用）
- **子集选择**: 大规模部署时限制连接数量
- **平滑切换**: Picker 更新时不丢失请求
- **动态端点**: 运行时动态更新服务节点
- **服务发现**: 支持静态、轮询、etcd、Consul 等多种服务发现方式

### Cautions

v1.45.0 & v1.46.0-dev running panic like this:

```
panic: runtime error: comparing uncomparable type MyAttribute

goroutine 1 [running]:
google.golang.org/grpc/attributes.(*Attributes).Equal(0xc00015e4b0, 0xc00013c140)
        /project/vendor/google.golang.org/grpc/attributes/attributes.go:95 +0x194
google.golang.org/grpc/resolver.addressMapEntryList.find({0xc00013c1b8, 0x1, 0x8000102}, {{0xc00015e4c8, 0x13}, {0xc00015e4c8, 0xd}, 0xc00013c140, 0x0, 0x0, ...})
        /project/vendor/google.golang.org/grpc/resolver/map.go:49 +0xb9
google.golang.org/grpc/resolver.(*AddressMap).Get(0xc00013c188, {{0xc00015e4c8, 0x13}, {0xc00015e4c8, 0xd}, 0xc00013c140, 0x0, 0x0, {0x0, 0x0}})
        /project/vendor/google.golang.org/grpc/resolver/map.go:59 +0x94
mesh-sidecar/grpclient_balancer/balancer.(*baseBalancer).UpdateClientConnState(0xc00013b500, {{{0xc000414380, 0x3, 0x3}, 0x0, 0x0}, {0x0, 0x0}})
```

Your `attribute` needs implement `Equal` function. https://github.com/grpc/grpc-go/blob/v1.46.0-dev/attributes/attributes.go#L91


```go
type MyAttribute struct {
    attr string
}

func (ma MyAttribute) Equal(o interface{}) bool {
    return true
}
```

### How it works

The gRPC client-side load balancing to work need to main components, the [naming resolver](https://github.com/grpc/grpc/blob/master/doc/naming.md) and the [load balancing policy](https://github.com/grpc/grpc/blob/master/doc/load-balancing.md)

![load balancing work image](https://github.com/xkeyideal/grpcbalance/blob/master/examples/balancer.png)

The infra image source [itnext.io](https://itnext.io/on-grpc-load-balancing-683257c5b7b3)

### gRPC naming resolver & load balancing working principle

[On gRPC Load Balancing](https://itnext.io/on-grpc-load-balancing-683257c5b7b3)


### Running the Example Application

The gRPC client and server applications used in the example are based on the [proto/echo]((https://github.com/grpc/grpc-go/blob/master/examples/features/proto/echo/echo.proto)) & [load_balancing](https://github.com/grpc/grpc-go/blob/master/examples/features/load_balancing/README.md) examples found on the **gRPC-go examples** with the following modifications:

* The [server](https://github.com/xkeyideal/grpcbalance/blob/master/examples/server/server.go) running with port args
* The [client](https://github.com/xkeyideal/grpcbalance/blob/master/examples/client/client.go) used customized balance

### Support Balance Strategy

* round robin [balancer.RoundRobinBalanceName](https://github.com/xkeyideal/grpcbalance/blob/master/grpclient/balancer/roundrobin.go#L11)
* weighted round robin [balancer.WeightedRobinBalanceName](https://github.com/xkeyideal/grpcbalance/blob/master/grpclient/balancer/weightedroundrobin.go#L11)
* random weighted round robin [balancer.RandomWeightedRobinBalanceName](https://github.com/xkeyideal/grpcbalance/blob/master/grpclient/balancer/randomweightedroundrobin.go#L11)
* minimum connection number [balancer.MinConnectBalanceName](https://github.com/xkeyideal/grpcbalance/blob/master/grpclient/balancer/minconnect.go#L11)
* minimum response consume [balancer.MinRespTimeBalanceName](https://github.com/xkeyideal/grpcbalance/blob/master/grpclient/balancer/minresptime.go#L11), keep 10 response consume time, remove maximum and minimum and then take the average value.
* **P2C (Power of Two Choices)** [balancer.P2CBalancerName](https://github.com/xkeyideal/grpcbalance/blob/master/grpclient/balancer/p2c.go) - 随机选择两个节点，选择负载更低的那个，参考 Kratos 实现

### Examples

项目提供了丰富的使用示例，位于 `examples/` 目录：

| 目录 | 说明 |
|------|------|
| [examples/basic](examples/basic) | 基本负载均衡算法使用示例 |
| [examples/circuitbreaker](examples/circuitbreaker) | 熔断器功能使用示例 |
| [examples/p2c](examples/p2c) | P2C 负载均衡算法示例 |
| [examples/filter](examples/filter) | 节点过滤功能示例 |
| [examples/subset](examples/subset) | 子集选择功能示例 |
| [examples/graceful](examples/graceful) | 平滑切换 Picker 示例 |
| [examples/dynamic](examples/dynamic) | 动态端点更新示例 |
| [examples/discovery](examples/discovery) | 服务发现功能使用示例 |
| [examples/comprehensive](examples/comprehensive) | 综合使用最佳实践 |

### Configuration

#### 主要配置项

| 配置项 | 类型 | 说明 |
|-------|------|------|
| `Endpoints` | `[]string` | 服务端点列表 |
| `BalanceName` | `string` | 负载均衡算法名称 |
| `EnableCircuitBreaker` | `bool` | 是否启用熔断器 |
| `EnableNodeFilter` | `bool` | 是否启用节点过滤功能（默认关闭） |
| `Discovery` | `discovery.Discovery` | 服务发现实现（设置后 Endpoints 被忽略） |
| `DiscoveryPollInterval` | `time.Duration` | 服务发现轮询间隔（默认 30s） |

#### 快速开始

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

#### 使用 P2C 负载均衡

```go
import (
    "github.com/xkeyideal/grpcbalance/grpclient/balancer"
)

// 先注册 P2C 负载均衡器
balancer.RegisterP2CBalance()

cfg := &grpclient.Config{
    Endpoints:   addrs,
    BalanceName: balancer.P2CBalancerName,
}
```

#### 使用节点过滤

```go
import (
    "github.com/xkeyideal/grpcbalance/grpclient"
    "github.com/xkeyideal/grpcbalance/grpclient/picker"
)

// 配置中启用节点过滤
cfg := &grpclient.Config{
    Endpoints:        addrs,
    BalanceName:      balancer.RoundRobinBalanceName,
    EnableNodeFilter: true, // 必须显式启用
}

// 然后在请求时过滤节点
ctx := picker.WithNodeFilter(context.Background(),
    picker.VersionPrefixFilter("v2."),
    picker.MetadataFilter("env", "prod"),
)
resp, err := client.SomeRPC(ctx, req)
```

#### 使用子集选择

```go
import (
    "github.com/xkeyideal/grpcbalance/grpclient/resolver"
)

// 从 100 个节点中选择 10 个
selector := resolver.NewSubsetSelector(resolver.SubsetConfig{
    SubsetSize: 10,
    ClientKey:  "my-service",
})
subset := selector.Select(allAddrs)
```

#### 使用服务发现

```go
import (
    "github.com/xkeyideal/grpcbalance/grpclient"
    "github.com/xkeyideal/grpcbalance/grpclient/discovery"
)

// 静态发现
staticDiscovery := discovery.NewStaticDiscovery([]string{
    "127.0.0.1:8081",
    "127.0.0.1:8082",
})

// 轮询发现（自定义获取端点的函数）
pollingDiscovery := discovery.NewPollingDiscovery(
    discovery.DiscoveryFunc(func(ctx context.Context) ([]discovery.Endpoint, error) {
        return fetchEndpointsFromRegistry()
    }),
    30*time.Second,
)

cfg := &grpclient.Config{
    BalanceName: balancer.RoundRobinBalanceName,
    Discovery:   pollingDiscovery, // 设置后 Endpoints 被忽略
}
```

### Changelog

详细的变更记录请参阅 [upgrade.md](upgrade.md)。

### Customize Advanced Balancing Strategy

1. Modify [naming resolver](https://github.com/xkeyideal/grpcbalance/blob/master/grpclient/resolver/resolver.go) with your requirements, first set [attributes.Attributes](https://github.com/grpc/grpc-go/blob/master/attributes/attributes.go) for per endpoint address, second when one endpoint attributes.Attributes changed then update subConn state.

2. Implement yourself balancer & picker function, then based on [attributes.Attributes](https://github.com/grpc/grpc-go/blob/master/attributes/attributes.go) picker subConn in `Pick(balancer.PickInfo) (balancer.PickResult, error)`

### License

Apache 2.0 license.