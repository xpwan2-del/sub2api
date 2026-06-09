# Bundle Subscription Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Sub2API 新增「套餐订阅」功能，将多个 AI 模型 Group 捆绑成套餐售卖，每个 Group 独立配置日/周/月额度。

**Architecture:** 新增 4 个 Ent Schema + 对应的 Repository/Service/Handler 层，通过自动创建 UserSubscription 桥接到现有网关计费体系。对现有代码改动约 70 行，新增约 22 个文件。

**Tech Stack:** Go 1.26, Ent ORM, Gin, Wire DI, PostgreSQL, Redis, Vue 3, TypeScript, pnpm

**Design Doc:** `docs/BUNDLE_SUBSCRIPTION_DESIGN.md`

---

## File Structure

### New Files (22)

```
backend/ent/schema/
  bundle_plan.go                       # 套餐商品 Schema
  bundle_plan_group_quota.go           # 套餐-Group 额度配置 Schema
  bundle_subscription.go               # 用户套餐实例 Schema
  bundle_subscription_usage.go         # 套餐用量跟踪 Schema

backend/internal/service/
  bundle_plan_port.go                  # BundlePlanRepo 接口（端口）
  bundle_subscription_port.go          # BundleSubscriptionRepo 接口（端口）
  bundle_usage_port.go                 # BundleUsageRepo 接口（端口）
  bundle_plan_service.go               # 套餐商品 Service
  bundle_subscription_service.go       # 套餐订阅 Service（激活/撤销/到期）
  bundle_usage_service.go              # 套餐用量 Service（累加/查询）
  bundle_route_resolver.go             # 单 Key 模型→Group 路由解析
  bundle_errors.go                     # Bundle 错误码定义
  bundle_constants.go                  # Bundle 常量定义
  bundle_models.go                     # Bundle Service 层数据模型

backend/internal/repository/
  bundle_plan_repo.go                  # BundlePlanRepo 实现
  bundle_subscription_repo.go          # BundleSubscriptionRepo 实现
  bundle_usage_repo.go                 # BundleUsageRepo 实现

backend/internal/handler/
  bundle_handler.go                    # 用户端 Bundle Handler
  bundle_admin_handler.go              # 管理端 Bundle Handler

backend/internal/server/middleware/
  bundle_resolver.go                   # 单 Key 自动路由中间件

backend/internal/server/routes/
  bundle.go                            # Bundle 路由注册

frontend/src/types/bundle.ts           # TypeScript 类型
frontend/src/api/admin/bundles.ts      # 管理端 API
frontend/src/api/bundles.ts            # 用户端 API
```

### Modified Files (~70 lines)

```
backend/ent/schema/user_subscription.go    # +4 fields (~15 lines)
backend/ent/schema/api_key.go              # +1 field  (~5 lines)
backend/cmd/server/wire.go                 # +Bundle providers (~20 lines)
backend/internal/server/middleware/middleware.go  # RequireGroupAssignment bundle branch (~5 lines)
backend/internal/server/routes/gateway.go        # Register bundle_resolver (~3 lines)
backend/internal/service/billing_cache_service.go # Limit fallback (~10 lines)
backend/internal/service/gateway_service.go       # postUsageBilling bundle branch (~15 lines)
backend/internal/repository/wire.go              # +3 NewXXXRepository
backend/internal/service/wire.go                 # +4 NewXXXService + bindings
backend/internal/handler/wire.go                 # +Bundle handlers
backend/internal/handler/handler.go              # +Bundle fields in Handlers/AdminHandlers
backend/internal/server/routes/admin.go          # +registerBundleRoutes
```

---

## Phase 1: Data Model + Ent Codegen

### Task 1: Create BundlePlan Ent Schema

**Files:**
- Create: `backend/ent/schema/bundle_plan.go`

- [ ] **Step 1: Create the schema file**

遵循 `subscription_plan.go` 的模式（无 Mixin，手动 created_at/updated_at，entsql annotation）：

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"time"
)

type BundlePlan struct {
	ent.Schema
}

func (BundlePlan) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "bundle_plans"},
	}
}

func (BundlePlan) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().Comment("套餐名称"),
		field.String("description").Default("").Comment("套餐描述"),
		field.String("tier").NotEmpty().Comment("套餐层级: starter/pro/enterprise"),
		field.Float("price").Default(0).Comment("售价"),
		field.Float("original_price").Default(0).Comment("原价（划线价）"),
		field.String("currency").Default("USD").Comment("货币: USD/CNY"),
		field.Int("validity_days").Default(30).Positive().Comment("有效天数"),
		field.Int("concurrency_limit").Default(0).NonNegative().Comment("并发上限（0=不限）"),
		field.Int("rpm_limit").Default(0).NonNegative().Comment("RPM上限（0=不限）"),
		field.Strings("features").Optional().Comment("功能特性列表"),
		field.Bool("for_sale").Default(true).Comment("是否在售"),
		field.Int("sort_order").Default(0).NonNegative().Comment("排序"),
		field.String("status").Default("active").Comment("状态: active/disabled"),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Comment("创建时间"),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Comment("更新时间"),
	}
}

func (BundlePlan) Edges() []ent.Edge {
	return nil
}

func (BundlePlan) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "for_sale"),
		index.Fields("tier"),
	}
}
```

- [ ] **Step 2: Verify file compiles**

Run: `cd /Users/maybewaityou/Desktop/MeePwn/climb2fame/workspace/ai/sub2api/backend && go vet ./ent/schema/bundle_plan.go`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add backend/ent/schema/bundle_plan.go
git commit -m "feat(bundle): add BundlePlan ent schema"
```

---

### Task 2: Create BundlePlanGroupQuota Ent Schema

**Files:**
- Create: `backend/ent/schema/bundle_plan_group_quota.go`

- [ ] **Step 1: Create the schema file**

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type BundlePlanGroupQuota struct {
	ent.Schema
}

func (BundlePlanGroupQuota) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "bundle_plan_group_quotas"},
	}
}

func (BundlePlanGroupQuota) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("plan_id").Comment("→ BundlePlan"),
		field.Int64("group_id").Comment("→ Group（复用现有 Group）"),
		field.String("quota_scope").Default("platform").Comment("额度粒度: platform/model"),
		field.String("model_pattern").Default("").Comment("仅 model 级别生效，glob 模式"),
		field.Float("daily_limit_usd").Default(0).NonNegative().Comment("日额度（0=不限）"),
		field.Float("weekly_limit_usd").Default(0).NonNegative().Comment("周额度（0=不限）"),
		field.Float("monthly_limit_usd").Default(0).NonNegative().Comment("月额度（0=不限）"),
	}
}

func (BundlePlanGroupQuota) Edges() []ent.Edge {
	return nil
}

func (BundlePlanGroupQuota) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("plan_id", "group_id"),
	}
}
```

- [ ] **Step 2: Verify file compiles**

Run: `cd backend && go vet ./ent/schema/bundle_plan_group_quota.go`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add backend/ent/schema/bundle_plan_group_quota.go
git commit -m "feat(bundle): add BundlePlanGroupQuota ent schema"
```

---

### Task 3: Create BundleSubscription + BundleSubscriptionUsage Ent Schemas

**Files:**
- Create: `backend/ent/schema/bundle_subscription.go`
- Create: `backend/ent/schema/bundle_subscription_usage.go`

- [ ] **Step 1: Create BundleSubscription schema**

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"time"

	"sub2api/backend/ent/schema/mixins"
)

type BundleSubscription struct {
	ent.Schema
}

func (BundleSubscription) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "bundle_subscriptions"},
	}
}

func (BundleSubscription) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (BundleSubscription) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id").Comment("→ User"),
		field.Int64("plan_id").Comment("→ BundlePlan"),
		field.String("status").Default("active").Comment("active/expired/revoked"),
		field.Time("starts_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Comment("生效时间"),
		field.Time("expires_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Comment("到期时间"),
		field.Int("concurrency_limit").Default(0).NonNegative().Comment("快照：并发上限"),
		field.Int("rpm_limit").Default(0).NonNegative().Comment("快照：RPM上限"),
		field.String("source").Default("purchase").Comment("来源: purchase/redeem/admin_assign"),
	}
}

func (BundleSubscription) Edges() []ent.Edge {
	return nil
}

func (BundleSubscription) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "status", "expires_at"),
		index.Fields("plan_id"),
	}
}
```

- [ ] **Step 2: Create BundleSubscriptionUsage schema**

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"time"
)

type BundleSubscriptionUsage struct {
	ent.Schema
}

func (BundleSubscriptionUsage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "bundle_subscription_usages"},
	}
}

func (BundleSubscriptionUsage) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("bundle_subscription_id").Comment("→ BundleSubscription"),
		field.Int64("group_id").Comment("→ Group"),
		field.String("model_pattern").Default("").Comment("空=平台级，有值=模型级"),
		// Daily
		field.Float("daily_usage_usd").Default(0).NonNegative().Comment("当日已用"),
		field.Time("daily_window_start").Default(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Comment("日窗口起点"),
		// Weekly
		field.Float("weekly_usage_usd").Default(0).NonNegative().Comment("当周已用"),
		field.Time("weekly_window_start").Default(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Comment("周窗口起点"),
		// Monthly
		field.Float("monthly_usage_usd").Default(0).NonNegative().Comment("当月已用"),
		field.Time("monthly_window_start").Default(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Comment("月窗口起点"),
	}
}

func (BundleSubscriptionUsage) Edges() []ent.Edge {
	return nil
}

func (BundleSubscriptionUsage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("bundle_subscription_id", "group_id"),
	}
}
```

- [ ] **Step 3: Commit**

```bash
git add backend/ent/schema/bundle_subscription.go backend/ent/schema/bundle_subscription_usage.go
git commit -m "feat(bundle): add BundleSubscription and BundleSubscriptionUsage ent schemas"
```

---

### Task 4: Modify Existing Schemas + Generate Ent Code

**Files:**
- Modify: `backend/ent/schema/user_subscription.go` — add 4 fields
- Modify: `backend/ent/schema/api_key.go` — add 1 field

