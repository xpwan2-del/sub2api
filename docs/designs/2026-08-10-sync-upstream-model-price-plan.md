# 同步上游模型价格(new-api)实现计划 — P1

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 从 new-api 上游拉取模型定价,与本平台目标渠道定价 diff,生成逐条审批单,管理员改值/批准/忽略后写入 `channel_model_pricing`。

**Architecture:** 仿 `CRSSyncService` 的按需触发三段式(fetch→diff→apply)。独立 `UpstreamPriceSyncService` + `UpstreamPricingClient`,凭证存独立配置表(不碰 Account),写入经 `ChannelService` 新增的逐条定价方法。审批单双表(批次 + 逐条 items)。

**Tech Stack:** Go 1.25 + Gin + Wire + 原生 SQL(不经 ent);前端 Vue3 + TS + TailwindCSS + 自研组件。

## Global Constraints

- 迁移文件 `backend/migrations/NNN_description.sql`,**幂等**(`IF NOT EXISTS`),从 `179` 起,文件头 `SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='10min';`,启动自动执行(`//go:embed *.sql`,`migrations.go:33`)。
- `channel_model_pricing` 是**原生 SQL 表,不经 ent**;定价列类型 `NUMERIC(20,12)`。
- new-api 基准 `base_price_per_1k = 0.002`(`QuotaPerUnit=500000`,`500000 quota = $1`),per-config 可覆盖。
- 上游 HTTP 调用必须复用 `httpclient.GetClient` + `urlvalidator`(SSRF 防护),base_url 经 `URLAllowlist.NewAPIHosts` 白名单。
- 计费口径:本平台 `渠道USD单价 × Group.rate_multiplier`;new-api `model_ratio × group_ratio × 基准`。**分组倍率层直接传(P2),模型价做一步 USD 还原**。
- 前端用 **pnpm**;UI 用 TailwindCSS + `@/components/common/*` 自研组件,无第三方 UI 库。
- 后端测试同包 `_test.go`,HTTP mock 用 `httptest`(baseURL 作函数参数传入)。

## Spec Deviations(基于代码事实,对 design §6.1 的修正)

design §6.1 原写「`Account(platform=newapi)` 做凭证载体」。核实源码后**改为:凭证直接存 `upstream_source_configs` 表,不碰 `Account`**。理由:
1. `Account.platform` 决定 gateway 转发行为(claude/openai/gemini…),new-api 非 AI 转发平台,作 platform 语义错位;
2. 避开 `account.go` 的 platform 分支与 Account 创建/调度全套;
3. 与 `CRSSyncService`(凭证即传即用)模式一致,独立 `UpstreamPriceSyncService` 不进 `provideCleanup`(按需触发,非 worker)。

副作用:`upstream_source_configs` 增 `name/base_url/api_key/dashboard_token` 字段,去掉 `account_id` 外键;凭证加密复用 `internal/payment/crypto.go`。

## File Structure

**后端:**
- `backend/internal/config/config.go` — Modify:`URLAllowlistConfig` 加 `NewAPIHosts`
- `backend/migrations/179_create_upstream_price_sync.sql` — Create:3 张表
- `backend/internal/service/upstream_price_sync.go` — Create:types + 还原/平台推断/diff 纯函数
- `backend/internal/service/upstream_pricing_client.go` — Create:`FetchPricing`
- `backend/internal/service/upstream_price_sync_service.go` — Create:`SyncNow`/`Review`/`Apply`
- `backend/internal/service/channel_service.go` — Modify:加 `ApplyUpstreamPricingEntry`
- `backend/internal/repository/upstream_price_sync_repo.go` — Create:repo(含凭证加解密)
- `backend/internal/service/wire.go` — Modify:`ProviderSet` 加 `NewUpstreamPriceSyncService`
- `backend/internal/handler/admin/upstream_price_sync_handler.go` — Create:`(h *ChannelHandler)` 的同步/审批方法
- `backend/internal/handler/admin/channel_handler.go` — Modify:struct 加 service 字段
- `backend/internal/handler/wire.go` — Modify:`NewChannelHandler` 加参数
- `backend/internal/server/routes/admin.go` — Modify:`registerChannelRoutes` 加路由

**前端:**
- `frontend/src/api/admin/upstreamPriceSync.ts` — Create
- `frontend/src/api/admin/index.ts` — Modify:注册
- `frontend/src/views/admin/UpstreamSourcesView.vue` — Create
- `frontend/src/views/admin/PriceChangeRequestsView.vue` — Create
- `frontend/src/router/index.ts` — Modify:路由

---

## Task 1: Config — `URLAllowlistConfig` 加 `NewAPIHosts`

**Files:**
- Modify: `backend/internal/config/config.go:659-667`(`URLAllowlistConfig`)
- Test: `backend/internal/config/config_test.go`(若存在;否则编译验证)

**Interfaces:**
- Produces: `URLAllowlistConfig.NewAPIHosts []string`,供 Task 6 client 校验上游 host。

- [ ] **Step 1: 加字段**

在 `URLAllowlistConfig`(config.go:659-667)`CRSHosts` 下一行加:
```go
NewAPIHosts      []string `mapstructure:"newapi_hosts"`
```

- [ ] **Step 2: 加默认值(空切片即可,不强求配置)**

在 config.go:1944 附近(与 `usage_cleanup` 默认值同区)无需新增(空即不启用白名单强制)。

- [ ] **Step 3: 验证编译**

Run: `cd backend && go build ./internal/config/...`
Expected: 编译通过,无报错。

- [ ] **Step 4: Commit**

```bash
git add backend/internal/config/config.go
git commit -m "feat(upstream-sync): add NewAPIHosts to URLAllowlistConfig"
```

---

## Task 2: 迁移 — 3 张表

**Files:**
- Create: `backend/migrations/179_create_upstream_price_sync.sql`

**Interfaces:**
- Produces: 表 `upstream_source_configs` / `upstream_price_change_requests` / `upstream_price_change_items`,供 Task 7 repo 使用。

- [ ] **Step 1: 写迁移文件**

```sql
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS upstream_source_configs (
    id                        BIGSERIAL     PRIMARY KEY,
    name                      VARCHAR(128)  NOT NULL,
    base_url                  TEXT          NOT NULL,
    api_key_encrypted         TEXT          NOT NULL DEFAULT '',
    dashboard_token_encrypted TEXT          NOT NULL DEFAULT '',
    target_channel_id         BIGINT        NOT NULL REFERENCES channels(id) ON DELETE RESTRICT,
    enabled                   BOOLEAN       NOT NULL DEFAULT TRUE,
    base_price_per_1k         NUMERIC(10,6) NOT NULL DEFAULT 0.002,
    pricing_source            VARCHAR(20)   NOT NULL DEFAULT 'auto',
    sync_model_price          BOOLEAN       NOT NULL DEFAULT TRUE,
    sync_group_ratio          BOOLEAN       NOT NULL DEFAULT FALSE,
    group_mapping             JSONB         NOT NULL DEFAULT '{}'::jsonb,
    balance_threshold_usd     NUMERIC(12,4),
    last_sync_at              TIMESTAMPTZ,
    last_pricing_version      VARCHAR(128),
    last_error                TEXT,
    created_at                TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ   NOT NULL DEFAULT now()
);
COMMENT ON TABLE upstream_source_configs IS '上游 new-api 定价同步源配置';
CREATE UNIQUE INDEX IF NOT EXISTS idx_upstream_source_configs_name ON upstream_source_configs (name);

CREATE TABLE IF NOT EXISTS upstream_price_change_requests (
    id                       BIGSERIAL    PRIMARY KEY,
    source_config_id         BIGINT       NOT NULL REFERENCES upstream_source_configs(id) ON DELETE CASCADE,
    trigger_type             VARCHAR(20)  NOT NULL,
    status                   VARCHAR(20)  NOT NULL DEFAULT 'open',
    upstream_pricing_version VARCHAR(128),
    summary                  JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_by               BIGINT,
    created_at               TIMESTAMPTZ  NOT NULL DEFAULT now(),
    closed_at                TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_upcr_source_status ON upstream_price_change_requests (source_config_id, status);
CREATE INDEX IF NOT EXISTS idx_upcr_created ON upstream_price_change_requests (created_at);

CREATE TABLE IF NOT EXISTS upstream_price_change_items (
    id                 BIGSERIAL   PRIMARY KEY,
    request_id         BIGINT      NOT NULL REFERENCES upstream_price_change_requests(id) ON DELETE CASCADE,
    kind               VARCHAR(20) NOT NULL,
    platform           VARCHAR(32),
    model_name         VARCHAR(128),
    target_channel_id  BIGINT,
    upstream_raw       JSONB,
    upstream_converted JSONB,
    local_current      JSONB,
    apply_value        JSONB,
    status             VARCHAR(20) NOT NULL DEFAULT 'pending',
    reviewer_id        BIGINT,
    review_note        TEXT,
    reviewed_at        TIMESTAMPTZ,
    applied_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_upci_request_status ON upstream_price_change_items (request_id, status);
```

