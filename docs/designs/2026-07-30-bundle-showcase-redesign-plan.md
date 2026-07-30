# 模型广场套餐区重新设计 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把模型广场(`/`)顶部套餐展示区从复用的白底 `BundlePlanCard` 换成与模型卡视觉统一的深色 HUD 套餐卡,新建 `ShowcaseBundleCard.vue`,不改 `BundlePlanCard`。

**Architecture:** 展示逻辑(featured 判定 + 平台色)提取为 `src/utils/showcaseBundle.ts` 纯函数(先测后写);新建 `ShowcaseBundleCard.vue` 用 scoped CSS + `isDark` prop 复刻模型广场深色 HUD DNA;改写 `BundleShowcaseSection.vue` 用新卡 + 对齐 `ModelCatalogHeader` 的 header;`ModelCatalogView` 透传 `isDark`;新增 `modelCatalog.*` i18n key。

**Tech Stack:** Vue 3.4 `<script setup>` + TypeScript + vue-i18n + scoped CSS;Vitest + @vue/test-utils 测试。

## Global Constraints

- 包管理:**pnpm**(禁用 npm);改 `package.json` 才需 `pnpm install`。本计划不改 `package.json`。
- 单测单文件运行:`pnpm exec vitest run <path>`(注意:`pnpm run test:run -- <path>` 透传不生效)。
- `typecheck` 不覆盖测试文件 → 改测试必须跑 vitest 验证,typecheck 过 ≠ 测试无语法错。
- **暗色机制**:新组件一律用 `isDark` prop + scoped CSS `.is-dark` 选择器,**禁止** Tailwind `dark:` 变体(消除现存的「两套暗色实现」)。
- **零污染**:不得改动 `src/components/bundles/BundlePlanCard.vue`、`src/views/user/BundlesView.vue`、`src/views/user/BundleUsageView.vue`。
- 接口数据约束:`PublicBundlePlan` **无 `id` 字段**,用 `sort_order` + `name` 作 key/匹配;**不含** concurrency/rpm/group_quotas,卡片不渲染这些。
- i18n 命名空间:`modelCatalog.*`,文件 `src/i18n/locales/{zh,en}/custom.ts`,zh/en 必须同步。

参考 spec:`docs/designs/2026-07-30-bundle-showcase-redesign-design.md`。

---

## File Structure

| 文件 | 动作 | 责任 |
|---|---|---|
| `src/utils/showcaseBundle.ts` | 新建 | 纯函数:`pickFeaturedPlan`、`platformDotColor` |
| `src/utils/__tests__/showcaseBundle.spec.ts` | 新建 | 上述纯函数单测 |
| `src/components/models/ShowcaseBundleCard.vue` | 新建 | 深色 HUD 套餐卡(scoped CSS + isDark) |
| `src/components/__tests__/ShowcaseBundleCard.spec.ts` | 新建 | 卡片渲染/featured/click/空数据降级单测 |
| `src/components/models/BundleShowcaseSection.vue` | 改写 | 新 header + 网格 + featured 判定 + isDark 透传,改用 ShowcaseBundleCard |
| `src/components/__tests__/BundleShowcaseSection.spec.ts` | 新建 | section featured 标记/统计/渲染数单测 |
| `src/views/public/ModelCatalogView.vue` | 小改 | 给 BundleShowcaseSection 传 `:is-dark="isDark"` |
| `src/i18n/locales/zh/custom.ts` | 改 | modelCatalog 命名空间新增 5 个 key |
| `src/i18n/locales/en/custom.ts` | 改 | 同步 5 个 key(英文) |

---

## Task 1: showcaseBundle 纯函数 + 单测

**Files:**
- Create: `src/utils/showcaseBundle.ts`
- Test: `src/utils/__tests__/showcaseBundle.spec.ts`

**Interfaces:**
- Produces: `pickFeaturedPlan(plans: PublicBundlePlan[]): PublicBundlePlan | null`、`platformDotColor(platform: string): string`(被 Task 2/3 使用)

- [ ] **Step 1: 写失败测试**

