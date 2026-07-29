# 模型广场计费筛选适配 + 管理端运营统计 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让公开页模型广场的「计费模式」筛选覆盖平台全部 5 种计费模式，并给管理端模型广场新增纯前端的运营概览统计。

**Architecture:** Part 1 把 3 处漂移的计费模式白名单收敛到一个由规范常量驱动的共享 helper `getModelCatalogBillingModeLabel`，两个展示组件改调它，i18n 补 `video` 键。Part 2 抽纯函数 `computeCatalogStats` 从 `CatalogConfigItem[]` 聚合运营计数，管理端页面用 `computed` 渲染统计区块。两部分均纯前端、零后端改动。

**Tech Stack:** Vue 3.4 + TypeScript + Vite + TailwindCSS + vue-i18n + vitest；前端包管理 **pnpm**。

**Reference spec:** `docs/designs/2026-07-30-catalog-billing-filter-stats-design.md`

## Global Constraints

- **不改后端、不改 ent schema、不加 SQL 迁移**——本计划全部前端文件。
- **前端必须用 pnpm**（不是 npm）。typecheck 直接跑 `npx vue-tsc --noEmit`，**不要**跑会触发依赖重解析的 `pnpm` 命令（[[pnpm-v11-overrides-migration]]：pnpm v11 会从 lockfile 删安全补丁块）；若不慎跑了 `pnpm install` 类命令，结束前 `git restore pnpm-lock.yaml` 还原。
- **i18n 新增键一律在 `frontend/src/i18n/locales/{zh,en}/custom.ts` 深合并补缺，不改 main**（[[i18n-audit-and-custom-ts-fix]]）。
- 平台计费模式为单一轴 5 取值：`token / per_request / image / video / per_second`（后端 `service/channel.go`、前端 `constants/channel.ts` 与 `utils/billingMode.ts` 三处定义一致）。`unknown` 为前端兜底文案键，非真实模式。
- `docs/*` 被 gitignore（[[docs-design-gitignore-convention]]）；本计划文件本身已 `git add -f` 提交，普通源码改动不受影响。

## File Structure

| 文件 | 责任 | 改动类型 |
|---|---|---|
| `frontend/src/utils/billingMode.ts` | 计费模式常量 + 标签 helper | 新增 `CATALOG_BILLING_MODES` + `getModelCatalogBillingModeLabel` |
| `frontend/src/utils/__tests__/billingMode.spec.ts` | helper 单测 | 新建 |
| `frontend/src/components/models/ModelCatalogFilters.vue` | 公开页筛选 UI | 删本地白名单，改调 helper |
| `frontend/src/components/models/ModelPriceSummary.vue` | 公开页价格摘要 | 删本地白名单，改调 helper |
| `frontend/src/utils/modelCatalog.ts` | 目录构建/过滤/排序 | 新增 `CatalogStats` + `computeCatalogStats` |
| `frontend/src/utils/__tests__/modelCatalog.spec.ts` | 目录工具单测 | 扩展 `computeCatalogStats` 用例 |
| `frontend/src/views/admin/CatalogManageView.vue` | 管理端模型广场页 | 新增统计区块 + computed |
| `frontend/src/i18n/locales/zh/custom.ts` | 中文文案 | 补 `video` + `stats.*` |
| `frontend/src/i18n/locales/en/custom.ts` | 英文文案 | 补 `video` + `stats.*` |

---

## Task 1: 计费模式标签共享 helper + `video` i18n 键

**Files:**
- Modify: `frontend/src/utils/billingMode.ts`（在现有 `getBillingModeLabel` 之后，约 line 15 后追加）
- Modify: `frontend/src/i18n/locales/zh/custom.ts`（`modelCatalog.billingModes` 内补 `video`）
- Modify: `frontend/src/i18n/locales/en/custom.ts`（同上）
- Test: `frontend/src/utils/__tests__/billingMode.spec.ts`（Create）

**Interfaces:**
- Produces: `getModelCatalogBillingModeLabel(mode: string | null | undefined, t: (key: string) => string): string` —— 已知模式返回 `t('modelCatalog.billingModes.<mode>')`；空值返回 `t('modelCatalog.billingModes.unknown')`；未知非空值原样返回。

- [ ] **Step 1: 写失败测试**

创建 `frontend/src/utils/__tests__/billingMode.spec.ts`：

