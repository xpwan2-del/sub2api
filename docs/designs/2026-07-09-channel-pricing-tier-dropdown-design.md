# 渠道定价：图片/视频计费层级下拉框化 + 匹配闭环

- **分支**：`feat/channel-pricing-tier-dropdown`（基于 `main` `dc1bc154`）
- **日期**：2026-07-09
- **状态**：设计已与用户确认，待审查后转实现计划

## 1. 背景与问题

在渠道（channel）新增/编辑页面的「定价配置」中，用户可为模型配置分层计费。当前计费模式有三种：`token`（按 token 区间）、`per_request`（按次）、`image`（图片按次）。`per_request`/`image` 模式下，每个「层级」用一个**自由文本输入框**（字段 `tier_label`）填写分辨率标签（如 `1K`/`2K`/`4K`），极易输错。

探索代码后发现四个问题：

1. **分辨率是手输文本框**：`IntervalRow.vue` 中 `tier_label` 是 `<input type="text">`，`addImageTier` 虽预填 `['1K','2K','4K','HD']`，但用户可随意改成任何值。
2. **大小写敏感导致匹配静默失败**：计费热路径 `GetRequestTierPrice`（`model_pricing_resolver.go`）用大小写敏感的 `==` 比较 `tier_label`，而后端分类器输出恒为大写 `1K`。若 DB 存小写 `1k`（手输常见），请求来 `1K` 对不上 → 静默落到兜底价。另一处 `GetTierByLabel`（`channel.go`）却是大小写不敏感——两处不一致。
3. **HD 是死档**：前端预填了 `HD`，但后端 `ClassifyImageBillingTier`（`image_billing_size.go`）**只产生 `1K`/`2K`/`4K`**，从不产生 `HD`。配了 HD 永远不会被任何请求命中。
4. **视频无分辨率分层**：全后端无 `480p`/`720p`/`videoTier` 等逻辑；`VideoCount` 仅用于套餐额度累加，不参与金额计算。视频请求金额要么走 token（通常 0 元），要么走 per_request 兜底价。历史上的 sora 方案（`047` 加 / `090_drop_sora.sql` 删）已废弃。
5. **Min/Max 框在 per_request/image 模式下是摆设**：`IntervalRow.vue` 在非 token 模式仍展示 `min_tokens`/`max_tokens` 输入框，但 `types.ts` 注释明确说明后端按 `tier_label` 匹配、不读 min/max，徒增困惑。

## 2. 目标

- **前端**：把图片/视频计费层级的「分辨率」从手输文本框改为固定枚举下拉框，消除输错。
- **后端（图片）**：修复匹配逻辑，让「配了就能命中」闭环成立（统一大小写、对齐枚举）。
- **后端（视频）**：本轮只搭好计费骨架（新增 `video` 模式 + 接进计费分发，让兜底价生效）；分辨率解析链路留待后续专项。

## 3. 范围

**做：**
- 图片：下拉框化（1K/2K/4K）+ 后端大小写归一化 + 去掉 HD 死档 + 移除无用的 Min/Max 框
- 视频：新增独立 `video` 计费模式 + 前端分辨率下拉框（480P/720P/1080P/4K）+ 后端常量与计费分发接入
- 旧数据：后端归一化 + 前端标红提示清理

**不做（明确边界）：**
- ❌ 视频分辨率解析链路（从视频请求/响应解析分辨率 → `ClassifyVideoBillingTier` → 命中 tier）。依赖具体视频上游 API 协议（Sora/Veo/Runway 各不同），为独立专项。本轮 video 档位配了不命中，视频按默认兜底价计费。
- ❌ 数据库表结构变更 / migration。`channel_pricing_intervals.tier_label VARCHAR(50)` 已支持任意字符串，无需改。
- ❌ 旧数据自动迁移脚本（采用前端提示、用户手动清理策略）。

## 4. 已确认决策

| # | 决策点 | 选择 |
|---|---|---|
| 1 | 改动范围 | 图片完整闭环 + 视频仅前端配置（后端解析链路本轮不建） |
| 2 | 图片档位枚举 | `1K` / `2K` / `4K`（去掉 HD） |
| 3 | 下拉框严格度 | 严格固定枚举，不允许手输；旧非标数据走「后端归一化 + 前端标红提示」 |
| 4 | 视频配置形态 | 新增独立 `video` 计费模式 |
| 5 | `per_request` 模式层级 | 保持自由文本不动（语义通用，不强加分辨率） |
| 6 | 视频档位枚举 | `480P` / `720P` / `1080P` / `4K` |