> 注:列名用 `trigger_type` 而非 `trigger`(SQL 保留字)。

- [ ] **Step 2: 启动应用跑迁移验证(或本地 psql)**

Run: `cd backend && go build ./... && ./server`(开发环境启动,观察日志迁移 179 应用);或 `psql -f backend/migrations/179_create_upstream_price_sync.sql` 后 `\d upstream_source_configs` 验证。
Expected: 三表创建成功。

- [ ] **Step 3: Commit**

```bash
git add backend/migrations/179_create_upstream_price_sync.sql
git commit -m "feat(upstream-sync): add migration 179 for price sync tables"
```

---

## Task 3: domain types

**Files:**
- Create: `backend/internal/service/upstream_price_sync.go`(本任务只放 types,后续任务同文件追加纯函数)

**Interfaces:**
- Produces: 下列类型,被 Task 4-9 消费。

- [ ] **Step 1: 写 types**

```go
package service

import "time"

// PricingSource 上游定价数据源选择
type UpstreamPricingSource string

const (
	PricingSourceAuto       UpstreamPricingSource = "auto"
	PricingSourceRatioConfig UpstreamPricingSource = "ratio_config"
	PricingSourcePricing    UpstreamPricingSource = "pricing"
)

// UpstreamModelPricing 从上游拉到的单个模型原始定价
type UpstreamModelPricing struct {
	ModelName        string
	ModelRatio       float64
	CompletionRatio  float64
	CacheRatio       *float64
	CreateCacheRatio *float64
	ModelPrice       *float64 // 按次 USD;<0 或 nil = 未启用按次
	QuotaType        int      // 0=ratio, 1=按次
	EnableGroups     []string
}

// PricingSnapshot 一次拉取的快照
type PricingSnapshot struct {
	Version    string                   // pricing_version
	Models     []UpstreamModelPricing
	GroupRatio map[string]float64       // P2
	UsableGroup map[string]string       // P2
	FetchedAt  time.Time
	Source     string
}

// ConvertedPrice 还原后的 USD 定价(对应 channel_model_pricing 字段)
type ConvertedPrice struct {
	BillingMode     BillingMode // token / per_request
	InputPrice      *float64
	OutputPrice     *float64
	CacheReadPrice  *float64
	CacheWritePrice *float64
	PerRequestPrice *float64
}

// PriceChangeItemKind 审批条目类型
type PriceChangeItemKind string

const (
	ItemKindModelPrice  PriceChangeItemKind = "model_price"
	ItemKindModelAdded  PriceChangeItemKind = "model_added"
	ItemKindModelRemoved PriceChangeItemKind = "model_removed"
)

// ReviewAction 审批动作
type ReviewAction string

const (
	ReviewApply  ReviewAction = "apply"
	ReviewReject ReviewAction = "reject"
	ReviewIgnore ReviewAction = "ignore"
)

// PriceChangeRequest 审批批次
type PriceChangeRequest struct {
	ID                  int64
	SourceConfigID      int64
	TriggerType         string // manual/scheduled
	Status              string // open/partially_applied/closed/expired
	UpstreamPricingVersion string
	Summary             map[string]int
	CreatedBy           int64
	CreatedAt           time.Time
	ClosedAt            *time.Time
}

// PriceChangeItem 审批条目
type PriceChangeItem struct {
	ID                int64
	RequestID         int64
	Kind              PriceChangeItemKind
	Platform          string
	ModelName         string
	TargetChannelID   int64
	UpstreamRaw       map[string]any
	UpstreamConverted *ConvertedPrice
	LocalCurrent      *ConvertedPrice
	ApplyValue        *ConvertedPrice
	Status            string // pending/approved/rejected/ignored/applied/failed
	ReviewerID        int64
	ReviewNote        string
	ReviewedAt        *time.Time
	AppliedAt         *time.Time
	CreatedAt         time.Time
}
```

- [ ] **Step 2: 验证编译**

Run: `cd backend && go build ./internal/service/...`
Expected: 编译通过。

- [ ] **Step 3: Commit**

```bash
git add backend/internal/service/upstream_price_sync.go
git commit -m "feat(upstream-sync): add domain types"
```

---

## Task 4: 还原公式 + 平台推断(纯函数,TDD)

**Files:**
- Modify: `backend/internal/service/upstream_price_sync.go`(追加函数)
- Test: `backend/internal/service/upstream_price_sync_test.go`

**Interfaces:**
- Consumes: Task 3 的 `UpstreamModelPricing` / `ConvertedPrice`
- Produces: `ConvertPricing(m UpstreamModelPricing, basePer1k float64) ConvertedPrice`;`InferPlatform(modelName string) string`

- [ ] **Step 1: 写失败测试**

`backend/internal/service/upstream_price_sync_test.go`:
```go
package service

import (
	"math"
	"testing"
)

func floatPtr(v float64) *float64 { return &v }

func TestConvertPricing_RatioMode(t *testing.T) {
	// model_ratio=1.5, completion_ratio=2, cache_ratio=0.5, base=0.002
	m := UpstreamModelPricing{
		ModelName: "claude-test", ModelRatio: 1.5, CompletionRatio: 2, QuotaType: 0,
		CacheRatio: floatPtr(0.5), CreateCacheRatio: floatPtr(1.25),
	}
	got := ConvertPricing(m, 0.002)
	// input = 1.5 * 0.002 / 1000 = 0.000003
	if math.Abs(*got.InputPrice-0.000003) > 1e-12 {
		t.Fatalf("input = %v, want 0.000003", *got.InputPrice)
	}
	// output = input * 2
	if math.Abs(*got.OutputPrice-0.000006) > 1e-12 {
		t.Fatalf("output = %v, want 0.000006", *got.OutputPrice)
	}
	// cache_read = input * 0.5
	if math.Abs(*got.CacheReadPrice-0.0000015) > 1e-12 {
		t.Fatalf("cache_read = %v, want 0.0000015", *got.CacheReadPrice)
	}
	if got.BillingMode != BillingModeToken {
		t.Fatalf("mode = %v, want token", got.BillingMode)
	}
}

func TestConvertPricing_PerRequestMode(t *testing.T) {
	m := UpstreamModelPricing{ModelName: "x", QuotaType: 1, ModelPrice: floatPtr(0.05)}
	got := ConvertPricing(m, 0.002)
	if got.PerRequestPrice == nil || math.Abs(*got.PerRequestPrice-0.05) > 1e-12 {
		t.Fatalf("per_request = %v, want 0.05", got.PerRequestPrice)
	}
	if got.BillingMode != BillingModePerRequest {
		t.Fatalf("mode = %v, want per_request", got.BillingMode)
	}
}

func TestConvertPricing_NilCache(t *testing.T) {
	m := UpstreamModelPricing{ModelName: "x", ModelRatio: 1, CompletionRatio: 1, QuotaType: 0}
	got := ConvertPricing(m, 0.002)
	if got.CacheReadPrice != nil || got.CacheWritePrice != nil {
		t.Fatalf("cache should be nil when upstream ratio nil")
	}
}

func TestInferPlatform(t *testing.T) {
	cases := map[string]string{
		"gpt-4o": PlatformOpenAI, "chatgpt-4o-latest": PlatformOpenAI, "o3-mini": PlatformOpenAI,
		"claude-sonnet-4": PlatformAnthropic,
		"gemini-2.0-flash": PlatformGemini,
		"grok-2": PlatformGrok,
		"unknown-model": "",
	}
	for name, want := range cases {
		if got := InferPlatform(name); got != want {
			t.Errorf("InferPlatform(%q) = %q, want %q", name, got, want)
		}
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/service/ -run 'TestConvertPricing|TestInferPlatform' -v`
Expected: FAIL(函数未定义)。

- [ ] **Step 3: 实现**