```ts
import { describe, expect, it } from 'vitest'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_PER_SECOND,
  BILLING_MODE_TOKEN,
  BILLING_MODE_VIDEO,
  getModelCatalogBillingModeLabel
} from '../billingMode'

// identity mock：返回 key 本身，便于断言命中了哪个 i18n key
const t = (key: string) => key

describe('getModelCatalogBillingModeLabel', () => {
  it('把每个规范计费模式映射到对应的 modelCatalog i18n key', () => {
    expect(getModelCatalogBillingModeLabel(BILLING_MODE_TOKEN, t)).toBe('modelCatalog.billingModes.token')
    expect(getModelCatalogBillingModeLabel(BILLING_MODE_PER_REQUEST, t)).toBe('modelCatalog.billingModes.per_request')
    expect(getModelCatalogBillingModeLabel(BILLING_MODE_IMAGE, t)).toBe('modelCatalog.billingModes.image')
    expect(getModelCatalogBillingModeLabel(BILLING_MODE_VIDEO, t)).toBe('modelCatalog.billingModes.video')
    expect(getModelCatalogBillingModeLabel(BILLING_MODE_PER_SECOND, t)).toBe('modelCatalog.billingModes.per_second')
  })

  it('mode 为空/null/undefined 时回退到 unknown 文案键', () => {
    expect(getModelCatalogBillingModeLabel('', t)).toBe('modelCatalog.billingModes.unknown')
    expect(getModelCatalogBillingModeLabel(null, t)).toBe('modelCatalog.billingModes.unknown')
    expect(getModelCatalogBillingModeLabel(undefined, t)).toBe('modelCatalog.billingModes.unknown')
  })

  it('未知但非空的 mode 原样返回（便于运营发现脏数据/新模式）', () => {
    expect(getModelCatalogBillingModeLabel('future_mode', t)).toBe('future_mode')
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd frontend && npx vitest run src/utils/__tests__/billingMode.spec.ts`
Expected: FAIL —— `getModelCatalogBillingModeLabel is not a function`（尚未导出）。

- [ ] **Step 3: 实现 helper**

在 `frontend/src/utils/billingMode.ts` 末尾（现有 `getBillingModeBadgeClass` 之后，line 25 后）追加：

```ts
/** 模型广场已知的计费模式集合（与后端 service.BillingMode* 一致）。 */
const CATALOG_BILLING_MODES = new Set<string>([
  BILLING_MODE_TOKEN,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_IMAGE,
  BILLING_MODE_VIDEO,
  BILLING_MODE_PER_SECOND,
])

/**
 * 模型广场计费模式标签（单一真相源）。
 * - 已知模式：走 modelCatalog.billingModes.<mode> 文案
 * - 空/null/undefined：回退 modelCatalog.billingModes.unknown（"价格待配置"）
 * - 未知非空：原样返回，便于发现新脏数据/未补文案的新模式
 */
export function getModelCatalogBillingModeLabel(
  mode: string | null | undefined,
  t: (key: string) => string,
): string {
  if (mode && CATALOG_BILLING_MODES.has(mode)) {
    return t(`modelCatalog.billingModes.${mode}`)
  }
  return mode || t('modelCatalog.billingModes.unknown')
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd frontend && npx vitest run src/utils/__tests__/billingMode.spec.ts`
Expected: PASS（3 个用例全过）。

- [ ] **Step 5: 补 `video` i18n 键**

在 `frontend/src/i18n/locales/zh/custom.ts` 的 `modelCatalog.billingModes` 块（约 line 128-134）内，`per_second` 下一行加：

```ts
      video: '视频计费',
```

在 `frontend/src/i18n/locales/en/custom.ts` 同结构处加：

```ts
      video: 'Video',
```

- [ ] **Step 6: typecheck**

Run: `cd frontend && npx vue-tsc --noEmit`
Expected: 无错误。（确认锁文件未被改：`git diff --name-only frontend/pnpm-lock.yaml` 应为空。）

- [ ] **Step 7: 提交**

```bash
git add frontend/src/utils/billingMode.ts frontend/src/utils/__tests__/billingMode.spec.ts \
        frontend/src/i18n/locales/zh/custom.ts frontend/src/i18n/locales/en/custom.ts
git commit -m "feat(catalog): shared billing-mode label helper for all 5 modes"
```

---

## Task 2: 两个展示组件改调共享 helper

**Files:**
- Modify: `frontend/src/components/models/ModelCatalogFilters.vue`（删 line 58 `knownBillingModes`、line 67-69 `billingModeLabel`、改 line 30 模板、加 import）
- Modify: `frontend/src/components/models/ModelPriceSummary.vue`（删 line 54 `knownBillingModes`、改 line 56-59 `billingLabel` computed、加 import）

