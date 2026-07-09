# 套餐升级（Bundle Upgrade）功能设计

- **日期**: 2026-07-08
- **状态**: Draft（待复核）
- **分支**: feat/bundles
- **作者**: brainstorming session

---

## 1. 背景与现状

当前套餐（bundle）子系统**不支持「切换/升级/退费」**。一个用户同一时刻只能持有 1 个生效订阅，三道机制共同强制这一点：

1. **下单前预检** — `internal/handler/bundle_handler.go:169-184`：若用户已有 active 套餐，直接返回 `409 bundle_conflict / "您已有生效中的套餐，无法重复购买"`，注释明确 *"套餐暂不支持退款"*。
2. **激活层兜底** — `internal/service/bundle_subscription_service.go:109-121`：`ActivateBundle` 检测到已有 active 套餐时，普通购买返回 `ErrBundleConflict`；仅 `admin_assign` 来源会「撤销旧 + 激活新」（且无任何退费/抵扣计算）。
3. **数据模型** — `ent/schema/bundle_subscription.go` 状态枚举仅 `active/expired/revoked`，来源仅 `purchase/redeem/admin_assign`；全局搜索 `upgrade|prorat|refund|退费|抵扣` 零命中。

典型受挫场景：用户买了 starter（30 天），用了 2 天觉得额度不够，想换 pro —— 当前只能等 starter 到期再买，或求助管理员手动覆盖（不退款）。本设计补齐「自助升级」能力。

## 2. 目标与非目标

### 目标
- 用户可在当前套餐未到期时，自助升级到更高价值的套餐
- 旧套餐剩余价值按比例折算（prorate credit）抵扣新套餐价格，用户只补差价
- 资金不出平台（不原路退回支付渠道），全程复用现有支付/履约/余额基础设施
- 升级过程原子、幂等、可审计、防 IDOR

### 非目标（YAGNI，本期不做）
- 降级（pro→starter）与同级切换 —— 仅允许升级
- 原路退回支付渠道（支付宝/微信/Stripe 退款 API）
- 阶梯式/非线性退款规则
- 升级后再「反悔撤销升级」
- 把 credit 退成可见的、可提现的钱包余额（credit 只用于当次差价抵扣，不落余额；唯一例外见 §10 边界「支付期间过期」）

## 3. 产品决策汇总

| 维度 | 决策 |
|---|---|
| 退款模型 | **prorate credit 差价抵扣**：旧套餐剩余价值算成 credit，从新套餐价格中扣除，用户只补差价，不产生实际退款动作，资金不出平台 |
| credit 口径 | **按剩余天数线性比例**：`实付金额 × 剩余秒数 / 总秒数`，不追溯已消费的请求额度（套餐卖的是「有效期内使用权」，不是预付 token） |
| 切换范围 | **仅允许升级**：差价 > 0 才允许；降级/同级（差价 ≤ 0）提示用户等当前套餐到期后再购买 |
| 新套餐有效期 | **从升级当下重新起算完整 `ValidityDays`**（行业标准 prorate 语义） |
| 架构方案 | **方案 A：升级走支付订单 + 复用 `payment_fulfillment` 履约链路**（见 §4） |

## 4. 架构（方案 A）

升级本质是「带退款抵扣的二次购买」，套进现有「订单 + 履约」两段式：

- **下单段**（不碰订阅状态）：服务端实时算 credit 与差价，建一笔 `order_type=bundle_upgrade` 订单，金额=差价，锁定 `prorate_credit`。发起支付（渠道/余额抵扣，复用 `balance_deduct_amount`）。
- **履约段**（支付成功才执行）：`payment_fulfillment.go` 新增 `doBundleUpgrade` 分支，一个事务内「标记旧订阅 upgraded + 激活新订阅 + 回写对账」，复用幂等审计 / 状态机 / 通知。

**核心好处**：支付失败/放弃时旧订阅零影响（切换只在支付成功回调里发生），无需任何回滚逻辑。

## 5. 数据模型变更

