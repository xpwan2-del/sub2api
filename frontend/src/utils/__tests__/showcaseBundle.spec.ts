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