- [ ] **Step 1: Add fields to UserSubscription schema**

在 `user_subscription.go` 的 `Fields()` 返回数组末尾（`notes` 字段之后）追加：

```go
		// ========== Bundle subscription fields ==========
		field.Int64("bundle_subscription_id").Optional().Nillable().Comment("关联的套餐实例ID"),
		field.Float("daily_limit_usd").Default(0).NonNegative().Comment("独立日限额（0=fallback到Group配置）"),
		field.Float("weekly_limit_usd").Default(0).NonNegative().Comment("独立周限额"),
		field.Float("monthly_limit_usd").Default(0).NonNegative().Comment("独立月限额"),
```

同时添加索引：

```go
		index.Fields("bundle_subscription_id"),
```

- [ ] **Step 2: Add field to APIKey schema**

在 `api_key.go` 的 `Fields()` 返回数组中找到合适位置（`group_id` 字段附近），追加：

```go
		field.Int64("bundle_subscription_id").Optional().Nillable().Comment("关联的套餐实例ID（单Key模式）"),
```

- [ ] **Step 3: Run Ent code generation**

Run: `cd /Users/maybewaityou/Desktop/MeePwn/climb2fame/workspace/ai/sub2api/backend && go generate ./ent`
Expected: 成功生成 ent 代码，无错误

- [ ] **Step 4: Verify compilation**

Run: `cd backend && go build ./...`
Expected: 编译成功

- [ ] **Step 5: Commit all generated code**

```bash
git add backend/ent/
git commit -m "feat(bundle): modify UserSubscription+APIKey schemas, regenerate ent ORM code"
```

---

## Phase 2: Service Domain Layer (Constants, Errors, Models, Ports)

### Task 5: Create Bundle Constants and Errors

**Files:**
- Create: `backend/internal/service/bundle_constants.go`
- Create: `backend/internal/service/bundle_errors.go`

- [ ] **Step 1: Create constants file**

遵循 `domain/constants.go` 的模式：

```go
package service

// Bundle Tier
const (
	BundleTierStarter    = "starter"
	BundleTierPro        = "pro"
	BundleTierEnterprise = "enterprise"
)

// Bundle Status
const (
	BundleStatusActive  = "active"
	BundleStatusExpired = "expired"
	BundleStatusRevoked = "revoked"
)

// Bundle Source
const (
	BundleSourcePurchase    = "purchase"
	BundleSourceRedeem      = "redeem"
	BundleSourceAdminAssign = "admin_assign"
)

// Quota Scope
const (
	QuotaScopePlatform = "platform"
	QuotaScopeModel    = "model"
)

// Cache key patterns
const (
	BundleSubCacheKey     = "bundle_sub:%d"
	BundleUsageCacheKey   = "bundle_usage:%d:%d"
	BundleRouteCacheKey   = "bundle_route:%s"
	BundlePlansCacheKey   = "bundle_plans:for_sale"
)

// Cache TTL
const (
	BundleSubCacheTTL   = 5 * time.Minute
	BundleUsageCacheTTL = 5 * time.Minute
	BundleRouteCacheTTL = 30 * time.Minute
	BundlePlansCacheTTL = 10 * time.Minute
)
```

- [ ] **Step 2: Create errors file**

遵循项目使用 `infraerrors` 包的模式（参考 `subscription_service.go` 中的错误定义）：

```go
package service

import "sub2api/backend/internal/infraerrors"

var (
	ErrBundleNotFound        = infraerrors.NotFound("BUNDLE_NOT_FOUND", "套餐不存在")
	ErrBundlePlanNotFound    = infraerrors.NotFound("BUNDLE_PLAN_NOT_FOUND", "套餐商品不存在")
	ErrBundleExpired         = infraerrors.Forbidden("BUNDLE_EXPIRED", "您的套餐已过期，请续费或购买新套餐")
	ErrBundleConflict        = infraerrors.Conflict("BUNDLE_CONFLICT", "您已有活跃套餐，请先等待到期或联系管理员")
	ErrBundlePlanDisabled    = infraerrors.BadRequest("BUNDLE_PLAN_DISABLED", "该套餐已下架")
	ErrBundleModelNotIncluded = infraerrors.BadRequest("BUNDLE_MODEL_NOT_INCLUDED", "请求的模型不在套餐范围内")
	ErrBundleGroupQuotaExceeded = infraerrors.TooManyRequests("BUNDLE_GROUP_QUOTA_EXCEEDED", "套餐额度已用完")
	ErrBundleConcurrencyExceeded = infraerrors.TooManyRequests("BUNDLE_CONCURRENCY_EXCEEDED", "并发请求数已达套餐上限")
	ErrBundleRPMExceeded     = infraerrors.TooManyRequests("BUNDLE_RPM_EXCEEDED", "请求频率已达套餐上限")
)
```

- [ ] **Step 3: Commit**

```bash
git add backend/internal/service/bundle_constants.go backend/internal/service/bundle_errors.go
git commit -m "feat(bundle): add bundle constants and error definitions"
```

---

### Task 6: Create Bundle Service Models

**Files:**
- Create: `backend/internal/service/bundle_models.go`

- [ ] **Step 1: Create models file**

遵循 `subscription_service.go` 中 `UserSubscription` 等 model 定义模式：

```go
package service

import "time"

// ========== Bundle Plan Models ==========

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
	GroupQuotas      []BundlePlanGroupQuota
}

type BundlePlanGroupQuota struct {
	ID             int64
	PlanID         int64
	GroupID        int64
	QuotaScope     string
	ModelPattern   string
	DailyLimitUSD  float64
	WeeklyLimitUSD float64
	MonthlyLimitUSD float64
	Group          *Group
}

// ========== Bundle Subscription Models ==========

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
	Plan             *BundlePlan
	GroupUsages      []BundleSubscriptionUsage
}

type BundleSubscriptionUsage struct {
	ID                     int64
	BundleSubscriptionID   int64
	GroupID                int64
	ModelPattern           string
	DailyUsageUSD          float64
	DailyWindowStart       time.Time
	WeeklyUsageUSD         float64
	WeeklyWindowStart      time.Time
	MonthlyUsageUSD        float64
	MonthlyWindowStart     time.Time
	Group                  *Group
}

// ========== DTOs ==========

type CreateBundlePlanRequest struct {
	Name             string                `json:"name" binding:"required"`
	Description      string                `json:"description"`
	Tier             string                `json:"tier" binding:"required,oneof=starter pro enterprise"`
	Price            float64               `json:"price" binding:"required,gte=0"`
	OriginalPrice    float64               `json:"original_price" binding:"gte=0"`
	Currency         string                `json:"currency" binding:"required,oneof=USD CNY"`
	ValidityDays     int                   `json:"validity_days" binding:"required,min=1"`
	ConcurrencyLimit int                   `json:"concurrency_limit" binding:"gte=0"`
	RPMLimit         int                   `json:"rpm_limit" binding:"gte=0"`
	Features         []string              `json:"features"`
	GroupQuotas      []CreateGroupQuotaRequest `json:"group_quotas" binding:"required,min=1"`
}

type CreateGroupQuotaRequest struct {
	GroupID         int64   `json:"group_id" binding:"required"`
	QuotaScope      string  `json:"quota_scope" binding:"required,oneof=platform model"`
	ModelPattern    string  `json:"model_pattern"`
	DailyLimitUSD   float64 `json:"daily_limit_usd" binding:"gte=0"`
	WeeklyLimitUSD  float64 `json:"weekly_limit_usd" binding:"gte=0"`
	MonthlyLimitUSD float64 `json:"monthly_limit_usd" binding:"gte=0"`
}

type UpdateBundlePlanRequest struct {
	Name             *string               `json:"name"`
	Description      *string               `json:"description"`
	Tier             *string               `json:"tier"`
	Price            *float64              `json:"price"`
	OriginalPrice    *float64              `json:"original_price"`
	ValidityDays     *int                  `json:"validity_days"`
	ConcurrencyLimit *int                  `json:"concurrency_limit"`
	RPMLimit         *int                  `json:"rpm_limit"`
	Features         []string              `json:"features"`
	ForSale          *bool                 `json:"for_sale"`
	SortOrder        *int                  `json:"sort_order"`
	Status           *string               `json:"status"`
	GroupQuotas      []CreateGroupQuotaRequest `json:"group_quotas"`
}

type BundleUsageProgress struct {
	GroupID        int64   `json:"group_id"`
	GroupName      string  `json:"group_name"`
	Platform       string  `json:"platform"`
	ModelPattern   string  `json:"model_pattern"`
	DailyUsed      float64 `json:"daily_used"`
	DailyLimit     float64 `json:"daily_limit"`
	WeeklyUsed     float64 `json:"weekly_used"`
	WeeklyLimit    float64 `json:"weekly_limit"`
	MonthlyUsed    float64 `json:"monthly_used"`
	MonthlyLimit   float64 `json:"monthly_limit"`
}
```

- [ ] **Step 2: Commit**

```bash
git add backend/internal/service/bundle_models.go
git commit -m "feat(bundle): add bundle service models and DTOs"
```

---

### Task 7: Create Repository Port Interfaces

**Files:**
- Create: `backend/internal/service/bundle_plan_port.go`
- Create: `backend/internal/service/bundle_subscription_port.go`
- Create: `backend/internal/service/bundle_usage_port.go`

- [ ] **Step 1: Create BundlePlan port**

遵循 `user_subscription_port.go` 的模式：

```go
package service

import (
	"context"
	"sub2api/backend/internal/pagination"
)

type BundlePlanRepository interface {
	Create(ctx context.Context, plan *BundlePlan, groupQuotas []BundlePlanGroupQuota) (*BundlePlan, error)
	Update(ctx context.Context, plan *BundlePlan, groupQuotas []BundlePlanGroupQuota) (*BundlePlan, error)
	GetByID(ctx context.Context, id int64) (*BundlePlan, error)
	List(ctx context.Context, params pagination.PaginationParams, tier, status string) ([]BundlePlan, *pagination.PaginationResult, error)
	ListForSale(ctx context.Context) ([]BundlePlan, error)
	Delete(ctx context.Context, id int64) error
}
```

