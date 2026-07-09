# Bundle Upgrade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让用户在当前套餐未到期时自助升级到更高价值套餐，旧套餐剩余价值按比例折算（prorate credit）抵扣新套餐价格，用户只补差价。

**Architecture:** 升级 = 「带退款抵扣的二次购买」，复用现有「支付订单 + payment_fulfillment 履约」两段式。下单时锁定 credit 到订单；支付成功回调里 `doBundleUpgrade` 在一个事务内「标记旧订阅 upgraded + 激活新订阅」。支付失败旧订阅零影响。

**Tech Stack:** Go 1.26.4 / Ent ORM / Wire DI / golangci-lint v2.9（depguard 强制分层）/ PostgreSQL（手写迁移）/ Vue 3 + TS + Pinia + pnpm

**Spec:** `docs/BUNDLE_UPGRADE_DESIGN.md`

## Global Constraints

- **分层**：handler 不得 import repository/gorm/redis；service 不得 import repository/gorm/redis（depguard 强制，CI 拦截）。credit 计算要查 `payment_orders`，必须经 service 层接口，不进 handler。
- **Ent schema 改动**：改 `ent/schema/*.go` 后必须 `cd backend && go generate ./ent` 并提交生成代码；生产建表靠 `backend/migrations/*.sql`（不是 ent auto-migrate），必须同步手写迁移。
- **迁移规范**：幂等（`ADD COLUMN IF NOT EXISTS` / `CREATE INDEX IF NOT EXISTS`），文件头加 `SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='10min';`，金额精度 `NUMERIC(20,12)`。
- **interface 变更**：给 Go interface 新增方法后，所有测试 stub/mock 必须补全，否则编译失败。搜索 `type.*Noop.*struct` 和 `type.*Stub.*struct`。
- **API 响应**：列表空结果返回 `[]`（`make([]T,0)`），by-id 不存在返回 404。nil slice 序列化为 `null`，禁止。
- **前端**：必须 pnpm（非 npm）；`pnpm.overrides` 两条安全补丁（js-cookie/form-data）禁止删除。
- **版本**：Go 1.26.4；金额计算用 float64（项目现状），落库 NUMERIC(20,12)。

## File Structure

**新建：**
- `backend/migrations/161_bundle_upgrade.sql` — schema 迁移
- `backend/internal/service/bundle_upgrade_service.go` — 升级核心：`computeProrateCredit` + `PreviewUpgrade` + `UpgradeBundle`（独立文件，单一职责）
- `backend/internal/service/bundle_upgrade_service_test.go` — 升级单测

**修改（后端）：**
- `backend/ent/schema/bundle_subscription.go` — 加 `upgraded_from_id` 字段
- `backend/ent/schema/payment_order.go` — 加 `source_bundle_subscription_id` + `prorate_credit`
- `backend/internal/payment/types.go:44` — 加 `OrderTypeBundleUpgrade`
- `backend/internal/service/bundle_constants.go` — 加 `BundleStatusUpgraded` + `BundleSourceUpgrade`
- `backend/internal/service/bundle_models.go` — `BundleSubscription` 加 `UpgradedFromID`；新增 `UpgradePreview` + `UpgradeBundleRequest` + `PaymentOrderReader` 接口
- `backend/internal/service/bundle_subscription_port.go` — 无需改（升级复用现有 repo 方法）
- `backend/internal/service/payment_service.go:73` — `CreateOrderRequest` 加 `SourceBundleSubscriptionID int64` + `ProrateCredit float64`
- `backend/internal/service/payment_order.go` — `validateOrderInput` 加 `bundle_upgrade` 分支；`ensureNoDuplicateBundleOrder` 覆盖升级
- `backend/internal/service/payment_fulfillment.go:212` — `executeFulfillment` 加 `bundle_upgrade` 分支；新增 `ExecuteBundleUpgradeFulfillment` + `doBundleUpgrade` + `refundUpgradeToBalance`
- `backend/internal/service/payment_service.go` — `PaymentService` 加 `bundleSubscriptionSvc`（若无）；wire 注入 `PaymentOrderReader`
- `backend/internal/handler/bundle_handler.go` — 加 `PreviewUpgrade` + `Upgrade` handler + DTO
- `backend/internal/server/routes/bundle.go` — 注册 `GET /bundles/upgrade/preview` + `POST /bundles/upgrade`
- `backend/cmd/server/wire.go` — 新增 `PaymentOrderReader` provider（若新增接口）

**修改（前端）：**
- `frontend/src/api/bundles.ts` — 加 `previewBundleUpgrade` + `createBundleUpgradeOrder`
- `frontend/src/views/user/BundlesView.vue` — 升级按钮 + 差价确认窗
- `frontend/src/i18n/locales/{zh,en}.json`（或对应 locale 文件）— 升级文案

**ent 生成（go generate 自动，须提交）：**
- `ent/bundlesubscription_create.go` / `ent/paymentorder_create.go` 等的 `Set*` 方法

---

## Task 1: 常量 + Ent Schema + 迁移（地基）

**Files:**
- Modify: `backend/internal/payment/types.go:44`
- Modify: `backend/internal/service/bundle_constants.go`
- Modify: `backend/ent/schema/bundle_subscription.go`
- Modify: `backend/ent/schema/payment_order.go`
- Create: `backend/migrations/161_bundle_upgrade.sql`

**Interfaces:**
- Produces: `OrderTypeBundleUpgrade`、`BundleStatusUpgraded`、`BundleSourceUpgrade` 常量；ent 生成的 `SetSourceBundleSubscriptionID`/`SetProrateCredit`/`SetUpgradedFromID` 方法（后续 task 依赖）

- [ ] **Step 1: 加 OrderType 常量**

在 `backend/internal/payment/types.go` 的 `OrderTypeBundle` 行（:44）后加：
```go
OrderTypeBundle       = "bundle"
OrderTypeBundleUpgrade = "bundle_upgrade" // 套餐升级（差价订单）
```

- [ ] **Step 2: 加 status/source 常量**

