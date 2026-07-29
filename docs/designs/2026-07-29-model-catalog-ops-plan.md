# 模型广场可运营排序 + 套餐展示 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让运营能在后台控制模型广场的模型顺序（置顶/标签/隐藏）并把在售套餐展示到广场顶部。

**Architecture:** 在「渠道→去重模型卡片」链路中插入 `model_catalog_display` 配置层（ent + 迁移），运营在 `/admin/catalog` 管理页控制置顶/标签/隐藏，广场默认排序从「价格」改为「推荐」；另新增无鉴权公开套餐接口，在广场顶部渲染套餐区。零配置时广场与现状一致（灰度安全）。

**Tech Stack:** Go 1.26 / Gin / Ent ORM / Wire DI / golangci-lint v2.9 ｜ Vue 3 / TS / Vite / Pinia / TailwindCSS / pnpm ｜ PostgreSQL 18

**对应 spec:** `docs/designs/2026-07-29-model-catalog-ops-design.md`

## Global Constraints

每个任务的隐性前置约束（逐字照做）：
- **ent schema 改后必须 `cd backend && go generate ./ent`** 并提交生成代码。
- **新增表必须手写迁移** `backend/migrations/176_*.sql`，幂等（`CREATE TABLE IF NOT EXISTS`），文件头 `SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='10min';`。迁移一旦应用不可改（SHA256 锁定）。
- **新增 service 必须在 `backend/cmd/server/wire.go` 加 Provider** 并 `go generate ./cmd/server` 重生成 `wire_gen.go`。
- **给 interface 加方法后**，搜索 `type.*Stub.*struct` / `type.*Mock.*struct` 补全所有测试 stub，否则编译失败。
- **列表端点空结果必须返回 `[]`**（`make([]T,0)`），禁止 `nil`。
- **depguard**：handler 不得 import repository/gorm/redis；service 不得 import repository/gorm/redis（少数 ops 例外）。
- **前端必须 pnpm**（非 npm）；i18n 新 key 一律在 `frontend/src/i18n/locales/{zh,en}/custom.ts` 深合并补缺，不改 main。
- **迁移验证**：起 `postgres:18-alpine` 容器、预建 `schema_migrations`、从零按序跑全部迁移（每个文件仅一次）。
- **错误响应**统一用项目 `response` 包（`response.Success/NotFound/BadRequest/InternalError/Paginated`）；plan 中 `response.Xxx(c, ...)` 的 `...` = 按上下文自拟的具体消息。

## File Structure

**后端 Create:**
- `backend/ent/schema/model_catalog_display.go` — 展示配置实体
- `backend/migrations/176_create_model_catalog_display.sql` — 建表
- `backend/internal/repository/model_catalog_repo.go` — 配置 CRUD（ent client）
- `backend/internal/service/model_catalog_service.go` — merge 配置 + 过滤 + 标签判定
- `backend/internal/handler/admin_model_catalog_handler.go` — 管理页 GET/PUT
- `backend/internal/handler/public_bundle_plan_handler.go` — 公开套餐列表（Part B）

**后端 Modify:**
- `backend/internal/handler/public_model_catalog_handler.go` — catalog 加运营字段、过滤 hidden、调用 merge
- `backend/internal/server/routes/public_models.go` — 加 admin catalog 路由 + 公开套餐路由
- `backend/cmd/server/wire.go` + `wire_gen.go` — DI 注入
- `backend/internal/service/setting_service.go` — 加 `model_catalog.new_model_days` / `model_catalog.ops_enabled` 键

**前端 Create:**
- `frontend/src/views/admin/CatalogManageView.vue` — 管理页
- `frontend/src/api/adminCatalog.ts` — 管理 API client
- `frontend/src/api/publicBundles.ts` — 公开套餐 API client（Part B）
- `frontend/src/components/models/BundleShowcaseSection.vue` — 广场顶部套餐区（Part B）
- `frontend/src/components/bundles/BundlePlanCard.vue` — 抽取的公共套餐卡（Part B）

**前端 Modify:**
- `frontend/src/utils/modelCatalog.ts` — `recommended` 排序分支 + 权重常量
- `frontend/src/views/public/ModelCatalogView.vue` — 默认 `recommended` + 顶部套餐区
- `frontend/src/components/models/ModelCard.vue` — 运营标签 badge + 置顶角标
- `frontend/src/router/index.ts` — admin 路由 `/admin/catalog`
- `frontend/src/i18n/locales/{zh,en}/custom.ts` — 新 key

---

# Part A — 模型可运营排序（先落地）

里程碑：Part A 完成后，广场默认按「推荐」排序，运营可在 `/admin/catalog` 控制置顶/标签/隐藏；开关关掉回退原状。

### Task A1: ent schema + 迁移 + 生成

**Files:**
- Create: `backend/ent/schema/model_catalog_display.go`
- Create: `backend/migrations/176_create_model_catalog_display.sql`

**Interfaces:**
- Produces: ent 实体 `ModelCatalogDisplay`（表 `model_catalog_display`），业务键 `(platform, model_name)` unique

- [ ] **Step 1: 写 ent schema**

```go
// backend/ent/schema/model_catalog_display.go
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ModelCatalogDisplay struct{ ent.Schema }

func (ModelCatalogDisplay) Fields() []ent.Field {
	return []ent.Field{
		field.String("platform").NotEmpty().MaxLen(64),
		field.String("model_name").NotEmpty().MaxLen(128),
		field.Bool("pinned").Default(false),
		field.Int("sort_weight").Default(0),
		field.JSON("custom_tags", []string{}).Default([]string{}),
		field.Time("featured_until").Optional().Nillable(),
		field.Bool("hidden").Default(false),
		field.Time("first_seen_at").Default(time.Now),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (ModelCatalogDisplay) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("platform", "model_name").Unique(),
	}
}
```

