# 模型广场运营优化 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: 用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现。步骤用 checkbox(`- [ ]`) 跟踪。

**Goal:** 把模型广场 NEW/featured 改为 admin 手动可控的持久化开关、统一全量拖拽排序，并修复公开页筛选失效 bug。

**Architecture:** `model_catalog_displays` 表新增 `is_new`/`featured` 两个 bool 列（手动开关），下线 `first_seen_at` 时间窗自动 NEW 逻辑；admin 合并为单一拖拽列表写 `sort_weight`；公开页排序尊重 `sort_weight`、筛选 `reactive→ref` 修复响应式。

**Tech Stack:** Go 1.26 + Ent + Gin（后端）；Vue 3 + TS + vitest（前端）；PostgreSQL 迁移。

## Global Constraints

- 后端分层强约束：handler 不 import repository/gorm/redis；service 不 import repository（depguard 强制）。adapter 写在 repository 包内做 DTO 互转。
- Ent schema 改动后必须 `cd backend && go generate ./ent` 并提交生成代码。
- 迁移文件幂等（`ADD COLUMN IF NOT EXISTS`），文件头 `SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='10min';`；迁移一旦应用不可修改（SHA256 锁定）。下一序号 = **177**。
- Interface 方法签名变更后，所有 `type.*Stub*` / `type.*Mock*` 实现必须补全，否则编译失败。
- API 列表端点空结果返回 `[]` 非 `null`。
- 前端包管理用 **pnpm**；`pnpm.overrides` 安全补丁块禁止删除（跑完 pnpm 命令若 lockfile 被改，`git restore pnpm-lock.yaml`）。
- i18n 新增 key 一律在 `i18n/locales/{zh,en}/custom.ts` 深合并补缺，不改 main。
- 设计依据：`docs/designs/2026-07-30-model-catalog-ops-design.md`。

---

## 文件结构

**后端**
- 改 `backend/ent/schema/model_catalog_display.go`（+is_new/+featured）
- 建迁移 `backend/migrations/177_model_catalog_manual_flags.sql`
- 改 `backend/internal/service/model_catalog_service.go`（DTO/classify/merge/BatchSave/接口）
- 改 `backend/internal/service/model_catalog_service_test.go`（改写 4 测试 + 去 newDays）
- 改 `backend/internal/repository/model_catalog_repo.go`（DTO/BatchUpsert/entTo…）
- 改 `backend/internal/repository/model_catalog_service_adapter.go`（双向映射）
- 改对应 repo/adapter/integration 测试
- `backend/internal/handler/public_model_catalog_handler.go`（mergeCatalog 透传，改动小）
- 改 `backend/internal/service/setting_features.go` 仅在需要时（保留 `GetModelCatalogNewModelDays` 方法不删，仅从接口移除）

**前端**
- 改 `frontend/src/views/public/ModelCatalogView.vue`（filters reactive→ref）
- 改 `frontend/src/utils/modelCatalog.ts`（`compareCatalogCards` 尊重 sort_weight）+ 其 `__tests__/modelCatalog.spec.ts`
- 改 `frontend/src/api/adminCatalog.ts`（`CatalogConfigItem` is_new/featured 可写）
- 改 `frontend/src/views/admin/CatalogManageView.vue`（统一列表 + 手动开关 + 移除 new_model_days）
- 改 `frontend/src/i18n/locales/{zh,en}/custom.ts`（新增 key）

---

## Task 1: 后端 ent schema + 迁移 177（is_new / featured）

**Files:**
- Modify: `backend/ent/schema/model_catalog_display.go`
- Create: `backend/migrations/177_model_catalog_manual_flags.sql`

**Interfaces:**
- Produces: ent 实体 `ModelCatalogDisplay` 新增 `IsNew bool`/`Featured bool` 字段（默认 false），供 Task 2/3 的 service/repo 使用。生成后 ent 提供 `SetIsNew`/`SetFeatured`/`UpdateIsNew`/`UpdateFeatured` 等。

- [ ] **Step 1: 改 ent schema**

在 `field.Bool("hidden")` 之后、`field.Time("first_seen_at")` 之前插入两行：

```go
		field.Bool("is_new").Default(false),
		field.Bool("featured").Default(false),
```

- [ ] **Step 2: 重新生成 ent**

```bash
cd backend && go generate ./ent
```

预期：`backend/ent/modelcatalogdisplay/` 下生成 `IsNew`/`Featured` 常量与 setter；`backend/ent/migrate/schema.go` 的 `model_catalog_displays` 表定义新增两列。

- [ ] **Step 3: 建迁移文件**

创建 `backend/migrations/177_model_catalog_manual_flags.sql`：

