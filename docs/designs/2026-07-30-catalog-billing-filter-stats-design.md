# 模型广场计费筛选适配 + 管理端运营统计设计

- 日期：2026-07-30
- 分支：`feat/model-catalog-ops`
- 状态：设计已确认，待实现
- 前置：`2026-07-30-model-catalog-ops-design.md`（模型广场运营优化，已落地）

## 1. 背景与目标

模型广场（model catalog）在 `2026-07-30-model-catalog-ops` 落地后，用户提出两点后续优化：

1. **公开页筛选条件的「计费模式」未覆盖平台全部计费模式** —— 视频/按秒计费模型在筛选下拉与价格摘要里显示成原始英文 key。
2. **管理端模型广场没有任何统计** —— 运营看不到模型总量、可见/隐藏、置顶/NEW/精选/推荐、平台分布等概览。

本次纯前端、低风险地解决这两点。**不改后端、不改数据模型、不加迁移。**

## 2. 平台计费模式（权威定义，单一轴 5 取值）

后端 `backend/internal/service/channel.go:10-37`：

| 常量 | 值 | 含义 |
|---|---|---|
| `BillingModeToken` | `token` | 按 token 区间计费（默认） |
| `BillingModePerRequest` | `per_request` | 按次计费 |
| `BillingModeImage` | `image` | 图片计费 |
| `BillingModeVideo` | `video` | 视频生成计费 |
| `BillingModePerSecond` | `per_second` | 视频按秒计费 |

前端两处重复定义同一组常量：`frontend/src/constants/channel.ts:7-17`（含 `as const` + `BillingMode` 联合类型）与 `frontend/src/utils/billingMode.ts:1-5`。

**排除项**（不是计费模式，不并入筛选器）：
- `BillingModelSource`（`requested/upstream/channel_mapped`）——「计费基准」，非计费模式。
- `billing_type`（余额=0 / 订阅=1）——「扣费来源/支付方式」，非模型定价模式。
- `BundlePlan`/`SubscriptionPlan`/`credit` —— 售卖套餐 / Antigravity 积分，与 `billing_mode` 无关。

## 3. Part 1 — 公开页计费模式筛选适配

### 3.1 根因（枚举漂移）

平台有 5 种计费模式，但前端 **3 处各自维护、互不同步的白名单副本**：

| 位置 | 当前白名单 | 缺失 |
|---|---|---|
| `components/models/ModelCatalogFilters.vue:58` | `token, image, per_request, unknown` | `video`, `per_second` |
| `components/models/ModelPriceSummary.vue:54` | `token, image, per_request, per_second, unknown` | `video` |
| `i18n/locales/{zh,en}/custom.ts` `modelCatalog.billingModes` | `token, image, per_request, per_second, unknown` | `video` 键 |

下拉选项本身是数据驱动的（`utils/modelCatalog.ts:118-126,133` 从 `item.pricing.billing_mode` 聚合 `facets.billingModes`），所以选项已自适应；**问题在于标签映射的白名单滞后**：未覆盖模式经 `billingModeLabel()` fallback 成原始字符串。

### 3.2 方案选型

- ❌ A. 复用 `utils/billingMode.ts:getBillingModeLabel` —— 它走 `admin.usage.billingMode*` 命名空间，会改模型广场现有文案（如 "Token 计费"→"按量"），不采纳。
- ✅ **B. 保留 `modelCatalog.billingModes.*` 文案，把「模式集合」收敛到规范常量 + 抽共享 helper，根除漂移。**

### 3.3 改动

**3.3.1 `utils/billingMode.ts` —— 新增模型广场专用 helper（单一真相源）**

```ts
const CATALOG_BILLING_MODES = new Set([
  BILLING_MODE_TOKEN,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_IMAGE,
  BILLING_MODE_VIDEO,
  BILLING_MODE_PER_SECOND,
])

/** 模型广场计费模式标签：已知模式走 i18n；空→兜底文案；未知非空→原样显示（便于运营发现新脏数据）。 */
export function getModelCatalogBillingModeLabel(
  mode: string | null | undefined,
  t: (key: string) => string,
): string {
  if (mode && CATALOG_BILLING_MODES.has(mode)) return t(`modelCatalog.billingModes.${mode}`)
  return mode || t('modelCatalog.billingModes.unknown')
}
```

未来新增计费模式只需：① `channel.go` 加常量 → ② 前端两处常量同步 → ③ 此处集合加一项 → ④ i18n 加键。

**3.3.2 `ModelCatalogFilters.vue`**

删 `const knownBillingModes = ...`（line 58）与 `billingModeLabel`（line 67-69），模板 `{{ billingModeLabel(mode) }}` 改为 `{{ getModelCatalogBillingModeLabel(mode, t) }}`，import 该 helper。

**3.3.3 `ModelPriceSummary.vue`**

删 `const knownBillingModes = ...`（line 54）；`billingLabel` computed（line 56-59）改为：
```ts
const billingLabel = computed(() =>
  getModelCatalogBillingModeLabel(props.pricing?.billing_mode, t),
)
```
注意：原代码 `mode = props.pricing?.billing_mode || 'unknown'` 的 `unknown` 兜底语义由 helper 内部 `mode || t(...unknown)` 接管，行为等价。

**3.3.4 i18n（`zh/en custom.ts` 的 `modelCatalog.billingModes`）**

补 `video` 键：
- zh：`video: '视频计费'`
- en：`video: 'Video'`

`per_second` 已存在；`unknown`（zh="价格待配置" / en="Price pending"）保留为兜底。

### 3.4 边界

