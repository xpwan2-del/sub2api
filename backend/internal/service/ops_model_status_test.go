package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type opsModelStatusAccountRepoStub struct {
	AccountRepository
	accounts []Account
}

func (s *opsModelStatusAccountRepoStub) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]Account, *pagination.PaginationResult, error) {
	filtered := make([]Account, 0, len(s.accounts))
	for _, acc := range s.accounts {
		if platform != "" && acc.Platform != platform {
			continue
		}
		filtered = append(filtered, acc)
	}
	return filtered, &pagination.PaginationResult{
		Page:     params.Page,
		PageSize: params.PageSize,
		Total:    int64(len(filtered)),
	}, nil
}

func TestOpsServiceGetModelStatusSnapshotMergesConfigAndTraffic(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	lastError := end.Add(-time.Minute)
	lastSeen := end.Add(-30 * time.Second)

	repo := &opsRepoMock{
		GetModelTrafficStatsFn: func(ctx context.Context, filter *OpsModelStatusFilter) ([]*OpsModelTrafficStats, error) {
			return []*OpsModelTrafficStats{
				{
					Platform:      "openai",
					Model:         "gpt-4o",
					RequestCount:  100,
					SuccessCount:  98,
					ErrorCount:    2,
					TokenConsumed: 5000,
					LastSeenAt:    &lastSeen,
				},
				{
					Platform:      "openai",
					Model:         "gpt-4.1",
					RequestCount:  10,
					SuccessCount:  5,
					ErrorCount:    5,
					LastErrorAt:   &lastError,
					LastErrorType: "upstream_error",
				},
			}, nil
		},
		GetModelHealthBucketsFn: func(ctx context.Context, filter *OpsModelStatusFilter, bucketSeconds int) ([]*OpsModelHealthBucket, error) {
			bucketStart := filter.EndTime.Add(-time.Hour)
			return []*OpsModelHealthBucket{
				{
					Platform: "openai",
					Model:    "gpt-4o",
					OpsHealthHistoryPoint: OpsHealthHistoryPoint{
						BucketStart:  bucketStart,
						BucketEnd:    bucketStart.Add(time.Hour),
						RequestCount: 100,
						SuccessCount: 98,
						ErrorCount:   2,
					},
				},
			}, nil
		},
		GetGatewayRouteHealthFn: func(ctx context.Context, filter *OpsModelStatusFilter, limit int) ([]*OpsGatewayRouteHealth, error) {
			rate := 98.5
			p95 := 456.0
			return []*OpsGatewayRouteHealth{
				{
					Endpoint:     "/v1/chat/completions",
					RequestCount: 100,
					SuccessCount: 98,
					ErrorCount:   2,
					SuccessRate:  &rate,
					P95LatencyMs: &p95,
				},
			}, nil
		},
	}
	accounts := []Account{
		{
			ID:          1,
			Name:        "openai-main",
			Platform:    "openai",
			Status:      StatusActive,
			Schedulable: true,
			Credentials: map[string]any{
				"model_mapping": map[string]any{"gpt-4o": "gpt-4o"},
			},
			Groups: []*Group{{
				ID:       10,
				Name:     "OpenAI",
				Platform: "openai",
				ModelsListConfig: GroupModelsListConfig{
					Enabled: true,
					Models:  []string{"gpt-4o", "gpt-4.1"},
				},
			}},
		},
	}

	svc := NewOpsService(repo, nil, nil, &opsModelStatusAccountRepoStub{accounts: accounts}, nil, nil, nil, nil, nil, nil, nil)
	resp, err := svc.GetModelStatusSnapshot(context.Background(), &OpsModelStatusFilter{
		StartTime: start,
		EndTime:   end,
		Page:      1,
		PageSize:  20,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, int64(2), resp.Pagination.Total)
	require.Len(t, resp.Models, 2)
	require.Equal(t, 1, resp.ModelSummary.Operational)
	require.Equal(t, 1, resp.ModelSummary.Failed)
	require.Equal(t, int64(1), resp.AccountAvailability.AvailableAccounts)
	require.Equal(t, "disabled", resp.CloudMetrics.Status)
	require.NotEmpty(t, resp.RecentErrors)
	var gpt4o *OpsModelStatusItem
	for _, item := range resp.Models {
		if item.Model == "gpt-4o" {
			gpt4o = item
			break
		}
	}
	require.NotNil(t, gpt4o)
	require.Len(t, gpt4o.History, opsModelHealthBucketCount)
	require.Equal(t, "degraded", gpt4o.History[opsModelHealthBucketCount-1].Status)
	require.Len(t, resp.GatewaySummary.History, opsModelHealthBucketCount)
	require.Len(t, resp.GatewaySummary.Routes, 1)
	require.Equal(t, "degraded", resp.GatewaySummary.Routes[0].Status)
}

func TestOpsServiceGetModelStatusSnapshotRequiresRepo(t *testing.T) {
	svc := NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	_, err := svc.GetModelStatusSnapshot(context.Background(), &OpsModelStatusFilter{
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now(),
	})
	require.Error(t, err)
}