- [ ] **Step 2: 写迁移**

```sql
-- backend/migrations/176_create_model_catalog_display.sql
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS model_catalog_display (
    id              BIGSERIAL    PRIMARY KEY,
    platform        VARCHAR(64)  NOT NULL,
    model_name      VARCHAR(128) NOT NULL,
    pinned          BOOLEAN      NOT NULL DEFAULT FALSE,
    sort_weight     INTEGER      NOT NULL DEFAULT 0,
    custom_tags     JSONB        NOT NULL DEFAULT '[]',
    featured_until  TIMESTAMPTZ,
    hidden          BOOLEAN      NOT NULL DEFAULT FALSE,
    first_seen_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (platform, model_name)
);
COMMENT ON TABLE model_catalog_display IS '模型广场展示配置（置顶/标签/隐藏），按平台+模型名';
```

- [ ] **Step 3: 生成 ent 代码**

Run: `cd backend && go generate ./ent`
Expected: 生成 `ent/modelcatalogdisplay/`、`ent/modelcatalogdisplay_create.go`、`ent/modelcatalogdisplay_query.go`，`ent/mutation.go` / `ent/migrate/schema.go` 出现 ModelCatalogDisplay。

- [ ] **Step 4: 编译验证**

Run: `cd backend && go build ./ent/...`
Expected: PASS

- [ ] **Step 5: 迁移验证（一次性容器）**

Run（从零跑全部迁移至 176，验证 176 建表成功、unique 约束存在）:
```bash
docker run --rm -d --name pg-mc -e POSTGRES_PASSWORD=x -p 55432:5432 postgres:18-alpine
# 预建 schema_migrations 后按序跑 backend/migrations/*.sql（脚本或手动，每个一次）
# 确认 \d model_catalog_display 存在且 platform+model_name 有 UNIQUE
docker stop pg-mc
```

- [ ] **Step 6: Commit**

```bash
git add backend/ent/schema/model_catalog_display.go backend/ent/modelcatalogdisplay* backend/ent/mutation.go backend/ent/migrate/schema.go backend/migrations/176_create_model_catalog_display.sql
git commit -m "feat(catalog): add model_catalog_display ent schema + migration 176"
```

---

### Task A2: repository（配置 CRUD）

**Files:**
- Create: `backend/internal/repository/model_catalog_repo.go`
- Create: `backend/internal/repository/model_catalog_repo_test.go`（unit）

**Interfaces:**
- Consumes: `*ent.Client`（项目 repository 惯例注入）
- Produces:
  - `type ModelCatalogDisplay struct { Platform, ModelName string; Pinned bool; SortWeight int; CustomTags []string; FeaturedUntil *time.Time; Hidden bool; FirstSeenAt time.Time }`
  - `type ModelKey struct { Platform, ModelName string }`
  - `ModelCatalogRepo interface { GetByModelKeys(ctx, keys []ModelKey) (map[string]*ModelCatalogDisplay, error); UpsertMissing(ctx, keys []ModelKey) error; ListAll(ctx) ([]*ModelCatalogDisplay, error); BatchUpsert(ctx, cfgs []*ModelCatalogDisplay) error }`
  - 构造函数 `NewModelCatalogRepo(client *ent.Client) ModelCatalogRepo`

- [ ] **Step 1: 写失败测试**（`model_catalog_repo_test.go`，用 `-tags=unit` + sqlite 或 mock；项目里 repo 单测惯例见 `channel_repo_test.go`）

核心断言（伪代码，按项目测试风格落实）：
```go
// UpsertMissing：首次插入两条 → map 有两条；再次调 UpsertMissing 同 keys → 不覆盖（first_seen_at 不变）
// GetByModelKeys：返回 map key = platform+"\x00"+model_name
// BatchUpsert：更新 pinned/sort_weight/custom_tags/featured_until/hidden
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend && go test -tags=unit ./internal/repository/ -run ModelCatalogRepo -v`
Expected: FAIL（未实现）

- [ ] **Step 3: 实现 repository**

