// Package main 展示了综合使用所有功能的最佳实践
// 包括负载均衡、熔断器、节点过滤、子集选择等
package main

import (
	"context"
	"log"
	"time"

	pb "github.com/xkeyideal/grpcbalance/examples/proto/echo"
	"github.com/xkeyideal/grpcbalance/grpclient"
	"github.com/xkeyideal/grpcbalance/grpclient/balancer"
	"github.com/xkeyideal/grpcbalance/grpclient/circuitbreaker"
	"github.com/xkeyideal/grpcbalance/grpclient/picker"
	"github.com/xkeyideal/grpcbalance/grpclient/resolver"

	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/metadata"
	grpcResolver "google.golang.org/grpc/resolver"
)

func main() {
	log.Println("=== grpcbalance 综合使用示例 ===")
	log.Println("本示例展示如何组合使用各种功能构建生产级的 gRPC 客户端")

	// 示例1: 生产环境配置
	log.Println("\n--- 示例1: 生产环境推荐配置 ---")
	productionExample()

	// 示例2: 多区域部署
	log.Println("\n--- 示例2: 多区域部署策略 ---")
	multiRegionExample()

	// 示例3: 灰度发布
	log.Println("\n--- 示例3: 灰度发布策略 ---")
	canaryDeploymentExample()

	// 示例4: 大规模部署优化
	log.Println("\n--- 示例4: 大规模部署优化 ---")
	largeScaleOptimizationExample()
}

// productionExample 展示生产环境推荐配置
func productionExample() {
	// 1. 设置节点属性 (版本、区域、权重)
	attrs := make(map[string]*attributes.Attributes)
	nodeConfig := []struct {
		addr    string
		version string
		region  string
		env     string
		weight  int32
	}{
		{"10.0.1.1:50051", "v2.1.0", "cn-north", "prod", 3},
		{"10.0.1.2:50051", "v2.1.0", "cn-north", "prod", 3},
		{"10.0.1.3:50051", "v2.1.0", "cn-north", "prod", 2},
		{"10.0.2.1:50051", "v2.1.0", "cn-south", "prod", 3},
		{"10.0.2.2:50051", "v2.1.0", "cn-south", "prod", 3},
	}

	var addrs []string
	for _, node := range nodeConfig {
		addrs = append(addrs, node.addr)
		attrs[node.addr] = attributes.New(
			picker.WeightAttributeKey, node.weight,
		).WithValue(
			picker.VersionFilterKey, node.version,
		).WithValue(
			picker.MetadataFilterKey, map[string]string{
				"region": node.region,
				"env":    node.env,
			},
		)
	}

	// 2. 配置熔断器
	cbConfig := &circuitbreaker.Config{
		FailureThreshold:    5,
		OpenTimeout:         10 * time.Second,
		HalfOpenMaxRequests: 3,
		SuccessThreshold:    2,
	}

	// 3. 创建客户端
	cfg := &grpclient.Config{
		Endpoints:         addrs,
		BalanceName:       balancer.MinRespTimeBalanceName, // 使用最小响应时间算法
		Attributes:        attrs,
		EnableHealthCheck: true,

		EnableCircuitBreaker: true,
		CircuitBreakerConfig: cbConfig,
		EnableNodeFilter:     true, // 启用节点过滤

		DialTimeout:          10 * time.Second,
		DialKeepAliveTime:    15 * time.Second, // 稍长的保活间隔
		DialKeepAliveTimeout: 5 * time.Second,
		PermitWithoutStream:  true,
	}

	client, err := grpclient.NewClient(cfg)
	if err != nil {
		log.Printf("创建客户端失败: %v", err)
		return
	}
	defer client.Close()

	log.Println("生产环境配置:")
	log.Println("  - 负载均衡: 最小响应时间 (自动选择最快的节点)")
	log.Println("  - 熔断器: 连续 5 次失败后熔断")
	log.Println("  - 节点属性: 版本、区域、权重")
	log.Println("  - 保活: 15 秒检测一次")

	cc := client.ActiveConnection()
	echoClient := pb.NewEchoClient(cc)

	// 只请求生产环境
	ctx := picker.WithNodeFilter(context.Background(),
		picker.MetadataFilter("env", "prod"),
	)
	for i := 0; i < 5; i++ {
		callEchoWithContext(echoClient, ctx, "生产环境请求")
	}
}