- [ ] **Step 2: Create BundleSubscription port**

```go
package service

import (
	"context"
	"sub2api/backend/internal/pagination"
)

type BundleSubscriptionRepository interface {
	Create(ctx context.Context, sub *BundleSubscription) (*BundleSubscription, error)
	GetByID(ctx context.Context, id int64) (*BundleSubscription, error)
	GetActiveByUserID(ctx context.Context, userID int64) (*BundleSubscription, error)
	GetByIDWithUsages(ctx context.Context, id int64) (*BundleSubscription, error)
	List(ctx context.Context, params pagination.PaginationParams, userID *int64, status string) ([]BundleSubscription, *pagination.PaginationResult, error)
	UpdateStatus(ctx context.Context, id int64, status string) error
	UpdateExpiry(ctx context.Context, id int64, expiresAt interface{}) error
}
```

- [ ] **Step 3: Create BundleUsage port**

```go
package service

import "context"

type BundleUsageRepository interface {
	GetBySubscriptionAndGroup(ctx context.Context, subscriptionID, groupID int64) (*BundleSubscriptionUsage, error)
	Create(ctx context.Context, usage *BundleSubscriptionUsage) (*BundleSubscriptionUsage, error)
	IncrementUsage(ctx context.Context, subscriptionID, groupID int64, costUSD float64) error
	ResetDailyWindow(ctx context.Context, subscriptionID, groupID int64, windowStart interface{}) error
	ResetWeeklyWindow(ctx context.Context, subscriptionID, groupID int64, windowStart interface{}) error
	ResetMonthlyWindow(ctx context.Context, subscriptionID, groupID int64, windowStart interface{}) error
	ListBySubscription(ctx context.Context, subscriptionID int64) ([]BundleSubscriptionUsage, error)
	BatchUpdateExpiredStatus(ctx context.Context) (int64, error)
}
```

- [ ] **Step 4: Commit**

```bash
git add backend/internal/service/bundle_plan_port.go backend/internal/service/bundle_subscription_port.go backend/internal/service/bundle_usage_port.go
git commit -m "feat(bundle): add repository port interfaces"
```

---

## Phase 3: Repository Implementations

### Task 8: Create Bundle Repository Implementations

**Files:**
- Create: `backend/internal/repository/bundle_plan_repo.go`
- Create: `backend/internal/repository/bundle_subscription_repo.go`
- Create: `backend/internal/repository/bundle_usage_repo.go`

- [ ] **Step 1: Create BundlePlanRepo**

遵循 `user_subscription_repo.go` 的模式（私有结构体 + 公开构造函数返回接口）：

```go
package repository

import (
	"context"
	"sub2api/backend/ent"
	dbent "sub2api/backend/ent"
	"sub2api/backend/ent/bundleplan"
	"sub2api/backend/ent/bundleplangroupquota"
	"sub2api/backend/ent/predicate"
	"sub2api/backend/internal/pagination"
	"sub2api/backend/internal/service"
)

type bundlePlanRepository struct {
	client *dbent.Client
}

func NewBundlePlanRepository(client *dbent.Client) service.BundlePlanRepository {
	return &bundlePlanRepository{client: client}
}

func (r *bundlePlanRepository) Create(ctx context.Context, plan *service.BundlePlan, groupQuotas []service.BundlePlanGroupQuota) (*service.BundlePlan, error) {
	client := clientFromContext(ctx, r.client)
	builder := client.BundlePlan.Create().
		SetName(plan.Name).
		SetDescription(plan.Description).
		SetTier(plan.Tier).
		SetPrice(plan.Price).
		SetOriginalPrice(plan.OriginalPrice).
		SetCurrency(plan.Currency).
		SetValidityDays(plan.ValidityDays).
		SetConcurrencyLimit(plan.ConcurrencyLimit).
		SetRpmLimit(plan.RPMLimit).
		SetForSale(plan.ForSale).
		SetSortOrder(plan.SortOrder).
		SetStatus(plan.Status)
	if plan.Features != nil {
		builder.SetFeatures(plan.Features)
	}
	created, err := builder.Save(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBundlePlanNotFound, nil)
	}
	// Create group quotas
	for _, q := range groupQuotas {
		_, err = client.BundlePlanGroupQuota.Create().
			SetPlanID(created.ID).
			SetGroupID(q.GroupID).
			SetQuotaScope(q.QuotaScope).
			SetModelPattern(q.ModelPattern).
			SetDailyLimitUsd(q.DailyLimitUSD).
			SetWeeklyLimitUsd(q.WeeklyLimitUSD).
			SetMonthlyLimitUsd(q.MonthlyLimitUSD).
			Save(ctx)
		if err != nil {
			return nil, translatePersistenceError(err, service.ErrBundlePlanNotFound, nil)
		}
	}
	return r.GetByID(ctx, created.ID)
}

func (r *bundlePlanRepository) GetByID(ctx context.Context, id int64) (*service.BundlePlan, error) {
	client := clientFromContext(ctx, r.client)
	plan, err := client.BundlePlan.Get(ctx, id)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBundlePlanNotFound, nil)
	}
	result := bundlePlanToService(plan)
	// Load group quotas
	quotas, err := client.BundlePlanGroupQuota.Query().Where(bundleplangroupquota.PlanID(id)).All(ctx)
	if err == nil {
		for _, q := range quotas {
			result.GroupQuotas = append(result.GroupQuotas, bundlePlanGroupQuotaToService(q))
		}
	}
	return result, nil
}

func (r *bundlePlanRepository) Update(ctx context.Context, plan *service.BundlePlan, groupQuotas []service.BundlePlanGroupQuota) (*service.BundlePlan, error) {
	client := clientFromContext(ctx, r.client)
	builder := client.BundlePlan.UpdateOneID(plan.ID).
		SetName(plan.Name).
		SetDescription(plan.Description).
		SetTier(plan.Tier).
		SetPrice(plan.Price).
		SetOriginalPrice(plan.OriginalPrice).
		SetValidityDays(plan.ValidityDays).
		SetConcurrencyLimit(plan.ConcurrencyLimit).
		SetRpmLimit(plan.RPMLimit).
		SetForSale(plan.ForSale).
		SetSortOrder(plan.SortOrder).
		SetStatus(plan.Status)
	if plan.Features != nil {
		builder.SetFeatures(plan.Features)
	}
	_, err := builder.Save(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBundlePlanNotFound, nil)
	}
	// Delete old quotas, create new ones
	_, err = client.BundlePlanGroupQuota.Delete().Where(bundleplangroupquota.PlanID(plan.ID)).Exec(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBundlePlanNotFound, nil)
	}
	for _, q := range groupQuotas {
		_, err = client.BundlePlanGroupQuota.Create().
			SetPlanID(plan.ID).
			SetGroupID(q.GroupID).
			SetQuotaScope(q.QuotaScope).
			SetModelPattern(q.ModelPattern).
			SetDailyLimitUsd(q.DailyLimitUSD).
			SetWeeklyLimitUsd(q.WeeklyLimitUSD).
			SetMonthlyLimitUsd(q.MonthlyLimitUSD).
			Save(ctx)
		if err != nil {
			return nil, translatePersistenceError(err, service.ErrBundlePlanNotFound, nil)
		}
	}
	return r.GetByID(ctx, plan.ID)
}

func (r *bundlePlanRepository) List(ctx context.Context, params pagination.PaginationParams, tier, status string) ([]service.BundlePlan, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	var predicates []predicate.BundlePlan
	if tier != "" {
		predicates = append(predicates, bundleplan.Tier(tier))
	}
	if status != "" {
		predicates = append(predicates, bundleplan.Status(status))
	}
	query := client.BundlePlan.Query()
	if len(predicates) > 0 {
		query.Where(bundleplan.And(predicates...))
	}
	total, err := query.Count(ctx)
	if err != nil {
		return nil, nil, translatePersistenceError(err, service.ErrBundlePlanNotFound, nil)
	}
	items, err := query.
		Offset(params.Offset).
		Limit(params.Limit).
		Order(dbent.Desc(bundleplan.FieldSortOrder), dbent.Desc(bundleplan.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, nil, translatePersistenceError(err, service.ErrBundlePlanNotFound, nil)
	}
	plans := make([]service.BundlePlan, len(items))
	for i, item := range items {
		plans[i] = *bundlePlanToService(item)
	}
	pagResult := pagination.NewPaginationResult(params, int64(total))
	return plans, pagResult, nil
}

func (r *bundlePlanRepository) ListForSale(ctx context.Context) ([]service.BundlePlan, error) {
	client := clientFromContext(ctx, r.client)
	items, err := client.BundlePlan.Query().
		Where(bundleplan.Status("active"), bundleplan.ForSale(true)).
		Order(dbent.Asc(bundleplan.FieldSortOrder)).
		All(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBundlePlanNotFound, nil)
	}
	plans := make([]service.BundlePlan, len(items))
	for i, item := range items {
		p := bundlePlanToService(item)
		quotas, qerr := client.BundlePlanGroupQuota.Query().Where(bundleplangroupquota.PlanID(item.ID)).All(ctx)
		if qerr == nil {
			for _, q := range quotas {
				p.GroupQuotas = append(p.GroupQuotas, bundlePlanGroupQuotaToService(q))
			}
		}
		plans[i] = *p
	}
	return plans, nil
}

func (r *bundlePlanRepository) Delete(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	err := client.BundlePlan.DeleteOneID(id).Exec(ctx)
	return translatePersistenceError(err, service.ErrBundlePlanNotFound, nil)
}

// ========== Entity converters ==========

func bundlePlanToService(m *ent.BundlePlan) *service.BundlePlan {
	return &service.BundlePlan{
		ID:               m.ID,
		Name:             m.Name,
		Description:      m.Description,
		Tier:             m.Tier,
		Price:            m.Price,
		OriginalPrice:    m.OriginalPrice,
		Currency:         m.Currency,
		ValidityDays:     m.ValidityDays,
		ConcurrencyLimit: m.ConcurrencyLimit,
		RPMLimit:         m.RpmLimit,
		Features:         m.Features,
		ForSale:          m.ForSale,
		SortOrder:        m.SortOrder,
		Status:           m.Status,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
	}
}

func bundlePlanGroupQuotaToService(m *ent.BundlePlanGroupQuota) service.BundlePlanGroupQuota {
	return service.BundlePlanGroupQuota{
		ID:              m.ID,
		PlanID:          m.PlanID,
		GroupID:         m.GroupID,
		QuotaScope:      m.QuotaScope,
		ModelPattern:    m.ModelPattern,
		DailyLimitUSD:   m.DailyLimitUsd,
		WeeklyLimitUSD:  m.WeeklyLimitUsd,
		MonthlyLimitUSD: m.MonthlyLimitUsd,
	}
}
```

