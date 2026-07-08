// bundle_usage_repo.go 套餐用量数据访问实现
// 基于 Ent ORM 实现 BundleUsageRepository 接口，
// 提供用量的创建、累加、查询和日/周/月窗口重置操作。

package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/bundlesubscription"
	"github.com/Wei-Shaw/sub2api/ent/bundlesubscriptionusage"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// bundleUsageRepository 套餐用量仓库实现
type bundleUsageRepository struct {
	client *dbent.Client
}

// NewBundleUsageRepository 创建套餐用量仓库
func NewBundleUsageRepository(client *dbent.Client) service.BundleUsageRepository {
	return &bundleUsageRepository{client: client}
}

// GetBySubscriptionAndGroup 按订阅ID和渠道组ID查询用量记录。记录不存在时返回 (nil, nil)
// （非错误）：调用方据此判断——AccumulateUsage 触发自愈建行（H2），CheckQuotaEligibility 视为满额度。
func (r *bundleUsageRepository) GetBySubscriptionAndGroup(ctx context.Context, subscriptionID, groupID int64, modelPattern string) (*service.BundleSubscriptionUsage, error) {
	client := clientFromContext(ctx, r.client)

	m, err := client.BundleSubscriptionUsage.Query().
		Where(
			bundlesubscriptionusage.BundleSubscriptionIDEQ(subscriptionID),
			bundlesubscriptionusage.GroupIDEQ(groupID),
			bundlesubscriptionusage.ModelPatternEQ(modelPattern),
		).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil
		}
		return nil, translatePersistenceError(err, nil, nil)
	}

	result := bundleSubscriptionUsageToService(m)
	return &result, nil
}

// GetOrCreateUsage 获取或自愈创建用量记录（修复 H2）。事务内查询 (subscriptionID,groupID,modelPattern)：
// 存在则原样返回（不修改累计值）；不存在则用 now 所在自然日 0 点初始化空 usage 的 window_start。并发安全
// ——并发自愈请求各自 INSERT 时，唯一约束 (subscription_id,group_id,model_pattern) 使其中一个冲突，冲突方重查返回已建行。
func (r *bundleUsageRepository) GetOrCreateUsage(ctx context.Context, bundleSubscriptionID, groupID int64, modelPattern string, now time.Time) (*service.BundleSubscriptionUsage, error) {
	var result *service.BundleSubscriptionUsage
	if err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		m, err := txClient.BundleSubscriptionUsage.Query().
			Where(
				bundlesubscriptionusage.BundleSubscriptionIDEQ(bundleSubscriptionID),
				bundlesubscriptionusage.GroupIDEQ(groupID),
				bundlesubscriptionusage.ModelPatternEQ(modelPattern),
			).
			Only(txCtx)
		if err == nil {
			usage := bundleSubscriptionUsageToService(m)
			result = &usage
			return nil
		}
		if !dbent.IsNotFound(err) {
			return translatePersistenceError(err, nil, nil)
		}

		// 不存在 → INSERT ON CONFLICT DO NOTHING（单条 SQL，并发冲突时不报错、不使事务进入
		// aborted 状态）。避免「Create 撞唯一约束 → PG 事务 aborted → 后续重查失败」的陷阱
		// （PG 规则：事务内任一语句报错后，后续语句全部失败直到 ROLLBACK）。
		// 与 user_platform_quota_repo.IncrementUsageWithReset 的 fail-open create 同范式。
		const insertSQL = `INSERT INTO bundle_subscription_usages
			(bundle_subscription_id, group_id, model_pattern,
			 daily_window_start, weekly_window_start, monthly_window_start)
			VALUES ($1, $2, $3, $4, $4, $4)
			ON CONFLICT (bundle_subscription_id, group_id, model_pattern) DO NOTHING`
		// window_start 对齐到自然日 0 点（与 rollBundleWindow 重置语义一致），保证 DB 值语义清晰。
		windowStart := service.BundleWindowStart(now)
		if _, err := txClient.ExecContext(txCtx, insertSQL,
			bundleSubscriptionID, groupID, modelPattern, windowStart,
		); err != nil {
			return translatePersistenceError(err, nil, nil)
		}

		// 重查：事务状态正常（ON CONFLICT 未 abort），返回新建行或并发已建行。
		m2, err := txClient.BundleSubscriptionUsage.Query().
			Where(
				bundlesubscriptionusage.BundleSubscriptionIDEQ(bundleSubscriptionID),
				bundlesubscriptionusage.GroupIDEQ(groupID),
				bundlesubscriptionusage.ModelPatternEQ(modelPattern),
			).
			Only(txCtx)
		if err != nil {
			return translatePersistenceError(err, nil, nil)
		}
		usage := bundleSubscriptionUsageToService(m2)
		result = &usage
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

// Create 创建用量记录，初始化各时间窗口
func (r *bundleUsageRepository) Create(ctx context.Context, usage *service.BundleSubscriptionUsage) error {
	if usage == nil {
		return nil
	}

	client := clientFromContext(ctx, r.client)

	created, err := client.BundleSubscriptionUsage.Create().
		SetBundleSubscriptionID(usage.BundleSubscriptionID).
		SetGroupID(usage.GroupID).
		SetModelPattern(usage.ModelPattern).
		SetDailyUsageUsd(usage.DailyUsageUSD).
		SetDailyWindowStart(usage.DailyWindowStart).
		SetWeeklyUsageUsd(usage.WeeklyUsageUSD).
		SetWeeklyWindowStart(usage.WeeklyWindowStart).
		SetMonthlyUsageUsd(usage.MonthlyUsageUSD).
		SetMonthlyWindowStart(usage.MonthlyWindowStart).
		SetDailyImageUsageCount(usage.DailyImageUsageCount).
		SetDailyVideoUsageCount(usage.DailyVideoUsageCount).
		SetWeeklyImageUsageCount(usage.WeeklyImageUsageCount).
		SetWeeklyVideoUsageCount(usage.WeeklyVideoUsageCount).
		SetMonthlyImageUsageCount(usage.MonthlyImageUsageCount).
		SetMonthlyVideoUsageCount(usage.MonthlyVideoUsageCount).
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, nil)
	}

	usage.ID = created.ID
	return nil
}

