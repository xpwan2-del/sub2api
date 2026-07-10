# per_request 按次模式移除层级 — 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让渠道定价的 `per_request`（按次）模式仅保留"默认单次价格"，移除层级（tier）能力；`image`（图片按次）模式完全不变。

**Architecture:** 纯前端三处改动。① 把"per_request 提交时清空 intervals"的规则下沉到 `types.ts` 的 `formIntervalsToAPI`（新增可选 `mode` 参数），配单测，`ChannelsView` 两个提交转换点改为传入 `billing_mode`。② `PricingEntryCard.vue` 删除 per_request 分支的层级 UI（标题/按钮/列表/空状态）。③ i18n 改文案"默认单次价格（未命中层级时使用）"→"默认单次价格"，并清理 per_request 专用的 dead key。后端无需改动。

**Tech Stack:** Vue 3.4 + TypeScript + vue-i18n + Vite + Vitest；包管理用 pnpm。

## Global Constraints

- `image` 模式**完全不变**：层级入口、`imageTiers`/`addTier`/`defaultImagePrice` 文案、计费逻辑均保留。
- `token` 模式**不变**。
- **后端不改**：`per_request` 无 tier 时后端回退到 `DefaultPerRequestPrice`（`model_pricing_resolver.go:266-277` + `billing_service.go:994`），清空 intervals 后按默认单次价格计费。
- `per_request` 保存时 intervals 必须**一律清空**（含编辑历史数据场景）。
- 前端必须用 **pnpm**（不是 npm）。
- 文案改动须中英文（`zh.ts` + `en.ts`）同步。

## File Structure

| 文件 | 责任 | 本计划改动 |
|---|---|---|
| `frontend/src/components/admin/channel/types.ts` | 定价表单纯函数（转换/校验） | `formIntervalsToAPI` 新增 `mode` 参数，per_request 返回 `[]` |
| `frontend/src/components/admin/channel/__tests__/types.spec.ts` | types.ts 单测 | 新增 `formIntervalsToAPI` 的 per_request/image/token 用例 |
| `frontend/src/views/admin/ChannelsView.vue` | 渠道创建/编辑表单 + 提交 | 两个提交转换点传 `billing_mode` 给 `formIntervalsToAPI` |
| `frontend/src/components/admin/channel/PricingEntryCard.vue` | 单条定价配置卡片 | 删除 per_request 分支层级 UI，更新模板内 fallback 文案 |
| `frontend/src/i18n/locales/zh.ts` / `en.ts` | 国际化 | 改 `defaultPerRequestPrice`；删 `requestTiers`、`noTiersYet` |

---

## Task 1: `formIntervalsToAPI` 按 mode 清空 per_request intervals（含 ChannelsView 接线）

**Files:**
- Modify: `frontend/src/components/admin/channel/types.ts:63-75`
- Modify: `frontend/src/views/admin/ChannelsView.vue:1071,1111`
- Test: `frontend/src/components/admin/channel/__tests__/types.spec.ts`

**Interfaces:**
- Consumes: `IntervalFormEntry`、`BillingMode`、`PricingInterval`（types.ts 已有）
- Produces: `formIntervalsToAPI(intervals, mode?)` — 第二参可选，默认 `'token'`；`mode === 'per_request'` 时返回 `[]`，其余模式行为不变（向后兼容现有不传 mode 的调用）

- [ ] **Step 1: 写失败测试**

在 `frontend/src/components/admin/channel/__tests__/types.spec.ts` 顶部把 import 改为引入 `formIntervalsToAPI`：

```ts
import { describe, expect, it } from 'vitest'
import { formIntervalsToAPI, validateIntervals, type IntervalFormEntry } from '../types'
```

在文件末尾（第 79 行 `})` 之后）追加新 describe 块：

```ts
describe('formIntervalsToAPI', () => {
  it('returns empty array for per_request mode (tiers not supported)', () => {
    const intervals: IntervalFormEntry[] = [
      makeInterval({ tier_label: '1K', per_request_price: 0.04 }),
      makeInterval({ tier_label: '2K', per_request_price: 0.06 }),
    ]
    expect(formIntervalsToAPI(intervals, 'per_request')).toEqual([])
  })

  it('still converts intervals for image mode', () => {
    const intervals: IntervalFormEntry[] = [
      makeInterval({ tier_label: '1K', per_request_price: 0.04 }),
    ]
    const result = formIntervalsToAPI(intervals, 'image')
    expect(result).toHaveLength(1)
    expect(result[0].tier_label).toBe('1K')
    expect(result[0].per_request_price).toBe(0.04)
  })

  it('converts intervals for token mode when mode omitted', () => {
    const intervals: IntervalFormEntry[] = [
      makeInterval({ min_tokens: 0, max_tokens: 200000, input_price: 1 }),
    ]
    const result = formIntervalsToAPI(intervals)
    expect(result).toHaveLength(1)
    expect(result[0].per_request_price).toBeNull()
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd frontend && pnpm run test:run -- src/components/admin/channel/__tests__/types.spec.ts`
Expected: FAIL — `returns empty array for per_request mode` 用例失败，形如 `Expected: [] Received: [{ min_tokens: 0, ... }]`（当前 `formIntervalsToAPI` 不识别 mode，仍把 intervals 全量转换）。

