// bundle_expiry_service.go 套餐过期检查后台服务
// 定期扫描已过期但仍为 active 状态的套餐订阅，批量更新为 expired。
// 同时处理关联的桥接 UserSubscription 的过期状态同步。

package service

import (
	"context"
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// bundleExpiryLeaderLockKey gates the per-cycle expiry sweep so that only one
	// instance runs BatchUpdateExpiredStatus + bridged-subscription sync, avoiding
	// N× redundant full-table scans across instances. The sweep is idempotent, so
	// this is an optimization, not a correctness requirement.
	bundleExpiryLeaderLockKey = "bundle:expiry:sweep:leader"
	// bundleExpiryLeaderLockTTL bounds crash recovery; keep it above one cycle.
	bundleExpiryLeaderLockTTL = 5 * time.Minute
)

// BundleExpiryService 套餐过期检查服务，以后台定时任务方式运行
// BundleExpiryService periodically marks expired bundle subscriptions.
type BundleExpiryService struct {
	bundleUsageRepo BundleUsageRepository
	bundleSubRepo   BundleSubscriptionRepository
	userSubRepo     UserSubscriptionRepository
	interval        time.Duration
	stopCh          chan struct{}
	stopOnce        sync.Once
	wg              sync.WaitGroup

	lockCache  LeaderLockCache
	db         *sql.DB
	instanceID string
}

// NewBundleExpiryService 创建过期检查服务，interval 控制扫描间隔
// NewBundleExpiryService creates a new BundleExpiryService.
func NewBundleExpiryService(
	bundleUsageRepo BundleUsageRepository,
	bundleSubRepo BundleSubscriptionRepository,
	userSubRepo UserSubscriptionRepository,
	interval time.Duration,
) *BundleExpiryService {
	return &BundleExpiryService{
		bundleUsageRepo: bundleUsageRepo,
		bundleSubRepo:   bundleSubRepo,
		userSubRepo:     userSubRepo,
		interval:        interval,
		stopCh:          make(chan struct{}),
		instanceID:      uuid.NewString(),
	}
}

// SetLeaderLock 注入 leader 锁缓存与 DB，用于多实例选主，仅 leader 执行周期过期扫描。
// SetLeaderLock injects the leader-lock cache and DB used to elect a single
// instance for the periodic expiry sweep. When both are nil the sweep runs
// ungated (single-instance / test behavior).
func (s *BundleExpiryService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache = lockCache
	s.db = db
}

// Start 启动后台过期检查定时器
// Start launches the background expiry ticker.
func (s *BundleExpiryService) Start() {
	if s == nil || s.bundleUsageRepo == nil || s.interval <= 0 {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.runOnce()
		for {
			select {
			case <-ticker.C:
				s.runOnce()
			case <-s.stopCh:
				return
			}
		}
	}()
}

// Stop 优雅关闭后台定时器
// Stop gracefully shuts down the expiry ticker.
func (s *BundleExpiryService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

// runOnce 执行一次过期扫描，将到期但仍 active 的订阅批量标记为 expired，
// 并同步撤销关联的桥接 UserSubscription。
func (s *BundleExpiryService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 多实例选主：仅 leader 执行批量过期扫描。BatchUpdateExpiredStatus 与桥接订阅同步
	// 均为幂等 UPDATE，加锁是为避免 N 个实例每周期重复全表扫描。
	// Multi-instance guard: only the leader runs the batch expiry sweep.
	release, ok := tryAcquireSingletonLeaderLock(ctx, s.lockCache, s.db, bundleExpiryLeaderLockKey, s.instanceID, bundleExpiryLeaderLockTTL)
	if !ok {
		return
	}
	defer release()

	updated, err := s.bundleUsageRepo.BatchUpdateExpiredStatus(ctx)
	if err != nil {
		log.Printf("[BundleExpiry] Update expired bundle subscriptions failed: %v", err)
		return
	}
	if updated > 0 {
		log.Printf("[BundleExpiry] Updated %d expired bundle subscriptions", updated)
	}

	// Sync: expire bridged UserSubscriptions for any newly-expired bundles.
	s.syncExpiredBridgedUserSubscriptions(ctx)
}

// syncExpiredBridgedUserSubscriptions finds bundle subscriptions that just expired
// and expires their bridged UserSubscription records.
func (s *BundleExpiryService) syncExpiredBridgedUserSubscriptions(ctx context.Context) {
	// Use the existing batch expiry query to find expired bundle subscriptions.
	// We can look up UserSubscriptions linked to expired bundles and expire them.
	// For simplicity, we iterate user subscriptions in batches where bundle_subscription_id is set
	// and the bundle has expired status.
	count, err := s.userSubRepo.ExpireBridgedSubscriptionsForExpiredBundles(ctx)
	if err != nil {
		log.Printf("[BundleExpiry] Sync bridged user subscriptions failed: %v", err)
		return
	}
	if count > 0 {
		log.Printf("[BundleExpiry] Expired %d bridged user subscriptions", count)
	}
}
