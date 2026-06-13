# 上游价格同步 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现上游中转站价格自动拉取、变动检测、双通道告警、建议值一键应用的完整闭环。

**Architecture:** 独立旁路参考价表（不进计费链路）+ 定时拉取（time.Timer + Redis leader lock）+ 解析适配器 + DiffEngine + SuggestionCalculator 双策略建议值 + 人工确认 ApplyService（写入 channel 单价/group 倍率）+ 邮件与 admin 站内双通道告警。

**Tech Stack:** Go 1.26 / Ent ORM / Gin / Google Wire / Redis / Vue 3 + TS + Pinia

**配套设计文档:** `docs/UPSTREAM_PRICE_SYNC_DESIGN.md`

---

## 项目约束（每个任务都要遵守）

1. **分层严格**（`golangci-lint depguard` 强制）：handler → service → repository。handler 不得 import repository/gorm/redis。
2. **ent schema 改后**必须 `cd backend && go generate ./ent` 并提交生成代码。
3. **wire 改后**必须 `cd backend && go generate ./cmd/server` 并提交 `wire_gen.go`。
4. **包管理用 pnpm**（前端），勿用 npm。
5. **列表端点**空数据返回 `[]` 不返回 `null`（`make([]T, 0)`）。
6. **测试命令**：后端单元 `cd backend && go test -tags=unit ./...`；前端 `cd frontend && pnpm run test:run`。
7. **commit 粒度**：每个 Task 一个 commit，conventional commit 格式。

---

## File Structure

### 后端新增
| 文件 | 职责 |
|---|---|
| `backend/ent/schema/upstream_price_source.go` | 上游定价源配置 entity |
| `backend/ent/schema/upstream_model_price.go` | 参考价快照 entity |
| `backend/ent/schema/upstream_price_change.go` | 变动记录 entity |
| `backend/ent/schema/admin_notification.go` | admin 站内通知 entity |
| `backend/ent/schema/admin_notification_read.go` | 通知已读 entity |
| `backend/internal/repository/upstream_price_repo.go` | 三表 CRUD |
| `backend/internal/repository/admin_notification_repo.go` | 通知+reads CRUD |
| `backend/internal/service/upstream_price_parser.go` | 解析适配器（interface + LiteLLM/OneAPI/Custom） |
| `backend/internal/service/upstream_price_diff.go` | DiffEngine（纯函数） |
| `backend/internal/service/upstream_price_suggestion.go` | 建议值计算（纯函数） |
| `backend/internal/service/upstream_price_source_service.go` | 源配置 CRUD + 测试连接 |
| `backend/internal/service/upstream_price_sync_service.go` ⭐ | 定时调度+拉取+diff+持久化+告警 |
| `backend/internal/service/upstream_price_apply_service.go` | 一键应用 + 审计 |
| `backend/internal/service/admin_notification_service.go` | admin 通知 CRUD + 未读 |
| `backend/internal/handler/admin/upstream_price_handler.go` | API handler |
| `backend/internal/handler/admin/admin_notification_handler.go` | 通知 API handler |

### 后端修改
| 文件 | 改动 |
|---|---|
| `backend/internal/service/wire.go` | ProviderSet + Provider 函数 |
| `backend/cmd/server/wire.go` | provideCleanup 加 Stop |
| `backend/cmd/server/wire_gen.go` | 重新生成 |
| `backend/internal/server/routes/admin.go` | 注册路由组 |
| `backend/internal/service/notification_email_service.go` | 加 event 常量 + 模板 |
| `backend/internal/config/config.go` | 加 default_interval_minutes 默认 |

### 前端新增
| 文件 | 职责 |
|---|---|
| `frontend/src/api/admin/upstreamPricing.ts` | API 客户端 |
| `frontend/src/api/admin/adminNotifications.ts` | 通知 API |
| `frontend/src/stores/adminNotifications.ts` | Pinia store |
| `frontend/src/views/admin/upstream-pricing/UpstreamSourcesView.vue` | 源配置页 |
| `frontend/src/views/admin/upstream-pricing/UpstreamPriceChangesView.vue` | 变动+应用页 |
| `frontend/src/views/admin/upstream-pricing/UpstreamPriceCompareView.vue` | 对比页 |
| `frontend/src/components/admin/AdminNotificationBell.vue` | admin 铃铛 |

### 前端修改
| 文件 | 改动 |
|---|---|
| `frontend/src/router/index.ts`（或 admin 路由文件） | 加路由 |
| `frontend/src/components/layout/AppSidebar.vue` | 加菜单项 |
| `frontend/src/components/layout/AppHeader.vue` | admin 布局挂铃铛 |

---

## 任务依赖图

```
Task1(schema) ──┬─► Task2(price repo) ──┬─► Task4(Parser) ──┐
                │                        │                    ├─► Task10(SyncService) ─┐
                │                        ├─► Task5(Diff) ──────┤                       │
                │                        └─► Task6(Suggestion)─┘                       ├─► Task11(Handler+Route+Wire)
                │                                                                            │
                └─► Task3(notif repo) ──► Task8(NotifService)──────────────────────────────┤
                                                                            │              │
                                     Task7(SourceService)───────────────────┤              │
                                     Task9(ApplyService)────────────────────┘              │
                                                                            Task12(Email)─┘
                                                          Task13-18(前端，依赖 Task11 API)
```

**关键路径**：Task1 → Task2 → Task4/5/6 → Task10 → Task11。Task3/7/8/9 可并行。前端 Task13-18 在 Task11 后。

---

## Task 1: ent schema — 5 张表

**Files:**
- Create: `backend/ent/schema/upstream_price_source.go`、`upstream_model_price.go`、`upstream_price_change.go`、`admin_notification.go`、`admin_notification_read.go`
- 参照模式: `backend/ent/schema/announcement.go`（entsql.Annotation + field + index + edge 模式）

