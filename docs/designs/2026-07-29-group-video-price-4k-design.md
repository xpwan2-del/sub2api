# Design: group 视频价 4K 字段 + 4K 视频计价全路径支持

日期: 2026-07-29
分支: feat/video-price
状态: 已批准（待写实施计划）

## 1. 背景与目标

前序工作（commit `785ba951f` + `f7c73b685`）已让通用视频模型的 per_second 计费生效，并打通渠道定价的 4K 档（per_second 按秒、video/image 按次）。但 **group 视频价没有 4K 字段**：`Group.GetVideoPrice("4k")` 当前返回 `nil`，4K 请求在 group 路径完全缺位，只能回落渠道。

**目标**：给 group 视频价补齐 4K 档（`video_price_4k` 每秒价，与 480P/720P/1080P 同口径），使 4K 视频在**按秒**（group 每秒价 + 渠道 per_second）与**按次**（渠道 video/image）两条计价路径下都能正确计价；group 显式配 4K 时优先用 group 价，未配回落渠道。

**非目标**（明确排除）：
- group 视频价**不新增"按次"字段**（按次由渠道 video/image 模式覆盖）。
- 4K 完全未配价时**不改默认价兜底**（保持现状：通用模型→图片 2K 兜底，grok→480p 默认）。
- `per_request` 模式**不纳入视频计费入口**（保持 `isOpenAIVideoUsage` 已收紧的现状）。

## 2. 4K 视频计价完整逻辑（优先级链）

视频计费入口 `OpenAIGatewayService.calculateOpenAIVideoCost`（`openai_gateway_usage.go:522`），4K 请求（`resolution = NormalizeVideoBillingResolutionOrDefault("4k") = "4k"`）按以下优先级处理。优先级与 480P/720P/1080P **完全一致**，仅补齐 4K 档。

### A. 按秒路径（每秒价 × 时长 × 段数）
`isOpenAIVideoUsage` 放行 `per_second/video/image` 三种渠道模式进入 `calculateOpenAIVideoCost`：

1. **group 配了 video_price_4k** → `GetVideoPrice("4k") = VideoPrice4K`（非 nil）→ `apiKeyHasConfiguredVideoPrice("4k")=true` → `CalculateVideoCost` → `getVideoUnitPrice` 4K case → **group 4K 每秒价 × 时长 × 段数**。
2. **group 未配 4K + 渠道 per_second 配了 4K 档** → `GetRequestTierPrice(resolved, "4k") > 0` → `computePerSecondVideoCost`（渠道 4K 每秒价）。
3. **渠道 per_second 无 4K 档但配了默认每秒价** → `DefaultPerRequestPrice > 0` → `computePerSecondVideoCost`。
4. **都没配** → `CalculateVideoCost` → `getDefaultVideoPrice`（现状兜底，不动）。

### B. 按次路径（每次价 × 段数，不乘时长）
`calculateOpenAIVideoCost:560` 的 `video/image` 分支 → `CalculateCostUnified(SizeTier="4k")` → `GetRequestTierPrice(resolved, "4k")`（`billing_service.go:1064`）→ 渠道 4K 档 × `videoCount`。**已支持，无需改**（本次加测试验证）。

### C. per_request 模式
`isOpenAIVideoUsage` 已排除 `per_request`（intentional，`f7c73b685`），不进视频计费。保持现状。

## 3. 改动清单（13 层）

### 3.1 Ent Schema（源头）
- `backend/ent/schema/group.go`：在 `video_price_1080p`（约 :141）后并列加：
  ```go
  field.Float("video_price_4k").
      Optional().
      Nillable().
      SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
  ```
- 改后执行 `go generate ./ent`，自动产出 `ent/group*.go`、`ent/mutation.go`、`ent/migrate/schema.go` 的 4K 变体（无需手改生成代码）。

### 3.2 数据库迁移（新增）
- 新建 `backend/migrations/175_add_group_video_price_4k.sql`：
  ```sql
  SET LOCAL lock_timeout = '5s';
  SET LOCAL statement_timeout = '10min';
  ALTER TABLE groups ADD COLUMN IF NOT EXISTS video_price_4k DECIMAL(20,8);
  COMMENT ON COLUMN groups.video_price_4k IS '4K 视频生成每秒单价 (USD/s)';
  ```
- 参考 `170_add_grok_video_pricing_controls.sql`（建列）+ `172_video_per_second_billing_metadata.sql`（USD/s 注释口径）。幂等（IF NOT EXISTS）。

