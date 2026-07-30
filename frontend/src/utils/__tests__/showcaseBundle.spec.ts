import { describe, it, expect } from 'vitest'
import { pickFeaturedPlan, platformDotColor } from '@/utils/showcaseBundle'
import type { PublicBundlePlan } from '@/api/publicBundles'

function plan(over: Partial<PublicBundlePlan> = {}): PublicBundlePlan {
  return {
    name: 'x', tier: 'starter', description: '', price: 10,
    original_price: 0, currency: 'USD', validity_days: 30,
    features: [], sort_order: 0, platforms: [], featured: false, ...over,
  }
}

describe('pickFeaturedPlan', () => {
  it('返回 featured=true 的套餐', () => {
    const featured = plan({ featured: true, name: '专业版', sort_order: 1 })
    const result = pickFeaturedPlan([
      plan({ featured: false, sort_order: 0 }),
      featured,
      plan({ featured: false, price: 200, sort_order: 2 }),
    ])
    expect(result).toBe(featured)
  })
  it('多个 featured 取第一个（按数组顺序）', () => {
    const a = plan({ featured: true, name: 'A', sort_order: 0 })
    const b = plan({ featured: true, name: 'B', sort_order: 1 })
    expect(pickFeaturedPlan([a, b])).toBe(a)
  })
  it('无 featured 时返回 null', () => {
    expect(pickFeaturedPlan([plan({ featured: false }), plan({ featured: false })])).toBeNull()
    expect(pickFeaturedPlan([])).toBeNull()
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