追加到 `upstream_price_sync.go`:
```go
import "strings"

// ConvertPricing 把上游倍率还原成 USD 绝对单价。
func ConvertPricing(m UpstreamModelPricing, basePer1k float64) ConvertedPrice {
	if m.QuotaType == 1 && m.ModelPrice != nil && *m.ModelPrice >= 0 {
		p := *m.ModelPrice
		return ConvertedPrice{BillingMode: BillingModePerRequest, PerRequestPrice: &p}
	}
	input := m.ModelRatio * basePer1k / 1000
	out := ConvertedPrice{BillingMode: BillingModeToken, InputPrice: &input}
	if m.CompletionRatio != 0 {
		o := input * m.CompletionRatio
		out.OutputPrice = &o
	}
	if m.CacheRatio != nil {
		c := input * *m.CacheRatio
		out.CacheReadPrice = &c
	}
	if m.CreateCacheRatio != nil {
		cw := input * *m.CreateCacheRatio
		out.CacheWritePrice = &cw
	}
	return out
}

// InferPlatform 按模型名前缀推断平台;未识别返回空串。
func InferPlatform(modelName string) string {
	name := strings.ToLower(modelName)
	switch {
	case strings.HasPrefix(name, "gpt-") || strings.HasPrefix(name, "chatgpt-") ||
		strings.HasPrefix(name, "o1-") || strings.HasPrefix(name, "o3-") || strings.HasPrefix(name, "text-"):
		return PlatformOpenAI
	case strings.HasPrefix(name, "claude-"):
		return PlatformAnthropic
	case strings.HasPrefix(name, "gemini-"):
		return PlatformGemini
	case strings.HasPrefix(name, "grok-"):
		return PlatformGrok
	}
	return ""
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test ./internal/service/ -run 'TestConvertPricing|TestInferPlatform' -v`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/upstream_price_sync.go backend/internal/service/upstream_price_sync_test.go
git commit -m "feat(upstream-sync): ratio->USD convert + platform infer"
```

---

## Task 5: diff 算法(纯函数,TDD)

**Files:**
- Modify: `backend/internal/service/upstream_price_sync.go`(追加)
- Test: `backend/internal/service/upstream_price_sync_test.go`(追加)

**Interfaces:**
- Consumes: Task 3/4 的 types + `ChannelModelPricing`(channel.go:86)
- Produces: `DiffPricing(upstream map[string]ConvertedPrice, upstreamPlatforms map[string]string, local []ChannelModelPricing, channelID int64) []PriceChangeItemDraft`(draft 即待落库 item 的数据,不含 DB 字段)

- [ ] **Step 1: 写失败测试**

追加到 test 文件:
```go
func TestDiffPricing_PriceChange(t *testing.T) {
	up := map[string]ConvertedPrice{"claude-x": {BillingMode: BillingModeToken, InputPrice: floatPtr(3e-6), OutputPrice: floatPtr(6e-6)}}
	plat := map[string]string{"claude-x": PlatformAnthropic}
	local := []ChannelModelPricing{{ID: 10, ChannelID: 1, Platform: PlatformAnthropic, Models: []string{"claude-x"},
		BillingMode: BillingModeToken, InputPrice: floatPtr(2e-6), OutputPrice: floatPtr(6e-6)}}
	drafts := DiffPricing(up, plat, local, 1)
	if len(drafts) != 1 {
		t.Fatalf("got %d drafts, want 1", len(drafts))
	}
	if drafts[0].Kind != ItemKindModelPrice {
		t.Fatalf("kind = %v, want model_price", drafts[0].Kind)
	}
}

func TestDiffPricing_AddedAndRemoved(t *testing.T) {
	up := map[string]ConvertedPrice{"new-model": {BillingMode: BillingModeToken, InputPrice: floatPtr(1e-6)}}
	plat := map[string]string{"new-model": PlatformOpenAI}
	local := []ChannelModelPricing{{ID: 9, ChannelID: 1, Platform: PlatformOpenAI, Models: []string{"gone-model"},
		BillingMode: BillingModeToken, InputPrice: floatPtr(1e-6)}}
	drafts := DiffPricing(up, plat, local, 1)
	kinds := map[string]bool{}
	for _, d := range drafts {
		kinds[string(d.Kind)] = true
	}
	if !kinds[string(ItemKindModelAdded)] || !kinds[string(ItemKindModelRemoved)] {
		t.Fatalf("expected model_added + model_removed, got %v", kinds)
	}
}