```go
// backend/internal/repository/model_catalog_repo.go
package repository

import (
	"context"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/backend/ent"
	"github.com/Wei-Shaw/sub2api/backend/ent/modelcatalogdisplay"
)

type ModelCatalogDisplay struct {
	Platform      string
	ModelName     string
	Pinned        bool
	SortWeight    int
	CustomTags    []string
	FeaturedUntil *time.Time
	Hidden        bool
	FirstSeenAt   time.Time
}

type ModelKey struct{ Platform, ModelName string }

type ModelCatalogRepo interface {
	GetByModelKeys(ctx context.Context, keys []ModelKey) (map[string]*ModelCatalogDisplay, error)
	UpsertMissing(ctx context.Context, keys []ModelKey) error
	ListAll(ctx context.Context) ([]*ModelCatalogDisplay, error)
	BatchUpsert(ctx context.Context, cfgs []*ModelCatalogDisplay) error
}

type modelCatalogRepo struct{ client *ent.Client }

func NewModelCatalogRepo(client *ent.Client) ModelCatalogRepo {
	return &modelCatalogRepo{client: client}
}

func modelKey(p, m string) string { return p + "\x00" + m }

func (r *modelCatalogRepo) GetByModelKeys(ctx context.Context, keys []ModelKey) (map[string]*ModelCatalogDisplay, error) {
	out := make(map[string]*ModelCatalogDisplay, len(keys))
	if len(keys) == 0 {
		return out, nil
	}
	// 按 (platform,model_name) 二元组批量查；ent 无原生复合 in，按 platform 分组查询后内存过滤
	// 落地时可改为 raw SQL：SELECT ... WHERE (platform,model_name) IN (...)，参考 channel_repo_pricing.go 的 raw SQL 风格
	rows, err := r.client.ModelCatalogDisplay.Query().All(ctx)
	if err != nil {
		return nil, err
	}
	want := make(map[string]bool, len(keys))
	for _, k := range keys {
		want[modelKey(k.Platform, k.ModelName)] = true
	}
	for _, e := range rows {
		k := modelKey(e.Platform, e.ModelName)
		if want[k] {
			out[k] = entToModelCatalogDisplay(e)
		}
	}
	return out, nil
}

func (r *modelCatalogRepo) UpsertMissing(ctx context.Context, keys []ModelKey) error {
	existing, err := r.GetByModelKeys(ctx, keys)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if _, ok := existing[modelKey(k.Platform, k.ModelName)]; ok {
			continue
		}
		 builders := r.client.ModelCatalogDisplay.Create().
			SetPlatform(k.Platform).
			SetModelName(k.ModelName)
		if _, err := builders.Save(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *modelCatalogRepo) ListAll(ctx context.Context) ([]*ModelCatalogDisplay, error) {
	rows, err := r.client.ModelCatalogDisplay.Query().All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*ModelCatalogDisplay, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToModelCatalogDisplay(e))
	}
	return out, nil
}

func (r *modelCatalogRepo) BatchUpsert(ctx context.Context, cfgs []*ModelCatalogDisplay) error {
	// 简化：逐条 upsert（ON CONFLICT (platform,model_name) DO UPDATE）
	for _, c := range cfgs {
		q := r.client.ModelCatalogDisplay.Create().
			SetPlatform(c.Platform).
			SetModelName(c.ModelName).
			SetPinned(c.Pinned).
			SetSortWeight(c.SortWeight).
			SetCustomTags(c.CustomTags).
			SetHidden(c.Hidden)
		if c.FeaturedUntil != nil {
			q = q.SetFeaturedUntil(*c.FeaturedUntil)
		}
		// 用 ent 的 OnConflict 或先 Query 再 Create/Update；参考项目内 ent upsert 用法
		if err := q.OnConflict(
			sql.ConflictColumns(modelcatalogdisplay.FieldPlatform, modelcatalogdisplay.FieldModelName),
		).UpdateNewValues().Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

func entToModelCatalogDisplay(e *ent.ModelCatalogDisplay) *ModelCatalogDisplay {
	return &ModelCatalogDisplay{
		Platform: e.Platform, ModelName: e.ModelName, Pinned: e.Pinned,
		SortWeight: e.SortWeight, CustomTags: e.CustomTags, FeaturedUntil: e.FeaturedUntil,
		Hidden: e.Hidden, FirstSeenAt: e.FirstSeenAt,
	}
}
```
> 注：`GetByModelKeys` 的高效实现是 raw SQL 复合 IN（参考 `channel_repo_pricing.go`）；上例先给正确性，落地时按项目 raw SQL 风格优化。`OnConflict` 用法以项目内现有 ent upsert 为准（若项目无先例，改为 Query→Create/Update 两步）。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend && go test -tags=unit ./internal/repository/ -run ModelCatalogRepo -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/repository/model_catalog_repo.go backend/internal/repository/model_catalog_repo_test.go
git commit -m "feat(catalog): add model_catalog repository"
```

---

### Task A3: service（merge + 标签判定）+ 系统设置键

**Files:**
- Create: `backend/internal/service/model_catalog_service.go`
- Create: `backend/internal/service/model_catalog_service_test.go`（unit）
- Modify: `backend/internal/service/setting_service.go` — 注册两个键

**Interfaces:**
- Consumes: `repository.ModelCatalogRepo`、`SettingService`（读 `new_model_days`、`ops_enabled`）
- Produces:
  - `type CatalogDisplayInfo struct { Pinned bool; SortWeight int; Tags []string; IsNew bool; Featured bool }`
  - `type ModelCatalogService interface { MergeDisplayConfig(ctx, items []CatalogItem) ([]CatalogItem, error); EnsureFirstSeen(ctx, keys []repository.ModelKey) error; IsEnabled(ctx) bool }`
  - 构造函数 `NewModelCatalogService(repo repository.ModelCatalogRepo, settings SettingService) ModelCatalogService`
  - `CatalogItem`：由 public handler 既有结构体派生（Platform/ModelName/Name + 现有字段 + 新增 `Display *CatalogDisplayInfo`）；具体字段以 handler 现有 `publicModelCatalogItem` 为准，本任务在 service 定义输入/输出最小契约。

- [ ] **Step 1: 在 setting_service.go 注册键**

参照项目现有键注册方式（grep `new_model_days` 无 → 新增；找现有 `RegisterKey`/默认值模式），加：
```
model_catalog.new_model_days  int   default 30
model_catalog.ops_enabled     bool  default true
```
（按 `setting_service.go` 现有 key 注册结构照搬）

- [ ] **Step 2: 写失败测试**（`model_catalog_service_test.go`）

核心用例（用 stub repo + stub settings）：
```go
// 1. hidden=true 的 item 被 MergeDisplayConfig 过滤掉（不返回）
// 2. featured_until 在过去 → Featured=false，tags 不含 "featured"
// 3. first_seen_at 在 new_model_days 内 → IsNew=true，tags 含 "new"；超出 → false
// 4. custom_tags=["recommended"] + 自动 multimodal → Tags 合并两者
// 5. repo/setting 报错 → 降级：返回原 items 不变（不阻断）
// 6. IsEnabled 读 ops_enabled
```

- [ ] **Step 3: 运行测试确认失败**

Run: `cd backend && go test -tags=unit ./internal/service/ -run ModelCatalogService -v`
Expected: FAIL

- [ ] **Step 4: 实现 service**

```go
// backend/internal/service/model_catalog_service.go
package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/backend/internal/repository"
)

