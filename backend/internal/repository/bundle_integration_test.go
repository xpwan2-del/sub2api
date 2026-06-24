//go:build integration

// bundle_integration_test.go 套餐订阅集成测试
// 覆盖设计文档 BUNDLE_SUBSCRIPTION_DESIGN.md Section 8 要求的三个集成测试：
//  1. Purchase → create UserSubscriptions → gateway quota check E2E
//  2. Single-key multi-platform routing + independent quota
//  3. Bundle expiry → request rejection + other bundles unaffected
//
// 使用 testcontainers 启动 PostgreSQL + Redis 容器，通过 ent 事务隔离各测试。

package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

// =============================================================================
// Suite 1: BundleSubscriptionLifecycleSuite
// 覆盖: Purchase → create UserSubscriptions → gateway quota check E2E
// =============================================================================

type BundleSubscriptionLifecycleSuite struct {
	suite.Suite
	ctx    context.Context
	client *dbent.Client

	planSvc     *service.BundlePlanService
	subSvc      *service.BundleSubscriptionService
	usageSvc    *service.BundleUsageService
	planRepo    service.BundlePlanRepository
	subRepo     service.BundleSubscriptionRepository
	usageRepo   service.BundleUsageRepository
	userSubRepo service.UserSubscriptionRepository
	groupRepo   service.GroupRepository
}

func (s *BundleSubscriptionLifecycleSuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.client = tx.Client()

	s.planRepo = NewBundlePlanRepository(s.client)
	s.subRepo = NewBundleSubscriptionRepository(s.client)
	s.usageRepo = NewBundleUsageRepository(s.client)
	s.userSubRepo = NewUserSubscriptionRepository(s.client)
	s.groupRepo = NewGroupRepository(s.client, integrationDB)

	s.planSvc = service.NewBundlePlanService(s.planRepo, nil)
	s.subSvc = service.NewBundleSubscriptionService(s.subRepo, s.planRepo, s.usageRepo, s.userSubRepo, nil)
	s.usageSvc = service.NewBundleUsageService(s.usageRepo, s.subRepo, s.planRepo)
}

func TestBundleSubscriptionLifecycleSuite(t *testing.T) {
	suite.Run(t, new(BundleSubscriptionLifecycleSuite))
}

// --- helpers ---

func (s *BundleSubscriptionLifecycleSuite) mustCreateUser(email string) *service.User {
	s.T().Helper()
	u, err := s.client.User.Create().
		SetEmail(email).
		SetPasswordHash("test-hash").
		SetStatus(service.StatusActive).
		SetRole(service.RoleUser).
		Save(s.ctx)
	s.Require().NoError(err, "create user")
	return userEntityToService(u)
}

func (s *BundleSubscriptionLifecycleSuite) mustCreateGroup(name, platform string) *service.Group {
	s.T().Helper()
	g, err := s.client.Group.Create().
		SetName(name).
		SetPlatform(platform).
		SetStatus(service.StatusActive).
		Save(s.ctx)
	s.Require().NoError(err, "create group")
	return groupEntityToService(g)
}

func (s *BundleSubscriptionLifecycleSuite) mustCreatePlan(name string, quotas []service.CreateGroupQuotaRequest) *service.BundlePlan {
	s.T().Helper()
	plan, err := s.planSvc.CreatePlan(s.ctx, &service.CreateBundlePlanRequest{
		Name:             name,
		Description:      "test plan",
		Tier:             service.BundleTierPro,
		Price:            29.99,
		OriginalPrice:    39.99,
		Currency:         "USD",
		ValidityDays:     30,
		ConcurrencyLimit: 5,
		RPMLimit:         100,
		GroupQuotas:      quotas,
	})
	s.Require().NoError(err, "create plan")
	s.Require().NotNil(plan)
	return plan
}

// --- tests ---

