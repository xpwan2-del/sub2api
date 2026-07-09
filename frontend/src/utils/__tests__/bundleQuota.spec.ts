import { describe, expect, it } from 'vitest'
import { formatSegmentValue, hasAnyQuotaLimit, quotaSegments, splitModelPatterns } from '@/utils/bundleQuota'
import type { BundlePlanGroupQuota } from '@/types/bundle'

const baseGq = (overrides: Partial<BundlePlanGroupQuota> = {}): BundlePlanGroupQuota => ({
  id: 1,
  plan_id: 1,
  group_id: 1,
  group_name: 'g',
  group_platform: 'openai',
  quota_scope: 'platform',
  model_pattern: '',
  daily_limit_usd: 0,
  weekly_limit_usd: 0,
  monthly_limit_usd: 0,
  daily_image_limit_count: 0,
  weekly_image_limit_count: 0,
  monthly_image_limit_count: 0,
  daily_video_limit_count: 0,
  weekly_video_limit_count: 0,
  monthly_video_limit_count: 0,
  ...overrides,
})

describe('bundleQuota utils', () => {
  describe('hasAnyQuotaLimit', () => {
    it('is false when every limit is zero', () => {
      expect(hasAnyQuotaLimit(baseGq())).toBe(false)
    })
    it('is true when only USD daily limit is set', () => {
      expect(hasAnyQuotaLimit(baseGq({ daily_limit_usd: 10 }))).toBe(true)
    })
    it('is true when only image count limit is set', () => {
      expect(hasAnyQuotaLimit(baseGq({ monthly_image_limit_count: 100 }))).toBe(true)
    })
    it('is true when only video count limit is set', () => {
      expect(hasAnyQuotaLimit(baseGq({ weekly_video_limit_count: 50 }))).toBe(true)
    })
  })

  describe('quotaSegments', () => {
    it('returns no segments when all limits are zero', () => {
      expect(quotaSegments(baseGq())).toEqual([])
    })
    it('returns only the usd segment for a text group', () => {
      const segs = quotaSegments(baseGq({ daily_limit_usd: 10, weekly_limit_usd: 50, monthly_limit_usd: 200 }))
      expect(segs).toEqual([{ kind: 'usd', daily: 10, weekly: 50, monthly: 200 }])
    })
    it('returns only the image segment for an image group', () => {
      const segs = quotaSegments(baseGq({ daily_image_limit_count: 100, weekly_image_limit_count: 500, monthly_image_limit_count: 2000 }))
      expect(segs).toEqual([{ kind: 'image', daily: 100, weekly: 500, monthly: 2000 }])
    })
    it('returns segments in fixed order usd → image → video for a mixed group', () => {
      const segs = quotaSegments(baseGq({
        monthly_limit_usd: 200,
        daily_image_limit_count: 100,
        weekly_video_limit_count: 50,
      }))
      expect(segs.map((s) => s.kind)).toEqual(['usd', 'image', 'video'])
    })
    it('keeps zero values inside a segment so the template can skip per-dimension', () => {
      // daily unset, weekly/monthly set → segment still emitted, daily === 0
      const segs = quotaSegments(baseGq({ weekly_limit_usd: 50, monthly_limit_usd: 200 }))
      expect(segs).toHaveLength(1)
      expect(segs[0].daily).toBe(0)
      expect(segs[0].weekly).toBe(50)
    })
  })

  describe('formatSegmentValue', () => {
    it('prefixes $ for usd', () => {
      expect(formatSegmentValue('usd', 10)).toBe('$10')
    })
    it('appends 次 for image counts', () => {
      expect(formatSegmentValue('image', 100)).toBe('100次')
    })
    it('appends 次 for video counts', () => {
      expect(formatSegmentValue('video', 50)).toBe('50次')
    })
  })

  describe('splitModelPatterns', () => {
    it('returns empty for null/undefined/empty', () => {
      expect(splitModelPatterns(null)).toEqual([])
      expect(splitModelPatterns(undefined)).toEqual([])
      expect(splitModelPatterns('')).toEqual([])
    })
    it('splits comma-separated patterns', () => {
      expect(splitModelPatterns('gpt-4o,claude-3-opus')).toEqual(['gpt-4o', 'claude-3-opus'])
    })
    it('trims whitespace', () => {
      expect(splitModelPatterns(' gpt-4* , claude-3-opus ')).toEqual(['gpt-4*', 'claude-3-opus'])
    })
    it('drops empty segments', () => {
      expect(splitModelPatterns('gpt-4o,,claude-3-opus,')).toEqual(['gpt-4o', 'claude-3-opus'])
    })
    it('single pattern returns one element', () => {
      expect(splitModelPatterns('gpt-4*')).toEqual(['gpt-4*'])
    })
  })
})