Create `src/utils/__tests__/showcaseBundle.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { pickFeaturedPlan, platformDotColor } from '@/utils/showcaseBundle'
import type { PublicBundlePlan } from '@/api/publicBundles'

function plan(over: Partial<PublicBundlePlan> = {}): PublicBundlePlan {
  return {
    name: 'x', tier: 'starter', description: '', price: 10,
    original_price: 0, currency: 'USD', validity_days: 30,
    features: [], sort_order: 0, platforms: [], ...over,
  }
}

describe('pickFeaturedPlan', () => {
  it('套餐数 < 2 时返回 null（单套餐无需突出）', () => {
    expect(pickFeaturedPlan([])).toBeNull()
    expect(pickFeaturedPlan([plan()])).toBeNull()
  })
  it('优先选 tier=pro 的套餐', () => {
    const pro = plan({ tier: 'pro', name: '专业版', sort_order: 1 })
    const result = pickFeaturedPlan([
      plan({ tier: 'starter', sort_order: 0 }),
      pro,
      plan({ tier: 'enterprise', price: 200, sort_order: 2 }),
    ])
    expect(result).toBe(pro)
  })
  it('无 pro 时取价格升序的中间档', () => {
    const low = plan({ price: 9, name: '低', sort_order: 0 })
    const mid = plan({ price: 49, name: '中', sort_order: 1 })
    const high = plan({ price: 199, name: '高', sort_order: 2 })
    expect(pickFeaturedPlan([high, low, mid])).toBe(mid)
  })
})

describe('platformDotColor', () => {
  it('已知平台返回对应颜色', () => {
    expect(platformDotColor('openai')).toBe('#10b981')
    expect(platformDotColor('anthropic')).toBe('#f97316')
    expect(platformDotColor('gemini')).toBe('#3b82f6')
  })
  it('未知平台回落中性色', () => {
    expect(platformDotColor('whatever')).toBe('#64748b')
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/utils/__tests__/showcaseBundle.spec.ts`
Expected: FAIL(模块不存在 / 函数未定义)

- [ ] **Step 3: 写实现**

Create `src/utils/showcaseBundle.ts`:

```ts
/**
 * 模型广场套餐展示区的纯展示逻辑。
 *
 * pickFeaturedPlan:选出「主推」套餐（贴 ★推荐 badge + 加强 glow）。
 *   规则：套餐数 < 2 → null；否则优先 tier==='pro'；否则取价格升序中间档。
 * platformDotColor:平台 → 圆点 CSS 颜色（深色 HUD scoped CSS 用；
 *   platformColors.ts 只提供 Tailwind 类，此处用不上）。
 */
import type { PublicBundlePlan } from '@/api/publicBundles'

/** 选出主推套餐。套餐数 < 2 时返回 null。 */
export function pickFeaturedPlan(plans: PublicBundlePlan[]): PublicBundlePlan | null {
  if (!plans || plans.length < 2) return null
  const pro = plans.find(p => p.tier === 'pro')
  if (pro) return pro
  const sorted = [...plans].sort((a, b) => a.price - b.price)
  return sorted[Math.floor((sorted.length - 1) / 2)]
}

/** 平台 → 圆点 CSS 颜色。未知平台回落 slate-500。 */
export function platformDotColor(platform: string): string {
  switch (platform) {
    case 'anthropic': return '#f97316'   // orange-500
    case 'openai': return '#10b981'      // emerald-500
    case 'antigravity': return '#a855f7' // purple-500
    case 'gemini': return '#3b82f6'      // blue-500
    case 'grok': return '#71717a'        // zinc-500
    default: return '#64748b'            // slate-500
  }
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `pnpm exec vitest run src/utils/__tests__/showcaseBundle.spec.ts`
Expected: PASS(5 passed)

- [ ] **Step 5: Commit**

```bash
git add src/utils/showcaseBundle.ts src/utils/__tests__/showcaseBundle.spec.ts
git commit -m "feat(catalog): add showcaseBundle helpers (featured pick + platform dot color)"
```

---

## Task 2: ShowcaseBundleCard 组件 + 单测

**Files:**
- Create: `src/components/models/ShowcaseBundleCard.vue`
- Test: `src/components/__tests__/ShowcaseBundleCard.spec.ts`

**Interfaces:**
- Consumes: `platformDotColor` from Task 1; `platformLabel` from `@/utils/platformColors`; i18n keys `modelCatalog.bundleViewDetails` / `bundleFeaturedTag` / `bundleDays`(Task 4 加,本任务测试用 mock 兜底)
- Produces: Vue 组件,props `{ plan: PublicBundlePlan; featured?: boolean; isDark?: boolean }`,emit `click(plan)`

- [ ] **Step 1: 写失败测试**

Create `src/components/__tests__/ShowcaseBundleCard.spec.ts`:

```ts
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ShowcaseBundleCard from '@/components/models/ShowcaseBundleCard.vue'
import type { PublicBundlePlan } from '@/api/publicBundles'

