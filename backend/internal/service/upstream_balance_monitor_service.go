package service

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

const (
	upstreamBalanceMonitorInterval = time.Hour
	upstreamBalanceMonitorLockKey  = "upstream:balance:monitor:leader"
	upstreamBalanceMonitorLockTTL  = 10 * time.Minute
	upstreamBalanceRefreshTimeout  = 30 * time.Second
	upstreamBalanceMonitorTimer    = "upstream:balance:monitor"
)

type UpstreamBalanceMonitorService struct {
	syncService *UpstreamPriceSyncService
	timingWheel *TimingWheelService
	lockCache   LeaderLockCache
	db          *sql.DB
	instanceID  string
	stopped     atomic.Bool
	stopOnce    sync.Once
}

func NewUpstreamBalanceMonitorService(syncService *UpstreamPriceSyncService, timingWheel *TimingWheelService, lockCache LeaderLockCache, db *sql.DB) *UpstreamBalanceMonitorService {
	return &UpstreamBalanceMonitorService{
		syncService: syncService,
		timingWheel: timingWheel,
		lockCache:   lockCache,
		db:          db,
		instanceID:  uuid.NewString(),
	}
}

func (s *UpstreamBalanceMonitorService) Start() {
	if s == nil || s.syncService == nil || s.timingWheel == nil {
		return
	}
	go s.runOnce()
	s.timingWheel.ScheduleRecurring(upstreamBalanceMonitorTimer, upstreamBalanceMonitorInterval, s.runOnce)
}

func (s *UpstreamBalanceMonitorService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		s.stopped.Store(true)
		if s.timingWheel != nil {
			s.timingWheel.Cancel(upstreamBalanceMonitorTimer)
		}
	})
}

func (s *UpstreamBalanceMonitorService) runOnce() {
	if s == nil || s.stopped.Load() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), upstreamBalanceMonitorLockTTL)
	defer cancel()
	release, ok := tryAcquireSingletonLeaderLock(ctx, s.lockCache, s.db, upstreamBalanceMonitorLockKey, s.instanceID, upstreamBalanceMonitorLockTTL)
	if !ok {
		return
	}
	defer release()

	configs, err := s.syncService.ListConfigs(ctx)
	if err != nil {
		slog.Error("failed to list upstream sources for balance monitor", "error", err)
		return
	}
	for i := range configs {
		cfgRec := &configs[i]
		if !cfgRec.Enabled || cfgRec.DashboardToken == "" {
			continue
		}
		refreshCtx, refreshCancel := context.WithTimeout(ctx, upstreamBalanceRefreshTimeout)
		_, refreshErr := s.syncService.RefreshBalance(refreshCtx, cfgRec.ID)
		refreshCancel()
		if refreshErr != nil {
			slog.Warn("upstream balance refresh failed", "source_id", cfgRec.ID, "source_name", cfgRec.Name, "error", refreshErr)
		}
	}
}
