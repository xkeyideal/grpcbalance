// Package main 展示了子集选择功能
// 当后端节点数量过多时，可以选择一个固定大小的子集进行负载均衡
// 这样可以减少连接数，提高连接复用率
package main

import (
	"fmt"
	"log"

	"github.com/xkeyideal/grpcbalance/grpclient/resolver"

	grpcResolver "google.golang.org/grpc/resolver"
)

func main() {
	log.Println("=== 子集选择功能示例 ===")

	// 模拟大量后端节点
	addrs := generateAddresses(100)
	log.Printf("总共有 %d 个后端节点", len(addrs))

	// 示例1: 确定性子集选择
	log.Println("\n--- 示例1: 确定性子集选择 ---")
	deterministicSubsetExample(addrs)

	// 示例2: 随机子集选择
	log.Println("\n--- 示例2: 随机子集选择 ---")
	randomSubsetExample(addrs)

	// 示例3: 权重子集选择
	log.Println("\n--- 示例3: 权重子集选择 ---")
	weightedSubsetExample(addrs)

	// 示例4: 多客户端子集分布
	log.Println("\n--- 示例4: 多客户端子集分布 ---")
	multiClientSubsetExample(addrs)
}

// generateAddresses 生成模拟的地址列表
func generateAddresses(count int) []grpcResolver.Address {
	addrs := make([]grpcResolver.Address, count)
	for i := 0; i < count; i++ {
		addrs[i] = grpcResolver.Address{
			Addr: fmt.Sprintf("192.168.1.%d:5005%d", i/10, i%10),
		}
	}
	return addrs
}

// deterministicSubsetExample 展示确定性子集选择
// 相同的 ClientKey 总是选择相同的子集
func deterministicSubsetExample(addrs []grpcResolver.Address) {
	config := resolver.SubsetConfig{
		SubsetSize: 10,              // 子集大小为10
		ClientKey:  "client-app-01", // 客户端唯一标识
	}

	selector := resolver.NewSubsetSelector(config)
	subset := selector.Select(addrs)

	log.Printf("确定性子集选择 (ClientKey: %s, SubsetSize: %d)", config.ClientKey, config.SubsetSize)
	log.Printf("选择的子集 (%d 个节点):", len(subset))
	for i, addr := range subset {
		log.Printf("  %d: %s", i+1, addr.Addr)
	}

	// 验证相同 ClientKey 总是选择相同子集
	log.Println("\n验证: 相同 ClientKey 多次选择结果一致:")
	for i := 0; i < 3; i++ {
		subset2 := selector.Select(addrs)
		same := true
		for j := range subset {
			if subset[j].Addr != subset2[j].Addr {
				same = false
				break
			}
		}
		log.Printf("  第 %d 次选择: 一致=%v", i+1, same)
	}
}

// randomSubsetExample 展示随机子集选择
func randomSubsetExample(addrs []grpcResolver.Address) {
	config := resolver.SubsetConfig{
		SubsetSize: 10,
		ClientKey:  "", // 空 ClientKey 使用随机值
	}

	selector := resolver.NewSubsetSelector(config)
	subset := selector.SelectRandom(addrs)

	log.Printf("随机子集选择 (SubsetSize: %d)", config.SubsetSize)
	log.Printf("选择的子集 (%d 个节点):", len(subset))
	for i, addr := range subset {
		log.Printf("  %d: %s", i+1, addr.Addr)
	}

	// 多次随机选择，结果可能不同
	log.Println("\n随机选择多次，结果可能不同:")
	for i := 0; i < 3; i++ {
		subset2 := selector.SelectRandom(addrs)
		log.Printf("  第 %d 次: 第一个节点=%s", i+1, subset2[0].Addr)
	}
}

// weightedSubsetExample 展示权重子集选择
func weightedSubsetExample(addrs []grpcResolver.Address) {
	// 注意: SelectWithWeight 从 Address.Attributes 中获取权重
	// 这里展示基本用法，实际使用需要为地址设置 Attributes

	config := resolver.SubsetConfig{
		SubsetSize: 10,
		ClientKey:  "client-app-02",
	}

	selector := resolver.NewSubsetSelector(config)
	// SelectWithWeight 使用 attribute key 来获取权重，这里传递 "weight" key
	subset := selector.SelectWithWeight(addrs, "weight")

	log.Printf("权重子集选择 (SubsetSize: %d)", config.SubsetSize)
	log.Printf("选择的子集 (%d 个节点):", len(subset))
	for i, addr := range subset {
		log.Printf("  %d: %s", i+1, addr.Addr)
	}

	log.Println("\n权重子集选择优势:")
	log.Println("  - 优先选择高权重节点")
	log.Println("  - 适合性能差异明显的集群")
}

// multiClientSubsetExample 展示多客户端的子集分布
func multiClientSubsetExample(addrs []grpcResolver.Address) {
	log.Println("模拟多个客户端连接到同一集群:")
	log.Println("每个客户端只连接 10 个节点，但不同客户端连接不同的子集")

	clients := []string{"client-01", "client-02", "client-03", "client-04", "client-05"}
	nodeConnections := make(map[string]int)

	for _, clientKey := range clients {
		config := resolver.SubsetConfig{
			SubsetSize: 10,
			ClientKey:  clientKey,
		}

		selector := resolver.NewSubsetSelector(config)
		subset := selector.Select(addrs)

		log.Printf("\n%s 连接的节点:", clientKey)
		for _, addr := range subset {
			nodeConnections[addr.Addr]++
			log.Printf("  - %s", addr.Addr)
		}
	}

	// 统计节点被连接的次数
	log.Println("\n节点连接分布统计:")
	connectionCounts := make(map[int]int)
	for _, count := range nodeConnections {
		connectionCounts[count]++
	}
	for count, nodes := range connectionCounts {
		log.Printf("  被 %d 个客户端连接的节点数: %d", count, nodes)
	}

	log.Println("\n子集选择优势:")
	log.Println("  - 每个客户端只维护少量连接，降低资源消耗")
	log.Println("  - 不同客户端连接不同子集，负载自然分散")
	log.Println("  - 相同 ClientKey 的客户端连接相同节点，便于调试")
}