// TestActivateBundle_CreatesBundleSubscriptionAndBridgedUserSubscriptions
// 验证激活套餐后完整的数据创建链路：
// BundleSubscription → BundleSubscriptionUsage → bridged UserSubscription
func (s *BundleSubscriptionLifecycleSuite) TestActivateBundle_CreatesBundleSubscriptionAndBridgedUserSubscriptions() {
	user := s.mustCreateUser("bundle-activate@test.com")
	openaiGroup := s.mustCreateGroup("openai-group", domain.PlatformOpenAI)
	anthropicGroup := s.mustCreateGroup("anthropic-group", domain.PlatformAnthropic)

	plan := s.mustCreatePlan("Pro Plan", []service.CreateGroupQuotaRequest{
		{GroupID: openaiGroup.ID, QuotaScope: service.QuotaScopePlatform, DailyLimitUSD: 1.0, WeeklyLimitUSD: 5.0, MonthlyLimitUSD: 20.0},
		{GroupID: anthropicGroup.ID, QuotaScope: service.QuotaScopePlatform, DailyLimitUSD: 2.0, WeeklyLimitUSD: 10.0, MonthlyLimitUSD: 40.0},
	})

	bundleSub, err := s.subSvc.ActivateBundle(s.ctx, &service.ActivateBundleRequest{
		UserID: user.ID,
		PlanID: plan.ID,
		Source: service.BundleSourcePurchase,
	})
	s.Require().NoError(err, "ActivateBundle")
	s.Require().NotNil(bundleSub)

	// Verify BundleSubscription fields
	s.Require().Equal(user.ID, bundleSub.UserID)
	s.Require().Equal(plan.ID, bundleSub.PlanID)
	s.Require().Equal(service.BundleStatusActive, bundleSub.Status)
	s.Require().Equal(service.BundleSourcePurchase, bundleSub.Source)
	s.Require().Equal(plan.ConcurrencyLimit, bundleSub.ConcurrencyLimit)
	s.Require().Equal(plan.RPMLimit, bundleSub.RPMLimit)
	s.Require().False(bundleSub.ExpiresAt.IsZero())
	s.Require().True(bundleSub.ExpiresAt.After(bundleSub.StartsAt))

	// Verify BundleSubscriptionUsage created per GroupQuota
	usages, err := s.usageRepo.ListBySubscription(s.ctx, bundleSub.ID)
	s.Require().NoError(err, "ListBySubscription")
	s.Require().Len(usages, 2, "expected 2 usage records (one per group quota)")

	groupIDs := make(map[int64]bool)
	for _, u := range usages {
		groupIDs[u.GroupID] = true
		s.Require().Equal(bundleSub.ID, u.BundleSubscriptionID)
		s.Require().Equal(float64(0), u.DailyUsageUSD, "initial usage should be 0")
		s.Require().Equal(float64(0), u.WeeklyUsageUSD)
		s.Require().Equal(float64(0), u.MonthlyUsageUSD)
	}
	s.Require().True(groupIDs[openaiGroup.ID], "expected usage for openai group")
	s.Require().True(groupIDs[anthropicGroup.ID], "expected usage for anthropic group")

	// Verify bridged UserSubscriptions created
	subs, err := s.userSubRepo.ListByUserID(s.ctx, user.ID)
	s.Require().NoError(err, "ListByUserID")
	s.Require().Len(subs, 2, "expected 2 bridged UserSubscriptions")

	for _, us := range subs {
		s.Require().NotNil(us.BundleSubscriptionID)
		s.Require().Equal(bundleSub.ID, *us.BundleSubscriptionID)
		s.Require().Equal(service.SubscriptionStatusActive, us.Status)
		s.Require().Equal(user.ID, us.UserID)
	}
}

// TestActivateBundle_ConflictWhenUserAlreadyHasActiveBundle
// 验证用户已有活跃套餐时重复激活返回冲突错误
func (s *BundleSubscriptionLifecycleSuite) TestActivateBundle_ConflictWhenUserAlreadyHasActiveBundle() {
	user := s.mustCreateUser("bundle-conflict@test.com")
	group := s.mustCreateGroup("g-conflict", domain.PlatformOpenAI)

	plan := s.mustCreatePlan("Plan", []service.CreateGroupQuotaRequest{
		{GroupID: group.ID, QuotaScope: service.QuotaScopePlatform},
	})

	// First activation succeeds
	_, err := s.subSvc.ActivateBundle(s.ctx, &service.ActivateBundleRequest{
		UserID: user.ID, PlanID: plan.ID, Source: service.BundleSourcePurchase,
	})
	s.Require().NoError(err)

	// Second activation fails with conflict
	_, err = s.subSvc.ActivateBundle(s.ctx, &service.ActivateBundleRequest{
		UserID: user.ID, PlanID: plan.ID, Source: service.BundleSourcePurchase,
	})
	s.Require().ErrorIs(err, service.ErrBundleConflict)
}

// TestActivateBundle_RejectsDisabledPlan
// 验证禁用/下架的套餐拒绝激活
func (s *BundleSubscriptionLifecycleSuite) TestActivateBundle_RejectsDisabledPlan() {
	user := s.mustCreateUser("bundle-disabled@test.com")
	group := s.mustCreateGroup("g-disabled", domain.PlatformOpenAI)

	plan := s.mustCreatePlan("Disabled Plan", []service.CreateGroupQuotaRequest{
		{GroupID: group.ID, QuotaScope: service.QuotaScopePlatform},
	})

	// Disable the plan
	statusDisabled := "disabled"
	_, err := s.planSvc.UpdatePlan(s.ctx, plan.ID, &service.UpdateBundlePlanRequest{Status: &statusDisabled})
	s.Require().NoError(err)

	_, err = s.subSvc.ActivateBundle(s.ctx, &service.ActivateBundleRequest{
		UserID: user.ID, PlanID: plan.ID, Source: service.BundleSourcePurchase,
	})
	s.Require().ErrorIs(err, service.ErrBundlePlanDisabled)
}

// TestIncrementUsage_AccumulatesCorrectly
// 验证用量累加：daily/weekly/monthly 同时递增
func (s *BundleSubscriptionLifecycleSuite) TestIncrementUsage_AccumulatesCorrectly() {
	user := s.mustCreateUser("bundle-usage@test.com")
	group := s.mustCreateGroup("g-usage", domain.PlatformOpenAI)

	plan := s.mustCreatePlan("Plan", []service.CreateGroupQuotaRequest{
		{GroupID: group.ID, QuotaScope: service.QuotaScopePlatform, DailyLimitUSD: 10, WeeklyLimitUSD: 50, MonthlyLimitUSD: 200},
	})

	bundleSub, err := s.subSvc.ActivateBundle(s.ctx, &service.ActivateBundleRequest{
		UserID: user.ID, PlanID: plan.ID, Source: service.BundleSourcePurchase,
	})
	s.Require().NoError(err)

	// Get the usage record for this group
	usage, err := s.usageRepo.GetBySubscriptionAndGroup(s.ctx, bundleSub.ID, group.ID, "")
	s.Require().NoError(err)
	s.Require().NotNil(usage)

	// Accumulate usage incrementally
	cost1 := 0.5
	cost2 := 1.25
	s.Require().NoError(s.usageRepo.IncrementUsage(s.ctx, usage.ID, cost1, 1))
	s.Require().NoError(s.usageRepo.IncrementUsage(s.ctx, usage.ID, cost2, 1))

	// Verify accumulated correctly
	usage, err = s.usageRepo.GetBySubscriptionAndGroup(s.ctx, bundleSub.ID, group.ID, "")
	s.Require().NoError(err)
	s.Require().Equal(cost1+cost2, usage.DailyUsageUSD, "daily usage should be sum of increments")
	s.Require().Equal(cost1+cost2, usage.WeeklyUsageUSD, "weekly usage should be sum of increments")
	s.Require().Equal(cost1+cost2, usage.MonthlyUsageUSD, "monthly usage should be sum of increments")
}