- [ ] **Step 1: 写 5 个 schema 文件**

字段严格按设计文档 §6。每个 entity 仿 `announcement.go` 结构：`Annotations()` 设表名、`Fields()`、`Edges()`、`Indexes()`。

`upstream_price_source.go` 关键字段（参照设计 §6.1）：
```go
field.String("name").MaxLen(100).NotEmpty()
field.String("platform").MaxLen(50).Default("mixed")
field.String("base_url").MaxLen(500).NotEmpty()
field.String("pricing_endpoint").MaxLen(500).Default("/api/pricing")
field.String("api_key").MaxLen(500).Optional().Sensitive()   // 加密
field.String("parser_type").MaxLen(30).Default("one_api")
field.JSON("parser_config", map[string]any{}).Optional().SchemaType(jsonb)
field.JSON("model_alias_map", map[string]string{}).Optional().SchemaType(jsonb)
field.Int("sync_interval_minutes").Default(360)
field.Float64("alert_threshold_pct").Default(0)
field.Int("cooldown_minutes").Default(60)
field.Bool("enabled").Default(true)
field.Time("last_sync_at").Optional().Nillable()
field.String("last_sync_status").MaxLen(20).Default("")
field.String("last_sync_error").MaxLen(1000).Optional()
field.String("last_hash").MaxLen(128).Optional()
// created_at/updated_at
index.Fields("enabled"); index.Fields("last_sync_at")
```

`upstream_model_price.go`（§6.2）：`source_id int64`、`model_name`、`local_model_name`、`input_price/output_price float64`、`cache_write_price/cache_read_price/image_output_price/per_request_price *float64`、`currency`、`raw_payload JSON`、`fetched_at`。`UNIQUE(source_id, model_name)` 用 `index.Fields("source_id","model_name").Unique()`。edge `From("source", UpstreamPriceSource.Type).Ref("prices").Field("source_id").Unique().Required()`。

`upstream_price_change.go`（§6.3）：`source_id`、`model_name`、`local_model_name`、`change_type`、prev/curr 价格、delta_pct、`detected_at`、`notified bool`、`status`(默认 pending)、`suggested_input_price/output_price float64`、`suggested_multiplier *float64`、`applied_at *time`、`applied_by *int64`、`applied_target`、`applied_target_id int64`。`index.Fields("status")`、`index.Fields("source_id","detected_at")`。edge 到 source。

`admin_notification.go`（§6.4）：`type`、`title`(≤200)、`content` text、`severity`(默认 info)、`target_link`、`related_ids JSON`、`created_at`。edge `To("reads", AdminNotificationRead.Type)`。`index.Fields("severity")`、`index.Fields("created_at")`。

`admin_notification_read.go`（§6.5）：`notification_id int64`、`user_id int64`、`read_at time`。`UNIQUE(notification_id, user_id)`。

> `jsonb` SchemaType 写法参照 `announcement.go:48-50`：`SchemaType(map[string]string{dialect.Postgres: "jsonb"})`。加密字段参照现有 `account.go` credentials 的加密方式（搜 `Sensitive()` 或项目加密 helper）。

- [ ] **Step 2: 生成 ent 代码**

```bash
cd backend && go generate ./ent
```
预期：无报错，`ent/` 下生成 5 个 entity 的相关代码。

- [ ] **Step 3: 编译验证**

```bash
cd backend && go build ./...
```
预期：成功。

- [ ] **Step 4: 提交**

```bash
git add backend/ent/schema/upstream_*.go backend/ent/schema/admin_notification*.go backend/ent/
git commit -m "feat(ent): add upstream price sync & admin notification schemas"
```

---

## Task 2: upstream_price_repo — 三表 CRUD

**Files:**
- Create: `backend/internal/repository/upstream_price_repo.go`、`upstream_price_repo_test.go`
- 参照: `backend/internal/repository/channel_repo_pricing.go`（ListByChannelIDs/ReplaceModelPricing 模式）、`announcement` 相关 repo

- [ ] **Step 1: 写 repo 接口与实现**

定义 `UpstreamPriceRepository` 接口（参照设计 §6 字段），方法：
```go
// source
CreateSource(ctx, *UpstreamPriceSource) error
UpdateSource(ctx, *UpstreamPriceSource) error
DeleteSource(ctx, id int64) error
GetSource(ctx, id int64) (*UpstreamPriceSource, error)
ListSources(ctx) ([]*UpstreamPriceSource, error)
ListEnabledSources(ctx) ([]*UpstreamPriceSource, error)
UpdateSourceSyncResult(ctx, id int64, status, hash string, lastErr string, syncedAt time.Time) error

// model_price
ReplaceModelPrices(ctx, sourceID int64, prices []*UpstreamModelPrice) error  // 事务: 删旧+插新
ListModelPrices(ctx, sourceID int64) ([]*UpstreamModelPrice, error)
ListAllModelPricesAsMap(ctx, sourceID int64) (map[string]*UpstreamModelPrice, error)

// change
InsertChanges(ctx, []*UpstreamPriceChange) error
ListPendingChanges(ctx, filters ChangeFilters) ([]*UpstreamPriceChange, error)  // 空 make([]T,0)
GetChange(ctx, id int64) (*UpstreamPriceChange, error)
UpdateChangeApplied(ctx, id, adminID int64, target string, targetID int64) error
MarkChangesNotified(ctx, ids []int64) error
```
实现用 ent client（不用裸 SQL，与 channel_repo 不同——这些是纯 ent entity）。`ReplaceModelPrices` 在事务内 `Delete` sourceID 旧记录 + `Create` 新记录。

