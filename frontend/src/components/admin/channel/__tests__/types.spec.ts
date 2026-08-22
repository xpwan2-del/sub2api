import { describe, expect, it } from 'vitest'
import {
  apiIntervalsToForm,
  apiTimePricingToForm,
  createDefaultTimePricingForm,
  formIntervalsToAPI,
  formTimePricingToAPI,
  resolutionOptionsForMode,
  nextDefaultTierLabel,
  isValidPositiveMultiplier,
  validateIntervals,
  validateTimePricing,
  type IntervalFormEntry,
  type TimePricingFormEntry,
  type TimePricingPeriodFormEntry,
} from '../types'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_VIDEO,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_TOKEN,
} from '@/constants/channel'
import { getBillingModeLabel, BILLING_MODE_PER_SECOND } from '@/utils/billingMode'

describe('interval multiplier conversion', () => {
  it('preserves component multipliers without MTok conversion', () => {
    const form = apiIntervalsToForm([{
      min_tokens: 272000,
      max_tokens: null,
      tier_label: '',
      input_price: null,
      output_price: null,
      cache_write_price: null,
      cache_read_price: null,
      input_multiplier: 2,
      output_multiplier: 1.5,
      cache_write_multiplier: 2,
      cache_read_multiplier: 2,
      per_request_price: null,
      sort_order: 0,
    }])

    expect(form[0].input_multiplier).toBe(2)
    expect(form[0].output_multiplier).toBe(1.5)
    expect(formIntervalsToAPI(form)[0]).toMatchObject({
      input_multiplier: 2,
      output_multiplier: 1.5,
      cache_write_multiplier: 2,
      cache_read_multiplier: 2,
    })
  })
})

describe('positive multiplier validation', () => {
  it('accepts empty and positive values but rejects zero and negative values', () => {
    expect(isValidPositiveMultiplier(null)).toBe(true)
    expect(isValidPositiveMultiplier('')).toBe(true)
    expect(isValidPositiveMultiplier('0.5')).toBe(true)
    expect(isValidPositiveMultiplier(0)).toBe(false)
    expect(isValidPositiveMultiplier(-1)).toBe(false)
  })

  it('rejects a zero interval multiplier', () => {
    expect(validateIntervals([
      makeInterval({ min_tokens: 100, input_multiplier: 0 }),
    ], 'token', t)).toContain('multiplierPositive')
  })
})

function makeInterval(over: Partial<IntervalFormEntry>): IntervalFormEntry {
  return {
    min_tokens: 0,
    max_tokens: null,
    tier_label: '',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    input_multiplier: null,
    output_multiplier: null,
    cache_write_multiplier: null,
    cache_read_multiplier: null,
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
}

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

describe('per_second billing mode', () => {
  it('exposes constant', () => {
    expect(BILLING_MODE_PER_SECOND).toBe('per_second')
  })
  it('renders label via i18n key', () => {
    const t = (k: string) => k
    expect(getBillingModeLabel(BILLING_MODE_PER_SECOND, t)).toBe('admin.usage.billingModePerSecond')
  })
})

describe('time pricing', () => {
  it('uses a disabled Shanghai default', () => {
    const form = createDefaultTimePricingForm()
    expect(form).toEqual({ timezone: 'Asia/Shanghai', periods: [] })
    expect(formTimePricingToAPI(form)).toBeNull()
  })

  it('round-trips and formats multiplier', () => {
    const form = apiTimePricingToForm({
      timezone: 'Asia/Shanghai',
      periods: [{ start_time: '09:00', end_time: '12:00', multiplier: 2 }],
    })
    expect(form.periods[0]).toEqual({
      start_time: '09:00:00',
      end_time: '12:00:00',
      multiplier: '2.00',
    })
    expect(formTimePricingToAPI(form)).toEqual({
      timezone: 'Asia/Shanghai',
      periods: [{ start_time: '09:00:00', end_time: '12:00:00', multiplier: 2 }],
    })
  })

  it.each([
    ['separated', [{ start_time: '09:00:00', end_time: '12:00:00', multiplier: '2.00' }, { start_time: '14:00:00', end_time: '18:00:00', multiplier: '2.00' }], null],
    ['adjacent', [{ start_time: '09:00:00', end_time: '12:00:00', multiplier: '2.00' }, { start_time: '12:00:00', end_time: '14:00:00', multiplier: '1.50' }], null],
    ['midnight split', [{ start_time: '22:00:00', end_time: '00:00:00', multiplier: '2.00' }, { start_time: '00:00:00', end_time: '02:00:00', multiplier: '2.00' }], null],
    ['overlap by one second', [{ start_time: '09:00:00', end_time: '12:00:00', multiplier: '2.00' }, { start_time: '11:59:59', end_time: '14:00:00', multiplier: '2.00' }], 'overlap'],
    ['cross midnight', [{ start_time: '22:00:00', end_time: '02:00:00', multiplier: '2.00' }], 'range'],
    ['equal midnight', [{ start_time: '00:00:00', end_time: '00:00:00', multiplier: '2.00' }], 'range'],
    ['missing seconds', [{ start_time: '09:00', end_time: '12:00', multiplier: '2.00' }], 'format'],
    ['zero', [{ start_time: '09:00:00', end_time: '12:00:00', multiplier: '0.00' }], 'multiplier'],
    ['three decimals', [{ start_time: '09:00:00', end_time: '12:00:00', multiplier: '1.001' }], 'multiplier'],
  ])('%s', (_name, periods, errorKey) => {
    const result = validateTimePricing({
      timezone: 'Asia/Shanghai',
      periods: periods as TimePricingPeriodFormEntry[],
    }, t)
    if (errorKey === null) expect(result).toBeNull()
    else expect(result).toContain(String(errorKey))
  })

  it('rejects non-IANA timezone', () => {
    expect(validateTimePricing({
      timezone: 'UTC+8',
      periods: [{ start_time: '09:00:00', end_time: '12:00:00', multiplier: '2.00' }],
    }, t)).toContain('timezone')
  })

  it.each([
    ['missing', undefined],
    ['blank', '   '],
  ])('rejects a %s timezone without throwing during conversion', (_name, timezone) => {
    const form = {
      timezone,
      periods: [{ start_time: '09:00:00', end_time: '12:00:00', multiplier: '2.00' }],
    } as unknown as TimePricingFormEntry

    expect(validateTimePricing(form, t)).toContain('timezone')
    expect(() => formTimePricingToAPI(form)).not.toThrow()
    expect(formTimePricingToAPI(form)?.timezone).toBe('')
  })
})