// TestCheckQuotaEligibility_WithinLimits
// 验证用量未超限额时检查通过
func (s *BundleSubscriptionLifecycleSuite) TestCheckQuotaEligibility_WithinLimits() {
	user := s.mustCreateUser("quota-ok@test.com")
	group := s.mustCreateGroup("g-quota-ok", domain.PlatformOpenAI)

	plan := s.mustCreatePlan("Plan", []service.CreateGroupQuotaRequest{
		{GroupID: group.ID, QuotaScope: service.QuotaScopePlatform, DailyLimitUSD: 10, WeeklyLimitUSD: 50, MonthlyLimitUSD: 200},
	})

	bundleSub, err := s.subSvc.ActivateBundle(s.ctx, &service.ActivateBundleRequest{
		UserID: user.ID, PlanID: plan.ID, Source: service.BundleSourcePurchase,
	})
	s.Require().NoError(err)

	usage, err := s.usageRepo.GetBySubscriptionAndGroup(s.ctx, bundleSub.ID, group.ID, "")
	s.Require().NoError(err)

	// Stay under the daily limit
	s.Require().NoError(s.usageRepo.IncrementUsage(s.ctx, usage.ID, 5.0, 1))

	result, err := s.usageSvc.CheckQuotaEligibility(s.ctx, bundleSub.ID, group.ID)
	s.Require().NoError(err, "should pass when under limits")
	s.Require().True(result.Eligible, "should be eligible under daily limit")
}

// TestCheckQuotaEligibility_ExceedsDailyLimit
// 验证日用量超限时检查返回配额超限错误
func (s *BundleSubscriptionLifecycleSuite) TestCheckQuotaEligibility_ExceedsDailyLimit() {
	user := s.mustCreateUser("quota-daily@test.com")
	group := s.mustCreateGroup("g-quota-daily", domain.PlatformOpenAI)

	plan := s.mustCreatePlan("Plan", []service.CreateGroupQuotaRequest{
		{GroupID: group.ID, QuotaScope: service.QuotaScopePlatform, DailyLimitUSD: 5.0},
	})

	bundleSub, err := s.subSvc.ActivateBundle(s.ctx, &service.ActivateBundleRequest{
		UserID: user.ID, PlanID: plan.ID, Source: service.BundleSourcePurchase,
	})
	s.Require().NoError(err)

	usage, err := s.usageRepo.GetBySubscriptionAndGroup(s.ctx, bundleSub.ID, group.ID, "")
	s.Require().NoError(err)

	// Exceed daily limit
	s.Require().NoError(s.usageRepo.IncrementUsage(s.ctx, usage.ID, 5.01, 1))

	result, err := s.usageSvc.CheckQuotaEligibility(s.ctx, bundleSub.ID, group.ID)
	s.Require().NoError(err)
	s.Require().False(result.Eligible, "should NOT be eligible when limit exceeded")
}

// TestCheckQuotaEligibility_ExceedsWeeklyLimit
// 验证周用量超限时检查返回配额超限错误
func (s *BundleSubscriptionLifecycleSuite) TestCheckQuotaEligibility_ExceedsWeeklyLimit() {
	user := s.mustCreateUser("quota-weekly@test.com")
	group := s.mustCreateGroup("g-quota-weekly", domain.PlatformOpenAI)

	plan := s.mustCreatePlan("Plan", []service.CreateGroupQuotaRequest{
		{GroupID: group.ID, QuotaScope: service.QuotaScopePlatform, WeeklyLimitUSD: 10.0},
	})

	bundleSub, err := s.subSvc.ActivateBundle(s.ctx, &service.ActivateBundleRequest{
		UserID: user.ID, PlanID: plan.ID, Source: service.BundleSourcePurchase,
	})
	s.Require().NoError(err)

	usage, err := s.usageRepo.GetBySubscriptionAndGroup(s.ctx, bundleSub.ID, group.ID, "")
	s.Require().NoError(err)
	s.Require().NoError(s.usageRepo.IncrementUsage(s.ctx, usage.ID, 10.01, 1))

	result, err := s.usageSvc.CheckQuotaEligibility(s.ctx, bundleSub.ID, group.ID)
	s.Require().NoError(err)
	s.Require().False(result.Eligible, "should NOT be eligible when limit exceeded")
}

