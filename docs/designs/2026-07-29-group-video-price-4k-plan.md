# group 视频价 4K 字段 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给 group 视频价补齐 4K 每秒价字段，使 4K 视频在按秒（group 每秒价 + 渠道 per_second）与按次（渠道 video/image）两条计价路径下都正确计价。

**Architecture:** 13 层字段扩展（ent schema → 迁移 → repo → service → cache → handler → dto → 前端），核心是 `Group.GetVideoPrice("4k")` 从 `nil` 改为返回 `VideoPrice4K`，让 group 显式配 4K 时优先于渠道，未配回落渠道。计费优先级链与现有 480P/720P/1080P 完全一致。

**Tech Stack:** Go 1.26.4 / Ent ORM / PostgreSQL 迁移 / Vue 3 + TypeScript + pnpm

## Global Constraints

- 后端：Go 1.26.4，Ent ORM；**生产建表靠 `backend/migrations/*.sql`（`//go:embed` + 按序号应用），不是 ent auto-migrate**。
- 字段精度：`DECIMAL(20,8)`（对齐现有 `video_price_480p/720p/1080p`，迁移 `170`）。
- 前端：**必须用 pnpm**（不是 npm）；`video_price_4k` 与现有档位同口径（每秒价 USD/s）。
- `per_request` 模式**不纳入**视频计费入口（`isOpenAIGatewayService` 已排除，保持现状）。
- 4K 完全未配价时**不改默认价兜底**（保持 `getDefaultVideoPrice` 现状）。
- 计费优先级：group 视频价 > 渠道定价 > 系统默认。
- 工作目录：后端命令在 `backend/` 下执行；前端命令在 `frontend/` 下执行。
- commit message 结尾加 `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。

## File Structure

| 文件 | 职责 |
|---|---|
| `backend/ent/schema/group.go` | ent schema 源：新增 `video_price_4k` 字段定义 |
| `backend/ent/**` | `go generate` 自动产出（setter/where/mutation/migrate schema），不手改 |
| `backend/migrations/175_add_group_video_price_4k.sql` | 生产迁移：ADD COLUMN + COMMENT |
| `backend/internal/service/group.go` | `Group` struct 加字段 + `GetVideoPrice` 4K 语义反转 |
| `backend/internal/service/billing_service.go` | `VideoPriceConfig` 加字段 + `getVideoUnitPrice` 加 4K case |
| `backend/internal/service/media_price_config.go` | `videoPriceConfigFromAPIKey` 加 4K |
| `backend/internal/service/openai_gateway_usage.go` | `groupMediaPricingLooksIncomplete` 聚合判断加 4K |
| `backend/internal/service/admin_service.go` + `admin_group.go` | Create/Update Input + 归一化 |
| `backend/internal/repository/group_repo.go` + `api_key_repo.go` | ent setter + ent→Group 映射 |
| `backend/internal/service/api_key_auth_cache.go` + `impl` | 缓存字段 + 2 处映射 |
| `backend/internal/handler/admin/group_handler.go` | 请求 struct + 映射 |
| `backend/internal/handler/dto/types.go` + `mappers.go` | 响应 DTO + 映射 |
| `frontend/src/types/index.ts` + `groupsImagePricing.ts` + `GroupsView.vue` | 前端类型 + 表单 + 占位 |

---

### Task 1: Ent schema + 代码生成 + 迁移 SQL

**Files:**
- Modify: `backend/ent/schema/group.go`（`video_price_1080p` 字段后，约 :141-144）
- Create: `backend/migrations/175_add_group_video_price_4k.sql`
- Generated（自动）: `backend/ent/group.go`, `backend/ent/group/group.go`, `backend/ent/group/where.go`, `backend/ent/group_create.go`, `backend/ent/group_update.go`, `backend/ent/mutation.go`, `backend/ent/migrate/schema.go`

**Interfaces:**
- Produces: ent 生成的 `SetVideoPrice4k` / `SetNillableVideoPrice4k` / `VideoPrice4k` 字段访问器（供 Task 3 使用）

- [ ] **Step 1: 加 ent schema 字段**

在 `backend/ent/schema/group.go` 的 `video_price_1080p` 字段块后并列加：
```go
		field.Float("video_price_4k").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
```

- [ ] **Step 2: 重新生成 ent 代码**

Run: `cd backend && go generate ./ent`
Expected: 无错误；`backend/ent/` 下 group 相关文件出现 `VideoPrice4k` 变体。

- [ ] **Step 3: 验证生成代码编译**

Run: `cd backend && go build ./ent/...`
Expected: 编译通过（确认 `SetNillableVideoPrice4k`、`VideoPrice4k` 已生成）。

- [ ] **Step 4: 写迁移 SQL**

创建 `backend/migrations/175_add_group_video_price_4k.sql`：
```sql
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE groups ADD COLUMN IF NOT EXISTS video_price_4k DECIMAL(20,8);
COMMENT ON COLUMN groups.video_price_4k IS '4K 视频生成每秒单价 (USD/s)';
```

- [ ] **Step 5: 确认迁移序号无冲突**

Run: `cd backend && ls migrations/*.sql | sort | tail`
Expected: `175_add_group_video_price_4k.sql` 是新的最高序号，无重复。

- [ ] **Step 6: Commit**

```bash
git add backend/ent/schema/group.go backend/ent/ backend/migrations/175_add_group_video_price_4k.sql
git commit -m "feat(ent): group 加 video_price_4k 字段 + 迁移 175

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: service 计费核心 — group 4K 计价（TDD）

**Files:**
- Modify: `backend/internal/service/group.go`（`Group` struct :52；`GetVideoPrice` :143-146）
- Modify: `backend/internal/service/billing_service.go`（`VideoPriceConfig` :1313；`getVideoUnitPrice` :1436-1452）
- Modify: `backend/internal/service/media_price_config.go`（`videoPriceConfigFromAPIKey` :22-26）
- Modify: `backend/internal/service/openai_gateway_usage.go`（`groupMediaPricingLooksIncomplete` :617-618）
- Test: `backend/internal/service/billing_service_test.go`（扩展）
- Test: `backend/internal/service/openai_gateway_per_second_fallback_test.go`（扩展，已有 4K 渠道用例，补 group 4K 用例）

**Interfaces:**
- Consumes: 无（Group struct 字段在本任务手写，不依赖 ent）
- Produces: `Group.VideoPrice4K *float64`；`GetVideoPrice("4k") -> *float64`；`VideoPriceConfig.Price4K *float64`；`getVideoUnitPrice` 识别 4K

- [ ] **Step 1: 写失败测试 — getVideoUnitPrice 4K + GetVideoPrice 4K**

在 `billing_service_test.go` 加（或扩展已有视频价测试）：
```go
func TestGetVideoUnitPrice_4K(t *testing.T) {
	p4k := 0.05
	cfg := &VideoPriceConfig{Price4K: &p4k}
	bs := &BillingService{cfg: &config.Config{}, fallbackPrices: map[string]*ModelPricing{}}
	// groupConfig 配了 4K → 用 groupConfig 价
	require.InDelta(t, 0.05, bs.getVideoUnitPrice("any-model", VideoBillingResolution4K, cfg), 1e-12)
	// groupConfig 无 4K → 回落默认（不 panic）
	require.Equal(t, 0.0, bs.getVideoUnitPrice("any-model", VideoBillingResolution4K, &VideoPriceConfig{}))
}

func TestGroupGetVideoPrice_4K(t *testing.T) {
	p4k := 0.05
	g := &Group{VideoPrice4K: &p4k}
	require.Equal(t, &p4k, g.GetVideoPrice("4k"))
	// 未配 4K → nil（回落渠道）
	require.Nil(t, (&Group{}).GetVideoPrice("4k"))
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test -tags=unit -run 'TestGetVideoUnitPrice_4K|TestGroupGetVideoPrice_4K' ./internal/service/`
Expected: FAIL（`Group.VideoPrice4K` 未定义 / `getVideoUnitPrice` 无 4K case）。

- [ ] **Step 3: 加 Group struct 字段 + GetVideoPrice 4K 返回字段**

`backend/internal/service/group.go`：
- struct（`VideoPrice1080P` 后）加：
  ```go
  	VideoPrice4K                  *float64
  ```
- `GetVideoPrice`（:143-146）把 4K 分支从：
  ```go
  	case VideoBillingResolution4K:
  		// group 视频价未覆盖 4K（无 VideoPrice4K 字段）；返回 nil 使计费回落到渠道定价的 4K 档，
  		// 而非借用 480P 价格导致 4K 请求绕过渠道 4K 单价。
  		return nil
  ```
  改为：
  ```go
  	case VideoBillingResolution4K:
  		return g.VideoPrice4K
  ```

- [ ] **Step 4: 加 VideoPriceConfig.Price4K + getVideoUnitPrice 4K case**

`backend/internal/service/billing_service.go`：
- `VideoPriceConfig`（:1313）加：
  ```go
  	Price4K    *float64
  ```
- `getVideoUnitPrice`（:1447 后，1080P case 后）加：
  ```go
  	case VideoBillingResolution4K:
  		if groupConfig.Price4K != nil {
  			return *groupConfig.Price4K
  		}
  ```

- [ ] **Step 5: 跑 Step 1 测试确认通过**

Run: `cd backend && go test -tags=unit -run 'TestGetVideoUnitPrice_4K|TestGroupGetVideoPrice_4K' ./internal/service/`
Expected: PASS。

- [ ] **Step 6: 写失败测试 — calculateOpenAIVideoCost group 4K 价**

在 `openai_gateway_per_second_fallback_test.go` 加（group 配 4K + 渠道配 4K，期望用 group 价）：
```go
func TestCalculateOpenAIVideoCost_Group4KOverridesChannel(t *testing.T) {
	groupID := int64(21)
	group4k := 0.20   // group 4K 每秒价
	channel4k := 0.05 // 渠道 4K 每秒价（应被 group 覆盖）
	cache := newEmptyChannelCache()
	cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: "kling-video"}] = &ChannelModelPricing{
		BillingMode: BillingModePerSecond,
		Intervals:   []PricingInterval{{TierLabel: "4k", PerRequestPrice: &channel4k}},
	}
	cache.channelByGroupID[groupID] = &Channel{ID: groupID, Status: StatusActive}
	cache.groupPlatform[groupID] = ""
	cache.loadedAt = time.Now()
	cs := &ChannelService{}
	cs.cache.Store(cache)
	bs := &BillingService{cfg: &config.Config{}, fallbackPrices: map[string]*ModelPricing{}}
	svc := &OpenAIGatewayService{resolver: NewModelPricingResolver(cs, bs), billingService: bs}
	apiKey := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, VideoRateIndependent: true, VideoPrice4K: &group4k}}

	r := &OpenAIForwardResult{VideoCount: 1, VideoResolution: "4k"} // duration 默认 8
	c := svc.calculateOpenAIVideoCost(context.Background(), "kling-video", apiKey, r, 1.0)
	require.InDelta(t, 1.60, c.TotalCost, 1e-10) // group 0.20 × 8（非渠道 0.05）
}
```

- [ ] **Step 7: 跑测试确认通过（Step 3-4 已让 GetVideoPrice 返回字段，本测试应直接绿）**

Run: `cd backend && go test -tags=unit -run 'TestCalculateOpenAIVideoCost_Group4KOverridesChannel' ./internal/service/`
Expected: PASS。若 FAIL，检查 `videoPriceConfigFromAPIKey` 是否已加 Price4K（下一步）。

- [ ] **Step 8: videoPriceConfigFromAPIKey 加 Price4K**

`backend/internal/service/media_price_config.go`（:25 后）：
```go
	return &VideoPriceConfig{
		Price480P:  apiKey.Group.VideoPrice480P,
		Price720P:  apiKey.Group.VideoPrice720P,
		Price1080P: apiKey.Group.VideoPrice1080P,
		Price4K:    apiKey.Group.VideoPrice4K,
	}
```

- [ ] **Step 9: 写失败测试 — 聚合判断含 4K**

在 `openai_gateway_video_usage_test.go` 或同包测试加：
```go
func TestGroupMediaPricingLooksIncomplete_Only4K(t *testing.T) {
	p := 0.05
	// 只配 4K → 不应被判为"完全未配价"
	g := &Group{VideoPrice4K: &p}
	require.False(t, groupMediaPricingLooksIncomplete(g))
	// 全未配 → true
	require.True(t, groupMediaPricingLooksIncomplete(&Group{}))
}
```

- [ ] **Step 10: 跑测试确认失败**

Run: `cd backend && go test -tags=unit -run 'TestGroupMediaPricingLooksIncomplete_Only4K' ./internal/service/`
Expected: FAIL（聚合判断未含 4K，只配 4K 时返回 true）。

- [ ] **Step 11: 聚合判断加 4K**

`backend/internal/service/openai_gateway_usage.go:617-618` 末尾加 `&& group.VideoPrice4K == nil`：
```go
	return group.ImagePrice1K == nil && group.ImagePrice2K == nil && group.ImagePrice4K == nil &&
		group.VideoPrice480P == nil && group.VideoPrice720P == nil && group.VideoPrice1080P == nil &&
		group.VideoPrice4K == nil
```

- [ ] **Step 12: 跑全部 Task 2 测试确认通过**

Run: `cd backend && go test -tags=unit -run 'TestGetVideoUnitPrice_4K|TestGroupGetVideoPrice_4K|TestCalculateOpenAIVideoCost_Group4KOverridesChannel|TestGroupMediaPricingLooksIncomplete_Only4K|TestCalculateOpenAIVideoCost' ./internal/service/`
Expected: PASS（含现有 4K 渠道用例无回归）。

- [ ] **Step 13: gofmt + Commit**

```bash
cd backend && gofmt -w internal/service/group.go internal/service/billing_service.go internal/service/media_price_config.go internal/service/openai_gateway_usage.go
git add backend/internal/service/group.go backend/internal/service/billing_service.go backend/internal/service/media_price_config.go backend/internal/service/openai_gateway_usage.go backend/internal/service/billing_service_test.go backend/internal/service/openai_gateway_per_second_fallback_test.go backend/internal/service/openai_gateway_video_usage_test.go
git commit -m "feat(billing): group 视频价 4K 计价核心(GetVideoPrice 反转 + getVideoUnitPrice 4K + 聚合判断)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: Repository 映射（ent setter + ent→Group）

**Files:**
- Modify: `backend/internal/repository/group_repo.go`（:65, :155, :213-214 三处）
- Modify: `backend/internal/repository/api_key_repo.go`（:970-972 映射）

**Interfaces:**
- Consumes: Task 1 的 `SetNillableVideoPrice4k` / `SetVideoPrice4k`；Task 2 的 `Group.VideoPrice4K`
- Produces: ent 持久化 4K；api_key 加载时 ent→service.Group 映射 4K

- [ ] **Step 1: group_repo.go Create 链加 setter**

`backend/internal/repository/group_repo.go:65` 后加（紧跟 `SetNillableVideoPrice1080p`）：
```go
		SetNillableVideoPrice4k(groupIn.VideoPrice4K).
```

- [ ] **Step 2: group_repo.go Update 链加 setter**

同文件 `:155` 后（紧跟 Update 链的 `SetNillableVideoPrice1080p`）加同样一行：
```go
		SetNillableVideoPrice4k(groupIn.VideoPrice4K).
```

- [ ] **Step 3: group_repo.go 条件 Update 加 4K**

同文件 `:213-214`（`if groupIn.VideoPrice1080P != nil` 块）后加：
```go
	if groupIn.VideoPrice4K != nil {
		builder = builder.SetVideoPrice4k(*groupIn.VideoPrice4K)
	}
```

- [ ] **Step 4: api_key_repo.go ent→Group 映射加 4K**

`backend/internal/repository/api_key_repo.go:972` 后加：
```go
		VideoPrice4K:                    g.VideoPrice4k,
```
（注意：ent 生成的字段名是小写 `VideoPrice4k`，service.Group 是 `VideoPrice4K`。若该文件 `:206` 附近有显式 select 字段列表，核对是否需补 `group.FieldVideoPrice4k`；若为全量 `*` 查询则无需。）

- [ ] **Step 5: 编译验证**

Run: `cd backend && go build ./internal/repository/...`
Expected: 编译通过。

- [ ] **Step 6: Commit**

```bash
git add backend/internal/repository/group_repo.go backend/internal/repository/api_key_repo.go
git commit -m "feat(repo): group video_price_4k 持久化与映射

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: Admin Input + 归一化

**Files:**
- Modify: `backend/internal/service/admin_service.go`（`CreateGroupInput` :220；`UpdateGroupInput` :274）
- Modify: `backend/internal/service/admin_group.go`（Create :156-158/:287-289；Update :537-544）
- Test: `backend/internal/service/admin_service_group_test.go`

**Interfaces:**
- Consumes: Task 2 的 `Group.VideoPrice4K`
- Produces: `CreateGroupInput.VideoPrice4K` / `UpdateGroupInput.VideoPrice4K`（供 Task 6 handler 映射）

- [ ] **Step 1: admin_service.go Input struct 加字段**

`CreateGroupInput`（:222 后）与 `UpdateGroupInput`（:276 后）各加：
```go
	VideoPrice4K     *float64
```

- [ ] **Step 2: admin_group.go Create 归一化 + 赋值**

`backend/internal/service/admin_group.go`：
- `:158` 后加：`videoPrice4K := normalizePrice(input.VideoPrice4K)`
- `:289` 后加：`VideoPrice4K: videoPrice4K,`

- [ ] **Step 3: admin_group.go Update 条件赋值**

`:544` 后加：
```go
	if input.VideoPrice4K != nil {
		group.VideoPrice4K = normalizePrice(input.VideoPrice4K)
	}
```

- [ ] **Step 4: 编译 + 现有 admin group 测试**

Run: `cd backend && go build ./internal/service/... && go test -tags=unit -run 'Group' ./internal/service/`
Expected: 编译通过；现有 group 测试无回归。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/admin_service.go backend/internal/service/admin_group.go
git commit -m "feat(admin): group Create/Update 接收 video_price_4k

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: API Key 缓存字段 + 映射

**Files:**
- Modify: `backend/internal/service/api_key_auth_cache.go`（:83 后）
- Modify: `backend/internal/service/api_key_auth_cache_impl.go`（:273, :357 两处映射）

**Interfaces:**
- Consumes: Task 2 的 `Group.VideoPrice4K`
- Produces: 缓存序列化 `video_price_4k`

- [ ] **Step 1: 缓存 struct 加字段**

`backend/internal/service/api_key_auth_cache.go:83` 后加：
```go
	VideoPrice4K                  *float64 `json:"video_price_4k,omitempty"`
```

- [ ] **Step 2: 构建缓存映射**

`backend/internal/service/api_key_auth_cache_impl.go:273` 后加：
```go
		VideoPrice4K:  apiKey.Group.VideoPrice4K,
```

- [ ] **Step 3: 从快照恢复映射**

同文件 `:357` 后加：
```go
		VideoPrice4K:  snapshot.Group.VideoPrice4K,
```

- [ ] **Step 4: 编译验证**

Run: `cd backend && go build ./internal/service/...`
Expected: 编译通过。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/api_key_auth_cache.go backend/internal/service/api_key_auth_cache_impl.go
git commit -m "feat(cache): api_key 缓存 group video_price_4k

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: Handler 请求/响应 + DTO

**Files:**
- Modify: `backend/internal/handler/admin/group_handler.go`（`CreateGroupRequest` :112；`UpdateGroupRequest` :165；Create 映射 :336；Update 映射 :404）
- Modify: `backend/internal/handler/dto/types.go`（`Group` :126；核对 `AdminGroup` :150）
- Modify: `backend/internal/handler/dto/mappers.go`（:202）
- Test: `backend/internal/server/api_contract_test.go`

**Interfaces:**
- Consumes: Task 4 的 `CreateGroupInput/UpdateGroupInput.VideoPrice4K`；Task 2 的 `Group.VideoPrice4K`
- Produces: HTTP `video_price_4k` 请求/响应字段

- [ ] **Step 1: 请求 struct 加字段**

`group_handler.go` 的 `CreateGroupRequest`（:112）与 `UpdateGroupRequest`（:165）各加：
```go
	VideoPrice4K *float64 `json:"video_price_4k"`
```

- [ ] **Step 2: handler→service 映射**

`group_handler.go` Create（:336）与 Update（:404）各加：
```go
		VideoPrice4K:                  req.VideoPrice4K,
```

- [ ] **Step 3: DTO 响应字段**

`dto/types.go` 的 `Group`（:126）加：
```go
	VideoPrice4K    *float64 `json:"video_price_4k"`
```
核对 `AdminGroup`（:150）：若它独立声明视频价字段（非内嵌 Group），同样加。

- [ ] **Step 4: DTO mapper**

`dto/mappers.go:202` 后加：
```go
		VideoPrice4K:                  g.VideoPrice4K,
```

- [ ] **Step 5: 编译 + 现有 contract 测试**

Run: `cd backend && go build ./... && go test -tags=unit -run 'Group' ./internal/server/ ./internal/handler/...`
Expected: 编译通过；若 `api_contract_test.go` 有 group 字段断言需同步 4K，按失败信息补。

- [ ] **Step 6: Commit**

```bash
git add backend/internal/handler/admin/group_handler.go backend/internal/handler/dto/types.go backend/internal/handler/dto/mappers.go
git commit -m "feat(handler): group video_price_4k 请求/响应字段

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: 前端（类型 + 表单 + 占位 + i18n）

**Files:**
- Modify: `frontend/src/types/index.ts`（:534, :671, :718）
- Modify: `frontend/src/views/admin/groupsImagePricing.ts`（:24, :52）
- Modify: `frontend/src/views/admin/GroupsView.vue`（10+ 处，见步骤）
- Modify: `frontend/src/i18n/locales/zh/admin/overview.ts:925`、`frontend/src/i18n/locales/en/admin/overview.ts:934`（可选）
- Test: `frontend/src/views/admin/__tests__/groupsImagePricing.spec.ts`

**Interfaces:**
- Consumes: Task 6 的后端 `video_price_4k` 字段
- Produces: 前端可配置 4K 每秒价

- [ ] **Step 1: TypeScript 类型**

`frontend/src/types/index.ts`：
- `AdminGroup`（:534）加：`video_price_4k: number | null`
- `CreateGroupRequest`（:671）加：`video_price_4k?: number | null`
- `UpdateGroupRequest`（:718）加：`video_price_4k?: number | null`

- [ ] **Step 2: 定价占位配置**

`frontend/src/views/admin/groupsImagePricing.ts`：
- `VideoPricingTierKey`（:24）加 `| "video_price_4k"`
- `defaultVideoPricePlaceholders.grok`（:52）加 `video_price_4k: ""`（后端无 4K 默认价，留空）

- [ ] **Step 3: GroupsView.vue Create 表单**

`frontend/src/views/admin/GroupsView.vue`：
- 外层 grid（:965）`grid-cols-3` → `grid-cols-4`
- `:998` 后（1080p 输入后）加 4K 输入控件（仿 1080p 的 `<input v-model.number="createForm.video_price_4k" ... :placeholder="getVideoPricePlaceholder(createForm.platform, 'video_price_4k')">`，label `4K ($/s)`）

- [ ] **Step 4: GroupsView.vue Edit 表单**

- 外层 grid（:2398）`grid-cols-3` → `grid-cols-4`
- `:2431` 后加 4K 输入（`editForm.video_price_4k`，同上模式）

- [ ] **Step 5: GroupsView.vue 状态与清洗（10 处）**

按现有 `video_price_1080p` 模式，在以下位置加 4K：
- `createForm` 初值（:3798）：`video_price_4k: null as number | null,`
- `editForm` 初值（:4143）：同上
- `VideoPricingFormState` 类型（:4200）：`video_price_4k: number | string | null;`
- `videoPricingTiers`（:4212）：`{ key: "video_price_4k", label: "4K" }`
- `resetCreateForm`（:4525）：`createForm.video_price_4k = null;`
- Create `requestData`（:4661）：`requestData.video_price_4k = emptyToNull(requestData.video_price_4k);`
- `editForm` 回填（:4714）：`editForm.video_price_4k = group.video_price_4k;`
- `resetEditForm`（:4771）：`editForm.video_price_4k = null;`
- Update `payload` 清洗（:4857）：`payload.video_price_4k = emptyPriceToClear(payload.video_price_4k);`

- [ ] **Step 6: i18n（可选）**

`overview.ts` 的 `videoPricing.description`（列举分辨率档位处）补 4K（zh + en）。

- [ ] **Step 7: 前端测试 + typecheck**

Run: `cd frontend && pnpm run typecheck && pnpm run test:run -- groupsImagePricing`
Expected: typecheck 通过；groupsImagePricing spec 通过（含新 4K tier key）。

- [ ] **Step 8: Commit**

```bash
git add frontend/src/types/index.ts frontend/src/views/admin/groupsImagePricing.ts frontend/src/views/admin/GroupsView.vue frontend/src/i18n/locales/zh/admin/overview.ts frontend/src/i18n/locales/en/admin/overview.ts
git commit -m "feat(frontend): group 视频价 4K 配置(类型/表单/占位)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 8: 全量验证 + 回归

**Files:** 无新改动（验证性任务）

- [ ] **Step 1: 后端全量构建 + lint**

Run: `cd backend && go build ./... && golangci-lint run ./internal/service/... ./internal/handler/... ./internal/repository/...`
Expected: 编译通过；本次新增/修改文件无 lint 问题（预存 gofmt/gosec 问题不在本次范围，忽略）。

- [ ] **Step 2: 后端 service 全量测试（回归）**

Run: `cd backend && go test -tags=unit -run 'Video|PerSecond|Billing|Resolution|Group|Image|Cost|Usage' ./internal/service/`
Expected: PASS。注意：`TestDoBundleUpgrade_*` 等是预存 sqlite lease 失败（与本次无关），不计入。

- [ ] **Step 3: 前端 lint + typecheck**

Run: `cd frontend && pnpm run lint:check && pnpm run typecheck`
Expected: 通过。

- [ ] **Step 4: 端到端确认（手动，可选）**

配一个通用视频模型的 group `video_price_4k`，调 `POST /v1/videos`（请求体 `{"model":"...","resolution":"4k","duration":8}`），确认：余额按 group 4K 每秒价 × 8 扣减；group 未配 4K 时回落渠道 4K 档；使用记录 `video_resolution=4k`、`billing_mode=per_second`、费用>0。

- [ ] **Step 5: 最终 commit（如有 lint/i18n 修正）**

```bash
git add -A
git commit -m "chore(billing): group 4K 视频计价 lint/收尾

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Self-Review 记录

**Spec coverage:** spec 第 3 节 13 层 → Task 1(schema+迁移) / Task 2(group.go+billing_service+media_price_config+openai_gateway_usage:618) / Task 3(repo) / Task 4(admin) / Task 5(cache) / Task 6(handler+dto) / Task 7(前端)。spec 第 4 节两个风险（GetVideoPrice 反转、:618 聚合）→ Task 2 Step 3 + Step 9-11 覆盖。✓

**关键同步点已显式列为测试任务：** GetVideoPrice 4K（Task 2 Step 1/3）、聚合判断 :618（Task 2 Step 9-11）、group 4K 覆盖渠道（Task 2 Step 6）。✓

**类型一致性：** ent 生成字段 `VideoPrice4k`（小写 k）；service.Group / VideoPriceConfig / 缓存 / DTO 用 `VideoPrice4K`（大写 K）；JSON / SQL / 前端用 `video_price_4k`。Task 3 Step 4 已标注 ent→Group 的大小写差异。✓