```sql
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE model_catalog_displays ADD COLUMN IF NOT EXISTS is_new BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE model_catalog_displays ADD COLUMN IF NOT EXISTS featured BOOLEAN NOT NULL DEFAULT FALSE;

-- 平滑回填：旧 featured_until 未到期的行置 featured=true，避免升级丢标签
UPDATE model_catalog_displays
SET featured = TRUE
WHERE featured_until IS NOT NULL AND featured_until > now();
```

- [ ] **Step 4: 迁移可用性验证（fresh postgres 容器）**

按 `backend/migrations/README.md` 的方式起一个一次性 `postgres:18-alpine` 容器、预建 `schema_migrations` 表、从零按序跑全部迁移（每个文件仅一次）。确认 177 成功应用、`model_catalog_displays` 含 `is_new`/`featured` 列。

> 注意：testcontainers 集成测试本地受迁移 150 阻塞（见 memory），此步用独立 postgres 容器验证迁移，不要与 `-tags=integration` 混淆。

- [ ] **Step 5: 编译**

```bash
cd backend && go build ./...
```

预期：通过（此时 service/repo 尚未使用新字段，但 ent 已生成）。

- [ ] **Step 6: Commit**

```bash
git add backend/ent/schema/model_catalog_display.go backend/ent backend/migrations/177_model_catalog_manual_flags.sql
git commit -m "feat(catalog): add is_new/featured columns to model_catalog_displays"
```

---

## Task 2: 后端 service 层——手动 NEW/featured（TDD）

**Files:**
- Test: `backend/internal/service/model_catalog_service_test.go`
- Modify: `backend/internal/service/model_catalog_service.go`

**Interfaces:**
- Consumes: Task 1 的 ent `IsNew`/`Featured` 字段。
- Produces: `ModelCatalogDisplay`（service DTO）与 `AdminCatalogConfig` 含 `IsNew`/`Featured` 可写字段；`classifyDisplay`/`mergeDisplay` 不再依赖 `newModelDays`；`modelCatalogSettings` 接口移除 `GetModelCatalogNewModelDays`。

- [ ] **Step 1: 改测试——移除 newDays，改写 4 个用例**

(a) `stubCatalogSettings` 去掉 `newDays` 字段与 `GetModelCatalogNewModelDays` 方法：

```go
type stubCatalogSettings struct {
	enabled bool
}

func (s stubCatalogSettings) IsModelCatalogOpsEnabled(_ context.Context) bool { return s.enabled }
```

`newSvc` 收掉 newDays 参数：

```go
func newSvc(repo *stubCatalogRepo, enabled bool) ModelCatalogService {
	return NewModelCatalogService(repo, stubCatalogSettings{enabled: enabled})
}
```

把文件中**所有** `newSvc(..., true, 30)` / `newSvc(..., false, 30)` 调用改为 `newSvc(..., true)` / `newSvc(..., false)`（约 9 处：IsEnabled×2、DisabledNoOp、HiddenFiltered、FeaturedExpiry、NewWindow、TagsMergeDedup、DegradeOnRepoError、EmptyInput、EnsureFirstSeen、BatchSave、ListAllForAdmin）。

(b) 把 `TestModelCatalogService_MergeDisplayConfig_FeaturedExpiry` 整体替换为：

```go
func TestModelCatalogService_MergeDisplayConfig_FeaturedManualAndExpiry(t *testing.T) {
	now := time.Now()
	future := now.Add(1 * time.Hour)
	past := now.Add(-1 * time.Hour)

	repo := &stubCatalogRepo{getByModelKeys: map[string]*ModelCatalogDisplay{
		modelCatalogMapKey("p", "on"):      {Platform: "p", ModelName: "on", Featured: true, FeaturedUntil: &future},
		modelCatalogMapKey("p", "expired"): {Platform: "p", ModelName: "expired", Featured: true, FeaturedUntil: &past},
		modelCatalogMapKey("p", "forever"): {Platform: "p", ModelName: "forever", Featured: true},
	}}
	svc := newSvc(repo, true)

	out, _ := svc.MergeDisplayConfig(context.Background(), []CatalogItem{
		{Platform: "p", ModelName: "on"},
		{Platform: "p", ModelName: "expired"},
		{Platform: "p", ModelName: "forever"},
	})
	byName := map[string]*CatalogDisplayInfo{}
	for _, it := range out {
		byName[it.ModelName] = it.Display
	}
	if !byName["on"].Featured {
		t.Fatal("featured=true with future until should be Featured")
	}
	if byName["expired"].Featured {
		t.Fatal("featured=true with past until should not be Featured")
	}
	if contains(byName["expired"].Tags, "featured") {
		t.Fatal("expired tags must not contain featured")
	}
	if !byName["forever"].Featured {
		t.Fatal("featured=true with nil until should be Featured")
	}
}
```

(c) 把 `TestModelCatalogService_MergeDisplayConfig_NewWindow` 整体替换为：