### 5.1 `bundle_subscriptions` 表
| 变更 | 说明 |
|---|---|
| 新增字段 `upgraded_from_id Int64 NULL` | 升级产生的新订阅才有值，指向被替换的旧订阅 ID，形成升级链供客服/对账追溯 |
| status 枚举新增 `upgraded` 值 | 旧订阅被升级时置此值（区别于 `expired` 自然过期、`revoked` 人为撤销） |
| source 枚举新增 `upgrade` 值 | 升级产生的新订阅 source=`upgrade`，运营报表可区分新购/兑换/升级 |

> status/source 是 string 字段无 DB 枚举约束（见 `ent/schema/bundle_subscription.go`），新增值只需在 `bundle_constants.go` 加常量，零迁移成本。

### 5.2 `payment_orders` 表
| 变更 | 说明 |
|---|---|
| 新增字段 `source_bundle_subscription_id Int64 NULL` | 升级订单才有值，指向被升级的旧订阅 |
| 新增字段 `prorate_credit Numeric(20,12) NOT NULL DEFAULT 0` | 下单时锁定的 credit。履约时以订单记录为准，**不重算** |

复用已有字段：`plan_id`（=目标套餐）、`bundle_subscription_id`（履约回写新订阅）、`balance_deduct_amount`（余额抵扣）、`amount`（=差价）。

### 5.3 常量新增
- `backend/internal/payment/types.go`：`OrderTypeBundleUpgrade = "bundle_upgrade"`（现有 `OrderTypeBalance/OrderTypeSubscription/OrderTypeBundle` 在 `:42-44`）
- `backend/internal/service/bundle_constants.go`：`BundleStatusUpgraded = "upgraded"`、`BundleSourceUpgrade = "upgrade"`

### 5.4 迁移 `backend/migrations/161_bundle_upgrade.sql`
```sql
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE bundle_subscriptions ADD COLUMN IF NOT EXISTS upgraded_from_id BIGINT NULL;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS source_bundle_subscription_id BIGINT NULL;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS prorate_credit NUMERIC(20,12) NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_bundle_subs_upgraded_from ON bundle_subscriptions(upgraded_from_id);
CREATE INDEX IF NOT EXISTS idx_payment_orders_src_bundle ON payment_orders(source_bundle_subscription_id);
```
（序号 161 = 当前最大 `160_bundle_order_subscription_ref.sql` + 1。schema 改后须 `go generate ./ent` 并提交生成代码。）

## 6. credit 契约

### 6.1 公式（时间用秒，避免整数天数边界争议）
```
credit      = 实付金额 × max(0, 剩余秒数) / 总秒数
剩余秒数    = 旧订阅.expires_at - now
总秒数      = 旧订阅.expires_at - 旧订阅.starts_at
差价        = 目标套餐.price - credit
允许升级    ⟺ 差价 > 0
```
精度：credit 保留 2 位小数（货币精度），落库用 `Numeric(20,12)` 对齐项目计费链路。

### 6.2 实付金额来源
反查 `payment_orders` 中 `bundle_subscription_id = 旧订阅ID` 且 `status=completed` 的订单 `amount`（取最早一笔，正常只有一笔）。

### 6.3 边界
- **兑换/管理员分配的旧订阅**（source=redeem/admin_assign）：无购买订单 → 实付=0 → credit=0 → 全额补差价。UI 标注「当前套餐为兑换/赠送，升级无剩余价值抵扣」。合理（白嫖套餐本无剩余价值）。
- **二次升级**（pro→enterprise）：pro 是 upgrade 来源，对应订单 `amount` = 当初补的差价。credit 基于该差价折算 —— 逻辑自洽，**无需特判**。不变量：每次升级的实付 = 那次补的差价。
- **旧订阅非 active / 已过期**：拒绝升级，引导直接购买。
- **剩余秒数 ≤ 0**：视为已过期，拒绝。

### 6.4 防时序竞态
**credit 在下单时锁定、存入 `payment_orders.prorate_credit`，履约时不重算**。套餐价格运营可随时改（`UpdatePlan`），若履约时重读 `plan.price` 会与用户下单看到的差价不一致。锁进订单字段即以「订单为事实来源」，与现有 `payment_fulfillment` 哲学一致。

## 7. 交易链路（端到端）

