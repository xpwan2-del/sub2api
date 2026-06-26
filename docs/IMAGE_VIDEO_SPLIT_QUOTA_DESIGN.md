# 设计:套餐图片/视频分开限流

- **日期**: 2026-06-26
- **分支**: feat/bundles
- **状态**: 已与用户确认设计,待写实现计划

## 1. 背景与目标

套餐订阅计费当前把**图片生成**(按张)与**视频生成**(按段)合并成单一 `OutputCount`,累加进同一条 `*_limit_count` / `*_usage_count` 次数限额。运营无法对图片和视频分别设限(例如"每天 100 张图片 + 10 段视频")。

**目标**: 把次数维度拆成「图片」「视频」两条独立轨道(各 day/week/month),限额配置与用量跟踪都分开,USD 维度保持不变。

## 2. 现状分析(关键事实)

### 两层限额配置
- **分组层 `Group`**(`ent/schema/group.go:61-72`): 计费类型 `subscription_type=subscription` 时只有 `daily/weekly/monthly_limit_usd`(`DECIMAL(20,8)`),**无 count 字段**。前端 `frontend/src/views/admin/GroupsView.vue:604-645`。
- **套餐计划层 `BundlePlanGroupQuota`**(`ent/schema/bundle_plan_group_quota.go:34-39`): USD × 3 + `daily/weekly/monthly_limit_count` × 3,另有 `quota_scope`(platform/model)+ `model_pattern`(glob)。前端 `frontend/src/views/admin/bundles/BundlePlansView.vue:192-229`。

### 计费链路(合并点)
- `ForwardResult` 已分别有 `ImageCount` / `VideoCount` 两个字段(`internal/service/gateway_service.go:567-568`)。
- 合并发生在:
  - `internal/service/gateway_service.go:9529` — `OutputCount: result.ImageCount + result.VideoCount`
  - `internal/service/openai_gateway_service.go:6114` — 同上
- `AccumulateUsage(ctx, bundleSubID, groupID, costUSD, count)`(`internal/service/bundle_usage_service.go:73`)用合并后的 `OutputCount` 累加。
- 仓库 `IncrementUsage(ctx, id, costUSD, count, roll)`(`internal/repository/bundle_usage_repo.go:81`,接口 `internal/service/bundle_usage_port.go:28`)。

### 视频计费单位
- 视频**按段/次**计,非秒: `internal/service/openai_videos.go:132/154/175` 每段固定 `VideoCount: 1`;Veo 走 Gemini 路径由转发层填充(`gateway_service.go:9528`)。

### 校验时机(fail-open)
- Pre-flight 软检查在中间件 `internal/server/middleware/bundle_resolver.go:184-204`,**fail-open**(注释:"strictness is enforced post-billing")。瞬时错误/并发超额都不阻塞请求,严格扣减在 post-billing。→ modality 推断不精确也安全。

### 现有 count 字段来源
- 迁移 `154_bundle_count_quotas.sql`(bundle_plan_group_quotas + bundle_subscription_usages)、`155_bundle_user_sub_count_snapshot.sql`(user_subscriptions),均为 `ADD COLUMN IF NOT EXISTS INTEGER NOT NULL DEFAULT 0`。最新序号 **155**。

## 3. 设计决策(均已与用户确认)

| 决策点 | 选择 |
|---|---|
| 限额落点 | **套餐计划层** `BundlePlanGroupQuota`(分组层保持 USD 兜底不变) |
| 视频单位 | 沿用**段/次** |
| 字段过渡 | **复用现有 count 为图片 + 新增视频** |
| 实现形态 | **方案 A:直接拆字段**(不引入 modality 通用维度,YAGNI) |
| 列重命名 | **RENAME** `*_count` → `*_image_count` + 新增 `*_video_count` |
| usage_log | **新增 `video_count` 字段**,与 `image_count` 对称 |

## 4. 数据模型变更

### 4.1 `bundle_plan_group_quotas`(限额配置)
- RENAME: `daily/weekly/monthly_limit_count` → `daily/weekly/monthly_image_limit_count`
- ADD: `daily/weekly/monthly_video_limit_count`(`INTEGER NOT NULL DEFAULT 0`,0=不限)

### 4.2 `bundle_subscription_usages`(用量跟踪)
- RENAME: `daily/weekly/monthly_usage_count` → `daily/weekly/monthly_image_usage_count`
- ADD: `daily/weekly/monthly_video_usage_count`(`INTEGER NOT NULL DEFAULT 0`)
- **窗口字段不动**: `*_window_start` 由图片/视频共享(窗口是时间维度)

