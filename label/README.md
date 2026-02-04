# label

`label` 包提供两件事：

1. `Labels`：从原始的 `key=value` 列表解析并校验，支持同一个 key 对应多个 value。
2. `Selector` / `Selectors`：解析类 Kubernetes 的 label selector 语法，并对 `Labels` 做匹配。

它适合用在“服务发现 / 注册中心 / 端点元数据”场景：把节点的元信息放进 labels，然后用 selector 过滤节点（例如在 grpcbalance 的节点过滤里复用）。

## Labels（多值标签）

### 数据模型

- `Labels`：接口抽象，底层实现是 `map[string]Set`。
- `Set`：`map[string]struct{}` 风格集合，用来承载同一 key 的多个 value。

同一个 key 可以拥有多个值，例如：

```go
raw := []string{
		"region=cn",
		"tag=blue",
		"tag=canary",
}
lbs, _ := label.ParseLabels(raw)
// lbs.Get("tag") 里同时包含 blue、canary
```

### 解析与校验

#### ParseLabels

把 `[]string{"k=v", ...}` 解析为 `Labels` 并做校验：

```go
func ParseLabels(rawLabels []string) (Labels, error)
```

注意：每个元素必须且只能出现一次 `=`（`a=b=c` 会被拒绝）。key/value 会做格式校验。

#### ParseRawLabels

把一个字符串按 `\r\n` 分隔成多行 `k=v` 再解析：

```go
func ParseRawLabels(rawLabels string) (Labels, error)
```

该函数使用固定分隔符 `\r\n`（CRLF）。如果你的输入是 `\n`，请先自行替换/转换。

### Key/Value 允许字符

- key：必须是“qualified name”，长度 ≤ 63。
- value：长度 ≤ 63；允许字母数字，以及 `- + : _ .`（并支持常见 CJK 字符）。

## Selector（选择器）

### AND / OR 语义

- `Selector`：由多个 `Requirement` 组成，语义是 AND（用逗号 `,` 连接）。
	- 例：`"env=prod,region in (cn,us),!deprecated"`
- `Selectors`：多个 `Selector` 的集合，语义是 OR（任意一个 `Selector` 命中即为命中）。
	- 适合表达“多条规则兜底”：primary 命中不了就让 fallback 规则命中。

### 解析 API

```go
func ParseSelector(selector string) (Selector, error)

func ParseSelectors(rawSelectors []string) (Selectors, error)
```

匹配：

```go
func (s Selectors) Match(labels Labels) bool
func (s Selectors) MatchRawLabels(rawLabels []string) (bool, error)
```

### 支持的操作符

下表中，`key` 表示标签名，`value` 表示标签值：

| 语法 | 操作符 | 说明 | key 不存在时 |
|---|---|---|---|
| `key` | Exists | key 存在即可 | `false` |
| `!key` | DoesNotExist | key 必须不存在 | `true` |
| `key=value` / `key==value` | Equals / DoubleEquals | 任一值等于 value | `false` |
| `key!=value` | NotEquals | 所有值都不等于 value | `true` |
| `key in (a,b)` | In | 任一值在集合内 | `false` |
| `key notin (a,b)` | NotIn | 所有值都不在集合内 | `true` |
| `key~=pattern` | Pattern | wildcard 匹配（支持 `*` / `?`） | `false` |
| `key@constraint` | Semver | semver 约束（基于 Masterminds/semver v3） | `false` |
| `key>10` / `key<0xff` | GreaterThan / LessThan | 数值比较（按 `strconv.ParseInt(base=0)`） | `false` |

补充说明：

- `NotIn` / `NotEquals` 在 key 不存在时返回 `true`，因此它们天然带“缺省放行”的语义。
- `Pattern` / `Semver` / 数值比较要求 key 存在且值可解析，否则为 `false`。

### 语法示例

#### 基本

```go
import "github.com/xkeyideal/grpcbalance/label"

lbs, err := label.ParseLabels([]string{
		"env=prod",
		"region=cn",
		"az=cn-hz-1",
		"version=1.7.3",
		"system.ip=10.0.12.34",
		"weight=0x20",
		"tag=blue",
		"tag=canary",
})
if err != nil {
	// handle invalid labels
}

sel, err := label.ParseSelector("env=prod,region in (cn,us),!deprecated")
if err != nil {
	// handle invalid selector
}
_ = sel.Matches(lbs) // true/false
```