// IncrementUsage 原子累加日/周/月用量（USD + 图片/视频次数），并在过期窗口上滚动重置。
//
// 并发安全设计（修复历史 bug H1）：判断窗口是否过期与清零/累加写入必须在同一原子操作内。
// 旧实现由 service 层先读到 window_start 快照、算出 roll 标志、再无条件 UPDATE（Set/Add），
// 窗口边界瞬间的并发请求会各自基于过期快照算出 roll=true、各自 Set，互相覆盖、丢失计费。
// 现改为事务内 SELECT ... FOR UPDATE 锁行，基于 DB 真实 window_start 判断过期：过期则
// 置为本次值并推进 window_start，否则累加。与 user_platform_quota_repo.IncrementUsageWithReset
// 同范式。三条窗口独立判断，单次事务往返。
func (r *bundleUsageRepository) IncrementUsage(ctx context.Context, id int64, costUSD float64, imageCount, videoCount int, now time.Time) error {
	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		existing, err := txClient.BundleSubscriptionUsage.Query().
			Where(bundlesubscriptionusage.IDEQ(id)).
			ForUpdate().
			Only(txCtx)
		if err != nil {
			return translatePersistenceError(err, service.ErrBundleNotFound, nil)
		}

		dUSD, dImg, dVid, dStart := rollBundleWindow(existing.DailyUsageUsd, existing.DailyImageUsageCount, existing.DailyVideoUsageCount, existing.DailyWindowStart, now, service.BundleDailyWindowDays, costUSD, imageCount, videoCount)
		wUSD, wImg, wVid, wStart := rollBundleWindow(existing.WeeklyUsageUsd, existing.WeeklyImageUsageCount, existing.WeeklyVideoUsageCount, existing.WeeklyWindowStart, now, service.BundleWeeklyWindowDays, costUSD, imageCount, videoCount)
		mUSD, mImg, mVid, mStart := rollBundleWindow(existing.MonthlyUsageUsd, existing.MonthlyImageUsageCount, existing.MonthlyVideoUsageCount, existing.MonthlyWindowStart, now, service.BundleMonthlyWindowDays, costUSD, imageCount, videoCount)

		_, err = existing.Update().
			SetDailyUsageUsd(dUSD).SetDailyImageUsageCount(dImg).SetDailyVideoUsageCount(dVid).SetDailyWindowStart(dStart).
			SetWeeklyUsageUsd(wUSD).SetWeeklyImageUsageCount(wImg).SetWeeklyVideoUsageCount(wVid).SetWeeklyWindowStart(wStart).
			SetMonthlyUsageUsd(mUSD).SetMonthlyImageUsageCount(mImg).SetMonthlyVideoUsageCount(mVid).SetMonthlyWindowStart(mStart).
			Save(txCtx)
		return translatePersistenceError(err, nil, nil)
	})
}

