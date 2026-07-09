# 图片/视频分开限流 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把套餐次数限额从单一 `OutputCount` 拆成「图片」「视频」两条独立轨道(各 day/week/month),使运营可对图片和视频分别设限。

**Architecture:** 落点在套餐计划层 `BundlePlanGroupQuota`。现有 `*_limit_count`/`*_usage_count` 经 SQL `RENAME` 复用为图片维度(`*_image_*`),新增视频维度(`*_video_*`)。计费链路去掉 `OutputCount = ImageCount + VideoCount` 合并,改为分别累加;pre-flight 校验按请求路径推断 modality(fail-open 容错)。

**Tech Stack:** Go 1.26.4 / Ent ORM / golangci-lint v2.9(depguard);Vue 3 + TS + pnpm;PostgreSQL 迁移(`backend/migrations/NNN.sql`)。

**Spec:** `docs/IMAGE_VIDEO_SPLIT_QUOTA_DESIGN.md`

## Global Constraints

- 后端代码在 `backend/`,前端在 `frontend/`。所有 `cd backend &&` / `cd frontend &&` 前缀不可省。
- **必须 pnpm**,禁止 npm。
- ent schema 改动后必须 `cd backend && make generate`(= `go generate ./ent && go generate ./cmd/server`)并提交生成的 `ent/` 代码。
- **必须同步手写迁移 SQL**:`backend/migrations/NNN_*.sql`,`NNN` 取最新序号 +1;文件头 `SET LOCAL lock_timeout = '5s'; SET LOCAL statement_timeout = '10min';`;迁移 SHA256 锁定,一旦应用不可修改。
- count 字段精度:`INTEGER NOT NULL DEFAULT 0`(0 = 不限),对齐现有 154/155 风格。
- interface 签名变更后,所有 stub/mock 必须补全(搜索 `type.*Stub.*struct` / `type.*Mock.*struct` 及现有 fake 实现)。
- golangci-lint depguard:service 不得 import `repository`/`gorm`/`redis`。
- 提交信息末尾加 `Co-Authored-By: Claude <noreply@anthropic.com>`。

## File Structure

| 文件 | 责任 | 改动类型 |
|---|---|---|
| `backend/ent/schema/bundle_plan_group_quota.go` | 限额配置字段 | 重命名+新增 |
| `backend/ent/schema/bundle_subscription_usage.go` | 用量跟踪字段 | 重命名+新增 |
| `backend/ent/schema/user_subscription.go` | 限额快照字段 | 重命名+新增 |
| `backend/ent/schema/usage_log.go` | 明细字段 | 新增 `video_count` |
| `backend/ent/**`(生成) | ent ORM 代码 | `make generate` |
| `backend/migrations/156_*.sql` | 列重命名+新增 | 新建 |
| `backend/internal/service/bundle_usage_port.go` | `BundleUsageRepository` 接口 | 签名 |
| `backend/internal/repository/bundle_usage_repo.go` | `IncrementUsage`/Reset 实现 | 签名+实现 |
| `backend/internal/service/bundle_usage_service.go` | `AccumulateUsage`/`CheckQuotaEligibility`/Result/modality | 签名+逻辑 |
| `backend/internal/service/gateway_service.go` | `postUsageBillingParams`/调用点/UsageLog | 字段+调用 |
| `backend/internal/service/openai_gateway_service.go` | 调用点 | 调用 |
| `backend/internal/service/bundle_subscription_service.go` | 激活快照 | 字段复制 |
| `backend/internal/server/middleware/bundle_resolver.go` | pre-flight modality 推断 | 新增逻辑 |
| `backend/internal/service/bundle_models.go` | DTO 映射 | 字段名 |
| `frontend/src/types/bundle.ts` | quota 类型 | 字段 |
| `frontend/src/views/admin/bundles/BundlePlansView.vue` | count 表单拆双行 | 模板+脚本 |
| `frontend/src/i18n/locales/{zh,en}.ts` | 文案 | 新增 key |

---

### Task 1: 数据模型重命名落地(迁移 + ent schema + 全量编译修复)

**目标:** 让 `*_image_*` 字段就位(语义=原合并 count),`*_video_*` 字段就位(默认 0,未使用),`usage_logs.video_count` 就位。本 task 后项目编译通过、迁移可跑、**功能不变**(video 字段未被任何代码读写)。

**Files:**
- Create: `backend/migrations/156_bundle_image_video_split_quota.sql`
- Modify: `backend/ent/schema/bundle_plan_group_quota.go:34-39`
- Modify: `backend/ent/schema/bundle_subscription_usage.go:35-43`
- Modify: `backend/ent/schema/user_subscription.go:85-90`
- Modify: `backend/ent/schema/usage_log.go:131-132`
- Modify(generate 产物): `backend/ent/**`

**Interfaces:**
- Produces: ent 生成的字段方法 `DailyImageLimitCount` / `DailyVideoLimitCount` / `DailyImageUsageCount` / `DailyVideoUsageCount` / `VideoCount`(UsageLog),供后续 task 使用。

- [ ] **Step 1: 写迁移 156**

Create `backend/migrations/156_bundle_image_video_split_quota.sql`:

```sql
-- 156_bundle_image_video_split_quota.sql
-- 套餐次数限额拆分图片/视频:现有 count 复用为图片(RENAME 保留旧值),新增视频。
-- 迁移 SHA256 锁定只跑一次,RENAME 非幂等可接受。
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

-- bundle_plan_group_quotas (限额配置)
ALTER TABLE bundle_plan_group_quotas RENAME COLUMN daily_limit_count   TO daily_image_limit_count;
ALTER TABLE bundle_plan_group_quotas RENAME COLUMN weekly_limit_count  TO weekly_image_limit_count;
ALTER TABLE bundle_plan_group_quotas RENAME COLUMN monthly_limit_count TO monthly_image_limit_count;
ALTER TABLE bundle_plan_group_quotas
    ADD COLUMN IF NOT EXISTS daily_video_limit_count   INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS weekly_video_limit_count  INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monthly_video_limit_count INTEGER NOT NULL DEFAULT 0;

-- bundle_subscription_usages (用量跟踪)
ALTER TABLE bundle_subscription_usages RENAME COLUMN daily_usage_count   TO daily_image_usage_count;
ALTER TABLE bundle_subscription_usages RENAME COLUMN weekly_usage_count  TO weekly_image_usage_count;
ALTER TABLE bundle_subscription_usages RENAME COLUMN monthly_usage_count TO monthly_image_usage_count;
ALTER TABLE bundle_subscription_usages
    ADD COLUMN IF NOT EXISTS daily_video_usage_count   INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS weekly_video_usage_count  INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monthly_video_usage_count INTEGER NOT NULL DEFAULT 0;

-- user_subscriptions (限额快照)
ALTER TABLE user_subscriptions RENAME COLUMN daily_limit_count   TO daily_image_limit_count;
ALTER TABLE user_subscriptions RENAME COLUMN weekly_limit_count  TO weekly_image_limit_count;
ALTER TABLE user_subscriptions RENAME COLUMN monthly_limit_count TO monthly_image_limit_count;
ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS daily_video_limit_count   INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS weekly_video_limit_count  INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monthly_video_limit_count INTEGER NOT NULL DEFAULT 0;

-- usage_logs (明细)
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS video_count INTEGER NOT NULL DEFAULT 0;
```

