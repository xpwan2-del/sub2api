import type { BundlePlanGroupQuota } from '@/types/bundle'

export type QuotaKind = 'usd' | 'image' | 'video'

export interface QuotaSegment {
  kind: QuotaKind
  daily: number
  weekly: number
  monthly: number
}

const hasUsdLimit = (gq: BundlePlanGroupQuota): boolean =>
  !!(gq.daily_limit_usd || gq.weekly_limit_usd || gq.monthly_limit_usd)

const hasImageLimit = (gq: BundlePlanGroupQuota): boolean =>
  !!(gq.daily_image_limit_count || gq.weekly_image_limit_count || gq.monthly_image_limit_count)

const hasVideoLimit = (gq: BundlePlanGroupQuota): boolean =>
  !!(gq.daily_video_limit_count || gq.weekly_video_limit_count || gq.monthly_video_limit_count)

/** Whether the group has any non-zero quota limit (USD, image, or video). */
export const hasAnyQuotaLimit = (gq: BundlePlanGroupQuota): boolean =>
  hasUsdLimit(gq) || hasImageLimit(gq) || hasVideoLimit(gq)

/** Non-zero quota segments in fixed order: usd → image → video. */
export const quotaSegments = (gq: BundlePlanGroupQuota): QuotaSegment[] => {
  const segments: QuotaSegment[] = []
  if (hasUsdLimit(gq)) {
    segments.push({
      kind: 'usd',
      daily: gq.daily_limit_usd,
      weekly: gq.weekly_limit_usd,
      monthly: gq.monthly_limit_usd,
    })
  }
  if (hasImageLimit(gq)) {
    segments.push({
      kind: 'image',
      daily: gq.daily_image_limit_count,
      weekly: gq.weekly_image_limit_count,
      monthly: gq.monthly_image_limit_count,
    })
  }
  if (hasVideoLimit(gq)) {
    segments.push({
      kind: 'video',
      daily: gq.daily_video_limit_count,
      weekly: gq.weekly_video_limit_count,
      monthly: gq.monthly_video_limit_count,
    })
  }
  return segments
}

/** Format a single quota value with its unit: `$10` for USD, `100次` for counts. */
export const formatSegmentValue = (kind: QuotaKind, value: number): string =>
  kind === 'usd' ? `$${value}` : `${value}次`

/**
 * Split a comma-separated model_pattern string into individual pattern chips.
 * Trims whitespace and drops empty segments. Returns [] for empty/null/undefined.
 * Used by the tag input + display components to share one parsing rule.
 */
export const splitModelPatterns = (pattern: string | null | undefined): string[] => {
  if (!pattern) return []
  return pattern
    .split(',')
    .map(p => p.trim())
    .filter(p => p.length > 0)
}