在 `backend/internal/service/bundle_constants.go` 的 BundleStatus 组加：
```go
BundleStatusActive   = "active"
BundleStatusExpired  = "expired"
BundleStatusRevoked  = "revoked"
BundleStatusUpgraded = "upgraded" // 旧订阅被升级替换
```
BundleSource 组加：
```go
BundleSourcePurchase    = "purchase"
BundleSourceRedeem      = "redeem"
BundleSourceAdminAssign = "admin_assign"
BundleSourceUpgrade     = "upgrade" // 升级产生的新订阅
```

- [ ] **Step 3: 改 bundle_subscription schema**

在 `backend/ent/schema/bundle_subscription.go` 的 `Fields()` 里 `field.String("source")...` 之后加：
```go
field.Int64("upgraded_from_id").Optional().Comment("升级来源：指向被替换的旧订阅ID，仅升级产生的新订阅有值"),
```

- [ ] **Step 4: 改 payment_order schema**

在 `backend/ent/schema/payment_order.go` 的 `field.Int64("bundle_subscription_id")...` 之后加：
```go
field.Int64("source_bundle_subscription_id").Optional().Comment("升级订单：被升级的旧订阅ID"),
```
在 `field.Float("refund_amount")...` 之后加：
```go
field.Float("prorate_credit").SchemaType(map[string]string{dialect.Postgres: "numeric(20,12)"}).Default(0).Comment("升级订单锁定的旧套餐剩余价值(credit)"),
```
（确认文件顶部已 import `dialect`，若无则加 `"entgo.io/ent/dialect"`）

- [ ] **Step 5: 重新生成 ent**

Run: `cd backend && go generate ./ent`
Expected: 无错误，`ent/bundlesubscription` 和 `ent/paymentorder` 包生成新字段的 query/set 方法。

- [ ] **Step 6: 写迁移 SQL**

创建 `backend/migrations/161_bundle_upgrade.sql`：
```sql
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE bundle_subscriptions ADD COLUMN IF NOT EXISTS upgraded_from_id BIGINT NULL;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS source_bundle_subscription_id BIGINT NULL;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS prorate_credit NUMERIC(20,12) NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_bundle_subs_upgraded_from ON bundle_subscriptions(upgraded_from_id);
CREATE INDEX IF NOT EXISTS idx_payment_orders_src_bundle ON payment_orders(source_bundle_subscription_id);
```

- [ ] **Step 7: 验证后端编译 + 迁移可跑**

Run: `cd backend && go build ./...`
Expected: 编译通过。

迁移验证（按 CLAUDE.md 规范，起一次性 PG 容器从零跑全部迁移）：
```bash
docker run --rm -d --name pg-bundle-up -e POSTGRES_PASSWORD=x -e POSTGRES_DB=sub2api postgres:18-alpine
# 预建 schema_migrations 表后按序跑 migrations/*.sql（每个仅一次）
docker stop pg-bundle-up
```
Expected: 161 迁移成功应用，`\d bundle_subscriptions` 含 `upgraded_from_id`，`\d payment_orders` 含两个新列。

- [ ] **Step 8: Commit**

```bash
git add backend/ent/ backend/ent/schema/ backend/migrations/161_bundle_upgrade.sql \
        backend/internal/payment/types.go backend/internal/service/bundle_constants.go
git commit -m "feat(bundle): 升级功能 schema 与常量地基

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 2: computeProrateCredit 纯函数 + 单测

**Files:**
- Create: `backend/internal/service/bundle_upgrade_service.go`
- Test: `backend/internal/service/bundle_upgrade_service_test.go`

**Interfaces:**
- Produces: `func computeProrateCredit(paidAmount float64, startsAt, expiresAt, now time.Time) float64`（纯函数，后续 PreviewUpgrade 与 doBundleUpgrade 共用）

- [ ] **Step 1: 写失败测试**

创建 `bundle_upgrade_service_test.go`（`//go:build unit`）：
```go
//go:build unit

package service

import (
	"math"
	"testing"
	"time"
)

func TestComputeProrateCredit(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	expires := start.AddDate(0, 0, 30) // 30 天套餐
	paid := 100.0

	cases := []struct {
		name    string
		now     time.Time
		wantLow float64 // 下界（舍入波动）
		wantHi  float64 // 上界
	}{
		{"用了2天剩28天", start.AddDate(0, 0, 2), 93.0, 93.5},
		{"刚买1分钟", start.Add(time.Minute), 99.0, 100.0},
		{"剩1天", start.AddDate(0, 0, 29), 0.5, 4.0},
		{"已过期剩0", expires.Add(time.Hour), 0, 0},
		{"已过期(now==expires)", expires, 0, 0},
		{"无实付(兑换套餐)", 0, 0, 0}, // paidAmount=0 特例
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			amt := paid
			if c.name == "无实付(兑换套餐)" {
				amt = 0
			}
			got := computeProrateCredit(amt, start, expires, c.now)
			if math.IsNaN(got) || got < c.wantLow-0.01 || got > c.wantHi+0.01 {
				t.Errorf("credit=%.4f 不在 [%.2f, %.2f]", got, c.wantLow, c.wantHi)
			}
			if c.now.Equal(expires) || c.now.After(expires) {
				if got != 0 {
					t.Errorf("过期应得 0，得 %.4f", got)
				}
			}
		})
	}
}

func TestComputeProrateCredit_ZeroTotal(t *testing.T) {
	// startsAt==expiresAt 退化保护：总秒数为 0 不能除零
	zero := time.Now()
	if got := computeProrateCredit(100, zero, zero, zero); got != 0 {
		t.Errorf("总秒数为0应返回0，得 %.4f", got)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test -tags=unit -run TestComputeProrateCredit ./internal/service/`
Expected: FAIL（`computeProrateCredit` 未定义）

- [ ] **Step 3: 实现**