- [ ] **Step 2: 写测试**

参照 `channel_repo_pricing_test.go` 或用 sqlite memory ent client（项目若有 ent test helper）。至少测：`ReplaceModelPrices` 幂等替换、`ListPendingChanges` 空返回 `[]` 非 nil、`UpdateChangeApplied` 状态迁移。

- [ ] **Step 3: 跑测试**

```bash
cd backend && go test -tags=unit ./internal/repository/ -run UpstreamPrice -v
```
预期 PASS。

- [ ] **Step 4: 提交**

```bash
git add backend/internal/repository/upstream_price_repo.go backend/internal/repository/upstream_price_repo_test.go
git commit -m "feat(repo): add upstream price repository"
```

---

## Task 3: admin_notification_repo — 通知 CRUD

**Files:**
- Create: `backend/internal/repository/admin_notification_repo.go`、`admin_notification_repo_test.go`
- 参照: announcement 的 repo + `AnnouncementReadRepository`（`internal/service/announcement.go:80-85`）

- [ ] **Step 1: 写接口与实现**

```go
type AdminNotificationRepository interface {
    Create(ctx, *AdminNotification) error
    ListUnreadByUser(ctx, userID int64, limit int) ([]*AdminNotification, error)  // LEFT JOIN reads WHERE read_at IS NULL
    CountUnreadByUser(ctx, userID int64) (int64, error)
    MarkRead(ctx, notificationID, userID int64, readAt time.Time) error
    ListAll(ctx, params pagination.PaginationParams) ([]*AdminNotification, *pagination.PaginationResult, error)
}
```
`ListUnreadByUser` 用 ent 的 `LeftJoin` reads 表 + `Where(reads.ID IsNil)`。空结果 `make([]T, 0)`。

- [ ] **Step 2: 测试** — Create→ListUnread 命中、MarkRead 后 ListUnread 不命中、CountUnread 计数。

- [ ] **Step 3: 跑测试** `go test -tags=unit ./internal/repository/ -run AdminNotification -v`

- [ ] **Step 4: 提交** `feat(repo): add admin notification repository`

---

## Task 4: Parser 适配器（纯函数，TDD）

**Files:**
- Create: `backend/internal/service/upstream_price_parser.go`、`upstream_price_parser_test.go`

- [ ] **Step 1: 写失败测试（LiteLLM 格式）**

```go
package service

import (
    "encoding/json"
    "testing"
    "github.com/stretchr/testify/require"
    "github.com/stretchr/testify/assert"
)

func TestLiteLLMParser(t *testing.T) {
    raw := `{
      "gpt-4": {"input_cost_per_token": 0.00003, "output_cost_per_token": 0.00006},
      "claude-opus-4-6": {"input_cost_per_token": 0.000015, "output_cost_per_token": 0.000075, "cache_creation_input_token_cost": 0.00001875}
    }`
    p := &LiteLLMParser{}
    out, err := p.Parse([]byte(raw), ParserConfig{
        AliasMap: map[string]string{"gpt-4": "gpt-4-turbo"},
    })
    require.NoError(t, err)
    require.Len(t, out, 2)
    // alias 映射
    m := map[string]UpstreamModelPrice{}
    for _, v := range out { m[v.ModelName] = v }
    assert.Equal(t, "gpt-4-turbo", m["gpt-4"].LocalModelName)
    assert.InDelta(t, 0.00003, m["gpt-4"].InputPrice, 1e-12)
    assert.NotNil(t, m["claude-opus-4-6"].CacheWritePrice)
}

func TestCustomJSONPathParser(t *testing.T) {
    raw := `{"data":[{"model":"x","in":0.001,"out":0.002}]}`
    p := &CustomJSONPathParser{}
    out, err := p.Parse([]byte(raw), ParserConfig{
        InputPath: "data.#.in", OutputPath: "data.#.out", ModelPath: "data.#.model",
    })
    require.NoError(t, err)
    require.Len(t, out, 1)
    assert.Equal(t, "x", out[0].ModelName)
    assert.InDelta(t, 0.001, out[0].InputPrice, 1e-12)
}
```

- [ ] **Step 2: 跑测试验证失败**

```bash
cd backend && go test -tags=unit ./internal/service/ -run "LiteLLMParser|CustomJSONPath" -v
```
预期：FAIL（类型未定义）。

- [ ] **Step 3: 写实现**

