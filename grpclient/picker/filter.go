package picker

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/xkeyideal/grpcbalance/label"

	"google.golang.org/grpc/attributes"
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
const VersionFilterKey = "_x_grpc_version_"

// MetadataFilterKey 用于在 Address.Attributes 中存储元数据的 key
const MetadataFilterKey = "_x_grpc_metadata_"

// LabelSelectorFilter creates a NodeFilter that evaluates a label selector
// against SubConn's resolver.Address.Attributes.
//
// Typical usage in grpcbalance filter:
//   - selector: "system.ip=127.0.0.1" (exact match)
//   - selector: "env in (prod,staging)" (set match)
//   - selector: "version@^1.2.0" (semver)
//   - selector: "id~=foo*bar?" (wildcard)
//
// It reads attribute values by key (string key). Supported attribute value types:
// string, []string, fmt.Stringer, and common integer types (converted to decimal string).
//
// If a key is not found directly, it will also try to read from the reserved
// metadata map stored at `MetadataFilterKey` when it is a map[string]string.
func LabelSelectorFilter(selector string) (NodeFilter, error) {
	sel, err := label.ParseSelector(selector)
	if err != nil {
		return nil, err
	}
	return func(info SubConnInfo) bool {
		attrs := info.Address.Attributes
		if attrs == nil {
			// No attributes: selector can only match if it is empty (Everything()).
			return sel.Empty()
		}
		return sel.Matches(newAttrsLabels(attrs))
	}, nil
}

type attrsLabels struct {
	attrs *attributes.Attributes
	seen  map[string]label.Set
}

func newAttrsLabels(attrs *attributes.Attributes) label.Labels {
	return &attrsLabels{attrs: attrs, seen: make(map[string]label.Set)}
}

func (a *attrsLabels) Has(key string) bool {
	_, ok := a.getSet(key)
	return ok
}

func (a *attrsLabels) Get(key string) label.Set {
	set, _ := a.getSet(key)
	return set
}

func (a *attrsLabels) Clone() label.Labels {
	// Clone seen keys only; underlying attributes are immutable.
	n := &attrsLabels{attrs: a.attrs, seen: make(map[string]label.Set, len(a.seen))}
	for k, v := range a.seen {
		n.seen[k] = copySet(v)
	}
	return n
}

func (a *attrsLabels) Merge(labels2 label.Labels) label.Labels {
	// Best-effort merge into cached view.
	if labels2 == nil {
		return a
	}
	for _, k := range labels2.Keys() {
		vs := labels2.Get(k)
		if set, ok := a.seen[k]; ok {
			set.AddSet(vs)
		} else {
			a.seen[k] = copySet(vs)
		}
	}
	return a
}

func (a *attrsLabels) Keys() []string {
	keys := make([]string, 0, len(a.seen))
	for k := range a.seen {
		keys = append(keys, k)
	}
	return keys
}

func (a *attrsLabels) ToList() []string {
	var out []string
	for k, vs := range a.seen {
		for v := range vs {
			out = append(out, k+"="+v)
		}
	}
	return out
}

func (a *attrsLabels) String() string { return a.Stringify("\r\n") }

func (a *attrsLabels) Stringify(sep string) string { return strings.Join(a.ToList(), sep) }

func (a *attrsLabels) Equal(other label.Labels) bool {
	if other == nil {
		return len(a.seen) == 0
	}
	keys := other.Keys()
	if len(keys) != len(a.seen) {
		return false
	}
	for _, k := range keys {
		if !other.Get(k).Equal(a.seen[k]) {
			return false
		}
	}
	return true
}

func (a *attrsLabels) getSet(key string) (label.Set, bool) {
	if set, ok := a.seen[key]; ok {
		return set, true
	}
	if a.attrs == nil {
		return nil, false
	}
	// 1) direct attribute value
	if v := a.attrs.Value(key); v != nil {
		set, ok := attrValueToSet(v)
		if ok {
			a.seen[key] = set
			return set, true
		}
	}
	// 2) fallback to metadata map stored at MetadataFilterKey
	if key != MetadataFilterKey {
		if mm, ok := a.attrs.Value(MetadataFilterKey).(map[string]string); ok {
			if mv, ok := mm[key]; ok {
				set := make(label.Set, 1)
				set.Add(mv)
				a.seen[key] = set
				return set, true
			}
		}
	}
	return nil, false
}

func attrValueToSet(v any) (label.Set, bool) {
	if v == nil {
		return nil, false
	}
	s := make(label.Set)
	switch vv := v.(type) {
	case string:
		s.Add(vv)
		return s, true
	case []string:
		for _, x := range vv {
			s.Add(x)
		}
		return s, true
	case fmt.Stringer:
		s.Add(vv.String())
		return s, true
	case int:
		s.Add(strconv.FormatInt(int64(vv), 10))
		return s, true
	case int32:
		s.Add(strconv.FormatInt(int64(vv), 10))
		return s, true
	case int64:
		s.Add(strconv.FormatInt(vv, 10))
		return s, true
	case uint:
		s.Add(strconv.FormatUint(uint64(vv), 10))
		return s, true
	case uint32:
		s.Add(strconv.FormatUint(uint64(vv), 10))
		return s, true
	case uint64:
		s.Add(strconv.FormatUint(vv, 10))
		return s, true
	default:
		return nil, false
	}
}

func copySet(s label.Set) label.Set {
	if s == nil {
		return nil
	}
	n := make(label.Set, len(s))
	for k := range s {
		n.Add(k)
	}
	return n
}

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
