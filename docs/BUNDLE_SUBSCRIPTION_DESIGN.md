# Bundle Subscription Design

**Date**: 2026-06-08
**Status**: Draft
**Author**: Claude Code (with user collaboration)

## 1. Overview

Sub2API 平台新增「套餐订阅」功能：将多个 AI 模型（通过现有 Group 体系）捆绑成套餐售卖，每个 Group 可独立配置日/周/月额度。支持多种套餐层级（入门/专业/企业），用户购买后获得套餐内所有模型的访问权。

### Core Requirements

- **混合额度粒度**：管理员可按平台或按具体模型设置额度
- **独立额度**：每个套餐独立设置各 Group 的日/周/月限额
- **复用现有 Group**：不改变 Group 的账户绑定、模型路由、调度能力
- **一个套餐 = 多个 Group**：用户购买一个套餐自动获得所有包含 Group 的访问权
- **独立超额**：某 Group 超额不影响其他 Group 的使用
- **套餐互斥**：同一时间只能有一个活跃套餐
- **管控范围**：额度 + 并发数 + RPM
- **计费模式**：固定价格包月，额度内无限使用
- **API Key 双模式**：支持单 Key 自动路由 + 多 Key 专用绑定

### Design Principle

**二次开发最小侵入**：>90% 为新增代码，对现有代码改动约 70 行。

---

## 2. Data Model

### 2.1 New Ent Schemas (4 new files, zero modifications to existing schemas)

#### BundlePlan

套餐商品定义。

| Field | Type | Description |
|---|---|---|
| `name` | string | 套餐名称（如"入门款"、"专业款"） |
| `description` | string | 套餐描述 |
| `tier` | string | 套餐层级: `starter` / `pro` / `enterprise` |
| `price` | float64 | 售价 |
| `original_price` | float64 | 原价（划线价） |
| `currency` | string | 货币: `USD` / `CNY` |
| `validity_days` | int | 有效天数 |
| `concurrency_limit` | int | 并发上限（0=不限） |
| `rpm_limit` | int | RPM 上限（0=不限） |
| `features` | []string | 功能特性列表（前端展示） |
| `for_sale` | bool | 是否在售 |
| `sort_order` | int | 排序 |
| `status` | string | `active` / `disabled` |

Edges:
- `group_quotas` → BundlePlanGroupQuota (1:N)
- `subscriptions` → BundleSubscription (1:N)

#### BundlePlanGroupQuota

套餐内每个 Group 的额度配置。

| Field | Type | Description |
|---|---|---|
| `plan_id` | int | → BundlePlan |
| `group_id` | int | → Group（复用现有 Group） |
| `quota_scope` | string | `platform`（按平台共享）/ `model`（按模型独立） |
| `model_pattern` | string | 仅 `model` 级别生效，glob 模式匹配（`*` 匹配任意字符），如 `"claude-opus-*"`, `"gpt-4*"`, `"deepseek-chat"` |
| `daily_limit_usd` | float64 | 日额度（0=不限） |
| `weekly_limit_usd` | float64 | 周额度（0=不限） |
| `monthly_limit_usd` | float64 | 月额度（0=不限） |

Edges:
- `plan` → BundlePlan
- `group` → Group

#### BundleSubscription

用户购买的套餐实例。

| Field | Type | Description |
|---|---|---|
| `user_id` | int | → User |
| `plan_id` | int | → BundlePlan |
| `status` | string | `active` / `expired` / `revoked` |
| `starts_at` | time | 生效时间 |
| `expires_at` | time | 到期时间 |
| `concurrency_limit` | int | 快照：创建时从 Plan 复制 |
| `rpm_limit` | int | 快照：创建时从 Plan 复制 |
| `source` | string | `purchase` / `redeem` / `admin_assign` |

Edges:
- `user` → User
- `plan` → BundlePlan
- `group_usages` → BundleSubscriptionUsage (1:N)
- `user_subscriptions` → UserSubscription (1:N, bridge)