- [ ] **Step 2: 改 `bundle_plan_group_quota.go`**

Replace `field.Int("daily_limit_count")...monthly_limit_count` 三行(原 37-39)为:

```go
		field.Int("daily_image_limit_count").Default(0).Comment("日图片次数上限（0=不限）"),
		field.Int("weekly_image_limit_count").Default(0).Comment("周图片次数上限（0=不限）"),
		field.Int("monthly_image_limit_count").Default(0).Comment("月图片次数上限（0=不限）"),
		field.Int("daily_video_limit_count").Default(0).Comment("日视频次数上限（0=不限）"),
		field.Int("weekly_video_limit_count").Default(0).Comment("周视频次数上限（0=不限）"),
		field.Int("monthly_video_limit_count").Default(0).Comment("月视频次数上限（0=不限）"),
```

- [ ] **Step 3: 改 `bundle_subscription_usage.go`**

Replace `field.Int("daily_usage_count")...monthly_usage_count` 三行(原 41-43)为:

```go
		field.Int("daily_image_usage_count").Default(0).Comment("当日已用图片次数"),
		field.Int("weekly_image_usage_count").Default(0).Comment("当周已用图片次数"),
		field.Int("monthly_image_usage_count").Default(0).Comment("当月已用图片次数"),
		field.Int("daily_video_usage_count").Default(0).Comment("当日已用视频次数"),
		field.Int("weekly_video_usage_count").Default(0).Comment("当周已用视频次数"),
		field.Int("monthly_video_usage_count").Default(0).Comment("当月已用视频次数"),
```

- [ ] **Step 4: 改 `user_subscription.go`**

Replace `field.Int("daily_limit_count")...monthly_limit_count` 三行(原 88-90)为:

```go
		field.Int("daily_image_limit_count").Default(0).Comment("独立日图片次数限额快照（0=不限）"),
		field.Int("weekly_image_limit_count").Default(0).Comment("独立周图片次数限额快照"),
		field.Int("monthly_image_limit_count").Default(0).Comment("独立月图片次数限额快照"),
		field.Int("daily_video_limit_count").Default(0).Comment("独立日视频次数限额快照（0=不限）"),
		field.Int("weekly_video_limit_count").Default(0).Comment("独立周视频次数限额快照"),
		field.Int("monthly_video_limit_count").Default(0).Comment("独立月视频次数限额快照"),
```

- [ ] **Step 5: 改 `usage_log.go`**

在 `field.Int("image_count").Default(0),`(原 131-132)之后插入:

```go
		// 视频生成段数（仅视频模型使用,与 image_count 对称）
		field.Int("video_count").
			Default(0),
```

- [ ] **Step 6: 重新生成 ent**

Run:
```bash
cd backend && make generate
```
Expected: 无错误,`ent/` 下生成的代码包含新字段方法(`DailyImageLimitCount` 等)。

- [ ] **Step 7: 批量替换旧字段引用(Go 标识符 + JSON tag)**

字段名映射(ent 方法名 / Go struct 字段 / JSON tag 三类同名):

| 旧 | 新 |
|---|---|
| `DailyLimitCount` / `daily_limit_count` | `DailyImageLimitCount` / `daily_image_limit_count` |
| `WeeklyLimitCount` / `weekly_limit_count` | `WeeklyImageLimitCount` / `weekly_image_limit_count` |
| `MonthlyLimitCount` / `monthly_limit_count` | `MonthlyImageLimitCount` / `monthly_image_limit_count` |
| `DailyUsageCount` / `daily_usage_count` | `DailyImageUsageCount` / `daily_image_usage_count` |
| `WeeklyUsageCount` / `weekly_usage_count` | `WeeklyImageUsageCount` / `weekly_image_usage_count` |
| `MonthlyUsageCount` / `monthly_usage_count` | `MonthlyImageUsageCount` / `monthly_image_usage_count` |

用 sed 批量替换后端(排除已生成的 ent 目录,它已正确):

```bash
cd backend
# Go 标识符(CamelCase)
grep -rl "DailyLimitCount\|WeeklyLimitCount\|MonthlyLimitCount" internal/ | grep -v "_test" | xargs sed -i \
  -e 's/DailyLimitCount/DailyImageLimitCount/g' \
  -e 's/WeeklyLimitCount/WeeklyImageLimitCount/g' \
  -e 's/MonthlyLimitCount/MonthlyImageLimitCount/g'
grep -rl "DailyUsageCount\|WeeklyUsageCount\|MonthlyUsageCount" internal/ | grep -v "_test" | xargs sed -i \
  -e 's/DailyUsageCount/DailyImageUsageCount/g' \
  -e 's/WeeklyUsageCount/WeeklyImageUsageCount/g' \
  -e 's/MonthlyUsageCount/MonthlyImageUsageCount/g'
# JSON tag / 字符串(snake_case)
grep -rl "daily_limit_count\|weekly_limit_count\|monthly_limit_count" internal/ | xargs sed -i \
  -e 's/daily_limit_count/daily_image_limit_count/g' \
  -e 's/weekly_limit_count/weekly_image_limit_count/g' \
  -e 's/monthly_limit_count/monthly_image_limit_count/g'
grep -rl "daily_usage_count\|weekly_usage_count\|monthly_usage_count" internal/ | xargs sed -i \
  -e 's/daily_usage_count/daily_image_usage_count/g' \
  -e 's/weekly_usage_count/weekly_image_usage_count/g' \
  -e 's/monthly_usage_count/monthly_image_usage_count/g'
```

> 注意:`OutputCount`/`ImageCount`/`VideoCount`(ForwardResult、postUsageBillingParams)本 task **不动**——它们在 Task 3 处理。`Count` 作为 `QuotaEligibilityResult` 字段后缀(`DailyRemainingCount` 等)在 Task 4 处理。若 sed 误伤,以 `go build` 报错为准修正。

- [ ] **Step 8: 编译驱动修复残留**

Run:
```bash
cd backend && go build ./...
```
Expected: 若有残留旧名引用(如 fake/mock/stub、bundle_models.go DTO、ops_*.go),编译器逐一指出。按映射表修正每一处。重复直到 `go build ./...` 通过。