type CatalogDisplayInfo struct {
	Pinned     bool
	SortWeight int
	Tags       []string // 合并自动推断标签 + 手动 custom_tags + new/featured
	IsNew      bool
	Featured   bool
}

type CatalogItem struct {
	Platform  string
	ModelName string
	Name      string
	// 其余展示字段（定价等）由 handler 填入，merge 不改
	Display *CatalogDisplayInfo // merge 后填充；nil=无配置
}

type AdminCatalogConfig struct { // 管理页 GET/PUT，对应 repository.ModelCatalogDisplay
	Platform      string
	ModelName     string
	Pinned        bool
	SortWeight    int
	CustomTags    []string
	FeaturedUntil *time.Time
	Hidden        bool
	FirstSeenAt   time.Time
	Tags          []string // 自动+手动合并，只读展示
	IsNew         bool
	Featured      bool
}

type ModelCatalogService interface {
	MergeDisplayConfig(ctx context.Context, items []CatalogItem) ([]CatalogItem, error)
	EnsureFirstSeen(ctx context.Context, keys []repository.ModelKey) error
	ListAllForAdmin(ctx context.Context) ([]AdminCatalogConfig, error)
	BatchSave(ctx context.Context, cfgs []AdminCatalogConfig) error
	IsEnabled(ctx context.Context) bool
}

