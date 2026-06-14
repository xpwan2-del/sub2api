package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// upstream price 同步任务的运维标识与调度参数。
const (
	upstreamPriceSyncJobName = "upstream_price_sync"

	// 轮询节奏：每分钟检查一次有哪些 source 到期。具体每个 source 的实际同步频率
	// 由 source.SyncIntervalMinutes 决定（syncDueSources 只挑选到期者）。
	upstreamPriceSyncTickInterval = 1 * time.Minute

	// 单个 source 同步的整体超时（含 HTTP 拉取 + 解析 + 持久化）。
	upstreamPriceSyncSourceTimeout = 30 * time.Second

	upstreamPriceSyncHeartbeatTimeout = 2 * time.Second

	// Redis leader lock：多实例部署时只让一个实例跑同步，避免重复拉取/重复告警。
	upstreamPriceSyncLeaderLockKey = "ops:upstream_price_sync:leader"
	upstreamPriceSyncLeaderLockTTL  = 90 * time.Second

	// 默认同步间隔（分钟）。source.SyncIntervalMinutes<=0 时回退到此值。
	upstreamPriceSyncDefaultIntervalMinutes = 60

	// severity 定级阈值（按 |InputDeltaPct| 的最大值）。
	upstreamPriceSyncSeverityCritical = 0.20 // >20%
	upstreamPriceSyncSeverityWarning  = 0.05 // >5%

	// admin 通知类型常量（写入 admin_notifications.type）。
	upstreamPriceChangeNotificationType = "upstream_price_change"
)

// UpstreamPriceSyncConfig 携带同步服务的可调参数（便于测试与未来扩展）。
type UpstreamPriceSyncConfig struct {
	// TickInterval 调度器轮询间隔。<=0 时用 upstreamPriceSyncTickInterval。
	TickInterval time.Duration
	// DefaultIntervalMinutes source.SyncIntervalMinutes<=0 时的回退同步间隔。
	// <=0 时用 upstreamPriceSyncDefaultIntervalMinutes。
	DefaultIntervalMinutes int
	// FrontendURL 站点基础 URL（来自 server.frontend_url 配置），用于把告警邮件里的
	// target_link 拼成绝对 URL（站内通知仍可用相对路径，前端 bell 会处理两者）。
	// 留空时 target_link 保持相对路径（站内通知可用，邮件链接不可点击）。
	FrontendURL string
}

// GroupRateReader 读取某本地模型相关 group 的代表性计费倍率。
//
// 用于建议值计算（CalcSuggestion.CurrentMultiplier）。
// 实现可取该模型涉及的第一个 group 的 rate_multiplier；找不到时返回 (1.0, nil)。
// 这是一个"尽力而为"的代表性值——精确的逐 group 倍率由 apply 阶段处理。
type GroupRateReader interface {
	// RateMultiplierForModel 返回 localModelName 相关 group 的代表性倍率。
	// 找不到关联 group 时返回 (1.0, nil)（默认倍率，不阻塞建议值生成）。
	RateMultiplierForModel(ctx context.Context, localModelName string) (float64, error)
}

// AlertRecipientReader 读取上游价格变动告警的邮件收件人列表。
//
// 真实实现从系统设置（ops 通知收件人）读取；此处仅定义接口以便测试 mock。
type AlertRecipientReader interface {
	ListAlertRecipients(ctx context.Context) ([]string, error)
}

// UpstreamPriceSyncService 上游价格同步核心编排服务。
//
// 把前 9 任务的零件串成自动同步闭环：
//  1. 定时（每分钟 tick）→ 抢 Redis leader lock → syncDueSources
//  2. syncDueSources 挑选到期 source（last_sync_at + interval < now）
//  3. SyncSource：HTTP 拉取 → sha256 判变 → 解析 → diff → 建议值 → 持久化 → 聚合告警
//
// 调度模式严格参照 OpsMetricsCollector：Start/Stop/run/tryAcquireLeaderLock/心跳。
type UpstreamPriceSyncService struct {
	priceRepo     UpstreamPriceRepository
	notifService  *AdminNotificationService
	emailService  *NotificationEmailService
	groupReader   GroupRateReader
	recipientReader AlertRecipientReader
	opsRepo       OpsRepository
	redis         *redis.Client
	encryptor     SecretEncryptor
	httpClient    HTTPUpstream
	cfg           UpstreamPriceSyncConfig

	instanceID string
	stopCh     chan struct{}
	startOnce  sync.Once
	stopOnce   sync.Once

	skipLogMu sync.Mutex
	skipLogAt time.Time
}