## 5. 详细设计

### 5.1 计费模式扩展

枚举变为 `token` / `per_request` / `image` / **`video`**（新增）。

- 前端 `frontend/src/constants/channel.ts`：加 `BILLING_MODE_VIDEO = 'video' as const`，`BillingMode` 类型并入。
- 前端 `frontend/src/api/admin/channels.ts`：`BillingMode` 类型并入 `video`。
- 后端 `backend/internal/service/channel.go`：加 `BillingModeVideo BillingMode = "video"`，`IsValid()` 补 `video` 为合法值。

### 5.2 分辨率档位枚举（前后端共享同一套 key）

这是整个方案的基石——**运行期解析出的 key 与配置侧枚举必须来自同一集合**，否则「配了不生效」。

- **图片**：`1K` / `2K` / `4K`，与后端 `ClassifyImageBillingTier` 输出对齐（≤1024→1K，≤2048→2K，更大→4K，认不出默认 2K）。
- **视频**：`480P` / `720P` / `1080P` / `4K`。

前端在 `frontend/src/constants/channel.ts`（或新建 `billingTiers.ts`）定义：
```ts
export const IMAGE_RESOLUTION_OPTIONS = [
  { value: '1K', label: '1K' },
  { value: '2K', label: '2K' },
  { value: '4K', label: '4K' },
]
export const VIDEO_RESOLUTION_OPTIONS = [
  { value: '480P', label: '480P' },
  { value: '720P', label: '720P' },
  { value: '1080P', label: '1080P' },
  { value: '4K', label: '4K' },
]
```
作为图片/视频下拉框的**唯一数据源**。后端无需新增枚举常量（图片复用现有分类器输出，视频本轮不解析）。

### 5.3 前端组件改造

**`frontend/src/components/admin/channel/IntervalRow.vue`（核心）：**
- `image` / `video` 模式：「分辨率」字段从 `<input type="text">` 改为自研 `Select.vue` 下拉框，选项按 `mode` 取 `IMAGE_RESOLUTION_OPTIONS` / `VIDEO_RESOLUTION_OPTIONS`。
- 移除 `image` / `video` 模式下的 `Min` / `Max` 输入框（后端不读）。
- `token` 模式：完全不动（保留 token 区间 + 各 $/MTok 价）。
- `per_request` 模式：保持自由文本「层级」（按决策 5，不动）。

**`frontend/src/components/admin/channel/PricingEntryCard.vue`：**
- `billingModeOptions` 追加 `{ value: 'video', label: t('admin.channels.billingMode.video', '视频（按次）') }`。
- 新增 `v-else-if="entry.billing_mode === 'video'"` 模板分支，结构同 `image`：默认单次价 + 层级列表（i18n key `admin.channels.form.videoTiers`）。
- 把 `addImageTier` 泛化为「按模式预填档位」：`image` → `1K`/`2K`/`4K`，`video` → `480P`/`720P`/`1080P`/`4K`；**去掉 HD**。

**`frontend/src/components/admin/channel/types.ts`：**
- `validateIntervals`：对 `image` / `video` 模式增加两条校验：
  - 「`tier_label` 必须是合法枚举值」（非白名单值报错）。
  - 「同档位不可重复」（同一 `tier_label` 出现多次报错——现状 `per_request`/`image` 跳过重叠校验，配重了不报错）。
- `token` / `per_request` 模式校验逻辑不变。

**`frontend/src/components/admin/channel/` 的「标红提示」：**
- `IntervalRow.vue`：若 `tier_label` 不在当前模式的合法枚举内（旧非标数据），下拉框区域标红并提示「该档位无法匹配，请重新选择」。校验在 `validateIntervals` 中触发。

**i18n（`frontend/src/i18n/locales/zh.ts` / `en.ts`）：** 补 `billingMode.video`、`form.videoTiers`、`form.invalidResolution`（标红提示）、`form.duplicateResolution`（重复档位）等 key。

### 5.4 后端匹配修复（图片闭环）

