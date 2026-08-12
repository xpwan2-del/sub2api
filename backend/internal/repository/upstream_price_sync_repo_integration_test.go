//go:build integration

package repository

import (
	"context"
	"database/sql"
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

// TestUpstreamPriceSyncRepository_FinalizeItemCAS_RejectRecompute 覆盖 reject 路径:
// FinalizeItemCAS 的 UPDATE 曾把 $3 同时用于 SET status(varchar) 和 CASE WHEN $3='applied'(text),
// lib/pq 报 "inconsistent types deduced for parameter $3" 致整条 UPDATE 失败(生产 500)。
// 改用独立 bool 参数后,reject 应成功落库 applied_at=NULL,并触发 request summary/status 重算。
func TestUpstreamPriceSyncRepository_FinalizeItemCAS_RejectRecompute(t *testing.T) {
	ctx := context.Background()
	var channelID, sourceID int64
	err := integrationDB.QueryRowContext(ctx, `
INSERT INTO channels (name, description, status, billing_model_source)
VALUES ('finalize-item-test-channel', '', 'active', 'manual') RETURNING id`).Scan(&channelID)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = integrationDB.ExecContext(ctx, `DELETE FROM channels WHERE id=$1`, channelID) })

	err = integrationDB.QueryRowContext(ctx, `
INSERT INTO upstream_source_configs (name, base_url, target_channel_id, sync_model_price, sync_group_ratio, target_upstream_group)
VALUES ('finalize-item-test-source', 'https://example.com', $1, true, false, '') RETURNING id`, channelID).Scan(&sourceID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM upstream_source_configs WHERE id=$1`, sourceID)
	})

	repo := NewUpstreamPriceSyncRepository(integrationDB, payment.EncryptionKey(make([]byte, 32)))
	req := &service.PriceChangeRequest{SourceConfigID: sourceID, TriggerType: "manual"}
	items := []service.PriceChangeItem{
		{Kind: service.ItemKindModelAdded, Platform: "anthropic", ModelName: "claude-a", TargetChannelID: channelID, Status: "pending"},
		{Kind: service.ItemKindModelAdded, Platform: "anthropic", ModelName: "claude-b", TargetChannelID: channelID, Status: "pending"},
	}
	require.NoError(t, repo.CreateRequest(ctx, req, items))
	stored, err := repo.ListItems(ctx, req.ID)
	require.NoError(t, err)
	require.Len(t, stored, 2)

	// reject 第一条:仍有 pending → partially_applied;rejected item 的 applied_at 必须为 NULL。
	require.NoError(t, repo.FinalizeItemCAS(ctx, req.ID, stored[0].ID, "rejected", 1, "nope", nil))
	r1, err := repo.GetRequest(ctx, req.ID)
	require.NoError(t, err)
	require.Equal(t, "partially_applied", r1.Status)
	require.Equal(t, 1, r1.Summary["rejected"])
	require.Equal(t, 1, r1.Summary["pending"])
	var appliedAt sql.NullTime
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT applied_at FROM upstream_price_change_items WHERE id=$1`, stored[0].ID).Scan(&appliedAt))
	require.False(t, appliedAt.Valid, "applied_at must be NULL for rejected item")

	// reject 第二条:全部终态 → closed。
	require.NoError(t, repo.FinalizeItemCAS(ctx, req.ID, stored[1].ID, "rejected", 1, "nope", nil))
	r2, err := repo.GetRequest(ctx, req.ID)
	require.NoError(t, err)
	require.Equal(t, "closed", r2.Status)
	require.Equal(t, 2, r2.Summary["rejected"])
}
