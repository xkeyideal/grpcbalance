package picker

import (
	"context"
	"sort"
	"strings"
	"sync"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/resolver"
)

// NodeFilter 是节点过滤器函数类型
// 返回 true 表示节点应该被保留，false 表示应该被过滤掉
type NodeFilter func(info SubConnInfo) bool

// nodeFilterContextKey 用于在 context 中传递节点过滤器
type nodeFilterContextKey struct{}

// WithNodeFilter 将节点过滤器添加到 context 中
func WithNodeFilter(ctx context.Context, filters ...NodeFilter) context.Context {
	existingFilters := NodeFiltersFromContext(ctx)
	return context.WithValue(ctx, nodeFilterContextKey{}, append(existingFilters, filters...))
}

// NodeFiltersFromContext 从 context 中提取节点过滤器
func NodeFiltersFromContext(ctx context.Context) []NodeFilter {
	v := ctx.Value(nodeFilterContextKey{})
	if v == nil {
		return nil
	}
	if filters, ok := v.([]NodeFilter); ok {
		return filters
	}
	return nil
}

// VersionFilterKey 用于在 Address.Attributes 中存储版本信息的 key
const VersionFilterKey = "version"

// MetadataFilterKey 用于在 Address.Attributes 中存储元数据的 key
const MetadataFilterKey = "metadata"

// VersionFilter 创建一个按版本号过滤节点的过滤器
// 只保留版本号匹配的节点
func VersionFilter(version string) NodeFilter {
	return func(info SubConnInfo) bool {
		if info.Address.Attributes == nil {
			return version == ""
		}
		v := info.Address.Attributes.Value(VersionFilterKey)
		if v == nil {
			return version == ""
		}
		if ver, ok := v.(string); ok {
			return ver == version
		}
		return false
	}
}

// VersionPrefixFilter 创建一个按版本号前缀过滤节点的过滤器
func VersionPrefixFilter(prefix string) NodeFilter {
	return func(info SubConnInfo) bool {
		if info.Address.Attributes == nil {
			return prefix == ""
		}
		v := info.Address.Attributes.Value(VersionFilterKey)
		if v == nil {
			return prefix == ""
		}
		if ver, ok := v.(string); ok {
			return strings.HasPrefix(ver, prefix)
		}
		return false
	}
}

// MetadataFilter 创建一个按元数据过滤节点的过滤器
// 只保留包含指定 key-value 对的节点
func MetadataFilter(key, value string) NodeFilter {
	return func(info SubConnInfo) bool {
		if info.Address.Attributes == nil {
			return false
		}
		v := info.Address.Attributes.Value(MetadataFilterKey)
		if v == nil {
			return false
		}
		if metadata, ok := v.(map[string]string); ok {
			return metadata[key] == value
		}
		return false
	}
}

// MetadataExistsFilter 创建一个按元数据 key 存在性过滤节点的过滤器
func MetadataExistsFilter(key string) NodeFilter {
	return func(info SubConnInfo) bool {
		if info.Address.Attributes == nil {
			return false
		}
		v := info.Address.Attributes.Value(MetadataFilterKey)
		if v == nil {
			return false
		}
		if metadata, ok := v.(map[string]string); ok {
			_, exists := metadata[key]
			return exists
		}
		return false
	}
}

// FilteredPickerBuilder 包装一个 PickerBuilder，在构建时应用节点过滤
// 注意：对于有状态的负载均衡算法（如 P2C、MinConnect、MinRespTime），
// 过滤后的子集使用独立的统计数据，与全量节点的统计不共享。
// 这在大多数场景下是可接受的，因为过滤通常用于灰度发布、版本控制等场景，
// 不同版本的节点本身就应该独立统计。
type FilteredPickerBuilder struct {
	inner PickerBuilder
}

// NewFilteredPickerBuilder 创建一个支持过滤的 PickerBuilder
func NewFilteredPickerBuilder(pb PickerBuilder) *FilteredPickerBuilder {
	return &FilteredPickerBuilder{inner: pb}
}