**Interfaces:**
- Consumes: `getModelCatalogBillingModeLabel` from Task 1。

- [ ] **Step 1: 改 `ModelCatalogFilters.vue`**

在 `<script setup lang="ts">` 顶部 import 区加：

```ts
import { getModelCatalogBillingModeLabel } from '@/utils/billingMode'
```

删掉 line 58 的 `const knownBillingModes = new Set(['token', 'image', 'per_request', 'unknown'])` 与 line 67-69 的 `function billingModeLabel(mode: string) { ... }`。

模板 line 30 的 `{{ billingModeLabel(mode) }}` 改为：

```vue
        {{ getModelCatalogBillingModeLabel(mode, t) }}
```

- [ ] **Step 2: 改 `ModelPriceSummary.vue`**

import 区（line 34 附近）加：

```ts
import { getModelCatalogBillingModeLabel } from '@/utils/billingMode'
```

删掉 line 54 的 `const knownBillingModes = new Set([...])`。

把 line 56-59 的 `billingLabel` computed 改为：

```ts
const billingLabel = computed(() =>
  getModelCatalogBillingModeLabel(props.pricing?.billing_mode, t),
)
```

（模板 line 3 `{{ billingLabel }}` 不变。）

- [ ] **Step 3: typecheck + 全量单测无回归**

Run: `cd frontend && npx vue-tsc --noEmit && npx vitest run`
Expected: typecheck 无错误；vitest 全过（Task 1 的 helper 测试 + 现有 modelCatalog 等套件）。确认 `git diff --name-only frontend/pnpm-lock.yaml` 为空。

- [ ] **Step 4: 提交**

```bash
git add frontend/src/components/models/ModelCatalogFilters.vue \
        frontend/src/components/models/ModelPriceSummary.vue
git commit -m "refactor(catalog): adopt shared billing-mode label in filters + price summary"
```

---

## Task 3: `computeCatalogStats` 纯函数 + 单测

**Files:**
- Modify: `frontend/src/utils/modelCatalog.ts`（文件末尾追加）
- Test: `frontend/src/utils/__tests__/modelCatalog.spec.ts`（扩展）

**Interfaces:**
- Consumes: `CatalogConfigItem`（from `@/api/adminCatalog`）：`{ platform: string; model_name: string; pinned: boolean; sort_weight: number; custom_tags: string[]; featured_until: string | null; hidden: boolean; first_seen_at: string; tags?: string[]; is_new: boolean; featured: boolean }`
- Produces:
  - `interface CatalogStats { total: number; visible: number; hidden: number; pinned: number; isNew: number; featured: number; recommended: number; byPlatform: Record<string, number> }`
  - `function computeCatalogStats(items: CatalogConfigItem[]): CatalogStats`

- [ ] **Step 1: 写失败测试**

在 `frontend/src/utils/__tests__/modelCatalog.spec.ts` 末尾追加（import 区补 `computeCatalogStats` 与 `CatalogConfigItem` 类型）：

先在文件顶部 import 块补：

```ts
import { computeCatalogStats } from '../modelCatalog'
import type { CatalogConfigItem } from '@/api/adminCatalog'
```

（若 `from '../modelCatalog'` 的具名 import 已存在，把 `computeCatalogStats` 加进同一花括号即可。）

文件末尾追加：

```ts
describe('computeCatalogStats', () => {
  const items: CatalogConfigItem[] = [
    {
      platform: 'openai', model_name: 'gpt-4o', pinned: true, sort_weight: 100,
      custom_tags: ['recommended'], featured_until: null, hidden: false,
      first_seen_at: '2026-07-01T00:00:00Z', tags: [], is_new: true, featured: true,
    },
    {
      platform: 'openai', model_name: 'gpt-3.5', pinned: false, sort_weight: 0,
      custom_tags: [], featured_until: null, hidden: true,
      first_seen_at: '2026-07-10T00:00:00Z', is_new: false, featured: false,
    },
    {
      platform: 'anthropic', model_name: 'claude-sonnet', pinned: false, sort_weight: 50,
      custom_tags: ['recommended'], featured_until: null, hidden: false,
      first_seen_at: '2026-07-20T00:00:00Z', is_new: false, featured: true,
    },
  ]

  it('聚合各项运营计数', () => {
    const stats = computeCatalogStats(items)
    expect(stats.total).toBe(3)
    expect(stats.visible).toBe(2)
    expect(stats.hidden).toBe(1)
    expect(stats.pinned).toBe(1)
    expect(stats.isNew).toBe(1)
    expect(stats.featured).toBe(2)
    expect(stats.recommended).toBe(2)
  })

  it('按平台分桶', () => {
    const { byPlatform } = computeCatalogStats(items)
    expect(byPlatform).toEqual({ openai: 2, anthropic: 1 })
  })

  it('空数组返回零值且无平台桶', () => {
    const stats = computeCatalogStats([])
    expect(stats).toEqual({
      total: 0, visible: 0, hidden: 0, pinned: 0,
      isNew: 0, featured: 0, recommended: 0, byPlatform: {},
    })
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd frontend && npx vitest run src/utils/__tests__/modelCatalog.spec.ts`
Expected: FAIL —— `computeCatalogStats is not defined`。