```go
func TestModelCatalogService_MergeDisplayConfig_NewManual(t *testing.T) {
	repo := &stubCatalogRepo{getByModelKeys: map[string]*ModelCatalogDisplay{
		modelCatalogMapKey("p", "flagged"): {Platform: "p", ModelName: "flagged", IsNew: true},
		// first_seen_at 不再驱动 NEW：即便刚登记，IsNew 仍为 false
		modelCatalogMapKey("p", "off"): {Platform: "p", ModelName: "off", FirstSeenAt: time.Now()},
	}}
	svc := newSvc(repo, true)

	out, _ := svc.MergeDisplayConfig(context.Background(), []CatalogItem{
		{Platform: "p", ModelName: "flagged"},
		{Platform: "p", ModelName: "off"},
	})
	byName := map[string]*CatalogDisplayInfo{}
	for _, it := range out {
		byName[it.ModelName] = it.Display
	}
	if !byName["flagged"].IsNew {
		t.Fatal("IsNew=true should surface")
	}
	if !contains(byName["flagged"].Tags, "new") {
		t.Fatal("flagged tags should contain new")
	}
	if byName["off"].IsNew {
		t.Fatal("first_seen_at must not drive IsNew anymore")
	}
	if contains(byName["off"].Tags, "new") {
		t.Fatal("off tags must not contain new")
	}
}
```

(d) 改 `TestModelCatalogService_ListAllForAdmin_DerivedFields` 的 listResult 第一行：去掉对 `FirstSeenAt`/`FeaturedUntil` 派生的依赖，直接给手动布尔：

```go
	repo := &stubCatalogRepo{listResult: []*ModelCatalogDisplay{
		{Platform: "p", ModelName: "star", CustomTags: []string{"recommended"}, IsNew: true, Featured: true, FeaturedUntil: &future, Pinned: true, SortWeight: 9},
		{Platform: "p", ModelName: "hidden", Hidden: true},
	}}
```

并把断言里的变量名 `fresh`→`star`（行为不变：`IsNew`/`Featured` 为真，tags=`["recommended","new","featured"]`）。

(e) 改 `TestModelCatalogService_BatchSave_DropsFirstSeenAt` 的 cfgs 增加 `IsNew: true, Featured: true`，并在断言里增加：

```go
	if !row.IsNew || !row.Featured {
		t.Fatalf("IsNew/Featured must be forwarded: %+v", row)
	}
```

- [ ] **Step 2: 运行测试，确认失败**

```bash
cd backend && go test -tags=unit ./internal/service/ -run 'TestModelCatalogService' -v
```

预期：编译失败（`ModelCatalogDisplay` 无 `IsNew`/`Featured` 字段；`newSvc` 签名不匹配；接口缺方法）。

- [ ] **Step 3: 改 service 源码**

(a) `ModelCatalogDisplay`（service 内部 DTO，约 line 56-65）加两字段：

```go
type ModelCatalogDisplay struct {
	Platform      string
	ModelName     string
	Pinned        bool
	SortWeight    int
	CustomTags    []string
	FeaturedUntil *time.Time
	Hidden        bool
	IsNew         bool
	Featured      bool
	FirstSeenAt   time.Time
}
```

(b) `AdminCatalogConfig`（约 line 40-52）：`IsNew`/`Featured` 保持 json tag，但现在由持久化字段驱动（BatchSave 会写回）；注释由"派生"改为"持久化（手动）"。

(c) `modelCatalogSettings` 接口移除 `GetModelCatalogNewModelDays`：

```go
type modelCatalogSettings interface {
	IsModelCatalogOpsEnabled(ctx context.Context) bool
}
```

(d) `MergeDisplayConfig`：删除 `newModelDays := s.settings.GetModelCatalogNewModelDays(ctx)` 一行；`mergeDisplay` 调用去掉 newModelDays 实参。

(e) `ListAllForAdmin`：删除 `newModelDays := ...` 一行；`displayToAdminConfig` 调用去掉实参。

(f) `mergeDisplay` 签名改为 `func mergeDisplay(now time.Time, it CatalogItem, cfg *ModelCatalogDisplay) *CatalogDisplayInfo`，内部 `isNew, featured := classifyDisplay(now, cfg)`。

(g) `displayToAdminConfig` 签名改为 `func displayToAdminConfig(now time.Time, r *ModelCatalogDisplay) AdminCatalogConfig`。

(h) `classifyDisplay` 改为：

```go
// classifyDisplay 判定 new/featured：
//   - new：手动开关 cfg.IsNew（first_seen_at 不再驱动）
//   - featured：手动开关 cfg.Featured，且（无 featured_until 或 featured_until 未过期）
func classifyDisplay(now time.Time, cfg *ModelCatalogDisplay) (isNew bool, featured bool) {
	if cfg == nil {
		return false, false
	}
	isNew = cfg.IsNew
	featured = cfg.Featured
	if featured && cfg.FeaturedUntil != nil && now.After(*cfg.FeaturedUntil) {
		featured = false
	}
	return isNew, featured
}
```

