// Package main 展示了 grpcbalance 的熔断器功能
// 当服务端出现连续错误时，熔断器会暂时阻止对该节点的请求
package main

import (
	"context"
	"log"
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
}

func main() {
	log.Println("=== grpcbalance 熔断器使用示例 ===")

	// 示例1: 使用默认熔断器配置
	log.Println("\n--- 示例1: 默认熔断器配置 ---")
	defaultCircuitBreakerExample()

	// 示例2: 自定义熔断器配置
	log.Println("\n--- 示例2: 自定义熔断器配置 ---")
	customCircuitBreakerExample()
}

// defaultCircuitBreakerExample 展示使用默认熔断器配置
// 默认配置：连续5次失败后熔断，熔断10秒后尝试恢复
func defaultCircuitBreakerExample() {
	cfg := &grpclient.Config{
		Endpoints:         addrs,
		BalanceName:       balancer.RoundRobinBalanceName,
		EnableHealthCheck: true,

		// 启用熔断器，使用默认配置
		EnableCircuitBreaker: true,

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

	log.Println("熔断器状态说明:")
	log.Println("  - CLOSED: 正常状态，所有请求都会被处理")
	log.Println("  - OPEN: 熔断状态，请求会被快速失败")
	log.Println("  - HALF_OPEN: 半开状态，允许少量请求通过以探测服务是否恢复")

	// 发送测试请求
	for i := 0; i < 10; i++ {
		callEcho(echoClient, "熔断器测试")
		time.Sleep(500 * time.Millisecond)
	}

	log.Println("默认熔断器配置: 连续5次失败后熔断，10秒后尝试恢复")
}

// customCircuitBreakerExample 展示自定义熔断器配置
func customCircuitBreakerExample() {
	// 自定义熔断器配置
	cbConfig := &circuitbreaker.Config{
		// 连续失败多少次后触发熔断
		FailureThreshold: 3,

		// 熔断持续时间，之后进入半开状态
		OpenTimeout: 5 * time.Second,

		// 半开状态下允许的最大请求数
		HalfOpenMaxRequests: 2,

		// 成功多少次后关闭熔断器
		SuccessThreshold: 2,
	}

	cfg := &grpclient.Config{
		Endpoints:         addrs,
		BalanceName:       balancer.RoundRobinBalanceName,
		EnableHealthCheck: true,

		EnableCircuitBreaker: true,
		CircuitBreakerConfig: cbConfig,

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

	log.Println("自定义熔断器配置:")
	log.Printf("  - FailureThreshold: %d (连续失败次数阈值)", cbConfig.FailureThreshold)
	log.Printf("  - OpenTimeout: %v (熔断持续时间)", cbConfig.OpenTimeout)
	log.Printf("  - HalfOpenMaxRequests: %d (半开状态最大请求数)", cbConfig.HalfOpenMaxRequests)
	log.Printf("  - SuccessThreshold: %d (恢复所需成功次数)", cbConfig.SuccessThreshold)

	// 发送测试请求
	for i := 0; i < 10; i++ {
		callEcho(echoClient, "自定义熔断器测试")
		time.Sleep(500 * time.Millisecond)
	}
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
