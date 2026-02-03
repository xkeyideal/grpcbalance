package resolver

import (
	"hash/crc32"
	"math/rand"
	"sort"
	"sync"
	"time"

	"google.golang.org/grpc/resolver"
)

// SubsetConfig 子集选择配置
type SubsetConfig struct {
	// SubsetSize 子集大小，0 表示不启用子集选择
	SubsetSize int

	// ClientKey 用于标识客户端的唯一 key，相同 key 的客户端会选择相同的子集
	// 如果为空，则使用随机值
	ClientKey string
}

// SubsetSelector 子集选择器
// 当后端节点数量过多时，只选择一个固定大小的子集进行负载均衡
// 这样可以减少连接数，提高连接复用率
type SubsetSelector struct {
	mu        sync.Mutex
	config    SubsetConfig
	clientKey uint32
	rand      *rand.Rand
}

// NewSubsetSelector 创建新的子集选择器
func NewSubsetSelector(config SubsetConfig) *SubsetSelector {
	var clientKey uint32
	if config.ClientKey != "" {
		clientKey = crc32.ChecksumIEEE([]byte(config.ClientKey))
	} else {
		clientKey = rand.Uint32()
	}

	return &SubsetSelector{
		config:    config,
		clientKey: clientKey,
		rand:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Select 从地址列表中选择一个子集
// 如果子集大小为 0 或地址数量小于等于子集大小，返回原地址列表
func (s *SubsetSelector) Select(addrs []resolver.Address) []resolver.Address {
	if s.config.SubsetSize <= 0 || len(addrs) <= s.config.SubsetSize {
		return addrs
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 对地址排序，确保相同的地址列表产生相同的顺序
	sorted := make([]resolver.Address, len(addrs))
	copy(sorted, addrs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Addr < sorted[j].Addr
	})

	// 使用客户端 key 和地址列表计算起始位置
	// 这样相同的客户端会选择相同的子集
	n := len(sorted)
	subset := make([]resolver.Address, s.config.SubsetSize)

	// 计算起始索引
	startIdx := int(s.clientKey) % n

	for i := 0; i < s.config.SubsetSize; i++ {
		subset[i] = sorted[(startIdx+i)%n]
	}

	return subset
}

// Shuffle 随机打乱地址列表
func (s *SubsetSelector) Shuffle(addrs []resolver.Address) []resolver.Address {
	s.mu.Lock()
	defer s.mu.Unlock()

	shuffled := make([]resolver.Address, len(addrs))
	copy(shuffled, addrs)

	s.rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	return shuffled
}

// SelectRandom 从地址列表中随机选择子集
func (s *SubsetSelector) SelectRandom(addrs []resolver.Address) []resolver.Address {
	if s.config.SubsetSize <= 0 || len(addrs) <= s.config.SubsetSize {
		return addrs
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 随机打乱后取前 N 个
	shuffled := make([]resolver.Address, len(addrs))
	copy(shuffled, addrs)

	s.rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	return shuffled[:s.config.SubsetSize]
}

// SelectWithWeight 根据权重选择子集
// 权重从 Address.Attributes 中获取
func (s *SubsetSelector) SelectWithWeight(addrs []resolver.Address, weightKey string) []resolver.Address {
	if s.config.SubsetSize <= 0 || len(addrs) <= s.config.SubsetSize {
		return addrs
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 构建带权重的列表
	type addrWeight struct {
		addr   resolver.Address
		weight int32
	}

	weighted := make([]addrWeight, len(addrs))
	for i, addr := range addrs {
		weight := int32(1)
		if addr.Attributes != nil {
			if v := addr.Attributes.Value(weightKey); v != nil {
				if w, ok := v.(int32); ok && w > 0 {
					weight = w
				}
			}
		}
		weighted[i] = addrWeight{addr: addr, weight: weight}
	}

	// 按权重降序排序
	sort.Slice(weighted, func(i, j int) bool {
		return weighted[i].weight > weighted[j].weight
	})

	// 选择权重最高的节点
	subset := make([]resolver.Address, s.config.SubsetSize)
	for i := 0; i < s.config.SubsetSize; i++ {
		subset[i] = weighted[i].addr
	}

	return subset
}

// UpdateClientKey 更新客户端 key
func (s *SubsetSelector) UpdateClientKey(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clientKey = crc32.ChecksumIEEE([]byte(key))
}
