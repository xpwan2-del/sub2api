// bundle_usage_service_test.go 套餐用量服务单元测试
// 覆盖 CheckQuotaEligibility 的 USD + 次数双重判断逻辑。

package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// fakeUsageRepo 内存实现的 BundleUsageRepository，仅覆盖测试需要的子集。
// 用 mutex 模拟 DB 行锁，IncrementUsage 在锁内基于 fake 自身 window_start 判断过期，
// 与修复后 bundleUsageRepository.IncrementUsage（事务内 FOR UPDATE + rollBundleWindow）同语义，
// 从而让并发测试能在单测层验证「窗口边界不丢累加」（H1）。
type fakeUsageRepo struct {
	mu          sync.Mutex
	usage       *BundleSubscriptionUsage
	lastPattern string
}

func (f *fakeUsageRepo) GetBySubscriptionAndGroup(_ context.Context, _, _ int64, pattern string) (*BundleSubscriptionUsage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastPattern = pattern
	return f.usage, nil
}
func (f *fakeUsageRepo) Create(_ context.Context, _ *BundleSubscriptionUsage) error { return nil }
func (f *fakeUsageRepo) IncrementUsage(_ context.Context, _ int64, costUSD float64, imageCount, videoCount int, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.usage == nil {
		return nil
	}
	f.applyWindow(&f.usage.DailyUsageUSD, &f.usage.DailyImageUsageCount, &f.usage.DailyVideoUsageCount, &f.usage.DailyWindowStart, now, BundleDailyWindowDays, costUSD, imageCount, videoCount)
	f.applyWindow(&f.usage.WeeklyUsageUSD, &f.usage.WeeklyImageUsageCount, &f.usage.WeeklyVideoUsageCount, &f.usage.WeeklyWindowStart, now, BundleWeeklyWindowDays, costUSD, imageCount, videoCount)
	f.applyWindow(&f.usage.MonthlyUsageUSD, &f.usage.MonthlyImageUsageCount, &f.usage.MonthlyVideoUsageCount, &f.usage.MonthlyWindowStart, now, BundleMonthlyWindowDays, costUSD, imageCount, videoCount)
	return nil
}

// applyWindow 模拟 rollBundleWindow（自然日 0 点对齐）：过期（BundleWindowExpired）置为本次值
// 并把起点推进到当天 0 点，否则累加。与生产 bundleUsageRepository.IncrementUsage 同语义。
func (f *fakeUsageRepo) applyWindow(usd *float64, img, vid *int, start *time.Time, now time.Time, windowDays int, costUSD float64, ic, vc int) {
	if BundleWindowExpired(*start, now, windowDays) {
		*usd = costUSD
		*img = ic
		*vid = vc
		*start = BundleWindowStart(now)
		return
	}
	*usd += costUSD
	*img += ic
	*vid += vc
}

// GetOrCreateUsage 模拟自愈建行（H2）：已有则返回，否则创建空 usage。用与生产 repo 相同的
// （sub,group,pattern）定位语义，让 AccumulateUsage 自愈路径可在单测层验证。
func (f *fakeUsageRepo) GetOrCreateUsage(_ context.Context, bundleSubID, groupID int64, pattern string, now time.Time) (*BundleSubscriptionUsage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastPattern = pattern
	if f.usage != nil {
		return f.usage, nil
	}
	f.usage = &BundleSubscriptionUsage{
		ID:                   500,
		BundleSubscriptionID: bundleSubID,
		GroupID:              groupID,
		ModelPattern:         pattern,
		DailyWindowStart:     now,
		WeeklyWindowStart:    now,
		MonthlyWindowStart:   now,
	}
	return f.usage, nil
}
func (f *fakeUsageRepo) ResetDailyWindow(_ context.Context, _ int64, _ time.Time) error  { return nil }
func (f *fakeUsageRepo) ResetWeeklyWindow(_ context.Context, _ int64, _ time.Time) error { return nil }
func (f *fakeUsageRepo) ResetMonthlyWindow(_ context.Context, _ int64, _ time.Time) error {
	return nil
}
func (f *fakeUsageRepo) ListBySubscription(_ context.Context, _ int64) ([]BundleSubscriptionUsage, error) {
	return nil, nil
}
func (f *fakeUsageRepo) BatchUpdateExpiredStatus(_ context.Context) (int64, error) { return 0, nil }

