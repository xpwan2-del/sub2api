// bundle_expiry_service.go 套餐过期检查后台服务
// 定期扫描已过期但仍为 active 状态的套餐订阅，批量更新为 expired。
// 同时处理关联的桥接 UserSubscription 的过期状态同步。

package service

import (
	"context"
	"log"
	"sync"
	"time"
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
	}
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