// MergeDisplayConfig：items 为去重后的模型卡片；按 (Platform,ModelName) 取配置 merge，
// 过滤 hidden，判定 new/featured 过期，合并 tags。配置层故障降级返回原 items。
```
> 关键实现点（务必落实，不留 TODO）：
> - `newModelDays`：`s.settings.GetInt(ctx, "model_catalog.new_model_days", 30)`
> - new 判定：`time.Since(firstSeenAt) < newModelDays*24h`
> - featured 过期：`featuredUntil != nil && now.After(*featuredUntil)` → `Featured=false`
> - Tags 合并：自动标签（由调用方/前端推断传入，或此处复用 `inferModelCapabilities` 等价逻辑）+ `custom_tags` + 条件性 `new`/`featured`；**去重保序**
> - 降级：`repo.GetByModelKeys` 或 settings 报错 → 记日志，`items` 原样返回（`Display=nil`）
> - `IsEnabled`：`s.settings.GetBool(ctx, "model_catalog.ops_enabled", true)`

- [ ] **Step 5: 运行测试确认通过**

Run: `cd backend && go test -tags=unit ./internal/service/ -run ModelCatalogService -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/service/model_catalog_service.go backend/internal/service/model_catalog_service_test.go backend/internal/service/setting_service.go
git commit -m "feat(catalog): add model catalog service + settings keys"
```

---

### Task A4: Wire DI 注入

**Files:**
- Modify: `backend/cmd/server/wire.go`
- Modify: `backend/cmd/server/wire_gen.go`（生成）

**Interfaces:**
- Consumes: Task A2 的 `NewModelCatalogRepo`、Task A3 的 `NewModelCatalogService`
- Produces: `*ModelCatalogService` 可注入到 handler

- [ ] **Step 1: wire.go 加 Provider**

在 `wire.go` 的 provider set 加 `repository.NewModelCatalogRepo`、`service.NewModelCatalogService`。确认 `*ent.Client` 已有 provider（项目内既有）。

- [ ] **Step 2: 重生成**

Run: `cd backend && go generate ./cmd/server`
Expected: `wire_gen.go` 出现 `NewModelCatalogRepo` / `NewModelCatalogService` 调用。

- [ ] **Step 3: 编译验证**

Run: `cd backend && go build ./cmd/server/`
Expected: PASS（若报 nil 守卫/缺依赖，补 wire provider）

- [ ] **Step 4: Commit**

```bash
git add backend/cmd/server/wire.go backend/cmd/server/wire_gen.go
git commit -m "feat(catalog): wire model catalog service"
```

---

### Task A5: public handler 接入 merge + 运营字段 + hidden 过滤

**Files:**
- Modify: `backend/internal/handler/public_model_catalog_handler.go`（`List:98`、`buildPublicModelCatalog:194`、`publicModelCatalogItem` 结构体）
- Modify: 若 `ModelCatalogService` 作为 handler 依赖 → 加构造注入（depguard：handler 不直接 import repository，只 import service）

**Interfaces:**
- Consumes: `service.ModelCatalogService`
- Produces: `GET /public/models/catalog` 响应每项新增 `pinned/sort_weight/tags/is_new/featured`；`hidden=true` 不返回

- [ ] **Step 1: 扩展响应结构体**

在 `publicModelCatalogItem`（handler 内）加字段（json tag）：`Pinned bool`、`SortWeight int`、`Tags []string`、`IsNew bool`、`Featured bool`。`buildPublicModelCatalog` 组装时这些先置零值。

- [ ] **Step 2: 在 List 接入 merge**

`List`（:98）：`catalog := buildPublicModelCatalog(channels)` 之后，
```go
if mcSvc != nil { // 依赖注入的 ModelCatalogService，nil 守卫
    catalog, _ = mcSvc.MergeDisplayConfig(ctx, catalog) // 内部过滤 hidden + merge + 标签判定；出错降级返回原 catalog
}
```
（handler 持有 `modelCatalogSvc service.ModelCatalogService` 字段，构造函数注入；参考现有 handler 的 service 注入方式）

- [ ] **Step 3: 补 handler 测试 / 调整既有测试**

既有 `public_model_catalog_handler_test.go` 若 stub 了 handler 构造，需补 `ModelCatalogService` mock 参数（interface 加方法/新依赖 → 补 stub，见 Global Constraints）。

- [ ] **Step 4: 编译 + 单测**

Run: `cd backend && go build ./internal/handler/... && go test -tags=unit ./internal/handler/ -run PublicModelCatalog -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handler/public_model_catalog_handler.go backend/internal/handler/public_model_catalog_handler_test.go
git commit -m "feat(catalog): merge display config into public catalog, filter hidden"
```

---

### Task A6: admin handler（管理页 GET/PUT）+ 路由

**Files:**
- Create: `backend/internal/handler/admin_model_catalog_handler.go`
- Create: `backend/internal/handler/admin_model_catalog_handler_test.go`
- Modify: `backend/internal/server/routes/public_models.go`（或项目 admin 路由文件）— 注册 `GET/PUT /admin/catalog/config`（在 admin 鉴权组）

**Interfaces:**
- Consumes: `service.ModelCatalogService`（EnsureFirstSeen）、`repository.ModelCatalogRepo`（ListAll/BatchUpsert）经 service 暴露，或 handler→service（depguard）
- Produces:
  - `GET /api/v1/admin/catalog/config` → 先 `EnsureFirstSeen(所有当前模型 keys)`，再返回 `[]configItem`（空返回 `[]`）
  - `PUT /api/v1/admin/catalog/config` → 批量保存（pinned/sort_weight/custom_tags/featured_until/hidden）

- [ ] **Step 1: 写失败测试**

```go
// GET：返回所有模型配置；无数据 → []（非 null）；调用过 EnsureFirstSeen
// PUT：批量 upsert；部分字段更新正确；返回 200
```

- [ ] **Step 2: 运行确认失败**

Run: `cd backend && go test -tags=unit ./internal/handler/ -run AdminModelCatalog -v`
Expected: FAIL

- [ ] **Step 3: 实现 handler**

```go
// GET /admin/catalog/config
func (h *AdminModelCatalogHandler) List(c *gin.Context) {
    ctx := c.Request.Context()
    _ = h.svc.EnsureFirstSeen(ctx, h.currentModelKeys(ctx)) // h.currentModelKeys: 复用 ChannelService.ListAvailable 聚合所有 (Platform,ModelName)，与 public catalog 同源
    cfgs, err := h.svc.ListAllForAdmin(ctx)
    if err != nil { response.InternalError(c, "load catalog config failed"); return }
    if cfgs == nil { cfgs = []service.AdminCatalogConfig{} }
    response.Success(c, cfgs)
}

// PUT /admin/catalog/config
func (h *AdminModelCatalogHandler) Update(c *gin.Context) {
    var req []service.AdminCatalogConfig
    if err := c.ShouldBindJSON(&req); err != nil { response.BadRequest(c, "invalid payload"); return }
    if err := h.svc.BatchSave(c.Request.Context(), req); err != nil { response.InternalError(c, "save catalog config failed"); return }
    response.Success(c, gin.H{"updated": len(req)})
}
```
> `ModelCatalogService` 需补 `ListAllForAdmin` / `BatchSave`（薄封装 repo，保持 handler 不碰 repository）。

- [ ] **Step 4: 注册路由**

在 admin 路由组（`requireAuth` + admin 权限中间件之后）加：
```go
admin.GET("/catalog/config", h.AdminModelCatalog.List)
admin.PUT("/catalog/config", h.AdminModelCatalog.Update)
```

- [ ] **Step 5: 运行确认通过 + lint**

Run: `cd backend && go test -tags=unit ./internal/handler/ -run AdminModelCatalog -v && golangci-lint run ./internal/handler/...`
Expected: PASS（depguard 不违规）

- [ ] **Step 6: Commit**

```bash
git add backend/internal/handler/admin_model_catalog_handler.go backend/internal/handler/admin_model_catalog_handler_test.go backend/internal/server/routes/
git commit -m "feat(catalog): admin endpoints for catalog config"
```

---

### Task A7: 前端排序逻辑 + 权重常量

**Files:**
- Modify: `frontend/src/utils/modelCatalog.ts`（`compareCatalogCards:312`、`buildModelCatalog` 排序选项 ~:107、类型 `ModelCatalogSort`）
- Create: `frontend/src/utils/__tests__/modelCatalog.spec.ts`（vitest，若无则仿现有 util 测试位置）

**Interfaces:**
- Consumes: 响应项新增 `pinned/sort_weight/tags/is_new/featured`
- Produces: `ModelCatalogSort` 加 `'recommended'`；`compareCatalogCards` 支持 recommended；导出 `TAG_WEIGHTS` 常量

- [ ] **Step 1: 写失败测试**

```ts
// compareCatalogCards recommended:
// 1) pinned true 排在 pinned false 前；pinned 区内按 sort_weight desc
// 2) 非 pinned：tag 权重累加大者在前（new+multimodal=140 > featured=80）
// 3) 权重并列 → modelPriceScore 升序 → name
describe('recommended sort', () => { /* 构造卡片断言上述 3 条 */ })
```

- [ ] **Step 2: 运行确认失败**

Run: `cd frontend && pnpm vitest run src/utils/__tests__/modelCatalog.spec.ts`
Expected: FAIL

- [ ] **Step 3: 实现**

```ts
// frontend/src/utils/modelCatalog.ts
export type ModelCatalogSort = 'recommended' | 'price' | 'name' | 'provider'