- [ ] **Step 3: 实现纯函数**

在 `frontend/src/utils/modelCatalog.ts` 末尾追加：

```ts
import type { CatalogConfigItem } from '@/api/adminCatalog'

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

/** 从管理端运营配置列表聚合概览统计（纯函数，供 CatalogManageView 渲染）。 */
export function computeCatalogStats(items: CatalogConfigItem[]): CatalogStats {
  const stats: CatalogStats = {
    total: items.length,
    visible: 0,
    hidden: 0,
    pinned: 0,
    isNew: 0,
    featured: 0,
    recommended: 0,
    byPlatform: {},
  }

  for (const it of items) {
    if (it.hidden) stats.hidden += 1
    else stats.visible += 1
    if (it.pinned) stats.pinned += 1
    if (it.is_new) stats.isNew += 1
    if (it.featured) stats.featured += 1
    if (it.custom_tags?.includes('recommended')) stats.recommended += 1
    const platform = it.platform || 'unknown'
    stats.byPlatform[platform] = (stats.byPlatform[platform] || 0) + 1
  }

  return stats
}
```

> 注：若文件顶部已有其它 `import type` from `@/api/...`，把 `CatalogConfigItem` 合并到顶部 import 区更符合惯例；末尾单独 import 在 ES module 里也合法（import 会被提升）。实现者可二选一，保持文件内仅一处该 import。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd frontend && npx vitest run src/utils/__tests__/modelCatalog.spec.ts`
Expected: PASS（新增 3 个 `computeCatalogStats` 用例 + 原有用例全过）。

- [ ] **Step 5: 提交**

```bash
git add frontend/src/utils/modelCatalog.ts frontend/src/utils/__tests__/modelCatalog.spec.ts
git commit -m "feat(catalog): add computeCatalogStats pure aggregator"
```

---

## Task 4: 管理端模型广场运营概览统计区块

**Files:**
- Modify: `frontend/src/views/admin/CatalogManageView.vue`（模板：在 line 28 `<template v-else>` 内、line 30 列表 `<section class="card">` 之前插入统计区块；script：加 import + 两个 computed）
- Modify: `frontend/src/i18n/locales/zh/custom.ts`（`admin.catalogManage` 下加 `stats` 块）
- Modify: `frontend/src/i18n/locales/en/custom.ts`（同上）

**Interfaces:**
- Consumes: `computeCatalogStats` + `CatalogStats` from Task 3；`items: Ref<CatalogConfigItem[]>`（页面已有，line 118）。

- [ ] **Step 1: 加 `stats.*` i18n 键**

在 `frontend/src/i18n/locales/zh/custom.ts` 的 `admin.catalogManage` 对象内（与 `allModelsSection`/`sortHint` 等同级）加：

```ts
      stats: {
        total: '模型总数',
        visible: '可见',
        hidden: '已隐藏',
        pinned: '置顶',
        isNew: 'NEW',
        featured: '精选',
        recommended: '推荐',
        byPlatform: '按平台分布',
      },
```

在 `frontend/src/i18n/locales/en/custom.ts` 的 `admin.catalogManage` 对象内加：

```ts
      stats: {
        total: 'Total',
        visible: 'Visible',
        hidden: 'Hidden',
        pinned: 'Pinned',
        isNew: 'New',
        featured: 'Featured',
        recommended: 'Recommended',
        byPlatform: 'By platform',
      },