### 3.3 Service 核心
- `backend/internal/service/group.go`：
  - `Group` struct（约 :52 后）加 `VideoPrice4K *float64`。
  - **`GetVideoPrice`（:143-146）4K 分支从 `return nil` 改为 `return g.VideoPrice4K`**（删除"返回 nil 使计费回落渠道"的注释，改为"返回 group 配置的 4K 每秒价"）。
- `backend/internal/service/billing_service.go`：
  - `VideoPriceConfig` struct（:1313）加 `Price4K *float64`。
  - `getVideoUnitPrice`（:1436）switch 加 `case VideoBillingResolution4K: if groupConfig.Price4K != nil { return *groupConfig.Price4K }`。
- `backend/internal/service/media_price_config.go`：
  - `videoPriceConfigFromAPIKey`（:22）加 `Price4K: apiKey.Group.VideoPrice4K,`。
  - `apiKeyHasConfiguredVideoPrice` **无需改**（通过 `GetVideoPrice` 自动覆盖 4K）。
- `backend/internal/service/openai_gateway_usage.go:617-618`：`groupMediaPricingLooksIncomplete` 聚合判断末尾加 `&& group.VideoPrice4K == nil`（否则只配 4K 的 group 被误判"完全未配价"→ 每请求回源 DB，性能 bug）。

### 3.4 Admin 输入与归一化
- `backend/internal/service/admin_service.go`：`CreateGroupInput`（:220）与 `UpdateGroupInput`（:274）各加 `VideoPrice4K *float64`。
- `backend/internal/service/admin_group.go`：
  - Create 归一化（:158 后）加 `videoPrice4K := normalizePrice(input.VideoPrice4K)`；赋值（:289 后）加 `VideoPrice4K: videoPrice4K,`。
  - Update 条件赋值（:544 后）加 `if input.VideoPrice4K != nil { group.VideoPrice4K = normalizePrice(input.VideoPrice4K) }`。

### 3.5 Repository
- `backend/internal/repository/group_repo.go`：Create 链（:65 后）、Update 链（:155 后）、条件 Update（:214 后）各加 `SetNillableVideoPrice4k(groupIn.VideoPrice4K)`（条件 Update 用 `if groupIn.VideoPrice4K != nil { builder = builder.SetVideoPrice4k(*groupIn.VideoPrice4K) }`）。
- `backend/internal/repository/api_key_repo.go:972` 后加 `VideoPrice4K: g.VideoPrice4k,`（ent→service.Group 映射）。

### 3.6 API Key 缓存
- `backend/internal/service/api_key_auth_cache.go:83` 后加 `VideoPrice4K *float64 \`json:"video_price_4k,omitempty"\``。
- `backend/internal/service/api_key_auth_cache_impl.go` 两处映射（:273、:357）各加 `VideoPrice4K: ...Group.VideoPrice4K,`。

### 3.7 Handler 与 DTO
- `backend/internal/handler/admin/group_handler.go`：`CreateGroupRequest`（:112）、`UpdateGroupRequest`（:165）各加 `VideoPrice4K *float64 \`json:"video_price_4k"\``；Create（:336）、Update（:404）映射各加 `VideoPrice4K: req.VideoPrice4K,`。
- `backend/internal/handler/dto/types.go`：`Group`（:126）加 `VideoPrice4K *float64 \`json:"video_price_4k"\``（并核对 `AdminGroup` 若独立声明视频价）。
- `backend/internal/handler/dto/mappers.go:202` 后加 `VideoPrice4K: g.VideoPrice4K,`。

### 3.8 前端类型
- `frontend/src/types/index.ts`：`AdminGroup`（:534）、`CreateGroupRequest`（:671）、`UpdateGroupRequest`（:718）各加 4K 字段（`video_price_4k: number | null` / `video_price_4k?: number | null`）。

### 3.9 前端 UI（GroupsView.vue）
`frontend/src/views/admin/GroupsView.vue` 每处按 480p/720p/1080p 模式加 4K：
- Create 表单输入（:998 后）+ 外层 `grid-cols-3`→`grid-cols-4`（:965）。
- Edit 表单输入（:2431 后）+ 外层 `grid-cols-3`→`grid-cols-4`（:2398）。
- `createForm` 初值（:3798）、`editForm` 初值（:4143）、`VideoPricingFormState` 类型（:4200）、`videoPricingTiers` 数组（:4212，加 `{ key: "video_price_4k", label: "4K" }`）。
- `resetCreateForm`（:4525）、Create `requestData` 清洗（:4661，`emptyToNull`）、`editForm` 回填（:4714）、`resetEditForm`（:4771）、Update `payload` 清洗（:4857，`emptyPriceToClear`）。
- 4K 输入 label 写 `4K ($/s)`（与现有档位硬编码 label 风格一致，i18n 不需要逐档 key）。