(i) `BatchSave` 的行映射增加两字段：

```go
		rows = append(rows, &ModelCatalogDisplay{
			Platform:      c.Platform,
			ModelName:     c.ModelName,
			Pinned:        c.Pinned,
			SortWeight:    c.SortWeight,
			CustomTags:    c.CustomTags,
			FeaturedUntil: c.FeaturedUntil,
			Hidden:        c.Hidden,
			IsNew:         c.IsNew,
			Featured:      c.Featured,
		})
```

- [ ] **Step 4: 运行测试，确认通过**

```bash
cd backend && go test -tags=unit ./internal/service/ -v
```

预期：PASS。

- [ ] **Step 5: 确认接口实现仍满足**

`*SettingService` 仍实现 `modelCatalogSettings`（仅需 `IsModelCatalogOpsEnabled`）。若 `GetModelCatalogNewModelDays` 方法在 `setting_features.go` 已无调用方，保留无害（公共方法不会被 lint 判 unused）。

- [ ] **Step 6: Commit**

```bash
git add backend/internal/service/model_catalog_service.go backend/internal/service/model_catalog_service_test.go
git commit -m "feat(catalog): manual is_new/featured flags in service layer"
```

---

## Task 3: 后端 repo + adapter 持久化新字段

**Files:**
- Modify: `backend/internal/repository/model_catalog_repo.go`
- Modify: `backend/internal/repository/model_catalog_service_adapter.go`
- Test: `backend/internal/repository/model_catalog_repo_test.go`、`model_catalog_service_adapter_test.go`、`model_catalog_integration_test.go`

**Interfaces:**
- Consumes: Task 1 ent setter；Task 2 service DTO `IsNew`/`Featured`。
- Produces: `BatchUpsert` 写入 `is_new`/`featured`；DTO 双向映射含两字段。

- [ ] **Step 1: 改 repo DTO**

`model_catalog_repo.go` 的 `ModelCatalogDisplay` struct（约 line 18-27）加：

```go
	IsNew       bool
	Featured    bool
```

- [ ] **Step 2: 改 BatchUpsert**

在 `builder` 链（约 line 147-153）增加 `.SetIsNew(c.IsNew).SetFeatured(c.Featured)`：

```go
			builder := client.ModelCatalogDisplay.Create().
				SetPlatform(c.Platform).
				SetModelName(c.ModelName).
				SetPinned(c.Pinned).
				SetSortWeight(c.SortWeight).
				SetCustomTags(c.CustomTags).
				SetIsNew(c.IsNew).
				SetFeatured(c.Featured).
				SetHidden(c.Hidden)
```

在 `Update(func(u *dbent.ModelCatalogDisplayUpsert) {...})`（约 line 160-169）的更新列表追加 `.UpdateIsNew().UpdateFeatured()`：

```go
					u.UpdatePinned().
						UpdateSortWeight().
						UpdateCustomTags().
						UpdateIsNew().
						UpdateFeatured().
						UpdateHidden()
```

- [ ] **Step 3: 改 entToModelCatalogDisplay**

（约 line 208-219）增加：

```go
		IsNew:       e.IsNew,
		Featured:    e.Featured,
```

- [ ] **Step 4: 改 adapter 双向映射**

`model_catalog_service_adapter.go` 的 `repoDisplayToService`（约 line 36-47）与 `serviceDisplayToRepo`（约 line 50-61）各加：

```go
		IsNew:       d.IsNew,
		Featured:    d.Featured,
```

- [ ] **Step 5: 更新 repo/adapter 测试断言**

在 `model_catalog_repo_test.go` 与 `model_catalog_service_adapter_test.go` 中所有构造 `ModelCatalogDisplay`/`service.ModelCatalogDisplay` 的用例，按需补 `IsNew`/`Featured` 字段；在 BatchUpsert 写回读回的断言里增加对两字段的校验，例如：

```go
	if got.IsNew != want.IsNew || got.Featured != want.Featured {
		t.Fatalf("is_new/featured roundtrip mismatch: got=%+v want=%+v", got, want)
	}
```

（实施时阅读这两个测试文件，把上述断言并入既有的字段比对处。）

- [ ] **Step 6: 编译 + 单测**

```bash
cd backend && go build ./... && go test -tags=unit ./internal/repository/ -run 'ModelCatalog' -v
```

预期：PASS。

- [ ] **Step 7: 集成测试（可选，受迁移 150 本地阻塞）**

`model_catalog_integration_test.go` 增加 is_new/featured 的端到端断言。本地若跑 `-tags=integration` 需临时处理迁移 150（见 memory），或在 CI 验证。

- [ ] **Step 8: public handler 透传确认**