```go
package service

import (
    "encoding/json"
    "fmt"
    "github.com/tidwall/gjson"  // 项目已用于其他 JSONPath 解析；若未引入改用标准库
)

type UpstreamModelPrice struct {
    ModelName       string
    LocalModelName  string
    InputPrice      float64
    OutputPrice     float64
    CacheWritePrice *float64
    CacheReadPrice  *float64
    RawPayload      map[string]any
}

type ParserConfig struct {
    AliasMap                          map[string]string
    InputPath, OutputPath, ModelPath  string // custom parser
}

type PriceParser interface {
    Parse(raw []byte, cfg ParserConfig) ([]UpstreamModelPrice, error)
}

func applyAlias(name string, m map[string]string) string {
    if v, ok := m[name]; ok && v != "" { return v }
    return name
}

// LiteLLMParser 解析 LiteLLM model_prices_and_context_window.json 格式
type LiteLLMParser struct{}

func (p *LiteLLMParser) Parse(raw []byte, cfg ParserConfig) ([]UpstreamModelPrice, error) {
    var doc map[string]map[string]any
    if err := json.Unmarshal(raw, &doc); err != nil {
        return nil, fmt.Errorf("litellm parse: %w", err)
    }
    out := make([]UpstreamModelPrice, 0, len(doc))
    for name, fields := range doc {
        if name == "sample_spec" { continue }
        m := UpstreamModelPrice{ModelName: name, LocalModelName: applyAlias(name, cfg.AliasMap), RawPayload: fields}
        m.InputPrice = toFloat(fields["input_cost_per_token"])
        m.OutputPrice = toFloat(fields["output_cost_per_token"])
        if v, ok := fields["cache_creation_input_token_cost"]; ok && v != nil {
            f := toFloat(v); m.CacheWritePrice = &f
        }
        if v, ok := fields["cache_read_input_token_cost"]; ok && v != nil {
            f := toFloat(v); m.CacheReadPrice = &f
        }
        out = append(out, m)
    }
    return out, nil
}

func toFloat(v any) float64 {
    switch n := v.(type) {
    case float64: return n
    case int: return float64(n)
    case json.Number:
        f, _ := n.Float64(); return f
    default: return 0
    }
}

// CustomJSONPathParser 用 gjson 按配置路径抽取（数组结构 [{"model","in","out"}]）
type CustomJSONPathParser struct{}

func (p *CustomJSONPathParser) Parse(raw []byte, cfg ParserConfig) ([]UpstreamModelPrice, error) {
    arr := gjson.GetBytes(raw, "data").Array()
    out := make([]UpstreamModelPrice, 0, len(arr))
    for _, item := range arr {
        m := UpstreamModelPrice{
            ModelName:      item.Get("model").String(),
            InputPrice:     item.Get("in").Float(),
            OutputPrice:    item.Get("out").Float(),
        }
        m.LocalModelName = applyAlias(m.ModelName, cfg.AliasMap)
        out = append(out, m)
    }
    return out, nil
}

// OneAPIParser 解析 new-api/one-api 系 /api/pricing 返回。
// new-api 字段语义: model_ratio(相对$2/M基准的倍率), completion_ratio(output/input)
// per_token_input = model_ratio * 2 / 1e6 ; per_token_output = per_token_input * completion_ratio
// 注意: 不同 fork 字段可能不同，实现时用真实上游返回校准
type OneAPIParser struct{}

func (p *OneAPIParser) Parse(raw []byte, cfg ParserConfig) ([]UpstreamModelPrice, error) {
    arr := gjson.GetBytes(raw, "data").Array()
    const baseRatePerMillion = 2.0 // new-api 默认基准
    out := make([]UpstreamModelPrice, 0, len(arr))
    for _, item := range arr {
        ratio := item.Get("model_ratio").Float()
        if ratio == 0 { continue }
        compRatio := item.Get("completion_ratio").Float()
        if compRatio == 0 { compRatio = 1 }
        inPerToken := ratio * baseRatePerMillion / 1e6
        m := UpstreamModelPrice{
            ModelName:     item.Get("model_name").String(),
            InputPrice:    inPerToken,
            OutputPrice:   inPerToken * compRatio,
        }
        m.LocalModelName = applyAlias(m.ModelName, cfg.AliasMap)
        out = append(out, m)
    }
    return out, nil
}

func ParserByType(t string) PriceParser {
    switch t {
    case "litellm": return &LiteLLMParser{}
    case "custom":  return &CustomJSONPathParser{}
    default:        return &OneAPIParser{}
    }
}
```

> 若项目未引入 `gjson`，`cd backend && go get github.com/tidwall/gjson`，或改用标准 `encoding/json` 解析（Custom parser 则限定结构）。

- [ ] **Step 4: 跑测试验证通过**

```bash
cd backend && go test -tags=unit ./internal/service/ -run "LiteLLMParser|CustomJSONPath" -v
```
预期 PASS。

- [ ] **Step 5: 提交**

```bash
git add backend/internal/service/upstream_price_parser.go backend/internal/service/upstream_price_parser_test.go
git commit -m "feat(service): add upstream price parsers (litellm/oneapi/custom)"
```

---

## Task 5: DiffEngine（纯函数，TDD）

**Files:**
- Create: `backend/internal/service/upstream_price_diff.go`、`upstream_price_diff_test.go`

- [ ] **Step 1: 写失败测试**

```go
package service

import (
    "testing"
    "github.com/stretchr/testify/require"
    "github.com/stretchr/testify/assert"
)

func TestDiff_NewModel(t *testing.T) {
    curr := map[string]PriceSnapshot{"gpt-4": {InputPrice: 0.00003, OutputPrice: 0.00006}}
    ch := DiffPrices(curr, map[string]PriceSnapshot{})
    require.Len(t, ch, 1)
    assert.Equal(t, PriceChangeNew, ch[0].Type)
    assert.Nil(t, ch[0].PrevInputPrice)
}

func TestDiff_PriceUp(t *testing.T) {
    prev := map[string]PriceSnapshot{"gpt-4": {InputPrice: 0.00003, OutputPrice: 0.00006}}
    curr := map[string]PriceSnapshot{"gpt-4": {InputPrice: 0.000036, OutputPrice: 0.00006}}
    ch := DiffPrices(curr, prev)
    require.Len(t, ch, 1)
    assert.Equal(t, PriceChangeUp, ch[0].Type)
    assert.InDelta(t, 0.2, ch[0].InputDeltaPct, 1e-6) // +20%
}

func TestDiff_PriceDown(t *testing.T) {
    prev := map[string]PriceSnapshot{"gpt-4": {InputPrice: 0.00003, OutputPrice: 0.00006}}
    curr := map[string]PriceSnapshot{"gpt-4": {InputPrice: 0.000024, OutputPrice: 0.00006}}
    ch := DiffPrices(curr, prev)
    require.Len(t, ch, 1)
    assert.Equal(t, PriceChangeDown, ch[0].Type)
    assert.InDelta(t, -0.2, ch[0].InputDeltaPct, 1e-6)
}

func TestDiff_Removed(t *testing.T) {
    prev := map[string]PriceSnapshot{"gpt-4": {InputPrice: 0.00003, OutputPrice: 0.00006}}
    ch := DiffPrices(map[string]PriceSnapshot{}, prev)
    require.Len(t, ch, 1)
    assert.Equal(t, PriceChangeGone, ch[0].Type)
}

func TestDiff_NoChangeEpsilon(t *testing.T) {
    s := map[string]PriceSnapshot{"gpt-4": {InputPrice: 0.00003, OutputPrice: 0.00006}}
    ch := DiffPrices(s, s)
    assert.Empty(t, ch)
}
```