### 4.3 `user_subscriptions`(限额快照)
- RENAME: `daily/weekly/monthly_limit_count` → `daily/weekly/monthly_image_limit_count`
- ADD: `daily/weekly/monthly_video_limit_count`(`INTEGER NOT NULL DEFAULT 0`)
- 快照逻辑在 `internal/service/bundle_subscription_service.go`(激活时从 BundlePlanGroupQuota 复制),需同步拆分 image/video。

### 4.4 `usage_logs`(明细,新增)
- ADD: `video_count` `INT DEFAULT 0`,与现有 `image_count`(`ent/schema/usage_log.go:130` 附近)对称。
- `gateway_service.go` 写 UsageLog 处(约 `9717` 行 `ImageCount: result.ImageCount` 附近)同步填 `VideoCount: result.VideoCount`。

### 4.5 ent schema 同步
- 改 `bundle_plan_group_quota.go`、`bundle_subscription_usage.go`、`user_subscription.go`、`usage_log.go` 4 个 schema。
- 运行 `cd backend && go generate ./ent`,提交生成的 `ent/` 代码。
- 全量替换字段引用(如 `DailyLimitCount` → `DailyImageLimitCount`):`grep -rn "LimitCount\|UsageCount" backend/internal/`。

## 5. 迁移策略

新建 `backend/migrations/156_bundle_image_video_split_quota.sql`:

```sql
-- 156_bundle_image_video_split_quota.sql
-- 套餐次数限额拆分图片/视频:现有 count 复用为图片,新增视频。
-- 迁移有 SHA256 锁定只跑一次,RENAME 非幂等可接受;用 DO 块包裹以求健壮。
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

-- bundle_plan_group_quotas
ALTER TABLE bundle_plan_group_quotas RENAME COLUMN daily_limit_count   TO daily_image_limit_count;
ALTER TABLE bundle_plan_group_quotas RENAME COLUMN weekly_limit_count  TO weekly_image_limit_count;
ALTER TABLE bundle_plan_group_quotas RENAME COLUMN monthly_limit_count TO monthly_image_limit_count;
ALTER TABLE bundle_plan_group_quotas
    ADD COLUMN IF NOT EXISTS daily_video_limit_count   INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS weekly_video_limit_count  INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monthly_video_limit_count INTEGER NOT NULL DEFAULT 0;

-- bundle_subscription_usages
ALTER TABLE bundle_subscription_usages RENAME COLUMN daily_usage_count   TO daily_image_usage_count;
ALTER TABLE bundle_subscription_usages RENAME COLUMN weekly_usage_count  TO weekly_image_usage_count;
ALTER TABLE bundle_subscription_usages RENAME COLUMN monthly_usage_count TO monthly_image_usage_count;
ALTER TABLE bundle_subscription_usages
    ADD COLUMN IF NOT EXISTS daily_video_usage_count   INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS weekly_video_usage_count  INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monthly_video_usage_count INTEGER NOT NULL DEFAULT 0;

-- user_subscriptions
ALTER TABLE user_subscriptions RENAME COLUMN daily_limit_count   TO daily_image_limit_count;
ALTER TABLE user_subscriptions RENAME COLUMN weekly_limit_count  TO weekly_image_limit_count;
ALTER TABLE user_subscriptions RENAME COLUMN monthly_limit_count TO monthly_image_limit_count;
ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS daily_video_limit_count   INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS weekly_video_limit_count  INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monthly_video_limit_count INTEGER NOT NULL DEFAULT 0;

-- usage_logs
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS video_count INTEGER NOT NULL DEFAULT 0;
```

- **数据语义**: RENAME 保留旧值,历史 count 配置/用量天然成为图片维度(视频是新功能,历史值基本由图片驱动)。
- **验证**: 起一次性 `postgres:18-alpine` 容器,预建 `schema_migrations` 表,从零按序跑全部迁移(每个文件仅一次)。

## 6. 计费链路改造(post-billing,精确点)

1. `AccumulateUsage` 签名: `count int` → `imageCount, videoCount int`(或引入 `UsageDelta` 结构体)。
2. `IncrementUsage`(`bundle_usage_repo.go:81`)+ 接口(`bundle_usage_port.go:28`)同步;repo 内 `imageCount` 加到 `*_image_usage_count`、`videoCount` 加到 `*_video_usage_count`,窗口滚动逻辑不变。
3. 调用点改传分开值:
   - `gateway_service.go:8932` / `:9124` — `p.OutputCount` → `result.ImageCount, result.VideoCount`(或 `p.ImageCount, p.VideoCount`,取决于 billing params 结构)
   - `openai_gateway_service.go:6114` — 同上
4. **去掉合并**: `gateway_service.go:9529` 与 `openai_gateway_service.go:6114` 的 `OutputCount = ImageCount + VideoCount` 不再用于累加(可保留 `OutputCount` 字段仅作日志/兼容,但累加必须用分开值)。
5. billing params 结构(含 `OutputCount` 字段者)评估是否需加 `ImageCount`/`VideoCount`,或调用处直接用 `result`。

