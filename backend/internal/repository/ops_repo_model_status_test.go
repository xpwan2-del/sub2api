package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryGetModelTrafficStats(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	filter := &service.OpsModelStatusFilter{
		StartTime: start,
		EndTime:   end,
		Platform:  "OpenAI",
		Query:     "gpt",
	}

	rows := sqlmock.NewRows([]string{
		"platform",
		"model",
		"request_count",
		"success_count",
		"error_count",
		"token_consumed",
		"avg_latency_ms",
		"p95_latency_ms",
		"last_seen_at",
		"last_error_at",
		"last_error_type",
		"last_error_status_code",
	}).AddRow(
		"openai",
		"gpt-4o",
		int64(12),
		int64(10),
		int64(2),
		int64(4096),
		123.45,
		456.78,
		end.Add(-time.Minute),
		end.Add(-30*time.Second),
		"upstream_error",
		500,
	)

	mock.ExpectQuery(`WITH usage_stats AS`).
		WithArgs(start, end, "openai", "%gpt%").
		WillReturnRows(rows)

	stats, err := repo.GetModelTrafficStats(context.Background(), filter)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, "openai", stats[0].Platform)
	require.Equal(t, "gpt-4o", stats[0].Model)
	require.Equal(t, int64(12), stats[0].RequestCount)
	require.Equal(t, int64(10), stats[0].SuccessCount)
	require.Equal(t, int64(2), stats[0].ErrorCount)
	require.NotNil(t, stats[0].AvgLatencyMs)
	require.InDelta(t, 123.45, *stats[0].AvgLatencyMs, 0.001)
	require.NotNil(t, stats[0].P95LatencyMs)
	require.NotNil(t, stats[0].LastErrorStatusCode)
	require.Equal(t, 500, *stats[0].LastErrorStatusCode)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryGetModelTrafficStatsRequiresFilter(t *testing.T) {
	db, _ := newSQLMock(t)
	repo := &opsRepository{db: db}

	_, err := repo.GetModelTrafficStats(context.Background(), nil)
	require.Error(t, err)
}

func TestOpsRepositoryGetModelHealthBuckets(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(48 * time.Hour)
	bucketStart := end.Add(-time.Hour)
	filter := &service.OpsModelStatusFilter{
		StartTime: start,
		EndTime:   end,
		Platform:  "OpenAI",
		Query:     "gpt",
	}

	rows := sqlmock.NewRows([]string{
		"platform",
		"model",
		"bucket_start",
		"request_count",
		"success_count",
		"error_count",
		"token_consumed",
		"avg_latency_ms",
		"p50_latency_ms",
		"p95_latency_ms",
		"p99_latency_ms",
	}).AddRow(
		"openai",
		"gpt-4o",
		bucketStart,
		int64(12),
		int64(10),
		int64(2),
		int64(4096),
		123.45,
		100.0,
		456.78,
		900.0,
	)

	mock.ExpectQuery(`WITH usage_buckets AS`).
		WithArgs(start, end, 3600, "openai", "%gpt%").
		WillReturnRows(rows)

	buckets, err := repo.GetModelHealthBuckets(context.Background(), filter, 3600)
	require.NoError(t, err)
	require.Len(t, buckets, 1)
	require.Equal(t, "openai", buckets[0].Platform)
	require.Equal(t, "gpt-4o", buckets[0].Model)
	require.Equal(t, int64(12), buckets[0].RequestCount)
	require.Equal(t, int64(10), buckets[0].SuccessCount)
	require.Equal(t, int64(2), buckets[0].ErrorCount)
	require.NotNil(t, buckets[0].SuccessRate)
	require.InDelta(t, 83.33, *buckets[0].SuccessRate, 0.01)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryGetGatewayRouteHealth(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	filter := &service.OpsModelStatusFilter{
		StartTime: start,
		EndTime:   end,
		Platform:  "OpenAI",
		Query:     "gpt",
	}

	rows := sqlmock.NewRows([]string{
		"endpoint",
		"request_count",
		"success_count",
		"error_count",
		"token_consumed",
		"avg_latency_ms",
		"p50_latency_ms",
		"p95_latency_ms",
		"p99_latency_ms",
	}).AddRow(
		"/v1/chat/completions",
		int64(20),
		int64(19),
		int64(1),
		int64(8192),
		250.0,
		180.0,
		600.0,
		900.0,
	)

	mock.ExpectQuery(`WITH route_events AS`).
		WithArgs(start, end, "openai", "%gpt%", 14).
		WillReturnRows(rows)

	routes, err := repo.GetGatewayRouteHealth(context.Background(), filter, 14)
	require.NoError(t, err)
	require.Len(t, routes, 1)
	require.Equal(t, "/v1/chat/completions", routes[0].Endpoint)
	require.Equal(t, int64(20), routes[0].RequestCount)
	require.NotNil(t, routes[0].SuccessRate)
	require.InDelta(t, 95.0, *routes[0].SuccessRate, 0.001)
	require.NoError(t, mock.ExpectationsWereMet())
}
