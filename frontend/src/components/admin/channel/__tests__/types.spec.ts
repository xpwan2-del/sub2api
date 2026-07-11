import { describe, expect, it } from 'vitest'
import {
  resolutionOptionsForMode,
  nextDefaultTierLabel,
  validateIntervals,
  type IntervalFormEntry,
} from '../types'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_VIDEO,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_TOKEN,
} from '@/constants/channel'

function makeInterval(over: Partial<IntervalFormEntry>): IntervalFormEntry {
  return {
    min_tokens: 0,
    max_tokens: null,
    tier_label: '',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    per_request_price: null,
    sort_order: 0,
    ...over,
  }
}

function tier(label: string): IntervalFormEntry {
  return {
    min_tokens: 0,
    max_tokens: null,
    tier_label: label,
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    per_request_price: 0.1,
    sort_order: 0,
  }
}

function t(key: string, params?: Record<string, unknown>): string {
  return `${key}${params ? ` ${JSON.stringify(params)}` : ''}`
describe('validateIntervals', () => {
  describe('token mode', () => {
    it('rejects unbounded interval that is not last', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ min_tokens: 0, max_tokens: null, input_price: 1, output_price: 1 }),
        makeInterval({ min_tokens: 200000, max_tokens: 500000, input_price: 2, output_price: 2 }),
      ]
      expect(validateIntervals(intervals, 'token', t)).toContain('unboundedLast')
    })

    it('accepts unbounded interval at the end', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ min_tokens: 0, max_tokens: 200000, input_price: 1, output_price: 1 }),
        makeInterval({ min_tokens: 200000, max_tokens: null, input_price: 2, output_price: 2 }),
      ]
      expect(validateIntervals(intervals, 'token', t)).toBeNull()
    })

    it('rejects overlapping intervals', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ min_tokens: 0, max_tokens: 250000, input_price: 1, output_price: 1 }),
        makeInterval({ min_tokens: 200000, max_tokens: 500000, input_price: 2, output_price: 2 }),
      ]
      expect(validateIntervals(intervals, 'token', t)).toContain('overlap')
    })

    it('rejects unbounded interval in token mode', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ min_tokens: 0, max_tokens: null, input_price: 1, output_price: 1 }),
        makeInterval({ min_tokens: 100, max_tokens: 200, input_price: 2, output_price: 2 }),
      ]
      expect(validateIntervals(intervals, 'token', t)).toContain('unboundedLast')
    })
  })

  describe('image / per_request mode', () => {
    it('allows multiple unbounded tiers identified by label', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ tier_label: '1K', per_request_price: 0.04 }),
        makeInterval({ tier_label: '2K', per_request_price: 0.06 }),
        makeInterval({ tier_label: '4K', per_request_price: 0.08 }),
      ]
      expect(validateIntervals(intervals, 'image', t)).toBeNull()
      expect(validateIntervals(intervals, 'per_request', t)).toBeNull()
    })

    it('still rejects negative prices', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ tier_label: '1K', per_request_price: -1 }),
      ]
      expect(validateIntervals(intervals, 'image', t)).toContain('negativePrice')
    })

    it('still rejects max <= min on a single tier', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ tier_label: '1K', min_tokens: 100, max_tokens: 50, per_request_price: 0.04 }),
      ]
      expect(validateIntervals(intervals, 'image', t)).toContain('maxGreaterThanMin')
    })
  })
})

describe('resolutionOptionsForMode', () => {
  it('image 返回 1K/2K/4K', () => {
    const vals = resolutionOptionsForMode(BILLING_MODE_IMAGE).map((o) => o.value)
    expect(vals).toEqual(['1K', '2K', '4K'])
  })
  it('video 返回 480P/720P/1080P/4K', () => {
    const vals = resolutionOptionsForMode(BILLING_MODE_VIDEO).map((o) => o.value)
    expect(vals).toEqual(['480P', '720P', '1080P', '4K'])
  })
  it('per_request / token 返回空数组（自由文本/不适用）', () => {
    expect(resolutionOptionsForMode(BILLING_MODE_PER_REQUEST)).toEqual([])
    expect(resolutionOptionsForMode(BILLING_MODE_TOKEN)).toEqual([])
  })
})

describe('nextDefaultTierLabel', () => {
  it('image 按 1K/2K/4K 顺序预填', () => {
    expect(nextDefaultTierLabel(BILLING_MODE_IMAGE, 0)).toBe('1K')
    expect(nextDefaultTierLabel(BILLING_MODE_IMAGE, 1)).toBe('2K')
    expect(nextDefaultTierLabel(BILLING_MODE_IMAGE, 3)).toBe('') // 超出预设
  })
  it('video 按 480P/720P/1080P/4K 顺序预填', () => {
    expect(nextDefaultTierLabel(BILLING_MODE_VIDEO, 0)).toBe('480P')
    expect(nextDefaultTierLabel(BILLING_MODE_VIDEO, 3)).toBe('4K')
    expect(nextDefaultTierLabel(BILLING_MODE_VIDEO, 4)).toBe('')
  })
})

describe('validateIntervals image/video', () => {
  it('image 非白名单 tier_label 报错', () => {
    expect(validateIntervals([tier('HD')], BILLING_MODE_IMAGE)).not.toBeNull()
  })
  it('image 合法档位通过', () => {
    expect(validateIntervals([tier('1K')], BILLING_MODE_IMAGE)).toBeNull()
  })
  it('image 同档位重复报错', () => {
    expect(validateIntervals([tier('1K'), tier('1K')], BILLING_MODE_IMAGE)).not.toBeNull()
  })
  it('video 非白名单报错', () => {
    expect(validateIntervals([tier('5K')], BILLING_MODE_VIDEO)).not.toBeNull()
  })
})