// multiRegionExample 展示多区域部署策略
func multiRegionExample() {
	// 为节点设置区域信息
	attrs := make(map[string]*attributes.Attributes)
	regions := map[string]string{
		"10.0.1.1:50051": "cn-north",
		"10.0.1.2:50051": "cn-north",
		"10.0.2.1:50051": "cn-south",
		"10.0.2.2:50051": "cn-south",
		"10.0.3.1:50051": "cn-east",
	}

	var addrs []string
	for addr, region := range regions {
		addrs = append(addrs, addr)
		attrs[addr] = attributes.New(
			picker.MetadataFilterKey, map[string]string{"region": region},
		)
	}

	cfg := &grpclient.Config{
		Endpoints:            addrs,
		BalanceName:          balancer.RoundRobinBalanceName,
		Attributes:           attrs,
		EnableHealthCheck:    true,
		EnableCircuitBreaker: true,
		EnableNodeFilter:     true, // 启用节点过滤

		DialTimeout:          10 * time.Second,
		DialKeepAliveTime:    10 * time.Second,
		DialKeepAliveTimeout: 2 * time.Second,
		PermitWithoutStream:  true,
	}

	client, err := grpclient.NewClient(cfg)
	if err != nil {
		log.Printf("创建客户端失败: %v", err)
		return
	}
	defer client.Close()

	cc := client.ActiveConnection()
	echoClient := pb.NewEchoClient(cc)

	log.Println("多区域部署策略:")

	// 策略1: 就近访问
	log.Println("\n1. 就近访问 (只请求华北区域):")
	ctx := picker.WithNodeFilter(context.Background(),
		picker.MetadataFilter("region", "cn-north"),
	)
	for i := 0; i < 4; i++ {
		callEchoWithContext(echoClient, ctx, "华北区域请求")
	}

	// 策略2: 跨区域容灾
	log.Println("\n2. 跨区域容灾 (华北或华南):")
	ctx = picker.WithNodeFilter(context.Background(),
		func(info picker.SubConnInfo) bool {
			if info.Address.Attributes == nil {
				return false
			}
			v := info.Address.Attributes.Value(picker.MetadataFilterKey)
			if v == nil {
				return false
			}
			if metadata, ok := v.(map[string]string); ok {
				region := metadata["region"]
				return region == "cn-north" || region == "cn-south"
			}
			return false
		},
	)
	for i := 0; i < 4; i++ {
		callEchoWithContext(echoClient, ctx, "跨区域请求")
	}
}