创建 `bundle_upgrade_service.go`：
```go
package service

import (
	"math"
	"time"
)

// computeProrateCredit 按剩余有效期线性折算旧套餐剩余价值。
// credit = paidAmount × max(0, 剩余秒) / 总秒。过期或总额为0返回0。
// 不追溯已消费的请求额度——套餐卖的是"有效期内使用权"而非预付token。
func computeProrateCredit(paidAmount float64, startsAt, expiresAt, now time.Time) float64 {
	if paidAmount <= 0 {
		return 0
	}
	totalSec := expiresAt.Sub(startsAt).Seconds()
	if totalSec <= 0 {
		return 0
	}
	remainSec := expiresAt.Sub(now).Seconds()
	if remainSec <= 0 {
		return 0
	}
	credit := paidAmount * remainSec / totalSec
	if credit < 0 || math.IsNaN(credit) {
		return 0
	}
	// 货币精度：2 位小数
	return math.Round(credit*100) / 100
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test -tags=unit -run TestComputeProrateCredit ./internal/service/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/bundle_upgrade_service.go backend/internal/service/bundle_upgrade_service_test.go
git commit -m "feat(bundle): computeProrateCredit 纯函数

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 3: 模型字段 + PaymentOrderReader 依赖 + wire

**Files:**
- Modify: `backend/internal/service/bundle_models.go`
- Modify: `backend/internal/repository/`（PaymentOrderReader 实现，定位见步骤）
- Modify: `backend/internal/service/bundle_subscription_service.go`（struct 加字段 + 构造函数加参数）
- Modify: `backend/cmd/server/wire.go`（若需新 provider）

**Interfaces:**
- Produces: `BundleSubscription.UpgradedFromID` 字段；`PaymentOrderReader` 接口（`GetPaidAmountByBundleSub(ctx, subID) (float64, error)`）注入 `BundleSubscriptionService`
- Consumes: Task 1 的 ent 生成方法

- [ ] **Step 1: bundle_models.go 加字段与 DTO**

`BundleSubscription` struct（:55）的 `Source string` 后加：
```go
Source           string    `json:"source"`
UpgradedFromID   int64     `json:"upgraded_from_id,omitempty"`
```
文件末尾加：
```go
// PaymentOrderReader 读支付订单（升级 credit 反查实付金额用），解耦 service 对 payment_orders 的访问。
type PaymentOrderReader interface {
	// GetPaidAmountByBundleSub 返回该 bundle 订阅对应的已完成购买订单实付金额。
	// 找不到订单（兑换/赠送来源）返回 0,nil。
	GetPaidAmountByBundleSub(ctx context.Context, bundleSubID int64) (float64, error)
}

// UpgradeUpgradePreview 升级预览（给前端展示差价）
type UpgradePreview struct {
	Credit       float64 `json:"credit"`
	TargetPrice  float64 `json:"target_price"`
	DueAmount    float64 `json:"due_amount"`
	ValidityDays int     `json:"validity_days"`
	Upgradeable  bool    `json:"upgradeable"`
	OldPlanName  string  `json:"old_plan_name"`
	NewPlanName  string  `json:"new_plan_name"`
}

// UpgradeBundleRequest 升级切换请求（履约时调用）
type UpgradeBundleRequest struct {
	UserID       int64
	SourceSubID  int64
	TargetPlanID int64
}
```
（顶部加 `"context"` import）

- [ ] **Step 2: 实现 PaymentOrderReader**

在 `backend/internal/repository/` 找 payment order 仓储实现（搜索 `PaymentOrder` 的 ent 查询文件）。新增方法：
```go
// GetPaidAmountByBundleSub 取该订阅对应已完成 bundle 购买订单的 amount（取最早一笔）。
func (r *PaymentOrderRepository) GetPaidAmountByBundleSub(ctx context.Context, bundleSubID int64) (float64, error) {
	o, err := r.client.PaymentOrder.Query().
		Where(paymentorder.BundleSubscriptionIDEQ(bundleSubID),
			paymentorder.OrderTypeIn(payment.OrderTypeBundle),
			paymentorder.StatusEQ(/* OrderStatusCompleted 常量，见 payment_order.go */)).
		Order(ent.Asc(paymentorder.FieldCreatedAt)).
		First(ctx)
	if ent.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return o.Amount, nil
}
```
（import `paymentorder`、`ent`、`payment`；OrderStatusCompleted 常量名按 `payment_order.go` 实际引用）

- [ ] **Step 3: BundleSubscriptionService 加依赖**

`bundle_subscription_service.go` 的 `BundleSubscriptionService` struct 加字段 `paidAmountReader PaymentOrderReader`；构造函数 `NewBundleSubscriptionService`（:32）加该参数并赋值。

- [ ] **Step 4: wire 注入**

`backend/cmd/server/wire.go`：确认 `PaymentOrderRepository` 已是 provider；若 `NewBundleSubscriptionService` 签名变了，`go generate ./cmd/server` 重生成 `wire_gen.go`。
Run: `cd backend && go generate ./cmd/server`
Expected: wire_gen.go 更新，编译通过。

- [ ] **Step 5: repo 映射 ent→model 补字段**

在 `internal/repository/` 的 bundle subscription 仓储里，所有 ent `BundleSubscription` → service `BundleSubscription` 的映射函数（Create 入参与 Get 出参）补 `UpgradedFromID` 字段读写。搜索 `bundleSubscriptionFromEnt` 或类似映射函数。

- [ ] **Step 6: 编译 + 跑现有测试确认无回归**

Run: `cd backend && go build ./... && go test -tags=unit ./internal/service/`
Expected: 编译通过，现有测试不回归。

- [ ] **Step 7: Commit**

```bash
git add backend/internal/service/bundle_models.go backend/internal/repository/ \
        backend/internal/service/bundle_subscription_service.go backend/cmd/server/wire_gen.go
git commit -m "feat(bundle): UpgradedFromID 字段与 PaymentOrderReader 依赖

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 4: PreviewUpgrade + 单测

**Files:**
- Modify: `backend/internal/service/bundle_upgrade_service.go`
- Test: `backend/internal/service/bundle_upgrade_service_test.go`

**Interfaces:**
- Consumes: `computeProrateCredit`（Task 2）、`PaymentOrderReader`（Task 3）、`BundleSubscriptionService` 的 planRepo/bundleSubRepo
- Produces: `func (s *BundleSubscriptionService) PreviewUpgrade(ctx, userID, sourceSubID, targetPlanID) (*UpgradePreview, error)`

- [ ] **Step 1: 写失败测试**