// withTx 在数据库事务中执行 fn；若 ctx 已携带外层事务（集成测试隔离事务 / 上层事务）
// 则直接复用，不再嵌套开事务。与 BundleSubscriptionService.withTx 同范式：
// r.client 已处于事务中时 Tx() 返回 ErrTxStarted，此时复用既有 client，fn 内的
// FOR UPDATE/Update 仍在该事务内执行。
func (r *bundleUsageRepository) withTx(ctx context.Context, fn func(txCtx context.Context, txClient *dbent.Client) error) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return fn(ctx, tx.Client())
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		if errors.Is(err, dbent.ErrTxStarted) {
			return fn(ctx, r.client)
		}
		return fmt.Errorf("begin bundle usage transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := fn(txCtx, tx.Client()); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit bundle usage transaction: %w", err)
	}
	return nil
}

// rollBundleWindow 计算单个滚动窗口累加后的 (USD, imageCount, videoCount, windowStart)。
// 自然日 0 点对齐语义：过期判定委托 service.BundleWindowExpired（与读路径 rolledBundleUsage 逐字一致），
// 过期 → 重置为本次值并以 now 所在自然日 0 点为新窗口起点；未过期 → 在原值上累加并保留原起点。
// 纯函数，供 IncrementUsage 在事务锁内调用，保证「判断 + 写入」原子（修复历史并发丢累加 bug H1）。
func rollBundleWindow(prevUSD float64, prevImg, prevVid int, prevStart, now time.Time, windowDays int, costUSD float64, img, vid int) (float64, int, int, time.Time) {
	if service.BundleWindowExpired(prevStart, now, windowDays) {
		return costUSD, img, vid, service.BundleWindowStart(now)
	}
	return prevUSD + costUSD, prevImg + img, prevVid + vid, prevStart
}

// ResetDailyWindow 重置日窗口：清零日用量并更新窗口起点
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

// ResetWeeklyWindow 重置周窗口：清零周用量并更新窗口起点
func (r *bundleUsageRepository) ResetWeeklyWindow(ctx context.Context, id int64, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)

	_, err := client.BundleSubscriptionUsage.UpdateOneID(id).
		SetWeeklyUsageUsd(0).
		SetWeeklyImageUsageCount(0).
		SetWeeklyVideoUsageCount(0).
		SetWeeklyWindowStart(newWindowStart).
		Save(ctx)
	return translatePersistenceError(err, nil, nil)
}

// ResetMonthlyWindow 重置月窗口：清零月用量并更新窗口起点
func (r *bundleUsageRepository) ResetMonthlyWindow(ctx context.Context, id int64, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)

	_, err := client.BundleSubscriptionUsage.UpdateOneID(id).
		SetMonthlyUsageUsd(0).
		SetMonthlyImageUsageCount(0).
		SetMonthlyVideoUsageCount(0).
		SetMonthlyWindowStart(newWindowStart).
		Save(ctx)
	return translatePersistenceError(err, nil, nil)
}

// ListBySubscription 查询订阅下的所有用量记录
func (r *bundleUsageRepository) ListBySubscription(ctx context.Context, subscriptionID int64) ([]service.BundleSubscriptionUsage, error) {
	client := clientFromContext(ctx, r.client)

	usages, err := client.BundleSubscriptionUsage.Query().
		Where(bundlesubscriptionusage.BundleSubscriptionIDEQ(subscriptionID)).
		All(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, nil, nil)
	}

	results := make([]service.BundleSubscriptionUsage, len(usages))
	for i, u := range usages {
		results[i] = bundleSubscriptionUsageToService(u)
	}
	return results, nil
}

// BatchUpdateExpiredStatus 批量将已过期但仍为 active 的订阅标记为 expired
func (r *bundleUsageRepository) BatchUpdateExpiredStatus(ctx context.Context) (int64, error) {
	client := clientFromContext(ctx, r.client)

	// Find all bundle subscriptions that are active but expired.
	expiredIDs, err := client.BundleSubscription.Query().
		Where(
			bundlesubscription.StatusEQ("active"),
			bundlesubscription.ExpiresAtLTE(time.Now()),
		).
		IDs(ctx)
	if err != nil {
		return 0, err
	}

	if len(expiredIDs) == 0 {
		return 0, nil
	}

	n, err := client.BundleSubscription.Update().
		Where(bundlesubscription.IDIn(expiredIDs...)).
		SetStatus("expired").
		Save(ctx)
	if err != nil {
		return 0, err
	}

	return int64(n), nil
}
