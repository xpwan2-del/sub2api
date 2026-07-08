// bundle_subscription_service.go 套餐订阅服务实现
// 处理套餐订阅的完整生命周期：激活、撤销、延期、用量进度查询。
// 核心设计：激活套餐时会"桥接"创建 UserSubscription，使网关中间件
// 无需感知 Bundle 层即可正常执行额度检查。

package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// BundleSubscriptionService 套餐订阅服务，管理订阅的完整生命周期
// BundleSubscriptionService handles bundle subscription lifecycle.
type BundleSubscriptionService struct {
	bundleSubRepo BundleSubscriptionRepository
	planRepo      BundlePlanRepository
	usageRepo     BundleUsageRepository
	userSubRepo   UserSubscriptionRepository
	cache         BillingCache
	entClient     *dbent.Client
}

// NewBundleSubscriptionService 创建套餐订阅服务实例
// NewBundleSubscriptionService creates a new BundleSubscriptionService.
func NewBundleSubscriptionService(
	bundleSubRepo BundleSubscriptionRepository,
	planRepo BundlePlanRepository,
	usageRepo BundleUsageRepository,
	userSubRepo UserSubscriptionRepository,
	cache BillingCache,
	entClient *dbent.Client,
) *BundleSubscriptionService {
	return &BundleSubscriptionService{
		bundleSubRepo: bundleSubRepo,
		planRepo:      planRepo,
		usageRepo:     usageRepo,
		userSubRepo:   userSubRepo,
		cache:         cache,
		entClient:     entClient,
	}
}