// NewUpstreamPriceSyncService 构造同步服务。
//
// notifService / emailService / groupReader / recipientReader / opsRepo / redis 可为 nil：
//   - redis==nil：不开 leader lock（单实例开发场景，fail-open）
//   - opsRepo==nil：不写心跳
//   - emailService==nil 或 recipientReader==nil：跳过邮件，仅写 admin 通知
//   - groupReader==nil：建议值的 CurrentMultiplier 用默认 1.0
func NewUpstreamPriceSyncService(
	priceRepo UpstreamPriceRepository,
	notifService *AdminNotificationService,
	emailService *NotificationEmailService,
	groupReader GroupRateReader,
	recipientReader AlertRecipientReader,
	opsRepo OpsRepository,
	redisClient *redis.Client,
	encryptor SecretEncryptor,
	httpClient HTTPUpstream,
	cfg UpstreamPriceSyncConfig,
) *UpstreamPriceSyncService {
	if cfg.TickInterval <= 0 {
		cfg.TickInterval = upstreamPriceSyncTickInterval
	}
	if cfg.DefaultIntervalMinutes <= 0 {
		cfg.DefaultIntervalMinutes = upstreamPriceSyncDefaultIntervalMinutes
	}
	return &UpstreamPriceSyncService{
		priceRepo:       priceRepo,
		notifService:    notifService,
		emailService:    emailService,
		groupReader:     groupReader,
		recipientReader: recipientReader,
		opsRepo:         opsRepo,
		redis:           redisClient,
		encryptor:       encryptor,
		httpClient:      httpClient,
		cfg:             cfg,
		instanceID:      uuid.NewString(),
	}
}

// Start 启动后台 goroutine 跑 run()。幂等（多次调用只启动一次）。
func (s *UpstreamPriceSyncService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		if s.stopCh == nil {
			s.stopCh = make(chan struct{})
		}
		go s.run()
	})
}

// Stop 关闭调度循环。幂等。
func (s *UpstreamPriceSyncService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.stopCh != nil {
			close(s.stopCh)
		}
	})
}

// run 定时循环：每个 TickInterval → 抢 leader lock → syncDueSources。
func (s *UpstreamPriceSyncService) run() {
	// 启动后立即跑一次，让价格数据尽快可用。
	s.tick()

	interval := s.cfg.TickInterval
	for {
		timer := time.NewTimer(interval)
		select {
		case <-timer.C:
			s.tick()
		case <-s.stopCh:
			timer.Stop()
			return
		}
	}
}

// tick 单次调度：抢锁后同步到期 source，最后写心跳。
func (s *UpstreamPriceSyncService) tick() {
	if s == nil || s.priceRepo == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), upstreamPriceSyncSourceTimeout*5)
	defer cancel()

	release, ok := s.tryAcquireLeaderLock(ctx)
	if !ok {
		return
	}
	if release != nil {
		defer release()
	}

	startedAt := time.Now().UTC()
	err := s.syncDueSources(ctx)
	finishedAt := time.Now().UTC()
	dur := finishedAt.Sub(startedAt).Milliseconds()

	s.recordHeartbeat(ctx, err, startedAt, finishedAt, dur)
}

