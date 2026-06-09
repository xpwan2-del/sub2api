package service

import (
	"context"
	"log"
	"sync"
	"time"
)

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
}