// withTx 在数据库事务中执行套餐写操作（激活 / 撤销 / 延期），保证 BundleSubscription 状态变更
// 与桥接 UserSubscription 同步原子提交，任一失败整体回滚，杜绝状态不一致残留。
// entClient 为 nil 时（单元测试）退化为无事务直执行。
// withTx runs the bundle writes inside a DB transaction so partial failures roll back cleanly.
// Falls back to direct execution when entClient is nil.
func (s *BundleSubscriptionService) withTx(ctx context.Context, fn func(context.Context) error) error {
	if s.entClient == nil {
		return fn(ctx)
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		// entClient 已处于一个事务中（如集成测试的隔离事务 / 外层调用方的事务）：
		// 复用当前 ctx 不再嵌套开事务，repo 通过 clientFromContext 自动 join 既有事务。
		if errors.Is(err, dbent.ErrTxStarted) {
			return fn(ctx)
		}
		return fmt.Errorf("begin transaction: %w", err)
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	if err := fn(txCtx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// ActivateBundleRequest 激活套餐的输入 DTO
// ActivateBundleRequest is the input DTO for activating a bundle for a user.
type ActivateBundleRequest struct {
	UserID int64
	PlanID int64
	Source string // purchase, redeem, admin_assign
}

// ActivateBundle 激活套餐：检查冲突 → 加载计划 → 创建订阅 → 桥接 UserSubscription。
// 全流程在单个数据库事务内完成（withActivationTx），任一步失败整体回滚，杜绝半激活残留；
// 缓存失效在事务提交后执行，避免回滚后脏失效。
// ActivateBundle creates a bundle subscription and bridges per-group UserSubscriptions.
func (s *BundleSubscriptionService) ActivateBundle(ctx context.Context, req *ActivateBundleRequest) (*BundleSubscription, error) {
	if req == nil {
		return nil, ErrBundleNotFound
	}

	var activated *BundleSubscription
	if err := s.withTx(ctx, func(txCtx context.Context) error {
		// 1. Check user has no active bundle subscription.
		// Admin assignments bypass the conflict check: revoke existing active bundles first.
		activeBundles, err := s.bundleSubRepo.GetActiveByUserID(txCtx, req.UserID)
		if err != nil {
			return fmt.Errorf("check active bundles: %w", err)
		}
		if len(activeBundles) > 0 {
			if req.Source == BundleSourceAdminAssign {
				// Revoke all existing active bundles before activating the new one.
				// Pass txCtx so revoke + activate share one transaction.
				for _, existing := range activeBundles {
					if err := s.RevokeBundle(txCtx, existing.ID); err != nil {
						return fmt.Errorf("revoke existing bundle %d for admin assign: %w", existing.ID, err)
					}
				}
			} else {
				return ErrBundleConflict
			}
		}

		// 2. Load plan and validate.
		plan, err := s.planRepo.GetByID(txCtx, req.PlanID)
		if err != nil {
			return fmt.Errorf("load bundle plan: %w", err)
		}
		// Admin assignments can use any active plan, even if not publicly for sale.
		if req.Source != BundleSourceAdminAssign && (!plan.ForSale || plan.Status != domain.StatusActive) {
			return ErrBundlePlanDisabled
		}
		if plan.Status != domain.StatusActive {
			return ErrBundlePlanDisabled
		}

		// 3. Create BundleSubscription with snapshot concurrency/rpm.
		now := time.Now()
		expiresAt := now.AddDate(0, 0, plan.ValidityDays)

		bundleSub := &BundleSubscription{
			UserID:           req.UserID,
			PlanID:           req.PlanID,
			Status:           BundleStatusActive,
			StartsAt:         now,
			ExpiresAt:        expiresAt,
			ConcurrencyLimit: plan.ConcurrencyLimit,
			RPMLimit:         plan.RPMLimit,
			Source:           req.Source,
			Usages:           make([]BundleSubscriptionUsage, 0, len(plan.GroupQuotas)),
		}

		if err := s.bundleSubRepo.Create(txCtx, bundleSub); err != nil {
			return fmt.Errorf("create bundle subscription: %w", err)
		}

		// 4. For each GroupQuota, create BundleSubscriptionUsage + bridge UserSubscription.
		for _, gq := range plan.GroupQuotas {
			// Create usage tracker.
			usage := &BundleSubscriptionUsage{
				BundleSubscriptionID: bundleSub.ID,
				GroupID:              gq.GroupID,
				ModelPattern:         gq.ModelPattern,
				DailyWindowStart:     now,
				WeeklyWindowStart:    now,
				MonthlyWindowStart:   now,
			}
			if err := s.usageRepo.Create(txCtx, usage); err != nil {
				return fmt.Errorf("create bundle usage for group %d: %w", gq.GroupID, err)
			}
			bundleSub.Usages = append(bundleSub.Usages, *usage)

			// Bridge: create UserSubscription linked to this bundle.
			bundleSubID := bundleSub.ID
			userSub := &UserSubscription{
				UserID:                 req.UserID,
				GroupID:                gq.GroupID,
				StartsAt:               now,
				ExpiresAt:              expiresAt,
				Status:                 domain.SubscriptionStatusActive,
				DailyUsageUSD:          0,
				WeeklyUsageUSD:         0,
				MonthlyUsageUSD:        0,
				BundleSubscriptionID:   &bundleSubID,
				DailyLimitUSD:          gq.DailyLimitUSD,
				WeeklyLimitUSD:         gq.WeeklyLimitUSD,
				MonthlyLimitUSD:        gq.MonthlyLimitUSD,
				DailyImageLimitCount:   gq.DailyImageLimitCount,
				WeeklyImageLimitCount:  gq.WeeklyImageLimitCount,
				MonthlyImageLimitCount: gq.MonthlyImageLimitCount,
				DailyVideoLimitCount:   gq.DailyVideoLimitCount,
				WeeklyVideoLimitCount:  gq.WeeklyVideoLimitCount,
				MonthlyVideoLimitCount: gq.MonthlyVideoLimitCount,
				Notes:                  fmt.Sprintf("Bridged from bundle plan %q (ID:%d)", plan.Name, plan.ID),
			}
			if err := s.userSubRepo.Create(txCtx, userSub); err != nil {
				return fmt.Errorf("bridge user subscription for group %d: %w", gq.GroupID, err)
			}
		}

		activated = bundleSub
		return nil
	}); err != nil {
		return nil, err
	}

	// Invalidate user bundle cache after the transaction commits (avoid dirty
	// invalidation if the transaction rolled back).
	if s.cache != nil {
		_ = s.cache.InvalidateBundleSubscriptionCache(ctx, req.UserID)
	}

	return activated, nil
}

// RevokeBundle 撤销套餐订阅，同时撤销关联的桥接 UserSubscription
// RevokeBundle revokes an active bundle subscription and its bridged UserSubscriptions.
func (s *BundleSubscriptionService) RevokeBundle(ctx context.Context, bundleSubID int64) error {
	bundleSub, err := s.bundleSubRepo.GetByIDWithUsages(ctx, bundleSubID)
	if err != nil {
		return fmt.Errorf("load bundle subscription: %w", err)
	}
	if bundleSub.Status != BundleStatusActive {
		return ErrBundleExpired
	}

	// 事务化：bundle 状态翻转 + 桥接 UserSubscription 同步原子提交，任一失败回滚，
	// 杜绝「bundle 已 revoked 但桥接 userSub 仍 active」的状态不一致（历史 bug L2）。
	if err := s.withTx(ctx, func(txCtx context.Context) error {
		if err := s.bundleSubRepo.UpdateStatus(txCtx, bundleSubID, BundleStatusRevoked); err != nil {
			return fmt.Errorf("revoke bundle subscription: %w", err)
		}
		return s.syncBridgedUserSubscriptions(txCtx, bundleSub.UserID, bundleSubID, func(sub *UserSubscription) error {
			return s.userSubRepo.UpdateStatus(txCtx, sub.ID, domain.SubscriptionStatusExpired)
		})
	}); err != nil {
		return err
	}

	// Invalidate cache after revocation (after tx commits — avoid dirty invalidation on rollback).
	if s.cache != nil {
		_ = s.cache.InvalidateBundleSubscriptionCache(ctx, bundleSub.UserID)
	}

	return nil
}

// GetUserActiveBundle 获取用户的活跃套餐订阅列表
// GetUserActiveBundle returns the active bundle subscription for a user.
func (s *BundleSubscriptionService) GetUserActiveBundle(ctx context.Context, userID int64) ([]BundleSubscription, error) {
	// Cache-aside: try Redis first.
	if s.cache != nil {
		cached, err := s.cache.GetBundleSubscriptionCache(ctx, userID)
		if err == nil && cached != nil {
			sub := BundleSubscription{
				ID:               cached.ID,
				UserID:           userID,
				PlanID:           cached.PlanID,
				Status:           cached.Status,
				StartsAt:         time.Unix(cached.StartsAt, 0),
				ExpiresAt:        time.Unix(cached.ExpiresAt, 0),
				ConcurrencyLimit: cached.ConcurrencyLimit,
				RPMLimit:         cached.RPMLimit,
				Source:           cached.Source,
			}
			// Load the full plan (including group_quotas) for cache path.
			if plan, planErr := s.planRepo.GetByID(ctx, cached.PlanID); planErr == nil && plan != nil {
				sub.Plan = plan
			}
			return []BundleSubscription{sub}, nil
		}
	}

	subs, err := s.bundleSubRepo.GetActiveByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get active bundles: %w", err)
	}

	// Write back to cache if found.
	if s.cache != nil && len(subs) > 0 {
		sub := subs[0]
		planName := ""
		tier := ""
		if sub.Plan != nil {
			planName = sub.Plan.Name
			tier = sub.Plan.Tier
		}
		cacheData := &BundleSubscriptionCacheData{
			ID:               sub.ID,
			PlanID:           sub.PlanID,
			PlanName:         planName,
			Tier:             tier,
			Status:           sub.Status,
			StartsAt:         sub.StartsAt.Unix(),
			ExpiresAt:        sub.ExpiresAt.Unix(),
			ConcurrencyLimit: sub.ConcurrencyLimit,
			RPMLimit:         sub.RPMLimit,
			Source:           sub.Source,
		}
		_ = s.cache.SetBundleSubscriptionCache(ctx, userID, cacheData, BundleSubCacheTTL)
	}

	return subs, nil
}

// GetBundleUsageProgress 获取套餐用量进度，限额从桥接的 UserSubscription 快照中读取（保证一致性）
// GetBundleUsageProgress returns usage progress for a bundle subscription.
// Limits are read from the bridged UserSubscriptions (snapshotted at activation time)
// rather than the latest plan, ensuring consistency with the actual active limits.
func (s *BundleSubscriptionService) GetBundleUsageProgress(ctx context.Context, bundleSubID int64) ([]BundleUsageProgress, error) {
	bundleSub, err := s.bundleSubRepo.GetByIDWithUsages(ctx, bundleSubID)
	if err != nil {
		return nil, fmt.Errorf("load bundle subscription: %w", err)
	}

	// Load bridged UserSubscriptions to get snapshotted limits per group.
	userSubs, err := s.userSubRepo.ListByUserID(ctx, bundleSub.UserID)
	if err != nil {
		return nil, fmt.Errorf("load user subscriptions: %w", err)
	}

	// Build a lookup for snapshotted limits (USD + count) + group info by groupID from bridged UserSubscriptions.
	// Both USD and count limits are read from the UserSubscription snapshot (consistency with activation time);
	// changing the plan after subscription does not affect already-subscribed users.
	type groupMeta struct {
		dailyLimit             float64
		weeklyLimit            float64
		monthlyLimit           float64
		dailyLimitCount        int
		weeklyLimitCount       int
		monthlyLimitCount      int
		dailyVideoLimitCount   int
		weeklyVideoLimitCount  int
		monthlyVideoLimitCount int
		groupName              string
		platform               string
	}
	metaMap := make(map[int64]groupMeta)
	for _, sub := range userSubs {
		if sub.BundleSubscriptionID != nil && *sub.BundleSubscriptionID == bundleSubID {
			name, platform := "", ""
			if sub.Group != nil {
				name = sub.Group.Name
				platform = sub.Group.Platform
			}
			metaMap[sub.GroupID] = groupMeta{
				dailyLimit:             sub.DailyLimitUSD,
				weeklyLimit:            sub.WeeklyLimitUSD,
				monthlyLimit:           sub.MonthlyLimitUSD,
				dailyLimitCount:        sub.DailyImageLimitCount,
				weeklyLimitCount:       sub.WeeklyImageLimitCount,
				monthlyLimitCount:      sub.MonthlyImageLimitCount,
				dailyVideoLimitCount:   sub.DailyVideoLimitCount,
				weeklyVideoLimitCount:  sub.WeeklyVideoLimitCount,
				monthlyVideoLimitCount: sub.MonthlyVideoLimitCount,
				groupName:              name,
				platform:               platform,
			}
		}
	}

	// Load plan to resolve quota_scope per (group, pattern); usage records don't store scope.
	scopeMap := make(map[string]string)
	if plan, pErr := s.planRepo.GetByID(ctx, bundleSub.PlanID); pErr == nil && plan != nil {
		for _, gq := range plan.GroupQuotas {
			scopeMap[fmt.Sprintf("%d|%s", gq.GroupID, gq.ModelPattern)] = gq.QuotaScope
		}
	}

	progress := make([]BundleUsageProgress, 0, len(bundleSub.Usages))
	for _, usage := range bundleSub.Usages {
		meta, hasMeta := metaMap[usage.GroupID]
		if !hasMeta {
			meta = groupMeta{} // zero limits = unlimited
		}
		progress = append(progress, BundleUsageProgress{
			GroupID:                usage.GroupID,
			GroupName:              meta.groupName,
			Platform:               meta.platform,
			QuotaScope:             scopeMap[fmt.Sprintf("%d|%s", usage.GroupID, usage.ModelPattern)],
			ModelPattern:           usage.ModelPattern,
			DailyImageUsageCount:   usage.DailyImageUsageCount,
			DailyUsageUSD:          usage.DailyUsageUSD,
			DailyImageLimitCount:   meta.dailyLimitCount,
			DailyLimitUSD:          meta.dailyLimit,
			WeeklyImageUsageCount:  usage.WeeklyImageUsageCount,
			WeeklyUsageUSD:         usage.WeeklyUsageUSD,
			WeeklyImageLimitCount:  meta.weeklyLimitCount,
			WeeklyLimitUSD:         meta.weeklyLimit,
			MonthlyImageUsageCount: usage.MonthlyImageUsageCount,
			MonthlyUsageUSD:        usage.MonthlyUsageUSD,
			MonthlyImageLimitCount: meta.monthlyLimitCount,
			MonthlyLimitUSD:        meta.monthlyLimit,
			DailyVideoUsageCount:   usage.DailyVideoUsageCount,
			DailyVideoLimitCount:   meta.dailyVideoLimitCount,
			WeeklyVideoUsageCount:  usage.WeeklyVideoUsageCount,
			WeeklyVideoLimitCount:  meta.weeklyVideoLimitCount,
			MonthlyVideoUsageCount: usage.MonthlyVideoUsageCount,
			MonthlyVideoLimitCount: meta.monthlyVideoLimitCount,
		})
	}
	return progress, nil
}

// List 分页查询套餐订阅，支持按用户ID和状态过滤
// List returns a paginated list of bundle subscriptions with optional filters.
func (s *BundleSubscriptionService) List(ctx context.Context, params pagination.PaginationParams, userID *int64, status string) ([]BundleSubscription, *pagination.PaginationResult, error) {
	subs, result, err := s.bundleSubRepo.List(ctx, params, userID, status)
	if err != nil {
		return nil, nil, fmt.Errorf("list bundle subscriptions: %w", err)
	}
	return subs, result, nil
}

// EnrichPlansForList 为订阅列表批量加载 Plan 信息（handler 层调用，planCache 去重避免 N+1）
// EnrichPlansForList batch-loads Plan info for a subscription list (called from handler layer).
func (s *BundleSubscriptionService) EnrichPlansForList(ctx context.Context, subs []BundleSubscription) {
	planCache := make(map[int64]*BundlePlan)
	for i := range subs {
		if subs[i].Plan != nil {
			continue
		}
		if plan, ok := planCache[subs[i].PlanID]; ok {
			subs[i].Plan = plan
			continue
		}
		plan, err := s.planRepo.GetByID(ctx, subs[i].PlanID)
		if err != nil {
			continue
		}
		planCache[subs[i].PlanID] = plan
		subs[i].Plan = plan
	}
}

// ExtendBundle 延长套餐订阅有效期，同时延长关联的桥接 UserSubscription
// ExtendBundle extends a bundle subscription's expiry by the given number of days.
func (s *BundleSubscriptionService) ExtendBundle(ctx context.Context, bundleSubID int64, days int) error {
	bundleSub, err := s.bundleSubRepo.GetByID(ctx, bundleSubID)
	if err != nil {
		return fmt.Errorf("load bundle subscription: %w", err)
	}
	if bundleSub.Status != BundleStatusActive {
		return ErrBundleExpired
	}

	newExpiry := bundleSub.ExpiresAt.AddDate(0, 0, days)
	// 事务化：bundle 到期延长 + 桥接 UserSubscription 同步原子提交，任一失败回滚，
	// 杜绝「bundle 已延期但桥接 userSub 未延期」的状态不一致（历史 bug L2）。
	if err := s.withTx(ctx, func(txCtx context.Context) error {
		if err := s.bundleSubRepo.UpdateExpiry(txCtx, bundleSubID, newExpiry); err != nil {
			return fmt.Errorf("extend bundle subscription: %w", err)
		}
		return s.syncBridgedUserSubscriptions(txCtx, bundleSub.UserID, bundleSubID, func(sub *UserSubscription) error {
			extendedExpiry := sub.ExpiresAt.AddDate(0, 0, days)
			return s.userSubRepo.ExtendExpiry(txCtx, sub.ID, extendedExpiry)
		})
	}); err != nil {
		return err
	}

	// Invalidate cache after extension (after tx commits).
	if s.cache != nil {
		_ = s.cache.InvalidateBundleSubscriptionCache(ctx, bundleSub.UserID)
	}

	return nil
}

// GetBundleByID returns a single bundle subscription by ID with plan info loaded.
// Used for bundle enrichment in subscription handler.
func (s *BundleSubscriptionService) GetBundleByID(ctx context.Context, bundleSubID int64) (*BundleSubscription, error) {
	sub, err := s.bundleSubRepo.GetByID(ctx, bundleSubID)
	if err != nil {
		return nil, err
	}
	// Load plan for tier/name info.
	if sub.Plan == nil {
		plan, err := s.planRepo.GetByID(ctx, sub.PlanID)
		if err == nil {
			sub.Plan = plan
		}
	}
	return sub, nil
}

// syncBridgedUserSubscriptions 查找并批量操作某套餐关联的所有桥接 UserSubscription
// syncBridgedUserSubscriptions finds all bridged UserSubscriptions for a bundle
// and applies the given mutation function to each one.
// syncBridgedUserSubscriptions 查找并批量操作某套餐关联的所有桥接 UserSubscription，
// 返回首个错误（不中断后续操作，但暴露不一致给调用方记录）。
func (s *BundleSubscriptionService) syncBridgedUserSubscriptions(ctx context.Context, userID, bundleSubID int64, mutFn func(*UserSubscription) error) error {
	userSubs, err := s.userSubRepo.ListByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("list user subscriptions for sync: %w", err)
	}
	var firstErr error
	for i := range userSubs {
		sub := &userSubs[i]
		if sub.BundleSubscriptionID != nil && *sub.BundleSubscriptionID == bundleSubID {
			if err := mutFn(sub); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