#### 通配符（Pattern）

```go
sel1, err := label.ParseSelector("system.ip~=10.0.*")
if err == nil {
	_ = sel1.Matches(lbs)
}
sel2, err := label.ParseSelector("az~=cn-?-1")
if err == nil {
	_ = sel2.Matches(lbs)
}
```

`pattern` 支持：

- `*`：任意长度（可为 0）
- `?`：单个字符

#### Semver

```go
sel1, err := label.ParseSelector("version@^1.7.0")
if err == nil {
	_ = sel1.Matches(lbs)
}
sel2, err := label.ParseSelector("version@>=1.6 <2.0.0")
if err == nil {
	_ = sel2.Matches(lbs)
}
```

注意：逗号 `,` 在 selector 里是 AND 分隔符，因此 `@` 后面的 semver 约束不能包含逗号。需要多个条件时，请使用空格把条件写在同一个约束字符串里（Masterminds/semver 支持）。

#### 数值比较

```go
sel1, err := label.ParseSelector("weight>10")
if err == nil {
	_ = sel1.Matches(lbs) // 0x20 > 10 => true
}
sel2, err := label.ParseSelector("weight<0b100")
if err == nil {
	_ = sel2.Matches(lbs)
}
```

数值采用 `ParseInt(base=0)`，支持十进制/十六进制(`0x`)/二进制(`0b`)/八进制(`0o`)。

### 性能建议

- selector 解析（`ParseSelector`）会做词法/语法分析与校验；推荐在初始化阶段解析一次并复用，运行时只调用 `Matches()`。
- `Selector.String()`/`Bytes()` 会输出“按 key 排序”的稳定表示，适合日志与调试。

## 与 Kubernetes labels/selector 的对比

本包的 selector 语法“看起来像” Kubernetes 的 label selector，但它面向的是“服务发现/注册中心/端点元数据过滤”场景，
因此在保持核心语义一致的同时做了一些扩展。

### 共同点

| 项 | 说明 |
|---|---|
| AND 语义 | 多个条件用逗号 `,` 连接时，整体是 AND（全部条件满足才算命中）。 |
| 基础操作符 | 支持 `key`（存在）、`!key`（不存在）、`=`/`==`、`!=`、`in (...)`、`notin (...)`。 |
| 缺失 key 的行为 | key 不存在时，`!=` 与 `notin` 返回 `true`（缺省放行语义）；`key`/`in`/`=`/`==` 返回 `false`。 |

### 不同点

| 维度 | Kubernetes | 本包（grpcbalance/label） | 影响/备注 |
|---|---|---|---|
| 标签模型 | `map[string]string`（单值） | 同一 key 支持多个 value（Set） | 影响匹配语义：例如 `key in (a,b)` 在本包里是“任一值命中即可”；`key notin (a,b)` 是“所有值都不在集合内”。 |
| 组合能力 | label selector 本身不支持 OR | 提供 `Selectors`（多个 `Selector` 的集合）表达 OR（任一命中即命中） | 可用来做 primary/fallback 规则兜底。 |
| 操作符扩展 | 不支持 | 额外支持 `key~=pattern`（通配符 `*`/`?`）、`key@constraint`（semver）、`key>n`/`key<n`（数值比较，`strconv.ParseInt(base=0)`） | 这些是本包扩展语法，不可直接拿去给 K8s 使用。 |
| key/value 校验 | K8s 有自己的 label 规则 | 与 K8s 不完全一致 | 例如：本包 key 采用“qualified name”风格校验，可能不接受某些包含 `/` 的 Kubernetes 常见 key；value 允许字符集合也有所不同。 |

### 使用建议

- 如果你需要与 Kubernetes selector 文本做互转/复用，请优先使用两者共有的“基础操作符 + AND”子集。
- 使用扩展能力（OR、Pattern、Semver、数值比较、多值）时，建议在文档/配置层明确标注“这是 grpcbalance/label 的扩展语法”。