在 `bundle_upgrade_service_test.go` 加（复用 `bundleSubRepoNoop` 基类造 stub）：
```go
type previewPaidReader struct{ amt float64; err error }
func (r previewPaidReader) GetPaidAmountByBundleSub(context.Context, int64) (float64, error) {
	return r.amt, r.err
}

func TestPreviewUpgrade_NotUpgradeableWhenDueLeqZero(t *testing.T) {
	now := time.Now()
	oldSub := &BundleSubscription{
		ID: 1, UserID: 10, PlanID: 5, Status: BundleStatusActive,
		StartsAt: now.AddDate(0, 0, -2), ExpiresAt: now.AddDate(0, 0, 28),
		Plan: &BundlePlan{ID: 5, Name: "starter", Price: 100},
	}
	// 便宜的目标套餐：差价 <= 0
	targetPlan := &BundlePlan{ID: 6, Name: "pro", Price: 30, ValidityDays: 30, Status: BundlePlanStatusActive, ForSale: true}

	svc := NewBundleSubscriptionService(
		&previewSubRepo{sub: oldSub},          // GetByID 返回 oldSub
		&previewPlanRepo{plan: targetPlan},    // GetByID 返回 targetPlan
		bundleUsageRepoNoop{}, nil,
		nil, nil, previewPaidReader{amt: 100},
	)
	// 注：构造函数实际参数顺序按 NewBundleSubscriptionService 签名对齐

	pv, err := svc.PreviewUpgrade(context.Background(), 10, 1, 6)
	require.NoError(t, err)
	require.False(t, pv.Upgradeable, "差价<=0 应 not upgradeable")
}

func TestPreviewUpgrade_OwnerMismatch(t *testing.T) {
	// sourceSub 属用户 10，请求用户 20 → 应报错（IDOR）
	// ... 构造 svc，PreviewUpgrade(ctx, 20, 1, 6) 期望 err != nil
}
```
（stub `previewSubRepo`/`previewPlanRepo` 仿照 `activateBundleSubRepoStub` 写法，仅实现 `GetByID`/`GetByIDWithUsages`，嵌入对应 noop 基类。`planRepo` 的 noop 基类在 `bundle_plan_service_test.go`，搜索复用。）

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test -tags=unit -run TestPreviewUpgrade ./internal/service/`
Expected: FAIL（`PreviewUpgrade` 未定义）

- [ ] **Step 3: 实现 PreviewUpgrade**

`bundle_upgrade_service.go` 加：
```go
func (s *BundleSubscriptionService) PreviewUpgrade(ctx context.Context, userID, sourceSubID, targetPlanID int64) (*UpgradePreview, error) {
	// 1. 加载旧订阅，校验归属 + active（IDOR 防护）
	old, err := s.bundleSubRepo.GetByID(ctx, sourceSubID)
	if err != nil {
		return nil, ErrBundleNotFound
	}
	if old.UserID != userID {
		return nil, ErrBundleNotFound // 不泄露存在性
	}
	if old.Status != BundleStatusActive {
		return nil, ErrBundleExpired
	}

	// 2. 加载目标套餐，校验在售
	plan, err := s.planRepo.GetByID(ctx, targetPlanID)
	if err != nil {
		return nil, ErrBundlePlanNotFound
	}
	if !plan.ForSale || plan.Status != BundlePlanStatusActive {
		return nil, ErrBundlePlanDisabled
	}

	// 3. 反查旧订阅实付，算 credit
	paid, err := s.paidAmountReader.GetPaidAmountByBundleSub(ctx, sourceSubID)
	if err != nil {
		return nil, fmt.Errorf("lookup paid amount: %w", err)
	}
	credit := computeProrateCredit(paid, old.StartsAt, old.ExpiresAt, time.Now())
	due := plan.Price - credit

	oldName, newName := "", ""
	if old.Plan != nil {
		oldName = old.Plan.Name
	}
	return &UpgradePreview{
		Credit: credit, TargetPrice: plan.Price, DueAmount: due,
		ValidityDays: plan.ValidityDays, Upgradeable: due > 0,
		OldPlanName: oldName, NewPlanName: plan.Name,
	}, nil
}
```
（加 `"fmt"` import）

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test -tags=unit -run TestPreviewUpgrade ./internal/service/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/bundle_upgrade_service.go backend/internal/service/bundle_upgrade_service_test.go
git commit -m "feat(bundle): PreviewUpgrade 预览差价

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 5: UpgradeBundle 原子切换 + 单测

**Files:**
- Modify: `backend/internal/service/bundle_upgrade_service.go`
- Test: `backend/internal/service/bundle_upgrade_service_test.go`

**Interfaces:**
- Consumes: `withTx`（`bundle_subscription_service.go:55`）、`syncBridgedUserSubscriptions`（:491）
- Produces: `func (s *BundleSubscriptionService) UpgradeBundle(ctx, req *UpgradeBundleRequest) (*BundleSubscription, error)` —— 履约时被 `doBundleUpgrade` 调用

- [ ] **Step 1: 写失败测试（含回滚验证）**

```go
func TestUpgradeBundle_AtomicRollbackOnBridgeFailure(t *testing.T) {
	// 构造：旧订阅 active；userSubRepo.Create 失败 → 期望事务回滚，旧订阅未被标 upgraded
	// 用 stub 让 usageRepo 或 userSubRepo 在新订阅桥接步骤返回 error
	// 断言：UpgradeBundle 返回 err；UpdateStatus(旧, upgraded) 未被提交（通过 stub 计数器或重新 GetByID 验证 status 仍 active）
}

func TestUpgradeBundle_OwnerMismatchOrNotActive(t *testing.T) {
	// 旧订阅非本人 / 非 active → 返回错误，不创建新订阅
}

func TestUpgradeBundle_Success(t *testing.T) {
	// 成功路径：旧→upgraded，新订阅 source=upgrade, upgraded_from_id=旧ID, status=active
}
```
（造 `upgradeSubRepoStub`：嵌入 `bundleSubRepoNoop`，实现 GetByIDWithUsages/GetByID/UpdateStatus/Create，记录调用；usageRepo/userSubRepo stub 仿现有）

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test -tags=unit -run TestUpgradeBundle ./internal/service/`
Expected: FAIL（`UpgradeBundle` 未定义）

- [ ] **Step 3: 实现 UpgradeBundle**