```
用户点"升级到 pro"
        │
        ▼
① GET /bundles/upgrade/preview   (source_sub_id, target_plan_id)
   └─ PreviewUpgrade(): 校验归属+active → 反查旧订单实付 → 算 credit
   └─ 返回 {credit, target_price, due_amount, validity_days, upgradeable}
        │  （只读，不改任何状态）
        ▼
② POST /bundles/upgrade          (source_sub_id, target_plan_id, payment_type, use_balance, return_url)
   └─ 再算一次 credit → 差价 ≤0 则 409（仅升级）
   └─ CreateOrder(order_type=bundle_upgrade, amount=差价,
                  source_bundle_subscription_id=旧, prorate_credit=credit, plan_id=目标)
   └─ 发起支付（渠道 / 余额抵扣）
        │  （旧订阅仍是 active，未动）
        ▼
③ 支付回调 → payment_fulfillment.doBundleUpgrade(订单)
   └─ 幂等: hasAuditLog("BUNDLE_UPGRADE_SUCCESS")?
   └─ bundleSubscriptionSvc.UpgradeBundle() 一个事务内原子切换（§9）
   └─ 回写 order.bundle_subscription_id = 新订阅
   └─ markCompleted("BUNDLE_UPGRADE_SUCCESS") + 推送升级成功通知
```

## 8. 新增服务方法

分层严守 depguard：credit 计算需反查 `payment_orders`（repository 层），不能进 handler。

```go
// 只读预览：给前端展示"剩余价值 ¥X 抵扣，需补差价 ¥Y"
// 归属校验在此：source_sub 必须属于 userID 且 status=active，否则报错
func (s *BundleSubscriptionService) PreviewUpgrade(
    ctx context.Context, userID, sourceSubID, targetPlanID int64,
) (*UpgradePreview, error)

type UpgradePreview struct {
    Credit        float64 // 旧套餐剩余价值
    TargetPrice   float64
    DueAmount     float64 // 差价 = TargetPrice - Credit
    ValidityDays  int     // 新套餐有效期
    Upgradeable   bool    // 差价 > 0
    OldPlanName   string
    NewPlanName   string
}

// 履约时调用：原子切换
type UpgradeBundleRequest struct {
    UserID       int64
    SourceSubID  int64 // 旧订阅
    TargetPlanID int64
}

func (s *BundleSubscriptionService) UpgradeBundle(
    ctx context.Context, req *UpgradeBundleRequest,
) (*BundleSubscription, error)
```

`PaymentService.doBundleUpgrade(ctx, o *dbent.PaymentOrder) error` —— 照 `doBundle`（`payment_fulfillment.go:727`）骨架，调 `UpgradeBundle`。

## 9. UpgradeBundle 原子切换（核心，不复用 ActivateBundle）

`ActivateBundle:109` 的 conflict 检查会拦住升级，故升级走独立方法。在一个事务内（复用现有 `withTx`（`:55`）+ `syncBridgedUserSubscriptions`（`:491`）模式）：

```
事务开始(txCtx)
 ├─ ① 加载旧订阅，校验: 归属==req.UserID 且 status==active
 │     （防并发升级 / 支付期间过期 / IDOR —— 否则 abort）
 ├─ ② 旧订阅 status = upgraded
 │     其桥接 userSub → expired（syncBridgedUserSubscriptions，与 RevokeBundle:217 同构）
 ├─ ③ 加载 target plan（校验 active + for_sale）
 ├─ ④ 创建新订阅:
 │     status=active, source=upgrade, upgraded_from_id=旧ID,
 │     concurrency_limit/rpm_limit 从 target plan 快照拷贝,
 │     starts_at=now, expires_at=now+target.ValidityDays
 ├─ ⑤ 为新订阅每个 group_quota 建 usage tracker + 桥接 userSub(active)
 │     （与 ActivateBundle:156-198 同构）
 └─ 提交（任一步失败全部回滚，旧订阅不被标记 upgraded）
事务结束后 → InvalidateBundleSubscriptionCache(userID)
```

并发防护点在 ①（数据库行级：旧订阅 status），而非下单接口。两个升级订单可能都建成功，但第一个事务把旧订阅置 `upgraded` 提交后，第二个事务进入 ① 发现非 active → abort，其订单走失败退款（§10）。

## 10. 边界与错误处理矩阵

