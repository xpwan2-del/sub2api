import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import BundleShowcaseSection from '@/components/models/BundleShowcaseSection.vue'
import type { PublicBundlePlan } from '@/api/publicBundles'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (k: string, args?: Record<string, unknown>) => (args ? `${k}:${JSON.stringify(args)}` : k),
  }),
}))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))

// stub 子卡：暴露 featured/isDark/name，便于断言 section 的判定结果
const CardStub = defineComponent({
  props: {
    plan: { type: Object as () => PublicBundlePlan, required: true },
    featured: { type: Boolean, default: false },
    isDark: { type: Boolean, default: false },
  },
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
    expect(cards[1].attributes('data-featured')).toBe('true') // 专业
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
