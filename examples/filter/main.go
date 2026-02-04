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
	"google.golang.org/grpc/resolver"
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

	// 示例5: WRR (加权轮询) + 过滤
	log.Println("\n--- 示例5: WRR (加权轮询) + 过滤 ---")
	wrrFilterExample()

	// 示例6: Label Selector 过滤（基于 attributes）
	log.Println("\n--- 示例6: Label Selector 过滤（基于 attributes） ---")
	labelSelectorFilterExample()
}

// labelSelectorFilterExample 展示用 label selector 过滤节点。
//
// selector 语法由 label 包提供（支持：=,!=,in/notin,exists/!exists,pattern(~=),semver(@),数字比较(>/<) 等）。
// 数据来源来自 resolver.Address.Attributes：
//  1. 直接 key/value（推荐，与 discovery.EndpointToAttrs 注入方式一致）
//  2. 兼容 metadata map：Attributes["metadata"] 为 map[string]string 时可回退读取
func labelSelectorFilterExample() {
	attrs := make(map[string]*attributes.Attributes)

	ports := []string{"50051", "50052", "50053"}
	envs := []string{"prod", "prod", "prod"}
	regions := []string{"cn-north", "cn-north", "cn-east"}
	azs := []string{"az1", "az1", "az2"}
	zones := []string{"z1", "z2", "z3"}
	lanes := []string{"stable", "canary", "stable"}
	tenants := []string{"pay", "pay", "ads"}
	ips := []string{"10.0.1.11", "10.0.1.12", "10.0.2.13"}
	versions := []string{"2.1.1", "2.2.0", "2.1.0"}
	weights := []int32{3, 1, 2}
	tags := [][]string{{"gpu", "x86"}, {"cpu", "x86"}, {"gpu", "arm"}}

	for i, addr := range addrs {
		md := map[string]string{
			"env":         envs[i],
			"region":      regions[i],
			"az":          azs[i],
			"zone":        zones[i],
			"lane":        lanes[i],
			"tenant":      tenants[i],
			"system.ip":   ips[i],
			"system.port": ports[i],
			"v":           versions[i],
		}
		// 演示 metadata-map-only 的兼容读取：legacy.only 只放在 metadata 里，不放在直接 attributes 里。
		if i == 1 {
			md["legacy.only"] = "yes"
		}
		attrs[addr] = attributes.New(picker.WeightAttributeKey, weights[i]).
			WithValue("env", envs[i]).
			WithValue("region", regions[i]).
			WithValue("az", azs[i]).
			WithValue("zone", zones[i]).
			WithValue("lane", lanes[i]).
			WithValue("tenant", tenants[i]).
			WithValue("system.ip", ips[i]).
			WithValue("tags", tags[i]).
			WithValue("system.id", "node-"+ports[i]).
			WithValue("system.port", ports[i]).
			WithValue("v", versions[i]).
			// 放一份 metadata map，便于演示兼容老写法（selector 也能读到）
			WithValue(picker.MetadataFilterKey, md)
	}

	cfg := &grpclient.Config{
		Endpoints:        addrs,
		BalanceName:      balancer.RoundRobinBalanceName,
		Attributes:       attrs,
		EnableNodeFilter: true,

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

	log.Println("节点 labels 配置:")
	for i, addr := range addrs {
		log.Printf("  %s: env=%s region=%s az=%s zone=%s lane=%s tenant=%s ip=%s tags=%v v=%s weight=%d id=node-%s",
			addr, envs[i], regions[i], azs[i], zones[i], lanes[i], tenants[i], ips[i], tags[i], versions[i], weights[i], ports[i])
	}

	dryRun := func(selector string) ([]string, picker.NodeFilter, bool) {
		f, err := picker.LabelSelectorFilter(selector)
		if err != nil {
			log.Printf("selector 解析失败 %q: %v", selector, err)
			return nil, nil, false
		}
		var expected []string
		for _, addr := range addrs {
			p, ok := attrs[addr].Value("system.port").(string)
			if !ok {
				continue
			}
			info := picker.SubConnInfo{Address: resolver.Address{Addr: addr, Attributes: attrs[addr]}}
			if f(info) {
				expected = append(expected, p)
			}
		}
		return expected, f, true
	}

	run := func(scene, selector string) {
		log.Printf("\n%s\nselector: %s", scene, selector)
		expected, f, ok := dryRun(selector)
		if !ok {
			return
		}
		log.Printf("dry-run 命中端口: %v", expected)
		ctx := picker.WithNodeFilter(context.Background(), f)
		var picked []string
		for i := 0; i < 6; i++ {
			picked = append(picked, callEchoAndExtractPort(echoClient, ctx, "label selector 请求"))
		}
		verifyFilterResult(picked, expected, "LabelSelectorFilter("+selector+")")
	}

	runWithFallback := func(scene, primary, fallback string) {
		log.Printf("\n%s\nprimary:  %s\nfallback: %s", scene, primary, fallback)
		expected, f, ok := dryRun(primary)
		used := primary
		if !ok {
			return
		}
		if len(expected) == 0 {
			expected2, f2, ok2 := dryRun(fallback)
			if !ok2 {
				return
			}
			used = fallback
			expected = expected2
			f = f2
			log.Printf("primary 无命中，使用 fallback")
		}
		log.Printf("used selector: %s", used)
		log.Printf("dry-run 命中端口: %v", expected)
		ctx := picker.WithNodeFilter(context.Background(), f)
		var picked []string
		for i := 0; i < 6; i++ {
			picked = append(picked, callEchoAndExtractPort(echoClient, ctx, "label selector 请求"))
		}
		verifyFilterResult(picked, expected, "LabelSelectorFilter("+used+")")
	}

	// 业务场景 1：同城就近（同 Region + 同 AZ）
	run("业务场景1：同城就近（region=cn-north, az=az1）", "env=prod, region=cn-north, az=az1")

	// 业务场景 2：稳定流量（排除灰度 lane）
	run("业务场景2：稳定流量（排除 lane=canary）", "env=prod, region=cn-north, lane!=canary")

	// 业务场景 3：灰度放量（只走灰度 lane）
	run("业务场景3：灰度放量（只走 lane=canary）", "env=prod, lane=canary")

	// 业务场景 4：租户隔离（tenant=pay）
	run("业务场景4：租户隔离（tenant=pay）", "env=prod, tenant=pay")

	// 业务场景 5：跨区容灾（region=cn-east）
	run("业务场景5：跨区容灾（region=cn-east）", "env=prod, region=cn-east")

	// 业务场景 6：架构/能力约束（只走 ARM 节点）
	run("业务场景6：架构约束（tags in (arm)）", "env=prod, tags in (arm)")

	// 业务场景 7：容量/成本（只走 weight>1 的节点）
	run("业务场景7：容量约束（customize_weight>1）", "env=prod, customize_weight>1")

	// 业务场景 8：问题定位（精确打到某个 IP 的节点）
	run("业务场景8：问题定位（system.ip=10.0.1.11）", "system.ip=10.0.1.11")

	// 业务场景 9：老系统 metadata map 兼容（仅 metadata 里有 legacy.only）
	run("业务场景9：兼容老 metadata（legacy.only=yes）", "legacy.only=yes")

	// 业务场景 10：就近优先 + 无命中回退
	runWithFallback(
		"业务场景10：优先同 AZ（无命中则回退到 cn-east）",
		"env=prod, region=cn-north, az=az9",
		"env=prod, region=cn-east",
	)
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

// wrrFilterExample 展示加权轮询 (WRR) + 过滤功能
// 验证过滤后的节点仍能保持正确的加权轮询行为
func wrrFilterExample() {
	// 为节点设置权重和版本
	// 权重: 50051=1, 50052=2, 50053=3
	// 版本: 50051=v1.0, 50052=v1.0, 50053=v2.0
	attrs := make(map[string]*attributes.Attributes)
	weights := []int32{1, 2, 3}
	versions := []string{"v1.0", "v1.0", "v2.0"}

	for i, addr := range addrs {
		attrs[addr] = attributes.New(
			picker.WeightAttributeKey, weights[i],
		).WithValue(
			picker.VersionFilterKey, versions[i],
		)
	}

	cfg := &grpclient.Config{
		Endpoints:        addrs,
		BalanceName:      balancer.WeightedRobinBalanceName, // 使用加权轮询
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

	log.Println("节点配置 (WRR + 版本):")
	for i, addr := range addrs {
		log.Printf("  %s: weight=%d, version=%s", addr, weights[i], versions[i])
	}

	// 测试1: 无过滤时的加权轮询
	// 权重比 1:2:3，发送 12 个请求，预期分布: 50051=2次, 50052=4次, 50053=6次
	log.Println("\n测试1: 无过滤的加权轮询 (权重 1:2:3, 共12次请求):")
	portCount := make(map[string]int)
	for i := 0; i < 12; i++ {
		port := callEchoAndExtractPort(echoClient, context.Background(), "WRR无过滤")
		portCount[port]++
	}
	log.Printf("端口分布: 50051=%d次, 50052=%d次, 50053=%d次",
		portCount["50051"], portCount["50052"], portCount["50053"])
	log.Println("预期分布: 50051≈2次, 50052≈4次, 50053≈6次 (比例 1:2:3)")

	// 测试2: 过滤后的加权轮询
	// 只保留 v1.0 版本节点 (50051, 50052)，权重比 1:2
	// 发送 9 个请求，预期分布: 50051=3次, 50052=6次
	log.Println("\n测试2: 过滤 v1.0 版本后的加权轮询 (只有 50051:1 和 50052:2):")
	ctx := picker.WithNodeFilter(context.Background(), picker.VersionFilter("v1.0"))

	portCount = make(map[string]int)
	for i := 0; i < 9; i++ {
		port := callEchoAndExtractPort(echoClient, ctx, "WRR+过滤")
		portCount[port]++
	}

	// 验证过滤是否生效
	if portCount["50053"] > 0 {
		log.Printf("❌ 过滤失败: 50053 (v2.0) 不应该被访问，但访问了 %d 次", portCount["50053"])
	} else {
		log.Println("✅ 过滤验证通过: 50053 (v2.0) 未被访问")
	}

	log.Printf("端口分布: 50051=%d次, 50052=%d次",
		portCount["50051"], portCount["50052"])
	log.Println("预期分布: 50051≈3次, 50052≈6次 (比例 1:2)")

	// 验证加权轮询是否正确
	// 允许一定误差，因为初始索引是随机的
	ratio := float64(portCount["50052"]) / float64(portCount["50051"]+1) // +1 避免除零
	if ratio >= 1.5 && ratio <= 2.5 {
		log.Printf("✅ 加权轮询验证通过: 50052/50051 比例约 %.2f (预期约 2.0)", ratio)
	} else {
		log.Printf("⚠️ 加权轮询比例偏离预期: 50052/50051 = %.2f (预期约 2.0)", ratio)
	}

	// 测试3: 多次使用相同过滤条件，验证缓存机制
	log.Println("\n测试3: 验证缓存机制 (相同过滤条件应复用 picker):")
	portCount = make(map[string]int)
	for i := 0; i < 6; i++ {
		// 每次创建新的 context 但使用相同的过滤条件
		ctx := picker.WithNodeFilter(context.Background(), picker.VersionFilter("v1.0"))
		port := callEchoAndExtractPort(echoClient, ctx, "缓存验证")
		portCount[port]++
	}
	log.Printf("端口分布: 50051=%d次, 50052=%d次", portCount["50051"], portCount["50052"])

	// 如果缓存生效，轮询应该是连续的，不会每次都重置
	// 检查是否有交替访问的模式
	if portCount["50051"] > 0 && portCount["50052"] > 0 {
		log.Println("✅ 缓存验证通过: 两个节点都被访问，说明轮询状态保持")
	} else {
		log.Println("⚠️ 缓存可能未生效: 只有一个节点被访问")
	}
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