| 场景 | 处理 |
|---|---|
| 旧订阅非 active（expired/revoked/upgraded） | preview/下单 409，引导直接购买 |
| **旧订阅非本人（IDOR）** | 404（`source.user_id != 当前用户` 判不属，不泄露存在性） |
| 目标套餐下架/非 for_sale | 400 |
| 差价 ≤ 0（降级/同级/特殊定价） | preview 返回 `upgradeable=false`；下单 409 |
| 兑换/赠送旧订阅 credit=0 | 正常流程，UI 标注"无剩余价值抵扣" |
| 支付失败/放弃 | 旧订阅零影响（两段式天然保护） |
| **支付期间旧订阅自然过期** | 履约①失败 → 订单 Failed → **退款到 `user.balance`**（复用 `doBalance` 加余额机制 + 审计日志 + 通知）。这是整个设计唯一动余额的场景 |
| 并发升级（多端/双击） | 第一个成功；第二个履约①失败 → Failed → 退 balance |
| 二次升级（pro→enterprise） | credit 基于升级订单差价反查，自洽，无特判 |
| 重复支付回调 | `hasAuditLog("BUNDLE_UPGRADE_SUCCESS")` 幂等跳过 |

**安全重点**：升级接口的 `source_bundle_subscription_id` 必须在 service 层校验归属，且查询走当前用户 scope —— 与近期 IDOR 修复（`ecb747d9`/`024c7879`）的收窄原则一致。

## 11. 前端（`src/views/user/` 套餐区 + `src/api/`）

套餐列表页，当用户已有 active 订阅时，卡片按钮按差价动态化：
- **当前套餐** → "使用中"（置灰）
- **更高价值套餐**（preview 返回 `upgradeable=true`）→ "升级"按钮 → 弹确认窗显示「旧套餐剩余价值 ¥X 抵扣，需补差价 ¥Y，新有效期 30 天」→ 选支付方式（含余额）→ 调 `/bundles/upgrade`
- **更低/同级**（`upgradeable=false`）→ 置灰"到期后可购买"
- 升级成功 toast + 刷新"我的套餐"
- i18n 文案补 `src/i18n`（升级相关 key）

## 12. 测试策略

复用现有 `bundle_subscription_service_test.go` 的 mock 模式。

- **纯函数单测 `computeProrateCredit`**：刚买 1 分钟、剩 1 天、剩 0 天、credit=0（无订单）、二次升级（按差价反查）—— 全覆盖比例公式边界
- **PreviewUpgrade**：差价 ≤0 返回 `upgradeable=false`；非本人/非 active 返回错误
- **UpgradeBundle 事务**：模拟第⑤步桥接 userSub 创建失败 → 断言旧订阅**未**被标记 upgraded（回滚验证）；并发双调用只成一个
- **doBundleUpgrade 履约**：完整 starter→pro 链路（下单→支付→履约→断言旧=upgraded、新=active、订单回写、credit 锁定）；重复回调幂等；支付期间过期→退 balance

## 13. 实现顺序建议

1. 常量 + Ent schema + 迁移 161（`go generate ./ent`，起 PG 容器验证）
2. `computeProrateCredit` 纯函数 + 单测（地基，先验正确）
3. `PreviewUpgrade` + 单测
4. `UpgradeBundle` 事务 + 单测（含回滚、并发）
5. `doBundleUpgrade` 履约 + 幂等 + 退 balance 边界
6. handler `/bundles/upgrade` + `/bundles/upgrade/preview`（含 IDOR 校验）
7. 前端卡片按钮 + 确认窗 + api 客户端 + i18n
8. E2E 集成测试（完整链路）

## 14. 风险与取舍

- **`prorate_credit` 用 Numeric(20,12)**：略宽于货币所需的 2 位小数，对齐项目计费链路精度，避免后续聚合计算再溢出。代价可忽略。
- **支付期间过期退 balance 而非原路**：用户已付款但旧套餐过期，按「资金不出平台」原则退余额。用户若强烈期望原路退，属未来需求（需对接各渠道退款 API）。
- **升级判定用「差价 > 0」而非「tier 提升」**：更贴近「补差价」的付费本质；tier 仅作 UI 引导。若未来套餐出现「tier 高但价格低」的特殊定价，仍由差价正确判定。
- **新增 `upgraded` 状态 / `upgrade` source**：所有 status/source switch 须覆盖新值（interface 未变，但分支逻辑要补）。