// syncDueSources 列出全部启用 source，对到期的逐个 SyncSource。
//
// 到期判定：last_sync_at==nil（从未同步）或 now - last_sync_at >= interval。
// 单个 source 失败不影响其他 source（逐个 try）。
func (s *UpstreamPriceSyncService) syncDueSources(ctx context.Context) error {
	sources, err := s.priceRepo.ListEnabledSources(ctx)
	if err != nil {
		return fmt.Errorf("list enabled sources: %w", err)
	}

	now := time.Now().UTC()
	var firstErr error
	for _, src := range sources {
		if src == nil || src.ID == 0 {
			continue
		}
		if !s.isDue(src, now) {
			continue
		}
		// 单个 source 用独立 ctx，避免一个慢源耗尽整体预算。
		srcCtx, srcCancel := context.WithTimeout(ctx, upstreamPriceSyncSourceTimeout)
		if syncErr := s.SyncSource(srcCtx, src.ID); syncErr != nil {
			slog.Warn("upstream price sync source failed",
				"source_id", src.ID, "source_name", src.Name, "err", syncErr)
			if firstErr == nil {
				firstErr = syncErr
			}
		}
		srcCancel()
	}
	return firstErr
}

// isDue 判断 source 是否到期需要同步。
func (s *UpstreamPriceSyncService) isDue(src *dbent.UpstreamPriceSource, now time.Time) bool {
	interval := time.Duration(src.SyncIntervalMinutes) * time.Minute
	if interval <= 0 {
		interval = time.Duration(s.cfg.DefaultIntervalMinutes) * time.Minute
	}
	if interval <= 0 {
		interval = time.Duration(upstreamPriceSyncDefaultIntervalMinutes) * time.Minute
	}
	if src.LastSyncAt == nil {
		return true
	}
	return now.Sub(*src.LastSyncAt) >= interval
}

// SyncSource 同步单个源。可被 run() 自动触发，也可被 handler 手动触发。
//
// 流程：
//  1. GetSource → 若 disabled 或不存在直接返回
//  2. HTTP GET base_url + pricing_endpoint（Bearer <Decrypt(api_key)>）→ raw body
//  3. hash = sha256(raw)；若 == source.LastHash → 仅更新 success 状态，返回（未变）
//  4. ParserByType(parserType).Parse(raw, cfg)
//  5. oldMap = ListAllModelPricesAsMap；DiffPrices(curr, prev)
//  6. 对每个 change 算建议值（CalcSuggestion）
//  7. ReplaceModelPrices + InsertChanges
//  8. 有变动 → emitAlert
//  9. UpdateSourceSyncResult(success)
func (s *UpstreamPriceSyncService) SyncSource(ctx context.Context, sourceID int64) error {
	if sourceID == 0 {
		return errors.New("source id is required")
	}

	src, err := s.priceRepo.GetSource(ctx, sourceID)
	if err != nil {
		return fmt.Errorf("get source: %w", err)
	}
	if src == nil || !src.Enabled {
		return nil
	}

	// 1) HTTP 拉取。
	raw, fetchErr := s.fetchRaw(ctx, src)
	if fetchErr != nil {
		s.markFailed(ctx, src, fmt.Sprintf("fetch: %v", fetchErr))
		return fmt.Errorf("fetch upstream pricing: %w", fetchErr)
	}

	// 2) hash 判变。
	hash := sha256Hex(raw)
	if hash == src.LastHash {
		// 内容未变：仅刷新同步时间与状态，跳过解析/diff/持久化。
		s.markSuccess(ctx, src.ID, hash, "")
		return nil
	}

	// 3) 解析。
	aliasMap := src.ModelAliasMap
	if aliasMap == nil {
		aliasMap = map[string]string{}
	}
	prices, parseErr := ParserByType(src.ParserType).Parse(raw, ParserConfig{AliasMap: aliasMap})
	if parseErr != nil {
		s.markFailed(ctx, src, fmt.Sprintf("parse: %v", parseErr))
		return fmt.Errorf("parse upstream response: %w", parseErr)
	}

	// 4) diff。
	oldMap, err := s.priceRepo.ListAllModelPricesAsMap(ctx, src.ID)
	if err != nil {
		s.markFailed(ctx, src, fmt.Sprintf("load prev prices: %v", err))
		return fmt.Errorf("load prev model prices: %w", err)
	}
	currSnap := pricesToSnapshotMap(prices)
	prevSnap := modelPricesToSnapshotMap(oldMap)
	changes := DiffPrices(currSnap, prevSnap)

	// 5) 建议值。
	now := time.Now().UTC()
	changeRows := s.buildChangeRows(ctx, src, changes, now)

	// 6) 持久化：先替换参考价快照，再写变动记录。
	dbPrices := toDBModelPrices(src.ID, prices, now)
	if err := s.priceRepo.ReplaceModelPrices(ctx, src.ID, dbPrices); err != nil {
		s.markFailed(ctx, src, fmt.Sprintf("replace prices: %v", err))
		return fmt.Errorf("replace model prices: %w", err)
	}
	if len(changeRows) > 0 {
		if err := s.priceRepo.InsertChanges(ctx, changeRows); err != nil {
			// 参考价已更新但变动记录写入失败：记 partial 状态。
			s.markFailed(ctx, src, fmt.Sprintf("insert changes: %v", err))
			return fmt.Errorf("insert price changes: %w", err)
		}
	}

	// 7) 告警（仅在有变动时）。
	if len(changeRows) > 0 {
		s.emitAlert(ctx, src, changeRows)
	}

	// 8) 成功状态。
	summary := fmt.Sprintf("%d models, %d changes", len(dbPrices), len(changeRows))
	s.markSuccess(ctx, src.ID, hash, summary)
	return nil
}

