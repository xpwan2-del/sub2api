/**
 * Bundle tier shared constants and display configuration.
 *
 * Single source of truth for tier values used by admin
 * (`views/admin/bundles/BundlePlansView.vue`) and user-facing
 * (`views/user/BundlesView.vue`, `BundleUsageView.vue`) screens.
 *
 * When adding a new tier, only this file and the backend
 * `bundle_constants.go` need to be updated.
 */

// ── Tier value constants (must match backend BundleTier* in bundle_constants.go) ──

export const BUNDLE_TIER_STARTER = 'starter' as const
export const BUNDLE_TIER_PRO = 'pro' as const
export const BUNDLE_TIER_ENTERPRISE = 'enterprise' as const

/** Union type for all valid tier values */
export type BundleTier = typeof BUNDLE_TIER_STARTER | typeof BUNDLE_TIER_PRO | typeof BUNDLE_TIER_ENTERPRISE

/** Ordered list of all tier values (lowest to highest) */
export const BUNDLE_TIERS: readonly BundleTier[] = [
  BUNDLE_TIER_STARTER,
  BUNDLE_TIER_PRO,
  BUNDLE_TIER_ENTERPRISE,
] as const

// ── i18n key mapping ──

const TIER_I18N_KEYS: Record<BundleTier, { user: string; admin: string }> = {
  starter: { user: 'bundles.tierStarter', admin: 'bundles.admin.tierStarter' },
  pro: { user: 'bundles.tierPro', admin: 'bundles.admin.tierPro' },
  enterprise: { user: 'bundles.tierEnterprise', admin: 'bundles.admin.tierEnterprise' },
}

// ── Display theme per tier ──

export interface TierTheme {
  badgeClass: string
  borderClass: string
  accentClass: string
  textClass: string
  iconClass: string
  btnClass: string
  discountClass: string
  adminBadgeClass: string
}

const TIER_THEME: Record<BundleTier, TierTheme> = {
  starter: {
    badgeClass: 'bg-blue-500/10 text-blue-600 border border-blue-500/30 dark:text-blue-400 rounded-md px-2 py-0.5 text-[11px] font-medium',
    borderClass: 'border-blue-500/20',
    accentClass: 'bg-gradient-to-r from-blue-400 to-blue-500',
    textClass: 'text-blue-600 dark:text-blue-400',
    iconClass: 'text-blue-500 dark:text-blue-400',
    btnClass: 'bg-blue-500 text-white hover:bg-blue-600 active:bg-blue-700',
    discountClass: 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300',
    adminBadgeClass: 'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300',
  },
  pro: {
    badgeClass: 'bg-purple-500/10 text-purple-600 border border-purple-500/30 dark:text-purple-400 rounded-md px-2 py-0.5 text-[11px] font-medium',
    borderClass: 'border-purple-500/20',
    accentClass: 'bg-gradient-to-r from-purple-400 to-purple-500',
    textClass: 'text-purple-600 dark:text-purple-400',
    iconClass: 'text-purple-500 dark:text-purple-400',
    btnClass: 'bg-purple-500 text-white hover:bg-purple-600 active:bg-purple-700',
    discountClass: 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300',
    adminBadgeClass: 'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300',
  },
  enterprise: {
    badgeClass: 'bg-amber-500/10 text-amber-600 border border-amber-500/30 dark:text-amber-400 rounded-md px-2 py-0.5 text-[11px] font-medium',
    borderClass: 'border-amber-500/20',
    accentClass: 'bg-gradient-to-r from-amber-400 to-amber-500',
    textClass: 'text-amber-600 dark:text-amber-400',
    iconClass: 'text-amber-500 dark:text-amber-400',
    btnClass: 'bg-amber-500 text-white hover:bg-amber-600 active:bg-amber-700',
    discountClass: 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300',
    adminBadgeClass: 'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300',
  },
}

const DEFAULT_TIER_THEME: TierTheme = {
  badgeClass: 'bg-gray-500/10 text-gray-600 border border-gray-500/30 dark:text-gray-400 rounded-md px-2 py-0.5 text-[11px] font-medium',
  borderClass: 'border-gray-200 dark:border-dark-700',
  accentClass: 'bg-gradient-to-r from-primary-400 to-primary-500',
  textClass: 'text-primary-600 dark:text-primary-400',
  iconClass: 'text-primary-500 dark:text-primary-400',
  btnClass: 'bg-primary-500 text-white hover:bg-primary-600',
  discountClass: 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300',
  adminBadgeClass: 'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300',
}

// ── Helper functions ──

/** Get theme for a tier, with fallback for unknown values */
export function getTierTheme(tier?: string): TierTheme {
  if (tier && tier in TIER_THEME) return TIER_THEME[tier as BundleTier]
  return DEFAULT_TIER_THEME
}

/** Get i18n display label key for a tier */
export function getTierI18nKey(tier?: string, namespace: 'user' | 'admin' = 'user'): string {
  if (tier && tier in TIER_I18N_KEYS) return TIER_I18N_KEYS[tier as BundleTier][namespace]
  return tier || ''
}

/** Build select dropdown options for tier (admin) */
export function getTierSelectOptions(t: (key: string) => string): Array<{ value: BundleTier; label: string }> {
  return BUNDLE_TIERS.map(tier => ({
    value: tier,
    label: t(TIER_I18N_KEYS[tier].admin),
  }))
}
