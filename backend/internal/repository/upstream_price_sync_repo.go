package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// upstreamPriceSyncRepo 实现 service.UpstreamPriceSyncRepository。
//
// 选型说明：
//   - 三张表(upstream_source_configs / upstream_price_change_requests /
//     upstream_price_change_items)均无 ent schema,直接走原生 SQL;
//   - api_key_encrypted / dashboard_token_encrypted 用 payment.Encrypt/Decrypt
//     (AES-256-GCM)加解密,key 由 Wire 通过 payment.EncryptionKey 注入;
//   - upstream_source_configs 没有 updated_at 触发器,所有 UPDATE 显式
//     SET updated_at=now()。
type upstreamPriceSyncRepo struct {
	db     *sql.DB
	encKey payment.EncryptionKey
}

// NewUpstreamPriceSyncRepository 创建仓储实例。key 由 Wire 通过
// payment.ProvideEncryptionKey 注入(命名类型 payment.EncryptionKey,
// 避免与其它 []byte 参数歧义)。
func NewUpstreamPriceSyncRepository(db *sql.DB, key payment.EncryptionKey) service.UpstreamPriceSyncRepository {
	return &upstreamPriceSyncRepo{db: db, encKey: key}
}

func (r *upstreamPriceSyncRepo) encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	return payment.Encrypt(plain, []byte(r.encKey))
}

func (r *upstreamPriceSyncRepo) decrypt(cipher string) (string, error) {
	if cipher == "" {
		return "", nil
	}
	return payment.Decrypt(cipher, []byte(r.encKey))
}

// ---------- config CRUD ----------

// selectConfigSQL 读取 upstream_source_configs 的全部列。api_key_encrypted /
// dashboard_token_encrypted 以密文读出,由 scanConfig 解密。
const selectConfigSQL = `SELECT c.id, c.name, c.base_url, c.api_key_encrypted, c.dashboard_token_encrypted,
 c.dashboard_auth_mode, c.dashboard_user_id, c.proxy_id,
 c.target_channel_id, c.enabled, c.base_price_per_1k, c.pricing_source,
 c.sync_model_price, c.sync_group_ratio, c.group_mapping, c.target_upstream_group,
 c.group_ratio_baseline_key, c.group_ratio_baseline_value, c.group_ratio_baseline_observed_at,
 c.balance_threshold_usd,
 c.last_balance_quota, c.last_used_quota, c.last_balance_usd, c.last_balance_at,
 c.last_balance_checked_at, c.last_balance_error,
 c.last_sync_at, c.last_pricing_version, c.last_error, c.created_at, c.updated_at,
 COALESCE((SELECT array_agg(e.group_id ORDER BY e.group_id)
           FROM upstream_source_excluded_groups e WHERE e.source_config_id=c.id), '{}'::bigint[])
FROM upstream_source_configs c`