func TestDiffPricing_NoChange(t *testing.T) {
	up := map[string]ConvertedPrice{"m": {BillingMode: BillingModeToken, InputPrice: floatPtr(1e-6), OutputPrice: floatPtr(2e-6)}}
	plat := map[string]string{"m": PlatformOpenAI}
	local := []ChannelModelPricing{{ID: 1, ChannelID: 1, Platform: PlatformOpenAI, Models: []string{"m"},
		BillingMode: BillingModeToken, InputPrice: floatPtr(1e-6), OutputPrice: floatPtr(2e-6)}}
	if got := DiffPricing(up, plat, local, 1); len(got) != 0 {
		t.Fatalf("expected no drafts, got %d", len(got))
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/service/ -run TestDiffPricing -v`
Expected: FAIL(`DiffPricing` 未定义)。

- [ ] **Step 3: 实现**

追加到 `upstream_price_sync.go`:
```go
const priceTolerance = 1e-9

// PriceChangeItemDraft diff 产出的待落库草稿
type PriceChangeItemDraft struct {
	Kind        PriceChangeItemKind
	Platform    string
	ModelName   string
	UpstreamRaw map[string]any
	Upstream    *ConvertedPrice
	Local       *ConvertedPrice
}

// DiffPricing 对比上游快照(已还原)与本地渠道现有定价。
// 以 (platform, model_name) 为键;local 一条 ChannelModelPricing 可能含多模型,展开。
func DiffPricing(upstream map[string]ConvertedPrice, upstreamPlatforms map[string]string, local []ChannelModelPricing, channelID int64) []PriceChangeItemDraft {
	type key struct{ platform, model string }
	localIdx := map[key]*ChannelModelPricing{}
	for i := range local {
		p := &local[i]
		for _, m := range p.Models {
			localIdx[key{p.Platform, m}] = p
		}
	}
	seen := map[key]bool{}
	var drafts []PriceChangeItemDraft
	for name, up := range upstream {
		plat := upstreamPlatforms[name]
		k := key{plat, name}
		seen[k] = true
		lp := localIdx[k]
		if lp == nil {
			upCopy := up
			drafts = append(drafts, PriceChangeItemDraft{Kind: ItemKindModelAdded, Platform: plat, ModelName: name, Upstream: &upCopy})
			continue
		}
		localPrice := channelPricingToConverted(lp)
		if !convertedEqual(up, localPrice) {
			upCopy := up
			drafts = append(drafts, PriceChangeItemDraft{Kind: ItemKindModelPrice, Platform: plat, ModelName: name, Upstream: &upCopy, Local: localPrice})
		}
	}
	// 本地有、上游无
	for k, lp := range localIdx {
		if !seen[k] {
			localPrice := channelPricingToConverted(lp)
			drafts = append(drafts, PriceChangeItemDraft{Kind: ItemKindModelRemoved, Platform: k.platform, ModelName: k.model, Local: localPrice})
		}
	}
	return drafts
}

func channelPricingToConverted(p *ChannelModelPricing) *ConvertedPrice {
	return &ConvertedPrice{
		BillingMode:     p.BillingMode,
		InputPrice:      p.InputPrice,
		OutputPrice:     p.OutputPrice,
		CacheReadPrice:  p.CacheReadPrice,
		CacheWritePrice: p.CacheWritePrice,
		PerRequestPrice: p.PerRequestPrice,
	}
}

func convertedEqual(a, b *ConvertedPrice) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.BillingMode == b.BillingMode &&
		floatEq(a.InputPrice, b.InputPrice) && floatEq(a.OutputPrice, b.OutputPrice) &&
		floatEq(a.CacheReadPrice, b.CacheReadPrice) && floatEq(a.CacheWritePrice, b.CacheWritePrice) &&
		floatEq(a.PerRequestPrice, b.PerRequestPrice)
}

func floatEq(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return math.Abs(*a-*b) <= priceTolerance
}
```
(在 import 块补 `"math"`。)

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test ./internal/service/ -run TestDiffPricing -v`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/upstream_price_sync.go backend/internal/service/upstream_price_sync_test.go
git commit -m "feat(upstream-sync): diff algorithm with tolerance"
```

---

## Task 6: `UpstreamPricingClient`(TDD,httptest mock)

**Files:**
- Create: `backend/internal/service/upstream_pricing_client.go`
- Test: `backend/internal/service/upstream_pricing_client_test.go`

**Interfaces:**
- Consumes: Task 3 `PricingSnapshot`/`UpstreamModelPricing`;`httpclient.GetClient`(`internal/pkg/httpclient/pool.go:65`);`cfg.Security.URLAllowlist`
- Produces: `UpstreamPricingClient` + `FetchPricing(ctx, baseURL, source) (*PricingSnapshot, error)`,供 Task 9 使用

- [ ] **Step 1: 写失败测试(mock /api/ratio_config 与 /api/pricing)**

```go
package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient() *UpstreamPricingClient {
	return &UpstreamPricingClient{httpOpts: testHTTPOpts()}
}

func TestFetchPricing_RatioConfig(t *testing.T) {
	body := `{"success":true,"data":{"ModelRatio":{"claude-x":1.5},"CompletionRatio":{"claude-x":2},"CacheRatio":{"claude-x":0.5},"CreateCacheRatio":{"claude-x":1.25},"ModelPrice":{},"GroupRatio":{"default":1}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ratio_config" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	snap, err := newTestClient().FetchPricing(context.Background(), srv.URL, PricingSourceRatioConfig)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(snap.Models) != 1 || snap.Models[0].ModelName != "claude-x" {
		t.Fatalf("unexpected models: %+v", snap.Models)
	}
	if snap.Models[0].ModelRatio != 1.5 {
		t.Fatalf("ratio = %v", snap.Models[0].ModelRatio)
	}
}

func TestFetchPricing_AutoFallback(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/api/ratio_config" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		// /api/pricing
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"pricing_version":"v9","data":[{"model_name":"gpt-4o","model_ratio":2.5,"completion_ratio":4,"quota_type":0,"enable_groups":["default"]}],"group_ratio":{"default":1}}`))
	}))
	defer srv.Close()
	snap, err := newTestClient().FetchPricing(context.Background(), srv.URL, PricingSourceAuto)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected fallback, calls=%d", calls)
	}
	if snap.Source != "pricing" || snap.Models[0].ModelName != "gpt-4o" {
		t.Fatalf("unexpected snap: %+v", snap)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/service/ -run TestFetchPricing -v`
Expected: FAIL(类型/函数未定义)。

- [ ] **Step 3: 实现**

`upstream_pricing_client.go`:
```go
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

type UpstreamPricingClient struct {
	httpOpts httpclient.Options
}

func NewUpstreamPricingClient(cfg *config.Config) *UpstreamPricingClient {
	allow := cfg.Security.URLAllowlist
	return &UpstreamPricingClient{httpOpts: httpclient.Options{
		Timeout:            20 * time.Second,
		ValidateResolvedIP: allow.Enabled,
		AllowPrivateHosts:  allow.AllowPrivateHosts,
	}}
}

type ratioConfigResp struct {
	Success bool `json:"success"`
	Data    struct {
		ModelRatio       map[string]float64  `json:"ModelRatio"`
		CompletionRatio  map[string]float64  `json:"CompletionRatio"`
		CacheRatio       map[string]float64  `json:"CacheRatio"`
		CreateCacheRatio map[string]float64  `json:"CreateCacheRatio"`
		ModelPrice       map[string]float64  `json:"ModelPrice"`
		GroupRatio       map[string]float64  `json:"GroupRatio"`
	} `json:"data"`
}

type pricingResp struct {
	Success        bool `json:"success"`
	PricingVersion string `json:"pricing_version"`
	Data           []struct {
		ModelName        string   `json:"model_name"`
		ModelRatio       float64  `json:"model_ratio"`
		CompletionRatio  float64  `json:"completion_ratio"`
		CacheRatio       *float64 `json:"cache_ratio"`
		CreateCacheRatio *float64 `json:"create_cache_ratio"`
		ModelPrice       *float64 `json:"model_price"`
		QuotaType        int      `json:"quota_type"`
		EnableGroups     []string `json:"enable_groups"`
	} `json:"data"`
	GroupRatio map[string]float64 `json:"group_ratio"`
}

func (c *UpstreamPricingClient) FetchPricing(ctx context.Context, baseURL string, source UpstreamPricingSource) (*PricingSnapshot, error) {
	client, err := httpclient.GetClient(c.httpOpts)
	if err != nil {
		return nil, fmt.Errorf("create http client: %w", err)
	}
	if source == PricingSourceRatioConfig || source == PricingSourceAuto {
		snap, err := c.fetchRatioConfig(ctx, client, baseURL)
		if err == nil {
			return snap, nil
		}
		if source == PricingSourceRatioConfig {
			return nil, err
		}
		// auto: 回退 pricing
	}
	return c.fetchPricing(ctx, client, baseURL)
}

func (c *UpstreamPricingClient) fetchRatioConfig(ctx context.Context, client *http.Client, baseURL string) (*PricingSnapshot, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/ratio_config", nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ratio_config status %d", resp.StatusCode)
	}
	var r ratioConfigResp
	if err := json.Unmarshal(body, &r); err != nil || !r.Success {
		return nil, fmt.Errorf("ratio_config parse failed")
	}
	snap := &PricingSnapshot{Source: "ratio_config", GroupRatio: r.Data.GroupRatio, FetchedAt: time.Now()}
	for name, ratio := range r.Data.ModelRatio {
		m := UpstreamModelPricing{ModelName: name, ModelRatio: ratio, QuotaType: 0}
		m.CompletionRatio = r.Data.CompletionRatio[name]
		if v, ok := r.Data.CacheRatio[name]; ok {
			m.CacheRatio = &v
		}
		if v, ok := r.Data.CreateCacheRatio[name]; ok {
			m.CreateCacheRatio = &v
		}
		if p, ok := r.Data.ModelPrice[name]; ok && p >= 0 {
			m.ModelPrice = &p
		}
		snap.Models = append(snap.Models, m)
	}
	return snap, nil
}

func (c *UpstreamPricingClient) fetchPricing(ctx context.Context, client *http.Client, baseURL string) (*PricingSnapshot, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/pricing", nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pricing status %d", resp.StatusCode)
	}
	var r pricingResp
	if err := json.Unmarshal(body, &r); err != nil || !r.Success {
		return nil, fmt.Errorf("pricing parse failed")
	}
	snap := &PricingSnapshot{Source: "pricing", Version: r.PricingVersion, GroupRatio: r.GroupRatio, FetchedAt: time.Now()}
	for _, d := range r.Data {
		snap.Models = append(snap.Models, UpstreamModelPricing{
			ModelName: d.ModelName, ModelRatio: d.ModelRatio, CompletionRatio: d.CompletionRatio,
			CacheRatio: d.CacheRatio, CreateCacheRatio: d.CreateCacheRatio, ModelPrice: d.ModelPrice,
			QuotaType: d.QuotaType, EnableGroups: d.EnableGroups,
		})
	}
	return snap, nil
}
```
> 测试辅助 `testHTTPOpts()` 放 test 文件:`func testHTTPOpts() httpclient.Options { return httpclient.Options{Timeout: 5 * time.Second, AllowPrivateHosts: true} }`。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test ./internal/service/ -run TestFetchPricing -v`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/upstream_pricing_client.go backend/internal/service/upstream_pricing_client_test.go
git commit -m "feat(upstream-sync): upstream pricing client with auto fallback"
```

---

## Task 7: repository(config + request/items CRUD,含凭证加解密)

**Files:**
- Create: `backend/internal/repository/upstream_price_sync_repo.go`
- Modify: `backend/internal/service/upstream_price_sync.go` — 追加 `UpstreamPriceSyncRepository` 接口 + `UpstreamSourceConfig` struct

**Interfaces:**
- Consumes: Task 2 表;`payment/crypto.go:24,67`(`Encrypt/Decrypt`)
- Produces: `UpstreamPriceSyncRepository`(service 包接口)+ `NewUpstreamPriceSyncRepository(db *sql.DB, encKey []byte) service.UpstreamPriceSyncRepository`

- [ ] **Step 1: 在 service 包追加 struct + 接口**

追加到 `upstream_price_sync.go`:
```go
import "encoding/json"

