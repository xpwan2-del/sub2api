package service

import "time"

// BundleTier constants define the tier level of a bundle plan.
const (
	BundleTierBasic       = "basic"
	BundleTierFlagship    = "flagship"
	BundleTierEnterprise  = "enterprise"
)

// BundleStatus constants define the status of a bundle subscription.
const (
	BundleStatusActive  = "active"
	BundleStatusExpired = "expired"
	BundleStatusRevoked = "revoked"
)

// BundleSource constants define how a bundle subscription was obtained.
const (
	BundleSourcePurchase    = "purchase"
	BundleSourceRedeem      = "redeem"
	BundleSourceAdminAssign = "admin_assign"
)

// QuotaScope constants define the granularity of a quota entry.
const (
	QuotaScopePlatform = "platform"
	QuotaScopeModel    = "model"
)

// Bundle plan status constants.
const (
	BundlePlanStatusActive   = "active"
	BundlePlanStatusDisabled = "disabled"
)

// Cache key patterns and TTL for bundle-related caching.
const (
	BundleCacheKeyPlanPrefix    = "bundle:plan:"
	BundleCacheKeySubPrefix     = "bundle:sub:"
	BundleCacheKeyUsagePrefix   = "bundle:usage:"
	BundleCacheKeyUserBundles   = "bundle:user:"

	BundlePlanCacheTTL    = 5 * time.Minute
	BundleSubCacheTTL     = 3 * time.Minute
	BundleUsageCacheTTL   = 1 * time.Minute
)