- [ ] **Step 2: Create BundleSubscriptionRepo**

```go
package repository

import (
	"context"
	"time"

	"sub2api/backend/ent"
	dbent "sub2api/backend/ent"
	"sub2api/backend/ent/bundlesubscription"
	"sub2api/backend/internal/pagination"
	"sub2api/backend/internal/service"
)

type bundleSubscriptionRepository struct {
	client *dbent.Client
}

func NewBundleSubscriptionRepository(client *dbent.Client) service.BundleSubscriptionRepository {
	return &bundleSubscriptionRepository{client: client}
}

func (r *bundleSubscriptionRepository) Create(ctx context.Context, sub *service.BundleSubscription) (*service.BundleSubscription, error) {
	client := clientFromContext(ctx, r.client)
	created, err := client.BundleSubscription.Create().
		SetUserID(sub.UserID).
		SetPlanID(sub.PlanID).
		SetStatus(sub.Status).
		SetStartsAt(sub.StartsAt).
		SetExpiresAt(sub.ExpiresAt).
		SetConcurrencyLimit(sub.ConcurrencyLimit).
		SetRpmLimit(sub.RPMLimit).
		SetSource(sub.Source).
		Save(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBundleNotFound, nil)
	}
	return bundleSubscriptionToService(created), nil
}

func (r *bundleSubscriptionRepository) GetByID(ctx context.Context, id int64) (*service.BundleSubscription, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.BundleSubscription.Get(ctx, id)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBundleNotFound, nil)
	}
	return bundleSubscriptionToService(m), nil
}

func (r *bundleSubscriptionRepository) GetActiveByUserID(ctx context.Context, userID int64) (*service.BundleSubscription, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.BundleSubscription.Query().
		Where(
			bundlesubscription.UserID(userID),
			bundlesubscription.Status("active"),
			bundlesubscription.ExpiresAtGT(time.Now()),
		).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBundleNotFound, nil)
	}
	return bundleSubscriptionToService(m), nil
}

func (r *bundleSubscriptionRepository) GetByIDWithUsages(ctx context.Context, id int64) (*service.BundleSubscription, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.BundleSubscription.Get(ctx, id)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBundleNotFound, nil)
	}
	result := bundleSubscriptionToService(m)
	// Load usages
	usages, err := client.BundleSubscriptionUsage.Query().
		Where(bundlesubscriptionusage.BundleSubscriptionID(id)).
		All(ctx)
	if err == nil {
		for _, u := range usages {
			result.GroupUsages = append(result.GroupUsages, *bundleSubscriptionUsageToService(u))
		}
	}
	return result, nil
}

func (r *bundleSubscriptionRepository) List(ctx context.Context, params pagination.PaginationParams, userID *int64, status string) ([]service.BundleSubscription, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	query := client.BundleSubscription.Query()
	if userID != nil {
		query.Where(bundlesubscription.UserID(*userID))
	}
	if status != "" {
		query.Where(bundlesubscription.Status(status))
	}
	total, err := query.Count(ctx)
	if err != nil {
		return nil, nil, translatePersistenceError(err, service.ErrBundleNotFound, nil)
	}
	items, err := query.
		Offset(params.Offset).
		Limit(params.Limit).
		Order(dbent.Desc(bundlesubscription.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, nil, translatePersistenceError(err, service.ErrBundleNotFound, nil)
	}
	subs := make([]service.BundleSubscription, len(items))
	for i, item := range items {
		subs[i] = *bundleSubscriptionToService(item)
	}
	pagResult := pagination.NewPaginationResult(params, int64(total))
	return subs, pagResult, nil
}

func (r *bundleSubscriptionRepository) UpdateStatus(ctx context.Context, id int64, status string) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.BundleSubscription.UpdateOneID(id).SetStatus(status).Save(ctx)
	return translatePersistenceError(err, service.ErrBundleNotFound, nil)
}

func (r *bundleSubscriptionRepository) UpdateExpiry(ctx context.Context, id int64, expiresAt interface{}) error {
	client := clientFromContext(ctx, r.client)
	builder := client.BundleSubscription.UpdateOneID(id)
	switch v := expiresAt.(type) {
	case time.Time:
		builder.SetExpiresAt(v)
	}
	_, err := builder.Save(ctx)
	return translatePersistenceError(err, service.ErrBundleNotFound, nil)
}

func bundleSubscriptionToService(m *ent.BundleSubscription) *service.BundleSubscription {
	return &service.BundleSubscription{
		ID:               m.ID,
		UserID:           m.UserID,
		PlanID:           m.PlanID,
		Status:           m.Status,
		StartsAt:         m.StartsAt,
		ExpiresAt:        m.ExpiresAt,
		ConcurrencyLimit: m.ConcurrencyLimit,
		RPMLimit:         m.RpmLimit,
		Source:           m.Source,
	}
}

func bundleSubscriptionUsageToService(m *ent.BundleSubscriptionUsage) *service.BundleSubscriptionUsage {
	return &service.BundleSubscriptionUsage{
		ID:                   m.ID,
		BundleSubscriptionID: m.BundleSubscriptionID,
		GroupID:              m.GroupID,
		ModelPattern:         m.ModelPattern,
		DailyUsageUSD:        m.DailyUsageUsd,
		DailyWindowStart:     m.DailyWindowStart,
		WeeklyUsageUSD:       m.WeeklyUsageUsd,
		WeeklyWindowStart:    m.WeeklyWindowStart,
		MonthlyUsageUSD:      m.MonthlyUsageUsd,
		MonthlyWindowStart:   m.MonthlyWindowStart,
	}
}
```

- [ ] **Step 3: Create BundleUsageRepo**

```go
package repository

import (
	"context"

	"sub2api/backend/ent"
	dbent "sub2api/backend/ent"
	"sub2api/backend/ent/bundlesubscriptionusage"
	"sub2api/backend/internal/service"
)

type bundleUsageRepository struct {
	client *dbent.Client
}

func NewBundleUsageRepository(client *dbent.Client) service.BundleUsageRepository {
	return &bundleUsageRepository{client: client}
}

func (r *bundleUsageRepository) GetBySubscriptionAndGroup(ctx context.Context, subscriptionID, groupID int64) (*service.BundleSubscriptionUsage, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.BundleSubscriptionUsage.Query().
		Where(
			bundlesubscriptionusage.BundleSubscriptionID(subscriptionID),
			bundlesubscriptionusage.GroupID(groupID),
		).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBundleNotFound, nil)
	}
	return bundleSubscriptionUsageToService(m), nil
}

func (r *bundleUsageRepository) Create(ctx context.Context, usage *service.BundleSubscriptionUsage) (*service.BundleSubscriptionUsage, error) {
	client := clientFromContext(ctx, r.client)
	created, err := client.BundleSubscriptionUsage.Create().
		SetBundleSubscriptionID(usage.BundleSubscriptionID).
		SetGroupID(usage.GroupID).
		SetModelPattern(usage.ModelPattern).
		Save(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBundleNotFound, nil)
	}
	return bundleSubscriptionUsageToService(created), nil
}

func (r *bundleUsageRepository) IncrementUsage(ctx context.Context, subscriptionID, groupID int64, costUSD float64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.BundleSubscriptionUsage.Update().
		Where(
			bundlesubscriptionusage.BundleSubscriptionID(subscriptionID),
			bundlesubscriptionusage.GroupID(groupID),
		).
		AddDailyUsageUsd(costUSD).
		AddWeeklyUsageUsd(costUSD).
		AddMonthlyUsageUsd(costUSD).
		Save(ctx)
	return translatePersistenceError(err, service.ErrBundleNotFound, nil)
}

func (r *bundleUsageRepository) ResetDailyWindow(ctx context.Context, subscriptionID, groupID int64, windowStart interface{}) error {
	client := clientFromContext(ctx, r.client)
	builder := client.BundleSubscriptionUsage.Update().
		Where(
			bundlesubscriptionusage.BundleSubscriptionID(subscriptionID),
			bundlesubscriptionusage.GroupID(groupID),
		).
		SetDailyUsageUsd(0)
	switch v := windowStart.(type) {
	case time.Time:
		builder.SetDailyWindowStart(v)
	}
	_, err := builder.Save(ctx)
	return translatePersistenceError(err, service.ErrBundleNotFound, nil)
}

func (r *bundleUsageRepository) ResetWeeklyWindow(ctx context.Context, subscriptionID, groupID int64, windowStart interface{}) error {
	client := clientFromContext(ctx, r.client)
	builder := client.BundleSubscriptionUsage.Update().
		Where(
			bundlesubscriptionusage.BundleSubscriptionID(subscriptionID),
			bundlesubscriptionusage.GroupID(groupID),
		).
		SetWeeklyUsageUsd(0)
	switch v := windowStart.(type) {
	case time.Time:
		builder.SetWeeklyWindowStart(v)
	}
	_, err := builder.Save(ctx)
	return translatePersistenceError(err, service.ErrBundleNotFound, nil)
}

func (r *bundleUsageRepository) ResetMonthlyWindow(ctx context.Context, subscriptionID, groupID int64, windowStart interface{}) error {
	client := clientFromContext(ctx, r.client)
	builder := client.BundleSubscriptionUsage.Update().
		Where(
			bundlesubscriptionusage.BundleSubscriptionID(subscriptionID),
			bundlesubscriptionusage.GroupID(groupID),
		).
		SetMonthlyUsageUsd(0)
	switch v := windowStart.(type) {
	case time.Time:
		builder.SetMonthlyWindowStart(v)
	}
	_, err := builder.Save(ctx)
	return translatePersistenceError(err, service.ErrBundleNotFound, nil)
}

func (r *bundleUsageRepository) ListBySubscription(ctx context.Context, subscriptionID int64) ([]service.BundleSubscriptionUsage, error) {
	client := clientFromContext(ctx, r.client)
	items, err := client.BundleSubscriptionUsage.Query().
		Where(bundlesubscriptionusage.BundleSubscriptionID(subscriptionID)).
		All(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBundleNotFound, nil)
	}
	usages := make([]service.BundleSubscriptionUsage, len(items))
	for i, item := range items {
		usages[i] = *bundleSubscriptionUsageToService(item)
	}
	return usages, nil
}

func (r *bundleUsageRepository) BatchUpdateExpiredStatus(ctx context.Context) (int64, error) {
	client := clientFromContext(ctx, r.client)
	n, err := client.BundleSubscription.Update().
		Where(
			bundlesubscription.Status("active"),
			bundlesubscription.ExpiresAtLTE(time.Now()),
		).
		SetStatus("expired").
		Save(ctx)
	if err != nil {
		return 0, err
	}
	return int64(n), nil
}
```