type UpstreamSourceConfig struct {
	ID                 int64
	Name               string
	BaseURL            string
	APIKey             string // 内存明文;落库加密
	DashboardToken     string // 内存明文;落库加密(P2 余额用)
	TargetChannelID    int64
	Enabled            bool
	BasePricePer1k     float64
	PricingSource      UpstreamPricingSource
	SyncModelPrice     bool
	SyncGroupRatio     bool
	GroupMapping       map[string]int64
	BalanceThresholdUSD *float64
	LastSyncAt         *time.Time
	LastPricingVersion string
	LastError          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type RequestFilter struct {
	SourceConfigID *int64
	Status         string
	Page           int
	PageSize       int
}

type UpstreamPriceSyncRepository interface {
	CreateConfig(ctx context.Context, c *UpstreamSourceConfig) error
	GetConfig(ctx context.Context, id int64) (*UpstreamSourceConfig, error)
	GetConfigByName(ctx context.Context, name string) (*UpstreamSourceConfig, error)
	ListConfigs(ctx context.Context) ([]UpstreamSourceConfig, error)
	UpdateConfig(ctx context.Context, c *UpstreamSourceConfig) error
	DeleteConfig(ctx context.Context, id int64) error
	UpdateConfigSyncState(ctx context.Context, id int64, lastSyncAt time.Time, version, lastErr string) error

	CreateRequest(ctx context.Context, req *PriceChangeRequest, items []PriceChangeItem) error
	GetRequest(ctx context.Context, id int64) (*PriceChangeRequest, error)
	ListRequests(ctx context.Context, f RequestFilter) ([]PriceChangeRequest, int64, error)
	ListItems(ctx context.Context, requestID int64) ([]PriceChangeItem, error)
	GetItem(ctx context.Context, id int64) (*PriceChangeItem, error)
	UpdateItemStatus(ctx context.Context, id int64, status string, reviewerID int64, note string, appliedAt *time.Time) error
	ExpireOpenRequests(ctx context.Context, configID int64) (int, error)
}

// MarshalConverted / UnmarshalConverted — JSONB 落库辅助
func MarshalConverted(c *ConvertedPrice) ([]byte, error) { return json.Marshal(c) }
func UnmarshalConverted(b []byte) (*ConvertedPrice, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var c ConvertedPrice
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
```

- [ ] **Step 2: 实现仓库(关键方法给出,CRUD 同模式)**

`upstream_price_sync_repo.go`(构造 + 凭证加解密 + CreateRequest 事务 + 其余 CRUD)。**加密 key 来源**:实现者先 `grep -rn "crypto.Encrypt(" backend/` 确认项目加密 key 的 config 字段(如 `cfg.Security.EncryptionKey`),Wire 注入到 `encKey`。
```go
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type upstreamPriceSyncRepo struct {
	db     *sql.DB
	encKey []byte
}

func NewUpstreamPriceSyncRepository(db *sql.DB, encKey []byte) service.UpstreamPriceSyncRepository {
	return &upstreamPriceSyncRepo{db: db, encKey: encKey}
}

func (r *upstreamPriceSyncRepo) encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	return payment.Encrypt(plain, r.encKey)
}
func (r *upstreamPriceSyncRepo) decrypt(cipher string) (string, error) {
	if cipher == "" {
		return "", nil
	}
	return payment.Decrypt(cipher, r.encKey)
}

func (r *upstreamPriceSyncRepo) CreateConfig(ctx context.Context, c *service.UpstreamSourceConfig) error {
	ak, _ := r.encrypt(c.APIKey)
	dt, _ := r.encrypt(c.DashboardToken)
	gm, _ := json.Marshal(c.GroupMapping)
	return r.db.QueryRowContext(ctx, `
INSERT INTO upstream_source_configs
(name, base_url, api_key_encrypted, dashboard_token_encrypted, target_channel_id, enabled,
 base_price_per_1k, pricing_source, sync_model_price, sync_group_ratio, group_mapping, balance_threshold_usd)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id, created_at, updated_at`,
		c.Name, c.BaseURL, ak, dt, c.TargetChannelID, c.Enabled,
		c.BasePricePer1k, string(c.PricingSource), c.SyncModelPrice, c.SyncGroupRatio, gm, c.BalanceThresholdUSD,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func (r *upstreamPriceSyncRepo) GetConfig(ctx context.Context, id int64) (*service.UpstreamSourceConfig, error) {
	row := r.db.QueryRowContext(ctx, selectConfigSQL+` WHERE id=$1`, id)
	return scanConfig(row, r)
}

// ListConfigs / GetConfigByName / UpdateConfig / DeleteConfig:同模式(SELECT/UPDATE/DELETE + scanConfig)
// UpdateConfigSyncState: UPDATE ... SET last_sync_at=$2,last_pricing_version=$3,last_error=$4,updated_at=now() WHERE id=$1

func (r *upstreamPriceSyncRepo) CreateRequest(ctx context.Context, req *service.PriceChangeRequest, items []service.PriceChangeItem) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	summary, _ := json.Marshal(req.Summary)
	err = tx.QueryRowContext(ctx, `
INSERT INTO upstream_price_change_requests (source_config_id, trigger_type, status, upstream_pricing_version, summary, created_by)
VALUES ($1,$2,$3,$4,$5,$6) RETURNING id, created_at`,
		req.SourceConfigID, req.TriggerType, "open", req.UpstreamPricingVersion, summary, req.CreatedBy,
	).Scan(&req.ID, &req.CreatedAt)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	for i := range items {
		it := &items[i]
		it.RequestID = req.ID
		raw, _ := json.Marshal(it.UpstreamRaw)
		up, _ := service.MarshalConverted(it.UpstreamConverted)
		loc, _ := service.MarshalConverted(it.LocalCurrent)
		apv, _ := service.MarshalConverted(it.ApplyValue)
		_, err = tx.ExecContext(ctx, `
INSERT INTO upstream_price_change_items
(request_id, kind, platform, model_name, target_channel_id, upstream_raw, upstream_converted, local_current, apply_value, status)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			it.RequestID, string(it.Kind), it.Platform, it.ModelName, it.TargetChannelID,
			raw, up, loc, apv, "pending")
		if err != nil {
			return fmt.Errorf("create item: %w", err)
		}
	}
	return tx.Commit()
}

// GetRequest / ListRequests / ListItems / GetItem / UpdateItemStatus / ExpireOpenRequests:
// 标准 SELECT/UPDATE;ExpireOpenRequests = UPDATE ... SET status='expired' WHERE source_config_id=$1 AND status='open'
```
> `selectConfigSQL` / `scanConfig` 把 12+ 列扫入 struct,并对 `api_key_encrypted`/`dashboard_token_encrypted` 调 `r.decrypt(...)` 还原到 `APIKey`/`DashboardToken`;`group_mapping` 解 JSON 到 map;`pricing_source` 转 `UpstreamPricingSource`。

- [ ] **Step 3: 验证编译(补全所有方法后)**

Run: `cd backend && go build ./internal/repository/... ./internal/service/...`
Expected: 编译通过。

- [ ] **Step 4: Commit**

```bash
git add backend/internal/repository/upstream_price_sync_repo.go backend/internal/service/upstream_price_sync.go
git commit -m "feat(upstream-sync): repository with credential encryption + request txn"
```

---

## Task 8: `ChannelService.ApplyUpstreamPricingEntry`(逐条写入)

**Files:**
- Modify: `backend/internal/service/channel_service.go`(追加方法)

**Interfaces:**
- Consumes: `ChannelRepository.ListModelPricing/CreateModelPricing/UpdateModelPricing`(channel_service.go:28-53);`ConvertedPrice`(Task 3)
- Produces: `ApplyUpstreamPricingEntry(ctx, channelID, platform string, models []string, price ConvertedPrice) (*ChannelModelPricing, error)`,供 Task 9 调用

- [ ] **Step 1: 实现(委托 repo 逐条写入 + 失效缓存)**

追加到 `channel_service.go`:
```go
// ApplyUpstreamPricingEntry 对目标渠道写入/更新单条模型定价,不影响该渠道其他定价。
// 同 (platform, models) 命中则更新,否则新建。用于上游价格同步审批应用。
func (s *ChannelService) ApplyUpstreamPricingEntry(ctx context.Context, channelID int64, platform string, models []string, price ConvertedPrice) (*ChannelModelPricing, error) {
	if channelID <= 0 {
		return nil, infraerrors.BadRequest("invalid_channel", "channel id required")
	}
	if platform == "" {
		platform = "anthropic" // 与 repo 默认一致(channel_repo_pricing.go:227)
	}
	existing, err := s.repo.ListModelPricing(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list model pricing: %w", err)
	}
	for i := range existing {
		p := &existing[i]
		if p.Platform != platform {
			continue
		}
		if overlapModel(p.Models, models) {
			p.Models = mergeModels(p.Models, models)
			applyConvertedToPricing(p, price)
			if err := s.repo.UpdateModelPricing(ctx, p); err != nil {
				return nil, err
			}
			s.invalidateCache()
			return p, nil
		}
	}
	np := &ChannelModelPricing{ChannelID: channelID, Platform: platform, Models: models, BillingMode: price.BillingMode}
	applyConvertedToPricing(np, price)
	if err := s.repo.CreateModelPricing(ctx, np); err != nil {
		return nil, err
	}
	s.invalidateCache()
	return np, nil
}

func applyConvertedToPricing(p *ChannelModelPricing, c ConvertedPrice) {
	p.BillingMode = c.BillingMode
	p.InputPrice = c.InputPrice
	p.OutputPrice = c.OutputPrice
	p.CacheReadPrice = c.CacheReadPrice
	p.CacheWritePrice = c.CacheWritePrice
	p.PerRequestPrice = c.PerRequestPrice
}
```
> `overlapModel` / `mergeModels` 为简单字符串集合工具(同文件实现,大小写不敏感)。`infraerrors` 包路径 `internal/pkg/errors`。`invalidateCache()` 见 channel_service.go:345。

- [ ] **Step 2: 验证编译**

Run: `cd backend && go build ./internal/service/...`
Expected: 编译通过。

- [ ] **Step 3: Commit**

```bash
git add backend/internal/service/channel_service.go
git commit -m "feat(upstream-sync): ChannelService.ApplyUpstreamPricingEntry"
```

---

## Task 9: `UpstreamPriceSyncService`(SyncNow/Review/Apply)

**Files:**
- Create: `backend/internal/service/upstream_price_sync_service.go`
- Test: `backend/internal/service/upstream_price_sync_service_test.go`

**Interfaces:**
- Consumes: Task 6 `UpstreamPricingClient`、Task 7 repo、Task 8 `ApplyUpstreamPricingEntry`、Task 4/5 纯函数;`urlvalidator.ValidateHTTPURL`(`internal/util/urlvalidator/validator.go:28`)用 `NewAPIHosts`
- Produces: `UpstreamPriceSyncService.SyncNow/GetRequest/ListRequests/ReviewItem/CloseRequest`,供 Task 11 handler 调用

- [ ] **Step 1: 写失败测试(集成:fake repo + mock client + fake channelService)**

```go
package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSyncNow_BuildsRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"ModelRatio":{"claude-x":1.5},"CompletionRatio":{"claude-x":2},"ModelPrice":{},"GroupRatio":{}}}`))
	}))
	defer srv.Close()
	cfg := testConfig(srv.URL) // NewAPIHosts 含 127.0.0.1, AllowPrivateHosts=true
	fakeRepo := newFakeRepo()
	client := &UpstreamPricingClient{httpOpts: testHTTPOpts()}
	chSvc := newFakeChannelService() // 记录 ApplyUpstreamPricingEntry 调用
	svc := NewUpstreamPriceSyncService(fakeRepo, client, chSvc, cfg)

	cfgRec := &UpstreamSourceConfig{ID: 1, BaseURL: srv.URL, TargetChannelID: 7, BasePricePer1k: 0.002, Enabled: true, PricingSource: PricingSourceAuto}
	_ = fakeRepo.CreateConfig(context.Background(), cfgRec)

	reqID, err := svc.SyncNow(context.Background(), 1, 99)
	if err != nil {
		t.Fatalf("SyncNow err: %v", err)
	}
	if reqID == 0 {
		t.Fatal("expected request id")
	}
	items, _ := fakeRepo.ListItems(context.Background(), reqID)
	if len(items) == 0 {
		t.Fatal("expected diff items (claude-x is new -> model_added)")
	}
}