`bundle_upgrade_service.go` 加（结构仿 `ActivateBundle:96` + `RevokeBundle:217`）：
```go
func (s *BundleSubscriptionService) UpgradeBundle(ctx context.Context, req *UpgradeBundleRequest) (*BundleSubscription, error) {
	if req == nil {
		return nil, ErrBundleNotFound
	}
	var upgraded *BundleSubscription
	if err := s.withTx(ctx, func(txCtx context.Context) error {
		// ① 校验旧订阅：归属 + active（防并发升级 / 支付期间过期 / IDOR）
		old, err := s.bundleSubRepo.GetByIDWithUsages(txCtx, req.SourceSubID)
		if err != nil {
			return ErrBundleNotFound
		}
		if old.UserID != req.UserID {
			return ErrBundleNotFound
		}
		if old.Status != BundleStatusActive {
			return ErrBundleExpired
		}

		// ② 旧订阅 → upgraded，桥接 userSub → expired
		if err := s.bundleSubRepo.UpdateStatus(txCtx, old.ID, BundleStatusUpgraded); err != nil {
			return fmt.Errorf("mark old upgraded: %w", err)
		}
		if err := s.syncBridgedUserSubscriptions(txCtx, old.UserID, old.ID, func(sub *UserSubscription) error {
			return s.userSubRepo.UpdateStatus(txCtx, sub.ID, domain.SubscriptionStatusExpired)
		}); err != nil {
			return fmt.Errorf("expire bridged userSubs: %w", err)
		}

		// ③ 加载目标 plan
		plan, err := s.planRepo.GetByID(txCtx, req.TargetPlanID)
		if err != nil {
			return fmt.Errorf("load target plan: %w", err)
		}
		if !plan.ForSale || plan.Status != BundlePlanStatusActive {
			return ErrBundlePlanDisabled
		}

		// ④ 创建新订阅：source=upgrade, upgraded_from_id=旧ID, 从当下起算完整有效期
		now := time.Now()
		newSub := &BundleSubscription{
			UserID:           req.UserID,
			PlanID:           req.TargetPlanID,
			Status:           BundleStatusActive,
			StartsAt:         now,
			ExpiresAt:        now.AddDate(0, 0, plan.ValidityDays),
			ConcurrencyLimit: plan.ConcurrencyLimit,
			RPMLimit:         plan.RPMLimit,
			Source:           BundleSourceUpgrade,
			UpgradedFromID:   old.ID,
			Usages:           make([]BundleSubscriptionUsage, 0, len(plan.GroupQuotas)),
		}
		if err := s.bundleSubRepo.Create(txCtx, newSub); err != nil {
			return fmt.Errorf("create new subscription: %w", err)
		}

		// ⑤ 每个渠道组建 usage tracker + 桥接 userSub(active) —— 同 ActivateBundle:156-198
		for _, gq := range plan.GroupQuotas {
			usage := &BundleSubscriptionUsage{
				BundleSubscriptionID: newSub.ID,
				GroupID:              gq.GroupID,
				ModelPattern:         gq.ModelPattern,
				DailyWindowStart: now, WeeklyWindowStart: now, MonthlyWindowStart: now,
			}
			if err := s.usageRepo.Create(txCtx, usage); err != nil {
				return fmt.Errorf("create usage for group %d: %w", gq.GroupID, err)
			}
			bid := newSub.ID
			userSub := &UserSubscription{
				UserID: req.UserID, GroupID: gq.GroupID,
				StartsAt: now, ExpiresAt: newSub.ExpiresAt,
				Status: domain.SubscriptionStatusActive,
				BundleSubscriptionID: &bid,
				DailyLimitUSD: gq.DailyLimitUSD, WeeklyLimitUSD: gq.WeeklyLimitUSD, MonthlyLimitUSD: gq.MonthlyLimitUSD,
				// image/video limit 字段按 ActivateBundle 同款补齐
			}
			if err := s.userSubRepo.Create(txCtx, userSub); err != nil {
				return fmt.Errorf("bridge userSub for group %d: %w", gq.GroupID, err)
			}
		}
		upgraded = newSub
		return nil
	}); err != nil {
		return nil, err
	}
	if s.cache != nil {
		_ = s.cache.InvalidateBundleSubscriptionCache(ctx, req.UserID)
	}
	return upgraded, nil
}
```
（确认 import `domain`、`time`、`fmt`；UserSubscription 的 image/video 字段照 `ActivateBundle:188-192` 补全）

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test -tags=unit -run TestUpgradeBundle ./internal/service/`
Expected: PASS（含回滚用例）

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/bundle_upgrade_service.go backend/internal/service/bundle_upgrade_service_test.go
git commit -m "feat(bundle): UpgradeBundle 原子切换事务

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 6: CreateOrderRequest 字段 + 升级防重

**Files:**
- Modify: `backend/internal/service/payment_service.go:73`
- Modify: `backend/internal/service/payment_order.go`（`validateOrderInput` + `ensureNoDuplicateBundleOrder`）

**Interfaces:**
- Consumes: Task 1 的 `OrderTypeBundleUpgrade`
- Produces: `CreateOrderRequest.SourceBundleSubscriptionID` + `.ProrateCredit`；`validateOrderInput` 对 `bundle_upgrade` 放行（不触发 bundle 冲突预检）

- [ ] **Step 1: 加 CreateOrderRequest 字段**

`payment_service.go:73` 的 struct 加：
```go
PlanID     int64
SourceBundleSubscriptionID int64 // 升级订单：旧订阅ID
ProrateCredit              float64 // 升级订单：锁定的旧套餐剩余价值
Locale     string
```

- [ ] **Step 2: validateOrderInput 加 bundle_upgrade 分支**

`payment_order.go` 的 `validateOrderInput`（:156）里，找到 bundle 校验分支。升级订单不应走 `ActivateBundle` 冲突预检（升级本就有 active 订阅）。加分支：对 `bundle_upgrade` 只校验 plan 存在 + 在售 + 用户有该 source 订阅且 active，不调 `ensureNoDuplicateBundleOrder` 的 bundle 互斥逻辑（但需防"升级订单重复"，见 Step 3）。

- [ ] **Step 3: 防重覆盖升级**

`payment_order.go:44` 的 `ensureNoDuplicateBundleOrder(ctx, userID)` 当前查 `OrderTypeBundle`。改为同时匹配 `bundle` 与 `bundle_upgrade`（同一用户存在未完成的任一 bundle 类订单即拒绝新建）：
```go
// 中危3：bundle 与 bundle_upgrade 严格防重
if req.OrderType == payment.OrderTypeBundle || req.OrderType == payment.OrderTypeBundleUpgrade {
    if err := s.ensureNoDuplicateBundleOrder(ctx, req.UserID); err != nil {
        return nil, err
    }
}
```
`ensureNoDuplicateBundleOrder` 内部查询的 `OrderTypeIn` 改为 `paymentorder.OrderTypeIn(payment.OrderTypeBundle, payment.OrderTypeBundleUpgrade)`。

- [ ] **Step 4: createOrderInTx 写入新字段**

`payment_order.go:273 createOrderInTx` 里 `PaymentOrder.Create()` 的 Set 链，对 `bundle_upgrade` 订单加：
```go
if req.OrderType == payment.OrderTypeBundleUpgrade {
    bld.SetSourceBundleSubscriptionID(req.SourceBundleSubscriptionID).
        SetProrateCredit(req.ProrateCredit)
}
```

- [ ] **Step 5: 编译 + 现有支付测试不回归**

Run: `cd backend && go build ./... && go test -tags=unit ./internal/service/`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/service/payment_service.go backend/internal/service/payment_order.go
git commit -m "feat(bundle): CreateOrderRequest 升级字段与防重

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 7: payment 履约 doBundleUpgrade + 退余额边界

**Files:**
- Modify: `backend/internal/service/payment_fulfillment.go`（:212 dispatch + 新增方法）

**Interfaces:**
- Consumes: `executeFulfillment`（:212）、`ExecuteBundleFulfillment`（:690 范围，状态锁模式）、`doBundle`（:727）、`doBalance`（:278）、`UpgradeBundle`（Task 5）、`bundleSubscriptionSvc`
- Produces: `ExecuteBundleUpgradeFulfillment` + `doBundleUpgrade` + `refundUpgradeToBalance`

- [ ] **Step 1: executeFulfillment 加分支**

`payment_fulfillment.go:212`，在 `OrderTypeBundle` 分支后加：
```go
if o.OrderType == payment.OrderTypeBundle {
    return s.ExecuteBundleFulfillment(ctx, oid)
}
if o.OrderType == payment.OrderTypeBundleUpgrade {
    return s.ExecuteBundleUpgradeFulfillment(ctx, oid)
}
```

- [ ] **Step 2: 实现 ExecuteBundleUpgradeFulfillment + doBundleUpgrade**

照 `ExecuteBundleFulfillment`（状态锁 Paid/Failed → Recharging → doBundleUpgrade → markCompleted/markFailed）：
```go
func (s *PaymentService) ExecuteBundleUpgradeFulfillment(ctx context.Context, oid int64) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.Status == OrderStatusCompleted {
		return nil
	}
	if psIsRefundStatus(o.Status) {
		return infraerrors.BadRequest("INVALID_STATUS", "refund-related order cannot fulfill")
	}
	if o.Status != OrderStatusPaid && o.Status != OrderStatusFailed {
		return infraerrors.BadRequest("INVALID_STATUS", "order cannot fulfill in status "+o.Status)
	}
	if o.SourceBundleSubscriptionID == 0 || o.PlanID == 0 {
		return infraerrors.BadRequest("INVALID_STATUS", "missing source_sub_id/plan_id for upgrade order")
	}
	c, err := s.entClient.PaymentOrder.Update().Where(
		paymentorder.IDEQ(oid), paymentorder.StatusIn(OrderStatusPaid, OrderStatusFailed),
	).SetStatus(OrderStatusRecharging).Save(ctx)
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	if c == 0 {
		return nil
	}
	if err := s.doBundleUpgrade(ctx, o); err != nil {
		s.markFailed(ctx, oid, err)
		return err
	}
	return nil
}