`backend/internal/handler/public_model_catalog_handler.go` 的 `mergeCatalog`（约 line 136-190）已把 `Display.IsNew/Featured` 写入公开 DTO；新字段经由 Task 2 的 `CatalogDisplayInfo` 自动流通。**无需改代码**，仅 `go build ./...` 确认编译。

- [ ] **Step 9: Commit**

```bash
git add backend/internal/repository backend/internal/handler
git commit -m "feat(catalog): persist is_new/featured in repo + adapter"
```

---

## Task 4: 公开页筛选失效修复（frontend，独立）

**Files:**
- Modify: `frontend/src/views/public/ModelCatalogView.vue`
- Test: `pnpm exec vue-tsc --noEmit`（类型）+ 手动验证筛选交互

**Interfaces:** 无新接口。修复 `v-model:filters` 对 reactive 常量赋值的运行时崩溃。

- [ ] **Step 1: filters 改 ref**

`ModelCatalogView.vue` line 90：

```ts
const filters = ref<CatalogFilters>({
  search: '',
  platform: '',
  capability: '',
  billingMode: '',
  sortBy: 'recommended'
})
```

line 100：

```ts
const filteredModels = computed(() => filterModelCatalog(catalog.value.items, filters.value))
```

line 102-110 的 `watch` 内 `filters.sortBy` → `filters.value.sortBy`：

```ts
watch(
  () => catalog.value.facets.sortOptions,
  (options) => {
    if (!options.includes(filters.value.sortBy)) {
      filters.value.sortBy = options[0] || 'name'
    }
  },
  { immediate: true }
)
```

line 58 的 import 去掉未用的 `reactive`（仅保留 `computed, onMounted, onUnmounted, ref, watch`）。

模板 line 19-23 `v-model:filters="filters"` **保持不变**（ref 的 v-model setter 会正确赋值 `.value`）。

- [ ] **Step 2: 类型检查**

```bash
cd frontend && pnpm exec vue-tsc --noEmit
```

预期：无错误。（若 pnpm 触动 lockfile，跑完 `git restore pnpm-lock.yaml`。）

- [ ] **Step 3: 手动验证**

`cd frontend && pnpm dev`，打开 `/models`：搜索框输入、平台/能力/计费/排序下拉切换，列表应实时变化（修复前任何筛选都无效）。

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/public/ModelCatalogView.vue
git commit -m "fix(catalog): repair public model catalog filters (reactive→ref)"
```

---

## Task 5: 公开页排序尊重 sort_weight（TDD）

**Files:**
- Modify: `frontend/src/utils/modelCatalog.ts`
- Test: `frontend/src/utils/__tests__/modelCatalog.spec.ts`

**Interfaces:**
- Produces: `compareCatalogCards` 在 `recommended` 模式下，非置顶卡片也按 `sort_weight` 降序，再回退 `tagScore → 价格`。

- [ ] **Step 1: 写失败测试**

在 `modelCatalog.spec.ts` 增加（导入 `compareCatalogCards` 需先在 `modelCatalog.ts` 把它 `export`——见 Step 2）：

```ts
import { compareCatalogCards, type ModelCatalogCard } from '@/utils/modelCatalog'

const card = (over: Partial<ModelCatalogCard>): ModelCatalogCard => ({
  id: 'x', name: 'x', provider: 'p', description: '', platforms: [], status: '',
  pricing: null, health: null, capabilities: [], pinned: false, sort_weight: 0,
  tags: [], is_new: false, featured: false, ...over
})

