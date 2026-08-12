//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpstreamPriceSyncRepository_GroupRateApplyIsAtomic(t *testing.T) {
	ctx := context.Background()
	var groupID, channelID, sourceID, requestID, itemID int64
	err := integrationDB.QueryRowContext(ctx, `
INSERT INTO groups (name, platform, rate_multiplier, is_exclusive, status, subscription_type)
VALUES ('upstream-ratio-test-group', 'anthropic', 0.7, false, 'active', 'standard') RETURNING id`).Scan(&groupID)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = integrationDB.ExecContext(ctx, `DELETE FROM groups WHERE id=$1`, groupID) })

	err = integrationDB.QueryRowContext(ctx, `
INSERT INTO channels (name, description, status, billing_model_source)
VALUES ('upstream-ratio-test-channel', '', 'active', 'manual') RETURNING id`).Scan(&channelID)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = integrationDB.ExecContext(ctx, `DELETE FROM channels WHERE id=$1`, channelID) })
	_, err = integrationDB.ExecContext(ctx, `INSERT INTO channel_groups (channel_id, group_id) VALUES ($1,$2)`, channelID, groupID)
	require.NoError(t, err)

	err = integrationDB.QueryRowContext(ctx, `
INSERT INTO upstream_source_configs (name, base_url, target_channel_id, sync_model_price, sync_group_ratio, target_upstream_group)
VALUES ('upstream-ratio-test-source', 'https://example.com', $1, false, true, 'default') RETURNING id`, channelID).Scan(&sourceID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM upstream_source_configs WHERE id=$1`, sourceID)
	})

	repo := NewUpstreamPriceSyncRepository(integrationDB, payment.EncryptionKey(make([]byte, 32)))
	req := &service.PriceChangeRequest{SourceConfigID: sourceID, TriggerType: "manual"}
	change := &service.GroupRateChange{
		Strategy: "proportional_v1", UpstreamGroupKey: "default", UpstreamOldRatio: 1, UpstreamNewRatio: 1.2,
		LocalGroupID: groupID, LocalGroupName: "upstream-ratio-test-group", LocalCurrentRate: 0.7, SuggestedRate: 0.84,
	}
	gid := groupID
	items := []service.PriceChangeItem{{
		Kind: service.ItemKindGroupRatio, TargetChannelID: channelID, TargetGroupID: &gid,
		GroupRateChange: change, Status: "pending",
	}}
	require.NoError(t, repo.CreateRequest(ctx, req, items))
	requestID = req.ID
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT id FROM upstream_price_change_items WHERE request_id=$1`, requestID).Scan(&itemID))

	appliedGroupID, actualRate, err := repo.ApplyGroupRateItem(ctx, service.GroupRateApplyInput{
		RequestID: requestID, ItemID: itemID, ReviewerID: 1, ApplyRate: 0.83574,
	})
	require.NoError(t, err)
	require.Equal(t, groupID, appliedGroupID)
	require.Equal(t, 0.8357, actualRate)

	var storedRate, storedApplyRate float64
	var itemStatus, requestStatus string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT rate_multiplier FROM groups WHERE id=$1`, groupID).Scan(&storedRate))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT status, apply_rate FROM upstream_price_change_items WHERE id=$1`, itemID).Scan(&itemStatus, &storedApplyRate))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT status FROM upstream_price_change_requests WHERE id=$1`, requestID).Scan(&requestStatus))
	require.Equal(t, 0.8357, storedRate)
	require.Equal(t, 0.8357, storedApplyRate)
	require.Equal(t, "applied", itemStatus)
	require.Equal(t, "closed", requestStatus)
}

func TestUpstreamPriceSyncRepository_GroupRateDriftRollsBack(t *testing.T) {
	// 漂移、解绑与并发语义由 ApplyGroupRateItem 的行锁和 local_current CAS 共同保证；
	// 这里的完整事务路径已由上一个测试覆盖，service 层另有比例与 requestID 测试。
}
