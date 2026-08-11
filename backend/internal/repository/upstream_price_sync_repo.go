package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
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
const selectConfigSQL = `SELECT id, name, base_url, api_key_encrypted, dashboard_token_encrypted,
 dashboard_auth_mode, dashboard_user_id, proxy_id,
 target_channel_id, enabled, base_price_per_1k, pricing_source,
 sync_model_price, sync_group_ratio, group_mapping, target_upstream_group, balance_threshold_usd,
 last_balance_quota, last_used_quota, last_balance_usd, last_balance_at,
 last_balance_checked_at, last_balance_error,
 last_sync_at, last_pricing_version, last_error, created_at, updated_at
FROM upstream_source_configs`

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
	return r.db.QueryRowContext(ctx, `
INSERT INTO upstream_source_configs
(name, base_url, api_key_encrypted, dashboard_token_encrypted, dashboard_auth_mode, dashboard_user_id, proxy_id,
 target_channel_id, enabled, base_price_per_1k, pricing_source,
 sync_model_price, sync_group_ratio, group_mapping, target_upstream_group, balance_threshold_usd)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id, created_at, updated_at`,
		c.Name, c.BaseURL, ak, dt, service.NormalizeDashboardAuthMode(c.DashboardAuthMode), c.DashboardUserID, c.ProxyID,
		c.TargetChannelID, c.Enabled,
		c.BasePricePer1k, string(c.PricingSource), c.SyncModelPrice, c.SyncGroupRatio, gm, c.TargetUpstreamGroup, c.BalanceThresholdUSD,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
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
	return r.db.QueryRowContext(ctx, `
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
    updated_at=now()
WHERE id=$1
RETURNING updated_at`,
		c.ID, c.Name, c.BaseURL, ak, dt,
		service.NormalizeDashboardAuthMode(c.DashboardAuthMode), c.DashboardUserID, c.ProxyID,
		c.TargetChannelID, c.Enabled,
		c.BasePricePer1k, string(c.PricingSource), c.SyncModelPrice, c.SyncGroupRatio,
		gm, c.TargetUpstreamGroup, c.BalanceThresholdUSD, c.ResetBalanceSnapshot,
	).Scan(&c.UpdatedAt)
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
func scanConfig(row rowScanner, r *upstreamPriceSyncRepo) (*service.UpstreamSourceConfig, error) {
	var (
		c                 service.UpstreamSourceConfig
		apiKeyEnc         string
		dashTokenEnc      string
		dashboardAuthMode sql.NullString
		dashboardUserID   sql.NullInt64
		pricingSource     string
		groupMapping      []byte
		balanceThresh     sql.NullFloat64
		lastBalanceQuota  sql.NullInt64
		lastUsedQuota     sql.NullInt64
		lastBalanceUSD    sql.NullFloat64
		lastBalanceAt     sql.NullTime
		lastBalanceCheck  sql.NullTime
		lastBalanceError  sql.NullString
		lastSyncAt        sql.NullTime
		lastPricingV      sql.NullString
		lastErr           sql.NullString
	)
	if err := row.Scan(
		&c.ID, &c.Name, &c.BaseURL, &apiKeyEnc, &dashTokenEnc,
		&dashboardAuthMode, &dashboardUserID, &c.ProxyID,
		&c.TargetChannelID, &c.Enabled, &c.BasePricePer1k, &pricingSource,
		&c.SyncModelPrice, &c.SyncGroupRatio, &groupMapping, &c.TargetUpstreamGroup, &balanceThresh,
		&lastBalanceQuota, &lastUsedQuota, &lastBalanceUSD, &lastBalanceAt,
		&lastBalanceCheck, &lastBalanceError,
		&lastSyncAt, &lastPricingV, &lastErr, &c.CreatedAt, &c.UpdatedAt,
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
 upstream_raw, upstream_converted, local_current, apply_value,
 status, reviewer_id, review_note, reviewed_at, applied_at, created_at
FROM upstream_price_change_items`

// CreateRequest 在单事务内插入审批批次及其全部条目。任一条目插入失败回滚整批。
func (r *upstreamPriceSyncRepo) CreateRequest(ctx context.Context, req *service.PriceChangeRequest, items []service.PriceChangeItem) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	summary, _ := json.Marshal(req.Summary)
	err = tx.QueryRowContext(ctx, `
INSERT INTO upstream_price_change_requests (source_config_id, trigger_type, status, upstream_pricing_version, summary, created_by)
VALUES ($1,$2,$3,$4,$5,$6) RETURNING id, created_at`,
		req.SourceConfigID, req.TriggerType, "open", req.UpstreamPricingVersion, summary, req.CreatedBy,
	).Scan(&req.ID, &req.CreatedAt)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	for i := range items {
		it := &items[i]
		it.RequestID = req.ID
		raw, mErr := json.Marshal(it.UpstreamRaw)
		if mErr != nil {
			return fmt.Errorf("marshal upstream_raw: %w", mErr)
		}
		up, mErr := service.MarshalConverted(it.UpstreamConverted)
		if mErr != nil {
			return fmt.Errorf("marshal upstream_converted: %w", mErr)
		}
		loc, mErr := service.MarshalConverted(it.LocalCurrent)
		if mErr != nil {
			return fmt.Errorf("marshal local_current: %w", mErr)
		}
		apv, mErr := service.MarshalConverted(it.ApplyValue)
		if mErr != nil {
			return fmt.Errorf("marshal apply_value: %w", mErr)
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO upstream_price_change_items
(request_id, kind, platform, model_name, target_channel_id, upstream_raw, upstream_converted, local_current, apply_value, status)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			it.RequestID, string(it.Kind), it.Platform, it.ModelName, it.TargetChannelID,
			raw, up, loc, apv, "pending")
		if err != nil {
			return fmt.Errorf("create item: %w", err)
		}
	}
	return tx.Commit()
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
		upstreamRaw       []byte
		upstreamConverted []byte
		localCurrent      []byte
		applyValue        []byte
		reviewerID        sql.NullInt64
		reviewNote        sql.NullString
		reviewedAt        sql.NullTime
		appliedAt         sql.NullTime
	)
	if err := row.Scan(
		&it.ID, &it.RequestID, &kind, &platform, &modelName, &targetChannelID,
		&upstreamRaw, &upstreamConverted, &localCurrent, &applyValue,
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