describe('compareCatalogCards recommended', () => {
  it('置顶卡片始终在非置顶之前', () => {
    const a = card({ name: 'a', pinned: true })
    const b = card({ name: 'b', pinned: false })
    expect(compareCatalogCards(a, b, 'recommended')).toBeLessThan(0)
  })
  it('非置顶：sort_weight 高者在前', () => {
    const a = card({ name: 'a', sort_weight: 0 })
    const b = card({ name: 'b', sort_weight: 50 })
    expect(compareCatalogCards(a, b, 'recommended')).toBeGreaterThan(0)
  })
  it('非置顶 sort_weight 均为 0 时回退 tagScore', () => {
    const a = card({ name: 'a', sort_weight: 0, tags: ['recommended'] })
    const b = card({ name: 'b', sort_weight: 0, tags: ['multimodal'] })
    // recommended(60) > multimodal(40) → a 在前
    expect(compareCatalogCards(a, b, 'recommended')).toBeLessThan(0)
  })
})
```

- [ ] **Step 2: 运行测试，确认失败**

```bash
cd frontend && pnpm exec vitest run src/utils/__tests__/modelCatalog.spec.ts
```

预期：FAIL（`compareCatalogCards` 未导出，且非置顶未按 sort_weight 排序）。

- [ ] **Step 3: 导出并改写 compareCatalogCards**

`modelCatalog.ts` line 347：把 `function compareCatalogCards` 改为 `export function compareCatalogCards`，并把 `recommended` 分支改为统一按 sort_weight：

```ts
export function compareCatalogCards(a: ModelCatalogCard, b: ModelCatalogCard, sortBy: ModelCatalogSort): number {
  if (sortBy === 'recommended') {
    // 置顶整体浮顶；置顶与非置顶块内均按 sort_weight 降序，未被手动排过(0)回退 tagScore→价格。
    if (a.pinned !== b.pinned) return a.pinned ? -1 : 1
    const swDelta = b.sort_weight - a.sort_weight
    if (swDelta !== 0) return swDelta
    const tagDelta = tagScore(b.tags) - tagScore(a.tags)
    if (tagDelta !== 0) return tagDelta
  }
  if (sortBy === 'recommended' || sortBy === 'price') {
    const delta = modelPriceScore(a.pricing) - modelPriceScore(b.pricing)
    if (!Number.isNaN(delta) && delta !== 0) return delta
  }
  if (sortBy === 'provider') {
    const providerDelta = a.provider.localeCompare(b.provider)
    if (providerDelta !== 0) return providerDelta
  }
  return a.name.localeCompare(b.name)
}
```

- [ ] **Step 4: 运行测试，确认通过**

```bash
cd frontend && pnpm exec vitest run src/utils/__tests__/modelCatalog.spec.ts
```

预期：PASS（含既有用例）。

- [ ] **Step 5: Commit**

```bash
git add frontend/src/utils/modelCatalog.ts frontend/src/utils/__tests__/modelCatalog.spec.ts
git commit -m "feat(catalog): respect sort_weight for non-pinned public cards"
```

---

## Task 6: admin API 类型——is_new/featured 可写

**Files:**
- Modify: `frontend/src/api/adminCatalog.ts`

**Interfaces:**
- Produces: `CatalogConfigItem.is_new`/`featured` 为可写 `boolean`（去 `?`），供 Task 7 双向绑定。

- [ ] **Step 1: 改类型与注释**

把 `is_new?: boolean` → `is_new: boolean`，注释改为"手动 NEW 开关（持久化）"；
把 `featured?: boolean` → `featured: boolean`，注释改为"手动精选开关（持久化；可选 featured_until 到期自动隐藏）"。
保留 `featured_until: string | null`、`first_seen_at: string`（只读）、`tags?`（只读）。

- [ ] **Step 2: 类型检查**

```bash
cd frontend && pnpm exec vue-tsc --noEmit
```

预期：`CatalogManageView.vue` 可能因仍把 is_new/featured 当只读而暂不报错；本任务仅改类型。Commit 合并进 Task 7。

---

## Task 7: admin 页——统一拖拽列表 + 手动开关 + 移除 new_model_days

**Files:**
- Modify: `frontend/src/views/admin/CatalogManageView.vue`
- Modify: `frontend/src/i18n/locales/zh/custom.ts`、`en/custom.ts`（Task 8 的 key 此处使用）

**Interfaces:**
- Consumes: Task 6 的可写 `is_new`/`featured`；Task 5 的 `sort_weight` 语义；后端 Task 1-3 持久化。
- Produces: 单一可拖拽列表，写 `sort_weight`；NEW/featured 手动开关；移除 new_model_days 设置条与置顶独立区。

- [ ] **Step 1: 移除 new_model_days 顶部设置条**

删除模板 line 14-45 整个 `<section class="card">`（new_model_days）。删除 script 中 `newModelDays`/`settingsLoading`/`savingSettings`/`cachedSettings`/`saveNewModelDays` 及对 `getSettings`/`updateSettings` 的 import 与调用。`load()` 改为只 `getCatalogConfig()`：

```ts
async function load(): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    items.value = await getCatalogConfig()
    dirty.value = false
  } catch (err: unknown) {
    const message = (err as { message?: string })?.message
    loadError.value = message || t('admin.catalogManage.loadFailed')
  } finally {
    loading.value = false
  }
}
```

import 行去掉 `getSettings, updateSettings, type SystemSettings`。

- [ ] **Step 2: 合并置顶区为单一拖拽列表**

删除模板 line 62-116 的独立"置顶区" `<section>`。把 line 118 起的"全部模型列表" `<section>` 改造为：搜索时只读过滤列表，非搜索时 VueDraggable 全量列表。

模板骨架：

```html
<section class="card">
  <div class="flex flex-col gap-3 border-b border-gray-100 px-4 py-3 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
    <div>
      <h2 class="text-base font-semibold text-gray-900 dark:text-white">
        {{ t('admin.catalogManage.allModelsSection') }}
      </h2>
      <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.catalogManage.sortHint') }}
      </p>
    </div>
    <div class="flex items-center gap-2">
      <input v-model="search" type="text" :placeholder="t('admin.catalogManage.searchPlaceholder')" class="..." />
      <button type="button" class="btn btn-primary btn-sm ..." :class="{ 'opacity-60': !dirty }" :disabled="!dirty || saving" @click="saveAll">
        <span v-if="saving" class="..." />
        {{ saving ? t('admin.catalogManage.saving') : t('admin.catalogManage.saveAll') }}
      </button>
    </div>
  </div>

  <div v-if="visibleList.length === 0" class="py-10 text-center text-sm text-gray-400 dark:text-gray-500">
    {{ t('admin.catalogManage.empty') }}
  </div>

  <!-- 非搜索：全量可拖拽 -->
  <VueDraggable
    v-else-if="!search.trim()"
    v-model="localItems"
    :animation="200"
    handle=".catalog-drag-handle"
    class="divide-y divide-gray-100 dark:divide-dark-700"
    @end="onDragEnd"
  >
    <div v-for="item in localItems" :key="keyOf(item)" class="catalog-row ...">
      <!-- 行内容（见 Step 3） -->
    </div>
  </VueDraggable>

  <!-- 搜索态：只读过滤列表，不可拖拽 -->
  <ul v-else class="divide-y divide-gray-100 dark:divide-dark-700">
    <li v-for="item in filteredList" :key="keyOf(item)" class="catalog-row ...">
      <!-- 同一行内容，无拖拽手柄 -->
    </li>
  </ul>
