// Package main 展示了 P2C (Power of Two Choices) 负载均衡算法
// P2C 算法随机选择两个节点，然后选择负载更低的那个
// 这种算法在大规模集群中表现优异，被 Kratos 等框架广泛使用
package main

import (
	"context"
	"log"
	"sync"
	"time"

	pb "github.com/xkeyideal/grpcbalance/examples/proto/echo"
	"github.com/xkeyideal/grpcbalance/grpclient"
	"github.com/xkeyideal/grpcbalance/grpclient/balancer"
	"github.com/xkeyideal/grpcbalance/grpclient/circuitbreaker"

	"google.golang.org/grpc/metadata"
)

var addrs = []string{
	"127.0.0.1:50051",
	"127.0.0.1:50052",
	"127.0.0.1:50053",
	"127.0.0.1:50054",
	"127.0.0.1:50055",
}

func main() {
	log.Println("=== P2C (Power of Two Choices) 负载均衡示例 ===")

	// 示例1: 基本 P2C 负载均衡
	log.Println("\n--- 示例1: 基本 P2C 负载均衡 ---")
	basicP2CExample()

	// 示例2: P2C + 熔断器
	log.Println("\n--- 示例2: P2C + 熔断器 ---")
	p2cWithCircuitBreakerExample()

	// 示例3: 高并发场景下的 P2C
	log.Println("\n--- 示例3: 高并发场景下的 P2C ---")
	highConcurrencyP2CExample()
}

// basicP2CExample 展示基本的 P2C 负载均衡
func basicP2CExample() {
	// 首先需要注册 P2C 负载均衡器
	balancer.RegisterP2CBalance()

	cfg := &grpclient.Config{
		Endpoints:   addrs,
		BalanceName: balancer.P2CBalancerName, // "p2c_x"

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

	log.Println("P2C 算法工作原理:")
	log.Println("  1. 从所有节点中随机选择两个")
	log.Println("  2. 比较两个节点的负载 (负载 = EWMA响应时间 × (进行中请求数 + 1))")
	log.Println("  3. 选择负载更低的节点")
	log.Println("  4. 使用 forcePick 机制避免节点饥饿")

	// 发送测试请求
	for i := 0; i < 10; i++ {
		callEcho(echoClient, "P2C测试")
		time.Sleep(200 * time.Millisecond)
	}
}

// p2cWithCircuitBreakerExample 展示 P2C 结合熔断器
func p2cWithCircuitBreakerExample() {
	// 注册带熔断器的 P2C 负载均衡器
	cbConfig := circuitbreaker.Config{
		FailureThreshold:    5,
		OpenTimeout:         10 * time.Second,
		HalfOpenMaxRequests: 3,
		SuccessThreshold:    2,
	}
	balancer.RegisterP2CBalanceWithCircuitBreaker(cbConfig)

	cfg := &grpclient.Config{
		Endpoints:   addrs,
		BalanceName: balancer.P2CBalancerName,

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

	log.Println("P2C + 熔断器优势:")
	log.Println("  - P2C 自动避免高负载节点")
	log.Println("  - 熔断器隔离故障节点")
	log.Println("  - 双重保护，提高系统稳定性")

	for i := 0; i < 10; i++ {
		callEcho(echoClient, "P2C+熔断器测试")
		time.Sleep(200 * time.Millisecond)
	}
}

// highConcurrencyP2CExample 展示高并发场景下的 P2C
func highConcurrencyP2CExample() {
	balancer.RegisterP2CBalance()

	cfg := &grpclient.Config{
		Endpoints:   addrs,
		BalanceName: balancer.P2CBalancerName,

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

	log.Println("模拟高并发场景...")

	var wg sync.WaitGroup
	concurrency := 20

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				callEcho(echoClient, "并发P2C测试")
			}
		}(i)
	}

	wg.Wait()
	log.Printf("完成 %d 个并发请求，每个发送 5 次调用", concurrency)
	log.Println("P2C 在高并发场景下的优势:")
	log.Println("  - 自动感知节点的实时负载")
	log.Println("  - 避免请求集中到某个节点")
	log.Println("  - 响应时间更稳定")
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
