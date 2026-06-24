package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) GetModelTrafficStats(ctx context.Context, filter *service.OpsModelStatusFilter) ([]*service.OpsModelTrafficStats, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		return nil, fmt.Errorf("nil filter")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, fmt.Errorf("start_time/end_time required")
	}
	if filter.StartTime.After(filter.EndTime) {
		return nil, fmt.Errorf("start_time must be <= end_time")
	}

	start := filter.StartTime.UTC()
	end := filter.EndTime.UTC()
	platform := strings.TrimSpace(strings.ToLower(filter.Platform))
	query := strings.TrimSpace(strings.ToLower(filter.Query))

	args := []any{start, end}
	idx := 3

	usageClauses := []string{
		"ul.created_at >= $1",
		"ul.created_at < $2",
		"COALESCE(NULLIF(ul.requested_model,''), NULLIF(ul.model,''), NULLIF(ul.upstream_model,'')) <> ''",
	}
	errorClauses := []string{
		"oe.created_at >= $1",
		"oe.created_at < $2",
		"oe.is_count_tokens = FALSE",
		"COALESCE(NULLIF(oe.requested_model,''), NULLIF(oe.model,''), NULLIF(oe.upstream_model,'')) <> ''",
	}

	if platform != "" {
		args = append(args, platform)
		usageClauses = append(usageClauses, fmt.Sprintf("COALESCE(NULLIF(g.platform,''), NULLIF(a.platform,''), 'unknown') = $%d", idx))
		errorClauses = append(errorClauses, fmt.Sprintf("oe.platform = $%d", idx))
		idx++
	}
	if query != "" {
		like := "%" + query + "%"
		args = append(args, like)
		usageClauses = append(usageClauses, fmt.Sprintf("LOWER(COALESCE(NULLIF(ul.requested_model,''), NULLIF(ul.model,''), NULLIF(ul.upstream_model,''))) LIKE $%d", idx))
		errorClauses = append(errorClauses, fmt.Sprintf("LOWER(COALESCE(NULLIF(oe.requested_model,''), NULLIF(oe.model,''), NULLIF(oe.upstream_model,''))) LIKE $%d", idx))
		idx++
	}

	q := `
WITH usage_stats AS (
  SELECT
    COALESCE(NULLIF(g.platform,''), NULLIF(a.platform,''), 'unknown') AS platform,
    COALESCE(NULLIF(ul.requested_model,''), NULLIF(ul.model,''), NULLIF(ul.upstream_model,'')) AS model,
    COUNT(*)::bigint AS success_count,
    COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens + ul.cache_creation_5m_tokens + ul.cache_creation_1h_tokens + ul.image_output_tokens), 0)::bigint AS token_consumed,
    ROUND(AVG(NULLIF(ul.duration_ms, 0))::numeric, 2)::float8 AS avg_latency_ms,
    percentile_cont(0.95) WITHIN GROUP (ORDER BY NULLIF(ul.duration_ms, 0))::float8 AS p95_latency_ms,
    MAX(ul.created_at) AS last_seen_at
  FROM usage_logs ul
  LEFT JOIN groups g ON g.id = ul.group_id
  LEFT JOIN accounts a ON a.id = ul.account_id
  WHERE ` + strings.Join(usageClauses, " AND ") + `
  GROUP BY 1, 2
),
error_stats AS (
  SELECT
    oe.platform AS platform,
    COALESCE(NULLIF(oe.requested_model,''), NULLIF(oe.model,''), NULLIF(oe.upstream_model,'')) AS model,
    COUNT(*)::bigint AS error_count,
    MAX(oe.created_at) AS last_error_at,
    (ARRAY_AGG(oe.error_type ORDER BY oe.created_at DESC))[1] AS last_error_type,
    (ARRAY_AGG(oe.status_code ORDER BY oe.created_at DESC))[1] AS last_error_status_code
  FROM ops_error_logs oe
  WHERE ` + strings.Join(errorClauses, " AND ") + `
  GROUP BY 1, 2
)
SELECT
  COALESCE(NULLIF(u.platform,''), NULLIF(e.platform,''), 'unknown') AS platform,
  COALESCE(u.model, e.model) AS model,
  COALESCE(u.success_count, 0) + COALESCE(e.error_count, 0) AS request_count,
  COALESCE(u.success_count, 0) AS success_count,
  COALESCE(e.error_count, 0) AS error_count,
  COALESCE(u.token_consumed, 0) AS token_consumed,
  u.avg_latency_ms,
  u.p95_latency_ms,
  u.last_seen_at,
  e.last_error_at,
  COALESCE(e.last_error_type, '') AS last_error_type,
  e.last_error_status_code
FROM usage_stats u
FULL OUTER JOIN error_stats e ON u.platform = e.platform AND u.model = e.model
ORDER BY request_count DESC, platform ASC, model ASC`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.OpsModelTrafficStats, 0, 64)
	for rows.Next() {
		item := &service.OpsModelTrafficStats{}
		var avgLatency sql.NullFloat64
		var p95Latency sql.NullFloat64
		var lastSeen sql.NullTime
		var lastError sql.NullTime
		var lastErrorStatus sql.NullInt64
		if err := rows.Scan(
			&item.Platform,
			&item.Model,
			&item.RequestCount,
			&item.SuccessCount,
			&item.ErrorCount,
			&item.TokenConsumed,
			&avgLatency,
			&p95Latency,
			&lastSeen,
			&lastError,
			&item.LastErrorType,
			&lastErrorStatus,
		); err != nil {
			return nil, err
		}
		if avgLatency.Valid {
			v := avgLatency.Float64
			item.AvgLatencyMs = &v
		}
		if p95Latency.Valid {
			v := p95Latency.Float64
			item.P95LatencyMs = &v
		}
		if lastSeen.Valid {
			t := lastSeen.Time
			item.LastSeenAt = &t
		}
		if lastError.Valid {
			t := lastError.Time
			item.LastErrorAt = &t
		}
		if lastErrorStatus.Valid {
			v := int(lastErrorStatus.Int64)
			item.LastErrorStatusCode = &v
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *opsRepository) GetModelHealthBuckets(ctx context.Context, filter *service.OpsModelStatusFilter, bucketSeconds int) ([]*service.OpsModelHealthBucket, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		return nil, fmt.Errorf("nil filter")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, fmt.Errorf("start_time/end_time required")
	}
	if filter.StartTime.After(filter.EndTime) {
		return nil, fmt.Errorf("start_time must be <= end_time")
	}
	if bucketSeconds <= 0 {
		return nil, fmt.Errorf("bucket_seconds must be positive")
	}

	start := filter.StartTime.UTC()
	end := filter.EndTime.UTC()
	platform := strings.TrimSpace(strings.ToLower(filter.Platform))
	query := strings.TrimSpace(strings.ToLower(filter.Query))

	args := []any{start, end, bucketSeconds}
	idx := 4

	usageClauses := []string{
		"ul.created_at >= $1",
		"ul.created_at < $2",
		"COALESCE(NULLIF(ul.requested_model,''), NULLIF(ul.model,''), NULLIF(ul.upstream_model,'')) <> ''",
	}
	errorClauses := []string{
		"oe.created_at >= $1",
		"oe.created_at < $2",
		"oe.is_count_tokens = FALSE",
		"COALESCE(NULLIF(oe.requested_model,''), NULLIF(oe.model,''), NULLIF(oe.upstream_model,'')) <> ''",
	}

	if platform != "" {
		args = append(args, platform)
		usageClauses = append(usageClauses, fmt.Sprintf("COALESCE(NULLIF(g.platform,''), NULLIF(a.platform,''), 'unknown') = $%d", idx))
		errorClauses = append(errorClauses, fmt.Sprintf("oe.platform = $%d", idx))
		idx++
	}
	if query != "" {
		like := "%" + query + "%"
		args = append(args, like)
		usageClauses = append(usageClauses, fmt.Sprintf("LOWER(COALESCE(NULLIF(ul.requested_model,''), NULLIF(ul.model,''), NULLIF(ul.upstream_model,''))) LIKE $%d", idx))
		errorClauses = append(errorClauses, fmt.Sprintf("LOWER(COALESCE(NULLIF(oe.requested_model,''), NULLIF(oe.model,''), NULLIF(oe.upstream_model,''))) LIKE $%d", idx))
		idx++
	}

	q := `
WITH usage_buckets AS (
  SELECT
    COALESCE(NULLIF(g.platform,''), NULLIF(a.platform,''), 'unknown') AS platform,
    COALESCE(NULLIF(ul.requested_model,''), NULLIF(ul.model,''), NULLIF(ul.upstream_model,'')) AS model,
    to_timestamp(floor(extract(epoch from ul.created_at) / $3) * $3) AT TIME ZONE 'UTC' AS bucket_start,
    COUNT(*)::bigint AS success_count,
    COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens + ul.cache_creation_5m_tokens + ul.cache_creation_1h_tokens + ul.image_output_tokens), 0)::bigint AS token_consumed,
    ROUND(AVG(NULLIF(ul.duration_ms, 0))::numeric, 2)::float8 AS avg_latency_ms,
    percentile_cont(0.5) WITHIN GROUP (ORDER BY NULLIF(ul.duration_ms, 0))::float8 AS p50_latency_ms,
    percentile_cont(0.95) WITHIN GROUP (ORDER BY NULLIF(ul.duration_ms, 0))::float8 AS p95_latency_ms,
    percentile_cont(0.99) WITHIN GROUP (ORDER BY NULLIF(ul.duration_ms, 0))::float8 AS p99_latency_ms
  FROM usage_logs ul
  LEFT JOIN groups g ON g.id = ul.group_id
  LEFT JOIN accounts a ON a.id = ul.account_id
  WHERE ` + strings.Join(usageClauses, " AND ") + `
  GROUP BY 1, 2, 3
),
error_buckets AS (
  SELECT
    oe.platform AS platform,
    COALESCE(NULLIF(oe.requested_model,''), NULLIF(oe.model,''), NULLIF(oe.upstream_model,'')) AS model,
    to_timestamp(floor(extract(epoch from oe.created_at) / $3) * $3) AT TIME ZONE 'UTC' AS bucket_start,
    COUNT(*)::bigint AS error_count,
    ROUND(AVG(NULLIF(oe.duration_ms, 0))::numeric, 2)::float8 AS avg_error_latency_ms
  FROM ops_error_logs oe
  WHERE ` + strings.Join(errorClauses, " AND ") + `
  GROUP BY 1, 2, 3
)
SELECT
  COALESCE(NULLIF(u.platform,''), NULLIF(e.platform,''), 'unknown') AS platform,
  COALESCE(u.model, e.model) AS model,
  COALESCE(u.bucket_start, e.bucket_start) AS bucket_start,
  COALESCE(u.success_count, 0) + COALESCE(e.error_count, 0) AS request_count,
  COALESCE(u.success_count, 0) AS success_count,
  COALESCE(e.error_count, 0) AS error_count,
  COALESCE(u.token_consumed, 0) AS token_consumed,
  COALESCE(u.avg_latency_ms, e.avg_error_latency_ms) AS avg_latency_ms,
  u.p50_latency_ms,
  u.p95_latency_ms,
  u.p99_latency_ms
FROM usage_buckets u
FULL OUTER JOIN error_buckets e
  ON u.platform = e.platform AND u.model = e.model AND u.bucket_start = e.bucket_start
ORDER BY bucket_start ASC, platform ASC, model ASC`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.OpsModelHealthBucket, 0, 256)
	for rows.Next() {
		item := &service.OpsModelHealthBucket{}
		var bucketStart time.Time
		var avgLatency sql.NullFloat64
		var p50Latency sql.NullFloat64
		var p95Latency sql.NullFloat64
		var p99Latency sql.NullFloat64
		if err := rows.Scan(
			&item.Platform,
			&item.Model,
			&bucketStart,
			&item.RequestCount,
			&item.SuccessCount,
			&item.ErrorCount,
			&item.TokenConsumed,
			&avgLatency,
			&p50Latency,
			&p95Latency,
			&p99Latency,
		); err != nil {
			return nil, err
		}
		item.BucketStart = bucketStart.UTC()
		item.BucketEnd = item.BucketStart.Add(time.Duration(bucketSeconds) * time.Second)
		if item.RequestCount > 0 {
			v := float64(item.SuccessCount) * 100 / float64(item.RequestCount)
			item.SuccessRate = &v
		}
		if avgLatency.Valid {
			v := avgLatency.Float64
			item.AvgLatencyMs = &v
		}
		if p50Latency.Valid {
			v := p50Latency.Float64
			item.P50LatencyMs = &v
		}
		if p95Latency.Valid {
			v := p95Latency.Float64
			item.P95LatencyMs = &v
		}
		if p99Latency.Valid {
			v := p99Latency.Float64
			item.P99LatencyMs = &v
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *opsRepository) GetGatewayRouteHealth(ctx context.Context, filter *service.OpsModelStatusFilter, limit int) ([]*service.OpsGatewayRouteHealth, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		return nil, fmt.Errorf("nil filter")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, fmt.Errorf("start_time/end_time required")
	}
	if filter.StartTime.After(filter.EndTime) {
		return nil, fmt.Errorf("start_time must be <= end_time")
	}
	if limit <= 0 {
		limit = 14
	}

	start := filter.StartTime.UTC()
	end := filter.EndTime.UTC()
	platform := strings.TrimSpace(strings.ToLower(filter.Platform))
	query := strings.TrimSpace(strings.ToLower(filter.Query))

	args := []any{start, end}
	idx := 3

	usageClauses := []string{
		"ul.created_at >= $1",
		"ul.created_at < $2",
	}
	errorClauses := []string{
		"oe.created_at >= $1",
		"oe.created_at < $2",
		"oe.is_count_tokens = FALSE",
	}
	if platform != "" {
		args = append(args, platform)
		usageClauses = append(usageClauses, fmt.Sprintf("COALESCE(NULLIF(g.platform,''), NULLIF(a.platform,''), 'unknown') = $%d", idx))
		errorClauses = append(errorClauses, fmt.Sprintf("oe.platform = $%d", idx))
		idx++
	}
	if query != "" {
		like := "%" + query + "%"
		args = append(args, like)
		usageClauses = append(usageClauses, fmt.Sprintf("LOWER(COALESCE(NULLIF(ul.requested_model,''), NULLIF(ul.model,''), NULLIF(ul.upstream_model,''))) LIKE $%d", idx))
		errorClauses = append(errorClauses, fmt.Sprintf("LOWER(COALESCE(NULLIF(oe.requested_model,''), NULLIF(oe.model,''), NULLIF(oe.upstream_model,''))) LIKE $%d", idx))
		idx++
	}
	args = append(args, limit)

	q := `
WITH route_events AS (
  SELECT
    COALESCE(NULLIF(ul.inbound_endpoint,''), NULLIF(ul.upstream_endpoint,''), 'unknown') AS endpoint,
    TRUE AS succeeded,
    COALESCE(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens + ul.cache_creation_5m_tokens + ul.cache_creation_1h_tokens + ul.image_output_tokens, 0)::bigint AS token_consumed,
    NULLIF(ul.duration_ms, 0)::float8 AS duration_ms
  FROM usage_logs ul
  LEFT JOIN groups g ON g.id = ul.group_id
  LEFT JOIN accounts a ON a.id = ul.account_id
  WHERE ` + strings.Join(usageClauses, " AND ") + `
  UNION ALL
  SELECT
    COALESCE(NULLIF(oe.inbound_endpoint,''), NULLIF(oe.request_path,''), NULLIF(oe.upstream_endpoint,''), 'unknown') AS endpoint,
    FALSE AS succeeded,
    0::bigint AS token_consumed,
    NULLIF(oe.duration_ms, 0)::float8 AS duration_ms
  FROM ops_error_logs oe
  WHERE ` + strings.Join(errorClauses, " AND ") + `
)
SELECT
  endpoint,
  COUNT(*)::bigint AS request_count,
  SUM(CASE WHEN succeeded THEN 1 ELSE 0 END)::bigint AS success_count,
  SUM(CASE WHEN succeeded THEN 0 ELSE 1 END)::bigint AS error_count,
  COALESCE(SUM(token_consumed), 0)::bigint AS token_consumed,
  ROUND(AVG(duration_ms)::numeric, 2)::float8 AS avg_latency_ms,
  percentile_cont(0.5) WITHIN GROUP (ORDER BY duration_ms)::float8 AS p50_latency_ms,
  percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms)::float8 AS p95_latency_ms,
  percentile_cont(0.99) WITHIN GROUP (ORDER BY duration_ms)::float8 AS p99_latency_ms
FROM route_events
GROUP BY endpoint
ORDER BY request_count DESC, endpoint ASC
LIMIT $` + fmt.Sprintf("%d", idx) + `
`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.OpsGatewayRouteHealth, 0, limit)
	for rows.Next() {
		item := &service.OpsGatewayRouteHealth{}
		var avgLatency sql.NullFloat64
		var p50Latency sql.NullFloat64
		var p95Latency sql.NullFloat64
		var p99Latency sql.NullFloat64
		if err := rows.Scan(
			&item.Endpoint,
			&item.RequestCount,
			&item.SuccessCount,
			&item.ErrorCount,
			&item.TokenConsumed,
			&avgLatency,
			&p50Latency,
			&p95Latency,
			&p99Latency,
		); err != nil {
			return nil, err
		}
		if item.RequestCount > 0 {
			v := float64(item.SuccessCount) * 100 / float64(item.RequestCount)
			item.SuccessRate = &v
		}
		if avgLatency.Valid {
			v := avgLatency.Float64
			item.AvgLatencyMs = &v
		}
		if p50Latency.Valid {
			v := p50Latency.Float64
			item.P50LatencyMs = &v
		}
		if p95Latency.Valid {
			v := p95Latency.Float64
			item.P95LatencyMs = &v
		}
		if p99Latency.Valid {
			v := p99Latency.Float64
			item.P99LatencyMs = &v
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
