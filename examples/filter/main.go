// Package main 展示了节点过滤功能
// 可以按版本、元数据、地址等条件过滤服务节点
package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	pb "github.com/xkeyideal/grpcbalance/examples/proto/echo"
	"github.com/xkeyideal/grpcbalance/grpclient"
	"github.com/xkeyideal/grpcbalance/grpclient/balancer"
	"github.com/xkeyideal/grpcbalance/grpclient/picker"

	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/metadata"
)

var addrs = []string{
	"127.0.0.1:50051",
	"127.0.0.1:50052",
	"127.0.0.1:50053",
}

func main() {
	log.Println("=== 节点过滤功能示例 ===")

	// 示例1: 按版本过滤
	log.Println("\n--- 示例1: 按版本过滤 ---")
	versionFilterExample()

	// 示例2: 按元数据过滤
	log.Println("\n--- 示例2: 按元数据过滤 ---")
	metadataFilterExample()

	// 示例3: 按地址过滤
	log.Println("\n--- 示例3: 按地址过滤 ---")
	addressFilterExample()

	// 示例4: 组合过滤器
	log.Println("\n--- 示例4: 组合过滤器 ---")
	combinedFilterExample()
}

// versionFilterExample 展示按版本过滤节点
func versionFilterExample() {
	// 为每个节点设置版本信息
	attrs := make(map[string]*attributes.Attributes)
	versions := []string{"v1.0.0", "v1.1.0", "v2.0.0"}

	for i, addr := range addrs {
		attrs[addr] = attributes.New(picker.VersionFilterKey, versions[i])
	}

	cfg := &grpclient.Config{
		Endpoints:        addrs,
		BalanceName:      balancer.RoundRobinBalanceName,
		Attributes:       attrs,
		EnableNodeFilter: true, // 启用节点过滤功能

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

	log.Println("节点版本配置:")
	for i, addr := range addrs {
		log.Printf("  %s: %s", addr, versions[i])
	}

	// 只请求 v1.x 版本的节点
	log.Println("\n使用 VersionPrefixFilter 过滤 v1.x 版本的节点:")
	ctx := context.Background()
	ctx = picker.WithNodeFilter(ctx, picker.VersionPrefixFilter("v1."))
	for i := 0; i < 6; i++ {
		callEchoWithContext(echoClient, ctx, "版本过滤测试")
	}

	// 只请求指定版本
	log.Println("\n使用 VersionFilter 过滤 v2.0.0 版本的节点:")
	ctx = context.Background()
	ctx = picker.WithNodeFilter(ctx, picker.VersionFilter("v2.0.0"))
	for i := 0; i < 6; i++ {
		callEchoWithContext(echoClient, ctx, "精确版本过滤测试")
	}
}

// metadataFilterExample 展示按元数据过滤节点
func metadataFilterExample() {
	// 为每个节点设置元数据
	attrs := make(map[string]*attributes.Attributes)

	// 模拟不同区域和环境的节点
	nodeMetadata := []map[string]string{
		{"region": "cn-north", "env": "prod"},
		{"region": "cn-south", "env": "prod"},
		{"region": "cn-east", "env": "staging"},
	}

	for i, addr := range addrs {
		attrs[addr] = attributes.New(picker.MetadataFilterKey, nodeMetadata[i])
	}

	cfg := &grpclient.Config{
		Endpoints:        addrs,
		BalanceName:      balancer.RoundRobinBalanceName,
		Attributes:       attrs,
		EnableNodeFilter: true, // 启用节点过滤功能

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

	log.Println("节点元数据配置:")
	for i, addr := range addrs {
		log.Printf("  %s: region=%s, env=%s", addr, nodeMetadata[i]["region"], nodeMetadata[i]["env"])
	}

	// 只请求生产环境的节点
	log.Println("\n过滤生产环境节点 (env=prod):")
	ctx := context.Background()
	ctx = picker.WithNodeFilter(ctx, picker.MetadataFilter("env", "prod"))
	for i := 0; i < 3; i++ {
		callEchoWithContext(echoClient, ctx, "生产环境请求")
	}

	// 只请求华北区域的节点
	log.Println("\n过滤华北区域节点 (region=cn-north):")
	ctx = context.Background()
	ctx = picker.WithNodeFilter(ctx, picker.MetadataFilter("region", "cn-north"))
	for i := 0; i < 3; i++ {
		callEchoWithContext(echoClient, ctx, "华北区域请求")
	}

	// 检查元数据 key 是否存在
	log.Println("\n检查具有 'region' 元数据的节点:")
	ctx = context.Background()
	ctx = picker.WithNodeFilter(ctx, picker.MetadataExistsFilter("region"))
	for i := 0; i < 3; i++ {
		callEchoWithContext(echoClient, ctx, "有region元数据的请求")
	}
}

// addressFilterExample 展示按地址过滤节点
func addressFilterExample() {
	cfg := &grpclient.Config{
		Endpoints:        addrs,
		BalanceName:      balancer.RoundRobinBalanceName,
		EnableNodeFilter: true, // 启用节点过滤功能

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

	// 只请求特定地址 (50051, 50052)
	log.Println("只请求特定地址 (50051, 50052):")
	ctx := context.Background()
	ctx = picker.WithNodeFilter(ctx, picker.AddressFilter("127.0.0.1:50051", "127.0.0.1:50052"))

	var ports []string
	for i := 0; i < 6; i++ {
		port := callEchoAndExtractPort(echoClient, ctx, "指定地址请求")
		ports = append(ports, port)
	}
	verifyFilterResult(ports, []string{"50051", "50052"}, "AddressFilter(50051, 50052)")

	// 排除特定地址 (50053)
	log.Println("\n排除特定地址 (50053):")
	ctx = context.Background()
	ctx = picker.WithNodeFilter(ctx, picker.ExcludeAddressFilter("127.0.0.1:50053"))

	ports = nil
	for i := 0; i < 6; i++ {
		port := callEchoAndExtractPort(echoClient, ctx, "排除地址请求")
		ports = append(ports, port)
	}
	verifyFilterResult(ports, []string{"50051", "50052"}, "ExcludeAddressFilter(50053)")

	// 只请求单个地址 (50051)
	log.Println("\n只请求单个地址 (50051):")
	ctx = context.Background()
	ctx = picker.WithNodeFilter(ctx, picker.AddressFilter("127.0.0.1:50051"))

	ports = nil
	for i := 0; i < 4; i++ {
		port := callEchoAndExtractPort(echoClient, ctx, "单地址请求")
		ports = append(ports, port)
	}
	verifyFilterResult(ports, []string{"50051"}, "AddressFilter(50051)")
}

// combinedFilterExample 展示组合多个过滤器
func combinedFilterExample() {
	// 为节点设置版本和元数据
	attrs := make(map[string]*attributes.Attributes)
	versions := []string{"v1.0.0", "v1.1.0", "v2.0.0"}
	nodeMetadata := []map[string]string{
		{"region": "cn-north", "env": "prod"},
		{"region": "cn-south", "env": "prod"},
		{"region": "cn-east", "env": "staging"},
	}

	for i, addr := range addrs {
		attrs[addr] = attributes.New(
			picker.VersionFilterKey, versions[i],
		).WithValue(
			picker.MetadataFilterKey, nodeMetadata[i],
		)
	}

	cfg := &grpclient.Config{
		Endpoints:        addrs,
		BalanceName:      balancer.RoundRobinBalanceName,
		Attributes:       attrs,
		EnableNodeFilter: true, // 启用节点过滤功能

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

	// 组合过滤: v1.x + 生产环境
	log.Println("组合过滤: v1.x 版本 + 生产环境:")
	ctx := context.Background()
	ctx = picker.WithNodeFilter(ctx,
		picker.VersionPrefixFilter("v1."),
		picker.MetadataFilter("env", "prod"),
	)

	var ports []string
	for i := 0; i < 6; i++ {
		port := callEchoAndExtractPort(echoClient, ctx, "组合过滤请求")
		ports = append(ports, port)
	}
	verifyFilterResult(ports, []string{"50051", "50052"}, "v1.x + prod环境")

	log.Println("\n预期结果: 只有 50051 和 50052 满足条件 (v1.0.0+prod, v1.1.0+prod)")
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

// callEchoAndExtractPort 调用 Echo 服务并从响应中提取端口号
// 服务器响应格式期望包含端口信息，用于验证过滤是否生效
func callEchoAndExtractPort(c pb.EchoClient, ctx context.Context, message string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	md := metadata.Pairs()
	ctx = metadata.NewOutgoingContext(ctx, md)

	resp, err := c.UnaryEcho(ctx, &pb.EchoRequest{Message: message})
	if err != nil {
		log.Printf("调用失败: %v", err)
		return ""
	}

	// 从响应消息中提取端口号
	// 假设服务器返回格式包含端口，如 "Echo from :50051: message" 或类似格式
	port := extractPortFromResponse(resp.Message)
	log.Printf("响应: %s (端口: %s)", resp.Message, port)
	return port
}

// extractPortFromResponse 从响应消息中提取端口号
func extractPortFromResponse(message string) string {
	// 尝试多种格式匹配端口
	// 格式1: 包含 ":5005x" 的字符串
	for _, port := range []string{"50051", "50052", "50053"} {
		if strings.Contains(message, port) {
			return port
		}
	}
	// 格式2: 查找 ":端口" 模式
	if idx := strings.LastIndex(message, ":"); idx != -1 {
		portPart := strings.TrimSpace(message[idx+1:])
		// 提取数字部分
		for i, c := range portPart {
			if c < '0' || c > '9' {
				if i > 0 {
					return portPart[:i]
				}
				break
			}
		}
		if len(portPart) > 0 && portPart[0] >= '0' && portPart[0] <= '9' {
			return portPart
		}
	}
	return "unknown"
}

// verifyFilterResult 验证过滤结果是否符合预期
func verifyFilterResult(ports []string, expectedPorts []string, filterDesc string) {
	// 统计端口出现次数
	portCount := make(map[string]int)
	for _, p := range ports {
		portCount[p]++
	}

	// 检查是否只有预期的端口
	expectedSet := make(map[string]bool)
	for _, p := range expectedPorts {
		expectedSet[p] = true
	}

	allValid := true
	for port := range portCount {
		if port != "" && port != "unknown" && !expectedSet[port] {
			allValid = false
			log.Printf("❌ 过滤失败 [%s]: 出现了非预期端口 %s", filterDesc, port)
		}
	}

	if allValid {
		log.Printf("✅ 过滤验证通过 [%s]: 只访问了预期端口 %v", filterDesc, expectedPorts)
	}

	// 打印统计
	fmt.Printf("   端口访问统计: ")
	for port, count := range portCount {
		fmt.Printf("%s=%d次 ", port, count)
	}
	fmt.Println()
}