补全 interface stub(若编译报缺方法):
```bash
cd backend && grep -rln "type.*Stub.*struct\|type.*Mock.*struct\|type.*fake.*struct" internal/ | xargs grep -l "UsageCount\|LimitCount"
```

- [ ] **Step 9: 现有测试仍通过(语义未变)**

Run:
```bash
cd backend && go test -tags=unit ./internal/service/... ./internal/repository/... ./internal/server/...
```
Expected: PASS。本 task 仅重命名,image 字段语义=原合并 count,行为不变。
> 若 `bundle_usage_service_test.go` 等仍引用旧字段名,按映射表修正(测试文件未被 sed 覆盖因 `-v "_test"`,需手动改:如 `MonthlyLimitCount`→`MonthlyImageLimitCount`、`MonthlyUsageCount`→`MonthlyImageUsageCount`)。

- [ ] **Step 10: 迁移验证(从零跑全部迁移)**

Run:
```bash
docker run --rm -e POSTGRES_PASSWORD=x -d --name pg156 -p 55432:5432 postgres:18-alpine
# 预建 schema_migrations 表并按序跑迁移(参考 backend/migrations/README.md 的验证流程)
# 确认 156 成功应用:列 daily_image_limit_count / daily_video_limit_count / video_count 存在
docker exec pg156 psql -U postgres -d postgres -c "\d bundle_plan_group_quotas" | grep -E "image_limit_count|video_limit_count"
docker stop pg156
```
Expected: 新列存在,旧列 `_limit_count` 已被 RENAME 不存在。

- [ ] **Step 11: golangci-lint**

Run:
```bash
cd backend && golangci-lint run ./...
```
Expected: PASS。

- [ ] **Step 12: Commit**