// setup.ts 未全局注册 i18n，组件用 useI18n → 这里 mock 成返回 key
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}))

const plan: PublicBundlePlan = {
  name: '专业版', tier: 'pro', description: '一句话描述', price: 49,
  original_price: 98, currency: 'USD', validity_days: 30,
  features: ['特性A', '特性B'], sort_order: 1, platforms: ['openai', 'anthropic'],
}

describe('ShowcaseBundleCard', () => {
  it('渲染套餐名 / 价格 / 划线原价 / 特性 / 平台', () => {
    const w = mount(ShowcaseBundleCard, { props: { plan } })
    expect(w.text()).toContain('专业版')
    expect(w.text()).toContain('$49')
    expect(w.text()).toContain('$98')
    expect(w.text()).toContain('特性A')
    expect(w.text()).toContain('OpenAI')
  })

  it('featured=true 时显示推荐 badge 并加 is-featured 类', () => {
    const w = mount(ShowcaseBundleCard, { props: { plan, featured: true } })
    expect(w.find('.sbc-featured-badge').exists()).toBe(true)
    expect(w.classes()).toContain('is-featured')
  })

  it('默认无推荐 badge', () => {
    const w = mount(ShowcaseBundleCard, { props: { plan } })
    expect(w.find('.sbc-featured-badge').exists()).toBe(false)
    expect(w.classes()).not.toContain('is-featured')
  })

  it('点击卡片触发 click 并携带 plan', async () => {
    const w = mount(ShowcaseBundleCard, { props: { plan } })
    await w.trigger('click')
    expect(w.emitted('click')).toBeTruthy()
    expect((w.emitted('click')![0] as unknown[])[0]).toStrictEqual(plan)
  })

  it('original_price 不大于 price 时不渲染划线价', () => {
    const w = mount(ShowcaseBundleCard, { props: { plan: { ...plan, original_price: 0 } } })
    expect(w.find('.sbc-price-strike').exists()).toBe(false)
  })

  it('features 为空时不渲染特性列表', () => {
    const w = mount(ShowcaseBundleCard, { props: { plan: { ...plan, features: [] } } })
    expect(w.find('.sbc-feats').exists()).toBe(false)
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/components/__tests__/ShowcaseBundleCard.spec.ts`
Expected: FAIL(组件文件不存在)

- [ ] **Step 3: 写实现**

Create `src/components/models/ShowcaseBundleCard.vue`:

```vue
<script lang="ts">
/**
 * 模型广场套餐展示卡（公开页专用，深色 HUD 风格）。
 *
 * 与 bundles/BundlePlanCard 区别：本卡是模型广场主视觉（深色切角 + mint 描边 +
 * 内发光 + monospace），用 scoped CSS + isDark prop（非 Tailwind dark:），
 * 不含 concurrency/rpm/精确额度（公开 DTO 无此数据）。
 * 仅展示 PublicBundlePlan 字段。featured 由父级据 pickFeaturedPlan 传入。
 */
</script>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { platformLabel } from '@/utils/platformColors'
import { platformDotColor } from '@/utils/showcaseBundle'
import type { PublicBundlePlan } from '@/api/publicBundles'

const props = defineProps<{
  plan: PublicBundlePlan
  featured?: boolean
  isDark?: boolean
}>()
const emit = defineEmits<{ (e: 'click', plan: PublicBundlePlan): void }>()

const { t } = useI18n()

const TIER_CODE: Record<string, string> = { starter: 'STARTER', pro: 'PRO', enterprise: 'ENTERPRISE' }
const tierCode = computed(() => TIER_CODE[props.plan.tier] ?? props.plan.tier.toUpperCase())
const hasDiscount = computed(() => props.plan.original_price > props.plan.price)

function onClick() {
  emit('click', props.plan)
}
</script>

<template>
  <article
    class="sbc-card"
    :class="{ 'is-featured': featured, 'is-dark': isDark }"
    @click="onClick"
  >
    <span v-if="featured" class="sbc-featured-badge">{{ t('modelCatalog.bundleFeaturedTag') }}</span>

    <span class="sbc-tier">{{ tierCode }}</span>
    <h3 class="sbc-name">{{ plan.name }}</h3>
    <p v-if="plan.description" class="sbc-desc">{{ plan.description }}</p>

    <div class="sbc-price">
      <span class="sbc-price-num">${{ plan.price }}</span>
      <span v-if="hasDiscount" class="sbc-price-strike">${{ plan.original_price }}</span>
      <span class="sbc-price-unit">/ {{ plan.validity_days }}{{ t('modelCatalog.bundleDays') }}</span>
    </div>

    <ul v-if="plan.features.length" class="sbc-feats">
      <li v-for="(f, i) in plan.features" :key="i">{{ f }}</li>
    </ul>

    <div v-if="plan.platforms.length" class="sbc-platforms">
      <span v-for="p in plan.platforms" :key="p" class="sbc-pchip">
        <span class="sbc-pdot" :style="{ background: platformDotColor(p) }" />
        {{ platformLabel(p) }}
      </span>
    </div>

    <button type="button" class="sbc-cta" @click.stop="onClick">
      {{ t('modelCatalog.bundleViewDetails') }}
    </button>
  </article>
</template>

<style scoped>
.sbc-card {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 22px 20px 20px;
  min-height: 380px;
  color: #f8fafc;
  border: 1px solid rgba(94, 234, 212, 0.20);
  background:
    linear-gradient(145deg, rgba(15, 23, 42, 0.94), rgba(3, 7, 18, 0.86));
  box-shadow: 0 26px 60px rgba(0, 0, 0, 0.32), inset 0 0 32px rgba(45, 212, 191, 0.04);
  clip-path: polygon(0 0, calc(100% - 18px) 0, 100% 18px, 100% 100%, 18px 100%, 0 calc(100% - 18px));
  cursor: pointer;
}
.sbc-card::before {
  content: "";
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: radial-gradient(circle at 20% 0%, rgba(34, 211, 238, 0.16), transparent 38%);
}
.sbc-card.is-featured {
  border-color: rgba(45, 212, 191, 0.45);
  box-shadow: 0 30px 70px rgba(0, 0, 0, 0.36), inset 0 0 40px rgba(45, 212, 191, 0.09);
}
.sbc-card.is-featured::before {
  background: radial-gradient(circle at 20% 0%, rgba(34, 211, 238, 0.26), transparent 42%);
}

.sbc-featured-badge {
  position: absolute;
  top: 0;
  right: 0;
  padding: 5px 11px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: #022622;
  background: linear-gradient(135deg, rgba(52, 211, 153, 0.96), rgba(16, 185, 129, 0.88));
  box-shadow: 0 6px 16px rgba(16, 185, 129, 0.45);
  clip-path: polygon(0 0, 100% 0, 100% 100%, 14px 100%);
}

.sbc-tier {
  align-self: flex-start;
  padding: 3px 8px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: #b9fff4;
  background: rgba(45, 212, 191, 0.12);
  border: 1px solid rgba(94, 234, 212, 0.25);
  border-radius: 4px;
}
.sbc-name {
  margin: 2px 0 0;
  font-size: 19px;
  font-weight: 850;
  line-height: 1.15;
  color: #f8fafc;
}
.sbc-desc {
  margin: -6px 0 0;
  min-height: 32px;
  font-size: 12px;
  line-height: 1.5;
  color: rgba(203, 213, 225, 0.68);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.sbc-price {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 10px 0 4px;
  border-top: 1px solid rgba(94, 234, 212, 0.14);
}
.sbc-price-num {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 26px;
  font-weight: 800;
  line-height: 1;
  color: #67e8f9;
}
.sbc-price-strike {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 13px;
  color: rgba(203, 213, 225, 0.45);
  text-decoration: line-through;
}
.sbc-price-unit {
  margin-left: auto;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 11px;
  color: rgba(203, 213, 225, 0.6);
}

.sbc-feats {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.sbc-feats li {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  font-size: 12.5px;
  line-height: 1.4;
  color: rgba(226, 232, 240, 0.84);
}
.sbc-feats li::before {
  content: "";
  flex: none;
  width: 7px;
  height: 7px;
  margin-top: 5px;
  background: #2dd4bf;
  transform: rotate(45deg);
}

.sbc-platforms {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: auto;
  padding-top: 12px;
  border-top: 1px solid rgba(94, 234, 212, 0.14);
}
.sbc-pchip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px 8px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 10px;
  font-weight: 600;
  color: #b9fff4;
  background: rgba(2, 6, 23, 0.5);
  border: 1px solid rgba(125, 211, 252, 0.22);
  border-radius: 4px;
}
.sbc-pdot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}

.sbc-cta {
  display: block;
  margin-top: 12px;
  padding: 11px 14px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  text-align: center;
  color: #b9fff4;
  background: transparent;
  border: 1.5px solid rgba(45, 212, 191, 0.5);
  cursor: pointer;
  transition: all 0.15s;
}
.sbc-cta:hover {
  color: #022622;
  background: linear-gradient(135deg, rgba(20, 184, 166, 0.96), rgba(34, 211, 238, 0.88));
  border-color: transparent;
}
/* featured 卡的 CTA 默认实心 */
.sbc-card.is-featured .sbc-cta {
  color: #022622;
  background: linear-gradient(135deg, rgba(20, 184, 166, 0.96), rgba(34, 211, 238, 0.88));
  border-color: transparent;
}
</style>
```

- [ ] **Step 4: 跑测试确认通过**

Run: `pnpm exec vitest run src/components/__tests__/ShowcaseBundleCard.spec.ts`
Expected: PASS(6 passed)

- [ ] **Step 5: Commit**

```bash
git add src/components/models/ShowcaseBundleCard.vue src/components/__tests__/ShowcaseBundleCard.spec.ts
git commit -m "feat(catalog): add ShowcaseBundleCard (dark HUD bundle card for public catalog)"
```

---

## Task 3: 改写 BundleShowcaseSection + 单测

**Files:**
- Modify: `src/components/models/BundleShowcaseSection.vue`(整体改写)
- Test: `src/components/__tests__/BundleShowcaseSection.spec.ts`

**Interfaces:**
- Consumes: `ShowcaseBundleCard`(Task 2)、`pickFeaturedPlan`(Task 1);i18n keys `modelCatalog.bundleSectionTitle/Subtitle/Kicker/Stats/viewAllBundles`(现有 + Task 4 加)
- Produces: 组件 props 变更为 `{ plans: PublicBundlePlan[]; isDark?: boolean }`(新增 `isDark`,供 ModelCatalogView 透传)

- [ ] **Step 1: 写失败测试**

Create `src/components/__tests__/BundleShowcaseSection.spec.ts`:

```ts
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import BundleShowcaseSection from '@/components/models/BundleShowcaseSection.vue'
import type { PublicBundlePlan } from '@/api/publicBundles'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string, args?: Record<string, unknown>) => args ? `${k}:${JSON.stringify(args)}` : k }),
}))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))