- 空 `pricing` / 空 `billing_mode` → helper 返回「价格待配置」（与现状一致）。
- 后端默认空 `billing_mode` → `token`（`public_model_catalog_handler.go:389`），故正常不会命中兜底。
- 未知非空值（脏数据 / 未来新模式未补 i18n）→ 原样显示原始字符串，便于发现。

## 4. Part 2 — 管理端模型广场运营统计（仅前端）

### 4.1 约束

按确认的「运营概览（仅前端）」范围：统计限定在 admin 接口已有的**运营字段维度**（`pinned/hidden/is_new/featured/custom_tags/platform`）。计费模式/能力/健康等维度只在公开端 DTO，本轮**不涉及后端改动**。

### 4.2 改动

**4.2.1 抽纯函数 `utils/modelCatalog.ts`**

```ts
export interface CatalogStats {
  total: number
  visible: number
  hidden: number
  pinned: number
  isNew: number
  featured: number
  recommended: number
  byPlatform: Record<string, number>
}

export function computeCatalogStats(items: CatalogConfigItem[]): CatalogStats {
  const stats: CatalogStats = {
    total: items.length, visible: 0, hidden: 0, pinned: 0,
    isNew: 0, featured: 0, recommended: 0, byPlatform: {},
  }
  for (const it of items) {
    if (it.hidden) stats.hidden++; else stats.visible++
    if (it.pinned) stats.pinned++
    if (it.is_new) stats.isNew++
    if (it.featured) stats.featured++
    if (it.custom_tags?.includes('recommended')) stats.recommended++
    const p = it.platform || 'unknown'
    stats.byPlatform[p] = (stats.byPlatform[p] || 0) + 1
  }
  return stats
}
```

`CatalogConfigItem` 来自 `@/api/adminCatalog`。`recommended` = `custom_tags` 含 `'recommended'`（设计使然，recommended 仍写 custom_tags）。统计基于**全量 `items`**，不受搜索框影响。

**4.2.2 `views/admin/CatalogManageView.vue`**

在列表 `<section class="card">`（line 30）**上方**、`<template v-else>`（line 28）内插入统计区块：

```vue
<section class="card">
  <div class="grid grid-cols-2 gap-3 p-4 sm:grid-cols-4 lg:grid-cols-7">
    <div class="stat-tile">
      <span class="stat-label">{{ t('admin.catalogManage.stats.total') }}</span>
      <strong>{{ stats.total }}</strong>
    </div>
    <!-- visible / hidden / pinned / isNew / featured / recommended 同结构 -->
  </div>
  <div class="flex flex-wrap gap-2 border-t ... px-4 py-3">
    <span class="text-xs text-gray-500">{{ t('admin.catalogManage.stats.byPlatform') }}</span>
    <span v-for="[platform, count] in platformEntries" :key="platform" class="chip">
      {{ platform }} · {{ count }}
    </span>
  </div>
</section>
```

script 内：
```ts
import { computeCatalogStats } from '@/utils/modelCatalog'
const stats = computed(() => computeCatalogStats(items.value))
const platformEntries = computed(() =>
  Object.entries(stats.value.byPlatform).sort((a, b) => b[1] - a[1]),
)
```

toggling 开关就地改 `item`，`items.value` 变化 → `stats` 自动刷新。样式沿用页面既有 Tailwind + dark mode 类（`card`、`text-gray-900 dark:text-white` 等）。

**4.2.3 i18n（`admin.catalogManage.stats.*`，`zh/en custom.ts`）**

新增键：`total / visible / hidden / pinned / isNew / featured / recommended / byPlatform`。按 [[i18n-audit-and-custom-ts-fix]] 惯例在 `custom.ts` 深合并补缺，不改 main。

## 5. 受影响文件

| 文件 | 改动 |
|---|---|
| `frontend/src/utils/billingMode.ts` | 新增 `getModelCatalogBillingModeLabel` + `CATALOG_BILLING_MODES` |
| `frontend/src/components/models/ModelCatalogFilters.vue` | 删本地白名单，改调 helper |
| `frontend/src/components/models/ModelPriceSummary.vue` | 删本地白名单，改调 helper |
| `frontend/src/utils/modelCatalog.ts` | 新增 `CatalogStats` + `computeCatalogStats` |
| `frontend/src/views/admin/CatalogManageView.vue` | 新增统计区块 UI + `stats`/`platformEntries` computed |
| `frontend/src/i18n/locales/zh/custom.ts` | 补 `modelCatalog.billingModes.video` + `admin.catalogManage.stats.*` |
| `frontend/src/i18n/locales/en/custom.ts` | 同上 |

## 6. 测试

- **`utils/__tests__/billingMode.spec.ts`**（新建或扩现有）：`getModelCatalogBillingModeLabel` 覆盖 5 种已知模式（返回对应 i18n key 调用）、空/null/undefined（返回 `unknown` 兜底）、未知非空值（原样返回）。
- **`utils/__tests__/modelCatalog.spec.ts`**（扩现有）：`computeCatalogStats` 覆盖各计数正确性 + `byPlatform` 分桶 + 空数组。
- 验证命令：`cd frontend && pnpm run test:run`、`cd frontend && pnpm run typecheck`。
- 注意 [[pnpm-v11-overrides-migration]]：typecheck 直接跑 `vue-tsc`，避免 `pnpm` 命令删 lockfile；跑完 `git restore pnpm-lock.yaml` 还原。

## 7. 不在范围内

- 管理端计费模式/能力/健康维度统计（需后端接口扩展，本轮已按用户选择排除）。
- `constants/channel.ts` 与 `utils/billingMode.ts` 的常量重复定义整合（顺手清理收益不抵风险，避免范围蔓延）。
- 后端 `billing_mode` 过滤（公开页筛选纯前端，无需后端参与）。