func (s *PaymentService) doBundleUpgrade(ctx context.Context, o *dbent.PaymentOrder) error {
	if s.bundleSubscriptionSvc == nil {
		return fmt.Errorf("bundle subscription service not available")
	}
	// 幂等
	if s.hasAuditLog(ctx, o.ID, "BUNDLE_UPGRADE_SUCCESS") {
		return s.markCompleted(ctx, o, "BUNDLE_UPGRADE_SUCCESS")
	}
	newSub, err := s.bundleSubscriptionSvc.UpgradeBundle(ctx, &UpgradeBundleRequest{
		UserID: o.UserID, SourceSubID: o.SourceBundleSubscriptionID, TargetPlanID: o.PlanID,
	})
	if err != nil {
		// 旧订阅已非 active（支付期间过期 / 并发升级）→ 退款到余额
		if errors.Is(err, ErrBundleExpired) || errors.Is(err, ErrBundleNotFound) {
			slog.Warn("upgrade fulfill: old sub no longer active, refund to balance", "orderID", o.ID, "err", err)
			return s.refundUpgradeToBalance(ctx, o)
		}
		return fmt.Errorf("upgrade bundle: %w", err)
	}
	if newSub != nil {
		if _, uErr := s.entClient.PaymentOrder.UpdateOneID(o.ID).
			SetBundleSubscriptionID(newSub.ID).Save(ctx); uErr != nil {
			slog.Warn("upgrade order: write back bundle_subscription_id failed", "orderID", o.ID, "error", uErr)
		}
	}
	return s.markCompleted(ctx, o, "BUNDLE_UPGRADE_SUCCESS")
}