- [ ] **Step 4: Commit**

```bash
git add backend/internal/repository/bundle_plan_repo.go backend/internal/repository/bundle_subscription_repo.go backend/internal/repository/bundle_usage_repo.go
git commit -m "feat(bundle): add repository implementations"
```

---

### Task 9: Register Repositories in Wire + Verify Compilation

**Files:**
- Modify: `backend/internal/repository/wire.go` — add 3 providers

- [ ] **Step 1: Add providers to repository wire.go**

在 `ProviderSet` 中追加：

```go
	NewBundlePlanRepository,
	NewBundleSubscriptionRepository,
	NewBundleUsageRepository,
```

- [ ] **Step 2: Verify compilation**

Run: `cd backend && go build ./internal/repository/...`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add backend/internal/repository/wire.go
git commit -m "feat(bundle): register bundle repositories in wire"
```

---

## Phase 4: Service Layer

### Task 10: Create BundlePlanService

**Files:**
- Create: `backend/internal/service/bundle_plan_service.go`

- [ ] **Step 1: Create service**

遵循 `SubscriptionService` 的模式（构造函数注入依赖）：

```go
package service

import (
	"context"
	"sub2api/backend/internal/pagination"
)

type BundlePlanService struct {
	bundlePlanRepo BundlePlanRepository
}

func NewBundlePlanService(bundlePlanRepo BundlePlanRepository) *BundlePlanService {
	return &BundlePlanService{bundlePlanRepo: bundlePlanRepo}
}

func (s *BundlePlanService) CreatePlan(ctx context.Context, req *CreateBundlePlanRequest) (*BundlePlan, error) {
	plan := &BundlePlan{
		Name:             req.Name,
		Description:      req.Description,
		Tier:             req.Tier,
		Price:            req.Price,
		OriginalPrice:    req.OriginalPrice,
		Currency:         req.Currency,
		ValidityDays:     req.ValidityDays,
		ConcurrencyLimit: req.ConcurrencyLimit,
		RPMLimit:         req.RPMLimit,
		Features:         req.Features,
		ForSale:          true,
		Status:           BundleStatusActive,
	}
	quotas := make([]BundlePlanGroupQuota, len(req.GroupQuotas))
	for i, q := range req.GroupQuotas {
		quotas[i] = BundlePlanGroupQuota{
			GroupID:         q.GroupID,
			QuotaScope:      q.QuotaScope,
			ModelPattern:    q.ModelPattern,
			DailyLimitUSD:   q.DailyLimitUSD,
			WeeklyLimitUSD:  q.WeeklyLimitUSD,
			MonthlyLimitUSD: q.MonthlyLimitUSD,
		}
	}
	return s.bundlePlanRepo.Create(ctx, plan, quotas)
}

func (s *BundlePlanService) UpdatePlan(ctx context.Context, id int64, req *UpdateBundlePlanRequest) (*BundlePlan, error) {
	existing, err := s.bundlePlanRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.Tier != nil {
		existing.Tier = *req.Tier
	}
	if req.Price != nil {
		existing.Price = *req.Price
	}
	if req.OriginalPrice != nil {
		existing.OriginalPrice = *req.OriginalPrice
	}
	if req.ValidityDays != nil {
		existing.ValidityDays = *req.ValidityDays
	}
	if req.ConcurrencyLimit != nil {
		existing.ConcurrencyLimit = *req.ConcurrencyLimit
	}
	if req.RPMLimit != nil {
		existing.RPMLimit = *req.RPMLimit
	}
	if req.Features != nil {
		existing.Features = req.Features
	}
	if req.ForSale != nil {
		existing.ForSale = *req.ForSale
	}
	if req.SortOrder != nil {
		existing.SortOrder = *req.SortOrder
	}
	if req.Status != nil {
		existing.Status = *req.Status
	}
	quotas := existing.GroupQuotas
	if req.GroupQuotas != nil {
		quotas = make([]BundlePlanGroupQuota, len(req.GroupQuotas))
		for i, q := range req.GroupQuotas {
			quotas[i] = BundlePlanGroupQuota{
				GroupID:         q.GroupID,
				QuotaScope:      q.QuotaScope,
				ModelPattern:    q.ModelPattern,
				DailyLimitUSD:   q.DailyLimitUSD,
				WeeklyLimitUSD:  q.WeeklyLimitUSD,
				MonthlyLimitUSD: q.MonthlyLimitUSD,
			}
		}
	}
	return s.bundlePlanRepo.Update(ctx, existing, quotas)
}

func (s *BundlePlanService) GetPlanDetail(ctx context.Context, id int64) (*BundlePlan, error) {
	return s.bundlePlanRepo.GetByID(ctx, id)
}

func (s *BundlePlanService) ListPlans(ctx context.Context, params pagination.PaginationParams, tier, status string) ([]BundlePlan, *pagination.PaginationResult, error) {
	return s.bundlePlanRepo.List(ctx, params, tier, status)
}

func (s *BundlePlanService) ListForSale(ctx context.Context) ([]BundlePlan, error) {
	return s.bundlePlanRepo.ListForSale(ctx)
}
```

- [ ] **Step 2: Commit**

```bash
git add backend/internal/service/bundle_plan_service.go
git commit -m "feat(bundle): add BundlePlanService"
```

---

### Task 11: Create BundleSubscriptionService

**Files:**
- Create: `backend/internal/service/bundle_subscription_service.go`

- [ ] **Step 1: Create service**

```go
package service

import (
	"context"
	"time"
)

type BundleSubscriptionService struct {
	bundleSubRepo   BundleSubscriptionRepository
	bundlePlanRepo  BundlePlanRepository
	bundleUsageRepo BundleUsageRepository
	userSubRepo     UserSubscriptionRepository
}

func NewBundleSubscriptionService(
	bundleSubRepo BundleSubscriptionRepository,
	bundlePlanRepo BundlePlanRepository,
	bundleUsageRepo BundleUsageRepository,
	userSubRepo UserSubscriptionRepository,
) *BundleSubscriptionService {
	return &BundleSubscriptionService{
		bundleSubRepo:   bundleSubRepo,
		bundlePlanRepo:  bundlePlanRepo,
		bundleUsageRepo: bundleUsageRepo,
		userSubRepo:     userSubRepo,
	}
}

// ActivateBundle creates a BundleSubscription + bridged UserSubscriptions
func (s *BundleSubscriptionService) ActivateBundle(ctx context.Context, userID int64, planID int64, source string) (*BundleSubscription, error) {
	// Check no active bundle already exists
	existing, _ := s.bundleSubRepo.GetActiveByUserID(ctx, userID)
	if existing != nil {
		return nil, ErrBundleConflict
	}

	// Load plan
	plan, err := s.bundlePlanRepo.GetByID(ctx, planID)
	if err != nil {
		return nil, err
	}
	if !plan.ForSale || plan.Status != BundleStatusActive {
		return nil, ErrBundlePlanDisabled
	}

	now := time.Now()
	expiresAt := now.AddDate(0, 0, plan.ValidityDays)

	// Create BundleSubscription
	bundleSub := &BundleSubscription{
		UserID:           userID,
		PlanID:           planID,
		Status:           BundleStatusActive,
		StartsAt:         now,
		ExpiresAt:        expiresAt,
		ConcurrencyLimit: plan.ConcurrencyLimit,
		RPMLimit:         plan.RPMLimit,
		Source:           source,
	}
	created, err := s.bundleSubRepo.Create(ctx, bundleSub)
	if err != nil {
		return nil, err
	}

	// For each Group quota: create BundleSubscriptionUsage + bridged UserSubscription
	for _, quota := range plan.GroupQuotas {
		// Create usage tracker
		usage := &BundleSubscriptionUsage{
			BundleSubscriptionID: created.ID,
			GroupID:              quota.GroupID,
			ModelPattern:         quota.ModelPattern,
		}
		_, _ = s.bundleUsageRepo.Create(ctx, usage)

		// Create bridged UserSubscription
		userSub := &UserSubscription{
			UserID:               userID,
			GroupID:              quota.GroupID,
			Status:               SubscriptionStatusActive,
			StartsAt:             now,
			ExpiresAt:            expiresAt,
			DailyLimitUSD:        quota.DailyLimitUSD,
			WeeklyLimitUSD:       quota.WeeklyLimitUSD,
			MonthlyLimitUSD:      quota.MonthlyLimitUSD,
			BundleSubscriptionID: &created.ID,
		}
		_ = s.userSubRepo.Create(ctx, userSub)
	}

	return created, nil
}

func (s *BundleSubscriptionService) RevokeBundle(ctx context.Context, id int64) error {
	sub, err := s.bundleSubRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if sub.Status != BundleStatusActive {
		return ErrBundleNotFound
	}
	return s.bundleSubRepo.UpdateStatus(ctx, id, BundleStatusRevoked)
}