// stub 子卡：暴露 featured/isDark/name,便于断言 section 的判定结果
const CardStub = defineComponent({
  props: { plan: { type: Object as () => PublicBundlePlan, required: true },
           featured: { type: Boolean, default: false },
           isDark: { type: Boolean, default: false } },
  template: '<div class="stub" :data-name="plan.name" :data-featured="featured" :data-dark="isDark" />',
})

function plan(over: Partial<PublicBundlePlan> = {}): PublicBundlePlan {
  return {
    name: 'x', tier: 'starter', description: '', price: 10,
    original_price: 0, currency: 'USD', validity_days: 30,
    features: [], sort_order: 0, platforms: [], ...over,
  }
}

const plans: PublicBundlePlan[] = [
  plan({ name: '入门', tier: 'starter', sort_order: 0, platforms: ['openai'], price: 9 }),
  plan({ name: '专业', tier: 'pro', sort_order: 1, platforms: ['openai', 'anthropic'], price: 49 }),
  plan({ name: '企业', tier: 'enterprise', sort_order: 2, platforms: ['openai', 'gemini'], price: 199 }),
]

function mountSection(props: Record<string, unknown> = {}) {
  return mount(BundleShowcaseSection, {
    props: { plans, ...props },
    global: { stubs: { ShowcaseBundleCard: CardStub } },
  })
}