#### BundleSubscriptionUsage

按 Group 维度的用量跟踪。

| Field | Type | Description |
|---|---|---|
| `bundle_subscription_id` | int | → BundleSubscription |
| `group_id` | int | → Group |
| `model_pattern` | string | 空=平台级，有值=模型级 |
| `daily_usage_usd` | float64 | 当日已用 |
| `daily_window_start` | time | 日窗口起点 |
| `weekly_usage_usd` | float64 | 当周已用 |
| `weekly_window_start` | time | 周窗口起点 |
| `monthly_usage_usd` | float64 | 当月已用 |
| `monthly_window_start` | time | 月窗口起点 |

Edges:
- `bundle_subscription` → BundleSubscription
- `group` → Group

### 2.2 Minimal Changes to Existing Schemas (2 files, ~20 lines total)

#### UserSubscription (add 4 fields)

| New Field | Type | Description |
|---|---|---|
| `bundle_subscription_id` | *int | 关联的套餐实例（nullable） |
| `daily_limit_usd` | float64 | 独立日限额（0=fallback 到 Group 配置） |
| `weekly_limit_usd` | float64 | 独立周限额 |
| `monthly_limit_usd` | float64 | 独立月限额 |

#### APIKey (add 1 field)

| New Field | Type | Description |
|---|---|---|
| `bundle_subscription_id` | *int | 关联的套餐实例（nullable，支持单 Key 模式） |

---

## 3. Core Workflows

### 3.1 Admin Configures Bundle Plan

1. Admin navigates to Bundle Plan management page
2. Creates BundlePlan with name, price, validity, tier, concurrency, RPM
3. Selects Groups from existing Group list and configures per-Group quotas:
   - Set `quota_scope` (platform or model level)
   - If model level, specify `model_pattern`
   - Set `daily_limit_usd`, `weekly_limit_usd`, `monthly_limit_usd`
4. Saves → creates `bundle_plan` + `bundle_plan_group_quotas` records

### 3.2 User Purchases Bundle