// fetchRaw 拉取上游定价接口的原始响应体。
func (s *UpstreamPriceSyncService) fetchRaw(ctx context.Context, src *dbent.UpstreamPriceSource) ([]byte, error) {
	targetURL, err := buildSourceURL(src.BaseURL, src.PricingEndpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid source url: %w", err)
	}
	plainKey, err := s.decryptAPIKey(src.APIKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt api key: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, upstreamPriceTestConnTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if plainKey != "" {
		req.Header.Set("Authorization", "Bearer "+plainKey)
	}

	resp, doErr := s.httpClient.Do(req, "", 0, 0)
	if doErr != nil {
		return nil, doErr
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // 8MB 上限
	if readErr != nil {
		return nil, readErr
	}
	return body, nil
}

// decryptAPIKey 解密 api_key。空串返回空串。
func (s *UpstreamPriceSyncService) decryptAPIKey(cipher string) (string, error) {
	if strings.TrimSpace(cipher) == "" {
		return "", nil
	}
	if s.encryptor == nil {
		return "", errors.New("encryptor is not configured")
	}
	return s.encryptor.Decrypt(cipher)
}

// buildChangeRows 把 PriceChange 列表转成 dbent.UpstreamPriceChange 入库行，
// 并为每个变动算建议值（CalcSuggestion）。
func (s *UpstreamPriceSyncService) buildChangeRows(
	ctx context.Context,
	src *dbent.UpstreamPriceSource,
	changes []PriceChange,
	now time.Time,
) []*dbent.UpstreamPriceChange {
	rows := make([]*dbent.UpstreamPriceChange, 0, len(changes))
	for _, ch := range changes {
		localName := ch.ModelName
		// 从快照里查 local_model_name 更准确（diff 只带 ModelName）。
		// 此处用 ModelName 作 fallback。
		row := &dbent.UpstreamPriceChange{
			SourceID:        src.ID,
			ModelName:       ch.ModelName,
			LocalModelName:  localName,
			ChangeType:      string(ch.Type),
			PrevInputPrice:  ch.PrevInputPrice,
			PrevOutputPrice: ch.PrevOutputPrice,
			CurrInputPrice:  ch.CurrInputPrice,
			CurrOutputPrice: ch.CurrOutputPrice,
			InputDeltaPct:   ch.InputDeltaPct,
			OutputDeltaPct:  ch.OutputDeltaPct,
			DetectedAt:      now,
			Notified:        false,
			Status:          UpstreamPriceChangeStatusPending,
		}

		// 建议值：仅对有成本变化的（price_up/price_down/new_model）计算。
		// removed 无新成本，跳过建议值。
		if ch.Type == PriceChangeUp || ch.Type == PriceChangeDown || ch.Type == PriceChangeNew {
			oldIn := 0.0
			if ch.PrevInputPrice != nil {
				oldIn = *ch.PrevInputPrice
			}
			mult := s.resolveMultiplier(ctx, localName)
			sug := CalcSuggestion(SuggestionInput{
				OldInputPrice:     oldIn,
				NewInputPrice:     ch.CurrInputPrice,
				CurrentMultiplier: mult,
			})
			row.SuggestedInputPrice = sug.SuggestedInputPrice
			// 建议输出价跟输入价同比例变化（简化：用 output 的旧/新比）。
			row.SuggestedOutputPrice = ch.CurrOutputPrice
			row.SuggestedMultiplier = sug.SuggestedMultiplier
		}

		rows = append(rows, row)
	}
	return rows
}

// resolveMultiplier 读取某 local model 的代表性 group 倍率；失败/无 reader 时回退 1.0。
func (s *UpstreamPriceSyncService) resolveMultiplier(ctx context.Context, localModelName string) float64 {
	if s.groupReader == nil || strings.TrimSpace(localModelName) == "" {
		return 1.0
	}
	mult, err := s.groupReader.RateMultiplierForModel(ctx, localModelName)
	if err != nil || mult <= 0 {
		return 1.0
	}
	return mult
}

// emitAlert 聚合一次同步产生的全部变动为一条 admin 通知 + 一封邮件，并标记 notified。
func (s *UpstreamPriceSyncService) emitAlert(ctx context.Context, src *dbent.UpstreamPriceSource, rows []*dbent.UpstreamPriceChange) {
	if len(rows) == 0 {
		return
	}

	severity := classifySeverity(rows)
	title, content := buildChangeReport(src, rows)
	targetLink := buildTargetLink(s.cfg.FrontendURL, src.ID)
	changeIDs := make([]int64, 0, len(rows))
	for _, r := range rows {
		if r.ID > 0 {
			changeIDs = append(changeIDs, r.ID)
		}
	}

	// admin 站内通知（始终写）。
	if s.notifService != nil {
		if _, err := s.notifService.Create(ctx,
			upstreamPriceChangeNotificationType, title, content, severity, targetLink, changeIDs); err != nil {
			slog.Warn("upstream price sync: create admin notification failed", "source_id", src.ID, "err", err)
		}
	}

	// 邮件（可选）。
	if s.emailService != nil && s.recipientReader != nil {
		recipients, err := s.recipientReader.ListAlertRecipients(ctx)
		if err != nil {
			slog.Warn("upstream price sync: list alert recipients failed", "source_id", src.ID, "err", err)
		} else {
			vars := map[string]string{
				"source_name":    src.Name,
				"change_summary": summarizeChanges(rows),
				"change_details": content,
				"target_link":    targetLink,
			}
			for _, rcpt := range recipients {
				if strings.TrimSpace(rcpt) == "" {
					continue
				}
				if err := s.emailService.Send(ctx, NotificationEmailSendInput{
					Event:          NotificationEmailEventUpstreamPriceChange,
					RecipientEmail: rcpt,
					SourceType:     "upstream_price_source",
					SourceID:       fmt.Sprintf("%d", src.ID),
					Variables:      vars,
				}); err != nil {
					slog.Warn("upstream price sync: send alert email failed",
						"source_id", src.ID, "recipient", rcpt, "err", err)
				}
			}
		}
	}

	// 标记已通知（即使邮件失败，站内通知已写即视为 notified=true）。
	if len(changeIDs) > 0 {
		if err := s.priceRepo.MarkChangesNotified(ctx, changeIDs); err != nil {
			slog.Warn("upstream price sync: mark changes notified failed", "source_id", src.ID, "err", err)
		}
	}
}

// buildTargetLink 生成价格变动详情链接。配置了 frontendURL（server.frontend_url）
// 时拼成绝对 URL（邮件可点击），否则回退到相对路径（仅站内通知可用）。
// 与 NotificationEmailService.baseURL 的语义保持一致，前端 AdminNotificationBell
// 对绝对/相对链接都已做处理（http 开头开新窗，否则 router.push）。
func buildTargetLink(frontendURL string, sourceID int64) string {
	path := fmt.Sprintf("/admin/upstream-pricing/changes?source=%d", sourceID)
	base := strings.TrimSpace(frontendURL)
	if base == "" {
		return path
	}
	return strings.TrimRight(base, "/") + path
}

// markSuccess 写成功同步状态。
func (s *UpstreamPriceSyncService) markSuccess(ctx context.Context, sourceID int64, hash, summary string) {
	if err := s.priceRepo.UpdateSourceSyncResult(ctx, sourceID, UpstreamSyncStatusSuccess, hash, "", time.Now().UTC()); err != nil {
		slog.Warn("upstream price sync: update source success result failed", "source_id", sourceID, "err", err)
	}
}

// markFailed 写失败同步状态。
func (s *UpstreamPriceSyncService) markFailed(ctx context.Context, src *dbent.UpstreamPriceSource, reason string) {
	msg := truncateString(reason, 2048)
	if err := s.priceRepo.UpdateSourceSyncResult(ctx, src.ID, UpstreamSyncStatusFailed, src.LastHash, msg, time.Now().UTC()); err != nil {
		slog.Warn("upstream price sync: update source failed result failed", "source_id", src.ID, "err", err)
	}
}

// recordHeartbeat 写一次调度的运维心跳（成功/失败均写）。
func (s *UpstreamPriceSyncService) recordHeartbeat(ctx context.Context, runErr error, startedAt, finishedAt time.Time, durMs int64) {
	if s.opsRepo == nil {
		return
	}
	hbCtx, cancel := context.WithTimeout(context.Background(), upstreamPriceSyncHeartbeatTimeout)
	defer cancel()

	runAt := startedAt
	dur := durMs
	if runErr != nil {
		msg := truncateString(runErr.Error(), 2048)
		errAt := finishedAt
		_ = s.opsRepo.UpsertJobHeartbeat(hbCtx, &OpsUpsertJobHeartbeatInput{
			JobName:        upstreamPriceSyncJobName,
			LastRunAt:      &runAt,
			LastErrorAt:    &errAt,
			LastError:      &msg,
			LastDurationMs: &dur,
		})
		return
	}
	successAt := finishedAt
	_ = s.opsRepo.UpsertJobHeartbeat(hbCtx, &OpsUpsertJobHeartbeatInput{
		JobName:        upstreamPriceSyncJobName,
		LastRunAt:      &runAt,
		LastSuccessAt:  &successAt,
		LastDurationMs: &dur,
	})
}

// tryAcquireLeaderLock 抢 Redis leader lock。参照 OpsMetricsCollector 模式。
//
// redis==nil → fail-open（返回 nil release, ok=true，不开锁）。
// 抢不到 → ok=false（跳过本次 tick）。
func (s *UpstreamPriceSyncService) tryAcquireLeaderLock(ctx context.Context) (func(), bool) {
	if s == nil || s.redis == nil {
		return nil, true
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ok, err := s.redis.SetNX(ctx, upstreamPriceSyncLeaderLockKey, s.instanceID, upstreamPriceSyncLeaderLockTTL).Result()
	if err != nil {
		// Redis 故障：fail-open（不阻塞同步，单实例或容忍重复的场景可接受）。
		slog.Warn("upstream price sync: leader lock acquire error, fail-open", "err", err)
		return nil, true
	}
	if !ok {
		s.maybeLogSkip()
		return nil, false
	}
	release := func() {
		rCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = upstreamPriceSyncReleaseScript.Run(rCtx, s.redis, []string{upstreamPriceSyncLeaderLockKey}, s.instanceID).Result()
	}
	return release, true
}

func (s *UpstreamPriceSyncService) maybeLogSkip() {
	s.skipLogMu.Lock()
	defer s.skipLogMu.Unlock()
	now := time.Now()
	if !s.skipLogAt.IsZero() && now.Sub(s.skipLogAt) < time.Minute {
		return
	}
	s.skipLogAt = now
	slog.Info("upstream price sync: leader lock held by another instance; skipping")
}

var upstreamPriceSyncReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

// ===== 辅助函数 =====

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// pricesToSnapshotMap 把解析出的 UpstreamModelPrice 列表转为 diff 用的快照 map。
func pricesToSnapshotMap(prices []UpstreamModelPrice) map[string]PriceSnapshot {
	m := make(map[string]PriceSnapshot, len(prices))
	for _, p := range prices {
		m[p.ModelName] = PriceSnapshot{
			InputPrice:  p.InputPrice,
			OutputPrice: p.OutputPrice,
		}
	}
	return m
}

// modelPricesToSnapshotMap 把库里的 UpstreamModelPrice 转为 diff 用的快照 map。
func modelPricesToSnapshotMap(m map[string]*dbent.UpstreamModelPrice) map[string]PriceSnapshot {
	out := make(map[string]PriceSnapshot, len(m))
	for name, p := range m {
		if p == nil {
			continue
		}
		out[name] = PriceSnapshot{
			InputPrice:  p.InputPrice,
			OutputPrice: p.OutputPrice,
		}
	}
	return out
}

// toDBModelPrices 把解析结果转成持久化实体。
func toDBModelPrices(sourceID int64, prices []UpstreamModelPrice, fetchedAt time.Time) []*dbent.UpstreamModelPrice {
	out := make([]*dbent.UpstreamModelPrice, 0, len(prices))
	for _, p := range prices {
		out = append(out, &dbent.UpstreamModelPrice{
			SourceID:        sourceID,
			ModelName:       p.ModelName,
			LocalModelName:  p.LocalModelName,
			InputPrice:      p.InputPrice,
			OutputPrice:     p.OutputPrice,
			CacheWritePrice: p.CacheWritePrice,
			CacheReadPrice:  p.CacheReadPrice,
			Currency:        "USD",
			RawPayload:      p.RawPayload,
			FetchedAt:       fetchedAt,
		})
	}
	return out
}

// classifySeverity 按最大 |InputDeltaPct| 定级。
func classifySeverity(rows []*dbent.UpstreamPriceChange) string {
	var maxAbs float64
	for _, r := range rows {
		d := r.InputDeltaPct
		if d < 0 {
			d = -d
		}
		if d > maxAbs {
			maxAbs = d
		}
	}
	switch {
	case maxAbs > upstreamPriceSyncSeverityCritical:
		return "critical"
	case maxAbs > upstreamPriceSyncSeverityWarning:
		return "warning"
	default:
		return "info"
	}
}

// summarizeChanges 生成"N涨 M跌 K新增"摘要。
func summarizeChanges(rows []*dbent.UpstreamPriceChange) string {
	var up, down, added, gone int
	for _, r := range rows {
		switch PriceChangeType(r.ChangeType) {
		case PriceChangeUp:
			up++
		case PriceChangeDown:
			down++
		case PriceChangeNew:
			added++
		case PriceChangeGone:
			gone++
		}
	}
	return fmt.Sprintf("%d up, %d down, %d new, %d removed", up, down, added, gone)
}

// buildChangeReport 构造 Markdown 明细表（admin 通知 content + 邮件 change_details 共用）。
func buildChangeReport(src *dbent.UpstreamPriceSource, rows []*dbent.UpstreamPriceChange) (title, content string) {
	summary := summarizeChanges(rows)
	title = fmt.Sprintf("%s price changes: %s", src.Name, summary)

	var b strings.Builder
	_, _ = b.WriteString("| model | prev in -> curr in | in delta% | suggested in |\n")
	_, _ = b.WriteString("|---|---|---|---|\n")
	for _, r := range rows {
		prev := "-"
		if r.PrevInputPrice != nil {
			prev = fmt.Sprintf("%.6g", *r.PrevInputPrice)
		}
		_, _ = fmt.Fprintf(&b, "| %s | %s -> %.6g | %.2f%% | %.6g |\n",
			r.ModelName, prev, r.CurrInputPrice, r.InputDeltaPct*100, r.SuggestedInputPrice)
	}
	content = b.String()
	return title, content
}