func TestReviewItem_ApplyWrites(t *testing.T) {
	// 构造一个 pending item,ReviewItem(apply) → chSvc 收到 ApplyUpstreamPricingEntry + item.status=applied
	// (fakeRepo 预置 item;断言 chSvc.applied != nil 且 fakeRepo 的 item status 更新)
}
```
> `newFakeRepo`/`newFakeChannelService`/`testConfig` 为测试 helper(fake 实现 `UpstreamPriceSyncRepository` 与最小 `ChannelService` 子集;`ChannelService` 不便 mock 时,定义一个 `channelApplier` 接口供 service 依赖,测试用 fake)。

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/service/ -run 'TestSyncNow|TestReviewItem' -v`
Expected: FAIL(类型未定义)。

- [ ] **Step 3: 实现**

```go
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

// channelApplier 解耦对 ChannelService 的依赖(便于测试)。
type channelApplier interface {
	ApplyUpstreamPricingEntry(ctx context.Context, channelID int64, platform string, models []string, price ConvertedPrice) (*ChannelModelPricing, error)
	GetByID(ctx context.Context, id int64) (*Channel, error)
}

type UpstreamPriceSyncService struct {
	repo           UpstreamPriceSyncRepository
	client         *UpstreamPricingClient
	channelService channelApplier
	cfg            *config.Config
}

func NewUpstreamPriceSyncService(repo UpstreamPriceSyncRepository, client *UpstreamPricingClient, chSvc *ChannelService, cfg *config.Config) *UpstreamPriceSyncService {
	return &UpstreamPriceSyncService{repo: repo, client: client, channelService: chSvc, cfg: cfg}
}

func (s *UpstreamPriceSyncService) SyncNow(ctx context.Context, configID, createdBy int64) (int64, error) {
	cfgRec, err := s.repo.GetConfig(ctx, configID)
	if err != nil {
		return 0, err
	}
	if !cfgRec.Enabled {
		return 0, fmt.Errorf("source disabled")
	}
	baseURL, err := s.validateBaseURL(cfgRec.BaseURL)
	if err != nil {
		return 0, err
	}
	snap, err := s.client.FetchPricing(ctx, baseURL, cfgRec.PricingSource)
	if err != nil {
		_ = s.repo.UpdateConfigSyncState(ctx, configID, time.Now(), "", err.Error())
		return 0, fmt.Errorf("fetch upstream: %w", err)
	}
	// 还原 + 平台
	up := map[string]ConvertedPrice{}
	plat := map[string]string{}
	rawByName := map[string]map[string]any{}
	for _, m := range snap.Models {
		up[m.ModelName] = ConvertPricing(m, cfgRec.BasePricePer1k)
		plat[m.ModelName] = InferPlatform(m.ModelName)
		rawByName[m.ModelName] = map[string]any{
			"model_ratio": m.ModelRatio, "completion_ratio": m.CompletionRatio,
			"quota_type": m.QuotaType,
		}
	}
	// 本地现有定价
	ch, err := s.channelService.GetByID(ctx, cfgRec.TargetChannelID)
	if err != nil {
		return 0, fmt.Errorf("load target channel: %w", err)
	}
	drafts := DiffPricing(up, plat, ch.ModelPricing, cfgRec.TargetChannelID)
	if len(drafts) == 0 {
		_ = s.repo.UpdateConfigSyncState(ctx, configID, time.Now(), snap.Version, "")
		return 0, nil // 无变化,不建空审批单
	}
	// 旧 open 单标记 expired
	_, _ = s.repo.ExpireOpenRequests(ctx, configID)
	// 组装 items
	items := make([]PriceChangeItem, 0, len(drafts))
	for _, d := range drafts {
		apv := d.Upstream // 默认 apply_value = 上游还原值;model_removed 无上游值,apply_value=nil
		items = append(items, PriceChangeItem{
			Kind: d.Kind, Platform: d.Platform, ModelName: d.ModelName,
			TargetChannelID: cfgRec.TargetChannelID,
			UpstreamRaw: rawByName[d.ModelName], UpstreamConverted: d.Upstream,
			LocalCurrent: d.Local, ApplyValue: apv,
		})
	}
	req := &PriceChangeRequest{SourceConfigID: configID, TriggerType: "manual", UpstreamPricingVersion: snap.Version, CreatedBy: createdBy}
	if err := s.repo.CreateRequest(ctx, req, items); err != nil {
		return 0, err
	}
	_ = s.repo.UpdateConfigSyncState(ctx, configID, time.Now(), snap.Version, "")
	return req.ID, nil
}

func (s *UpstreamPriceSyncService) ReviewItem(ctx context.Context, itemID int64, action ReviewAction, applyValue *ConvertedPrice, reviewerID int64, note string) error {
	it, err := s.repo.GetItem(ctx, itemID)
	if err != nil {
		return err
	}
	if it.Status != "pending" {
		return fmt.Errorf("item not pending (status=%s)", it.Status)
	}
	now := time.Now()
	switch action {
	case ReviewReject, ReviewIgnore:
		st := map[ReviewAction]string{ReviewReject: "rejected", ReviewIgnore: "ignored"}[action]
		return s.repo.UpdateItemStatus(ctx, itemID, st, reviewerID, note, nil)
	case ReviewApply:
		val := applyValue
		if val == nil {
			val = it.ApplyValue
		}
		if val == nil {
			val = it.UpstreamConverted
		}
		_, err := s.channelService.ApplyUpstreamPricingEntry(ctx, it.TargetChannelID, it.Platform, []string{it.ModelName}, *val)
		if err != nil {
			_ = s.repo.UpdateItemStatus(ctx, itemID, "failed", reviewerID, err.Error(), nil)
			return fmt.Errorf("apply: %w", err)
		}
		return s.repo.UpdateItemStatus(ctx, itemID, "applied", reviewerID, note, &now)
	}
	return fmt.Errorf("unknown action")
}

func (s *UpstreamPriceSyncService) validateBaseURL(raw string) (string, error) {
	allow := s.cfg.Security.URLAllowlist
	return urlvalidator.ValidateHTTPURL(raw, allow.AllowInsecureHTTP, urlvalidator.ValidationOptions{
		AllowedHosts:     allow.NewAPIHosts,
		RequireAllowlist: allow.Enabled,
		AllowPrivate:     allow.AllowPrivateHosts,
	})
}

// GetRequest / ListRequests / CloseRequest:薄封装委托 repo(CloseRequest 把无 pending 的 open 单置 closed)
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test ./internal/service/ -run 'TestSyncNow|TestReviewItem' -v`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/upstream_price_sync_service.go backend/internal/service/upstream_price_sync_service_test.go
git commit -m "feat(upstream-sync): sync service SyncNow/Review/Apply"
```

---

## Task 10: Wire 注入

**Files:**
- Modify: `backend/internal/repository/wire.go`(或 repository ProviderSet 所在)— 加 `NewUpstreamPriceSyncRepository`
- Modify: `backend/internal/service/wire.go:565` `ProviderSet` — 加 `NewUpstreamPricingClient`、`NewUpstreamPriceSyncService`、encKey provider
- Modify: `backend/internal/handler/wire.go` — `NewChannelHandler`(`wire.go:213`)加 `upstreamPriceSyncService` 参数

**Interfaces:**
- Consumes: Task 6/7/9 的构造函数
- Produces: 完整依赖图;`ChannelHandler.upstreamPriceSyncService` 字段(TASK 11 用)

- [ ] **Step 1: 确认加密 key 来源**

Run: `cd backend && grep -rn "crypto.Encrypt(\|payment.Encrypt(" internal/ cmd/ | head`
→ 找到现有调用点如何取 key(多为某个 cfg 字段或启动注入),照它在 service 包加 provider:
```go
func provideUpstreamSyncEncKey(cfg *config.Config) []byte { /* 返回与 payment/crypto 同源的 key */ }
```

- [ ] **Step 2: 加 providers**

`repository` ProviderSet 加 `repository.NewUpstreamPriceSyncRepository`。`service/wire.go` `ProviderSet` 加:
```go
service.NewUpstreamPricingClient,
service.NewUpstreamPriceSyncService,
service.ProvideUpstreamSyncEncKey, // 上面定义的 encKey provider
```
> `NewUpstreamPriceSyncService(repo, client, *ChannelService, cfg)` 由 Wire 自动解析 `*ChannelService`。

- [ ] **Step 3: `NewChannelHandler` 加参数**

`handler/wire.go:213` 的 `admin.NewChannelHandler` 调用追加 `upstreamPriceSyncService` 实参(Wire 会注入)。

- [ ] **Step 4: 重新生成 + 编译**

Run: `cd backend && go generate ./... && go build ./...`
Expected: Wire 生成成功,编译通过。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/wire.go backend/internal/repository/wire.go backend/internal/handler/wire.go backend/cmd/server/wire_gen.go
git commit -m "feat(upstream-sync): wire providers"
```