export const TAG_WEIGHTS: Record<string, number> = {
  new: 100, featured: 80, recommended: 60, multimodal: 40,
  reasoning: 20, coding: 15, longContext: 10, fast: 5, lowCost: 5,
}

export function tagScore(tags: string[] = []): number {
  return tags.reduce((s, t) => s + (TAG_WEIGHTS[t] ?? 0), 0)
}

function compareCatalogCards(a: ModelCatalogCard, b: ModelCatalogCard, sortBy: ModelCatalogSort): number {
  if (sortBy === 'recommended') {
    // pinned 区优先
    if (a.pinned !== b.pinned) return a.pinned ? -1 : 1
    if (a.pinned && b.pinned) return b.sort_weight - a.sort_weight
    const ds = tagScore(b.tags) - tagScore(a.tags)
    if (ds !== 0) return ds
    // 落到 price 兜底
  }
  if (sortBy === 'recommended' || sortBy === 'price') {
    const delta = modelPriceScore(a.pricing) - modelPriceScore(b.pricing)
    if (delta !== 0) return delta
  }
  if (sortBy === 'provider') {
    const d = a.provider.localeCompare(b.provider)
    if (d !== 0) return d
  }
  return a.name.localeCompare(b.name)
}
```
同步更新 `ModelCatalogCard` 类型加 `pinned: boolean; sort_weight: number; tags: string[]; is_new: boolean; featured: boolean`；`buildModelCatalog` 排序选项前置 `'recommended'`。

- [ ] **Step 4: 运行确认通过**

Run: `cd frontend && pnpm vitest run src/utils/__tests__/modelCatalog.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/utils/modelCatalog.ts frontend/src/utils/__tests__/modelCatalog.spec.ts
git commit -m "feat(catalog): recommended sort + tag weights"
```

---

### Task A8: 广场页默认 recommended + ModelCard badge

**Files:**
- Modify: `frontend/src/views/public/ModelCatalogView.vue:83`（默认 sortBy）、排序下拉 i18n
- Modify: `frontend/src/components/models/ModelCard.vue`（badge）
- Modify: `frontend/src/i18n/locales/{zh,en}/custom.ts`

**Interfaces:**
- Consumes: Task A7 的 `'recommended'` + 卡片新字段

- [ ] **Step 1: 改默认排序**

`ModelCatalogView.vue:83`：`sortBy: 'price'` → `sortBy: 'recommended'`。排序下拉选项加「推荐 / recommended」（i18n key `modelCatalog.sortRecommended`）。

- [ ] **Step 2: ModelCard badge**

`ModelCard.vue`：根据 `is_new`/`featured`/`tags` 渲染 badge（`NEW` / `特色` / `推荐`），pinned 显示置顶角标。运营标签与自动标签视觉区分（运营=强调色，自动=灰 chip，复用现有 `ModelCapabilityTags`）。

- [ ] **Step 3: i18n 补缺**

`custom.ts`（zh/en）加 `modelCatalog.sortRecommended`、`modelCatalog.badgeNew/Featured/Recommended`、`modelCatalog.pinned` 等 key（深合并，不改 main）。

- [ ] **Step 4: typecheck + lint + vitest**

Run: `cd frontend && pnpm run typecheck && pnpm run lint:check && pnpm vitest run`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/views/public/ModelCatalogView.vue frontend/src/components/models/ModelCard.vue frontend/src/i18n/locales/
git commit -m "feat(catalog): default recommended sort + model badges"
```

---

### Task A9: 管理页 `/admin/catalog` + api + router

**Files:**
- Create: `frontend/src/views/admin/CatalogManageView.vue`
- Create: `frontend/src/api/adminCatalog.ts`
- Modify: `frontend/src/router/index.ts`（admin 路由 + 侧边栏）
- Modify: `frontend/src/i18n/locales/{zh,en}/custom.ts`

**Interfaces:**
- Consumes: Task A6 的 `GET/PUT /admin/catalog/config`

- [ ] **Step 1: api client**

```ts
// frontend/src/api/adminCatalog.ts
export interface CatalogConfigItem {
  platform: string; model_name: string; pinned: boolean; sort_weight: number
  custom_tags: string[]; featured_until: string | null; hidden: boolean; first_seen_at: string
  tags?: string[]; is_new?: boolean; featured?: boolean
}
export const getCatalogConfig = () => apiClient.get<CatalogConfigItem[]>('/admin/catalog/config').then(r => r.data)
export const saveCatalogConfig = (cfgs: CatalogConfigItem[]) => apiClient.put('/admin/catalog/config', cfgs)
```