func (s *BundleSubscriptionService) GetUserActiveBundle(ctx context.Context, userID int64) (*BundleSubscription, error) {
	return s.bundleSubRepo.GetActiveByUserID(ctx, userID)
}

func (s *BundleSubscriptionService) GetBundleUsageProgress(ctx context.Context, userID int64) ([]BundleUsageProgress, error) {
	bundleSub, err := s.bundleSubRepo.GetActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	usages, err := s.bundleUsageRepo.ListBySubscription(ctx, bundleSub.ID)
	if err != nil {
		return nil, err
	}
	// Load plan for limits
	plan, err := s.bundlePlanRepo.GetByID(ctx, bundleSub.PlanID)
	if err != nil {
		return nil, err
	}
	progress := make([]BundleUsageProgress, 0, len(usages))
	for _, u := range usages {
		p := BundleUsageProgress{
			GroupID:      u.GroupID,
			ModelPattern: u.ModelPattern,
			DailyUsed:    u.DailyUsageUSD,
			WeeklyUsed:   u.WeeklyUsageUSD,
			MonthlyUsed:  u.MonthlyUsageUSD,
		}
		// Find matching quota for limits
		for _, q := range plan.GroupQuotas {
			if q.GroupID == u.GroupID {
				p.DailyLimit = q.DailyLimitUSD
				p.WeeklyLimit = q.WeeklyLimitUSD
				p.MonthlyLimit = q.MonthlyLimitUSD
				break
			}
		}
		progress = append(progress, p)
	}
	return progress, nil
}
```

**注意**：`UserSubscription` 结构体需要新增的 4 个字段（`BundleSubscriptionID`, `DailyLimitUSD`, `WeeklyLimitUSD`, `MonthlyLimitUSD`）。这些字段已在 Task 4 中添加到 Ent schema，但 Service 层的 `UserSubscription` model 也需要相应更新。如果 Service 层使用的是 Ent 生成的类型，则无需手动更新 model；如果是手动定义的 model，需要追加字段。

- [ ] **Step 2: Commit**

```bash
git add backend/internal/service/bundle_subscription_service.go
git commit -m "feat(bundle): add BundleSubscriptionService"
```

---

### Task 12: Create BundleRouteResolver + BundleUsageService

**Files:**
- Create: `backend/internal/service/bundle_route_resolver.go`
- Create: `backend/internal/service/bundle_usage_service.go`

- [ ] **Step 1: Create route resolver**

```go
package service

import (
	"context"
	"strings"
)

// ModelPrefixToPlatform maps model name prefixes to platform identifiers.
var ModelPrefixToPlatform = map[string]string{
	"gpt-":      "openai",
	"o1-":       "openai",
	"o3-":       "openai",
	"chatgpt-":  "openai",
	"claude-":   "anthropic",
	"gemini-":   "gemini",
	"deepseek-": "openai",
}

// PlatformToGroupPlatform maps our short names to Group platform constants.
var PlatformToGroupPlatform = map[string]string{
	"openai":    GroupPlatformOpenAI,
	"anthropic": GroupPlatformAnthropic,
	"gemini":    GroupPlatformGemini,
}

type BundleRouteResolver struct {
	bundleSubRepo  BundleSubscriptionRepository
	bundlePlanRepo BundlePlanRepository
	groupRepo      GroupRepository
}

func NewBundleRouteResolver(
	bundleSubRepo BundleSubscriptionRepository,
	bundlePlanRepo BundlePlanRepository,
	groupRepo GroupRepository,
) *BundleRouteResolver {
	return &BundleRouteResolver{
		bundleSubRepo:  bundleSubRepo,
		bundlePlanRepo: bundlePlanRepo,
		groupRepo:      groupRepo,
	}
}

// ResolveGroup maps a model name to the appropriate Group ID within a bundle.
func (r *BundleRouteResolver) ResolveGroup(ctx context.Context, modelName string, bundleSubID int64) (int64, error) {
	// Get the bundle subscription
	sub, err := r.bundleSubRepo.GetByID(ctx, bundleSubID)
	if err != nil {
		return 0, err
	}
	if sub.Status != BundleStatusActive {
		return 0, ErrBundleExpired
	}

	// Load plan with quotas
	plan, err := r.bundlePlanRepo.GetByID(ctx, sub.PlanID)
	if err != nil {
		return 0, err
	}

	// Determine platform from model name
	platform := resolveModelPlatform(modelName)

	// Try model-level match first (if quota_scope == "model")
	for _, quota := range plan.GroupQuotas {
		if quota.QuotaScope == QuotaScopeModel && quota.ModelPattern != "" {
			if matchGlob(quota.ModelPattern, modelName) {
				return quota.GroupID, nil
			}
		}
	}

	// Fallback to platform-level match
	for _, quota := range plan.GroupQuotas {
		if quota.QuotaScope == QuotaScopePlatform || quota.ModelPattern == "" {
			group, err := r.groupRepo.GetByID(ctx, quota.GroupID)
			if err != nil {
				continue
			}
			if group.Platform == platform {
				return quota.GroupID, nil
			}
		}
	}

	return 0, ErrBundleModelNotIncluded
}

// resolveModelPlatform determines the platform from a model name.
func resolveModelPlatform(modelName string) string {
	lower := strings.ToLower(modelName)
	for prefix, platform := range ModelPrefixToPlatform {
		if strings.HasPrefix(lower, prefix) {
			return PlatformToGroupPlatform[platform]
		}
	}
	// Default to openai for unknown models (openai-compatible)
	return GroupPlatformOpenAI
}

// matchGlob performs simple glob matching (only supports * wildcard).
func matchGlob(pattern, s string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == s
	}
	parts := strings.SplitN(pattern, "*", 2)
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	if len(parts) == 1 {
		return s == parts[0]
	}
	return strings.HasSuffix(s, parts[1]) || parts[1] == ""
}
```

- [ ] **Step 2: Create usage service**

```go
package service

import "context"

type BundleUsageService struct {
	bundleUsageRepo BundleUsageRepository
	bundleSubRepo   BundleSubscriptionRepository
	bundlePlanRepo  BundlePlanRepository
}

func NewBundleUsageService(
	bundleUsageRepo BundleUsageRepository,
	bundleSubRepo BundleSubscriptionRepository,
	bundlePlanRepo BundlePlanRepository,
) *BundleUsageService {
	return &BundleUsageService{
		bundleUsageRepo: bundleUsageRepo,
		bundleSubRepo:   bundleSubRepo,
		bundlePlanRepo:  bundlePlanRepo,
	}
}

// AccumulateUsage increments usage for a specific Group within a bundle subscription.
func (s *BundleUsageService) AccumulateUsage(ctx context.Context, bundleSubID, groupID int64, costUSD float64) error {
	return s.bundleUsageRepo.IncrementUsage(ctx, bundleSubID, groupID, costUSD)
}

// CheckQuotaEligibility checks if the quota for a specific Group is within limits.
func (s *BundleUsageService) CheckQuotaEligibility(ctx context.Context, bundleSubID, groupID int64) error {
	usage, err := s.bundleUsageRepo.GetBySubscriptionAndGroup(ctx, bundleSubID, groupID)
	if err != nil {
		return nil // No usage record = first use, allow
	}
	// Load plan for limits
	sub, err := s.bundleSubRepo.GetByID(ctx, bundleSubID)
	if err != nil {
		return err
	}
	plan, err := s.bundlePlanRepo.GetByID(ctx, sub.PlanID)
	if err != nil {
		return err
	}
	// Find matching quota
	for _, q := range plan.GroupQuotas {
		if q.GroupID == groupID {
			if q.DailyLimitUSD > 0 && usage.DailyUsageUSD >= q.DailyLimitUSD {
				return ErrBundleGroupQuotaExceeded
			}
			if q.WeeklyLimitUSD > 0 && usage.WeeklyUsageUSD >= q.WeeklyLimitUSD {
				return ErrBundleGroupQuotaExceeded
			}
			if q.MonthlyLimitUSD > 0 && usage.MonthlyUsageUSD >= q.MonthlyLimitUSD {
				return ErrBundleGroupQuotaExceeded
			}
			break
		}
	}
	return nil
}
```

- [ ] **Step 3: Commit**

```bash
git add backend/internal/service/bundle_route_resolver.go backend/internal/service/bundle_usage_service.go
git commit -m "feat(bundle): add BundleRouteResolver and BundleUsageService"
```

---

### Task 13: Register Services in Wire

**Files:**
- Modify: `backend/internal/service/wire.go` — add 4 providers

- [ ] **Step 1: Add providers to service wire.go**

在 `ProviderSet` 中追加：

```go
	NewBundlePlanService,
	NewBundleSubscriptionService,
	NewBundleRouteResolver,
	NewBundleUsageService,
```

- [ ] **Step 2: Verify compilation**

Run: `cd backend && go build ./internal/service/...`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add backend/internal/service/wire.go
git commit -m "feat(bundle): register bundle services in wire"
```

---

## Phase 5: Gateway Integration (Critical — modifies existing code)

### Task 14: Create Bundle Resolver Middleware

**Files:**
- Create: `backend/internal/server/middleware/bundle_resolver.go`

- [ ] **Step 1: Create middleware**

遵循 `middleware.go` 中 `RequireGroupAssignment` 的模式：