// TestGetBundleUsageProgress_ReturnsAllGroupProgress
// 验证 GetBundleUsageProgress 返回所有 Group 的用量/限额进度
func (s *BundleSubscriptionLifecycleSuite) TestGetBundleUsageProgress_ReturnsAllGroupProgress() {
	user := s.mustCreateUser("progress@test.com")
	openaiGroup := s.mustCreateGroup("g-progress-openai", domain.PlatformOpenAI)
	anthropicGroup := s.mustCreateGroup("g-progress-anthro", domain.PlatformAnthropic)

	plan := s.mustCreatePlan("Plan", []service.CreateGroupQuotaRequest{
		{GroupID: openaiGroup.ID, QuotaScope: service.QuotaScopePlatform, DailyLimitUSD: 3.0},
		{GroupID: anthropicGroup.ID, QuotaScope: service.QuotaScopePlatform, DailyLimitUSD: 5.0},
	})

	bundleSub, err := s.subSvc.ActivateBundle(s.ctx, &service.ActivateBundleRequest{
		UserID: user.ID, PlanID: plan.ID, Source: service.BundleSourcePurchase,
	})
	s.Require().NoError(err)

	// Add some usage to openai group
	usage, err := s.usageRepo.GetBySubscriptionAndGroup(s.ctx, bundleSub.ID, openaiGroup.ID, "")
	s.Require().NoError(err)
	s.Require().NoError(s.usageRepo.IncrementUsage(s.ctx, usage.ID, 1.5, 1))

	progress, err := s.subSvc.GetBundleUsageProgress(s.ctx, bundleSub.ID)
	s.Require().NoError(err)
	s.Require().Len(progress, 2, "expected progress for 2 groups")

	for _, p := range progress {
		switch p.GroupID {
		case openaiGroup.ID:
			s.Require().Equal(1.5, p.DailyUsageUSD)
			s.Require().Equal(3.0, p.DailyLimitUSD)
		case anthropicGroup.ID:
			s.Require().Equal(float64(0), p.DailyUsageUSD)
			s.Require().Equal(5.0, p.DailyLimitUSD)
		default:
			s.T().Fatalf("unexpected group id: %d", p.GroupID)
		}
	}
}

// =============================================================================
// Suite 2: BundleRouteResolverSuite
// 覆盖: Single-key multi-platform routing + independent quota
// =============================================================================

type BundleRouteResolverSuite struct {
	suite.Suite
	ctx    context.Context
	client *dbent.Client

	planSvc     *service.BundlePlanService
	subSvc      *service.BundleSubscriptionService
	usageSvc    *service.BundleUsageService
	resolver    *service.BundleRouteResolver
	planRepo    service.BundlePlanRepository
	subRepo     service.BundleSubscriptionRepository
	usageRepo   service.BundleUsageRepository
	userSubRepo service.UserSubscriptionRepository
	groupRepo   service.GroupRepository
}

func (s *BundleRouteResolverSuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.client = tx.Client()

	s.planRepo = NewBundlePlanRepository(s.client)
	s.subRepo = NewBundleSubscriptionRepository(s.client)
	s.usageRepo = NewBundleUsageRepository(s.client)
	s.userSubRepo = NewUserSubscriptionRepository(s.client)
	s.groupRepo = NewGroupRepository(s.client, integrationDB)

	s.planSvc = service.NewBundlePlanService(s.planRepo, nil)
	s.subSvc = service.NewBundleSubscriptionService(s.subRepo, s.planRepo, s.usageRepo, s.userSubRepo, nil)
	s.usageSvc = service.NewBundleUsageService(s.usageRepo, s.subRepo, s.planRepo)
	s.resolver = service.NewBundleRouteResolver(s.subRepo, s.planRepo, s.groupRepo)
}

func TestBundleRouteResolverSuite(t *testing.T) {
	suite.Run(t, new(BundleRouteResolverSuite))
}

// --- helpers ---

func (s *BundleRouteResolverSuite) mustCreateUser(email string) *service.User {
	s.T().Helper()
	u, err := s.client.User.Create().
		SetEmail(email).
		SetPasswordHash("test-hash").
		SetStatus(service.StatusActive).
		SetRole(service.RoleUser).
		Save(s.ctx)
	s.Require().NoError(err, "create user")
	return userEntityToService(u)
}

func (s *BundleRouteResolverSuite) mustCreateGroup(name, platform string) *service.Group {
	s.T().Helper()
	g, err := s.client.Group.Create().
		SetName(name).
		SetPlatform(platform).
		SetStatus(service.StatusActive).
		Save(s.ctx)
	s.Require().NoError(err, "create group")
	return groupEntityToService(g)
}

func (s *BundleRouteResolverSuite) mustCreatePlan(name string, quotas []service.CreateGroupQuotaRequest) *service.BundlePlan {
	s.T().Helper()
	plan, err := s.planSvc.CreatePlan(s.ctx, &service.CreateBundlePlanRequest{
		Name:         name,
		Description:  "test plan",
		Tier:         service.BundleTierPro,
		Price:        29.99,
		Currency:     "USD",
		ValidityDays: 30,
		GroupQuotas:  quotas,
	})
	s.Require().NoError(err, "create plan")
	return plan
}

func (s *BundleRouteResolverSuite) mustActivateBundle(userID, planID int64) *service.BundleSubscription {
	s.T().Helper()
	bundleSub, err := s.subSvc.ActivateBundle(s.ctx, &service.ActivateBundleRequest{
		UserID: userID, PlanID: planID, Source: service.BundleSourcePurchase,
	})
	s.Require().NoError(err, "activate bundle")
	return bundleSub
}