- [ ] **Step 2: 管理页**

`CatalogManageView.vue`：
- 顶部「新模型窗口」`new_model_days` 输入（调 `/admin/settings`）。
- **置顶区**（`pinned=true`）：可拖拽（`vuedraggable` 或项目现有拖拽组件；拖完按序赋 `sort_weight` 100/99/98…，调 save）。
- **全部模型列表**：每项切换 pinned、勾选 custom_tags（特色/推荐）、设 featured_until（日期选择器）、切换 hidden；只读展示自动 tags。
- 加载时调 `getCatalogConfig`（后端已 EnsureFirstSeen）。

- [ ] **Step 3: 路由 + 菜单 + i18n**

`router/index.ts` admin 区加 `{ path: '/admin/catalog', component: CatalogManageView, meta:{requiresAuth,admin} }`；侧边栏菜单项（`nav.adminCatalog` = 模型广场）。i18n 补缺。

- [ ] **Step 4: typecheck + lint**

Run: `cd frontend && pnpm run typecheck && pnpm run lint:check`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/views/admin/CatalogManageView.vue frontend/src/api/adminCatalog.ts frontend/src/router/index.ts frontend/src/i18n/locales/
git commit -m "feat(catalog): admin catalog management page"
```

---

### Task A10: 集成测试 + 全量验证

**Files:**
- Modify/Create: `backend/internal/service/model_catalog_service_integration_test.go`（`-tags=integration`，Postgres）

- [ ] **Step 1: 集成测试**

```go
// UpsertMissing 只插缺失不覆盖（first_seen_at 不变）
// BatchUpsert 事务 + unique 键冲突处理
// MergeDisplayConfig 端到端：构造 channel → catalog → 配置 → 验证 hidden 过滤/标签/排序字段
```
（Postgres 测试隔离按项目惯例：每测试独立 schema，参考 `payment_order_upgrade_test` / bundle lease CAS 测试的 schema 隔离写法）

- [ ] **Step 2: 跑集成**

Run: `cd backend && go test -tags=integration ./internal/service/ -run ModelCatalog -v`
Expected: PASS

- [ ] **Step 3: 全量后端 + 前端验证**

Run: `cd backend && golangci-lint run ./... && go test -tags=unit ./...`
Run: `cd frontend && pnpm run typecheck && pnpm run lint:check && pnpm vitest run`
Expected: 全 PASS

- [ ] **Step 4: 手动验收**

起服务，进 `/admin/catalog`：拖拽置顶一个模型 → 广场该模型排第一；给模型打「特色」→ 广场靠前；隐藏一个模型 → 广场不见。关 `ops_enabled` → 广场回退价格排序、无 badge。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/model_catalog_service_integration_test.go
git commit -m "test(catalog): integration tests for model catalog ops"
```

---

# Part B — 套餐展示（顶部独立套餐区）

前置：Part A 完成（共享开关 `ops_enabled` + 广场页结构）。Part B 完成后，广场顶部展示在售套餐卡，点击跳 `/bundles`。

### Task B1: 公开套餐接口 + DTO 裁剪 + platform 聚合

**Files:**
- Create: `backend/internal/handler/public_bundle_plan_handler.go`
- Create: `backend/internal/handler/public_bundle_plan_handler_test.go`
- Modify: `backend/internal/server/routes/public_models.go`（public 路由组加 `GET /public/bundles/plans`）

**Interfaces:**
- Consumes: `bundle_plan_service.ListForSale`（现有）
- Produces:
  - `GET /api/v1/public/bundles/plans` → `[]PublicBundlePlan`
  - `type PublicBundlePlan struct { Name, Tier, Description string; Price, OriginalPrice float64; Currency string; ValidityDays int; Features []string; SortOrder int; Platforms []string }`

- [ ] **Step 1: 写失败测试**

```go
// 只返回 active+for_sale；DTO 无 group_id/group_name/额度数字/concurrency/rpm
// Platforms = group_quotas 的 group_platform 去重（如 [openai,anthropic,gemini]）
// 无套餐 → []（非 null）；按 sort_order 升序
```

- [ ] **Step 2: 运行确认失败**

Run: `cd backend && go test -tags=unit ./internal/handler/ -run PublicBundlePlan -v`
Expected: FAIL

- [ ] **Step 3: 实现 handler**

```go
func (h *PublicBundlePlanHandler) List(c *gin.Context) {
    // ops_enabled=false 时不暴露套餐（开关由后端统一控制，前端无需读）。
    // 注：PublicBundlePlanHandler 需注入 service.ModelCatalogService（构造函数加参数 + Wire）。
    if !h.modelCatalogSvc.IsEnabled(c.Request.Context()) {
        response.Success(c, []PublicBundlePlan{})
        return
    }
    plans, err := h.bundlePlanSvc.ListForSale(c.Request.Context())
    if err != nil { response.InternalError(c, ...); return }
    out := make([]PublicBundlePlan, 0, len(plans))
    for _, p := range plans {
        platforms := dedupPlatforms(p.GroupQuotas) // 从 enrichGroupQuotas 回填的 group_platform 去重
        out = append(out, PublicBundlePlan{
            Name: p.Name, Tier: p.Tier, Description: p.Description,
            Price: p.Price, OriginalPrice: p.OriginalPrice, Currency: p.Currency,
            ValidityDays: p.ValidityDays, Features: p.Features, SortOrder: p.SortOrder,
            Platforms: platforms,
        })
    }
    response.Success(c, out) // 空时已是 []（make 初始化）
}
```
> 关键：裁剪掉 `GroupID/GroupName/*LimitUSD/*LimitCount/ConcurrencyLimit/RPMLimit`。`dedupPlatforms` 遍历 `GroupQuotas` 取 `GroupPlatform` 去重。