// refundUpgradeToBalance 升级履约失败时把已付差价退到用户余额（资金不出平台）。
// 复用 doBalance 的 redeem 机制：建一个 balance 兑换码并 Redeem。
func (s *PaymentService) refundUpgradeToBalance(ctx context.Context, o *dbent.PaymentOrder) error {
	if s.hasAuditLog(ctx, o.ID, "BUNDLE_UPGRADE_REFUND_BALANCE") {
		return s.markCompleted(ctx, o, "BUNDLE_UPGRADE_REFUND_BALANCE")
	}
	code := fmt.Sprintf("UPGRADERFND-%d-%d", o.ID, o.UserID)
	if _, lookupErr := s.redeemService.GetByCode(ctx, code); lookupErr != nil {
		rc := &RedeemCode{Code: code, Type: RedeemTypeBalance, Value: o.Amount, Status: StatusUnused}
		if err := s.redeemService.CreateCode(ctx, rc); err != nil {
			return fmt.Errorf("create refund redeem code: %w", err)
		}
	}
	if _, err := s.redeemService.Redeem(ContextSkipRedeemAffiliate(ctx), o.UserID, code); err != nil {
		return fmt.Errorf("redeem refund balance: %w", err)
	}
	return s.markCompleted(ctx, o, "BUNDLE_UPGRADE_REFUND_BALANCE")
}
```
（import `errors`、`log/slog`、`fmt`；`RedeemTypeBalance`/`StatusUnused`/`ContextSkipRedeemAffiliate` 见 `doBalance:291`）

- [ ] **Step 3: 写履约单测（幂等 + 退款路径）**

参考 `payment_order_lifecycle_test.go` 的构造模式，测：
- 重复调用 `doBundleUpgrade`（已有 audit log）→ 直接 markCompleted，UpgradeBundle 不被再次调用
- UpgradeBundle 返回 ErrBundleExpired → 触发 `refundUpgradeToBalance`，余额增加 o.Amount

- [ ] **Step 4: 跑测试**

Run: `cd backend && go test -tags=unit -run 'TestDoBundleUpgrade|TestRefundUpgrade' ./internal/service/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/payment_fulfillment.go backend/internal/service/*_test.go
git commit -m "feat(bundle): 升级履约 doBundleUpgrade + 失败退余额

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 8: Handler + 路由 + IDOR

**Files:**
- Modify: `backend/internal/handler/bundle_handler.go`
- Modify: `backend/internal/server/routes/bundle.go`

**Interfaces:**
- Consumes: `PreviewUpgrade`（Task 4）、`PaymentService.CreateOrder`（Task 6）、`payment.OrderTypeBundleUpgrade`
- Produces: `POST /bundles/upgrade/preview`、`POST /bundles/upgrade`

- [ ] **Step 1: PreviewUpgrade handler**

`bundle_handler.go` 加（仿 `GetMyBundle:78` 取鉴权 subject）：
```go
func (h *BundleHandler) PreviewUpgrade(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	var req struct {
		SourceBundleSubscriptionID int64 `json:"source_bundle_subscription_id" binding:"required"`
		TargetPlanID               int64 `json:"target_plan_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	pv, err := h.bundleSubscriptionService.PreviewUpgrade(c.Request.Context(), subject.UserID, req.SourceBundleSubscriptionID, req.TargetPlanID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, pv)
}
```

- [ ] **Step 2: Upgrade handler（下单）**

```go
func (h *BundleHandler) Upgrade(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	var req struct {
		SourceBundleSubscriptionID int64  `json:"source_bundle_subscription_id" binding:"required"`
		TargetPlanID               int64  `json:"target_plan_id" binding:"required"`
		PaymentType                string `json:"payment_type"`
		UseBalance                 bool   `json:"use_balance"`
		ReturnURL                  string `json:"return_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.ReturnURL != "" && !isValidReturnURL(req.ReturnURL) {
		response.BadRequest(c, "Invalid return_url: only relative paths are allowed")
		return
	}
	// 复用 PreviewUpgrade 算 credit（归属/active/在售校验都在里面）
	pv, err := h.bundleSubscriptionService.PreviewUpgrade(c.Request.Context(), subject.UserID, req.SourceBundleSubscriptionID, req.TargetPlanID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !pv.Upgradeable {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"type": "bundle_not_upgradeable", "message": "目标套餐需补差价<=0，请等当前套餐到期后再购买"}})
		return
	}
	result, err := h.paymentService.CreateOrder(c.Request.Context(), service.CreateOrderRequest{
		UserID:                     subject.UserID,
		Amount:                     pv.DueAmount,
		PaymentType:                req.PaymentType,
		ClientIP:                   c.ClientIP(),
		IsMobile:                   isMobile(c),
		SrcHost:                    c.Request.Host,
		SrcURL:                     c.Request.Referer(),
		ReturnURL:                  req.ReturnURL,
		OrderType:                  payment.OrderTypeBundleUpgrade,
		PlanID:                     req.TargetPlanID,
		SourceBundleSubscriptionID: req.SourceBundleSubscriptionID,
		ProrateCredit:              pv.Credit,
		Locale:                     c.GetHeader("Accept-Language"),
		UseBalance:                 req.UseBalance,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
```
（import `payment`、`http`、`service`；`isMobile` 见 `Checkout:191`）

- [ ] **Step 3: 注册路由**

`internal/server/routes/bundle.go`（仿现有 `/checkout` 注册）加：
```go
bundleGroup.GET("/upgrade/preview", bundleHandler.PreviewUpgrade)  // 或 POST，与现有风格一致
bundleGroup.POST("/upgrade", bundleHandler.Upgrade)
```
（确认现有路由组的变量名 `bundleGroup`/鉴权 middleware，按文件实际）

- [ ] **Step 4: 编译 + 跑 handler 测试（若有）**

Run: `cd backend && go build ./... && golangci-lint run ./internal/handler/... ./internal/service/...`
Expected: 通过（depguard 不违规——handler 只调 service）

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handler/bundle_handler.go backend/internal/server/routes/bundle.go
git commit -m "feat(bundle): 升级预览与下单 handler + 路由

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 9: 前端 api + BundlesView + i18n

**Files:**
- Modify: `frontend/src/api/bundles.ts`
- Modify: `frontend/src/views/user/BundlesView.vue`
- Modify: `frontend/src/i18n/locales/zh.json` + `en.json`（或项目实际 locale 文件）

**Interfaces:**
- Consumes: `POST /bundles/upgrade/preview`、`POST /bundles/upgrade`（Task 8）

- [ ] **Step 1: api 客户端**

`api/bundles.ts` 仿现有 `checkout` 函数加：
```ts
export interface UpgradePreview {
  credit: number
  target_price: number
  due_amount: number
  validity_days: number
  upgradeable: boolean
  old_plan_name: string
  new_plan_name: string
}

export async function previewBundleUpgrade(sourceBundleSubscriptionId: number, targetPlanId: number) {
  const { data } = await api.post('/bundles/upgrade/preview', {
    source_bundle_subscription_id: sourceBundleSubscriptionId,
    target_plan_id: targetPlanId,
  })
  return data as UpgradePreview
}

export async function createBundleUpgradeOrder(payload: {
  source_bundle_subscription_id: number
  target_plan_id: number
  payment_type: string
  use_balance: boolean
  return_url?: string
}) {
  const { data } = await api.post('/bundles/upgrade', payload)
  return data
}
```
（import 的 `api` 实例按文件现有）

- [ ] **Step 2: BundlesView 升级按钮 + 确认窗**

`BundlesView.vue`：用户有 active 订阅时，遍历在售套餐：
- 当前套餐 → "使用中"（置灰）
- `previewBundleUpgrade` 返回 `upgradeable===true` 的套餐 → 按钮文案"升级"，点击弹确认窗显示 `剩余价值 ¥{{credit}} 抵扣，需补差价 ¥{{due_amount}}，新有效期 {{validity_days}} 天`，选支付方式后调 `createBundleUpgradeOrder`，跳支付流程（复用现有 checkout 后的支付跳转逻辑）
- `upgradeable===false` → 置灰"到期后可购买"

（具体模板/script 按现有 BundlesView 结构插入；复用其支付方式选择与跳转组件）

- [ ] **Step 3: i18n 文案**

`zh.json` / `en.json` 的 bundle 段加：
```json
"upgrade": "升级",
"upgrade_confirm_title": "确认升级套餐",
"upgrade_credit_hint": "旧套餐剩余价值 ¥{credit} 将抵扣",
"upgrade_due_hint": "需补差价 ¥{due}，新有效期 {days} 天",
"not_upgradeable": "到期后可购买"
```

- [ ] **Step 4: 前端校验**

Run: `cd frontend && pnpm run typecheck && pnpm run lint:check`
Expected: 通过

- [ ] **Step 5: Commit**

```bash
git add frontend/src/api/bundles.ts frontend/src/views/user/BundlesView.vue frontend/src/i18n/
git commit -m "feat(bundle): 前端套餐升级流程

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 10: E2E 集成测试

**Files:**
- Test: `backend/internal/service/`（集成测试，`//go:build integration`）或 `make test-e2e-local`

- [ ] **Step 1: 完整升级链路集成测试**

覆盖 spec §12 全部场景（用真实 ent client + 测试 DB，仿 `payment_order_lifecycle_test.go`）：
1. **正常升级** starter→pro：建 starter 订阅 + 购买订单(completed) → preview 返回 upgradeable=true、credit 按比例 → 下升级单 → 模拟支付回调 → 断言旧订阅 status=upgraded、新订阅 status=active 且 source=upgrade、upgraded_from_id=旧ID、expires_at≈now+30d、订单 bundle_subscription_id=新订阅、prorate_credit 锁定值
2. **二次升级** pro→enterprise：基于上一步的 pro 订阅（对应升级订单 amount=差价）再 preview，credit 按差价折算
3. **并发升级**：两个 doBundleUpgrade 并发，只一个成功，另一个走 refundUpgradeToBalance
4. **支付期间过期**：构造旧订阅 expires_at 在过去 → doBundleUpgrade 走退款路径，user.balance += order.Amount
5. **幂等**：重复回调同一订单，只切换一次
6. **IDOR**：用户 A 用用户 B 的 source_sub_id 调 preview/upgrade → 报错

- [ ] **Step 2: 跑集成测试**

Run: `cd backend && go test -tags=integration -run TestBundleUpgrade ./internal/service/` 或 `make test-e2e-local`
Expected: 全 PASS

- [ ] **Step 3: Commit**

```bash
git add backend/internal/service/*_integration_test.go
git commit -m "test(bundle): 套餐升级集成测试

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Self-Review（写完后核对 spec）

**Spec 覆盖核对：**
- §5 数据模型 → Task 1 ✓
- §6 credit 契约（公式/实付来源/边界/防时序）→ Task 2（纯函数）+ Task 4（反查实付）+ Task 6（credit 锁定入订单）✓
- §7 交易链路 → Task 8（preview+下单）+ Task 7（履约）✓
- §8 服务方法签名 → Task 4/5/7 ✓
- §9 UpgradeBundle 原子切换 → Task 5 ✓
- §10 边界（IDOR/支付期间过期退余额/并发/幂等/二次升级）→ Task 5（IDOR+并发）+ Task 7（退余额+幂等）+ Task 10（二次升级 E2E）✓
- §11 前端 → Task 9 ✓
- §12 测试 → 各 task 单测 + Task 10 集成 ✓
- §13 实现顺序 → Task 1→10 顺序 ✓

**类型一致性：** `UpgradeBundleRequest{UserID, SourceSubID, TargetPlanID}`（Task 3 定义，Task 5/7 使用一致）；`UpgradePreview`（Task 3 定义，Task 4/8/9 使用一致）；`PaymentOrderReader.GetPaidAmountByBundleSub`（Task 3 定义，Task 4 使用一致）；`computeProrateCredit`（Task 2 定义，Task 4 使用）✓

**已知实现期需现场确认的点（非占位符，是定位指引）：**
- Task 3 Step 2：`PaymentOrderRepository` 的确切文件位置与 `OrderStatusCompleted` 常量名——搜索 `payment_order.go` 的 OrderStatus 常量
- Task 3 Step 5：ent→service 模型映射函数名——搜索 repository 层 `BundleSubscription` 转换
- Task 5 Step 3：`UserSubscription` 的 image/video 字段名——照 `ActivateBundle:188-192`
- Task 7 Step 2：`RedeemTypeBalance`/`StatusUnused`/`ContextSkipRedeemAffiliate`——已在 `doBalance:291/298` 出现，直接引用
- Task 8 Step 3：路由组变量名与 middleware——读 `routes/bundle.go`（713B）确认