### 3.10 前端定价占位
- `frontend/src/views/admin/groupsImagePricing.ts`：`VideoPricingTierKey`（:24）加 `| "video_price_4k"`；`defaultVideoPricePlaceholders.grok`（:52）加 `video_price_4k: ""`（后端无 4K 默认价，留空，`getVideoPricePlaceholder` 回退空串）。

### 3.11 i18n（可选）
- `frontend/src/i18n/locales/{zh,en}/admin/overview.ts` 的 `videoPricing.description`（列举 480p/720p/1080p）补 4K（zh:925 / en:934）。

### 3.12 测试同步
引用视频价字段的测试需同步断言/夹具：
- `backend/internal/service/admin_service_group_test.go`
- `backend/internal/service/openai_gateway_per_second_fallback_test.go`（含 4K group 用例）
- `backend/internal/server/api_contract_test.go`
- `frontend/src/views/admin/__tests__/groupsImagePricing.spec.ts`

## 4. 关键不变量与风险

1. **`GetVideoPrice` 4K 语义反转**（`nil → g.VideoPrice4K`）：4K 计费从"总回落渠道"变为"group 优先"。group 配 4K → 用 group 价；group 未配（`VideoPrice4K=nil`）→ `apiKeyHasConfiguredVideoPrice("4k")=false` → 回落渠道（与现状一致）。需测试覆盖两条分支。
2. **聚合判断 `:618` 必须同步加 `VideoPrice4K==nil`**，否则只配 4K 的 group 被判"完全未配价"，触发 `apiKeyWithFreshGroupMediaPricing` 每请求回源 DB（性能 bug，且非计费错误，易漏）。
3. **ent 重新生成 + 迁移手写**：生产建表靠 `migrations/*.sql`（`//go:embed` + `migrations_runner.go` 按序号应用），**不是** ent auto-migrate。只改 schema 不写迁移 → 生产缺列。
4. **默认价兜底不动**：`getDefaultGrokImagineVideoPrice` / `getDefaultVideoPrice` 维持现状，4K 全未配时走图片 2K 兜底（通用）/ 480p（grok）。

## 5. 测试策略

### 后端单元测试（-tags=unit）
- `getVideoUnitPrice`：4K + groupConfig.Price4K 返回正确每秒价；Price4K=nil 时回落默认。
- `GetVideoPrice`：4K 返回 `VideoPrice4K`；未配返回 nil。
- `calculateOpenAIVideoCost`（4K 场景）：
  - group 配 4K → 用 group 4K 每秒价 × 时长（非渠道）。
  - group 未配 + 渠道 per_second 配 4K 档 → 用渠道 4K（按秒）。
  - group 未配 + 渠道 video 模式配 4K 档 → 按次计费（`CalculateCostUnified`）。
- `groupMediaPricingLooksIncomplete`：只配 VideoPrice4K 时返回 false（已配价）。
- `admin_group` create/update：4K 字段归一化与持久化。

### 前端测试
- `groupsImagePricing.spec.ts`：`VideoPricingTierKey` 含 4K。
- `GroupsView` 4K 输入渲染 + Create/Update payload 含 `video_price_4k`（如现有 spec 覆盖）。

### 验证（端到端）
配一个通用视频模型的 group `video_price_4k`，调 `POST /v1/videos`（请求体带 `resolution: "4k"` 或 `size: "3840x2160"`），确认：余额按 group 4K 每秒价 × 时长 扣减；group 未配 4K 时回落渠道 4K 档；使用记录 `video_resolution=4k`、费用>0。

## 6. 实施顺序（依赖关系）

1. ent schema → `go generate ./ent`
2. 新增迁移 `175_*.sql`
3. 后端 service：`group.go`(struct+GetVideoPrice) → `billing_service.go`(VideoPriceConfig+getVideoUnitPrice) → `media_price_config.go` → `openai_gateway_usage.go:618` → `admin_service.go`+`admin_group.go`
4. 后端 repo + cache：`group_repo.go` → `api_key_repo.go` → `api_key_auth_cache.go`+`impl`
5. 后端 handler + dto
6. 前端：`types/index.ts` → `groupsImagePricing.ts` → `GroupsView.vue`
7. 测试 + i18n 收尾
8. 验证：`go build ./...` + `golangci-lint run` + `go test -tags=unit ./internal/service/...` + 前端 `pnpm typecheck`