describe('BundleShowcaseSection', () => {
  it('为每个套餐渲染一张卡', () => {
    const w = mountSection()
    expect(w.findAll('.stub')).toHaveLength(3)
  })

  it('把 featured 标记到 tier=pro 的卡', () => {
    const w = mountSection()
    const cards = w.findAll('.stub')
    expect(cards[0].attributes('data-featured')).toBe('false')
    expect(cards[1].attributes('data-featured')).toBe('true')   // 专业
    expect(cards[2].attributes('data-featured')).toBe('false')
  })

  it('透传 isDark 到每张卡', () => {
    const w = mountSection({ isDark: true })
    w.findAll('.stub').forEach(c => expect(c.attributes('data-dark')).toBe('true'))
  })

  it('统计渲染套餐数与去重平台数', () => {
    const w = mountSection()
    // openai/anthropic/gemini → 3 个平台
    expect(w.text()).toContain('"n":3')
    expect(w.text()).toContain('"m":3')
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/components/__tests__/BundleShowcaseSection.spec.ts`
Expected: FAIL(旧组件未透传 isDark、未用 ShowcaseBundleCard、无 featured 判定)

- [ ] **Step 3: 写实现（整体改写 BundleShowcaseSection.vue）**

Replace entire file `src/components/models/BundleShowcaseSection.vue`:

```vue
<script lang="ts">
/**
 * 模型广场顶部套餐展示区（公开页）。
 *
 * 改用专属 ShowcaseBundleCard（深色 HUD），不再复用 bundles/BundlePlanCard。
 * featured（主推）由 pickFeaturedPlan 判定，透传到对应卡。
 * 点击卡 / 「查看全部」均跳 /bundles（未登录由该路由 requiresAuth 拦截）。
 * isDark 由 ModelCatalogView 透传，再下传给每张卡（单套暗色机制）。
 */
</script>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import ShowcaseBundleCard from '@/components/models/ShowcaseBundleCard.vue'
import { pickFeaturedPlan } from '@/utils/showcaseBundle'
import type { PublicBundlePlan } from '@/api/publicBundles'

const props = defineProps<{
  plans: PublicBundlePlan[]
  isDark?: boolean
}>()

const router = useRouter()
const { t } = useI18n()

const featuredPlan = computed(() => pickFeaturedPlan(props.plans))
const platformCount = computed(() =>
  new Set(props.plans.flatMap(p => p.platforms)).size,
)

function isFeatured(plan: PublicBundlePlan): boolean {
  const f = featuredPlan.value
  return !!f && f.sort_order === plan.sort_order && f.name === plan.name
}
function goToBundles() {
  router.push('/bundles')
}
</script>

<template>
  <section class="bundle-showcase" :class="{ 'is-dark': isDark }">
    <header class="bs-head">
      <div class="bs-head-text">
        <div class="bs-kicker">// {{ t('modelCatalog.bundleSectionKicker') }}</div>
        <h2 class="bs-title">{{ t('modelCatalog.bundleSectionTitle') }}</h2>
        <p class="bs-sub">{{ t('modelCatalog.bundleSectionSubtitle') }}</p>
        <div class="bs-stats">
          {{ t('modelCatalog.bundleSectionStats', { n: plans.length, m: platformCount }) }}
        </div>
      </div>
      <button type="button" class="bs-viewall" @click="goToBundles">
        <span>{{ t('modelCatalog.viewAllBundles') }}</span>
        <svg class="bs-viewall-arrow" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
          <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
        </svg>
      </button>
    </header>

    <div class="bs-grid">
      <ShowcaseBundleCard
        v-for="plan in plans"
        :key="`${plan.sort_order}-${plan.name}`"
        :plan="plan"
        :featured="isFeatured(plan)"
        :is-dark="isDark"
        @click="goToBundles"
      />
    </div>
  </section>
</template>

<style scoped>
.bundle-showcase {
  --bs-fg: #0f172a;
  --bs-sub: #475569;
  --bs-teal: #14b8a6;
  color: var(--bs-fg);
}
.bundle-showcase.is-dark {
  --bs-fg: #f8fafc;
  --bs-sub: rgba(203, 213, 225, 0.78);
}

.bs-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 24px;
  margin-bottom: 22px;
}
.bs-head-text { min-width: 0; }
.bs-kicker {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--bs-teal);
  margin-bottom: 8px;
}
.bs-title {
  margin: 0;
  font-size: clamp(26px, 3.6vw, 36px);
  font-weight: 900;
  letter-spacing: -0.025em;
  line-height: 1.05;
}
.bs-sub {
  max-width: 560px;
  margin: 10px 0 0;
  font-size: 14px;
  line-height: 1.6;
  color: var(--bs-sub);
}
.bs-stats {
  margin-top: 8px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  font-weight: 700;
  color: #0d9488;
}
.bundle-showcase.is-dark .bs-stats { color: #2dd4bf; }

.bs-viewall {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 11px 18px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 13px;
  font-weight: 800;
  color: var(--bs-fg);
  background: rgba(255, 255, 255, 0.6);
  border: 1.5px solid rgba(20, 184, 166, 0.55);
  cursor: pointer;
  transition: all 0.15s;
}
.bundle-showcase.is-dark .bs-viewall {
  color: #f8fafc;
  background: rgba(2, 6, 23, 0.5);
}
.bs-viewall:hover {
  color: #022622;
  background: linear-gradient(135deg, rgba(20, 184, 166, 0.96), rgba(34, 211, 238, 0.88));
  border-color: transparent;
}
.bs-viewall-arrow { width: 16px; height: 16px; flex: none; }

.bs-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 18px;
}
</style>
```

- [ ] **Step 4: 跑测试确认通过**

Run: `pnpm exec vitest run src/components/__tests__/BundleShowcaseSection.spec.ts`
Expected: PASS(4 passed)

- [ ] **Step 5: Commit**

```bash
git add src/components/models/BundleShowcaseSection.vue src/components/__tests__/BundleShowcaseSection.spec.ts
git commit -m "feat(catalog): redesign BundleShowcaseSection with dark HUD cards + featured + isDark"
```

---

## Task 4: ModelCatalogView 透传 isDark + i18n keys + 全局验证

**Files:**
- Modify: `src/views/public/ModelCatalogView.vue`(BundleShowcaseSection 调用处,约第 8-11 行)
- Modify: `src/i18n/locales/zh/custom.ts`(modelCatalog 命名空间,约第 173-175 行附近)
- Modify: `src/i18n/locales/en/custom.ts`(同 key 英文)

**Interfaces:**
- Consumes: Task 3 的 `BundleShowcaseSection` 新 props `{ plans, isDark }`;`isDark` 已存在于 ModelCatalogView(`useThemeMode()` 解构,第 77 行)

- [ ] **Step 1: ModelCatalogView 透传 isDark**

In `src/views/public/ModelCatalogView.vue`,把:

```vue
      <BundleShowcaseSection
        v-if="bundlePlans.length > 0"
        :plans="bundlePlans"
      />
```

改为:

```vue
      <BundleShowcaseSection
        v-if="bundlePlans.length > 0"
        :plans="bundlePlans"
        :is-dark="isDark"
      />
```

- [ ] **Step 2: 新增 i18n key(zh)**

In `src/i18n/locales/zh/custom.ts`,定位到 `modelCatalog` 命名空间内现有:

```ts
    bundleSectionTitle: '套餐订阅',
    bundleSectionSubtitle: '选择套餐，解锁更多模型配额与特权',
    viewAllBundles: '查看全部套餐',
```

在其**同一对象**内追加(保持逗号规则):

```ts
    bundleSectionKicker: '订阅套餐',
    bundleSectionStats: '{n} 个套餐 · 覆盖 {m} 个平台',
    bundleViewDetails: '查看详情',
    bundleFeaturedTag: '推荐',
    bundleDays: '天',
```

- [ ] **Step 3: 同步 i18n key(en)**

In `src/i18n/locales/en/custom.ts`,定位到 `modelCatalog` 命名空间内对应的英文 `bundleSectionTitle` / `bundleSectionSubtitle` / `viewAllBundles` 处,追加:

```ts
    bundleSectionKicker: 'Subscription Plans',
    bundleSectionStats: '{n} plans · {m} platforms',
    bundleViewDetails: 'View Details',
    bundleFeaturedTag: 'Featured',
    bundleDays: '-day',
```

- [ ] **Step 4: typecheck**

Run: `cd frontend && pnpm run typecheck`
Expected: PASS(0 error)。确认 `:is-dark` prop 类型与 BundleShowcaseSection 新定义一致,无 `BundlePlanCard`/`toCardData` 残留引用。

- [ ] **Step 5: 跑全部相关单测**

Run: `cd frontend && pnpm exec vitest run src/utils/__tests__/showcaseBundle.spec.ts src/components/__tests__/ShowcaseBundleCard.spec.ts src/components/__tests__/BundleShowcaseSection.spec.ts`
Expected: 全部 PASS

- [ ] **Step 6: lint**

Run: `cd frontend && pnpm run lint:check`
Expected: PASS(新文件无 lint 错)

- [ ] **Step 7: 手动验证(起 dev server 或本地构建)**

- 起 `pnpm dev`,访问 `/`(模型广场):顶部套餐区卡片为深色 HUD 切角风格,与下方模型卡统一;`tier=pro` 卡有 `★ 推荐` badge 且 CTA 实心;其余卡 CTA 描边。
- 切换暗色:整区随主题正确切换(`isDark` 透传生效)。
- 点击卡 / 「查看全部套餐」→ 跳 `/bundles`。
- 确认 `/bundles` 页套餐卡外观**未变**(BundlePlanCard 零改动)。

- [ ] **Step 8: Commit**

```bash
git add src/views/public/ModelCatalogView.vue src/i18n/locales/zh/custom.ts src/i18n/locales/en/custom.ts
git commit -m "feat(catalog): wire isDark + i18n keys for redesigned bundle showcase"
```

---

## Self-Review

**1. Spec coverage:**
- §2 范围(仅 BundleShowcaseSection + 新卡,不动 BundlePlanCard)→ Task 2/3,Global Constraints 明令禁改。✓
- §3.3 视觉 DNA 精确值 → Task 2 scoped CSS 逐字复用。✓
- §4.3 tier 方案 C(teal 单色 + 推荐 badge,featured=pro/中间档)→ Task 1 pickFeaturedPlan + Task 2 badge/CTA。✓
- §4.4 信息结构(无并发/rpm/额度,空数据降级)→ Task 2 template `v-if` 守卫 + 测试覆盖。✓
- §4.5 header 对齐 ModelCatalogHeader(kicker/巨标/副标/统计/查看全部)→ Task 3。✓
- §4.6 CTA「查看详情」跳 /bundles → Task 2/3 + Task 4 i18n。✓
- §4.7 isDark 透传链 → Task 3(prop)+ Task 4(ModelCatalogView 传)。✓
- §5.3 平台色(platformColors 无圆点色 → 本地 platformDotColor)→ Task 1。✓
- §6 验收 1-9 → Task 2/3/4 步骤覆盖,Task 4 Step 4-7 跑 typecheck/lint/手测。✓
- §6-8 单测覆盖 featured + 空数据 → Task 1/2/3 测试。✓

**2. Placeholder scan:** 无 TBD/TODO;所有 code step 含完整可运行代码;i18n 文案为实际中英文字符串。

**3. Type consistency:**
- `pickFeaturedPlan(plans: PublicBundlePlan[])` 在 Task 1 定义,Task 3 同名同参调用。✓
- `platformDotColor(platform: string)` Task 1 定义,Task 2 同名调用。✓
- `ShowcaseBundleCard` props `{plan, featured?, isDark?}`、emit `click(plan)` 在 Task 2 定义,Task 3 模板用相同 prop 名 `:plan/:featured/:is-dark`、`@click`。✓
- `BundleShowcaseSection` props `{plans, isDark?}` Task 3 定义,Task 1(无)、Task 4 ModelCatalogView 用 `:plans/:is-dark`。✓
- i18n key 名(`bundleSectionKicker/Stats/bundleViewDetails/bundleFeaturedTag/bundleDays`)在 Task 2/3 使用与 Task 4 定义完全一致。✓
- `PublicBundlePlan` 字段全程按真实定义(name/tier/description/price/original_price/currency/validity_days/features/sort_order/platforms,无 id)。✓
