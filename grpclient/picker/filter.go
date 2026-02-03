package picker

import (
	"context"
	"strings"

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
type FilteredPickerBuilder struct {
	inner PickerBuilder
}

// NewFilteredPickerBuilder 创建一个支持过滤的 PickerBuilder
func NewFilteredPickerBuilder(pb PickerBuilder) *FilteredPickerBuilder {
	return &FilteredPickerBuilder{inner: pb}
}

func (fpb *FilteredPickerBuilder) Build(info PickerBuildInfo) balancer.Picker {
	// 直接构建，过滤在 Pick 时进行
	return &filteredPicker{
		innerBuilder: fpb.inner,
		allNodes:     info.ReadySCs,
	}
}

type filteredPicker struct {
	innerBuilder PickerBuilder
	allNodes     map[balancer.SubConn]SubConnInfo
}

func (fp *filteredPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	// 获取过滤器
	filters := NodeFiltersFromContext(info.Ctx)
	if len(filters) == 0 {
		// 没有过滤器，使用所有节点构建 picker
		picker := fp.innerBuilder.Build(PickerBuildInfo{ReadySCs: fp.allNodes})
		return picker.Pick(info)
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

	// 使用过滤后的节点构建临时 picker 并选择
	picker := fp.innerBuilder.Build(PickerBuildInfo{ReadySCs: filteredNodes})
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