---

## Task 11: admin handler + 路由

**Files:**
- Modify: `backend/internal/handler/admin/channel_handler.go:17-26` — struct 加字段 + 构造加参数
- Create: `backend/internal/handler/admin/upstream_price_sync_handler.go` — `(h *ChannelHandler)` 方法
- Modify: `backend/internal/server/routes/admin.go:643-654` `registerChannelRoutes` — 加路由

**Interfaces:**
- Consumes: Task 9 service;`response.Success/BadRequest/InternalError`(`internal/pkg/response`);`infraerrors`(`internal/pkg/errors`)
- Produces: REST 端点(见路由)

- [ ] **Step 1: struct + 构造加字段**

`channel_handler.go` `ChannelHandler` 加:
```go
upstreamPriceSyncService *service.UpstreamPriceSyncService
```
`NewChannelHandler` 签名加 `upstreamPriceSyncService *service.UpstreamPriceSyncService` 参数并赋值。

- [ ] **Step 2: handler 方法(config CRUD + 同步 + 审批)**

`upstream_price_sync_handler.go`:
```go
package admin

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// --- 上游源配置 ---

func (h *ChannelHandler) ListUpstreamSources(c *gin.Context) {
	list, err := h.upstreamPriceSyncService.ListConfigs(c.Request.Context())
	if err != nil {
		response.InternalError(c, "list upstream sources: "+err.Error())
		return
	}
	response.Success(c, list)
}

func (h *ChannelHandler) CreateUpstreamSource(c *gin.Context) {
	var req service.UpstreamSourceConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid: "+err.Error())
		return
	}
	if err := h.upstreamPriceSyncService.CreateConfig(c.Request.Context(), &req); err != nil {
		response.InternalError(c, "create: "+err.Error())
		return
	}
	response.Created(c, req)
}

// GetUpstreamSource / UpdateUpstreamSource / DeleteUpstreamSource:同模式(路径 :id)

func (h *ChannelHandler) SyncUpstreamNow(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	reqID, err := h.upstreamPriceSyncService.SyncNow(c.Request.Context(), id, adminUserID(c))
	if err != nil {
		response.InternalError(c, "sync: "+err.Error())
		return
	}
	response.Success(c, gin.H{"request_id": reqID})
}

// --- 审批单 ---

func (h *ChannelHandler) ListPriceChangeRequests(c *gin.Context) {
	// 解析 query:status, source_config_id, page, page_size → RequestFilter
}
func (h *ChannelHandler) GetPriceChangeRequest(c *gin.Context) {
	// id → request + items
}
func (h *ChannelHandler) ReviewPriceChangeItem(c *gin.Context) {
	var body struct {
		Action     string                  `json:"action"`
		ApplyValue *service.ConvertedPrice `json:"apply_value"`
		Note       string                  `json:"note"`
	}
	// _ = c.ShouldBindJSON(&body); itemID := c.Param("itemId")
	// h.upstreamPriceSyncService.ReviewItem(ctx, itemID, ReviewAction(body.Action), body.ApplyValue, adminUserID(c), body.Note)
}
func (h *ChannelHandler) ClosePriceChangeRequest(c *gin.Context) {}
```
> `adminUserID(c)` 取当前 admin 用户 id(复用现有中间件注入的 context helper;若项目用 `c.GetInt64("user_id")` 则照用)。其余方法照 `SyncFromCRS`(account_handler.go:1044)模式补全。

- [ ] **Step 3: 路由**

`registerChannelRoutes`(admin.go:643)的 `channels` 组内追加:
```go
channels.GET("/upstream-sources", h.Admin.Channel.ListUpstreamSources)
channels.POST("/upstream-sources", h.Admin.Channel.CreateUpstreamSource)
channels.GET("/upstream-sources/:id", h.Admin.Channel.GetUpstreamSource)
channels.PUT("/upstream-sources/:id", h.Admin.Channel.UpdateUpstreamSource)
channels.DELETE("/upstream-sources/:id", h.Admin.Channel.DeleteUpstreamSource)
channels.POST("/upstream-sources/:id/sync", h.Admin.Channel.SyncUpstreamNow)

channels.GET("/price-change-requests", h.Admin.Channel.ListPriceChangeRequests)
channels.GET("/price-change-requests/:id", h.Admin.Channel.GetPriceChangeRequest)
channels.POST("/price-change-requests/:id/items/:itemId/review", h.Admin.Channel.ReviewPriceChangeItem)
channels.POST("/price-change-requests/:id/close", h.Admin.Channel.ClosePriceChangeRequest)
```

- [ ] **Step 4: 编译 + 启动手测**

Run: `cd backend && go build ./... && ./server`
Expected: 启动正常,`curl -H "Authorization: ..." /admin/channels/upstream-sources` 返回空列表 envelope。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handler/admin/channel_handler.go backend/internal/handler/admin/upstream_price_sync_handler.go backend/internal/server/routes/admin.go
git commit -m "feat(upstream-sync): admin handler + routes"
```

---

## Task 12: 前端 api 模块

**Files:**
- Create: `frontend/src/api/admin/upstreamPriceSync.ts`
- Modify: `frontend/src/api/admin/index.ts` — 注册 `upstreamPriceSync`

**Interfaces:**
- Consumes: `apiClient`(`@/client.ts`),模式仿 `channelMonitorTemplate.ts`
- Produces: `upstreamPriceSyncAPI`(listSources/createSource/updateSource/deleteSource/syncNow/listRequests/getRequest/reviewItem/closeRequest)

- [ ] **Step 1: 写 api 模块**

```ts
import { apiClient } from '../client'

export interface UpstreamSourceConfig {
  id?: number
  name: string
  base_url: string
  api_key: string
  dashboard_token?: string
  target_channel_id: number
  enabled: boolean
  base_price_per_1k: number
  pricing_source: 'auto' | 'ratio_config' | 'pricing'
  sync_model_price: boolean
  last_sync_at?: string | null
  last_error?: string | null
}