func (fpb *FilteredPickerBuilder) Build(info PickerBuildInfo) balancer.Picker {
	// 直接构建，过滤在 Pick 时进行
	// 同时预构建一个使用所有节点的 picker，用于无过滤器场景
	return &filteredPicker{
		innerBuilder:  fpb.inner,
		allNodes:      info.ReadySCs,
		defaultPicker: fpb.inner.Build(info),
		cachedPickers: make(map[string]balancer.Picker),
	}
}

type filteredPicker struct {
	innerBuilder  PickerBuilder
	allNodes      map[balancer.SubConn]SubConnInfo
	defaultPicker balancer.Picker // 无过滤器时使用的 picker

	mu            sync.RWMutex
	cachedPickers map[string]balancer.Picker // 缓存按过滤条件分组的 picker
}

// filterCacheKey 生成过滤后节点集合的缓存 key
// 使用节点地址排序后拼接作为 key
func filterCacheKey(nodes map[balancer.SubConn]SubConnInfo) string {
	if len(nodes) == 0 {
		return ""
	}
	addrs := make([]string, 0, len(nodes))
	for _, info := range nodes {
		addrs = append(addrs, info.Address.Addr)
	}
	// 排序以确保相同节点集合生成相同的 key
	sort.Strings(addrs)
	return strings.Join(addrs, ",")
}

func (fp *filteredPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	// 获取过滤器
	filters := NodeFiltersFromContext(info.Ctx)
	if len(filters) == 0 {
		// 没有过滤器，使用预构建的 defaultPicker
		return fp.defaultPicker.Pick(info)
	}

	// 过滤节点
	filteredNodes := make(map[balancer.SubConn]SubConnInfo)
	for sc, scInfo := range fp.allNodes {
		keep := true
		for _, filter := range filters {
			if !filter(scInfo) {
				keep = false
				break
			}
		}
		if keep {
			filteredNodes[sc] = scInfo
		}
	}

	if len(filteredNodes) == 0 {
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}

	// 如果过滤后的节点与全部节点相同，使用 defaultPicker
	if len(filteredNodes) == len(fp.allNodes) {
		return fp.defaultPicker.Pick(info)
	}

	// 生成缓存 key
	cacheKey := filterCacheKey(filteredNodes)

	// 先尝试读取缓存
	fp.mu.RLock()
	cachedPicker, ok := fp.cachedPickers[cacheKey]
	fp.mu.RUnlock()

	if ok {
		return cachedPicker.Pick(info)
	}

	// 缓存未命中，需要构建新的 picker
	fp.mu.Lock()
	// 双重检查
	if cachedPicker, ok = fp.cachedPickers[cacheKey]; ok {
		fp.mu.Unlock()
		return cachedPicker.Pick(info)
	}

	// 构建并缓存
	picker := fp.innerBuilder.Build(PickerBuildInfo{ReadySCs: filteredNodes})
	fp.cachedPickers[cacheKey] = picker
	fp.mu.Unlock()

	return picker.Pick(info)
}

// AddressFilter 根据地址列表过滤节点
func AddressFilter(addrs ...string) NodeFilter {
	addrSet := make(map[string]struct{}, len(addrs))
	for _, addr := range addrs {
		addrSet[addr] = struct{}{}
	}
	return func(info SubConnInfo) bool {
		_, ok := addrSet[info.Address.Addr]
		return ok
	}
}

// ExcludeAddressFilter 排除指定地址的节点
func ExcludeAddressFilter(addrs ...string) NodeFilter {
	addrSet := make(map[string]struct{}, len(addrs))
	for _, addr := range addrs {
		addrSet[addr] = struct{}{}
	}
	return func(info SubConnInfo) bool {
		_, ok := addrSet[info.Address.Addr]
		return !ok
	}
}

// HealthyFilter 创建一个只保留健康节点的过滤器
// healthChecker 是一个检查节点健康状态的函数
func HealthyFilter(healthChecker func(addr resolver.Address) bool) NodeFilter {
	return func(info SubConnInfo) bool {
		return healthChecker(info.Address)
	}
}