// fakeSubRepo 内存实现的 BundleSubscriptionRepository 子集。
type fakeSubRepo struct {
	sub *BundleSubscription
}

func (f *fakeSubRepo) Create(_ context.Context, _ *BundleSubscription) error { return nil }
func (f *fakeSubRepo) GetByID(_ context.Context, _ int64) (*BundleSubscription, error) {
	return f.sub, nil
}
func (f *fakeSubRepo) GetActiveByUserID(_ context.Context, _ int64) ([]BundleSubscription, error) {
	return nil, nil
}
func (f *fakeSubRepo) GetByIDWithUsages(_ context.Context, _ int64) (*BundleSubscription, error) {
	return f.sub, nil
}
func (f *fakeSubRepo) List(_ context.Context, _ pagination.PaginationParams, _ *int64, _ string) ([]BundleSubscription, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (f *fakeSubRepo) UpdateStatus(_ context.Context, _ int64, _ string) error    { return nil }
func (f *fakeSubRepo) UpdateExpiry(_ context.Context, _ int64, _ time.Time) error { return nil }
func (f *fakeSubRepo) ExtendExpiryByDays(_ context.Context, _ int64, _ int) error { return nil }

// fakePlanRepo 内存实现的 BundlePlanRepository 子集。
type fakePlanRepo struct {
	plan         *BundlePlan
	getByIDCalls int
}

func (f *fakePlanRepo) Create(_ context.Context, _ *BundlePlan) error { return nil }
func (f *fakePlanRepo) Update(_ context.Context, _ *BundlePlan) error { return nil }
func (f *fakePlanRepo) GetByID(_ context.Context, _ int64) (*BundlePlan, error) {
	f.getByIDCalls++
	return f.plan, nil
}
func (f *fakePlanRepo) List(_ context.Context, _ pagination.PaginationParams, _, _ string) ([]BundlePlan, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (f *fakePlanRepo) ListForSale(_ context.Context) ([]BundlePlan, error) { return nil, nil }
func (f *fakePlanRepo) Delete(_ context.Context, _ int64) error             { return nil }

// newSvcWith 构造一个注入 fake repo 的 BundleUsageService。
func newSvcWith(plan *BundlePlan, sub *BundleSubscription, usage *BundleSubscriptionUsage) *BundleUsageService {
	return NewBundleUsageService(
		&fakeUsageRepo{usage: usage},
		&fakeSubRepo{sub: sub},
		&fakePlanRepo{plan: plan},
	)
}

func TestGetBundlePlan_ReturnsPlanWithAllGroupQuotas(t *testing.T) {
	plan := &BundlePlan{
		ID: 7,
		GroupQuotas: []BundlePlanGroupQuota{
			{GroupID: 100, GroupPlatform: PlatformOpenAI, QuotaScope: QuotaScopePlatform},
			{GroupID: 200, GroupPlatform: PlatformAnthropic, QuotaScope: QuotaScopeModel, ModelPattern: "claude-*"},
		},
	}
	sub := &BundleSubscription{ID: 3, PlanID: 7, Status: BundleStatusActive}

	svc := newSvcWith(plan, sub, nil)
	got, err := svc.GetBundlePlan(context.Background(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.ID != 7 {
		t.Fatalf("expected plan ID 7, got %+v", got)
	}
	if len(got.GroupQuotas) != 2 {
		t.Fatalf("expected 2 group quotas, got %d", len(got.GroupQuotas))
	}
}

func TestGetBundlePlan_NilSubscriptionReturnsError(t *testing.T) {
	// 订阅不存在（GetByID 返回 nil sub）时不应 panic，应返回 error。
	svc := newSvcWith(nil, nil, nil)
	got, err := svc.GetBundlePlan(context.Background(), 999)
	if err == nil {
		t.Fatal("expected error when subscription is nil")
	}
	if got != nil {
		t.Fatalf("expected nil plan on error, got %+v", got)
	}
}

func TestCheckQuotaEligibility_CountLimitExceeded(t *testing.T) {
	const groupID int64 = 100
	plan := &BundlePlan{
		GroupQuotas: []BundlePlanGroupQuota{{
			GroupID:                groupID,
			MonthlyImageLimitCount: 10,
			MonthlyLimitUSD:        100,
		}},
	}
	sub := &BundleSubscription{PlanID: 1, Status: BundleStatusActive}
	usage := &BundleSubscriptionUsage{
		MonthlyWindowStart:     timezone.StartOfDay(time.Now()), // 本月内累计有效
		MonthlyImageUsageCount: 10,
	}

	svc := newSvcWith(plan, sub, usage)
	res, err := svc.CheckQuotaEligibility(context.Background(), 1, groupID, ModalityImage)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Eligible {
		t.Fatalf("expected Eligible=false when monthly count exhausted, got true")
	}
	if res.MonthlyRemainingImageCount > 0 {
		t.Fatalf("expected MonthlyRemainingImageCount<=0, got %d", res.MonthlyRemainingImageCount)
	}
}

func TestCheckQuotaEligibility_CountZeroNoLimit(t *testing.T) {
	const groupID int64 = 100
	plan := &BundlePlan{
		GroupQuotas: []BundlePlanGroupQuota{{
			GroupID:                groupID,
			MonthlyImageLimitCount: 0, // 0 = 不限次数
			MonthlyLimitUSD:        0, // 0 = 不限额度
		}},
	}
	sub := &BundleSubscription{PlanID: 1, Status: BundleStatusActive}
	usage := &BundleSubscriptionUsage{MonthlyImageUsageCount: 999}

	svc := newSvcWith(plan, sub, usage)
	res, err := svc.CheckQuotaEligibility(context.Background(), 1, groupID, ModalityImage)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Eligible {
		t.Fatalf("expected Eligible=true when count limit is 0 (unlimited), got false")
	}
}

func TestCheckQuotaEligibility_ImageExceededBlocksImageOnly(t *testing.T) {
	const groupID int64 = 100
	plan := &BundlePlan{GroupQuotas: []BundlePlanGroupQuota{{
		GroupID:                groupID,
		MonthlyImageLimitCount: 10,
		MonthlyVideoLimitCount: 10,
		MonthlyLimitUSD:        100,
	}}}
	sub := &BundleSubscription{PlanID: 1, Status: BundleStatusActive}
	usage := &BundleSubscriptionUsage{
		MonthlyWindowStart:     timezone.StartOfDay(time.Now()), // 本月内累计有效
		MonthlyImageUsageCount: 10,
		MonthlyVideoUsageCount: 0,
	}

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
		GroupID:                groupID,
		MonthlyImageLimitCount: 1,
		MonthlyLimitUSD:        0, // USD 不限
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

// TestAccumulateUsage_RollsExpiredDailyWindow 验证窗口滚动：
// 日窗口已过期（window_start 距今 >24h）→ 本次累加前重置（count/usd 置为本次值）；
// 月窗口未过期 → 在原值上累加。修复「窗口永不重置导致限额永久锁死」的 bug。
func TestAccumulateUsage_RollsExpiredDailyWindow(t *testing.T) {
	const groupID int64 = 100
	now := time.Now()
	plan := &BundlePlan{GroupQuotas: []BundlePlanGroupQuota{{GroupID: groupID}}}
	sub := &BundleSubscription{PlanID: 1, Status: BundleStatusActive}
	usage := &BundleSubscriptionUsage{
		ID:                     50,
		BundleSubscriptionID:   1,
		GroupID:                groupID,
		DailyWindowStart:       now.Add(-25 * time.Hour), // 超过 24h → 日窗口过期
		DailyUsageUSD:          1.0,
		DailyImageUsageCount:   5,
		MonthlyWindowStart:     now.Add(-1 * time.Hour), // 1h < 30d → 月窗口未过期
		MonthlyUsageUSD:        10.0,
		MonthlyImageUsageCount: 3,
	}
	repo := &fakeUsageRepo{usage: usage}
	svc := NewBundleUsageService(repo, &fakeSubRepo{sub: sub}, &fakePlanRepo{plan: plan})

	if err := svc.AccumulateUsage(context.Background(), 1, groupID, 0.5, 2, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 日窗口过期 → 重置为本次值（而非累加历史值）。
	if usage.DailyImageUsageCount != 2 {
		t.Errorf("daily count: expired window should reset to 2, got %d", usage.DailyImageUsageCount)
	}
	if usage.DailyUsageUSD != 0.5 {
		t.Errorf("daily usd: expired window should reset to 0.5, got %v", usage.DailyUsageUSD)
	}
	// 月窗口未过期 → 在原值上累加。
	if usage.MonthlyImageUsageCount != 5 {
		t.Errorf("monthly count: active window should accumulate 3+2=5, got %d", usage.MonthlyImageUsageCount)
	}
	if usage.MonthlyUsageUSD != 10.5 {
		t.Errorf("monthly usd: active window should accumulate 10+0.5=10.5, got %v", usage.MonthlyUsageUSD)
	}
}

// TestAccumulateUsage_ConcurrentExpiredWindow_NoLostUpdate 守护 H1：窗口过期瞬间并发累加，
// 最终用量必须等于所有请求 cost 之和，不得因「Set 互相覆盖」丢失。旧实现（service 读
// window_start 快照算 roll → 无条件 Set/Add）在并发下会丢失中间累加；新实现把窗口判断
// 下推进 repo 事务（fake 用 mutex 模拟 DB 行锁），第一个请求重置、其余累加，结果精确。
func TestAccumulateUsage_ConcurrentExpiredWindow_NoLostUpdate(t *testing.T) {
	const groupID int64 = 100
	const goroutines = 50
	const perCost = 1.0

	now := time.Now()
	plan := &BundlePlan{GroupQuotas: []BundlePlanGroupQuota{{GroupID: groupID}}}
	sub := &BundleSubscription{PlanID: 1, Status: BundleStatusActive}
	// 日窗口已过期 25h → 触发滚动重置路径（H1 的危险区间）。
	usage := &BundleSubscriptionUsage{
		ID: 50, BundleSubscriptionID: 1, GroupID: groupID,
		DailyWindowStart: now.Add(-25 * time.Hour),
	}
	repo := &fakeUsageRepo{usage: usage}
	svc := NewBundleUsageService(repo, &fakeSubRepo{sub: sub}, &fakePlanRepo{plan: plan})

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_ = svc.AccumulateUsage(context.Background(), 1, groupID, perCost, 1, 0)
		}()
	}
	wg.Wait()

	// 50 次并发 × 1.0：首请求重置为 1.0，其余 49 次累加 → 50.0。旧实现会远小于此。
	want := float64(goroutines) * perCost
	if usage.DailyUsageUSD != want {
		t.Errorf("daily usd: concurrent expired-window increment lost updates, want %v, got %v", want, usage.DailyUsageUSD)
	}
	if usage.DailyImageUsageCount != goroutines {
		t.Errorf("daily image count: concurrent increment lost updates, want %d, got %d", goroutines, usage.DailyImageUsageCount)
	}
}

// TestAccumulateUsage_SelfHealsMissingUsage 守护 H2：当 (sub,group,pattern) 无 usage 行
// （典型场景——管理员编辑 plan 的 model_pattern 后路由命中新 pattern，激活快照无对应 usage），
// AccumulateUsage 应自愈创建空 usage 再计费，而非返回 ErrBundleNotFound 导致请求免费放行、计费丢失。
func TestAccumulateUsage_SelfHealsMissingUsage(t *testing.T) {
	const groupID int64 = 100
	plan := &BundlePlan{GroupQuotas: []BundlePlanGroupQuota{{GroupID: groupID, ModelPattern: "gpt-5*"}}}
	sub := &BundleSubscription{PlanID: 1, Status: BundleStatusActive}
	repo := &fakeUsageRepo{usage: nil} // 无 usage 行（H2 断链场景）

	// ctx quota 注入路由命中的新 pattern（生产路径由 bundle_resolver 注入）。
	ctxQuota := &BundlePlanGroupQuota{GroupID: groupID, ModelPattern: "gpt-5*"}
	ctx := context.WithValue(context.Background(), ctxkey.BundleResolvedQuota, ctxQuota)

	svc := NewBundleUsageService(repo, &fakeSubRepo{sub: sub}, &fakePlanRepo{plan: plan})

	// 旧实现：usage==nil → ErrBundleNotFound → gateway 忽略 → 请求免费放行。
	// 修复后：自愈创建 usage + 正常计费。
	if err := svc.AccumulateUsage(ctx, 1, groupID, 1.0, 2, 0); err != nil {
		t.Fatalf("AccumulateUsage should self-heal missing usage, got: %v", err)
	}
	if repo.usage == nil {
		t.Fatal("self-heal should create the missing usage record")
	}
	if repo.usage.ModelPattern != "gpt-5*" {
		t.Errorf("self-healed usage should use the routed pattern, got %q", repo.usage.ModelPattern)
	}
	if repo.usage.DailyUsageUSD != 1.0 {
		t.Errorf("self-healed usage should accumulate cost, got %v", repo.usage.DailyUsageUSD)
	}
	if repo.usage.DailyImageUsageCount != 2 {
		t.Errorf("self-healed usage should accumulate image count, got %d", repo.usage.DailyImageUsageCount)
	}
}

// TestAccumulateUsage_UsesModelPatternToLocateUsage 验证累加时用 matchingQuota.ModelPattern
// 定位 usage 记录（与创建 / CheckQuotaEligibility 一致），而非硬编码空串。模型级套餐
// （model_pattern 非空）场景下用空串查询会查不到记录 → ErrBundleNotFound → 计费丢失。
func TestAccumulateUsage_UsesModelPatternToLocateUsage(t *testing.T) {
	const groupID int64 = 100
	plan := &BundlePlan{GroupQuotas: []BundlePlanGroupQuota{{
		GroupID:      groupID,
		ModelPattern: "gpt-image*", // 模型级额度
	}}}
	sub := &BundleSubscription{PlanID: 1, Status: BundleStatusActive}
	usage := &BundleSubscriptionUsage{ID: 50, BundleSubscriptionID: 1, GroupID: groupID, ModelPattern: "gpt-image*"}
	repo := &fakeUsageRepo{usage: usage}
	svc := NewBundleUsageService(repo, &fakeSubRepo{sub: sub}, &fakePlanRepo{plan: plan})

	if err := svc.AccumulateUsage(context.Background(), 1, groupID, 0.5, 2, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastPattern != "gpt-image*" {
		t.Errorf("AccumulateUsage should locate usage via matchingQuota.ModelPattern=%q, got query pattern=%q", "gpt-image*", repo.lastPattern)
	}
}

// TestAccumulateUsage_ReusesResolvedQuotaFromContext 验证性能优化：当 ctx 携带路由中间件
// 已解析的 quota 时，AccumulateUsage 复用其 ModelPattern，跳过 plan 查询（消除重复 load）。
func TestAccumulateUsage_ReusesResolvedQuotaFromContext(t *testing.T) {
	const groupID int64 = 100
	plan := &BundlePlan{GroupQuotas: []BundlePlanGroupQuota{{GroupID: groupID, ModelPattern: "gpt-image*"}}}
	sub := &BundleSubscription{PlanID: 1, Status: BundleStatusActive}
	usage := &BundleSubscriptionUsage{ID: 50, GroupID: groupID, ModelPattern: "gpt-image*"}
	repo := &fakeUsageRepo{usage: usage}
	planRepo := &fakePlanRepo{plan: plan}
	svc := NewBundleUsageService(repo, &fakeSubRepo{sub: sub}, planRepo)

	// ctx 携带路由中间件解析的 quota（生产路径由 bundle_resolver 注入）。
	ctxQuota := &BundlePlanGroupQuota{GroupID: groupID, ModelPattern: "gpt-image*"}
	ctx := context.WithValue(context.Background(), ctxkey.BundleResolvedQuota, ctxQuota)

	if err := svc.AccumulateUsage(ctx, 1, groupID, 0.5, 2, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if planRepo.getByIDCalls != 0 {
		t.Errorf("should skip plan load when ctx carries resolved quota, got %d GetByID calls", planRepo.getByIDCalls)
	}
	if repo.lastPattern != "gpt-image*" {
		t.Errorf("pattern should come from ctx quota, got %q", repo.lastPattern)
	}
}

// TestAccumulateUsage_FallsBackToPlanLoadWithoutCtxQuota 验证未注入 ctx quota 时
// （测试 / 直接调用）仍 fallback 到 resolveMatchingQuota（向后兼容）。
func TestAccumulateUsage_FallsBackToPlanLoadWithoutCtxQuota(t *testing.T) {
	const groupID int64 = 100
	plan := &BundlePlan{GroupQuotas: []BundlePlanGroupQuota{{GroupID: groupID, ModelPattern: "gpt-image*"}}}
	sub := &BundleSubscription{PlanID: 1, Status: BundleStatusActive}
	usage := &BundleSubscriptionUsage{ID: 50, GroupID: groupID, ModelPattern: "gpt-image*"}
	repo := &fakeUsageRepo{usage: usage}
	planRepo := &fakePlanRepo{plan: plan}
	svc := NewBundleUsageService(repo, &fakeSubRepo{sub: sub}, planRepo)

	// 无 ctx quota → fallback 解析 plan。
	if err := svc.AccumulateUsage(context.Background(), 1, groupID, 0.5, 2, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if planRepo.getByIDCalls != 1 {
		t.Errorf("should load plan once when ctx has no resolved quota, got %d GetByID calls", planRepo.getByIDCalls)
	}
	if repo.lastPattern != "gpt-image*" {
		t.Errorf("pattern should come from loaded plan, got %q", repo.lastPattern)
	}
}

// TestAccumulateUsage_SplitsImageAndVideoCounts 验证按次累加将图片/视频分别写入各自的计数维度，
// 而非合并为单一 count。图片/视频独立限额（视频更稀缺）要求两维度分别累加，否则一个维度会用尽另一维度的配额。
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
	if usage.WeeklyImageUsageCount != 3 {
		t.Errorf("weekly image count: want 3, got %d", usage.WeeklyImageUsageCount)
	}
	if usage.WeeklyVideoUsageCount != 1 {
		t.Errorf("weekly video count: want 1, got %d", usage.WeeklyVideoUsageCount)
	}
	if usage.MonthlyImageUsageCount != 3 {
		t.Errorf("monthly image count: want 3, got %d", usage.MonthlyImageUsageCount)
	}
	if usage.MonthlyVideoUsageCount != 1 {
		t.Errorf("monthly video count: want 1, got %d", usage.MonthlyVideoUsageCount)
	}
}

// TestCheckQuotaEligibility_DailyQuotaResetsNextDay 守护死锁回归：昨天已用满日限额，
// 今天（跨过 0 点）读路径必须把昨天的累计归零、恢复额度并放行（Eligible=true）。
// 旧实现读路径不认窗口过期，直接拿昨天满值判定 → Eligible=false → 请求被 pre-flight 拒绝 →
// 永远进不到写路径 → 窗口永不重置，形成自锁。读时归零（rolledBundleUsage）解除该死锁。
func TestCheckQuotaEligibility_DailyQuotaResetsNextDay(t *testing.T) {
	const groupID int64 = 100
	plan := &BundlePlan{GroupQuotas: []BundlePlanGroupQuota{{
		GroupID:       groupID,
		DailyLimitUSD: 10,
	}}}
	sub := &BundleSubscription{PlanID: 1, Status: BundleStatusActive}
	// 昨天已用满日限额（10/10），window_start 在昨天 → 今天应判日窗口过期。
	usage := &BundleSubscriptionUsage{
		DailyWindowStart: time.Now().AddDate(0, 0, -1),
		DailyUsageUSD:    10.0,
	}

	svc := newSvcWith(plan, sub, usage)
	res, err := svc.CheckQuotaEligibility(context.Background(), 1, groupID, ModalityAny)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Eligible {
		t.Fatalf("daily quota must reset next day: expected Eligible=true, got false (deadlock regression)")
	}
	if res.DailyRemaining != 10.0 {
		t.Fatalf("daily remaining should be full 10 after reset, got %v", res.DailyRemaining)
	}
}

// TestCheckQuotaEligibility_BlocksWhenSameDayQuotaExhausted 互补回归：今天的用量仍超额时
// 必须照常拦截（读时归零不能误把今天有效累计也清掉）。window_start 在今天、用量达上限 → Eligible=false。
func TestCheckQuotaEligibility_BlocksWhenSameDayQuotaExhausted(t *testing.T) {
	const groupID int64 = 100
	today0 := timezone.StartOfDay(time.Now())
	plan := &BundlePlan{GroupQuotas: []BundlePlanGroupQuota{{
		GroupID:       groupID,
		DailyLimitUSD: 10,
	}}}
	sub := &BundleSubscription{PlanID: 1, Status: BundleStatusActive}
	usage := &BundleSubscriptionUsage{
		DailyWindowStart: today0, // 今天 → 日窗口未过期
		DailyUsageUSD:    10.0,   // 今天已用满
	}

	svc := newSvcWith(plan, sub, usage)
	res, err := svc.CheckQuotaEligibility(context.Background(), 1, groupID, ModalityAny)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Eligible {
		t.Fatalf("same-day exhausted quota must be blocked, got Eligible=true")
	}
}