- [ ] **Step 3: 实现函数**

把 `frontend/src/components/admin/channel/types.ts` 的 `formIntervalsToAPI`（第 63-75 行）整体替换为：

```ts
/** 表单区间 → API 区间。
 *
 * mode 决定是否提交层级：
 * - per_request：不支持层级，UI 已移除层级入口，提交时（含历史数据）一律清空
 * - token / image：按原逻辑转换
 */
export function formIntervalsToAPI(
  intervals: IntervalFormEntry[],
  mode: BillingMode = 'token',
): PricingInterval[] {
  if (mode === 'per_request') return []
  return (intervals || []).map(iv => ({
    min_tokens: iv.min_tokens,
    max_tokens: iv.max_tokens,
    tier_label: iv.tier_label,
    input_price: mTokToPerToken(iv.input_price),
    output_price: mTokToPerToken(iv.output_price),
    cache_write_price: mTokToPerToken(iv.cache_write_price),
    cache_read_price: mTokToPerToken(iv.cache_read_price),
    per_request_price: toNullableNumber(iv.per_request_price),
    sort_order: iv.sort_order
  }))
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd frontend && pnpm run test:run -- src/components/admin/channel/__tests__/types.spec.ts`
Expected: PASS — 三个新用例全绿，原有 `validateIntervals` 用例不受影响。

- [ ] **Step 5: 接线 ChannelsView 两个提交转换点**

`frontend/src/views/admin/ChannelsView.vue` 第 1071 行（`accountStatsRulesToAPI` 内）：

旧：
```ts
            intervals: formIntervalsToAPI(p.intervals || [])
```
新：
```ts
            intervals: formIntervalsToAPI(p.intervals || [], p.billing_mode)
```

第 1111 行（`formToAPI` 内）：

旧：
```ts
        intervals: formIntervalsToAPI(entry.intervals || [])
```
新：
```ts
        intervals: formIntervalsToAPI(entry.intervals || [], entry.billing_mode)
```

- [ ] **Step 6: typecheck 全量确认**

Run: `cd frontend && pnpm run typecheck`
Expected: 通过，无 TS 错误（`mode` 有默认值，现有不传 mode 的调用仍兼容；`p`/`entry` 均含 `billing_mode` 字段）。

- [ ] **Step 7: Commit**

```bash
git add frontend/src/components/admin/channel/types.ts \
        frontend/src/components/admin/channel/__tests__/types.spec.ts \
        frontend/src/views/admin/ChannelsView.vue
git commit -m "feat(channel): per_request 模式保存时清空计费层级

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 2: PricingEntryCard 移除 per_request 层级 UI 入口

**Files:**
- Modify: `frontend/src/components/admin/channel/PricingEntryCard.vue:156-190`

**Interfaces:**
- Consumes: Task 1 的数据规则（per_request 提交时 intervals 已清空，UI 删层级入口后数据双向一致）
- Produces: per_request 模式仅渲染"默认单次价格"输入框，不再有"添加层级"按钮与层级列表

**说明:** `addInterval`（第 271 行）仍被 token 模式的"添加区间"按钮（第 139 行）使用，`addImageTier`（第 282 行）仍被 image 模式（第 209 行）使用——两者保留不动。本任务只删 per_request 分支内的层级 UI 与其 `t()` 引用。

- [ ] **Step 1: 替换 per_request 分支模板**

把 `frontend/src/components/admin/channel/PricingEntryCard.vue` 第 156-190 行整段（从 `<!-- Per-request mode -->` 到该 `</div>` 结束）替换为：

```html
        <!-- Per-request mode (single flat price, no tiers) -->
        <div v-else-if="entry.billing_mode === 'per_request'">
          <!-- Default per-request price -->
          <label class="mt-3 block text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('admin.channels.form.defaultPerRequestPrice', '默认单次价格') }}
            <span class="ml-1 font-normal text-gray-400">$</span>
          </label>
          <div class="mt-1 w-48">
            <input :value="entry.per_request_price" @input="emitField('per_request_price', ($event.target as HTMLInputElement).value)"
              type="number" step="any" min="0" class="input text-sm" :placeholder="t('admin.channels.form.pricePlaceholder', '默认')" />
          </div>
        </div>