func (r *upstreamPriceSyncRepo) CreateConfig(ctx context.Context, c *service.UpstreamSourceConfig) error {
	ak, err := r.encrypt(c.APIKey)
	if err != nil {
		return fmt.Errorf("encrypt api_key: %w", err)
	}
	dt, err := r.encrypt(c.DashboardToken)
	if err != nil {
		return fmt.Errorf("encrypt dashboard_token: %w", err)
	}
	gm, err := json.Marshal(c.GroupMapping)
	if err != nil {
		return fmt.Errorf("marshal group_mapping: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.QueryRowContext(ctx, `
INSERT INTO upstream_source_configs
(name, base_url, api_key_encrypted, dashboard_token_encrypted, dashboard_auth_mode, dashboard_user_id, proxy_id,
 target_channel_id, enabled, base_price_per_1k, pricing_source,
 sync_model_price, sync_group_ratio, group_mapping, target_upstream_group, balance_threshold_usd)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id, created_at, updated_at`,
		c.Name, c.BaseURL, ak, dt, service.NormalizeDashboardAuthMode(c.DashboardAuthMode), c.DashboardUserID, c.ProxyID,
		c.TargetChannelID, c.Enabled,
		c.BasePricePer1k, string(c.PricingSource), c.SyncModelPrice, c.SyncGroupRatio, gm, c.TargetUpstreamGroup, c.BalanceThresholdUSD,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return err
	}
	if err := replaceExcludedGroups(ctx, tx, c.ID, c.TargetChannelID, c.ExcludedGroupIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *upstreamPriceSyncRepo) GetConfig(ctx context.Context, id int64) (*service.UpstreamSourceConfig, error) {
	row := r.db.QueryRowContext(ctx, selectConfigSQL+` WHERE id=$1`, id)
	return scanConfig(row, r)
}

func (r *upstreamPriceSyncRepo) GetConfigByName(ctx context.Context, name string) (*service.UpstreamSourceConfig, error) {
	row := r.db.QueryRowContext(ctx, selectConfigSQL+` WHERE name=$1`, name)
	return scanConfig(row, r)
}

func (r *upstreamPriceSyncRepo) ListConfigs(ctx context.Context) ([]service.UpstreamSourceConfig, error) {
	rows, err := r.db.QueryContext(ctx, selectConfigSQL+` ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list upstream source configs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.UpstreamSourceConfig, 0)
	for rows.Next() {
		c, err := scanConfig(rows, r)
		if err != nil {
			return nil, fmt.Errorf("scan upstream source config: %w", err)
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (r *upstreamPriceSyncRepo) UpdateConfig(ctx context.Context, c *service.UpstreamSourceConfig) error {
	ak, err := r.encrypt(c.APIKey)
	if err != nil {
		return fmt.Errorf("encrypt api_key: %w", err)
	}
	dt, err := r.encrypt(c.DashboardToken)
	if err != nil {
		return fmt.Errorf("encrypt dashboard_token: %w", err)
	}
	gm, err := json.Marshal(c.GroupMapping)
	if err != nil {
		return fmt.Errorf("marshal group_mapping: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.QueryRowContext(ctx, `
UPDATE upstream_source_configs
SET name=$2, base_url=$3, api_key_encrypted=$4, dashboard_token_encrypted=$5,
    dashboard_auth_mode=$6, dashboard_user_id=$7, proxy_id=$8,
    target_channel_id=$9, enabled=$10, base_price_per_1k=$11, pricing_source=$12,
    sync_model_price=$13, sync_group_ratio=$14, group_mapping=$15, target_upstream_group=$16, balance_threshold_usd=$17,
    last_balance_quota=CASE WHEN $18 THEN NULL ELSE last_balance_quota END,
    last_used_quota=CASE WHEN $18 THEN NULL ELSE last_used_quota END,
    last_balance_usd=CASE WHEN $18 THEN NULL ELSE last_balance_usd END,
    last_balance_at=CASE WHEN $18 THEN NULL ELSE last_balance_at END,
    last_balance_checked_at=CASE WHEN $18 THEN NULL ELSE last_balance_checked_at END,
    last_balance_error=CASE WHEN $18 THEN NULL ELSE last_balance_error END,
    group_ratio_baseline_key=CASE WHEN $19 THEN NULL ELSE group_ratio_baseline_key END,
    group_ratio_baseline_value=CASE WHEN $19 THEN NULL ELSE group_ratio_baseline_value END,
    group_ratio_baseline_observed_at=CASE WHEN $19 THEN NULL ELSE group_ratio_baseline_observed_at END,
    updated_at=now()
WHERE id=$1
RETURNING updated_at`,
		c.ID, c.Name, c.BaseURL, ak, dt,
		service.NormalizeDashboardAuthMode(c.DashboardAuthMode), c.DashboardUserID, c.ProxyID,
		c.TargetChannelID, c.Enabled,
		c.BasePricePer1k, string(c.PricingSource), c.SyncModelPrice, c.SyncGroupRatio,
		gm, c.TargetUpstreamGroup, c.BalanceThresholdUSD, c.ResetBalanceSnapshot, c.ResetGroupRatioBaseline,
	).Scan(&c.UpdatedAt); err != nil {
		return err
	}
	if err := replaceExcludedGroups(ctx, tx, c.ID, c.TargetChannelID, c.ExcludedGroupIDs); err != nil {
		return err
	}
	if c.ExpirePendingApprovals {
		if err := expirePendingItemsTx(ctx, tx, c.ID); err != nil {
			return err
		}
	} else if c.ExpirePendingGroupApprovals {
		if err := expirePendingGroupItemsTx(ctx, tx, c.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func replaceExcludedGroups(ctx context.Context, tx *sql.Tx, configID, channelID int64, groupIDs []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM upstream_source_excluded_groups WHERE source_config_id=$1`, configID); err != nil {
		return fmt.Errorf("clear excluded groups: %w", err)
	}
	for _, groupID := range groupIDs {
		res, err := tx.ExecContext(ctx, `
INSERT INTO upstream_source_excluded_groups (source_config_id, group_id)
SELECT $1, $2
WHERE EXISTS (SELECT 1 FROM channel_groups WHERE channel_id=$3 AND group_id=$2)
ON CONFLICT DO NOTHING`, configID, groupID, channelID)
		if err != nil {
			return fmt.Errorf("insert excluded group %d: %w", groupID, err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return fmt.Errorf("excluded group %d does not belong to channel %d", groupID, channelID)
		}
	}
	return nil
}

func expirePendingItemsTx(ctx context.Context, tx *sql.Tx, configID int64) error {
	if _, err := tx.ExecContext(ctx, `
UPDATE upstream_price_change_items i
SET status='ignored', review_note='source configuration changed', reviewed_at=now()
FROM upstream_price_change_requests r
WHERE i.request_id=r.id AND r.source_config_id=$1 AND i.status='pending'`, configID); err != nil {
		return fmt.Errorf("expire pending change items: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
SELECT id FROM upstream_price_change_requests
WHERE source_config_id=$1 AND status IN ('open','partially_applied')
FOR UPDATE`, configID)
	if err != nil {
		return err
	}
	requestIDs := make([]int64, 0)
	for rows.Next() {
		var requestID int64
		if err := rows.Scan(&requestID); err != nil {
			_ = rows.Close()
			return err
		}
		requestIDs = append(requestIDs, requestID)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, requestID := range requestIDs {
		if err := recomputeRequestTx(ctx, tx, requestID); err != nil {
			return err
		}
	}
	return nil
}

func expirePendingGroupItemsTx(ctx context.Context, tx *sql.Tx, configID int64) error {
	if _, err := tx.ExecContext(ctx, `
UPDATE upstream_price_change_items i
SET status='ignored', review_note='group ratio exclusions changed', reviewed_at=now()
FROM upstream_price_change_requests r
WHERE i.request_id=r.id AND r.source_config_id=$1
  AND i.kind='group_ratio' AND i.status='pending'`, configID); err != nil {
		return fmt.Errorf("expire pending group ratio items: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
SELECT id FROM upstream_price_change_requests
WHERE source_config_id=$1 AND status IN ('open','partially_applied')
FOR UPDATE`, configID)
	if err != nil {
		return err
	}
	requestIDs := make([]int64, 0)
	for rows.Next() {
		var requestID int64
		if err := rows.Scan(&requestID); err != nil {
			_ = rows.Close()
			return err
		}
		requestIDs = append(requestIDs, requestID)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, requestID := range requestIDs {
		if err := recomputeRequestTx(ctx, tx, requestID); err != nil {
			return err
		}
	}
	return nil
}

func expireSupersededModelRequestsTx(ctx context.Context, tx *sql.Tx, configID int64) error {
	if _, err := tx.ExecContext(ctx, `
UPDATE upstream_price_change_items i
SET status='ignored', review_note='superseded by newer upstream sync', reviewed_at=now()
FROM upstream_price_change_requests r
WHERE i.request_id=r.id AND r.source_config_id=$1 AND i.status='pending'
  AND i.kind <> 'group_ratio'`, configID); err != nil {
		return fmt.Errorf("expire superseded model items: %w", err)
	}
	_, err := tx.ExecContext(ctx, `
UPDATE upstream_price_change_requests r
SET status='expired', closed_at=now()
WHERE r.source_config_id=$1 AND r.status IN ('open','partially_applied')
  AND NOT EXISTS (
    SELECT 1 FROM upstream_price_change_items i
    WHERE i.request_id=r.id AND i.kind='group_ratio' AND i.status='pending'
  )`, configID)
	return err
}

func (r *upstreamPriceSyncRepo) DeleteConfig(ctx context.Context, id int64) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM upstream_source_configs WHERE id=$1`, id); err != nil {
		return fmt.Errorf("delete upstream source config: %w", err)
	}
	return nil
}

// UpdateConfigSyncState 更新最近一次同步时间/版本/错误;显式 updated_at=now()
// 因为该表没有 updated_at 触发器。
func (r *upstreamPriceSyncRepo) UpdateConfigSyncState(ctx context.Context, id int64, lastSyncAt time.Time, version, lastErr string) error {
	if _, err := r.db.ExecContext(ctx, `
UPDATE upstream_source_configs
SET last_sync_at=$2, last_pricing_version=$3, last_error=$4, updated_at=now()
WHERE id=$1`, id, lastSyncAt, version, lastErr); err != nil {
		return fmt.Errorf("update upstream source sync state: %w", err)
	}
	return nil
}

func (r *upstreamPriceSyncRepo) UpdateConfigBalanceSuccess(ctx context.Context, id int64, snapshot service.BalanceSnapshot) error {
	if _, err := r.db.ExecContext(ctx, `
UPDATE upstream_source_configs
SET last_balance_quota=$2, last_used_quota=$3, last_balance_usd=$4,
    last_balance_at=$5, last_balance_checked_at=$5, last_balance_error=NULL, updated_at=now()
WHERE id=$1`, id, snapshot.Quota, snapshot.UsedQuota, snapshot.BalanceUSD, snapshot.FetchedAt); err != nil {
		return fmt.Errorf("update upstream source balance: %w", err)
	}
	return nil
}

func (r *upstreamPriceSyncRepo) UpdateConfigBalanceError(ctx context.Context, id int64, checkedAt time.Time, lastErr string) error {
	if _, err := r.db.ExecContext(ctx, `
UPDATE upstream_source_configs
SET last_balance_checked_at=$2, last_balance_error=$3, updated_at=now()
WHERE id=$1`, id, checkedAt, lastErr); err != nil {
		return fmt.Errorf("update upstream source balance error: %w", err)
	}
	return nil
}

// scanConfig 把单行 upstream_source_configs 扫入 UpstreamSourceConfig:
//   - 解密 api_key_encrypted / dashboard_token_encrypted;
//   - 解析 group_mapping JSONB → map[string]int64;
//   - pricing_source 转为 UpstreamPricingSource。
type int64Array []int64

func (a *int64Array) Scan(src any) error {
	return pq.Array((*[]int64)(a)).Scan(src)
}

func scanConfig(row rowScanner, r *upstreamPriceSyncRepo) (*service.UpstreamSourceConfig, error) {
	var (
		c                  service.UpstreamSourceConfig
		apiKeyEnc          string
		dashTokenEnc       string
		dashboardAuthMode  sql.NullString
		dashboardUserID    sql.NullInt64
		pricingSource      string
		groupMapping       []byte
		baselineKey        sql.NullString
		baselineValue      sql.NullFloat64
		baselineObservedAt sql.NullTime
		balanceThresh      sql.NullFloat64
		lastBalanceQuota   sql.NullInt64
		lastUsedQuota      sql.NullInt64
		lastBalanceUSD     sql.NullFloat64
		lastBalanceAt      sql.NullTime
		lastBalanceCheck   sql.NullTime
		lastBalanceError   sql.NullString
		lastSyncAt         sql.NullTime
		lastPricingV       sql.NullString
		lastErr            sql.NullString
		excludedGroupIDs   int64Array
	)
	if err := row.Scan(
		&c.ID, &c.Name, &c.BaseURL, &apiKeyEnc, &dashTokenEnc,
		&dashboardAuthMode, &dashboardUserID, &c.ProxyID,
		&c.TargetChannelID, &c.Enabled, &c.BasePricePer1k, &pricingSource,
		&c.SyncModelPrice, &c.SyncGroupRatio, &groupMapping, &c.TargetUpstreamGroup,
		&baselineKey, &baselineValue, &baselineObservedAt, &balanceThresh,
		&lastBalanceQuota, &lastUsedQuota, &lastBalanceUSD, &lastBalanceAt,
		&lastBalanceCheck, &lastBalanceError,
		&lastSyncAt, &lastPricingV, &lastErr, &c.CreatedAt, &c.UpdatedAt, &excludedGroupIDs,
	); err != nil {
		return nil, err
	}
	apiKey, err := r.decrypt(apiKeyEnc)
	if err != nil {
		return nil, fmt.Errorf("decrypt api_key: %w", err)
	}
	dashToken, err := r.decrypt(dashTokenEnc)
	if err != nil {
		return nil, fmt.Errorf("decrypt dashboard_token: %w", err)
	}
	c.APIKey = apiKey
	c.DashboardToken = dashToken
	c.DashboardAuthMode = service.NormalizeDashboardAuthMode(dashboardAuthMode.String)
	if dashboardUserID.Valid {
		v := dashboardUserID.Int64
		c.DashboardUserID = &v
	}
	c.PricingSource = service.UpstreamPricingSource(pricingSource)
	if len(groupMapping) > 0 {
		if err := json.Unmarshal(groupMapping, &c.GroupMapping); err != nil {
			return nil, fmt.Errorf("unmarshal group_mapping: %w", err)
		}
	}
	c.ExcludedGroupIDs = []int64(excludedGroupIDs)
	c.GroupRatioBaselineKey = baselineKey.String
	if baselineValue.Valid {
		v := baselineValue.Float64
		c.GroupRatioBaselineValue = &v
	}
	if baselineObservedAt.Valid {
		t := baselineObservedAt.Time
		c.GroupRatioBaselineObserved = &t
	}
	if balanceThresh.Valid {
		v := balanceThresh.Float64
		c.BalanceThresholdUSD = &v
	}
	if lastBalanceQuota.Valid {
		v := lastBalanceQuota.Int64
		c.LastBalanceQuota = &v
	}
	if lastUsedQuota.Valid {
		v := lastUsedQuota.Int64
		c.LastUsedQuota = &v
	}
	if lastBalanceUSD.Valid {
		v := lastBalanceUSD.Float64
		c.LastBalanceUSD = &v
	}
	if lastBalanceAt.Valid {
		t := lastBalanceAt.Time
		c.LastBalanceAt = &t
	}
	if lastBalanceCheck.Valid {
		t := lastBalanceCheck.Time
		c.LastBalanceCheckedAt = &t
	}
	c.LastBalanceError = lastBalanceError.String
	if lastSyncAt.Valid {
		t := lastSyncAt.Time
		c.LastSyncAt = &t
	}
	c.LastPricingVersion = lastPricingV.String
	c.LastError = lastErr.String
	return &c, nil
}

// ---------- request / items CRUD ----------

// selectRequestSQL 读取 upstream_price_change_requests 全部列。
const selectRequestSQL = `SELECT id, source_config_id, trigger_type, status, upstream_pricing_version,
 summary, created_by, created_at, closed_at
FROM upstream_price_change_requests`

// selectItemSQL 读取 upstream_price_change_items 全部列。
const selectItemSQL = `SELECT id, request_id, kind, platform, model_name, target_channel_id,
 target_group_id, upstream_raw, upstream_converted, local_current, apply_value,
 group_rate_change, apply_rate,
 status, reviewer_id, review_note, reviewed_at, applied_at, created_at
FROM upstream_price_change_items`

// CreateRequest 在单事务内插入审批批次及其全部条目。任一条目插入失败回滚整批。
func (r *upstreamPriceSyncRepo) WithSourceSyncLock(ctx context.Context, configID int64, fn func(context.Context) error) error {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	const lockNamespace int64 = 0x55505352 // "UPSR"
	var acquired bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1,$2)`, lockNamespace, configID).Scan(&acquired); err != nil {
		return err
	}
	if !acquired {
		return service.ErrSourceSyncBusy
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1,$2)`, lockNamespace, configID)
	}()
	return fn(ctx)
}

func (r *upstreamPriceSyncRepo) ListGroupRateTargets(ctx context.Context, configID, channelID int64) ([]service.GroupRateTarget, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT g.id, g.name, g.sort_order, g.rate_multiplier
FROM channel_groups cg
JOIN groups g ON g.id=cg.group_id AND g.deleted_at IS NULL
LEFT JOIN upstream_source_excluded_groups e ON e.source_config_id=$1 AND e.group_id=g.id
WHERE cg.channel_id=$2 AND e.group_id IS NULL
ORDER BY g.sort_order ASC, g.id ASC`, configID, channelID)
	if err != nil {
		return nil, fmt.Errorf("list group rate targets: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.GroupRateTarget, 0)
	for rows.Next() {
		var target service.GroupRateTarget
		if err := rows.Scan(&target.ID, &target.Name, &target.SortOrder, &target.RateMultiplier); err != nil {
			return nil, err
		}
		out = append(out, target)
	}
	return out, rows.Err()
}

func (r *upstreamPriceSyncRepo) HasPendingGroupRateItems(ctx context.Context, configID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
SELECT EXISTS(
 SELECT 1 FROM upstream_price_change_items i
 JOIN upstream_price_change_requests r ON r.id=i.request_id
 WHERE r.source_config_id=$1 AND i.kind='group_ratio' AND i.status='pending'
)`, configID).Scan(&exists)
	return exists, err
}

func (r *upstreamPriceSyncRepo) PersistSyncResult(ctx context.Context, input service.SyncPersistInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if input.Request != nil {
		hasGroupItems := false
		for i := range input.Items {
			if input.Items[i].Kind == service.ItemKindGroupRatio {
				hasGroupItems = true
				break
			}
		}
		if hasGroupItems {
			if _, err := tx.ExecContext(ctx, `
UPDATE upstream_price_change_requests r
SET status='expired', closed_at=now()
WHERE r.source_config_id=$1 AND r.status IN ('open','partially_applied')
  AND NOT EXISTS (
    SELECT 1 FROM upstream_price_change_items i
    WHERE i.request_id=r.id AND i.kind='group_ratio' AND i.status='pending'
  )`, input.ConfigID); err != nil {
				return fmt.Errorf("expire superseded requests: %w", err)
			}
		} else if err := expireSupersededModelRequestsTx(ctx, tx, input.ConfigID); err != nil {
			return err
		}
		if err := createRequestTx(ctx, tx, input.Request, input.Items); err != nil {
			return err
		}
	}
	if input.AdvanceBaseline {
		if _, err := tx.ExecContext(ctx, `
UPDATE upstream_source_configs
SET group_ratio_baseline_key=$2, group_ratio_baseline_value=$3,
    group_ratio_baseline_observed_at=$4, last_sync_at=$4,
    last_pricing_version=$5, last_error='', updated_at=now()
WHERE id=$1`, input.ConfigID, input.BaselineKey, input.BaselineValue, input.ObservedAt, input.PricingVersion); err != nil {
			return fmt.Errorf("advance group ratio baseline: %w", err)
		}
	} else if _, err := tx.ExecContext(ctx, `
UPDATE upstream_source_configs
SET last_sync_at=$2, last_pricing_version=$3, last_error='', updated_at=now()
WHERE id=$1`, input.ConfigID, input.ObservedAt, input.PricingVersion); err != nil {
		return fmt.Errorf("update sync state: %w", err)
	}
	return tx.Commit()
}

func (r *upstreamPriceSyncRepo) CreateRequest(ctx context.Context, req *service.PriceChangeRequest, items []service.PriceChangeItem) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := createRequestTx(ctx, tx, req, items); err != nil {
		return err
	}
	return tx.Commit()
}

func createRequestTx(ctx context.Context, tx *sql.Tx, req *service.PriceChangeRequest, items []service.PriceChangeItem) error {
	if req.Summary == nil {
		req.Summary = make(map[string]int)
		for i := range items {
			status := items[i].Status
			if status == "" {
				status = "pending"
			}
			req.Summary[status]++
		}
	}
	summary, err := json.Marshal(req.Summary)
	if err != nil {
		return fmt.Errorf("marshal request summary: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `
INSERT INTO upstream_price_change_requests (source_config_id, trigger_type, status, upstream_pricing_version, summary, created_by)
VALUES ($1,$2,$3,$4,$5,$6) RETURNING id, created_at`,
		req.SourceConfigID, req.TriggerType, "open", req.UpstreamPricingVersion, summary, req.CreatedBy,
	).Scan(&req.ID, &req.CreatedAt); err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	for i := range items {
		it := &items[i]
		it.RequestID = req.ID
		raw, err := json.Marshal(it.UpstreamRaw)
		if err != nil {
			return fmt.Errorf("marshal upstream_raw: %w", err)
		}
		up, err := service.MarshalConverted(it.UpstreamConverted)
		if err != nil {
			return fmt.Errorf("marshal upstream_converted: %w", err)
		}
		loc, err := service.MarshalConverted(it.LocalCurrent)
		if err != nil {
			return fmt.Errorf("marshal local_current: %w", err)
		}
		apv, err := service.MarshalConverted(it.ApplyValue)
		if err != nil {
			return fmt.Errorf("marshal apply_value: %w", err)
		}
		groupRate, err := json.Marshal(it.GroupRateChange)
		if err != nil {
			return fmt.Errorf("marshal group_rate_change: %w", err)
		}
		status := it.Status
		if status == "" {
			status = "pending"
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO upstream_price_change_items
(request_id, kind, platform, model_name, target_channel_id, target_group_id,
 upstream_raw, upstream_converted, local_current, apply_value, group_rate_change, apply_rate, status)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
			it.RequestID, string(it.Kind), it.Platform, it.ModelName, it.TargetChannelID, it.TargetGroupID,
			raw, up, loc, apv, groupRate, it.ApplyRate, status); err != nil {
			return fmt.Errorf("create item: %w", err)
		}
	}
	return nil
}

func (r *upstreamPriceSyncRepo) GetRequest(ctx context.Context, id int64) (*service.PriceChangeRequest, error) {
	row := r.db.QueryRowContext(ctx, selectRequestSQL+` WHERE id=$1`, id)
	return scanRequest(row)
}

// ListRequests 按 RequestFilter 过滤 + 分页,返回 (列表, 总数, 错误)。
// SourceConfigID/Status 为零值时不过滤;Page/PageSize <= 0 时取默认值。
func (r *upstreamPriceSyncRepo) ListRequests(ctx context.Context, f service.RequestFilter) ([]service.PriceChangeRequest, int64, error) {
	where := "WHERE 1=1"
	var args []any
	idx := 1
	if f.SourceConfigID != nil {
		where += fmt.Sprintf(" AND source_config_id=$%d", idx)
		args = append(args, *f.SourceConfigID)
		idx++
	}
	if f.Status != "" {
		where += fmt.Sprintf(" AND status=$%d", idx)
		args = append(args, f.Status)
		idx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM upstream_price_change_requests `+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count upstream price change requests: %w", err)
	}

	pageSize := f.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	page := f.Page
	if page <= 0 {
		page = 1
	}
	listArgs := append(args, pageSize, (page-1)*pageSize)
	listQuery := selectRequestSQL + " " + where +
		fmt.Sprintf(" ORDER BY id DESC LIMIT $%d OFFSET $%d", idx, idx+1)

	rows, err := r.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list upstream price change requests: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.PriceChangeRequest, 0)
	for rows.Next() {
		req, err := scanRequest(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan upstream price change request: %w", err)
		}
		out = append(out, *req)
	}
	return out, total, rows.Err()
}

func (r *upstreamPriceSyncRepo) ListItems(ctx context.Context, requestID int64) ([]service.PriceChangeItem, error) {
	rows, err := r.db.QueryContext(ctx, selectItemSQL+` WHERE request_id=$1 ORDER BY id ASC`, requestID)
	if err != nil {
		return nil, fmt.Errorf("list upstream price change items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.PriceChangeItem, 0)
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan upstream price change item: %w", err)
		}
		out = append(out, *it)
	}
	return out, rows.Err()
}

func (r *upstreamPriceSyncRepo) GetItem(ctx context.Context, id int64) (*service.PriceChangeItem, error) {
	row := r.db.QueryRowContext(ctx, selectItemSQL+` WHERE id=$1`, id)
	return scanItem(row)
}

// UpdateItemStatus 更新条目审批状态。reviewed_at 始终刷为 now();
// appliedAt 为 *time.Time,nil 落 NULL(由 database/sql 自动处理)。
func recomputeRequestTx(ctx context.Context, tx *sql.Tx, requestID int64) error {
	rows, err := tx.QueryContext(ctx, `SELECT status, COUNT(*) FROM upstream_price_change_items WHERE request_id=$1 GROUP BY status`, requestID)
	if err != nil {
		return err
	}
	summary := make(map[string]int)
	total, pending := 0, 0
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			_ = rows.Close()
			return err
		}
		summary[status] = count
		total += count
		if status == "pending" {
			pending = count
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	status := "open"
	switch {
	case pending == 0:
		status = "closed"
	case pending < total:
		status = "partially_applied"
	}
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
UPDATE upstream_price_change_requests
SET status=$2, summary=$3, closed_at=CASE WHEN $4 THEN now() ELSE NULL END
WHERE id=$1`, requestID, status, summaryJSON, status == "closed")
	return err
}

func (r *upstreamPriceSyncRepo) FinalizeItemCAS(ctx context.Context, requestID, itemID int64, status string, reviewerID int64, note string, applyValue *service.ConvertedPrice) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	applyJSON, err := service.MarshalConverted(applyValue)
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `
UPDATE upstream_price_change_items
SET status=$3, reviewer_id=$4, review_note=$5, reviewed_at=now(),
    applied_at=CASE WHEN $3='applied' THEN now() ELSE NULL END,
    apply_value=CASE WHEN $6::jsonb IS NULL THEN apply_value ELSE $6::jsonb END
WHERE id=$1 AND request_id=$2 AND status='pending'`, itemID, requestID, status, reviewerID, note, nullableJSON(applyJSON))
	if err != nil {
		return fmt.Errorf("finalize price change item: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		var actualRequestID int64
		if err := tx.QueryRowContext(ctx, `SELECT request_id FROM upstream_price_change_items WHERE id=$1`, itemID).Scan(&actualRequestID); err != nil {
			return err
		}
		if actualRequestID != requestID {
			return service.ErrItemRequestMismatch
		}
		return service.ErrItemNotPending
	}
	if err := recomputeRequestTx(ctx, tx, requestID); err != nil {
		return fmt.Errorf("recompute price change request: %w", err)
	}
	return tx.Commit()
}

func nullableJSON(value []byte) any {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	return string(value)
}

func (r *upstreamPriceSyncRepo) ApplyGroupRateItem(ctx context.Context, input service.GroupRateApplyInput) (int64, float64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var (
		actualRequestID int64
		requestStatus   string
		configID        int64
		channelID       int64
		targetGroupID   sql.NullInt64
		itemStatus      string
		kind            string
		changeJSON      []byte
	)
	if err := tx.QueryRowContext(ctx, `
SELECT i.request_id, r.status, r.source_config_id, i.target_channel_id,
       i.target_group_id, i.status, i.kind, i.group_rate_change
FROM upstream_price_change_items i
JOIN upstream_price_change_requests r ON r.id=i.request_id
WHERE i.id=$1
FOR UPDATE OF i, r`, input.ItemID).Scan(
		&actualRequestID, &requestStatus, &configID, &channelID,
		&targetGroupID, &itemStatus, &kind, &changeJSON,
	); err != nil {
		return 0, 0, err
	}
	if actualRequestID != input.RequestID {
		return 0, 0, service.ErrItemRequestMismatch
	}
	if kind != string(service.ItemKindGroupRatio) || itemStatus != "pending" || (requestStatus != "open" && requestStatus != "partially_applied") {
		return 0, 0, service.ErrItemNotPending
	}
	if !targetGroupID.Valid {
		return 0, 0, service.ErrGroupRateTargetUnavailable
	}
	var change service.GroupRateChange
	if err := json.Unmarshal(changeJSON, &change); err != nil {
		return 0, 0, fmt.Errorf("decode group rate change: %w", err)
	}
	var (
		currentRate float64
		belongs     bool
		excluded    bool
	)
	if err := tx.QueryRowContext(ctx, `
SELECT g.rate_multiplier,
       EXISTS(SELECT 1 FROM channel_groups cg WHERE cg.channel_id=$2 AND cg.group_id=g.id),
       EXISTS(SELECT 1 FROM upstream_source_excluded_groups e WHERE e.source_config_id=$3 AND e.group_id=g.id)
FROM groups g
WHERE g.id=$1 AND g.deleted_at IS NULL
FOR UPDATE OF g`, targetGroupID.Int64, channelID, configID).Scan(&currentRate, &belongs, &excluded); err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, service.ErrGroupRateTargetUnavailable
		}
		return 0, 0, err
	}
	if !belongs || excluded {
		return 0, 0, service.ErrGroupRateTargetUnavailable
	}
	if service.RoundRateMultiplier(currentRate) != service.RoundRateMultiplier(change.LocalCurrentRate) {
		return 0, 0, service.ErrGroupRateDrift
	}
	rounded := service.RoundRateMultiplier(input.ApplyRate)
	if rounded <= 0 || rounded >= 1_000_000 || math.IsNaN(rounded) || math.IsInf(rounded, 0) {
		return 0, 0, fmt.Errorf("apply_rate must be finite, > 0 and fit DECIMAL(10,4)")
	}
	res, err := tx.ExecContext(ctx, `UPDATE groups SET rate_multiplier=$2, updated_at=now() WHERE id=$1 AND deleted_at IS NULL`, targetGroupID.Int64, rounded)
	if err != nil {
		return 0, 0, fmt.Errorf("update group rate: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected != 1 {
		return 0, 0, service.ErrGroupRateTargetUnavailable
	}
	groupID := targetGroupID.Int64
	if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventGroupChanged, nil, &groupID, nil); err != nil {
		return 0, 0, fmt.Errorf("enqueue group change: %w", err)
	}
	res, err = tx.ExecContext(ctx, `
UPDATE upstream_price_change_items
SET status='applied', apply_rate=$3, reviewer_id=$4, review_note=$5,
    reviewed_at=now(), applied_at=now()
WHERE id=$1 AND request_id=$2 AND status='pending'`, input.ItemID, input.RequestID, rounded, input.ReviewerID, input.Note)
	if err != nil {
		return 0, 0, err
	}
	if affected, _ := res.RowsAffected(); affected != 1 {
		return 0, 0, service.ErrItemNotPending
	}
	if err := recomputeRequestTx(ctx, tx, input.RequestID); err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return groupID, rounded, nil
}

func (r *upstreamPriceSyncRepo) UpdateItemStatus(ctx context.Context, id int64, status string, reviewerID int64, note string, appliedAt *time.Time) error {
	if _, err := r.db.ExecContext(ctx, `
UPDATE upstream_price_change_items
SET status=$2, reviewer_id=$3, review_note=$4, applied_at=$5, reviewed_at=now()
WHERE id=$1`,
		id, status, reviewerID, note, appliedAt,
	); err != nil {
		return fmt.Errorf("update upstream price change item status: %w", err)
	}
	return nil
}

// ExpireOpenRequests 把某个 config 下所有 open 批次置为 expired,返回受影响行数。
func (r *upstreamPriceSyncRepo) ExpireOpenRequests(ctx context.Context, configID int64) (int, error) {
	res, err := r.db.ExecContext(ctx, `
UPDATE upstream_price_change_requests
SET status='expired', closed_at=now()
WHERE source_config_id=$1 AND status='open'`, configID)
	if err != nil {
		return 0, fmt.Errorf("expire open requests: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("expire open requests rows affected: %w", err)
	}
	return int(n), nil
}

// CloseRequest 把单个审批批次标记为 closed(仅 open / partially_applied 可关闭)。
// 由 service 层在确认无 pending 条目后调用。
func (r *upstreamPriceSyncRepo) CloseRequest(ctx context.Context, requestID int64) error {
	res, err := r.db.ExecContext(ctx, `
UPDATE upstream_price_change_requests
SET status='closed', closed_at=now()
WHERE id=$1 AND status IN ('open', 'partially_applied')`, requestID)
	if err != nil {
		return fmt.Errorf("close upstream price change request: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("close upstream price change request rows affected: %w", err)
	}
	if n == 0 {
		return service.ErrRequestNotCloseable
	}
	return nil
}

// UpdateRequestStatus 由 service 层在 review 条目后重算并推进审批单状态:
// 同时刷新 summary(各 item status 的计数);status=closed 时一并落 closed_at。
// 与 CloseRequest 不同,此处不做「无 pending 才允许」校验——校验已在 service 层
// recomputeRequestStatus 内完成,这里仅负责落库。
func (r *upstreamPriceSyncRepo) UpdateRequestStatus(ctx context.Context, requestID int64, status string, summary map[string]int) error {
	summaryJSON, _ := json.Marshal(summary) // nil/空 map → "null"/"{}",前端按空处理
	// 注意:$2 不能同时裸用于 SET status=$2(varchar) 与 CASE WHEN $2='closed'(text)——
	// lib/pq 会报 "inconsistent types deduced for parameter $2",整条 UPDATE 失败、
	// recompute 静默(best-effort),表现为 item 已落终态但审批单 status 不变。
	// 改用独立 bool 参数 $4 表达「是否闭环」,每个占位符仅单一类型上下文,规避推断冲突。
	isClosed := status == "closed"
	_, err := r.db.ExecContext(ctx, `
UPDATE upstream_price_change_requests
SET status=$2, summary=$3,
    closed_at=CASE WHEN $4 THEN now() ELSE closed_at END
WHERE id=$1`, requestID, status, summaryJSON, isClosed)
	if err != nil {
		return fmt.Errorf("update upstream price change request status: %w", err)
	}
	return nil
}

// ---------- scan helpers ----------

func scanRequest(row rowScanner) (*service.PriceChangeRequest, error) {
	var (
		req         service.PriceChangeRequest
		upstreamVer sql.NullString
		summary     []byte
		createdBy   sql.NullInt64
		closedAt    sql.NullTime
	)
	if err := row.Scan(
		&req.ID, &req.SourceConfigID, &req.TriggerType, &req.Status,
		&upstreamVer, &summary, &createdBy, &req.CreatedAt, &closedAt,
	); err != nil {
		return nil, err
	}
	req.UpstreamPricingVersion = upstreamVer.String
	if len(summary) > 0 {
		if err := json.Unmarshal(summary, &req.Summary); err != nil {
			return nil, fmt.Errorf("unmarshal request summary: %w", err)
		}
	}
	if createdBy.Valid {
		req.CreatedBy = createdBy.Int64
	}
	if closedAt.Valid {
		t := closedAt.Time
		req.ClosedAt = &t
	}
	return &req, nil
}

func scanItem(row rowScanner) (*service.PriceChangeItem, error) {
	var (
		it                service.PriceChangeItem
		kind              string
		platform          sql.NullString
		modelName         sql.NullString
		targetChannelID   sql.NullInt64
		targetGroupID     sql.NullInt64
		upstreamRaw       []byte
		upstreamConverted []byte
		localCurrent      []byte
		applyValue        []byte
		groupRateChange   []byte
		applyRate         sql.NullFloat64
		reviewerID        sql.NullInt64
		reviewNote        sql.NullString
		reviewedAt        sql.NullTime
		appliedAt         sql.NullTime
	)
	if err := row.Scan(
		&it.ID, &it.RequestID, &kind, &platform, &modelName, &targetChannelID,
		&targetGroupID, &upstreamRaw, &upstreamConverted, &localCurrent, &applyValue,
		&groupRateChange, &applyRate,
		&it.Status, &reviewerID, &reviewNote, &reviewedAt, &appliedAt, &it.CreatedAt,
	); err != nil {
		return nil, err
	}
	it.Kind = service.PriceChangeItemKind(kind)
	it.Platform = platform.String
	it.ModelName = modelName.String
	if targetChannelID.Valid {
		it.TargetChannelID = targetChannelID.Int64
	}
	if targetGroupID.Valid {
		v := targetGroupID.Int64
		it.TargetGroupID = &v
	}
	if len(upstreamRaw) > 0 {
		if err := json.Unmarshal(upstreamRaw, &it.UpstreamRaw); err != nil {
			return nil, fmt.Errorf("unmarshal upstream_raw: %w", err)
		}
	}
	conv, err := service.UnmarshalConverted(upstreamConverted)
	if err != nil {
		return nil, fmt.Errorf("unmarshal upstream_converted: %w", err)
	}
	it.UpstreamConverted = conv
	loc, err := service.UnmarshalConverted(localCurrent)
	if err != nil {
		return nil, fmt.Errorf("unmarshal local_current: %w", err)
	}
	it.LocalCurrent = loc
	apv, err := service.UnmarshalConverted(applyValue)
	if err != nil {
		return nil, fmt.Errorf("unmarshal apply_value: %w", err)
	}
	it.ApplyValue = apv
	if len(groupRateChange) > 0 && string(groupRateChange) != "null" {
		var change service.GroupRateChange
		if err := json.Unmarshal(groupRateChange, &change); err != nil {
			return nil, fmt.Errorf("unmarshal group_rate_change: %w", err)
		}
		it.GroupRateChange = &change
	}
	if applyRate.Valid {
		v := applyRate.Float64
		it.ApplyRate = &v
	}
	if reviewerID.Valid {
		it.ReviewerID = reviewerID.Int64
	}
	it.ReviewNote = reviewNote.String
	if reviewedAt.Valid {
		t := reviewedAt.Time
		it.ReviewedAt = &t
	}
	if appliedAt.Valid {
		t := appliedAt.Time
		it.AppliedAt = &t
	}
	return &it, nil
}
