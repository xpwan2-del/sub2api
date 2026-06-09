// bundle_models.go 套餐捆绑销售模块数据模型
// 定义服务层的核心模型（BundlePlan、BundleSubscription 等）
// 以及用于创建/更新操作的请求 DTO（Data Transfer Object）。

package service

import "time"

// BundlePlan 套餐计划服务层模型，包含计划属性和关联的渠道组额度列表
// BundlePlan is the service-layer model for a bundle plan.
type BundlePlan struct {
	ID               int64
	Name             string
	Description      string
	Tier             string
	Price            float64
	OriginalPrice    float64
	Currency         string
	ValidityDays     int
	ConcurrencyLimit int
	RPMLimit         int
	Features         []string
	ForSale          bool
	SortOrder        int
	Status           string
	CreatedAt        time.Time
	UpdatedAt        time.Time

	GroupQuotas []BundlePlanGroupQuota
}

// BundlePlanGroupQuota 套餐计划中单个渠道组的额度配置
// BundlePlanGroupQuota is the service-layer model for per-group quota within a plan.
type BundlePlanGroupQuota struct {
	ID             int64
	PlanID         int64
	GroupID        int64
	QuotaScope     string
	ModelPattern   string
	DailyLimitUSD  float64
	WeeklyLimitUSD float64
	MonthlyLimitUSD float64
}

// BundleSubscription 用户套餐订阅实例，包含订阅状态、时间范围和用量数据
// BundleSubscription is the service-layer model for a user's bundle subscription.
type BundleSubscription struct {
	ID               int64
	UserID           int64
	PlanID           int64
	Status           string
	StartsAt         time.Time
	ExpiresAt        time.Time
	ConcurrencyLimit int
	RPMLimit         int
	Source           string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time

	Plan   *BundlePlan
	Usages []BundleSubscriptionUsage
}

// BundleSubscriptionUsage 套餐订阅的用量跟踪记录，按渠道组和时间窗口统计
// BundleSubscriptionUsage is the service-layer model for usage tracking per subscription + group.
type BundleSubscriptionUsage struct {
	ID                   int64
	BundleSubscriptionID int64
	GroupID              int64
	ModelPattern         string
	DailyUsageUSD        float64
	DailyWindowStart     time.Time
	WeeklyUsageUSD       float64
	WeeklyWindowStart    time.Time
	MonthlyUsageUSD      float64
	MonthlyWindowStart   time.Time
}

// CreateBundlePlanRequest 创建套餐计划的请求 DTO
// CreateBundlePlanRequest is the DTO for creating a new bundle plan.
type CreateBundlePlanRequest struct {
	Name             string                  `json:"name" binding:"required"`
	Description      string                  `json:"description"`
	Tier             string                  `json:"tier" binding:"required,oneof=starter pro enterprise"`
	Price            float64                 `json:"price"`
	OriginalPrice    float64                 `json:"original_price"`
	Currency         string                  `json:"currency"`
	ValidityDays     int                     `json:"validity_days" binding:"required,min=1"`
	ConcurrencyLimit int                     `json:"concurrency_limit"`
	RPMLimit         int                     `json:"rpm_limit"`
	Features         []string                `json:"features"`
	ForSale          bool                    `json:"for_sale"`
	SortOrder        int                     `json:"sort_order"`
	GroupQuotas      []CreateGroupQuotaRequest `json:"group_quotas" binding:"required,min=1"`
}

// CreateGroupQuotaRequest 创建渠道组额度条目的请求 DTO
// CreateGroupQuotaRequest is the DTO for creating a group quota entry within a plan.
type CreateGroupQuotaRequest struct {
	GroupID         int64   `json:"group_id" binding:"required"`
	QuotaScope      string  `json:"quota_scope" binding:"required,oneof=platform model"`
	ModelPattern    string  `json:"model_pattern"`
	DailyLimitUSD   float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD  float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD float64 `json:"monthly_limit_usd"`
}

// UpdateBundlePlanRequest 更新套餐计划的请求 DTO，所有字段为指针类型支持部分更新
// UpdateBundlePlanRequest is the DTO for updating an existing bundle plan.
type UpdateBundlePlanRequest struct {
	Name             *string                  `json:"name"`
	Description      *string                  `json:"description"`
	Tier             *string                  `json:"tier"`
	Price            *float64                 `json:"price"`
	OriginalPrice    *float64                 `json:"original_price"`
	Currency         *string                  `json:"currency"`
	ValidityDays     *int                     `json:"validity_days"`
	ConcurrencyLimit *int                     `json:"concurrency_limit"`
	RPMLimit         *int                     `json:"rpm_limit"`
	Features         *[]string                `json:"features"`
	ForSale          *bool                    `json:"for_sale"`
	SortOrder        *int                     `json:"sort_order"`
	Status           *string                  `json:"status"`
	GroupQuotas      *[]CreateGroupQuotaRequest `json:"group_quotas"`
}

// BundleUsageProgress 单个额度作用域的用量进度（已用/上限）
// BundleUsageProgress represents the current usage against limits for a single quota scope.
type BundleUsageProgress struct {
	GroupID         int64   `json:"group_id"`
	QuotaScope      string  `json:"quota_scope"`
	ModelPattern    string  `json:"model_pattern"`
	DailyUsageUSD   float64 `json:"daily_usage_usd"`
	DailyLimitUSD   float64 `json:"daily_limit_usd"`
	WeeklyUsageUSD  float64 `json:"weekly_usage_usd"`
	WeeklyLimitUSD  float64 `json:"weekly_limit_usd"`
	MonthlyUsageUSD float64 `json:"monthly_usage_usd"`
	MonthlyLimitUSD float64 `json:"monthly_limit_usd"`
}