```

- [ ] **Step 2: 脚本加 import + computed**

在 `frontend/src/views/admin/CatalogManageView.vue` 的 `<script setup lang="ts">` import 区（line 111 `@/api/adminCatalog` 附近）加：

```ts
import { computeCatalogStats } from '@/utils/modelCatalog'
```

在 `const visibleList = computed(...)`（line 150）之后加：

```ts
// 运营概览统计：基于全量 items（不受搜索框影响），随开关就地变更实时刷新。
const stats = computed(() => computeCatalogStats(items.value))
const platformEntries = computed(() =>
  Object.entries(stats.value.byPlatform).sort((a, b) => b[1] - a[1]),
)
```

- [ ] **Step 3: 模板插入统计区块**

在 `<template v-else>`（line 28）之后、列表 `<section class="card">`（line 30）之前插入：

```vue
      <!-- 运营概览统计 -->
      <section class="card">
        <div class="grid grid-cols-2 gap-3 p-4 sm:grid-cols-4 lg:grid-cols-7">
          <div v-for="tile in statTiles" :key="tile.key" class="rounded-md border border-gray-200 p-3 dark:border-dark-600">
            <span class="block text-xs text-gray-500 dark:text-gray-400">{{ t(tile.label) }}</span>
            <strong class="text-lg font-semibold text-gray-900 dark:text-white">{{ tile.value }}</strong>
          </div>
        </div>
        <div class="flex flex-wrap items-center gap-2 border-t border-gray-100 px-4 py-3 dark:border-dark-700">
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.catalogManage.stats.byPlatform') }}</span>
          <span
            v-for="[platform, count] in platformEntries"
            :key="platform"
            class="inline-flex items-center rounded-full bg-gray-100 px-2.5 py-0.5 text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-300"
          >
            {{ platform }} · {{ count }}
          </span>
        </div>
      </section>
```

并在 script（Step 2 新增 computed 之后）加 `statTiles` 派生数组，供模板 `v-for`：

```ts
const statTiles = computed(() => [
  { key: 'total', label: 'admin.catalogManage.stats.total', value: stats.value.total },
  { key: 'visible', label: 'admin.catalogManage.stats.visible', value: stats.value.visible },
  { key: 'hidden', label: 'admin.catalogManage.stats.hidden', value: stats.value.hidden },
  { key: 'pinned', label: 'admin.catalogManage.stats.pinned', value: stats.value.pinned },
  { key: 'isNew', label: 'admin.catalogManage.stats.isNew', value: stats.value.isNew },
  { key: 'featured', label: 'admin.catalogManage.stats.featured', value: stats.value.featured },
  { key: 'recommended', label: 'admin.catalogManage.stats.recommended', value: stats.value.recommended },
])
```

- [ ] **Step 4: typecheck + 全量单测无回归**

Run: `cd frontend && npx vue-tsc --noEmit && npx vitest run`
Expected: typecheck 无错误；vitest 全过。确认 `git diff --name-only frontend/pnpm-lock.yaml` 为空。

- [ ] **Step 5: 提交**

```bash
git add frontend/src/views/admin/CatalogManageView.vue \
        frontend/src/i18n/locales/zh/custom.ts frontend/src/i18n/locales/en/custom.ts
git commit -m "feat(catalog): admin operational overview stats (frontend-only)"
```

---

## Self-Review（计划完成后自检，已修正）

1. **Spec 覆盖**：
   - Part 1 helper + i18n video → Task 1 ✓
   - Part 1 两组件采纳 → Task 2 ✓
   - Part 2 纯函数 → Task 3 ✓
   - Part 2 管理端 UI + i18n → Task 4 ✓
   - 3 处漂移白名单（Filters / PriceSummary / i18n video 键）全部有任务覆盖 ✓
2. **占位扫描**：无 TBD/TODO；所有代码块均给出实际内容 ✓
3. **类型一致**：`getModelCatalogBillingModeLabel` 在 Task 1 定义、Task 2 消费签名一致；`computeCatalogStats` / `CatalogStats` 在 Task 3 定义、Task 4 消费一致；`CatalogConfigItem` 字段与 `@/api/adminCatalog` 实际定义对齐（`tags?` 可选、`custom_tags` 必需、`featured_until: string | null`）✓

## 执行注意事项

- 每个 Task 末尾的 `git commit` 不要加 `Co-Authored-By` 之外的多余信息；本仓库提交信息用 `type(catalog): ...` 规范（见近期提交）。
- 计划内命令均用 `npx` 直接驱动工具，避免 `pnpm install` 类命令重解析依赖树。
- 跑测试/类型检查无需数据库或后端。
