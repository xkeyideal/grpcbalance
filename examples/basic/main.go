// Package main 展示了 grpcbalance 的基本使用方式
// 包括各种负载均衡算法和基础配置
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

var addrs = []string{
	"127.0.0.1:50051",
	"127.0.0.1:50052",
	"127.0.0.1:50053",
}

func main() {
	log.Println("=== grpcbalance 基本使用示例 ===")

	// 示例1: 轮询负载均衡 (Round Robin)
	log.Println("\n--- 示例1: 轮询负载均衡 ---")
	roundRobinExample()

	// 示例2: 加权轮询 (Weighted Round Robin)
	log.Println("\n--- 示例2: 加权轮询 ---")
	weightedRoundRobinExample()

	// 示例3: 随机加权轮询 (Random Weighted Round Robin)
	log.Println("\n--- 示例3: 随机加权轮询 ---")
	randomWeightedRoundRobinExample()

	// 示例4: 最少连接 (Minimum Connections)
	log.Println("\n--- 示例4: 最少连接 ---")
	minConnectExample()

	// 示例5: 最小响应时间 (Minimum Response Time)
	log.Println("\n--- 示例5: 最小响应时间 ---")
	minRespTimeExample()
}

// roundRobinExample 展示轮询负载均衡
// 每个请求依次分发到不同的服务器
func roundRobinExample() {
	cfg := &grpclient.Config{
		Endpoints:   addrs,
		BalanceName: balancer.RoundRobinBalanceName, // "round_robin_x"

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

	// 发送多次请求，观察请求分布
	cc := client.ActiveConnection()
	echoClient := pb.NewEchoClient(cc)

	for i := 0; i < 6; i++ {
		callEcho(echoClient, "轮询测试")
	}

	log.Println("轮询负载均衡：请求依次分发到每个服务器")
}

// weightedRoundRobinExample 展示加权轮询
// 根据权重分配请求，权重越高接收的请求越多
func weightedRoundRobinExample() {
	// 为每个地址设置权重
	attrs := make(map[string]*attributes.Attributes)
	weights := []int32{1, 2, 3} // 服务器权重比例 1:2:3

	for i, addr := range addrs {
		attrs[addr] = attributes.New(picker.WeightAttributeKey, weights[i])
	}

	cfg := &grpclient.Config{
		Endpoints:   addrs,
		BalanceName: balancer.WeightedRobinBalanceName, // "weighted_round_robin_x"
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

	// 发送多次请求，观察权重分布
	for i := 0; i < 12; i++ {
		callEcho(echoClient, "加权轮询测试")
	}

	log.Println("加权轮询：服务器按 1:2:3 的比例接收请求")
}

// randomWeightedRoundRobinExample 展示随机加权轮询
// 基于权重随机选择服务器
func randomWeightedRoundRobinExample() {
	attrs := make(map[string]*attributes.Attributes)
	weights := []int32{1, 2, 3}

	for i, addr := range addrs {
		attrs[addr] = attributes.New(picker.WeightAttributeKey, weights[i])
	}

	cfg := &grpclient.Config{
		Endpoints:   addrs,
		BalanceName: balancer.RandomWeightedRobinBalanceName, // "random_weighted_round_robin_x"
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

	for i := 0; i < 12; i++ {
		callEcho(echoClient, "随机加权轮询测试")
	}

	log.Println("随机加权轮询：按权重概率随机选择服务器")
}

// minConnectExample 展示最少连接负载均衡
// 将请求发送到当前连接数最少的服务器
func minConnectExample() {
	cfg := &grpclient.Config{
		Endpoints:   addrs,
		BalanceName: balancer.MinConnectBalanceName, // "min_connect_x"

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

	for i := 0; i < 6; i++ {
		callEcho(echoClient, "最少连接测试")
	}

	log.Println("最少连接：请求发送到当前连接数最少的服务器")
}

// minRespTimeExample 展示最小响应时间负载均衡
// 使用 EWMA 算法计算响应时间，选择响应最快的服务器
func minRespTimeExample() {
	cfg := &grpclient.Config{
		Endpoints:   addrs,
		BalanceName: balancer.MinRespTimeBalanceName, // "min_resp_time_x"

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

	for i := 0; i < 10; i++ {
		callEcho(echoClient, "最小响应时间测试")
	}

	log.Println("最小响应时间：使用 EWMA 计算响应时间，选择最快的服务器")
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
