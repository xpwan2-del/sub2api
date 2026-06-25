// bundle_constants.go 套餐捆绑销售模块常量定义
// 定义套餐层级（tier）、订阅状态（status）、获取来源（source）、
// 额度粒度（quota_scope）以及缓存键和 TTL。

package service

import "time"

// BundleTier 常量定义套餐层级：starter（入门）/pro（专业）/enterprise（企业）
// BundleTier constants define the tier level of a bundle plan.
const (
	BundleTierStarter    = "starter"
	BundleTierPro        = "pro"
	BundleTierEnterprise = "enterprise"
)

// BundleStatus 常量定义套餐订阅状态：active（生效）/expired（过期）/revoked（已撤销）
// BundleStatus constants define the status of a bundle subscription.
const (
	BundleStatusActive  = "active"
	BundleStatusExpired = "expired"
	BundleStatusRevoked = "revoked"
)

// BundleSource 常量定义套餐订阅获取来源：purchase（购买）/redeem（兑换）/admin_assign（管理员分配）
// BundleSource constants define how a bundle subscription was obtained.
const (
	BundleSourcePurchase    = "purchase"
	BundleSourceRedeem      = "redeem"
	BundleSourceAdminAssign = "admin_assign"
)

// QuotaScope 常量定义额度粒度：platform（平台级，按渠道组整体计量）/model（模型级，按 glob 匹配特定模型）
// QuotaScope constants define the granularity of a quota entry.
const (
	QuotaScopePlatform = "platform"
	QuotaScopeModel    = "model"
)

// BundlePlanStatus 常量定义套餐计划状态：active（启用）/disabled（停用）
// Bundle plan status constants.
const (
	BundlePlanStatusActive   = "active"
	BundlePlanStatusDisabled = "disabled"
)

// 缓存键前缀和 TTL，用于套餐计划、订阅、用量的 Redis 缓存
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