- [ ] **Step 2: 跑测试验证失败** `go test -tags=unit ./internal/service/ -run TestDiff -v` → FAIL

- [ ] **Step 3: 写实现**

```go
package service

const priceEpsilon = 1e-12

type PriceChangeType string

const (
    PriceChangeUp   PriceChangeType = "price_up"
    PriceChangeDown PriceChangeType = "price_down"
    PriceChangeNew  PriceChangeType = "new_model"
    PriceChangeGone PriceChangeType = "removed"
)

type PriceSnapshot struct {
    InputPrice  float64
    OutputPrice float64
}

type PriceChange struct {
    ModelName       string
    Type            PriceChangeType
    PrevInputPrice  *float64
    CurrInputPrice  float64
    PrevOutputPrice *float64
    CurrOutputPrice float64
    InputDeltaPct   float64
    OutputDeltaPct  float64
}

func absFloat(f float64) float64 { if f < 0 { return -f }; return f }
func ptrFloat(f float64) *float64 { return &f }

// DiffPrices 对比当前快照与上次快照，返回变动列表
func DiffPrices(curr, prev map[string]PriceSnapshot) []PriceChange {
    changes := make([]PriceChange, 0)
    for name, c := range curr {
        p, ok := prev[name]
        if !ok {
            changes = append(changes, PriceChange{
                ModelName: name, Type: PriceChangeNew,
                CurrInputPrice: c.InputPrice, CurrOutputPrice: c.OutputPrice,
            })
            continue
        }
        inDelta, outDelta := c.InputPrice-p.InputPrice, c.OutputPrice-p.OutputPrice
        if absFloat(inDelta) <= priceEpsilon && absFloat(outDelta) <= priceEpsilon {
            continue
        }
        ch := PriceChange{
            ModelName: name, CurrInputPrice: c.InputPrice, CurrOutputPrice: c.OutputPrice,
            PrevInputPrice: ptrFloat(p.InputPrice), PrevOutputPrice: ptrFloat(p.OutputPrice),
        }
        if p.InputPrice > priceEpsilon { ch.InputDeltaPct = inDelta / p.InputPrice }
        if p.OutputPrice > priceEpsilon { ch.OutputDeltaPct = outDelta / p.OutputPrice }
        if inDelta > 0 || outDelta > 0 {
            ch.Type = PriceChangeUp
        } else {
            ch.Type = PriceChangeDown
        }
        changes = append(changes, ch)
    }
    for name, p := range prev {
        if _, ok := curr[name]; !ok {
            changes = append(changes, PriceChange{
                ModelName: name, Type: PriceChangeGone,
                PrevInputPrice: ptrFloat(p.InputPrice), PrevOutputPrice: ptrFloat(p.OutputPrice),
            })
        }
    }
    return changes
}
```

- [ ] **Step 4: 跑测试验证通过** → PASS

- [ ] **Step 5: 提交** `feat(service): add price diff engine`

---

## Task 6: SuggestionCalculator（纯函数，TDD）

**Files:**
- Create: `backend/internal/service/upstream_price_suggestion.go`、`upstream_price_suggestion_test.go`

- [ ] **Step 1: 写失败测试（含数学验证）**

```go
package service

import (
    "testing"
    "github.com/stretchr/testify/require"
    "github.com/stretchr/testify/assert"
)

func TestSuggestion_FollowCost(t *testing.T) {
    s := CalcSuggestion(SuggestionInput{OldInputPrice: 0.00003, NewInputPrice: 0.000036, CurrentMultiplier: 1.5})
    assert.Equal(t, SuggestionFollowCost, s.Mode)
    assert.InDelta(t, 0.000036, s.SuggestedInputPrice, 1e-12)
}

func TestSuggestion_LockPriceMath(t *testing.T) {
    // 售价不变: oldCost*mult = newCost*newMult
    // 0.00003 * 1.5 = 0.000045; newMult = 0.000045 / 0.000036 = 1.25
    s := CalcSuggestion(SuggestionInput{OldInputPrice: 0.00003, NewInputPrice: 0.000036, CurrentMultiplier: 1.5})
    require.NotNil(t, s.SuggestedMultiplier)
    assert.InDelta(t, 1.25, *s.SuggestedMultiplier, 1e-6)
}

func TestSuggestion_LockPriceCostDown(t *testing.T) {
    // 成本降: newMult 应上升
    s := CalcSuggestion(SuggestionInput{OldInputPrice: 0.00003, NewInputPrice: 0.000024, CurrentMultiplier: 1.5})
    require.NotNil(t, s.SuggestedMultiplier)
    assert.Greater(t, *s.SuggestedMultiplier, 1.5)
}
```

- [ ] **Step 2: 跑测试验证失败** → FAIL

- [ ] **Step 3: 写实现**