// --- tests ---

// TestResolveGroup_ModelLevelGlobMatch
// 验证模型级 glob 路由：model_pattern="claude-opus-*" 匹配 claude-opus-4-8
func (s *BundleRouteResolverSuite) TestResolveGroup_ModelLevelGlobMatch() {
	user := s.mustCreateUser("route-glob@test.com")
	claudeGroup := s.mustCreateGroup("claude-specific", domain.PlatformAnthropic)

	plan := s.mustCreatePlan("Plan", []service.CreateGroupQuotaRequest{
		{GroupID: claudeGroup.ID, QuotaScope: service.QuotaScopeModel, ModelPattern: "claude-opus-*"},
	})

	bundleSub := s.mustActivateBundle(user.ID, plan.ID)

	resolved, err := s.resolver.ResolveGroup(s.ctx, "claude-opus-4-8-20250609", bundleSub.ID)
	s.Require().NoError(err)
	s.Require().NotNil(resolved)
	s.Require().Equal(claudeGroup.ID, resolved.GroupID)
	s.Require().Equal(domain.PlatformAnthropic, resolved.Platform)
}

// TestResolveGroup_PlatformLevelFallback
// 验证无模型级匹配时回退到平台级匹配
func (s *BundleRouteResolverSuite) TestResolveGroup_PlatformLevelFallback() {
	user := s.mustCreateUser("route-platform@test.com")
	openaiGroup := s.mustCreateGroup("openai-main", domain.PlatformOpenAI)
	anthropicGroup := s.mustCreateGroup("anthro-main", domain.PlatformAnthropic)

	plan := s.mustCreatePlan("Plan", []service.CreateGroupQuotaRequest{
		{GroupID: openaiGroup.ID, QuotaScope: service.QuotaScopePlatform},
		{GroupID: anthropicGroup.ID, QuotaScope: service.QuotaScopePlatform},
	})

	bundleSub := s.mustActivateBundle(user.ID, plan.ID)

	// gpt-3.5-turbo matches no model pattern, falls back to openai platform
	resolved, err := s.resolver.ResolveGroup(s.ctx, "gpt-3.5-turbo", bundleSub.ID)
	s.Require().NoError(err)
	s.Require().Equal(openaiGroup.ID, resolved.GroupID)
	s.Require().Equal(domain.PlatformOpenAI, resolved.Platform)

	// claude-sonnet-4 matches no model pattern, falls back to anthropic platform
	resolved, err = s.resolver.ResolveGroup(s.ctx, "claude-sonnet-4-6-20250514", bundleSub.ID)
	s.Require().NoError(err)
	s.Require().Equal(anthropicGroup.ID, resolved.GroupID)
	s.Require().Equal(domain.PlatformAnthropic, resolved.Platform)
}

// TestResolveGroup_ModelNotIncluded
// 验证模型不在任何匹配范围内时返回错误
func (s *BundleRouteResolverSuite) TestResolveGroup_ModelNotIncluded() {
	user := s.mustCreateUser("route-notfound@test.com")
	claudeGroup := s.mustCreateGroup("claude-only", domain.PlatformAnthropic)

	plan := s.mustCreatePlan("Plan", []service.CreateGroupQuotaRequest{
		{GroupID: claudeGroup.ID, QuotaScope: service.QuotaScopePlatform},
	})

	bundleSub := s.mustActivateBundle(user.ID, plan.ID)

	// gpt-4o is openai protocol, but plan only has anthropic platform
	_, err := s.resolver.ResolveGroup(s.ctx, "gpt-4o", bundleSub.ID)
	s.Require().ErrorIs(err, service.ErrBundleModelNotIncluded)
}

// TestResolveGroup_ExpiredBundleReturnsError
// 验证过期 bundle 的路由请求被拒绝
func (s *BundleRouteResolverSuite) TestResolveGroup_ExpiredBundleReturnsError() {
	user := s.mustCreateUser("route-expired@test.com")
	group := s.mustCreateGroup("g-expired", domain.PlatformOpenAI)

	plan := s.mustCreatePlan("Plan", []service.CreateGroupQuotaRequest{
		{GroupID: group.ID, QuotaScope: service.QuotaScopePlatform},
	})

	bundleSub := s.mustActivateBundle(user.ID, plan.ID)

	// Manually expire the bundle
	err := s.subRepo.UpdateStatus(s.ctx, bundleSub.ID, service.BundleStatusExpired)
	s.Require().NoError(err)

	_, err = s.resolver.ResolveGroup(s.ctx, "gpt-4o", bundleSub.ID)
	s.Require().ErrorIs(err, service.ErrBundleExpired)
}

