// Package main 展示了平滑切换 Picker 的功能
// 当负载均衡器重建时，可以平滑切换而不丢失正在进行的请求
package main

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xkeyideal/grpcbalance/grpclient/picker"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"
)

func main() {
	log.Println("=== 平滑切换 Picker 示例 ===")

	// 示例1: 基本平滑切换
	log.Println("\n--- 示例1: 基本平滑切换 ---")
	basicGracefulSwitchExample()

	// 示例2: 模拟 Picker 重建场景
	log.Println("\n--- 示例2: Picker 重建场景 ---")
	pickerRebuildExample()

	// 示例3: 并发请求时的平滑切换
	log.Println("\n--- 示例3: 并发请求时的平滑切换 ---")
	concurrentGracefulSwitchExample()
}

// mockPicker 模拟一个简单的 Picker
type mockPicker struct {
	id        string
	pickCount int32
}

func (p *mockPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	atomic.AddInt32(&p.pickCount, 1)
	log.Printf("[%s] Pick called, total picks: %d", p.id, atomic.LoadInt32(&p.pickCount))

	// 模拟请求处理时间
	return balancer.PickResult{
		Done: func(doneInfo balancer.DoneInfo) {
			log.Printf("[%s] Request completed", p.id)
		},
	}, nil
}

// basicGracefulSwitchExample 展示基本的平滑切换
func basicGracefulSwitchExample() {
	// 创建平滑切换 Picker
	graceful := picker.NewGracefulSwitchPicker(5 * time.Second)

	// 创建初始 Picker
	initialPicker := &mockPicker{id: "picker-v1"}
	graceful.UpdatePicker(initialPicker, connectivity.Ready)

	log.Println("初始 Picker 设置完成")
	log.Printf("当前状态: %s", graceful.CurrentState())
	log.Printf("正在进行的请求数: %d", graceful.InflightCount())

	// 发起一个请求
	result, err := graceful.Pick(balancer.PickInfo{})
	if err != nil {
		log.Printf("Pick 失败: %v", err)
		return
	}
	log.Printf("请求发起成功，正在进行的请求数: %d", graceful.InflightCount())

	// 切换到新 Picker
	newPicker := &mockPicker{id: "picker-v2"}
	graceful.UpdatePicker(newPicker, connectivity.Ready)
	log.Println("新 Picker 已设置 (因为是 Ready 状态，立即切换)")

	// 完成之前的请求
	if result.Done != nil {
		result.Done(balancer.DoneInfo{})
	}
	log.Printf("请求完成后，正在进行的请求数: %d", graceful.InflightCount())
}

// pickerRebuildExample 展示 Picker 重建场景
func pickerRebuildExample() {
	graceful := picker.NewGracefulSwitchPicker(5 * time.Second)

	// 设置初始 Ready Picker
	picker1 := &mockPicker{id: "ready-picker"}
	graceful.UpdatePicker(picker1, connectivity.Ready)
	log.Printf("初始状态: %s", graceful.CurrentState())

	// 发起请求
	result, _ := graceful.Pick(balancer.PickInfo{})
	log.Printf("请求发起，正在进行的请求数: %d", graceful.InflightCount())

	// 切换到非 Ready 状态 (例如，所有节点都不可用)
	// 此时不会立即切换，而是等待正在进行的请求完成
	failedPicker := &mockPicker{id: "failed-picker"}
	graceful.UpdatePicker(failedPicker, connectivity.TransientFailure)
	log.Printf("尝试切换到 TransientFailure 状态")
	log.Printf("当前状态仍为: %s (因为有正在进行的请求)", graceful.CurrentState())

	// 完成请求
	if result.Done != nil {
		result.Done(balancer.DoneInfo{})
	}
	time.Sleep(100 * time.Millisecond) // 等待切换完成
	log.Printf("请求完成后状态: %s", graceful.CurrentState())

	log.Println("\n平滑切换原理:")
	log.Println("  - 当前 Picker 处于 Ready 状态时")
	log.Println("  - 新 Picker 也是 Ready 状态 -> 立即切换")
	log.Println("  - 新 Picker 不是 Ready 状态 -> 等待正在进行的请求完成后再切换")
	log.Println("  - 这样可以避免正在进行的请求失败")
}

// concurrentGracefulSwitchExample 展示并发请求时的平滑切换
func concurrentGracefulSwitchExample() {
	graceful := picker.NewGracefulSwitchPicker(10 * time.Second)

	initialPicker := &mockPicker{id: "concurrent-picker-v1"}
	graceful.UpdatePicker(initialPicker, connectivity.Ready)

	var wg sync.WaitGroup
	requestCount := 10

	log.Println("启动并发请求...")

	// 启动多个并发请求
	for i := 0; i < requestCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			result, err := graceful.Pick(balancer.PickInfo{})
			if err != nil {
				log.Printf("请求 %d Pick 失败: %v", id, err)
				return
			}

			// 模拟请求处理时间
			time.Sleep(time.Duration(100+id*50) * time.Millisecond)

			if result.Done != nil {
				result.Done(balancer.DoneInfo{})
			}
		}(i)
	}

	// 等待一段时间后切换 Picker
	time.Sleep(200 * time.Millisecond)
	log.Printf("切换前正在进行的请求数: %d", graceful.InflightCount())

	newPicker := &mockPicker{id: "concurrent-picker-v2"}
	graceful.UpdatePicker(newPicker, connectivity.Ready)
	log.Println("Picker 已切换")

	wg.Wait()
	log.Println("所有请求完成")
	log.Printf("最终正在进行的请求数: %d", graceful.InflightCount())

	log.Println("\n并发场景优势:")
	log.Println("  - 正在进行的请求不会因为 Picker 切换而失败")
	log.Println("  - 新请求自动使用新的 Picker")
	log.Println("  - Inflight 计数准确追踪请求状态")
}