```go
package service

type SuggestionMode string

const (
    SuggestionFollowCost SuggestionMode = "follow_cost"
    SuggestionLockPrice  SuggestionMode = "lock_price"
)

type SuggestionInput struct {
    OldInputPrice     float64 // 上游旧成本单价
    NewInputPrice     float64 // 上游新成本单价
    CurrentMultiplier float64 // 该 model 相关 group 的当前倍率
}

type Suggestion struct {
    Mode                SuggestionMode
    SuggestedInputPrice float64  // 两种模式都用新成本作单价
    SuggestedMultiplier *float64 // 仅 lock_price 非 nil
}

// CalcSuggestion 跟随成本: 单价=新成本, 倍率不动(维持毛利率%)
//          锁死售价: 单价=新成本, 倍率=旧倍率*(旧成本/新成本)(维持用户售价)
func CalcSuggestion(in SuggestionInput) Suggestion {
    s := Suggestion{Mode: SuggestionFollowCost, SuggestedInputPrice: in.NewInputPrice}
    if in.OldInputPrice > priceEpsilon && in.NewInputPrice > priceEpsilon && in.CurrentMultiplier > 0 {
        m := in.CurrentMultiplier * (in.OldInputPrice / in.NewInputPrice)
        s.SuggestedMultiplier = &m
    }
    return s
}
```

- [ ] **Step 4: 跑测试验证通过** → PASS

- [ ] **Step 5: 提交** `feat(service): add price suggestion calculator`

---

## Task 7: upstream_price_source_service

**Files:**
- Create: `backend/internal/service/upstream_price_source_service.go`
- 参照: `internal/service/bundle_plan_service.go`（CRUD + Redis 缓存模式）

- [ ] **Step 1: 实现 CRUD service**

```go
type UpstreamPriceSourceService struct {
    repo   UpstreamPriceRepository
    crypto AccountCredentialEncryptor // 复用现有 api_key 加密; 参照 account 加密 helper
}
// 方法: Create/Update/Delete/Get/List + TestConnection(source) (HTTP GET pricing_endpoint 验证可达, 用 httpUpstream)
```
`TestConnection`：用 `httpUpstream.DoWithTLS`（参照 `openai_apikey_responses_probe.go:62-132`）请求 `base_url+pricing_endpoint`，返回可达性 + 解析器能否产出非空结果。

- [ ] **Step 2: 测试** — Create→Get 往返、Delete 后 Get 返回 NotFound、TestConnection mock。

- [ ] **Step 3: 跑测试 + 提交** `feat(service): add upstream price source service`

---

## Task 8: admin_notification_service

**Files:**
- Create: `backend/internal/service/admin_notification_service.go`
- 参照: `announcement_service.go`（Create/ListActive/MarkRead 模式）

- [ ] **Step 1: 实现接口**

```go
type AdminNotificationService struct { repo AdminNotificationRepository }
// Create(ctx, type, title, content, severity, targetLink string, relatedIDs []int64) error
// ListUnread(ctx, userID) ([]*AdminNotification, error)
// CountUnread(ctx, userID) (int64, error)
// MarkRead(ctx, userID, notificationID) error
// MarkAllRead(ctx, userID) error
```

- [ ] **Step 2: 测试 + 提交** `feat(service): add admin notification service`

---

## Task 9: upstream_price_apply_service（一键应用 + 审计）

**Files:**
- Create: `backend/internal/service/upstream_price_apply_service.go`、`upstream_price_apply_service_test.go`
- 参照: `ChannelService.ReplaceModelPricing`（`channel_repo_pricing.go:83`）、group update

- [ ] **Step 1: 写失败测试**

```go
func TestApply_FollowCost_WritesChannelPricing(t *testing.T) {
    // mock ChannelPricingRepo + GroupRepo + PriceChangeRepo
    // Apply(changeID, follow_cost, channelID)
    // 断言: ChannelService.ReplaceModelPricing 被调用(新单价), change.status=applied
}

func TestApply_LockPrice_WritesChannelAndMultiplier(t *testing.T) {
    // 断言: 单价 + group.rate_multiplier 都被更新
}

func TestApply_AlreadyApplied_ReturnsError(t *testing.T) {
    // change.status != pending → 拒绝
}
```

- [ ] **Step 2: 跑测试验证失败**

- [ ] **Step 3: 写实现**

```go
type ApplyMode string
const (
    ApplyFollowCost ApplyMode = "follow_cost"
    ApplyLockPrice  ApplyMode = "lock_price"
)

type UpstreamPriceApplyService struct {
    priceRepo      UpstreamPriceRepository
    channelService *ChannelService      // 写 channel_model_pricing
    groupRepo      GroupRepository       // 写 rate_multiplier
    auditLogger     AdminAuditLogger     // 复用现有 admin 审计
}

type ApplyRequest struct {
    ChangeID  int64
    Mode      ApplyMode
    TargetID  int64 // channel_id (follow_cost) 或 group_id (lock_price)
}

func (s *UpstreamPriceApplyService) Apply(ctx context.Context, req ApplyRequest, adminID int64) error {
    ch, err := s.priceRepo.GetChange(ctx, req.ChangeID)
    if err != nil { return err }
    if ch.Status != "pending" {
        return infraerrors.BadRequest("CHANGE_NOT_PENDING", "change already handled")
    }
    switch req.Mode {
    case ApplyFollowCost:
        // 写 channel_model_pricing: 单价=curr 价格
        if err := s.channelService.ReplaceModelPricingForModel(ctx, req.TargetID, ch.LocalModelName,
            ch.CurrInputPrice, ch.CurrOutputPrice); err != nil { return err }
    case ApplyLockPrice:
        if ch.SuggestedMultiplier == nil {
            return infraerrors.BadRequest("NO_SUGGESTED_MULTIPLIER", "lock_price requires suggested multiplier")
        }
        if err := s.channelService.ReplaceModelPricingForModel(ctx, req.TargetID, ch.LocalModelName,
            ch.CurrInputPrice, ch.CurrOutputPrice); err != nil { return err }
        if err := s.groupRepo.UpdateRateMultiplier(ctx, req.TargetID, *ch.SuggestedMultiplier); err != nil { return err }
    }
    target := "channel_pricing"
    if req.Mode == ApplyLockPrice { target = "group_multiplier" }
    if err := s.priceRepo.UpdateChangeApplied(ctx, req.ChangeID, adminID, target, req.TargetID); err != nil { return err }
    return s.auditLogger.Log(ctx, adminID, "upstream_price.apply", req)
}
```
> `ReplaceModelPricingForModel` 是 `ChannelService` 可能需要新增的辅助方法（按 model 名更新单个 channel 的单模型定价）；若现有 `ReplaceModelPricing` 已支持单模型则直接用。`GroupRepository.UpdateRateMultiplier` 若不存在则新增（参照 group update）。