```go
package middleware

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"sub2api/backend/internal/handler/response"
	"sub2api/backend/internal/service"
)

type BundleRouteResolverMiddleware struct {
	resolver *service.BundleRouteResolver
}

func NewBundleRouteResolverMiddleware(resolver *service.BundleRouteResolver) *BundleRouteResolverMiddleware {
	return &BundleRouteResolverMiddleware{resolver: resolver}
}

// BundleResolver detects bundle API Keys and resolves model → Group.
// Must be placed after APIKeyAuth middleware and before RequireGroupAssignment.
func (m *BundleRouteResolverMiddleware) BundleResolver() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey, ok := GetAPIKeyFromContext(c)
		if !ok {
			c.Next()
			return
		}
		// Only handle bundle keys (bundle_subscription_id set, group_id nil)
		if apiKey.BundleSubscriptionID == nil || apiKey.GroupID != nil {
			c.Next()
			return
		}

		// Extract model name from request body
		modelName := extractModelFromRequest(c)
		if modelName == "" {
			c.Next()
			return
		}

		// Resolve group
		groupID, err := m.resolver.ResolveGroup(c.Request.Context(), modelName, *apiKey.BundleSubscriptionID)
		if err != nil {
			status := http.StatusBadRequest
			code := "BUNDLE_MODEL_NOT_INCLUDED"
			msg := err.Error()
			if err == service.ErrBundleExpired {
				status = http.StatusForbidden
				code = "BUNDLE_EXPIRED"
			}
			c.JSON(status, gin.H{"error": gin.H{"type": code, "message": msg}})
			c.Abort()
			return
		}

		// Inject resolved group_id into context for downstream middleware/handlers
		c.Set("bundle_resolved_group_id", groupID)
		c.Next()
	}
}

// extractModelFromRequest reads the model field from request body without consuming it.
func extractModelFromRequest(c *gin.Context) string {
	// Try query parameter first (for GET /models etc.)
	if model := c.Query("model"); model != "" {
		return model
	}
	// Read body, extract model, put it back
	if c.Request.Body == nil {
		return ""
	}
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil || len(bodyBytes) == 0 {
		return ""
	}
	// Restore body for downstream handlers
	c.Request.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))

	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		return ""
	}
	return req.Model
}
```

- [ ] **Step 2: Commit**

```bash
git add backend/internal/server/middleware/bundle_resolver.go
git commit -m "feat(bundle): add bundle resolver middleware for single-key auto-routing"
```

---

### Task 15: Modify RequireGroupAssignment Middleware

**Files:**
- Modify: `backend/internal/server/middleware/middleware.go` — ~5 lines

- [ ] **Step 1: Add bundle branch to RequireGroupAssignment**

在 `RequireGroupAssignment` 函数中，在 `apiKey.GroupID != nil` 判断后追加 bundle 判断：

找到：
```go
func RequireGroupAssignment(settingService *service.SettingService, writeError GatewayErrorWriter) gin.HandlerFunc {
    return func(c *gin.Context) {
        apiKey, ok := GetAPIKeyFromContext(c)
        if !ok || apiKey.GroupID != nil {
            c.Next()
            return
        }
```

在 `apiKey.GroupID != nil` 条件后追加（在检查 `IsUngroupedKeySchedulingAllowed` 之前）：

```go
        // Bundle key: group resolved by middleware, allow through
        if _, exists := c.Get("bundle_resolved_group_id"); exists {
            c.Next()
            return
        }
```

- [ ] **Step 2: Verify compilation**

Run: `cd backend && go build ./internal/server/middleware/...`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add backend/internal/server/middleware/middleware.go
git commit -m "feat(bundle): allow bundle-resolved keys through RequireGroupAssignment"
```

---

### Task 16: Modify checkSubscriptionEligibility — Limit Fallback

**Files:**
- Modify: `backend/internal/service/billing_cache_service.go` — ~10 lines

- [ ] **Step 1: Add limit fallback to checkSubscriptionEligibility**

找到 `checkSubscriptionEligibility` 函数中的限额检查代码：

```go
    if group.HasDailyLimit() && subData.DailyUsage >= *group.DailyLimitUSD { return ErrDailyLimitExceeded }
    if group.HasWeeklyLimit() && subData.WeeklyUsage >= *group.WeeklyLimitUSD { return ErrWeeklyLimitExceeded }
    if group.HasMonthlyLimit() && subData.MonthlyUsage >= *group.MonthlyLimitUSD { return ErrMonthlyLimitExceeded }
```

替换为：

```go
    // Limit source: prefer subscription's own limit (bundle snapshot), fallback to group
    dailyLimit := group.DailyLimitUSD
    if subData.DailyLimit > 0 {
        dailyLimit = &subData.DailyLimit
    }
    weeklyLimit := group.WeeklyLimitUSD
    if subData.WeeklyLimit > 0 {
        weeklyLimit = &subData.WeeklyLimit
    }
    monthlyLimit := group.MonthlyLimitUSD
    if subData.MonthlyLimit > 0 {
        monthlyLimit = &subData.MonthlyLimit
    }

    if dailyLimit != nil && *dailyLimit > 0 && subData.DailyUsage >= *dailyLimit { return ErrDailyLimitExceeded }
    if weeklyLimit != nil && *weeklyLimit > 0 && subData.WeeklyUsage >= *weeklyLimit { return ErrWeeklyLimitExceeded }
    if monthlyLimit != nil && *monthlyLimit > 0 && subData.MonthlyUsage >= *monthlyLimit { return ErrMonthlyLimitExceeded }
```

**注意**：`subData` 的类型需要包含 `DailyLimit`, `WeeklyLimit`, `MonthlyLimit` 字段。查看 `GetSubscriptionStatus` 返回的数据结构，确认这些字段已包含在缓存数据中。如果缓存结构是独立的 struct，需要同步添加字段。

- [ ] **Step 2: Verify compilation**

Run: `cd backend && go build ./internal/service/...`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add backend/internal/service/billing_cache_service.go
git commit -m "feat(bundle): add subscription-level limit fallback in checkSubscriptionEligibility"
```

---

### Task 17: Modify postUsageBilling — Bundle Usage Accumulation

**Files:**
- Modify: `backend/internal/service/gateway_service.go` — ~15 lines

- [ ] **Step 1: Add bundle usage accumulation branch**

在 `postUsageBilling` 函数中，找到 subscription 用量扣减的位置（`SubscriptionCost` 相关逻辑后），追加：

```go
        // Bundle usage accumulation
        if params.Subscription != nil && params.Subscription.BundleSubscriptionID != nil {
            bundleUsageSvc.AccumulateUsage(ctx, *params.Subscription.BundleSubscriptionID, params.Subscription.GroupID, cost)
        }
```

**注意**：`BundleSubscriptionID` 是在 Task 4 中新增到 UserSubscription 的字段。`bundleUsageSvc` 需要通过依赖注入传入。如果 `postUsageBilling` 是在 `GatewayService` 的方法中调用的，则需要将 `BundleUsageService` 作为 `GatewayService` 的依赖注入。具体注入方式参考 `GatewayService` 的构造函数。

- [ ] **Step 2: Verify compilation**

Run: `cd backend && go build ./internal/service/...`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add backend/internal/service/gateway_service.go
git commit -m "feat(bundle): add bundle usage accumulation in postUsageBilling"
```

---

## Phase 6: Handler Layer + Routes

### Task 18: Create Bundle Handlers

**Files:**
- Create: `backend/internal/handler/bundle_handler.go`
- Create: `backend/internal/handler/bundle_admin_handler.go`

- [ ] **Step 1: Create user-facing handler**

遵循 `subscription_handler.go` 的模式。实现以下方法：
- `ListPlans` — GET `/bundles/plans`
- `GetPlanDetail` — GET `/bundles/plans/:id`
- `GetMyBundle` — GET `/bundles/subscription`
- `GetMyUsage` — GET `/bundles/subscription/usage`
- `Checkout` — POST `/bundles/checkout`

每个方法：
1. 用 `middleware2.GetAuthSubjectFromContext(c)` 获取当前用户
2. 调用对应 service 方法
3. 用 `response.Success(c, result)` 或 `response.ErrorFrom(c, err)` 返回

- [ ] **Step 2: Create admin handler**

遵循 `admin/subscription_handler.go` 的模式。实现以下方法：
- `CreatePlan` — POST `/admin/bundles/plans`
- `UpdatePlan` — PUT `/admin/bundles/plans/:id`
- `ListPlans` — GET `/admin/bundles/plans`
- `GetPlanDetail` — GET `/admin/bundles/plans/:id`
- `DisablePlan` — DELETE `/admin/bundles/plans/:id`
- `ListSubscriptions` — GET `/admin/bundles/subscriptions`
- `RevokeSubscription` — POST `/admin/bundles/subscriptions/:id/revoke`
- `ExtendSubscription` — POST `/admin/bundles/subscriptions/:id/extend`

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handler/bundle_handler.go backend/internal/handler/bundle_admin_handler.go
git commit -m "feat(bundle): add bundle user and admin handlers"
```

---

### Task 19: Register Handlers + Routes in Wire

**Files:**
- Modify: `backend/internal/handler/handler.go` — add Bundle fields
- Modify: `backend/internal/handler/wire.go` — add providers
- Create: `backend/internal/server/routes/bundle.go` — route registration
- Modify: `backend/internal/server/routes/admin.go` — add bundle routes

- [ ] **Step 1: Add fields to Handlers struct in handler.go**

在 `Handlers` 中追加：
```go
    Bundle *BundleHandler
```

在 `AdminHandlers` 中追加：
```go
    Bundle *BundleAdminHandler
```

在 `ProvideHandlers` 构造函数中添加参数和赋值。

- [ ] **Step 2: Add providers to handler wire.go**

追加：
```go
    NewBundleHandler,
    NewBundleAdminHandler,
```

- [ ] **Step 3: Create bundle route registration**

遵循 `admin.go` 中 `registerSubscriptionRoutes` 的模式：

```go
package routes

import (
	"github.com/gin-gonic/gin"
	"sub2api/backend/internal/handler"
)

func RegisterBundleRoutes(r *gin.Engine, h *handler.Handlers, apiKeyAuth, requireAuth gin.HandlerFunc) {
	bundles := r.Group("/bundles")
	bundles.Use(requireAuth)
	{
		bundles.GET("/plans", h.Bundle.ListPlans)
		bundles.GET("/plans/:id", h.Bundle.GetPlanDetail)
		bundles.GET("/subscription", h.Bundle.GetMyBundle)
		bundles.GET("/subscription/usage", h.Bundle.GetMyUsage)
		bundles.POST("/checkout", h.Bundle.Checkout)
	}
}
```

- [ ] **Step 4: Add admin bundle routes in admin.go**

