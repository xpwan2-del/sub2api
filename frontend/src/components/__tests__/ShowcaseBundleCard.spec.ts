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
    // props.plan 经 Vue 包装后与原 plan 非同一引用,用深度结构相等比较
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