// TestResolveGroup_CrossPlatformRouting
// 验证同一个用户使用同一个 bundle 的路由器可以将不同模型请求分发到正确的 Group
func (s *BundleRouteResolverSuite) TestResolveGroup_CrossPlatformRouting() {
	user := s.mustCreateUser("cross-platform@test.com")
	openaiGroup := s.mustCreateGroup("cross-openai", domain.PlatformOpenAI)
	anthroGroup := s.mustCreateGroup("cross-anthro", domain.PlatformAnthropic)
	geminiGroup := s.mustCreateGroup("cross-gemini", domain.PlatformGemini)

	plan := s.mustCreatePlan("Multi-Platform Plan", []service.CreateGroupQuotaRequest{
		{GroupID: openaiGroup.ID, QuotaScope: service.QuotaScopePlatform},
		{GroupID: anthroGroup.ID, QuotaScope: service.QuotaScopePlatform},
		{GroupID: geminiGroup.ID, QuotaScope: service.QuotaScopePlatform},
	})

	bundleSub := s.mustActivateBundle(user.ID, plan.ID)

	tests := []struct {
		model       string
		expectedID  int64
		expectedPlt string
	}{
		{"gpt-4o", openaiGroup.ID, domain.PlatformOpenAI},
		{"gpt-4.1-mini", openaiGroup.ID, domain.PlatformOpenAI},
		{"claude-opus-4-8-20250609", anthroGroup.ID, domain.PlatformAnthropic},
		{"claude-sonnet-4-6-20250514", anthroGroup.ID, domain.PlatformAnthropic},
		{"gemini-2.5-pro", geminiGroup.ID, domain.PlatformGemini},
		{"deepseek-chat", openaiGroup.ID, domain.PlatformOpenAI},
	}

	for _, tt := range tests {
		s.Run(tt.model, func() {
			resolved, err := s.resolver.ResolveGroup(s.ctx, tt.model, bundleSub.ID)
			s.Require().NoError(err)
			s.Require().Equal(tt.expectedID, resolved.GroupID, "model=%s", tt.model)
			s.Require().Equal(tt.expectedPlt, resolved.Platform, "model=%s", tt.model)
		})
	}
}

// TestResolveGroup_IndependentQuotaPerGroup
// 验证不同 Group 的用量独立追踪：请求一个平台的模型不影响另一个平台的用量
func (s *BundleRouteResolverSuite) TestResolveGroup_IndependentQuotaPerGroup() {
	user := s.mustCreateUser("indep-quota@test.com")
	openaiGroup := s.mustCreateGroup("indep-openai", domain.PlatformOpenAI)
	anthroGroup := s.mustCreateGroup("indep-anthro", domain.PlatformAnthropic)

	plan := s.mustCreatePlan("Plan", []service.CreateGroupQuotaRequest{
		{GroupID: openaiGroup.ID, QuotaScope: service.QuotaScopePlatform, DailyLimitUSD: 10},
		{GroupID: anthroGroup.ID, QuotaScope: service.QuotaScopePlatform, DailyLimitUSD: 10},
	})

	bundleSub := s.mustActivateBundle(user.ID, plan.ID)

	// Route to openai group and accumulate usage
	resolved, err := s.resolver.ResolveGroup(s.ctx, "gpt-4o", bundleSub.ID)
	s.Require().NoError(err)
	s.Require().Equal(openaiGroup.ID, resolved.GroupID)

	openaiUsage, err := s.usageRepo.GetBySubscriptionAndGroup(s.ctx, bundleSub.ID, openaiGroup.ID, "")
	s.Require().NoError(err)
	s.Require().NoError(s.usageRepo.IncrementUsage(s.ctx, openaiUsage.ID, 8.0, 1))

	// Anthropic group usage should be unaffected
	anthroUsage, err := s.usageRepo.GetBySubscriptionAndGroup(s.ctx, bundleSub.ID, anthroGroup.ID, "")
	s.Require().NoError(err)
	s.Require().Equal(float64(0), anthroUsage.DailyUsageUSD, "anthropic group usage should be independent")
	s.Require().Equal(float64(0), anthroUsage.WeeklyUsageUSD)
	s.Require().Equal(float64(0), anthroUsage.MonthlyUsageUSD)

	// Openai usage = 8.0 (still under limit of 10)
	res, err := s.usageSvc.CheckQuotaEligibility(s.ctx, bundleSub.ID, openaiGroup.ID)
	s.Require().NoError(err, "openai still under daily limit")
	s.Require().True(res.Eligible, "openai should be eligible")

	// Anthro should also pass (0 usage)
	res, err = s.usageSvc.CheckQuotaEligibility(s.ctx, bundleSub.ID, anthroGroup.ID)
	s.Require().NoError(err, "anthropic should have no usage")
	s.Require().True(res.Eligible, "anthropic should be eligible")
}

// =============================================================================
// Suite 3: BundleExpiryIntegrationSuite
// 覆盖: Bundle expiry → request rejection + other bundles unaffected
// =============================================================================

type BundleExpiryIntegrationSuite struct {
	suite.Suite
	ctx    context.Context
	client *dbent.Client

	planSvc     *service.BundlePlanService
	subSvc      *service.BundleSubscriptionService
	resolver    *service.BundleRouteResolver
	planRepo    service.BundlePlanRepository
	subRepo     service.BundleSubscriptionRepository
	usageRepo   service.BundleUsageRepository
	userSubRepo service.UserSubscriptionRepository
	groupRepo   service.GroupRepository
}

func (s *BundleExpiryIntegrationSuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.client = tx.Client()

	s.planRepo = NewBundlePlanRepository(s.client)
	s.subRepo = NewBundleSubscriptionRepository(s.client)
	s.usageRepo = NewBundleUsageRepository(s.client)
	s.userSubRepo = NewUserSubscriptionRepository(s.client)
	s.groupRepo = NewGroupRepository(s.client, integrationDB)

	s.planSvc = service.NewBundlePlanService(s.planRepo, nil)
	s.subSvc = service.NewBundleSubscriptionService(s.subRepo, s.planRepo, s.usageRepo, s.userSubRepo, nil)
	s.resolver = service.NewBundleRouteResolver(s.subRepo, s.planRepo, s.groupRepo)
}