export interface PriceChangeItem {
  id: number
  request_id: number
  kind: 'model_price' | 'model_added' | 'model_removed'
  platform: string
  model_name: string
  target_channel_id: number
  upstream_converted: any
  local_current: any
  apply_value: any
  status: string
}

export interface PriceChangeRequest { id: number; source_config_id: number; status: string; summary: any; created_at: string }

const base = '/admin/channels'
export async function listSources() {
  const { data } = await apiClient.get<UpstreamSourceConfig[]>(`${base}/upstream-sources`)
  return data
}
export async function createSource(p: UpstreamSourceConfig) {
  const { data } = await apiClient.post<UpstreamSourceConfig>(`${base}/upstream-sources`, p)
  return data
}
export async function updateSource(id: number, p: UpstreamSourceConfig) {
  const { data } = await apiClient.put<UpstreamSourceConfig>(`${base}/upstream-sources/${id}`, p)
  return data
}
export async function deleteSource(id: number) {
  await apiClient.delete(`${base}/upstream-sources/${id}`)
}
export async function syncNow(id: number) {
  const { data } = await apiClient.post<{ request_id: number }>(`${base}/upstream-sources/${id}/sync`)
  return data
}
export async function listRequests(params: { status?: string }) {
  const { data } = await apiClient.get<PriceChangeRequest[]>(`${base}/price-change-requests`, { params })
  return data
}
export async function getRequest(id: number) {
  const { data } = await apiClient.get<{ request: PriceChangeRequest; items: PriceChangeItem[] }>(`${base}/price-change-requests/${id}`)
  return data
}
export async function reviewItem(reqId: number, itemId: number, body: { action: string; apply_value?: any; note?: string }) {
  const { data } = await apiClient.post(`${base}/price-change-requests/${reqId}/items/${itemId}/review`, body)
  return data
}
export async function closeRequest(id: number) {
  await apiClient.post(`${base}/price-change-requests/${id}/close`)
}
export const upstreamPriceSyncAPI = { listSources, createSource, updateSource, deleteSource, syncNow, listRequests, getRequest, reviewItem, closeRequest }
export default upstreamPriceSyncAPI
```

- [ ] **Step 2: 注册到聚合入口**

`frontend/src/api/admin/index.ts` 加:
```ts
import upstreamPriceSync from './upstreamPriceSync'
// ...
export const adminAPI = { /* ...existing... */, upstreamPriceSync }
```

- [ ] **Step 3: 类型检查**

Run: `cd frontend && pnpm typecheck`(或 `pnpm build`)
Expected: 无类型错误。

- [ ] **Step 4: Commit**

```bash
git add frontend/src/api/admin/upstreamPriceSync.ts frontend/src/api/admin/index.ts
git commit -m "feat(upstream-sync): frontend api module"
```

---

## Task 13: 前端 View(上游源配置 + 审批单)+ 路由

**Files:**
- Create: `frontend/src/views/admin/UpstreamSourcesView.vue`
- Create: `frontend/src/views/admin/PriceChangeRequestsView.vue`
- Modify: `frontend/src/router/index.ts` — 注册两个 admin 路由

**Interfaces:**
- Consumes: Task 12 `upstreamPriceSyncAPI`;`@/components/common/*`(`DataTable`/`BaseDialog`/`Pagination`/`ConfirmDialog`/`EmptyState`);`@/components/layout/TablePageLayout.vue`;`extractApiErrorMessage`(`@/utils/apiError`);`adminAPI.channels.list`(选目标渠道下拉);模式仿 `ChannelsView.vue:627-711`

- [ ] **Step 1: `UpstreamSourcesView.vue`(列表 + 新建/编辑弹窗 + 「立即同步」)**

`<script setup lang="ts">` 骨架:
- `onMounted` → `adminAPI.upstreamPriceSync.listSources()` + `adminAPI.channels.list()`(填目标渠道下拉)
- 列:`name / base_url / target_channel / pricing_source / last_sync_at / actions`
- 新建/编辑:`BaseDialog` 表单(name/base_url/api_key/dashboard_token/target_channel_id/base_price_per_1k/pricing_source/enabled)
- 「立即同步」→ `syncNow(id)` → 成功后 toast + 跳转 `/admin/price-change-requests`(或展开详情)
- 删除:`ConfirmDialog` → `deleteSource`
- 错误:`extractApiErrorMessage(err)`

- [ ] **Step 2: `PriceChangeRequestsView.vue`(审批单列表 + 详情逐条审批)**

`<script setup lang="ts">` 骨架:
- `onMounted` → `listRequests({})`;点击行 → `getRequest(id)` 展开详情 items 表
- items 列:`model_name / platform / kind / upstream_converted(还原值) / local_current(当前值) / apply_value(可编辑 input) / actions`
- 每行操作:批准(apply,提交 apply_value)/ 忽略(ignore)/ 拒绝(reject)→ `reviewItem`;批量栏(多选 + 批量 apply)
- status 计数展示(`request.summary`)
- `model_removed` 默认置灰(apply_value 不可编辑)

- [ ] **Step 3: 路由**

`router/index.ts`(仿 `AdminChannels` 块,index.ts:508)加:
```ts
{
  path: '/admin/upstream-sources',
  name: 'AdminUpstreamSources',
  component: () => import('@/views/admin/UpstreamSourcesView.vue'),
  meta: { requiresAuth: true, requiresAdmin: true, title: 'Upstream Sources', titleKey: 'admin.upstreamSources.title' }
},
{
  path: '/admin/price-change-requests',
  name: 'AdminPriceChangeRequests',
  component: () => import('@/views/admin/PriceChangeRequestsView.vue'),
  meta: { requiresAuth: true, requiresAdmin: true, title: 'Price Change Requests', titleKey: 'admin.priceChangeRequests.title' }
}
```
> 侧边栏导航:在 admin 布局的菜单配置里加两项(找现有菜单数组,grep `AdminChannels` 菜单项仿写)。

- [ ] **Step 4: 前端构建 + 手测**

Run: `cd frontend && pnpm build`
Expected: 构建成功。浏览器:建上游源 → 点同步 → 审批单出现 → 逐条批准 → 回渠道定价页验证价格已更新。

- [ ] **Step 5: Commit**

```bash
git add frontend/src/views/admin/UpstreamSourcesView.vue frontend/src/views/admin/PriceChangeRequestsView.vue frontend/src/router/index.ts
git commit -m "feat(upstream-sync): admin views + routes"
```

---

## Self-Review

**1. Spec 覆盖**(design P1 各节 → task):
- §5 架构/组件(client/service/handler/前端)→ Task 6/9/11/12/13 ✓
- §6 数据模型(3 表 + 凭证)→ Task 2/7(凭证移到配置表,见 Spec Deviations)✓
- §7 还原 + diff → Task 4/5 ✓
- §8 数据流(SyncNow/Review/Apply)→ Task 9 ✓
- §9 错误处理(不建空单/逐条隔离/重复 expired/幂等)→ Task 9(SyncNow 空单返回 0、ReviewItem failed 隔离、ExpireOpenRequests、ApplyItem 幂等由 repo upsert 保证)✓
- §10 测试 → Task 4/5/6/9 单测 + Task 11/13 手测 ✓
- §11 前置条件(NewAPIHosts/白名单)→ Task 1/9 validateBaseURL ✓

**2. 占位符扫描:**
- Task 7/10 的「加密 key 来源」用 grep 明确动作(非 TODO);Task 7 CRUD 其余方法标注「同模式」但给了 `selectConfigSQL/scanConfig` 与 `ExpireOpenRequests` 的确切 SQL——可接受。Task 11 部分方法体用注释描述 + 指明范本行号(account_handler.go:1044),已给足够可复用锚点。Task 13 View 给骨架而非全文(Vue 模板量大,script setup 模式已锚定 ChannelsView.vue)。

**3. 类型一致性:**
- `ConvertedPrice`(Task 3)在 Task 4/5/8/9/11/12 一致 ✓
- `UpstreamPricingSource`(Task 3)在 Task 6/9/11 一致 ✓
- `ReviewAction`(Task 3)在 Task 9/11 一致 ✓
- `UpstreamPriceSyncRepository`(Task 7)在 Task 9/10 一致 ✓
- `ApplyUpstreamPricingEntry`(Task 8 定义、Task 9 通过 `channelApplier` 消费)✓
- `PriceChangeItemDraft`(Task 5)在 Task 9 SyncNow 消费一致 ✓

**已知实现期需确认点(非占位符,已给定位):**
- 加密 key 的 cfg 字段(Task 10 grep)
- admin user id 的 context helper 取法(Task 11 `adminUserID(c)`)
- ChannelService 是否需导出 `GetByID`(已是公开方法 channel_service.go:737)✓