1. **大小写归一化（关键 bug 修复）**：`GetRequestTierPrice`（`model_pricing_resolver.go`）把 `tier.TierLabel == tierLabel` 改为 `strings.EqualFold(tier.TierLabel, tierLabel)`，与 `GetTierByLabel` 统一。根除「DB 小写 / 请求大写 → 静默落兜底价」。
2. **video 接进计费分发**：
   - `applyRequestTierOverrides`（`model_pricing_resolver.go`）：mode 判断 `per_request, image` 扩展为 `per_request, image, video`，使 `video` 的 intervals 进入 `RequestTiers`、`PerRequestPrice` 进入 `DefaultPerRequestPrice`。
   - `CalculateCostUnified`（`billing_service.go`）的 mode 分发：`video` 归到与 `per_request`/`image` 相同的 `calculatePerRequestCost` 路径。
   - 效果：本轮即使没有视频分辨率解析链路，配了 `video` 模式的渠道，视频请求也能按默认兜底价计费（不再 0 元或走 token）。
3. 图片分类器 `ClassifyImageBillingTier` **无需改动**——已与下拉框枚举对齐。
4. 后端 `ValidateIntervals`（`channel.go`）：`video` 同 `per_request`/`image`，按 label 分层、跳过 token 区间重叠校验。

### 5.5 旧数据兼容

- 后端从 DB 加载 `channel_pricing_intervals` 后，对 `tier_label` 做 `strings.ToUpper` 归一化（`1k`→`1K`、`720p`→`720P`）。仅归一化能映射到合法枚举的值；映射不上的（旧 `HD`、自定义词）**保留原值**。
- 前端在合法枚举之外的非标 `tier_label` 上标红提示，引导用户手动重新选择，**不自动删除/改写**。

## 6. 文件改动清单

**前端：**
- `frontend/src/constants/channel.ts` — `BILLING_MODE_VIDEO`、`IMAGE_RESOLUTION_OPTIONS`、`VIDEO_RESOLUTION_OPTIONS`
- `frontend/src/api/admin/channels.ts` — `BillingMode` 类型并入 `video`
- `frontend/src/components/admin/channel/IntervalRow.vue` — 下拉框替换手输、移除无用 Min/Max、标红提示
- `frontend/src/components/admin/channel/PricingEntryCard.vue` — `video` 模式分支、`addImageTier` 泛化、`billingModeOptions`
- `frontend/src/components/admin/channel/types.ts` — `validateIntervals` 校验加严
- `frontend/src/i18n/locales/zh.ts` / `en.ts` — video 模式与层级文案

**后端：**
- `backend/internal/service/channel.go` — `BillingModeVideo` 常量 + `IsValid()` + `ValidateIntervals` video 分支
- `backend/internal/service/model_pricing_resolver.go` — `GetRequestTierPrice` 大小写归一化 + `applyRequestTierOverrides` 接入 video
- `backend/internal/service/billing_service.go` — `CalculateCostUnified` mode 分发接入 video

**无 migration、无 ent schema 变更、无 Wire 变更。**

## 7. 测试策略

**后端（Go 单测，`-tags=unit`）：**
- `GetRequestTierPrice` 大小写归一化：DB 存 `1k`/`1K`/`1K`，请求 `1K`，三者均能命中对应 tier。
- `video` 模式计费分发：`resolved.Mode == video` + `DefaultPerRequestPrice` 时，视频请求（`RequestCount=1`）按兜底价计费，返回正确金额（非 0）。
- `video` 档位 tier 命中（为后续解析链路预留）：`RequestTiers` 含 `720P` tier 时，传入 `SizeTier="720P"` 命中对应单价。

**前端（Vitest）：**
- `validateIntervals`：非白名单 `tier_label` 报错；同档位重复报错；合法枚举通过。
- `IntervalRow` 渲染：`image` 模式下拉框选项为 1K/2K/4K；`video` 模式为 480P/720P/1080P/4K；非标值标红。

**类型检查：** `cd frontend && pnpm run typecheck` 必须通过。

## 8. 风险与边界

- **视频档位本轮不生效**：`video` 模式的 tier 单价（480P/720P/...）在视频解析链路建成前不会被命中，视频按默认兜底价计费。这是已知边界，需在 UI 上向运营说明（如 video 模式层级区加提示「分辨率匹配需后续支持」）。
- **后端大小写归一化的副作用**：若历史数据存在故意区分大小写的 tier_label（极少见），归一化会改变其匹配行为。风险极低，且符合「统一到枚举」的方向。
- **`per_request` 保持自由文本**：意味着「配了不生效」风险在 `per_request` 模式依然存在，但这是该模式通用性所需，且不在本次目标内。