</section>
```

- [ ] **Step 3: 行内控件（NEW/featured 手动开关 + 置顶 + recommended + 到期 + 隐藏）**

每行（拖拽态含手柄）：

```html
<span class="catalog-drag-handle ...">☜拖拽手柄svg☞</span>
<div class="min-w-[160px] flex-1">
  <div class="flex items-center gap-1.5">
    <span class="...">{{ item.model_name }}</span>
    <span v-if="item.is_new" class="...amber...">NEW</span>
    <span v-if="item.pinned" class="...teal...">{{ t('modelCatalog.pinned') }}</span>
  </div>
  <div class="text-xs text-gray-400">{{ item.platform }}</div>
</div>
<!-- 置顶 -->
<label class="..."><input type="checkbox" class="catalog-checkbox" :checked="item.pinned" @change="togglePin(item)" />{{ item.pinned ? t('admin.catalogManage.unpin') : t('admin.catalogManage.pin') }}</label>
<!-- NEW 手动开关（写 item.is_new） -->
<label class="..."><input type="checkbox" class="catalog-checkbox" :checked="item.is_new" @change="toggleFlag(item,'is_new')" />{{ t('admin.catalogManage.tagNew') }}</label>
<!-- featured 手动开关（写 item.featured） -->
<label class="...border-emerald..."><input type="checkbox" class="catalog-checkbox" :checked="item.featured" @change="toggleFlag(item,'featured')" />{{ t('admin.catalogManage.tagFeatured') }}</label>
<!-- recommended（仍写 custom_tags） -->
<label class="...border-indigo..."><input type="checkbox" class="catalog-checkbox" :checked="hasTag(item,'recommended')" @change="toggleTag(item,'recommended')" />{{ t('admin.catalogManage.tagRecommended') }}</label>
<!-- 可选精选到期 -->
<label class="...">{{ t('admin.catalogManage.colFeaturedUntil') }}
  <input type="date" :value="dateOf(item.featured_until)" class="..." @change="setFeaturedUntil(item, $event)" />
</label>
<!-- 隐藏 -->
<label class="..."><input type="checkbox" class="catalog-checkbox" :checked="item.hidden" @change="toggleHidden(item)" />{{ item.hidden ? t('admin.catalogManage.hidden') : t('admin.catalogManage.visible') }}</label>
<!-- 只读自动 tags -->
<div v-if="item.tags?.length" class="...">{{ tags chips }}</div>
```

- [ ] **Step 4: script 改造**

```ts
// 本地拖拽副本：全量、按 (pinned desc, sort_weight desc) 初始排序
const localItems = ref<CatalogConfigItem[]>([])
watch(
  items,
  (val) => {
    localItems.value = [...val].sort((a, b) => {
      if (a.pinned !== b.pinned) return a.pinned ? -1 : 1
      return b.sort_weight - a.sort_weight
    })
  },
  { immediate: true }
)

// 搜索态可见列表
const filteredList = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return items.value
  return items.value.filter(
    (i) => i.model_name.toLowerCase().includes(q) || i.platform.toLowerCase().includes(q),
  )
})
const visibleList = computed(() => (search.value.trim() ? filteredList.value : localItems.value))

// 布尔标志开关（is_new / featured）
function toggleFlag(item: CatalogConfigItem, key: 'is_new' | 'featured'): void {
  item[key] = !item[key]
  markDirty()
}