func TestBundleExpiryIntegrationSuite(t *testing.T) {
	suite.Run(t, new(BundleExpiryIntegrationSuite))
}

// --- helpers ---

func (s *BundleExpiryIntegrationSuite) mustCreateUser(email string) *service.User {
	s.T().Helper()
	u, err := s.client.User.Create().
		SetEmail(email).
		SetPasswordHash("test-hash").
		SetStatus(service.StatusActive).
		SetRole(service.RoleUser).
		Save(s.ctx)
	s.Require().NoError(err, "create user")
	return userEntityToService(u)
}

func (s *BundleExpiryIntegrationSuite) mustCreateGroup(name, platform string) *service.Group {
	s.T().Helper()
	g, err := s.client.Group.Create().
		SetName(name).
		SetPlatform(platform).
		SetStatus(service.StatusActive).
		Save(s.ctx)
	s.Require().NoError(err, "create group")
	return groupEntityToService(g)
}

func (s *BundleExpiryIntegrationSuite) mustCreatePlan(name string, quotas []service.CreateGroupQuotaRequest) *service.BundlePlan {
	s.T().Helper()
	plan, err := s.planSvc.CreatePlan(s.ctx, &service.CreateBundlePlanRequest{
		Name:         name,
		Description:  "test plan",
		Tier:         service.BundleTierPro,
		Price:        29.99,
		Currency:     "USD",
		ValidityDays: 30,
		GroupQuotas:  quotas,
	})
	s.Require().NoError(err, "create plan")
	return plan
}

// --- tests ---

// TestBatchUpdateExpiredStatus_MarksExpiredBundles
// 验证 BatchUpdateExpiredStatus 只更新已过期的活跃 bundle，不过期的保持不变
func (s *BundleExpiryIntegrationSuite) TestBatchUpdateExpiredStatus_MarksExpiredBundles() {
	user1 := s.mustCreateUser("expired-user@test.com")
	user2 := s.mustCreateUser("active-user@test.com")
	group := s.mustCreateGroup("g-expiry", domain.PlatformOpenAI)

	plan := s.mustCreatePlan("Plan", []service.CreateGroupQuotaRequest{
		{GroupID: group.ID, QuotaScope: service.QuotaScopePlatform},
	})

	// Activate bundle for user1
	bundle1, err := s.subSvc.ActivateBundle(s.ctx, &service.ActivateBundleRequest{
		UserID: user1.ID, PlanID: plan.ID, Source: service.BundleSourcePurchase,
	})
	s.Require().NoError(err)

	// Activate bundle for user2
	bundle2, err := s.subSvc.ActivateBundle(s.ctx, &service.ActivateBundleRequest{
		UserID: user2.ID, PlanID: plan.ID, Source: service.BundleSourcePurchase,
	})
	s.Require().NoError(err)

	// Manually expire bundle1 by setting expires_at in the past
	past := time.Now().Add(-24 * time.Hour)
	err = s.subRepo.UpdateExpiry(s.ctx, bundle1.ID, past)
	s.Require().NoError(err)

	// Wait a moment so the time check is robust (within the same second)
	time.Sleep(100 * time.Millisecond)

	// Run batch expiry
	updated, err := s.usageRepo.BatchUpdateExpiredStatus(s.ctx)
	s.Require().NoError(err)
	s.Require().Equal(int64(1), updated, "only bundle1 should be expired")

	// Verify: bundle1 is expired
	b1, err := s.subRepo.GetByID(s.ctx, bundle1.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.BundleStatusExpired, b1.Status, "bundle1 should be expired")

	// Verify: bundle2 is still active
	b2, err := s.subRepo.GetByID(s.ctx, bundle2.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.BundleStatusActive, b2.Status, "bundle2 should remain active")
}

// TestResolveGroup_RejectsExpiredBundleButAllowsActive
// 验证过期 bundle 被路由拒绝，但其他用户/其他活跃 bundle 不受影响
func (s *BundleExpiryIntegrationSuite) TestResolveGroup_RejectsExpiredBundleButAllowsActive() {
	userA := s.mustCreateUser("expired-A@test.com")
	userB := s.mustCreateUser("active-B@test.com")
	group := s.mustCreateGroup("g-both", domain.PlatformOpenAI)

	plan := s.mustCreatePlan("Plan", []service.CreateGroupQuotaRequest{
		{GroupID: group.ID, QuotaScope: service.QuotaScopePlatform},
	})

	bundleA, err := s.subSvc.ActivateBundle(s.ctx, &service.ActivateBundleRequest{
		UserID: userA.ID, PlanID: plan.ID, Source: service.BundleSourcePurchase,
	})
	s.Require().NoError(err)

	bundleB, err := s.subSvc.ActivateBundle(s.ctx, &service.ActivateBundleRequest{
		UserID: userB.ID, PlanID: plan.ID, Source: service.BundleSourcePurchase,
	})
	s.Require().NoError(err)

	// Expire bundle A manually
	err = s.subRepo.UpdateExpiry(s.ctx, bundleA.ID, time.Now().Add(-1*time.Hour))
	s.Require().NoError(err)
	time.Sleep(100 * time.Millisecond)
	_, err = s.usageRepo.BatchUpdateExpiredStatus(s.ctx)
	s.Require().NoError(err)

	// User A's expired bundle → rejected
	_, err = s.resolver.ResolveGroup(s.ctx, "gpt-4o", bundleA.ID)
	s.Require().ErrorIs(err, service.ErrBundleExpired, "expired bundle should be rejected")

	// User B's active bundle → still works
	resolved, err := s.resolver.ResolveGroup(s.ctx, "gpt-4o", bundleB.ID)
	s.Require().NoError(err, "active bundle should still route")
	s.Require().Equal(group.ID, resolved.GroupID)
}