## 7. 限额校验(pre-flight,fail-open)

1. `QuotaEligibilityResult`(`bundle_usage_service.go:115-123`): `*RemainingCount` → `*RemainingImageCount`,新增 `*RemainingVideoCount`。
2. `CheckQuotaEligibility`(`bundle_usage_service.go:128`)计算逻辑按 image/video 拆分(各 `limit - usage`,0=不限才校验,沿用现状 `:160-177`)。
3. 中间件 `bundle_resolver.go:190`: 按请求路径推断 modality(`/v1/videos*`→视频、`/v1/images*`→图片、其余→不区分),只校验对应 count 维度 + USD。fail-open 保证推断不精确也安全。
4. 返回 429 时 error message 可区分"图片额度"/"视频额度"达上限(可选增强)。

## 8. 前端

- `BundlePlansView.vue:212-229`: count 那行拆成「图片(次)」+「视频(次)」两行,各 day/week/month 输入(`step=1 min=0`,placeholder"不限")。
- `BundlePlansView.vue:438-445`(提交)与 `:483-490`(默认值): 字段名同步 `*_image_limit_count` / `*_video_limit_count`。
- `frontend/src/types/bundle.ts`: quota 类型字段重命名 + 加 video。
- i18n(`zh.ts`/`en.ts` `bundles.admin`): label 区分"图片"/"视频",单位沿用 `countUnit`('次'/count);新增 `imageLimitHint`/`videoLimitHint` 等按需。
- **`GroupsView.vue` 不改**(分组层 USD 兜底保持不变)。
- 用户侧用量展示(如 `views/user/BundleUsageView.vue`): 评估是否展示图片/视频分项剩余(可选增强)。

## 9. Interface 变更影响

- `BundleUsageRepository.IncrementUsage` 签名变 → 所有 stub/mock 补全(`grep -rn "type.*Stub.*struct\|type.*Mock.*struct"` 找实现)。
- `AccumulateUsage` 签名变 → 调用方与 mock 同步。
- ent 字段重命名 → 编译期会暴露所有引用点,逐一修正。

## 10. 测试

- **后端单测**(`-tags=unit`):
  - `bundle_usage_service`: image/video 分别累加正确;`CheckQuotaEligibility` 按 modality 校验;窗口过期时 image/video 同时清零(共享窗口)。
  - 仓库 `IncrementUsage`: image/video 独立递增、窗口滚动。
- **集成测试**(`-tags=integration`): 端到端——图片请求只扣图片额度、视频请求只扣视频额度;USD 仍共同扣减。
- **迁移验证**: 从零跑全部迁移成功,列名/类型正确。
- **前端 vitest**: quota 表单 image/video 双行渲染、提交 payload 字段正确。

## 11. 风险与回滚

- **风险**: ent 字段重命名波及面广,可能漏改引用 → 靠编译器 + `grep` 兜底。
- **风险**: pre-flight modality 推断不准 → fail-open 设计天然容错,且严格性在 post-billing。
- **回滚**: 迁移 156 反向操作为 RENAME `*_image_count` → `*_count` + DROP `*_video_*`;但生产迁移一旦应用不可修改(SHA256 锁定),回滚靠 revert 代码 + 新写正向恢复迁移。

## 12. 关键文件清单

| 文件 | 改动 |
|---|---|
| `backend/ent/schema/bundle_plan_group_quota.go` | 字段重命名 + 新增 |
| `backend/ent/schema/bundle_subscription_usage.go` | 字段重命名 + 新增 |
| `backend/ent/schema/user_subscription.go` | 字段重命名 + 新增 |
| `backend/ent/schema/usage_log.go` | 新增 `video_count` |
| `backend/ent/*`(生成) | `go generate ./ent` |
| `backend/migrations/156_*.sql` | 新建 |
| `backend/internal/service/bundle_usage_service.go` | `AccumulateUsage`/`CheckQuotaEligibility`/Result 拆分 |
| `backend/internal/service/bundle_usage_port.go` | 接口签名 |
| `backend/internal/repository/bundle_usage_repo.go` | `IncrementUsage` 拆分 |
| `backend/internal/service/bundle_subscription_service.go` | 快照拆分 |
| `backend/internal/service/gateway_service.go` | 调用点传分开值、UsageLog 填 VideoCount |
| `backend/internal/service/openai_gateway_service.go` | 调用点 |
| `backend/internal/server/middleware/bundle_resolver.go` | modality 推断 + 校验 |
| `frontend/src/views/admin/bundles/BundlePlansView.vue` | 表单拆双行 |
| `frontend/src/types/bundle.ts` | 类型同步 |
| `frontend/src/i18n/locales/{zh,en}.ts` | 文案 |