// canaryDeploymentExample 展示灰度发布策略
func canaryDeploymentExample() {
	// 设置版本信息
	attrs := make(map[string]*attributes.Attributes)
	versions := map[string]string{
		"10.0.1.1:50051": "v2.0.0", // 稳定版
		"10.0.1.2:50051": "v2.0.0", // 稳定版
		"10.0.1.3:50051": "v2.0.0", // 稳定版
		"10.0.1.4:50051": "v2.1.0", // 灰度版
		"10.0.1.5:50051": "v2.1.0", // 灰度版
	}

	var addrs []string
	for addr, version := range versions {
		addrs = append(addrs, addr)
		attrs[addr] = attributes.New(picker.VersionFilterKey, version)
	}

	cfg := &grpclient.Config{
		Endpoints:            addrs,
		BalanceName:          balancer.RoundRobinBalanceName,
		Attributes:           attrs,
		EnableHealthCheck:    true,
		EnableCircuitBreaker: true,
		EnableNodeFilter:     true, // 启用节点过滤

		DialTimeout:          10 * time.Second,
		DialKeepAliveTime:    10 * time.Second,
		DialKeepAliveTimeout: 2 * time.Second,
		PermitWithoutStream:  true,
	}

	client, err := grpclient.NewClient(cfg)
	if err != nil {
		log.Printf("创建客户端失败: %v", err)
		return
	}
	defer client.Close()

	cc := client.ActiveConnection()
	echoClient := pb.NewEchoClient(cc)

	log.Println("灰度发布策略:")

	// 正常流量: 只访问稳定版
	log.Println("\n1. 正常流量 (只访问稳定版 v2.0.0):")
	ctx := picker.WithNodeFilter(context.Background(),
		picker.VersionFilter("v2.0.0"),
	)
	for i := 0; i < 6; i++ {
		callEchoWithContext(echoClient, ctx, "稳定版请求")
	}

	// 灰度流量: 只访问灰度版
	log.Println("\n2. 灰度流量 (只访问灰度版 v2.1.0):")
	ctx = picker.WithNodeFilter(context.Background(),
		picker.VersionFilter("v2.1.0"),
	)
	for i := 0; i < 4; i++ {
		callEchoWithContext(echoClient, ctx, "灰度版请求")
	}

	// 回滚: 排除灰度版
	log.Println("\n3. 回滚策略 (排除灰度版):")
	ctx = picker.WithNodeFilter(context.Background(),
		picker.VersionPrefixFilter("v2.0"),
	)
	for i := 0; i < 4; i++ {
		callEchoWithContext(echoClient, ctx, "回滚请求")
	}
}

// largeScaleOptimizationExample 展示大规模部署优化
func largeScaleOptimizationExample() {
	// 模拟 1000 个节点
	var addrs []grpcResolver.Address
	for i := 0; i < 1000; i++ {
		addrs = append(addrs, grpcResolver.Address{
			Addr: "10.0." + string(rune('0'+i/256)) + "." + string(rune('0'+i%256)) + ":50051",
		})
	}

	log.Printf("总节点数: %d", len(addrs))

	// 使用子集选择器
	subsetConfig := resolver.SubsetConfig{
		SubsetSize: 20,              // 每个客户端只连接 20 个节点
		ClientKey:  "my-service-01", // 客户端标识
	}
	selector := resolver.NewSubsetSelector(subsetConfig)
	subset := selector.Select(addrs)

	log.Printf("子集大小: %d", len(subset))
	log.Println("选择的节点子集 (前 5 个):")
	for i := 0; i < 5 && i < len(subset); i++ {
		log.Printf("  - %s", subset[i].Addr)
	}

	log.Println("\n大规模部署优化策略:")
	log.Println("  1. 子集选择: 每个客户端只连接少量节点")
	log.Println("     - 减少连接数，降低资源消耗")
	log.Println("     - 提高连接复用率")
	log.Println("     - 相同 ClientKey 保证连接一致性")
	log.Println("  2. P2C 负载均衡: 自动避免高负载节点")
	log.Println("     - 随机选择两个节点，选择负载更低的")
	log.Println("     - 避免惊群效应")
	log.Println("  3. 熔断器: 隔离故障节点")
	log.Println("     - 快速失败，避免雪崩")
	log.Println("  4. 平滑切换: 节点变更时不丢失请求")
	log.Println("     - 等待进行中的请求完成")
}

func callEchoWithContext(c pb.EchoClient, ctx context.Context, message string) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	md := metadata.Pairs()
	ctx = metadata.NewOutgoingContext(ctx, md)

	resp, err := c.UnaryEcho(ctx, &pb.EchoRequest{Message: message})
	if err != nil {
		log.Printf("调用失败: %v", err)
		return
	}
	log.Printf("响应: %s", resp.Message)
}