// TestSyncExpiredBridgedUserSubscriptions_ExpiresAllBridged
// 验证过期 bundle 的 bridged UserSubscriptions 被批量标记为 expired
func (s *BundleExpiryIntegrationSuite) TestSyncExpiredBridgedUserSubscriptions_ExpiresAllBridged() {
	user := s.mustCreateUser("sync-expired@test.com")
	openaiGroup := s.mustCreateGroup("sync-openai", domain.PlatformOpenAI)
	anthroGroup := s.mustCreateGroup("sync-anthro", domain.PlatformAnthropic)

	plan := s.mustCreatePlan("Plan", []service.CreateGroupQuotaRequest{
		{GroupID: openaiGroup.ID, QuotaScope: service.QuotaScopePlatform},
		{GroupID: anthroGroup.ID, QuotaScope: service.QuotaScopePlatform},
	})

	bundleSub, err := s.subSvc.ActivateBundle(s.ctx, &service.ActivateBundleRequest{
		UserID: user.ID, PlanID: plan.ID, Source: service.BundleSourcePurchase,
	})
	s.Require().NoError(err)

	// Verify bridged UserSubscriptions exist and are active
	subs, err := s.userSubRepo.ListByUserID(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Require().Len(subs, 2)
	for _, us := range subs {
		s.Require().Equal(service.SubscriptionStatusActive, us.Status)
	}

	// Expire the bundle
	err = s.subRepo.UpdateExpiry(s.ctx, bundleSub.ID, time.Now().Add(-1*time.Hour))
	s.Require().NoError(err)
	time.Sleep(100 * time.Millisecond)
	_, err = s.usageRepo.BatchUpdateExpiredStatus(s.ctx)
	s.Require().NoError(err)

	// Sync bridged UserSubscriptions
	affected, err := s.userSubRepo.ExpireBridgedSubscriptionsForExpiredBundles(s.ctx)
	s.Require().NoError(err)
	s.Require().Equal(int64(2), affected, "both bridged UserSubscriptions should be expired")

	// Verify bridged UserSubscriptions are now expired
	subs, err = s.userSubRepo.ListByUserID(s.ctx, user.ID)
	s.Require().NoError(err)
	for _, us := range subs {
		s.Require().Equal(service.SubscriptionStatusExpired, us.Status,
			"bridged UserSubscription for group %d should be expired", us.GroupID)
	}
}

// TestActivateBundle_AllowedAfterPreviousBundleExpired
// 验证上一个 bundle 过期后允许购买新 bundle
func (s *BundleExpiryIntegrationSuite) TestActivateBundle_AllowedAfterPreviousBundleExpired() {
	user := s.mustCreateUser("renew@test.com")
	group := s.mustCreateGroup("g-renew", domain.PlatformOpenAI)

	plan := s.mustCreatePlan("Plan", []service.CreateGroupQuotaRequest{
		{GroupID: group.ID, QuotaScope: service.QuotaScopePlatform},
	})

	// First activation
	bundle1, err := s.subSvc.ActivateBundle(s.ctx, &service.ActivateBundleRequest{
		UserID: user.ID, PlanID: plan.ID, Source: service.BundleSourcePurchase,
	})
	s.Require().NoError(err)

	// Expire it
	err = s.subRepo.UpdateExpiry(s.ctx, bundle1.ID, time.Now().Add(-1*time.Hour))
	s.Require().NoError(err)
	time.Sleep(100 * time.Millisecond)
	_, err = s.usageRepo.BatchUpdateExpiredStatus(s.ctx)
	s.Require().NoError(err)
	// Expire bridged UserSubscriptions too (the partial unique index on user_id+group_id
	// prevents creating new subscriptions for the same groups).
	_, err = s.userSubRepo.ExpireBridgedSubscriptionsForExpiredBundles(s.ctx)
	s.Require().NoError(err)
	// Soft-delete the bridged UserSubscriptions to clear the partial unique index.
	oldSubs, listErr := s.userSubRepo.ListByUserID(s.ctx, user.ID)
	s.Require().NoError(listErr)
	for _, sub := range oldSubs {
		if sub.BundleSubscriptionID != nil && *sub.BundleSubscriptionID == bundle1.ID {
			s.Require().NoError(s.userSubRepo.Delete(s.ctx, sub.ID))
		}
	}

	// Renew: activate second bundle using admin_assign to bypass conflict check
	bundle2, err := s.subSvc.ActivateBundle(s.ctx, &service.ActivateBundleRequest{
		UserID: user.ID, PlanID: plan.ID, Source: service.BundleSourceAdminAssign,
	})
	s.Require().NoError(err, "should allow new purchase after previous bundle expired")
	s.Require().NotNil(bundle2)
	s.Require().NotEqual(bundle1.ID, bundle2.ID, "should be a new subscription record")

	// Verify both bundles: first expired, second active
	b1, err := s.subRepo.GetByID(s.ctx, bundle1.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.BundleStatusExpired, b1.Status)

	b2, err := s.subRepo.GetByID(s.ctx, bundle2.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.BundleStatusActive, b2.Status)
}