- [ ] **Step 4: 注册路由（无鉴权组）**

`routes/public_models.go` 的 `public` 组加：
```go
bundles := public.Group("/bundles")
bundles.GET("/plans", h.PublicBundlePlan.List)
```

- [ ] **Step 5: 运行确认通过 + lint**

Run: `cd backend && go test -tags=unit ./internal/handler/ -run PublicBundlePlan -v && golangci-lint run ./internal/handler/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/handler/public_bundle_plan_handler.go backend/internal/handler/public_bundle_plan_handler_test.go backend/internal/server/routes/public_models.go
git commit -m "feat(catalog): public bundle plans endpoint"
```

---

### Task B2: 前端公共套餐卡组件抽取

**Files:**
- Create: `frontend/src/components/bundles/BundlePlanCard.vue`
- Modify: `frontend/src/views/user/BundlesView.vue`（改用公共组件，行为不变）

**Interfaces:**
- Consumes: 套餐数据（`PublicBundlePlan` 或现有 `BundlePlan` 类型）
- Produces: `<BundlePlanCard :plan=... @click=... />`，供广场 + `/bundles` 共用

- [ ] **Step 1: 抽取组件**

把 `BundlesView.vue` 里的套餐卡（tier 主题色 via `getTierTheme`、price+划线价+折扣、features 打勾、group/platform chips）抽成 `BundlePlanCard.vue`，props：`plan` + 可选 `showActions`。`BundlesView` 改用该组件（保持原交互）。

- [ ] **Step 2: 验证 `/bundles` 不回归**

Run: `cd frontend && pnpm run typecheck && pnpm run lint:check && pnpm vitest run`
Expected: PASS（BundlesView 行为不变）

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/bundles/BundlePlanCard.vue frontend/src/views/user/BundlesView.vue
git commit -m "refactor(bundles): extract BundlePlanCard shared component"
```

---

### Task B3: 广场顶部套餐区 + 集成 + 开关联动

**Files:**
- Create: `frontend/src/api/publicBundles.ts`
- Create: `frontend/src/components/models/BundleShowcaseSection.vue`
- Modify: `frontend/src/views/public/ModelCatalogView.vue`（顶部挂套餐区 + 并行加载 + 开关）
- Modify: `frontend/src/i18n/locales/{zh,en}/custom.ts`

**Interfaces:**
- Consumes: Task B1 接口、Task B2 `BundlePlanCard`、`ops_enabled` 开关（前端读系统设置或独立 `/public/feature-flags`；若无可由后端 catalog 接口附带返回 `ops_enabled`，二选一，落地时定）

- [ ] **Step 1: api client**

```ts
// frontend/src/api/publicBundles.ts
export interface PublicBundlePlan {
  name: string; tier: string; description: string; price: number; original_price: number
  currency: string; validity_days: number; features: string[]; sort_order: number; platforms: string[]
}
export const getPublicBundlePlans = (signal?: AbortSignal) =>
  apiClient.get<PublicBundlePlan[]>('/public/bundles/plans', { signal }).then(r => r.data)
```

- [ ] **Step 2: 套餐区组件**

`BundleShowcaseSection.vue`：渲染套餐卡列表（复用 `BundlePlanCard`），卡片点击 `router.push('/bundles')`（未登录由 `/bundles` 的 `requireAuth` 拦截→登录回跳）。空数据不渲染整区。

- [ ] **Step 3: 广场页集成**

`ModelCatalogView.vue`：
- 顶部条件渲染 `<BundleShowcaseSection v-if="showBundleSection" :plans="bundlePlans" />`
- `showBundleSection` = `bundlePlans.length > 0`（**无需前端读 ops_enabled**：开关关闭时后端 `GET /public/bundles/plans` 已返回 `[]`（B1 的 IsEnabled 检查），区自然不渲染；排序开关也由后端 merge 跳过控制）
- `onMounted` 并行 `Promise.all([getCatalog(), getPublicBundlePlans()])`

- [ ] **Step 4: i18n 补缺**（`modelCatalog.bundleSectionTitle` 等）

- [ ] **Step 5: typecheck + lint + vitest**

Run: `cd frontend && pnpm run typecheck && pnpm run lint:check && pnpm vitest run`
Expected: PASS

- [ ] **Step 6: 手动验收 + Commit**

验收：广场顶部出现套餐卡 → 点击跳 `/bundles`（未登录弹登录）；`ops_enabled=false` → 套餐区消失。
```bash
git add frontend/src/api/publicBundles.ts frontend/src/components/models/BundleShowcaseSection.vue frontend/src/views/public/ModelCatalogView.vue frontend/src/i18n/locales/
git commit -m "feat(catalog): bundle showcase section on model catalog"
```

---

## 执行顺序与依赖

```
A1 → A2 → A3 → A4 → A5 → A6 → A7 → A8 → A9 → A10   (Part A，可独立上线)
                                                   ↓
                                                   B1 → B2 → B3   (Part B，依赖 A 的开关/广场页)
```

- Part A 是独立可交付里程碑（排序运营上线）。
- Part B 在 Part A 之后，复用开关与广场页结构。
- 每个任务结束即一次 commit，可随时回滚。