追加注册函数：
```go
func registerBundleRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
    plans := admin.Group("/bundles/plans")
    {
        plans.POST("", h.Admin.Bundle.CreatePlan)
        plans.PUT("/:id", h.Admin.Bundle.UpdatePlan)
        plans.GET("", h.Admin.Bundle.ListPlans)
        plans.GET("/:id", h.Admin.Bundle.GetPlanDetail)
        plans.DELETE("/:id", h.Admin.Bundle.DisablePlan)
    }
    subs := admin.Group("/bundles/subscriptions")
    {
        subs.GET("", h.Admin.Bundle.ListSubscriptions)
        subs.POST("/:id/revoke", h.Admin.Bundle.RevokeSubscription)
        subs.POST("/:id/extend", h.Admin.Bundle.ExtendSubscription)
    }
}
```

并在 `RegisterAdminRoutes` 中调用 `registerBundleRoutes(admin, h)`。

- [ ] **Step 5: Register bundle_resolver middleware in gateway routes**

在 `gateway.go` 的路由链中，在 `apiKeyAuth` 之后、`requireGroup` 之前添加 `bundleResolver.BundleResolver()`。

- [ ] **Step 6: Update Wire ProviderSets and regenerate**

在 `middleware/wire.go` 或相关位置注册 `NewBundleRouteResolverMiddleware`。
运行：`cd backend && go generate ./cmd/server`

- [ ] **Step 7: Verify full compilation**

Run: `cd backend && go build ./...`
Expected: 编译成功

- [ ] **Step 8: Commit**

```bash
git add backend/internal/handler/handler.go backend/internal/handler/wire.go backend/internal/server/routes/bundle.go backend/internal/server/routes/admin.go backend/internal/server/routes/gateway.go
git commit -m "feat(bundle): register handlers, routes, and middleware"
```

---

## Phase 7: Regenerate Wire + End-to-End Verification

### Task 20: Regenerate Wire + Full Build

- [ ] **Step 1: Regenerate Wire DI code**

Run: `cd backend && go generate ./cmd/server`
Expected: `wire_gen.go` 更新成功

- [ ] **Step 2: Full build**

Run: `cd backend && go build -o /dev/null ./cmd/server`
Expected: 编译成功

- [ ] **Step 3: Run existing tests to verify no regressions**

Run: `cd backend && go test -tags=unit ./...`
Expected: 所有现有测试通过

- [ ] **Step 4: Commit**

```bash
git add backend/cmd/server/wire_gen.go
git commit -m "feat(bundle): regenerate wire DI code"
```

---

## Phase 8: Unit Tests

### Task 21: Write Unit Tests for Bundle Services

**Files:**
- Create: `backend/internal/service/bundle_plan_service_test.go`
- Create: `backend/internal/service/bundle_subscription_service_test.go`
- Create: `backend/internal/service/bundle_route_resolver_test.go`

- [ ] **Step 1: Write BundleRouteResolver tests**

```go
package service

import (
	"testing"
)

func TestResolveModelPlatform(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		{"gpt-4o", GroupPlatformOpenAI},
		{"gpt-4.1-mini", GroupPlatformOpenAI},
		{"o1-preview", GroupPlatformOpenAI},
		{"o3-mini", GroupPlatformOpenAI},
		{"claude-opus-4-8-20250609", GroupPlatformAnthropic},
		{"claude-sonnet-4-6-20250514", GroupPlatformAnthropic},
		{"gemini-2.5-pro", GroupPlatformGemini},
		{"deepseek-chat", GroupPlatformOpenAI},
		{"deepseek-reasoner", GroupPlatformOpenAI},
		{"unknown-model", GroupPlatformOpenAI},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := resolveModelPlatform(tt.model)
			if got != tt.expected {
				t.Errorf("resolveModelPlatform(%q) = %q, want %q", tt.model, got, tt.expected)
			}
		})
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		s       string
		match   bool
	}{
		{"claude-opus-*", "claude-opus-4-8-20250609", true},
		{"claude-opus-*", "claude-sonnet-4-6", false},
		{"gpt-4*", "gpt-4o", true},
		{"gpt-4*", "gpt-4.1", true},
		{"gpt-4*", "gpt-3.5", false},
		{"deepseek-chat", "deepseek-chat", true},
		{"deepseek-chat", "deepseek-reasoner", false},
		{"*", "anything", true},
		{"", "anything", true},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.s, func(t *testing.T) {
			got := matchGlob(tt.pattern, tt.s)
			if got != tt.match {
				t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.s, got, tt.match)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests**

Run: `cd backend && go test -tags=unit -run "TestResolveModelPlatform|TestMatchGlob" ./internal/service/...`
Expected: PASS

- [ ] **Step 3: Write BundleSubscriptionService tests**

测试 `ActivateBundle` 的核心逻辑（使用 mock repo）：
- 测试正常激活流程
- 测试重复购买冲突
- 测试已下架套餐拒绝

- [ ] **Step 4: Run all bundle tests**

Run: `cd backend && go test -tags=unit -run "Bundle" ./internal/service/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/bundle_plan_service_test.go backend/internal/service/bundle_subscription_service_test.go backend/internal/service/bundle_route_resolver_test.go
git commit -m "test(bundle): add unit tests for bundle services"
```

---

## Phase 9: Frontend (Outline — follows existing patterns)

### Task 22: Frontend Types + API Clients

**Files:**
- Create: `frontend/src/types/bundle.ts`
- Create: `frontend/src/api/bundles.ts`
- Create: `frontend/src/api/admin/bundles.ts`

- [ ] **Step 1: Create TypeScript types**

从设计文档 Section 6.4 复制类型定义到 `types/bundle.ts`。

- [ ] **Step 2: Create user API client**

遵循 `frontend/src/api/subscriptions.ts` 的模式，实现 `getPlans`, `getPlanDetail`, `getMyBundle`, `getMyUsage`, `checkout`。

- [ ] **Step 3: Create admin API client**

遵循 `frontend/src/api/admin/subscriptions.ts` 的模式，实现管理员 CRUD。

- [ ] **Step 4: Commit**

```bash
git add frontend/src/types/bundle.ts frontend/src/api/bundles.ts frontend/src/api/admin/bundles.ts
git commit -m "feat(bundle): add frontend types and API clients"
```

---

### Task 23: Frontend Admin Pages

**Files:**
- Create: `frontend/src/views/admin/bundles/BundlePlansView.vue`
- Create: `frontend/src/views/admin/bundles/BundleSubscriptionsView.vue`

- [ ] **Step 1: Create BundlePlansView**

遵循 `SubscriptionsView.vue` 和 `GroupsView.vue` 的模式。包含：
- 套餐列表表格（分页）
- 创建/编辑弹窗（表单含 Group 额度配置）
- 状态切换、排序、删除操作

- [ ] **Step 2: Create BundleSubscriptionsView**

遵循现有订阅管理页面的模式。包含：
- 用户套餐订阅列表
- 撤销/延长操作
- 用量查看

- [ ] **Step 3: Add routes**

在 `frontend/src/router/index.ts` 中添加管理员路由。

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/admin/bundles/ frontend/src/router/index.ts
git commit -m "feat(bundle): add admin bundle management pages"
```

---

### Task 24: Frontend User Pages + Key Creation Modification

**Files:**
- Create: `frontend/src/views/user/BundlesView.vue`
- Create: `frontend/src/views/user/BundleUsageView.vue`
- Modify: `frontend/src/views/user/KeysView.vue`
- Modify: `frontend/src/views/user/PaymentView.vue`

- [ ] **Step 1: Create BundlesView**

套餐浏览卡片页面，展示各套餐价格、包含的模型、额度信息。

- [ ] **Step 2: Create BundleUsageView**

用量展示页面，按 Group/模型分组显示日/周/月用量进度条。

- [ ] **Step 3: Modify KeysView.vue**

在创建 API Key 的表单中：
- 检测用户是否有活跃的 `BundleSubscription`
- 如果有，展示 Key 模式选择：「通用 Key（自动路由）」或「专用 Key（绑定特定 Group）」
- 通用 Key：不设置 `group_id`，设置 `bundle_subscription_id`
- 专用 Key：从套餐包含的 Group 列表中选择

- [ ] **Step 4: Modify PaymentView.vue**

添加套餐购买入口，与现有余额充值/订阅购买并列展示。

- [ ] **Step 5: Add routes + commit**

```bash
git add frontend/src/views/user/ frontend/src/router/index.ts
git commit -m "feat(bundle): add user bundle pages and key creation dual mode"
```

---

### Task 25: i18n Translations

**Files:**
- Modify: `frontend/src/i18n/locales/zh.ts`

- [ ] **Step 1: Add bundle namespace translations**

在 `zh.ts` 中添加 `bundles` 命名空间，覆盖：
- 管理员：套餐表单字段、Group 额度配置、状态、层级名称
- 用户：套餐卡片文案、购买按钮、用量展示、Key 模式选择

- [ ] **Step 2: Verify no i18n key missing**

Run: `cd frontend && pnpm run typecheck`
Expected: 无类型错误

- [ ] **Step 3: Commit**

```bash
git add frontend/src/i18n/locales/zh.ts
git commit -m "feat(bundle): add Chinese i18n translations"
```

---

## Final Verification

### Task 26: End-to-End Build + Test

- [ ] **Step 1: Backend full build**

Run: `cd backend && go build -o /dev/null ./cmd/server`
Expected: 成功

- [ ] **Step 2: Backend all tests**

Run: `cd backend && go test -tags=unit ./...`
Expected: 全部通过

- [ ] **Step 3: Backend lint**

Run: `cd backend && golangci-lint run ./...`
Expected: 无新增错误

- [ ] **Step 4: Frontend build**

Run: `cd frontend && pnpm run build`
Expected: 成功

- [ ] **Step 5: Frontend lint + typecheck**

Run: `cd frontend && pnpm run lint:check && pnpm run typecheck`
Expected: 无错误

- [ ] **Step 6: Final commit with all changes**

```bash
git add -A
git commit -m "feat(bundle): complete bundle subscription feature implementation"
```