```

（即删除原 168-189 行的「按次计费层级」标题 +「+ 添加层级」按钮 + IntervalRow 列表 +「暂无层级…」空状态；同时把 `defaultPerRequestPrice` 的模板内 fallback 字符串从 `'默认单次价格（未命中层级时使用）'` 改为 `'默认单次价格'`。）

- [ ] **Step 2: 确认无残留 per_request 层级引用**

Run: `grep -n "requestTiers\|noTiersYet" frontend/src/components/admin/channel/PricingEntryCard.vue`
Expected: 无输出（模板里的 `t('...requestTiers')`、`t('...noTiersYet')` 随层级 UI 一并删除）。

- [ ] **Step 3: typecheck**

Run: `cd frontend && pnpm run typecheck`
Expected: 通过。

- [ ] **Step 4: lint**

Run: `cd frontend && pnpm run lint:check`
Expected: 通过（若提示未使用变量，按提示清理；`addInterval`/`addImageTier` 仍在用，不应被误报）。

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/admin/channel/PricingEntryCard.vue
git commit -m "feat(channel): per_request 模式移除层级 UI 入口

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 3: i18n 文案更新 + 清理 per_request 专用 dead key

**Files:**
- Modify: `frontend/src/i18n/locales/zh.ts:2685,2688,2709`
- Modify: `frontend/src/i18n/locales/en.ts:2608,2611,2632`

**Interfaces:**
- Consumes: Task 2 已删除模板内对 `requestTiers`/`noTiersYet` 的引用
- Produces: `defaultPerRequestPrice` 文案去括号；`requestTiers`/`noTiersYet` 从中英文 locale 移除；`addTier`/`imageTiers`/`defaultImagePrice` 保留（image 模式仍用）

- [ ] **Step 1: 改 zh.ts**

`frontend/src/i18n/locales/zh.ts`：

第 2709 行：
旧：`        defaultPerRequestPrice: '默认单次价格（未命中层级时使用）',`
新：`        defaultPerRequestPrice: '默认单次价格',`

第 2685 行删除整行：`        requestTiers: '按次计费层级',`
第 2688 行删除整行：`        noTiersYet: '暂无层级，点击添加配置按次计费价格',`

- [ ] **Step 2: 改 en.ts**

`frontend/src/i18n/locales/en.ts`：

第 2632 行：
旧：`        defaultPerRequestPrice: 'Default per-request price (fallback when no tier matches)',`
新：`        defaultPerRequestPrice: 'Default per-request price',`

第 2608 行删除整行：`        requestTiers: 'Request Tiers',`
第 2611 行删除整行：`        noTiersYet: 'No tiers yet. Click add to configure per-request pricing.',`

- [ ] **Step 3: 确认 dead key 全仓库无引用**

Run: `grep -rn "requestTiers\|noTiersYet" frontend/src`
Expected: 仅可能命中 i18n locale 文件之外无引用（若仍命中 `.vue`/`.ts` 业务代码，需先清除引用再删 key；本计划 Task 2 已删 PricingEntryCard 内引用，预期为空）。

- [ ] **Step 4: typecheck + lint**

Run: `cd frontend && pnpm run typecheck && pnpm run lint:check`
Expected: 通过（vue-i18n key 为字符串，删除不影响类型）。

- [ ] **Step 5: Commit**

```bash
git add frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "feat(channel): 更新按次单次价格文案并清理无用层级文案

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## 验收（全部 Task 完成后手动确认）

- 新建渠道 → 添加定价配置 → 计费模式选"按次"：仅显示「默认单次价格」输入框，**无**「添加层级」按钮、无层级列表。
- 同卡片切到"图片（按次）"：层级入口（「+ 添加层级」）仍在，可正常增删层级——确认 image 模式未受影响。
- 编辑一个历史上带 per_request 层级的渠道：卡片不显示层级，保存后接口请求体中该条目 `intervals` 为 `[]`（浏览器 DevTools Network 面板核对），后端按默认单次价格计费。
- `cd frontend && pnpm run typecheck && pnpm run lint:check && pnpm run test:run` 全绿。