```bash
git add backend/ent backend/migrations/156_bundle_image_video_split_quota.sql backend/internal backend/ent/schema
git commit -m "$(cat <<'EOF'
refactor(bundles): 套餐 count 字段重命名为图片维度+新增视频维度

ent schema + 迁移 156: *_limit_count/*_usage_count RENAME 为 *_image_*,
新增 *_video_*(DEFAULT 0),usage_logs 加 video_count。功能不变,
为图片/视频分开限流铺路。

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: 仓库层 IncrementUsage/Reset 拆 image/video

**目标:** `IncrementUsage` 接收 `imageCount, videoCount`,分别累加到 `*_image_usage_count` / `*_video_usage_count`;Reset 系列同时清零两个维度。

**Files:**
- Modify: `backend/internal/service/bundle_usage_port.go:28`
- Modify: `backend/internal/repository/bundle_usage_repo.go:81-139`
- Modify: `backend/internal/service/bundle_usage_service_test.go:27-58`(fakeUsageRepo)

**Interfaces:**
- Produces: `BundleUsageRepository.IncrementUsage(ctx, id int64, costUSD float64, imageCount, videoCount int, roll WindowRoll) error`

- [ ] **Step 1: 写失败测试(改 fakeUsageRepo + 新增验证)**

Modify `bundle_usage_service_test.go` 的 `fakeUsageRepo.IncrementUsage`(原 27-58):

```go
func (f *fakeUsageRepo) IncrementUsage(_ context.Context, _ int64, costUSD float64, imageCount, videoCount int, roll WindowRoll) error {
	f.lastRoll = roll
	if f.usage == nil {
		return nil
	}
	apply := func(set func(), add func()) { // helper 内联省略,见下展开
	}
	_ = apply
	if roll.Daily {
		f.usage.DailyUsageUSD = costUSD
		f.usage.DailyImageUsageCount = imageCount
		f.usage.DailyVideoUsageCount = videoCount
		f.usage.DailyWindowStart = roll.NewDailyStart
	} else {
		f.usage.DailyUsageUSD += costUSD
		f.usage.DailyImageUsageCount += imageCount
		f.usage.DailyVideoUsageCount += videoCount
	}
	if roll.Weekly {
		f.usage.WeeklyUsageUSD = costUSD
		f.usage.WeeklyImageUsageCount = imageCount
		f.usage.WeeklyVideoUsageCount = videoCount
		f.usage.WeeklyWindowStart = roll.NewWeeklyStart
	} else {
		f.usage.WeeklyUsageUSD += costUSD
		f.usage.WeeklyImageUsageCount += imageCount
		f.usage.WeeklyVideoUsageCount += videoCount
	}
	if roll.Monthly {
		f.usage.MonthlyUsageUSD = costUSD
		f.usage.MonthlyImageUsageCount = imageCount
		f.usage.MonthlyVideoUsageCount = videoCount
		f.usage.MonthlyWindowStart = roll.NewMonthlyStart
	} else {
		f.usage.MonthlyUsageUSD += costUSD
		f.usage.MonthlyImageUsageCount += imageCount
		f.usage.MonthlyVideoUsageCount += videoCount
	}
	return nil
}
```
(删除上面 `apply` 占位 helper 及 `_ = apply` 两行——此处为说明结构,实际直接写三个 if/else。)

新增测试验证 image/video 分别累加:

```go
func TestAccumulateUsage_SplitsImageAndVideoCounts(t *testing.T) {
	const groupID int64 = 100
	plan := &BundlePlan{GroupQuotas: []BundlePlanGroupQuota{{GroupID: groupID}}}
	sub := &BundleSubscription{PlanID: 1, Status: BundleStatusActive}
	usage := &BundleSubscriptionUsage{ID: 50, BundleSubscriptionID: 1, GroupID: groupID}
	repo := &fakeUsageRepo{usage: usage}
	svc := NewBundleUsageService(repo, &fakeSubRepo{sub: sub}, &fakePlanRepo{plan: plan})

	// 本次产出 3 张图 + 1 段视频
	if err := svc.AccumulateUsage(context.Background(), 1, groupID, 1.0, 3, 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage.DailyImageUsageCount != 3 {
		t.Errorf("daily image count: want 3, got %d", usage.DailyImageUsageCount)
	}
	if usage.DailyVideoUsageCount != 1 {
		t.Errorf("daily video count: want 1, got %d", usage.DailyVideoUsageCount)
	}
}
```

> 现有测试 `TestAccumulateUsage_*` 调用 `svc.AccumulateUsage(ctx, 1, groupID, 0.5, 2)` 只传 5 参。本 task 签名变为 6 参(imageCount, videoCount)。把现有 4 处调用改成 `svc.AccumulateUsage(ctx, 1, groupID, 0.5, 2, 0)`(原 count=2 视为图片,video=0)。

- [ ] **Step 2: 运行测试,确认失败(签名未改)**

Run: `cd backend && go test -tags=unit ./internal/service/ -run TestAccumulateUsage_SplitsImageAndVideoCounts -v`
Expected: 编译失败(`AccumulateUsage` 仍接收 1 个 count 参数)。

- [ ] **Step 3: 改接口 `bundle_usage_port.go`**

Replace 行 28:
```go
	IncrementUsage(ctx context.Context, id int64, costUSD float64, imageCount, videoCount int, roll WindowRoll) error
```

- [ ] **Step 4: 改实现 `bundle_usage_repo.go`**

Replace `IncrementUsage` 函数体(原 81-103):

```go
func (r *bundleUsageRepository) IncrementUsage(ctx context.Context, id int64, costUSD float64, imageCount, videoCount int, roll service.WindowRoll) error {
	client := clientFromContext(ctx, r.client)

	update := client.BundleSubscriptionUsage.UpdateOneID(id)
	if roll.Daily {
		update.SetDailyUsageUsd(costUSD).SetDailyImageUsageCount(imageCount).SetDailyVideoUsageCount(videoCount).SetDailyWindowStart(roll.NewDailyStart)
	} else {
		update.AddDailyUsageUsd(costUSD).AddDailyImageUsageCount(imageCount).AddDailyVideoUsageCount(videoCount)
	}
	if roll.Weekly {
		update.SetWeeklyUsageUsd(costUSD).SetWeeklyImageUsageCount(imageCount).SetWeeklyVideoUsageCount(videoCount).SetWeeklyWindowStart(roll.NewWeeklyStart)
	} else {
		update.AddWeeklyUsageUsd(costUSD).AddWeeklyImageUsageCount(imageCount).AddWeeklyVideoUsageCount(videoCount)
	}
	if roll.Monthly {
		update.SetMonthlyUsageUsd(costUSD).SetMonthlyImageUsageCount(imageCount).SetMonthlyVideoUsageCount(videoCount).SetMonthlyWindowStart(roll.NewMonthlyStart)
	} else {
		update.AddMonthlyUsageUsd(costUSD).AddMonthlyImageUsageCount(imageCount).AddMonthlyVideoUsageCount(videoCount)
	}

	_, err := update.Save(ctx)
	return translatePersistenceError(err, nil, nil)
}
```

改 Reset 三个方法(原 105-139),每个在 `SetNillableXxxUsageCount(0)` 旁加 video。例如 `ResetDailyWindow`:

```go
func (r *bundleUsageRepository) ResetDailyWindow(ctx context.Context, id int64, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.BundleSubscriptionUsage.UpdateOneID(id).
		SetDailyUsageUsd(0).
		SetDailyImageUsageCount(0).
		SetDailyVideoUsageCount(0).
		SetDailyWindowStart(newWindowStart).
		Save(ctx)
	return translatePersistenceError(err, nil, nil)
}
```
`ResetWeeklyWindow` / `ResetMonthlyWindow` 同理(替换对应的 Weekly/Monthly 字段)。

- [ ] **Step 5: 运行测试,确认通过**

Run: `cd backend && go test -tags=unit ./internal/service/ -run TestAccumulateUsage -v`
Expected: PASS(含新测试与改过的 4 个旧测试)。

> 此时 `AccumulateUsage` 还没改签名(service 层),测试会编译失败。**本 task 只改 repo + port + fake**;service 层 `AccumulateUsage` 仍传单 `count` 给 `IncrementUsage` → 编译断。因此本 task 必须连带改 `bundle_usage_service.go` 的 `AccumulateUsage` 签名与内部调用——见 Step 6。

- [ ] **Step 6: 改 `bundle_usage_service.go` 的 AccumulateUsage(签名+内部调用)**

Replace `AccumulateUsage` 签名与 `IncrementUsage` 调用(原 73、107):

```go
func (s *BundleUsageService) AccumulateUsage(ctx context.Context, bundleSubID, groupID int64, costUSD float64, imageCount, videoCount int) error {
```
原 107 行:
```go
	if err := s.usageRepo.IncrementUsage(ctx, usage.ID, costUSD, imageCount, videoCount, roll); err != nil {
```

- [ ] **Step 7: 编译 + 测试**

Run: `cd backend && go build ./... && go test -tags=unit ./internal/service/ ./internal/repository/`
Expected: 编译通过(repository 层完成);但 gateway 调用点(8932/9124)仍传单 `p.OutputCount` 给 `AccumulateUsage` → gateway 编译断。**这由 Task 3 修复**。

> ⚠️ Task 2 单独无法让 `go build ./...` 通过(service 签名变了,gateway 调用未跟上)。因此 **Task 2 与 Task 3 必须连续完成后再 commit**,或合并执行。下方 Task 3 完成后统一验证编译。

- [ ] **Step 8: Commit(与 Task 3 合并提交,见 Task 3 Step 6)**

---

### Task 3: 计费链路调用点拆分(postUsageBillingParams + gateway 调用)

**目标:** `postUsageBillingParams.OutputCount` 拆成 `ImageCount`+`VideoCount`;两处赋值点分别传 `result.ImageCount`/`result.VideoCount`;两处 `AccumulateUsage` 调用传分开值。完成后 `go build ./...` 通过。

**Files:**
- Modify: `backend/internal/service/gateway_service.go:8861-8862, 8909, 8932, 9527-9529, 9717`
- Modify: `backend/internal/service/openai_gateway_service.go:6114`

**Interfaces:**
- Consumes: Task 2 的 `AccumulateUsage(..., imageCount, videoCount int)` 签名。
- Produces: `postUsageBillingParams.ImageCount` / `VideoCount` 字段。

- [ ] **Step 1: 改 `postUsageBillingParams` 结构体**

Replace `gateway_service.go:8861-8862`:
```go
	// ImageCount/VideoCount 媒体产出数,用于套餐按次累加(图片张数 / 视频段数)。
	ImageCount int
	VideoCount int
```
(删除原 `OutputCount int` 字段。)

- [ ] **Step 2: 改 `shouldAccumulateBundleUsage`**

Replace `gateway_service.go:8909` 的条件:
```go
		p.Subscription.GroupID > 0 && (p.Cost.ActualCost > 0 || p.ImageCount > 0 || p.VideoCount > 0)
```

- [ ] **Step 3: 改两处 AccumulateUsage 调用**

`gateway_service.go:8932`:
```go
			if err := deps.bundleUsageService.AccumulateUsage(billingCtx, *p.Subscription.BundleSubscriptionID, p.Subscription.GroupID, cost.ActualCost, p.ImageCount, p.VideoCount); err != nil {
```
`gateway_service.go:9124`:
```go
			if err := deps.bundleUsageService.AccumulateUsage(ctx, *p.Subscription.BundleSubscriptionID, p.Subscription.GroupID, p.Cost.ActualCost, p.ImageCount, p.VideoCount); err != nil {
```

- [ ] **Step 4: 改两处赋值点(去掉合并)**

`gateway_service.go:9527-9529`(替换注释+赋值):
```go
		// 媒体产出数:图片张数 + 视频段数分开传递。视频通常经 OpenAI 网关(/videos),
		// 通用网关亦支持:视频转发层(Veo 等走 Gemini 路径时)填充 ForwardResult.VideoCount。
		ImageCount: result.ImageCount,
		VideoCount: result.VideoCount,
```
`openai_gateway_service.go:6114`:
```go
			ImageCount:           result.ImageCount,
			VideoCount:           result.VideoCount,
```

- [ ] **Step 5: 编译 + 全量单测**

Run:
```bash
cd backend && go build ./... && go test -tags=unit ./internal/service/ ./internal/repository/ ./internal/server/
```
Expected: BUILD PASS + 测试 PASS。

- [ ] **Step 6: Commit(Task 2+3 合并)**

```bash
git add backend/internal/service/bundle_usage_service.go backend/internal/service/bundle_usage_port.go backend/internal/repository/bundle_usage_repo.go backend/internal/service/bundle_usage_service_test.go backend/internal/service/gateway_service.go backend/internal/service/openai_gateway_service.go backend/internal/repository/bundle_integration_test.go
git commit -m "$(cat <<'EOF'
feat(billing): 套餐按次累加拆分图片/视频

AccumulateUsage/IncrementUsage 接收 imageCount+videoCount,
postUsageBillingParams 用 ImageCount/VideoCount 替代 OutputCount,
去掉 ImageCount+VideoCount 合并点。窗口滚动对两维度对称生效。

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: CheckQuotaEligibility 拆 image/video + pre-flight modality 推断

**目标:** `QuotaEligibilityResult` 暴露 image/video 各自剩余;`CheckQuotaEligibility` 接收 modality,只校验对应 count 维度 + USD;`bundle_resolver` 中间件按请求路径推断 modality 并传入。fail-open 保证安全。

**Files:**
- Modify: `backend/internal/service/bundle_usage_service.go:113-190`
- Modify: `backend/internal/service/bundle_usage_port.go`(若 modality 类型放此)
- Modify: `backend/internal/server/middleware/bundle_resolver.go:184-204`
- Modify: `backend/internal/service/bundle_usage_service_test.go`
- Modify: `backend/internal/server/middleware/bundle_resolver_test.go`

**Interfaces:**
- Produces: `type UsageModality string`(`ModalityImage`/`ModalityVideo`/`ModalityAny`);`CheckQuotaEligibility(ctx, bundleSubID, groupID int64, modality UsageModality)`;`QuotaEligibilityResult.{Daily,Weekly,Monthly}RemainingImageCount` / `...VideoCount`。

- [ ] **Step 1: 写失败测试**

在 `bundle_usage_service_test.go` 新增:

```go
func TestCheckQuotaEligibility_ImageExceededBlocksImageOnly(t *testing.T) {
	const groupID int64 = 100
	plan := &BundlePlan{GroupQuotas: []BundlePlanGroupQuota{{
		GroupID:               groupID,
		MonthlyImageLimitCount: 10,
		MonthlyVideoLimitCount: 10,
		MonthlyLimitUSD:       100,
	}}}
	sub := &BundleSubscription{PlanID: 1, Status: BundleStatusActive}
	usage := &BundleSubscriptionUsage{MonthlyImageUsageCount: 10, MonthlyVideoUsageCount: 0}

	svc := newSvcWith(plan, sub, usage)
	// 图片请求:图片额度耗尽 → 不可用
	res, _ := svc.CheckQuotaEligibility(context.Background(), 1, groupID, ModalityImage)
	if res.Eligible {
		t.Fatalf("image request should be blocked when image quota exhausted")
	}
	// 视频请求:视频额度未耗尽 → 可用(图片耗尽不应波及视频)
	res2, _ := svc.CheckQuotaEligibility(context.Background(), 1, groupID, ModalityVideo)
	if !res2.Eligible {
		t.Fatalf("video request should be eligible when only image quota exhausted")
	}
}

func TestCheckQuotaEligibility_ModalityAnySkipsCountCheck(t *testing.T) {
	const groupID int64 = 100
	plan := &BundlePlan{GroupQuotas: []BundlePlanGroupQuota{{
		GroupID:               groupID,
		MonthlyImageLimitCount: 1,
		MonthlyLimitUSD:       0, // USD 不限
	}}}
	sub := &BundleSubscription{PlanID: 1, Status: BundleStatusActive}
	usage := &BundleSubscriptionUsage{MonthlyImageUsageCount: 99} // 图片已超额

	svc := newSvcWith(plan, sub, usage)
	// 文本请求(ModalityAny):不应被图片 count 限额误拒
	res, _ := svc.CheckQuotaEligibility(context.Background(), 1, groupID, ModalityAny)
	if !res.Eligible {
		t.Fatalf("text request (ModalityAny) must not be blocked by image count limit")
	}
}
```

改 `TestCheckQuotaEligibility_CountLimitExceeded` / `_CountZeroNoLimit`:调用加第 4 参 `ModalityImage`,字段 `MonthlyRemainingCount`→`MonthlyRemainingImageCount`,`MonthlyLimitCount`→`MonthlyImageLimitCount`,`MonthlyUsageCount`→`MonthlyImageUsageCount`。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend && go test -tags=unit ./internal/service/ -run TestCheckQuotaEligibility -v`
Expected: 编译失败(modality 类型/新字段不存在)。

- [ ] **Step 3: 加 modality 类型 + 改 Result + 改 CheckQuotaEligibility**

在 `bundle_usage_service.go`(`QuotaEligibilityResult` 定义之前)加:

```go
// UsageModality 标识本次请求消耗的媒体维度,决定 pre-flight 校验哪些 count 限额。
type UsageModality string

const (
	ModalityImage UsageModality = "image" // 图片请求:校验 image count
	ModalityVideo UsageModality = "video" // 视频请求:校验 video count
	ModalityAny   UsageModality = "any"   // 文本/不确定:仅校验 USD,不校验 count
)
```

Replace `QuotaEligibilityResult`(原 115-123):
```go
type QuotaEligibilityResult struct {
	Eligible                   bool
	DailyRemaining             float64
	WeeklyRemaining            float64
	MonthlyRemaining           float64
	DailyRemainingImageCount   int
	WeeklyRemainingImageCount  int
	MonthlyRemainingImageCount int
	DailyRemainingVideoCount   int
	WeeklyRemainingVideoCount  int
	MonthlyRemainingVideoCount int
}
```

Replace `CheckQuotaEligibility` 签名与校验逻辑(原 128-190)。核心:计算所有 remaining(供展示),但 `Eligible` 按 modality 决定校验哪些 count:

```go
func (s *BundleUsageService) CheckQuotaEligibility(ctx context.Context, bundleSubID, groupID int64, modality UsageModality) (*QuotaEligibilityResult, error) {
	bundleSub, matchingQuota, err := s.resolveMatchingQuota(ctx, bundleSubID, groupID)
	if err != nil {
		return nil, err
	}
	if bundleSub.Status != BundleStatusActive {
		return nil, ErrBundleExpired
	}
	if matchingQuota == nil {
		return nil, ErrBundleGroupQuotaExceeded
	}

	usage, err := s.usageRepo.GetBySubscriptionAndGroup(ctx, bundleSubID, groupID, matchingQuota.ModelPattern)
	if err != nil {
		return nil, fmt.Errorf("load bundle usage: %w", err)
	}

	result := &QuotaEligibilityResult{Eligible: true}

	if usage != nil {
		result.DailyRemaining = matchingQuota.DailyLimitUSD - usage.DailyUsageUSD
		result.WeeklyRemaining = matchingQuota.WeeklyLimitUSD - usage.WeeklyUsageUSD
		result.MonthlyRemaining = matchingQuota.MonthlyLimitUSD - usage.MonthlyUsageUSD
		result.DailyRemainingImageCount = matchingQuota.DailyImageLimitCount - usage.DailyImageUsageCount
		result.WeeklyRemainingImageCount = matchingQuota.WeeklyImageLimitCount - usage.WeeklyImageUsageCount
		result.MonthlyRemainingImageCount = matchingQuota.MonthlyImageLimitCount - usage.MonthlyImageUsageCount
		result.DailyRemainingVideoCount = matchingQuota.DailyVideoLimitCount - usage.DailyVideoUsageCount
		result.WeeklyRemainingVideoCount = matchingQuota.WeeklyVideoLimitCount - usage.WeeklyVideoUsageCount
		result.MonthlyRemainingVideoCount = matchingQuota.MonthlyVideoLimitCount - usage.MonthlyVideoUsageCount
	} else {
		result.DailyRemaining = matchingQuota.DailyLimitUSD
		result.WeeklyRemaining = matchingQuota.WeeklyLimitUSD
		result.MonthlyRemaining = matchingQuota.MonthlyLimitUSD
		result.DailyRemainingImageCount = matchingQuota.DailyImageLimitCount
		result.WeeklyRemainingImageCount = matchingQuota.WeeklyImageLimitCount
		result.MonthlyRemainingImageCount = matchingQuota.MonthlyImageLimitCount
		result.DailyRemainingVideoCount = matchingQuota.DailyVideoLimitCount
		result.WeeklyRemainingVideoCount = matchingQuota.WeeklyVideoLimitCount
		result.MonthlyRemainingVideoCount = matchingQuota.MonthlyVideoLimitCount
	}

	// USD 维度:所有请求都校验(0=不限,>0 才校验)。
	if matchingQuota.DailyLimitUSD > 0 && result.DailyRemaining <= 0 {
		result.Eligible = false
	}
	if matchingQuota.WeeklyLimitUSD > 0 && result.WeeklyRemaining <= 0 {
		result.Eligible = false
	}
	if matchingQuota.MonthlyLimitUSD > 0 && result.MonthlyRemaining <= 0 {
		result.Eligible = false
	}
	// count 维度:仅按 modality 校验对应轨道。ModalityAny 不校验 count(文本请求不被媒体限额误拒)。
	checkImage := modality == ModalityImage
	checkVideo := modality == ModalityVideo
	if checkImage {
		if matchingQuota.DailyImageLimitCount > 0 && result.DailyRemainingImageCount <= 0 {
			result.Eligible = false
		}
		if matchingQuota.WeeklyImageLimitCount > 0 && result.WeeklyRemainingImageCount <= 0 {
			result.Eligible = false
		}
		if matchingQuota.MonthlyImageLimitCount > 0 && result.MonthlyRemainingImageCount <= 0 {
			result.Eligible = false
		}
	}
	if checkVideo {
		if matchingQuota.DailyVideoLimitCount > 0 && result.DailyRemainingVideoCount <= 0 {
			result.Eligible = false
		}
		if matchingQuota.WeeklyVideoLimitCount > 0 && result.WeeklyRemainingVideoCount <= 0 {
			result.Eligible = false
		}
		if matchingQuota.MonthlyVideoLimitCount > 0 && result.MonthlyRemainingVideoCount <= 0 {
			result.Eligible = false
		}
	}
	return result, nil
}
```

- [ ] **Step 4: 改 `bundle_resolver.go` 推断 modality 并传入**

在 `bundle_resolver.go:190` 调用处前加推断(中间件已有 `c *gin.Context` 与 `strings` import):

```go
		// 按请求路径粗略推断媒体维度,决定 pre-flight 校验哪条 count 轨道。
		// fail-open:推断不精确也安全(严格扣减在 post-billing)。
		path := c.Request.URL.Path
		var modality service.UsageModality
		switch {
		case strings.Contains(path, "/videos"):
			modality = service.ModalityVideo
		case strings.Contains(path, "/images"):
			modality = service.ModalityImage
		default:
			modality = service.ModalityAny
		}
		elig, qErr := m.usageSvc.CheckQuotaEligibility(c.Request.Context(), resolved.BundleSubID, resolved.GroupID, modality)
```
(原 190 行的 `elig, qErr := m.usageSvc.CheckQuotaEligibility(...)` 整体替换为上面这段。)

- [ ] **Step 5: 改 `bundle_resolver_test.go`**

搜索其中 `CheckQuotaEligibility` 调用,补第 4 参 `service.ModalityAny`(或对应 mock 期望)。若该测试用 mock `usageSvc`,同步改 mock 签名。

- [ ] **Step 6: 运行测试**

Run: `cd backend && go build ./... && go test -tags=unit ./internal/service/ ./internal/server/`
Expected: PASS。

- [ ] **Step 7: Commit**

```bash
git add backend/internal/service/bundle_usage_service.go backend/internal/service/bundle_usage_service_test.go backend/internal/server/middleware/bundle_resolver.go backend/internal/server/middleware/bundle_resolver_test.go
git commit -m "$(cat <<'EOF'
feat(billing): 套餐限额校验拆分图片/视频 + modality 推断

CheckQuotaEligibility 接收 modality,按请求路径(/images,/videos)
只校验对应 count 轨道 + USD;ModalityAny(文本)不校验 count。
fail-open 保证推断容错。

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: 激活快照拆分 + UsageLog 记录 VideoCount

**目标:** 套餐激活时把 `BundlePlanGroupQuota` 的 image/video limit 快照到 `UserSubscription`;UsageLog 写入 `VideoCount`。

**Files:**
- Modify: `backend/internal/service/bundle_subscription_service.go:145-150`
- Modify: `backend/internal/service/gateway_service.go:9717`
- Modify: `backend/internal/service/bundle_subscription_service_test.go`

**Interfaces:**
- Consumes: Task 1 生成的 `UserSubscription.{Daily,Weekly,Monthly}ImageLimitCount` / `...VideoLimitCount`、`BundlePlanGroupQuota.{...}ImageLimitCount` / `...VideoLimitCount`、`UsageLog.VideoCount`。

- [ ] **Step 1: 扩展现有快照测试(先写断言,再改实现)**

现有 `TestBundleSubscriptionService_ActivateBundle_Success`(`bundle_subscription_service_test.go:295-337`)已断言 `*ImageLimitCount` 快照(Task 1 重命名后,行 333-336 字段名应为 `DailyImageLimitCount` 等)。本 task 扩展它验证 video 维度。

(a) 在该文件的 `sampleActivePlan()` helper 里,给第一个 GroupQuota(已含 `DailyImageLimitCount: 50` 等)对称加 video limit:

```go
			DailyVideoLimitCount:   5,
			WeeklyVideoLimitCount:  25,
			MonthlyVideoLimitCount: 100,
```

(b) 在 `TestBundleSubscriptionService_ActivateBundle_Success` 断言段(336 行 `require.Equal(t, 30, ...DailyImageLimitCount)` 之后)加:

```go
	// video limit snapshotted symmetrically with image limit.
	require.Equal(t, 5, userSubRepo.createdSubs[0].DailyVideoLimitCount, "daily video limit must be snapshotted from plan quota")
	require.Equal(t, 25, userSubRepo.createdSubs[0].WeeklyVideoLimitCount)
	require.Equal(t, 100, userSubRepo.createdSubs[0].MonthlyVideoLimitCount)
```

- [ ] **Step 2: 改快照逻辑**

Replace `bundle_subscription_service.go:145-150`:

```go
			DailyLimitUSD:           gq.DailyLimitUSD,
			WeeklyLimitUSD:          gq.WeeklyLimitUSD,
			MonthlyLimitUSD:         gq.MonthlyLimitUSD,
			DailyImageLimitCount:    gq.DailyImageLimitCount,
			WeeklyImageLimitCount:   gq.WeeklyImageLimitCount,
			MonthlyImageLimitCount:  gq.MonthlyImageLimitCount,
			DailyVideoLimitCount:    gq.DailyVideoLimitCount,
			WeeklyVideoLimitCount:   gq.WeeklyVideoLimitCount,
			MonthlyVideoLimitCount:  gq.MonthlyVideoLimitCount,
```

- [ ] **Step 3: 改 UsageLog 写入**

`gateway_service.go:9717`(`ImageCount: result.ImageCount,` 之后)加:

```go
		ImageCount:            result.ImageCount,
		VideoCount:            result.VideoCount,
```

- [ ] **Step 4: 运行测试**

Run: `cd backend && go build ./... && go test -tags=unit ./internal/service/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/bundle_subscription_service.go backend/internal/service/bundle_subscription_service_test.go backend/internal/service/gateway_service.go
git commit -m "$(cat <<'EOF'
feat(bundles): 激活快照拆分图片/视频限额 + UsageLog 记录视频数

ActivateBundle 把 BundlePlanGroupQuota 的 image/video limit 快照到
UserSubscription;UsageLog 写入 VideoCount(result.VideoCount)。

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: 前端 — 套餐计划 quota 表单拆双行 + 类型 + 文案

**目标:** `BundlePlansView` 把 count 行拆成「图片(次)」+「视频(次)」两行;`types/bundle.ts` 字段同步;i18n 文案。

**Files:**
- Modify: `frontend/src/types/bundle.ts:31-44, 64-85, 89-105, 125-`(quota 相关接口)
- Modify: `frontend/src/views/admin/bundles/BundlePlansView.vue:192-229, 438-445, 483-490`
- Modify: `frontend/src/i18n/locales/zh.ts` / `en.ts`(`bundles.admin` 段)

**Interfaces:**
- Consumes: 后端 API 返回的 `*_image_limit_count` / `*_video_limit_count` / `*_image_usage_count` / `*_video_usage_count` 字段。

- [ ] **Step 1: 改 `types/bundle.ts`**

`BundlePlanGroupQuota`(原 42-44)、`BundleSubscriptionUsage`(83-85)、`BundleUsageProgress`(101-105)、`CreateGroupQuotaRequest`(125-)中,把每组三个 `*_limit_count` / `*_usage_count` 替换为 6 个:

```ts
  daily_image_limit_count: number
  weekly_image_limit_count: number
  monthly_image_limit_count: number
  daily_video_limit_count: number
  weekly_video_limit_count: number
  monthly_video_limit_count: number
```
(usage 接口对应 `*_image_usage_count` / `*_video_usage_count`。)

- [ ] **Step 2: 改 `BundlePlansView.vue` 模板**

把原 count 单行(212-229)拆成两行。替换整个 `<!-- Daily / Weekly / Monthly Count Limits (次) -->` 块为:

```html
            <!-- Daily / Weekly / Monthly Image Count Limits (图片 张) -->
            <div class="mt-2 grid grid-cols-12 gap-3 border-t border-gray-100 pt-2 dark:border-dark-700">
              <div class="col-span-3 text-xs text-gray-500 dark:text-gray-400">
                {{ t('bundles.admin.imageCountLimitHint') }}
              </div>
              <div class="col-span-4"></div>
              <div class="col-span-1">
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.admin.daily') }} ({{ t('bundles.admin.countUnit') }})</label>
                <input v-model.number="quota.daily_image_limit_count" type="number" step="1" min="0" class="input mt-1" :placeholder="t('bundles.admin.countPlaceholder')" />
              </div>
              <div class="col-span-1">
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.admin.weekly') }} ({{ t('bundles.admin.countUnit') }})</label>
                <input v-model.number="quota.weekly_image_limit_count" type="number" step="1" min="0" class="input mt-1" :placeholder="t('bundles.admin.countPlaceholder')" />
              </div>
              <div class="col-span-1">
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.admin.monthly') }} ({{ t('bundles.admin.countUnit') }})</label>
                <input v-model.number="quota.monthly_image_limit_count" type="number" step="1" min="0" class="input mt-1" :placeholder="t('bundles.admin.countPlaceholder')" />
              </div>
              <div class="col-span-2"></div>
            </div>
            <!-- Daily / Weekly / Monthly Video Count Limits (视频 段) -->
            <div class="mt-2 grid grid-cols-12 gap-3 border-t border-gray-100 pt-2 dark:border-dark-700">
              <div class="col-span-3 text-xs text-gray-500 dark:text-gray-400">
                {{ t('bundles.admin.videoCountLimitHint') }}
              </div>
              <div class="col-span-4"></div>
              <div class="col-span-1">
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.admin.daily') }} ({{ t('bundles.admin.countUnit') }})</label>
                <input v-model.number="quota.daily_video_limit_count" type="number" step="1" min="0" class="input mt-1" :placeholder="t('bundles.admin.countPlaceholder')" />
              </div>
              <div class="col-span-1">
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.admin.weekly') }} ({{ t('bundles.admin.countUnit') }})</label>
                <input v-model.number="quota.weekly_video_limit_count" type="number" step="1" min="0" class="input mt-1" :placeholder="t('bundles.admin.countPlaceholder')" />
              </div>
              <div class="col-span-1">
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.admin.monthly') }} ({{ t('bundles.admin.countUnit') }})</label>
                <input v-model.number="quota.monthly_video_limit_count" type="number" step="1" min="0" class="input mt-1" :placeholder="t('bundles.admin.countPlaceholder')" />
              </div>
              <div class="col-span-2"></div>
            </div>
```

- [ ] **Step 3: 改提交与默认值**

`BundlePlansView.vue:438-445`(提交 payload):
```ts
        daily_image_limit_count: q.daily_image_limit_count || 0,
        weekly_image_limit_count: q.weekly_image_limit_count || 0,
        monthly_image_limit_count: q.monthly_image_limit_count || 0,
        daily_video_limit_count: q.daily_video_limit_count || 0,
        weekly_video_limit_count: q.weekly_video_limit_count || 0,
        monthly_video_limit_count: q.monthly_video_limit_count || 0,
```
`BundlePlansView.vue:483-490`(新建 quota 默认值):
```ts
    daily_image_limit_count: 0,
    weekly_image_limit_count: 0,
    monthly_image_limit_count: 0,
    daily_video_limit_count: 0,
    weekly_video_limit_count: 0,
    monthly_video_limit_count: 0,
```

- [ ] **Step 4: 加 i18n 文案**

`zh.ts` `bundles.admin` 段(7733 `countPlaceholder` 附近)加:
```ts
      imageCountLimitHint: '图片次数上限（按张，0=不限）',
      videoCountLimitHint: '视频次数上限（按段，0=不限）',
```
`en.ts` 对应:
```ts
      imageCountLimitHint: 'Image count limit (per image, 0=unlimited)',
      videoCountLimitHint: 'Video count limit (per segment, 0=unlimited)',
```

- [ ] **Step 5: typecheck + lint + vitest**

Run:
```bash
cd frontend && pnpm run typecheck && pnpm run lint:check && pnpm run test:run
```
Expected: PASS。若 `BundlePlansView` 有快照测试引用旧字段,同步修正。

- [ ] **Step 6: Commit**

```bash
git add frontend/src/types/bundle.ts frontend/src/views/admin/bundles/BundlePlansView.vue frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "$(cat <<'EOF'
feat(bundles): 套餐计划额度表单拆分图片/视频次数

BundlePlansView 把单行 count 拆成图片(张)+视频(段)两行,
types/bundle.ts 与 i18n 同步新字段。

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: 全量验证

**目标:** 端到端确认图片/视频分开限流可用,CI 全绿。

- [ ] **Step 1: 后端全量测试 + lint**

```bash
cd backend && go test -tags=unit ./... && go test -tags=integration ./... && golangci-lint run ./...
```
Expected: 全 PASS。

- [ ] **Step 2: 前端关键测试**

```bash
cd frontend && pnpm run typecheck && pnpm run lint:check && pnpm run test:run
```
Expected: PASS。

- [ ] **Step 3: 端到端手测(可选,需运行环境)**

启动后端,创建套餐计划:某 group 配 `daily_image_limit_count=2`、`daily_video_limit_count=1`;用户订阅;连续发 2 次图片请求 → 第 3 次被拒(`BUNDLE_GROUP_QUOTA_EXCEEDED`);视频请求仍可用(独立额度)。

- [ ] **Step 4: 确认无残留旧字段名**

```bash
cd backend && grep -rn "DailyLimitCount\b\|DailyUsageCount\b\|OutputCount" internal/ | grep -v "_test\|//"
cd frontend && grep -rn "daily_limit_count\b\|daily_usage_count\b" src/
```
Expected: 后端无 `DailyLimitCount`/`DailyUsageCount`(应全为 `*Image*`);`OutputCount` 仅在 ForwardResult 注释或已无。前端无旧 snake_case。

---

## Self-Review

**1. Spec coverage:**
- 数据模型 4 表 → Task 1 ✓
- 迁移 156 → Task 1 Step 1 ✓
- 计费链路 AccumulateUsage/IncrementUsage/OutputCount 合并去除 → Task 2+3 ✓
- 校验 CheckQuotaEligibility + modality → Task 4 ✓
- 快照 + UsageLog video_count → Task 5 ✓
- 前端表单拆双行 → Task 6 ✓
- 测试 → 各 task TDD ✓

**2. Placeholder scan:** 已扫描,无 TBD/TODO。Task 5 Step 1 原占位已改为扩展现有 `TestBundleSubscriptionService_ActivateBundle_Success` 的具体代码(基于真实测试结构)。各步骤均含完整代码或精确 old→new 改动。

**3. Type consistency:** `UsageModality`/`ModalityImage|Video|Any` 在 Task 4 定义、Task 4 中间件使用,一致。`AccumulateUsage(..., imageCount, videoCount)` 在 Task 2 定义、Task 3 调用,一致。`postUsageBillingParams.ImageCount/VideoCount` 在 Task 3 定义并使用,一致。ent 字段名 `*ImageLimitCount`/`*VideoLimitCount`/`*ImageUsageCount`/`*VideoUsageCount` 全程一致。

**执行单元(subagent dispatch):** 因 `AccumulateUsage` 签名变更的编译依赖,Task 2 与 Task 3 必须由同一 implementer 连续完成、在 Task 3 Step 6 合并 commit、task review 基于该合并 commit 一次。dispatch 单元:
- **Unit A = Task 1**(数据模型重命名,多文件 + 编译驱动)
- **Unit B = Task 2 + Task 3**(计费链路拆分,合并 commit)
- **Unit C = Task 4**(校验 + modality)
- **Unit D = Task 5**(快照 + UsageLog)
- **Unit E = Task 6**(前端)
- **Unit F = Task 7**(全量验证)

顺序: A → B → C → D → E → F。