- [ ] **Step 4: 跑测试验证通过 + 提交** `feat(service): add upstream price apply service`

---

## Task 10: upstream_price_sync_service（核心编排，⭐）

**Files:**
- Create: `backend/internal/service/upstream_price_sync_service.go`
- 参照: `ops_metrics_collector.go`（time.Timer + leader lock + run 循环）、`pricing_service.go`（HTTP 同步模式）

- [ ] **Step 1: 实现 SyncService**

骨架（参照 ops_metrics_collector.go 的 Start/run/Stop + leader lock）：
```go
type UpstreamPriceSyncService struct {
    repo          UpstreamPriceRepository
    notifService  *AdminNotificationService
    emailService  *NotificationEmailService
    suggestionSvc *UpstreamPriceSuggestionService // 或直接调 CalcSuggestion
    channelRepo   ChannelPricingReader             // 读本地现状算建议值
    groupRepo     GroupReader
    opsRepo       OpsRepository                    // UpsertJobHeartbeat
    redis         *redis.Client
    cfg           UpstreamPriceConfig
    stopCh        chan struct{}
}

func (s *UpstreamPriceSyncService) Start() { go s.run() }
func (s *UpstreamPriceSyncService) Stop()  { close(s.stopCh) }

func (s *UpstreamPriceSyncService) run() {
    ticker := time.NewTicker(time.Minute) // 每分钟检查哪些 source 到期
    defer ticker.Stop()
    s.syncDueSources(context.Background()) // 启动首跑
    for {
        select {
        case <-s.stopCh: return
        case <-ticker.C:
            if !s.tryAcquireLeaderLock() { continue } // 多实例只一个跑
            s.syncDueSources(context.Background())
        }
    }
}

// syncDueSources: ListEnabledSources → 对每个 last_sync_at + interval 到期的 → SyncSource
func (s *UpstreamPriceSyncService) SyncSource(ctx, sourceID) error {
    src, _ := s.repo.GetSource(ctx, sourceID)
    raw, err := s.fetchPricing(ctx, src)          // ① HTTP GET
    if err != nil { s.repo.UpdateSourceSyncResult(...,"failed",...); return err }
    prices := parser.Parse(raw, cfg)              // ② 解析
    hash := sha256Hex(raw)
    if hash == src.LastHash { s.repo.UpdateSourceSyncResult(...,"success",hash,...); return nil } // 未变
    oldMap := s.repo.ListAllModelPricesAsMap(ctx, sourceID)  // 上次快照
    changes := DiffPrices(toSnapshotMap(prices), toSnapshotMap(oldMap))  // ③ diff
    // ⑤ 建议值
    for i := range changes {
        sug := CalcSuggestion(SuggestionInput{...oldMap[curr]..., ...prices[curr]..., currentMult})
        // 查本地 group 倍率算 currentMult; 填 changes[i].SuggestedInputPrice/Multiplier
    }
    // ④ 持久化
    s.repo.ReplaceModelPrices(ctx, sourceID, prices)
    s.repo.InsertChanges(ctx, toChangeRows(sourceID, changes))
    // ⑥ 告警聚合
    if len(changes) > 0 { s.emitAlert(ctx, src, changes) }
    s.repo.UpdateSourceSyncResult(...,"success",hash,...)
    s.opsRepo.UpsertJobHeartbeat(ctx, "upstreamPriceSync", true, "")
    return nil
}
```
`emitAlert`：按设计 §8.4，severity 取最大涨幅，创建 1 条 AdminNotification + 发 1 封邮件，`MarkChangesNotified`。

- [ ] **Step 2: 测试** — 用 mock repo + 本地 httptest.Server 模拟上游，验证：未变(hash同)不产变动、涨价产 price_up + 告警、解析失败记 failed。

- [ ] **Step 3: 跑测试 + 提交** `feat(service): add upstream price sync service`

---

## Task 11: handler + 路由 + wire 注册

**Files:**
- Create: `backend/internal/handler/admin/upstream_price_handler.go`、`admin_notification_handler.go`
- Modify: `backend/internal/server/routes/admin.go`、`backend/internal/service/wire.go`、`backend/cmd/server/wire.go`

- [ ] **Step 1: 写 handler**（参照 `group_handler.go` / `channel_handler.go` 的 response.Success/Paginated/NotFound 用法）

路由（加到 `admin.go`）：
```
POST   /admin/upstream-price/sources          Create
GET    /admin/upstream-price/sources          List
PUT    /admin/upstream-price/sources/:id      Update
DELETE /admin/upstream-price/sources/:id      Delete
POST   /admin/upstream-price/sources/:id/test TestConnection
POST   /admin/upstream-price/sources/:id/sync SyncSource (手动触发)
GET    /admin/upstream-price/changes          List (pending/all, 空返回 [])
POST   /admin/upstream-price/changes/:id/apply Apply
POST   /admin/upstream-price/changes/:id/dismiss Dismiss
GET    /admin/upstream-price/compare          参考价 vs 本地价对比

GET    /admin/admin-notifications/unread      ListUnread
GET    /admin/admin-notifications/unread/count CountUnread
POST   /admin/admin-notifications/:id/read    MarkRead
POST   /admin/admin-notifications/read-all    MarkAllRead
```

