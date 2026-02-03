// Package main 展示了动态更新端点的功能
// 可以在运行时动态添加或删除服务节点
package main

import (
	"context"
	"log"
	"time"

	pb "github.com/xkeyideal/grpcbalance/examples/proto/echo"
	"github.com/xkeyideal/grpcbalance/grpclient"
	"github.com/xkeyideal/grpcbalance/grpclient/balancer"
	"github.com/xkeyideal/grpcbalance/grpclient/picker"

	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/metadata"
)

var initialAddrs = []string{
	"127.0.0.1:50051",
	"127.0.0.1:50052",
}

func main() {
	log.Println("=== 动态端点更新示例 ===")

	// 示例1: 动态添加节点
	log.Println("\n--- 示例1: 动态添加节点 ---")
	addEndpointExample()

	// 示例2: 动态删除节点
	log.Println("\n--- 示例2: 动态删除节点 ---")
	removeEndpointExample()

	// 示例3: 更新节点属性
	log.Println("\n--- 示例3: 更新节点属性 ---")
	updateAttributesExample()

	// 示例4: 完整替换端点列表
	log.Println("\n--- 示例4: 完整替换端点列表 ---")
	replaceEndpointsExample()
}

// addEndpointExample 展示动态添加节点
func addEndpointExample() {
	cfg := &grpclient.Config{
		Endpoints:   initialAddrs,
		BalanceName: balancer.RoundRobinBalanceName,

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

	log.Printf("初始端点: %v", client.Endpoints())

	cc := client.ActiveConnection()
	echoClient := pb.NewEchoClient(cc)

	// 发送几个请求
	log.Println("使用初始端点发送请求:")
	for i := 0; i < 4; i++ {
		callEcho(echoClient, "初始请求")
	}

	// 动态添加新节点
	newAddrs := append(client.Endpoints(), "127.0.0.1:50053")
	client.SetEndpoints(newAddrs)
	log.Printf("\n添加新节点后的端点: %v", client.Endpoints())

	// 使用新端点发送请求
	log.Println("添加新节点后发送请求:")
	for i := 0; i < 6; i++ {
		callEcho(echoClient, "新节点请求")
	}

	log.Println("动态添加节点后，新请求会自动分配到新节点")
}

// removeEndpointExample 展示动态删除节点
func removeEndpointExample() {
	addrs := []string{
		"127.0.0.1:50051",
		"127.0.0.1:50052",
		"127.0.0.1:50053",
	}

	cfg := &grpclient.Config{
		Endpoints:   addrs,
		BalanceName: balancer.RoundRobinBalanceName,

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

	log.Printf("初始端点: %v", client.Endpoints())

	cc := client.ActiveConnection()
	echoClient := pb.NewEchoClient(cc)

	// 发送请求到所有节点
	log.Println("使用所有节点发送请求:")
	for i := 0; i < 6; i++ {
		callEcho(echoClient, "全节点请求")
	}

	// 移除一个节点 (模拟节点下线)
	reducedAddrs := []string{
		"127.0.0.1:50051",
		"127.0.0.1:50052",
	}
	client.SetEndpoints(reducedAddrs)
	log.Printf("\n移除节点后的端点: %v", client.Endpoints())

	// 发送请求，只会分配到剩余节点
	log.Println("移除节点后发送请求:")
	for i := 0; i < 4; i++ {
		callEcho(echoClient, "减少节点后请求")
	}

	log.Println("移除节点后，新请求不会再分配到已删除的节点")
}

// updateAttributesExample 展示更新节点属性
func updateAttributesExample() {
	// 初始权重
	attrs := make(map[string]*attributes.Attributes)
	attrs["127.0.0.1:50051"] = attributes.New(picker.WeightAttributeKey, int32(1))
	attrs["127.0.0.1:50052"] = attributes.New(picker.WeightAttributeKey, int32(1))

	cfg := &grpclient.Config{
		Endpoints:   initialAddrs,
		BalanceName: balancer.WeightedRobinBalanceName,
		Attributes:  attrs,

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

	log.Println("初始权重: 50051=1, 50052=1 (1:1)")
	log.Println("使用初始权重发送请求:")
	for i := 0; i < 8; i++ {
		callEcho(echoClient, "初始权重请求")
	}

	// 更新权重 (模拟根据节点性能调整)
	newAttrs := make(map[string]*attributes.Attributes)
	newAttrs["127.0.0.1:50051"] = attributes.New(picker.WeightAttributeKey, int32(3))
	newAttrs["127.0.0.1:50052"] = attributes.New(picker.WeightAttributeKey, int32(1))

	cfg.Attributes = newAttrs
	client.SetEndpoints(initialAddrs) // 重新设置端点以应用新属性

	log.Println("\n更新权重: 50051=3, 50052=1 (3:1)")
	log.Println("使用新权重发送请求:")
	for i := 0; i < 8; i++ {
		callEcho(echoClient, "新权重请求")
	}

	log.Println("权重更新后，请求分配比例会相应调整")
}

// replaceEndpointsExample 展示完整替换端点列表
func replaceEndpointsExample() {
	cfg := &grpclient.Config{
		Endpoints:   initialAddrs,
		BalanceName: balancer.RoundRobinBalanceName,

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

	log.Printf("初始端点: %v", client.Endpoints())

	cc := client.ActiveConnection()
	echoClient := pb.NewEchoClient(cc)

	log.Println("使用初始端点发送请求:")
	for i := 0; i < 4; i++ {
		callEcho(echoClient, "初始请求")
	}

	// 完全替换为新的端点列表 (模拟集群迁移)
	newAddrs := []string{
		"192.168.1.10:50051",
		"192.168.1.11:50051",
		"192.168.1.12:50051",
	}
	client.SetEndpoints(newAddrs)
	log.Printf("\n替换后的端点: %v", client.Endpoints())

	log.Println("使用新端点发送请求:")
	for i := 0; i < 6; i++ {
		callEcho(echoClient, "新集群请求")
	}

	log.Println("\n端点完全替换场景:")
	log.Println("  - 集群迁移")
	log.Println("  - 蓝绿部署切换")
	log.Println("  - 灾难恢复切换")
}

func callEcho(c pb.EchoClient, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