// togglePin 不再自动改 sort_weight（顺序由拖拽统一管理）；仅翻转 pinned
function togglePin(item: CatalogConfigItem): void {
  item.pinned = !item.pinned
  markDirty()
}

// 拖拽结束：按 localItems 可见顺序重写 sort_weight
function onDragEnd(): void {
  const total = localItems.value.length
  localItems.value.forEach((local, idx) => {
    const target = items.value.find((i) => keyOf(i) === keyOf(local))
    if (target) target.sort_weight = total - idx
  })
  markDirty()
}
```

删除旧 `pinnedItems`/`pinnedLocal`/`onPinnedDragEnd` 及对 `togglePin` 内 sort_weight 的旧逻辑。

- [ ] **Step 5: 类型检查 + lint**

```bash
cd frontend && pnpm exec vue-tsc --noEmit && pnpm exec eslint src/views/admin/CatalogManageView.vue
```

预期：通过。跑完 `git restore pnpm-lock.yaml`（若被改）。

- [ ] **Step 6: 手动验证**

`pnpm dev` → `/admin/catalog`：
- 拖拽任意模型换序 → "保存全部" → 刷新顺序保留。
- 勾选某模型 NEW → 保存 → `/models` 该模型显示 NEW 徽章。
- 勾选 featured（不填日期）→ 保存 → `/models` 显示"特色"徽章。
- 移除 new_model_days 设置条后页面仍正常加载。

- [ ] **Step 7: Commit**

```bash
git add frontend/src/views/admin/CatalogManageView.vue frontend/src/api/adminCatalog.ts frontend/src/i18n
git commit -m "feat(catalog): unified drag-sort + manual new/featured toggles in admin"
```

---

## Task 8: i18n 新增 key

**Files:**
- Modify: `frontend/src/i18n/locales/zh/custom.ts`、`frontend/src/i18n/locales/en/custom.ts`

- [ ] **Step 1: zh custom.ts 的 `admin.catalogManage` 下增加**

```ts
sortHint: '拖拽任意模型可调整顺序；置顶项在公开页始终置顶。保存全部后生效。',
tagNew: 'NEW',
```

（`en/custom.ts` 镜像：`sortHint: 'Drag any model to reorder; pinned models always lead on the public page. Takes effect after Save all.', tagNew: 'NEW'`。）

若 `newModelDays`/`newModelDaysHint`/`saveSettings`/`settingsSaved`/`settingsSaveFailed` 在移除设置条后无其它引用，可在 custom.ts 中保留（深合并补缺惯例：不改 main、不动既有 key，避免回归）。

- [ ] **Step 2: 类型检查**

```bash
cd frontend && pnpm exec vue-tsc --noEmit
```

- [ ] **Step 3: Commit**（合并进 Task 7 的 commit 或单独）

```bash
git add frontend/src/i18n
git commit -m "i18n(catalog): add sort hint + NEW toggle keys"
```

---

## Task 9: 全量校验

- [ ] **Step 1: 后端**

```bash
cd backend && golangci-lint run ./... && go test -tags=unit ./...
```

预期：lint 通过、单测全绿。集成测试在 CI 验证（本地受迁移 150 阻塞）。

- [ ] **Step 2: 前端**

```bash
cd frontend && pnpm exec vue-tsc --noEmit && pnpm exec eslint . && pnpm exec vitest run
```

预期：类型/lint/测试通过。`git restore pnpm-lock.yaml` 若被改。

- [ ] **Step 3: 端到端手测**

`make build` 或 dev 起服务，覆盖：admin 拖拽排序保存、NEW/featured 手动开关生效到公开页、公开页筛选全部生效、套餐区块（已跳过）。

---

## Self-Review

**Spec coverage：**
- 需求1 排序 → Task 1(sort_weight 列已存在)+Task 5(公开排序)+Task 7(admin 拖拽)。✅
- 需求1 NEW 手动 → Task 1(is_new 列)+Task 2(service)+Task 3(repo)+Task 6(类型)+Task 7(开关)+Task 2 移除时间窗。✅
- 需求2 筛选 → Task 4。✅ 模型差异 → 设计使然，admin 隐藏标签明示（无需任务）。✅
- 需求3 featured → Task 1(featured 列)+Task 2(service 统一)+Task 3(repo)+Task 6+Task 7(开关)，ModelCard 无需改。✅
- 需求4 套餐 → 跳过（已守卫）。✅

**Placeholder scan：** Task 3 Step 5 的 repo/adapter 测试断言以"阅读文件并入既有比对处"描述——给出了具体断言代码与字段名，非占位；其余步骤均含实码。✅

**Type consistency：** `IsNew`/`Featured`（Go）/ `is_new`/`featured`（TS json tag snake_case）跨层一致；`toggleFlag(item,'is_new'|'featured')` 与 `CatalogConfigItem` 字段名一致；`compareCatalogCards` 导出名与测试导入一致。✅