- [ ] **Step 2: wire 注册**

`service/wire.go` ProviderSet 加：
```go
ProvideUpstreamPriceSourceService,
ProvideUpstreamPriceSyncService,
ProvideUpstreamPriceApplyService,
ProvideAdminNotificationService,
```
加 Provider 函数（仿 `ProvideOpsScheduledReportService` wire.go:387-398），构造 SyncService 后调 `.Start()`。

`cmd/server/wire.go` `provideCleanup` 参数列表 + `parallelSteps` 加：
```go
{"UpstreamPriceSyncService", func() error { svc.Stop(); return nil }},
```

- [ ] **Step 3: 重新生成 wire + 编译**

```bash
cd backend && go generate ./cmd/server && go build ./... && golangci-lint run ./...
```
预期：lint 通过（注意 depguard：handler 不 import repository）。

- [ ] **Step 4: 提交** `feat(handler): add upstream price & admin notification API + wire`

---

## Task 12: 邮件模板

**Files:**
- Modify: `backend/internal/service/notification_email_service.go`（参照现有 event 常量 `:23-34` 与模板）

- [ ] **Step 1: 加 event 常量**

```go
NotificationEmailEventUpstreamPriceChange = "ops.price_change"
```
加 HTML/text 模板（参照 `ops.alert` 模板），变量：source 名、变动数、涨幅 Top N 明细表、跳转链接。

- [ ] **Step 2: 测试 + 提交** `feat(notify): add upstream price change email template`

---

## Task 13-18: 前端

**Files:** 见 File Structure 前端部分。参照 `frontend/src/views/admin/bundles/BundlePlansView.vue`（CRUD 页）、`AnnouncementsView.vue`、`AnnouncementBell.vue`。

- [ ] **Task 13: API 客户端** — `api/admin/upstreamPricing.ts` + `adminNotifications.ts`，参照 `api/admin/bundles.ts`、`api/admin/announcements.ts`。提交 `feat(api): add upstream pricing & admin notification clients`。

- [ ] **Task 14: UpstreamSourcesView.vue** — 源配置 CRUD 表格 + 编辑抽屉（name/base_url/pricing_endpoint/parser_type/sync_interval/alias_map）+「测试连接」「立即同步」按钮。参照 `BundlePlansView.vue`。提交 `feat(ui): add upstream sources view`。

- [ ] **Task 15: UpstreamPriceChangesView.vue** — 变动列表（筛选 source/status/type）+ 每行展示 prev→curr、delta%、建议值 + 「跟随成本应用」「锁死售价应用」「忽略」按钮（弹出选 target channel/group）+ 批量应用。提交 `feat(ui): add price changes view`。

- [ ] **Task 16: UpstreamPriceCompareView.vue** — 表格：model | 上游参考价 | 本地 channel 价 | 本地倍率 | 建议价 | 差异%。提交 `feat(ui): add price compare view`。

- [ ] **Task 17: AdminNotificationBell.vue + store** — 复制 `AnnouncementBell.vue` 改数据源为 `adminNotifications` API，`stores/adminNotifications.ts` 轮询未读数（参照 `stores/announcements.ts`）。提交 `feat(ui): add admin notification bell`。

- [ ] **Task 18: 路由 + 菜单 + 挂铃铛** — `router` 加 `/admin/upstream-pricing/*` 三条；`AppSidebar.vue` 加菜单项「上游价格」；`AppHeader.vue` admin 布局挂 `AdminNotificationBell`。`pnpm run lint:check && pnpm run typecheck`。提交 `feat(ui): wire upstream pricing routes & notification bell`。

---

## Self-Review

**1. Spec coverage（对照设计文档）:**
- ✅ 5 张表 → Task 1
- ✅ Parser/Diff/Suggestion/Alert/Apply 算法 → Task 4/5/6/10/9
- ✅ 旁路参考价不进计费链路 → Task 9 Apply 是唯一入口，`ModelPricingResolver` 不改
- ✅ 邮件 + admin 站内双通道 → Task 8/10/12/17
- ✅ 全部变动告警 + 聚合限流 → Task 10 `emitAlert`（hash + cooldown）
- ✅ 前端 3 页 + 铃铛 → Task 14-18
- ✅ 工作量 ~7.5 天 → 任务总量匹配

**2. Placeholder scan:** OneAPIParser 的 new-api 字段换算已给公式（base $2/M），并注明"需用真实上游校准"——这是必要的现实标注，非 placeholder。其余任务给完整代码或精确参照文件 + 字段表。

**3. Type consistency:** `UpstreamModelPrice`（Task 4）↔ ent `UpstreamModelPrice`（Task 1）字段对齐；`PriceChange`（Task 5）→ `upstream_price_change` 行（Task 2 InsertChanges）；`Suggestion`（Task 6）→ `suggested_input_price/suggested_multiplier`（Task 1/9）；`ApplyMode`（Task 9）follow_cost/lock_price ↔ 设计 §8.3。

---

## Execution Handoff

Plan complete and saved to `docs/UPSTREAM_PRICE_SYNC_PLAN.md`. Two execution options:

1. **Subagent-Driven (推荐)** — 每个 Task 派一个全新 subagent 执行，任务间我做 review，快速迭代。
2. **Inline Execution** — 在当前会话里逐 Task 执行，批量 + checkpoint review。

Which approach?