1. User browses available plans on purchase page
2. Selects plan → creates `PaymentOrder` with new `order_type = "bundle"`
3. Payment succeeds → **ActivateBundle**:
   - Create `BundleSubscription` record (snapshot plan's concurrency/rpm)
   - For each `BundlePlanGroupQuota`:
     - Create `BundleSubscriptionUsage` (usage tracking)
     - Create `UserSubscription` (bridge to existing system):
       - `group_id` = quota's group_id
       - `daily/weekly/monthly_limit_usd` = snapshot from quota
       - `bundle_subscription_id` = link to BundleSubscription
       - `status` = active, `expires_at` = same as bundle
4. User prompted to create API Key

### 3.3 API Key Creation (Dual Mode)

When user has an active `BundleSubscription`:

- **Universal Key mode**: API Key with no `group_id`, linked to `bundle_subscription_id`. System auto-routes based on requested model.
- **Dedicated Key mode**: API Key bound to a specific Group from the bundle. Existing routing flow, no gateway changes.

Both Key types share the same quota pool (tracked by `bundle_subscription_id` + `group_id`).

### 3.4 Single-Key Auto-Routing (New Gateway Middleware)

Inserted between auth middleware and `RequireGroupAssignment`:

```
if apiKey.bundle_subscription_id != nil && apiKey.group_id == nil:
    1. Load BundleSubscription (validate active + not expired)
    2. Extract model name from request body (query param or JSON body)
    3. Model → Platform mapping (via resolveModelPlatform):
       - "gpt-*" / "o1-*" / "o3-*" / "chatgpt-*" / "dall-*"  → openai
       - "claude-*"                                             → anthropic
       - "gemini-*"                                             → gemini
       - "deepseek-*"                                           → openai (compatible protocol)
       - unknown models                                         → openai (default)
    4. ResolveGroup (BundleRouteResolver):
       Phase 1: Try model-level glob match (quota_scope == "model")
       Phase 2: Fallback to platform-level match (quota_scope == "platform")
    5. Inject ResolvedGroup (groupID + platform + quota) into request context
       as "bundle_resolved_group_id"
    6. If no match → return 400 bundle_model_not_included
else:
    Pass through to existing logic (unchanged)
```

### 3.5 Quota Check Integration

**Existing `checkSubscriptionEligibility()` — limit fallback priority:**

```
For each time window (daily/weekly/monthly):
  dailyLimit := group.DailyLimitUSD
  if subData.DailyLimit > 0:
      dailyLimit = &subData.DailyLimit    // preference: subscription's own limit
  // ... same for weekly/monthly

  Check: subData.DailyUsage >= dailyLimit  → ErrDailyLimitExceeded
```

The `SubData` struct (used by the eligibility check) carries `DailyLimit` / `WeeklyLimit` / `MonthlyLimit` fields populated from the `UserSubscription` model. When a subscription is bridged from a bundle, these fields hold the snapshotted quota from `BundlePlanGroupQuota` at activation time. Non-bundle subscriptions leave these at zero, falling back to the Group's own limits.

This single change lets bundle subscriptions and regular subscriptions share the same check function.

**Concurrency and RPM check:**

Bundle-level concurrency and RPM limits are snapshotted into the `BundleSubscription` record at activation time. The gateway layer checks these values against current usage counts per-request, using the `BundleSubscription` record (or its cached version) rather than the live plan configuration.

### 3.6 Usage Accumulation (postUsageBilling extension)

In `gateway_service.go`, after subscription cost deduction:

```
if bundleUsageService != nil && subscription.BundleSubscriptionID != nil:
    → bundleUsageService.AccumulateUsage(ctx, bundleSubID, groupID, cost)
    → IncrementUsage() performs atomic DB ADD on daily/weekly/monthly usage columns
```

Usage accumulation uses database-level `ADD` operations (via Ent ORM) for atomicity, rather than Redis INCRBY.

### 3.7 Bundle Expiry

- `BundleExpiryService` (or extension of existing `SubscriptionExpiryService`):
  - Mark `BundleSubscription` as expired
  - Mark all linked `UserSubscription` records as expired
  - Next request with bundle API Key → rejected with `bundle_expired`

---

## 4. Error Handling

### Error Codes

| Code | HTTP | Scenario | User Message |
|---|---|---|---|
| `bundle_expired` | 403 | Bundle expired | "您的套餐已过期，请续费或购买新套餐" |
| `bundle_group_quota_exceeded` | 429 | Group daily/weekly/monthly quota exceeded | "您在 {platform} 的{日/周/月}额度已用完，额度将在 {reset_time} 重置" |
| `bundle_model_not_included` | 400 | Requested model not in bundle | "模型 {model} 不在您的套餐范围内，请升级套餐" |
| `bundle_concurrency_exceeded` | 429 | Concurrency limit reached | "当前并发请求数已达套餐上限 {limit}" |
| `bundle_rpm_exceeded` | 429 | RPM limit reached | "请求频率已达套餐上限" |
| `bundle_conflict` | 409 | User already has active bundle | "您已有活跃套餐，请先等待到期或联系管理员" |
| `bundle_plan_disabled` | 400 | Plan is no longer for sale | "该套餐已下架" |

### Edge Cases

1. **Duplicate purchase**: Check active BundleSubscription before allowing purchase. Admin assignments bypass this restriction (revoke old → activate new).

2. **Model name resolution failure**: Fallback to iterating all bundle Groups and matching `model_pattern`. If no match → `bundle_model_not_included`.

3. **Cross-platform requests with single Key**: Each request independently resolves to the correct Group. Usage tracked separately per Group.

4. **Expiry during active streaming**: Already-in-progress requests complete normally. New requests rejected.

5. **Single Group quota exceeded**: Only that Group's requests are rejected (429). Other Groups continue normally.

6. **Universal + Dedicated Keys coexist**: Both Key types share the same quota pool. Usage accumulates under the same `bundle_subscription_id` + `group_id` dimension.

---

## 5. Backend Architecture

### 5.1 New Files

```
backend/
├── ent/schema/
│   ├── bundle_plan.go
│   ├── bundle_plan_group_quota.go
│   ├── bundle_subscription.go
│   └── bundle_subscription_usage.go
├── internal/
│   ├── handler/
│   │   ├── bundle_handler.go
│   │   └── bundle_admin_handler.go
│   ├── service/
│   │   ├── bundle_plan_service.go
│   │   ├── bundle_subscription_service.go
│   │   ├── bundle_usage_service.go
│   │   └── bundle_route_resolver.go
│   ├── repository/
│   │   ├── bundle_plan_repo.go
│   │   ├── bundle_subscription_repo.go
│   │   └── bundle_usage_repo.go
│   └── server/
│       ├── middleware/
│       │   └── bundle_resolver.go
│       └── routes/
│           └── bundle.go
```

### 5.2 Repository Layer

| Repo | Core Methods |
|---|---|
| `BundlePlanRepo` | `Create(plan)`, `Update(plan)`, `GetByID`, `List`, `ListForSale`, `Delete` |
| `BundleSubscriptionRepo` | `Create(sub)`, `GetByID`, `GetActiveByUserID`, `GetByIDWithUsages`, `List`, `UpdateStatus`, `UpdateExpiry` |
| `BundleUsageRepo` | `GetBySubscriptionAndGroup`, `Create`, `IncrementUsage`, `ResetDailyWindow`, `ResetWeeklyWindow`, `ResetMonthlyWindow`, `ListBySubscription`, `BatchUpdateExpiredStatus` |

Note: `Create` methods accept the service model directly (group quotas embedded in `BundlePlan`, usage records embedded in `BundleSubscription`) and populate the `ID` field back on the passed pointer.

### 5.3 Service Layer

| Service | Core Methods | Description |
|---|---|---|
| `BundlePlanService` | `CreatePlan`, `UpdatePlan`, `GetPlanDetail`, `ListPlans`, `ListForSale` | Admin CRUD + user browsing |
| `BundleSubscriptionService` | `ActivateBundle`, `RevokeBundle`, `CheckExpiry`, `GetUserActiveBundle` | Purchase activation, status management |
| `BundleUsageService` | `AccumulateUsage`, `GetUsageProgress`, `CheckQuotaEligibility` | Usage tracking + progress query |
| `BundleRouteResolver` | `ResolveGroup(modelName, bundleSubID)` | Single-key model → Group mapping |

### 5.4 Handler Layer

**Admin APIs (require admin role):**

| Method | Path | Description |
|---|---|---|
| `POST` | `/admin/bundle/plans` | Create bundle plan |
| `PUT` | `/admin/bundle/plans/:id` | Update bundle plan |
| `GET` | `/admin/bundle/plans` | List bundle plans |
| `GET` | `/admin/bundle/plans/:id` | Get bundle plan detail |
| `DELETE` | `/admin/bundle/plans/:id` | Disable bundle plan |
| `GET` | `/admin/bundle/subscriptions` | List all user bundle subscriptions |
| `POST` | `/admin/bundle/subscriptions/:id/revoke` | Revoke user bundle |
| `POST` | `/admin/bundle/subscriptions/:id/extend` | Extend user bundle |

**User APIs:**

| Method | Path | Description |
|---|---|---|
| `GET` | `/bundles/plans` | List purchasable plans |
| `GET` | `/bundles/plans/:id` | Plan detail |
| `GET` | `/bundles/subscription` | My active bundle |
| `GET` | `/bundles/subscription/usage` | Bundle usage progress (by Group) |
| `POST` | `/bundles/checkout` | Create bundle order (reuses PaymentOrder) |

### 5.5 Existing Code Changes Summary

| File | Change | Lines |
|---|---|---|
| `ent/schema/user_subscription.go` | Add 4 fields + index | ~20 |
| `ent/schema/api_key.go` | Add 1 field | ~5 |
| `internal/handler/handler.go` | Add Bundle fields to Handlers/AdminHandlers | ~5 |
| `internal/handler/wire.go` | Add Bundle handler providers | ~5 |
| `internal/service/wire.go` | Add Bundle service providers + ProvideBundleExpiryService | ~10 |
| `internal/service/billing_service.go` | Add 6 BillingCache bundle methods | ~10 |
| `internal/service/billing_cache_service.go` | Limit source fallback priority | ~10 |
| `internal/service/gateway_service.go` | `postUsageBilling` add bundle usage branch; inject BundleUsageService | ~20 |
| `internal/service/user_subscription_port.go` | Add `ExtendExpiry`, `ExpireBridgedSubscriptionsForExpiredBundles` | ~5 |
| `internal/repository/wire.go` | Add 3 Bundle repository providers | ~5 |
| `internal/server/middleware/middleware.go` | `RequireGroupAssignment` add bundle branch | ~5 |
| `internal/server/middleware/wire.go` | Add BundleRouteResolverMiddleware provider | ~3 |
| `internal/server/routes/gateway.go` | Register bundle_resolver middleware | ~5 |
| `internal/server/routes/admin.go` | Register admin bundle routes | ~20 |
| **Total** | | **~130 lines** |

---

## 6. Frontend Changes

### 6.1 New Pages

| Page File | Route | Description |
|---|---|---|
| `views/admin/bundles/BundlePlansView.vue` | `/admin/bundle/plans` | Admin plan list + create/edit dialog |
| `views/admin/bundles/BundleSubscriptionsView.vue` | `/admin/bundle/subscriptions` | Admin user subscription management |
| `views/user/BundlesView.vue` | `/bundles` | User browsable plan cards |
| `views/user/BundleUsageView.vue` | `/bundles/usage` | User usage display by Group/model |

### 6.2 Modified Pages

| File | Change |
|---|---|
| `views/user/KeysView.vue` | Detect active bundle → show Universal Key option |
| `views/user/PaymentView.vue` | Add bundle purchase entry |
| `views/user/SubscriptionsView.vue` | Show bundle subscriptions (differentiated from regular) |

### 6.3 New API Clients

```
frontend/src/api/
├── admin/bundles.ts    # Admin bundle CRUD
├── bundles.ts          # User bundle APIs
```

### 6.4 New Type Definitions

```typescript
// types/bundle.ts
interface BundlePlan {
  id: number
  name: string
  description: string
  tier: 'starter' | 'pro' | 'enterprise'
  price: number
  original_price: number
  currency: string
  validity_days: number
  concurrency_limit: number
  rpm_limit: number
  features: string[]
  for_sale: boolean
  sort_order: number
  status: 'active' | 'disabled'
  group_quotas: BundlePlanGroupQuota[]
}

interface BundlePlanGroupQuota {
  id: number
  plan_id: number
  group_id: number
  group?: Group
  quota_scope: 'platform' | 'model'
  model_pattern: string
  daily_limit_usd: number
  weekly_limit_usd: number
  monthly_limit_usd: number
}

interface BundleSubscription {
  id: number
  user_id: number
  plan_id: number
  plan?: BundlePlan
  status: 'active' | 'expired' | 'revoked'
  starts_at: string
  expires_at: string
  concurrency_limit: number
  rpm_limit: number
  source: 'purchase' | 'redeem' | 'admin_assign'
  group_usages: BundleSubscriptionUsage[]
}

interface BundleSubscriptionUsage {
  id: number
  bundle_subscription_id: number
  group_id: number
  group?: Group
  model_pattern: string
  daily_usage_usd: number
  weekly_usage_usd: number
  monthly_usage_usd: number
  daily_limit_usd: number
  weekly_limit_usd: number
  monthly_limit_usd: number
}
```

### 6.5 i18n

Add `bundles.*` namespace to `zh.ts` covering:
- Admin: plan form, Group quota config, subscription management
- User: plan cards, purchase flow, usage display, Key creation options

---

## 7. Caching Strategy

### Redis Cache Keys

Cache key prefixes and TTLs are defined in `bundle_constants.go`:

| Prefix | Data | TTL | Purpose |
|---|---|---|---|
| `bundle:plan:` | BundlePlan by ID | 5 min | Plan detail cache |
| `bundle:sub:` | BundleSubscription by user ID | 3 min | Fast status check on request |
| `bundle:usage:` | BundleSubscriptionUsage by sub+group | 1 min | Quota check without DB hit |
| `bundle:user:` | User bundles list | — | Per-user bundle lookup |

The `BillingCache` interface (in `billing_service.go`) exposes 6 bundle-specific methods:

- `GetBundleSubscriptionCache` / `SetBundleSubscriptionCache` / `InvalidateBundleSubscriptionCache` — per-user active bundle snapshot
- `GetBundlePlansForSaleCache` / `SetBundlePlansForSaleCache` / `InvalidateBundlePlansForSaleCache` — purchasable plans list

### Update Strategy

- **High-frequency writes** (usage accumulation): Database `ADD` (atomic increment) via `IncrementUsage` on Ent ORM
- **Low-frequency writes** (activation/revocation): Write DB → invalidate cache (cache-aside)

---

## 8. Testing Strategy

### Backend

| Type | Coverage | Tag |
|---|---|---|
| Unit | BundlePlanService, BundleSubscriptionService, BundleUsageService, BundleRouteResolver | `unit` |
| Unit | Limit fallback priority (subscription.limit > group.limit) | `unit` |
| Unit | Model → platform → Group mapping cases | `unit` |
| Integration | Purchase → create UserSubscriptions → gateway quota check E2E | `integration` |
| Integration | Single-key multi-platform routing + independent quota | `integration` |
| Integration | Bundle expiry → request rejection + other bundles unaffected | `integration` |

### Frontend

| Type | Coverage |
|---|---|
| Vitest | Bundle API client, usage calculation utils, type definitions |
| Component test | Plan cards, usage progress bars, Key creation mode selector |

---

## 9. Implementation Phases

### Phase 1: Foundation (Data Model + Core Service)

- Ent schemas (4 new + 2 modified)
- Repository layer (3 new repos)
- `go generate ./ent` + regenerate Wire
- Core service layer (plan CRUD, subscription activation)
- Unit tests for services

### Phase 2: Gateway Integration

- `bundle_resolver.go` middleware (single-key auto-routing)
- `RequireGroupAssignment` bundle branch
- `checkSubscriptionEligibility` limit fallback
- `postUsageBilling` bundle usage accumulation
- `BundleRouteResolver` service
- Integration tests

### Phase 3: Payment Integration

- Payment order flow with `order_type = "bundle"`
- Activation logic (bundle purchase → auto-create UserSubscriptions)
- BundleExpiryService
- Admin management APIs

### Phase 4: Frontend

- Admin: plan management page + subscription management page
- User: plan browsing, purchase flow, usage display
- Key creation dual-mode UI
- i18n translations

---

## 10. Summary of Changes by Impact

| Category | New | Modified | Total |
|---|---|---|---|
| Ent Schemas | 4 files | 2 files (~20 lines) | 6 |
| Backend Services | 4 files | 2 files (~25 lines) | 6 |
| Backend Handlers | 2 files | 0 | 2 |
| Backend Repos | 3 files | 0 | 3 |
| Backend Middleware/Routes | 2 files | 2 files (~8 lines) | 4 |
| Wire Config | 0 | 1 file (~20 lines) | 1 |
| Frontend Pages | 4 files | 3 files | 7 |
| Frontend API/Types | 3 files | 0 | 3 |
| i18n | 0 | 1 file | 1 |
| **Total** | **22 new files** | **~70 lines modified** | **33** |
