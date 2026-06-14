package service

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- fakes ----

// applyFakeRepo 内存版 UpstreamPriceRepository，覆盖 GetChange / UpdateChangeApplied / UpdateChangeDismissed。
type applyFakeRepo struct {
	changes map[int64]*dbent.UpstreamPriceChange

	appliedChange struct {
		id, adminID, targetID int64
		target                string
		called                bool
	}
	dismissedChange struct {
		id, adminID int64
		called      bool
	}
	snapshot struct {
		id, channelID   int64
		prevIn, prevOut float64
		prevMul         *float64
		called          bool
	}
	reverted struct {
		id, adminID int64
		called      bool
	}
	channelsSnapshot []AppliedChannelSnapshot
}

func newApplyFakeRepo() *applyFakeRepo {
	return &applyFakeRepo{changes: map[int64]*dbent.UpstreamPriceChange{}}
}

func (r *applyFakeRepo) GetChange(_ context.Context, id int64) (*dbent.UpstreamPriceChange, error) {
	c, ok := r.changes[id]
	if !ok {
		return nil, ErrUpstreamPriceChangeNotFound
	}
	return c, nil
}
func (r *applyFakeRepo) UpdateChangeApplied(_ context.Context, id, adminID int64, target string, targetID int64) error {
	r.appliedChange = struct {
		id, adminID, targetID int64
		target                string
		called                bool
	}{id: id, adminID: adminID, target: target, targetID: targetID, called: true}
	if c, ok := r.changes[id]; ok {
		c.Status = UpstreamPriceChangeStatusApplied
	}
	return nil
}
func (r *applyFakeRepo) UpdateChangeDismissed(_ context.Context, id, adminID int64) error {
	r.dismissedChange = struct {
		id, adminID int64
		called      bool
	}{id: id, adminID: adminID, called: true}
	if c, ok := r.changes[id]; ok {
		c.Status = UpstreamPriceChangeStatusDismissed
	}
	return nil
}
func (r *applyFakeRepo) SetAppliedSnapshot(_ context.Context, id, channelID int64, prevIn, prevOut float64, prevMul *float64) error {
	r.snapshot = struct {
		id, channelID   int64
		prevIn, prevOut float64
		prevMul         *float64
		called          bool
	}{id: id, channelID: channelID, prevIn: prevIn, prevOut: prevOut, prevMul: prevMul, called: true}
	return nil
}
func (r *applyFakeRepo) MarkReverted(_ context.Context, id, adminID int64) error {
	r.reverted = struct {
		id, adminID int64
		called      bool
	}{id: id, adminID: adminID, called: true}
	if c, ok := r.changes[id]; ok {
		now := time.Now()
		c.RevertedAt = &now
	}
	return nil
}
func (r *applyFakeRepo) SetAppliedChannelsSnapshot(_ context.Context, _ int64, snapshots []AppliedChannelSnapshot) error {
	r.channelsSnapshot = snapshots
	return nil
}
func (r *applyFakeRepo) GetAppliedChannelsSnapshot(_ context.Context, _ int64) ([]AppliedChannelSnapshot, error) {
	return r.channelsSnapshot, nil
}

// 其余未使用方法（满足接口）
func (r *applyFakeRepo) CreateSource(context.Context, *dbent.UpstreamPriceSource) error {
	panic("unused")
}
func (r *applyFakeRepo) UpdateSource(context.Context, *dbent.UpstreamPriceSource) error {
	panic("unused")
}
func (r *applyFakeRepo) DeleteSource(context.Context, int64) error { panic("unused") }
func (r *applyFakeRepo) GetSource(context.Context, int64) (*dbent.UpstreamPriceSource, error) {
	panic("unused")
}
func (r *applyFakeRepo) ListSources(context.Context) ([]*dbent.UpstreamPriceSource, error) {
	panic("unused")
}
func (r *applyFakeRepo) ListEnabledSources(context.Context) ([]*dbent.UpstreamPriceSource, error) {
	panic("unused")
}
func (r *applyFakeRepo) UpdateSourceSyncResult(context.Context, int64, string, string, string, time.Time) error {
	panic("unused")
}
func (r *applyFakeRepo) ReplaceModelPrices(context.Context, int64, []*dbent.UpstreamModelPrice) error {
	panic("unused")
}
func (r *applyFakeRepo) ListModelPrices(context.Context, int64) ([]*dbent.UpstreamModelPrice, error) {
	panic("unused")
}
func (r *applyFakeRepo) ListAllModelPricesAsMap(context.Context, int64) (map[string]*dbent.UpstreamModelPrice, error) {
	panic("unused")
}
func (r *applyFakeRepo) InsertChanges(context.Context, []*dbent.UpstreamPriceChange) error {
	panic("unused")
}
func (r *applyFakeRepo) ListPendingChanges(_ context.Context, filters ChangeFilters) ([]*dbent.UpstreamPriceChange, error) {
	var out []*dbent.UpstreamPriceChange
	for _, c := range r.changes {
		if filters.Status != "" && c.Status != filters.Status {
			continue
		}
		if filters.SourceID != 0 && c.SourceID != filters.SourceID {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}
func (r *applyFakeRepo) MarkChangesNotified(context.Context, []int64) error { panic("unused") }

// channelWriteCall 记录单次 ReplaceModelPricingForModel 调用，供多 channel 撤销测试断言。
type channelWriteCall struct {
	channelID   int64
	model       string
	inputPrice  float64
	outputPrice float64
}

// fakeChannelWriter 记录对 channel_model_pricing 的写入调用。
type fakeChannelWriter struct {
	lastChannelID    int64
	lastModel        string
	lastInputPrice   float64
	lastOutputPrice  float64
	cacheInvalidated bool
	called           bool

	calls []channelWriteCall

	curInputPrice  float64
	curOutputPrice float64
}

func (f *fakeChannelWriter) ReplaceModelPricingForModel(_ context.Context, channelID int64, modelName string, inputPrice, outputPrice float64) error {
	f.called = true
	f.lastChannelID = channelID
	f.lastModel = modelName
	f.lastInputPrice = inputPrice
	f.lastOutputPrice = outputPrice
	f.calls = append(f.calls, channelWriteCall{
		channelID:   channelID,
		model:       modelName,
		inputPrice:  inputPrice,
		outputPrice: outputPrice,
	})
	return nil
}
func (f *fakeChannelWriter) InvalidateChannelCache() { f.cacheInvalidated = true }

// GetCurrentPriceForModel 支持 apply 前快照的读路径（type assertion 扩展）。
func (f *fakeChannelWriter) GetCurrentPriceForModel(_ context.Context, _ int64, _ string) (float64, float64, error) {
	return f.curInputPrice, f.curOutputPrice, nil
}

// fakeChannelWriterWithResolver 同时支持 lock_price 的 group→channel 解析。
type fakeChannelWriterWithResolver struct {
	fakeChannelWriter
	groupToChannel map[int64]int64
}

func (f *fakeChannelWriterWithResolver) GetChannelIDForGroup(_ context.Context, groupID int64) (int64, error) {
	if ch, ok := f.groupToChannel[groupID]; ok {
		return ch, nil
	}
	return 0, nil
}

// fakeGroupWriter 记录对 group.rate_multiplier 的写入调用。
type fakeGroupWriter struct {
	lastGroupID    int64
	lastMultiplier float64
	called         bool

	curMultiplier float64
}

func (f *fakeGroupWriter) UpdateRateMultiplier(_ context.Context, groupID int64, multiplier float64) error {
	f.called = true
	f.lastGroupID = groupID
	f.lastMultiplier = multiplier
	return nil
}

// GetRateMultiplierByGroupID 支持 apply 前快照的读路径（type assertion 扩展）。
func (f *fakeGroupWriter) GetRateMultiplierByGroupID(_ context.Context, _ int64) (float64, error) {
	return f.curMultiplier, nil
}

// captureAuditLogger 记录最后一条审计事件。
type captureAuditLogger struct {
	last AuditEvent
	n    int
}

func (l *captureAuditLogger) Log(_ context.Context, e AuditEvent) {
	l.last = e
	l.n++
}

// ---- helpers ----

func ptrFloatApply(v float64) *float64 { return &v }

func ptrInt64Apply(v int64) *int64 { return &v }

func newPendingChange(id int64) *dbent.UpstreamPriceChange {
	return &dbent.UpstreamPriceChange{
		ID:              id,
		Status:          UpstreamPriceChangeStatusPending,
		ModelName:       "claude-opus-4",
		LocalModelName:  "claude-opus-4",
		CurrInputPrice:  0.015,
		CurrOutputPrice: 0.075,
	}
}

// ---- tests ----

func TestApply_FollowCost_WritesChannelPricingAndMarksApplied(t *testing.T) {
	repo := newApplyFakeRepo()
	repo.changes[1] = newPendingChange(1)
	ch := &fakeChannelWriter{}
	grp := &fakeGroupWriter{}
	audit := &captureAuditLogger{}

	svc := NewUpstreamPriceApplyService(repo, ch, grp, nil, audit)
	err := svc.Apply(context.Background(), ApplyRequest{ChangeID: 1, Mode: ApplyFollowCost, TargetID: 10}, 99)
	require.NoError(t, err)

	// 写了 channel 单价
	assert.True(t, ch.called)
	assert.Equal(t, int64(10), ch.lastChannelID)
	assert.Equal(t, "claude-opus-4", ch.lastModel)
	assert.Equal(t, 0.015, ch.lastInputPrice)
	assert.Equal(t, 0.075, ch.lastOutputPrice)
	assert.True(t, ch.cacheInvalidated)

	// 没动 group 倍率
	assert.False(t, grp.called)

	// change 标记为 applied，target=channel_pricing
	assert.True(t, repo.appliedChange.called)
	assert.Equal(t, int64(1), repo.appliedChange.id)
	assert.Equal(t, int64(99), repo.appliedChange.adminID)
	assert.Equal(t, appliedTargetChannelPricing, repo.appliedChange.target)
	assert.Equal(t, int64(10), repo.appliedChange.targetID)

	// 审计
	assert.Equal(t, 1, audit.n)
	assert.Equal(t, "upstream_price.apply", audit.last.Action)
	assert.Equal(t, int64(99), audit.last.AdminID)
}

func TestApply_LockPrice_WritesChannelPricingAndGroupMultiplier(t *testing.T) {
	repo := newApplyFakeRepo()
	c := newPendingChange(1)
	c.SuggestedMultiplier = ptrFloatApply(1.25)
	repo.changes[1] = c

	ch := &fakeChannelWriterWithResolver{groupToChannel: map[int64]int64{7: 42}}
	grp := &fakeGroupWriter{}
	audit := &captureAuditLogger{}

	svc := NewUpstreamPriceApplyService(repo, ch, grp, nil, audit)
	err := svc.Apply(context.Background(), ApplyRequest{ChangeID: 1, Mode: ApplyLockPrice, TargetID: 7}, 99)
	require.NoError(t, err)

	// channel 单价写到 group 7 关联的 channel 42
	assert.True(t, ch.called)
	assert.Equal(t, int64(42), ch.lastChannelID)

	// group 倍率更新
	assert.True(t, grp.called)
	assert.Equal(t, int64(7), grp.lastGroupID)
	assert.Equal(t, 1.25, grp.lastMultiplier)

	// change target=group_multiplier
	assert.True(t, repo.appliedChange.called)
	assert.Equal(t, appliedTargetGroupMultiplier, repo.appliedChange.target)
	assert.Equal(t, int64(7), repo.appliedChange.targetID)
}

func TestApply_NonPending_ReturnsBadRequest(t *testing.T) {
	repo := newApplyFakeRepo()
	c := newPendingChange(1)
	c.Status = UpstreamPriceChangeStatusApplied
	repo.changes[1] = c
	ch := &fakeChannelWriter{}
	grp := &fakeGroupWriter{}
	audit := &captureAuditLogger{}

	svc := NewUpstreamPriceApplyService(repo, ch, grp, nil, audit)
	err := svc.Apply(context.Background(), ApplyRequest{ChangeID: 1, Mode: ApplyFollowCost, TargetID: 10}, 99)

	require.Error(t, err)
	var appErr *errors.ApplicationError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "CHANGE_NOT_PENDING", appErr.Reason)
	// 没写计费
	assert.False(t, ch.called)
	assert.False(t, grp.called)
	assert.False(t, repo.appliedChange.called)
}

func TestApply_LockPrice_WithoutSuggestedMultiplier_ReturnsBadRequest(t *testing.T) {
	repo := newApplyFakeRepo()
	repo.changes[1] = newPendingChange(1) // SuggestedMultiplier = nil
	ch := &fakeChannelWriterWithResolver{}
	grp := &fakeGroupWriter{}
	audit := &captureAuditLogger{}

	svc := NewUpstreamPriceApplyService(repo, ch, grp, nil, audit)
	err := svc.Apply(context.Background(), ApplyRequest{ChangeID: 1, Mode: ApplyLockPrice, TargetID: 7}, 99)

	require.Error(t, err)
	var appErr *errors.ApplicationError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "LOCK_PRICE_NO_MULTIPLIER", appErr.Reason)
	assert.False(t, ch.called)
	assert.False(t, grp.called)
}

func TestDismiss_MarksDismissed_NoBillingWrite(t *testing.T) {
	repo := newApplyFakeRepo()
	repo.changes[1] = newPendingChange(1)
	ch := &fakeChannelWriter{}
	grp := &fakeGroupWriter{}
	audit := &captureAuditLogger{}

	svc := NewUpstreamPriceApplyService(repo, ch, grp, nil, audit)
	err := svc.Dismiss(context.Background(), 1, 88)
	require.NoError(t, err)

	assert.True(t, repo.dismissedChange.called)
	assert.Equal(t, int64(1), repo.dismissedChange.id)
	assert.Equal(t, int64(88), repo.dismissedChange.adminID)
	// 没写计费
	assert.False(t, ch.called)
	assert.False(t, grp.called)
	// 没标 applied
	assert.False(t, repo.appliedChange.called)
	// 审计
	assert.Equal(t, 1, audit.n)
	assert.Equal(t, "upstream_price.dismiss", audit.last.Action)
}

func TestBatchApply_MixedResults(t *testing.T) {
	repo := newApplyFakeRepo()
	// 1, 2: pending 可应用；3: 已 applied → 失败
	repo.changes[1] = newPendingChange(1)
	c2 := newPendingChange(2)
	c2.CurrInputPrice = 0.02
	repo.changes[2] = c2
	c3 := newPendingChange(3)
	c3.Status = UpstreamPriceChangeStatusApplied
	repo.changes[3] = c3

	ch := &fakeChannelWriter{}
	grp := &fakeGroupWriter{}
	audit := &captureAuditLogger{}
	svc := NewUpstreamPriceApplyService(repo, ch, grp, nil, audit)

	res, err := svc.BatchApply(context.Background(), []ApplyRequest{
		{ChangeID: 1, Mode: ApplyFollowCost, TargetID: 10},
		{ChangeID: 2, Mode: ApplyFollowCost, TargetID: 11},
		{ChangeID: 3, Mode: ApplyFollowCost, TargetID: 12},
	}, 99)
	require.NoError(t, err)

	assert.ElementsMatch(t, []int64{1, 2}, res.Succeeded)
	assert.Contains(t, res.Failed, int64(3))
	require.Contains(t, res.Failed, int64(3))
}

// fakeApplyTargetReader 内存版 ApplyTargetReader。
type fakeApplyTargetReader struct {
	channels    []ChannelApplyTarget
	groups      []GroupApplyTarget
	modelCounts map[int64]int // group_id → model count
}

func (f *fakeApplyTargetReader) ListChannelsByModel(_ context.Context, _ string) ([]ChannelApplyTarget, error) {
	return f.channels, nil
}
func (f *fakeApplyTargetReader) ListGroupsByChannels(_ context.Context, _ []int64) ([]GroupApplyTarget, error) {
	return f.groups, nil
}
func (f *fakeApplyTargetReader) CountDistinctModelsByGroups(_ context.Context, _ []int64) (map[int64]int, error) {
	return f.modelCounts, nil
}

func TestGetApplyTargets_LockPriceMultiModelGroup_GeneratesWarning(t *testing.T) {
	repo := newApplyFakeRepo()
	repo.changes[1] = newPendingChange(1)
	targets := &fakeApplyTargetReader{
		channels:    []ChannelApplyTarget{{ID: 10, Name: "ch-1"}},
		groups:      []GroupApplyTarget{{ID: 7, Name: "g-multi", RateMultiplier: 1.5}},
		modelCounts: map[int64]int{7: 3}, // group 7 绑定 3 个模型 → 误伤
	}
	svc := NewUpstreamPriceApplyService(repo, nil, nil, targets, NewSlogAuditLogger())

	resp, err := svc.GetApplyTargets(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, resp.Groups, 1)
	assert.Equal(t, 3, resp.Groups[0].ModelCount)
	require.Len(t, resp.Warnings, 1)
	assert.Equal(t, int64(7), resp.Warnings[0].GroupID)
}

func TestGetApplyTargets_SingleModelGroup_NoWarning(t *testing.T) {
	repo := newApplyFakeRepo()
	repo.changes[1] = newPendingChange(1)
	targets := &fakeApplyTargetReader{
		channels:    []ChannelApplyTarget{{ID: 10, Name: "ch-1"}},
		groups:      []GroupApplyTarget{{ID: 7, Name: "g-single", RateMultiplier: 1.5}},
		modelCounts: map[int64]int{7: 1}, // 单模型 → 不警告
	}
	svc := NewUpstreamPriceApplyService(repo, nil, nil, targets, NewSlogAuditLogger())

	resp, err := svc.GetApplyTargets(context.Background(), 1)
	require.NoError(t, err)
	assert.Empty(t, resp.Warnings)
}

// 确保 UpdateSourceSyncResult 签名匹配（编译期检查）：上面 applyFakeRepo 用了
// 任意 interface{} 占位参数；这里验证编译通过即可（已由 go build 保证）。

// ---- Apply snapshot recording ----

func TestApply_FollowCost_RecordsAppliedSnapshot(t *testing.T) {
	repo := newApplyFakeRepo()
	repo.changes[1] = newPendingChange(1)
	ch := &fakeChannelWriter{curInputPrice: 0.012, curOutputPrice: 0.060} // apply 前 channel 实际价
	grp := &fakeGroupWriter{}
	svc := NewUpstreamPriceApplyService(repo, ch, grp, nil, NewSlogAuditLogger())

	err := svc.Apply(context.Background(), ApplyRequest{ChangeID: 1, Mode: ApplyFollowCost, TargetID: 10}, 99)
	require.NoError(t, err)

	assert.True(t, repo.snapshot.called)
	assert.Equal(t, int64(10), repo.snapshot.channelID)
	assert.Equal(t, 0.012, repo.snapshot.prevIn)
	assert.Equal(t, 0.060, repo.snapshot.prevOut)
	assert.Nil(t, repo.snapshot.prevMul) // follow_cost 不记倍率
}

func TestApply_LockPrice_RecordsSnapshotWithMultiplier(t *testing.T) {
	repo := newApplyFakeRepo()
	c := newPendingChange(1)
	c.SuggestedMultiplier = ptrFloatApply(1.25)
	repo.changes[1] = c
	ch := &fakeChannelWriterWithResolver{groupToChannel: map[int64]int64{7: 42}}
	ch.curInputPrice, ch.curOutputPrice = 0.012, 0.060
	grp := &fakeGroupWriter{curMultiplier: 1.5} // apply 前 group 实际倍率
	svc := NewUpstreamPriceApplyService(repo, ch, grp, nil, NewSlogAuditLogger())

	err := svc.Apply(context.Background(), ApplyRequest{ChangeID: 1, Mode: ApplyLockPrice, TargetID: 7}, 99)
	require.NoError(t, err)

	assert.True(t, repo.snapshot.called)
	assert.Equal(t, int64(42), repo.snapshot.channelID) // 解析出的 channel_id
	require.NotNil(t, repo.snapshot.prevMul)
	assert.Equal(t, 1.5, *repo.snapshot.prevMul)
}

// ---- Revert ----

func TestRevert_FollowCost_RestoresChannelPrice(t *testing.T) {
	repo := newApplyFakeRepo()
	c := newPendingChange(1)
	c.Status = UpstreamPriceChangeStatusApplied
	c.AppliedTarget = appliedTargetChannelPricing
	c.AppliedTargetID = 10
	c.AppliedChannelID = ptrInt64Apply(10)
	c.AppliedPrevInputPrice = ptrFloatApply(0.012)
	c.AppliedPrevOutputPrice = ptrFloatApply(0.060)
	repo.changes[1] = c
	ch := &fakeChannelWriter{}
	grp := &fakeGroupWriter{}
	svc := NewUpstreamPriceApplyService(repo, ch, grp, nil, NewSlogAuditLogger())

	err := svc.Revert(context.Background(), 1, 99)
	require.NoError(t, err)

	assert.True(t, ch.called)
	assert.Equal(t, int64(10), ch.lastChannelID)
	assert.Equal(t, 0.012, ch.lastInputPrice)
	assert.Equal(t, 0.060, ch.lastOutputPrice)
	assert.False(t, grp.called) // follow_cost 不动倍率
	assert.True(t, repo.reverted.called)
}

func TestRevert_LockPrice_RestoresChannelPriceAndMultiplier(t *testing.T) {
	repo := newApplyFakeRepo()
	c := newPendingChange(1)
	c.Status = UpstreamPriceChangeStatusApplied
	c.AppliedTarget = appliedTargetGroupMultiplier
	c.AppliedTargetID = 7 // group_id
	c.AppliedChannelID = ptrInt64Apply(42)
	c.AppliedPrevInputPrice = ptrFloatApply(0.012)
	c.AppliedPrevOutputPrice = ptrFloatApply(0.060)
	m := 1.5
	c.PrevMultiplier = &m
	repo.changes[1] = c
	ch := &fakeChannelWriter{}
	grp := &fakeGroupWriter{}
	svc := NewUpstreamPriceApplyService(repo, ch, grp, nil, NewSlogAuditLogger())

	err := svc.Revert(context.Background(), 1, 99)
	require.NoError(t, err)

	assert.Equal(t, int64(42), ch.lastChannelID) // applied_channel_id 锚点
	assert.True(t, grp.called)
	assert.Equal(t, int64(7), grp.lastGroupID) // applied_target_id = group_id
	assert.Equal(t, 1.5, grp.lastMultiplier)
}

func TestRevert_AlreadyReverted_ReturnsError(t *testing.T) {
	repo := newApplyFakeRepo()
	c := newPendingChange(1)
	c.Status = UpstreamPriceChangeStatusApplied
	now := time.Now()
	c.RevertedAt = &now // 已撤销
	repo.changes[1] = c
	svc := NewUpstreamPriceApplyService(repo, &fakeChannelWriter{}, &fakeGroupWriter{}, nil, NewSlogAuditLogger())

	err := svc.Revert(context.Background(), 1, 99)
	require.Error(t, err)
	var appErr *errors.ApplicationError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "ALREADY_REVERTED", appErr.Reason)
}

func TestRevert_NotApplied_ReturnsError(t *testing.T) {
	repo := newApplyFakeRepo()
	c := newPendingChange(1) // Status = pending (not applied)
	repo.changes[1] = c
	ch := &fakeChannelWriter{}
	grp := &fakeGroupWriter{}
	svc := NewUpstreamPriceApplyService(repo, ch, grp, nil, NewSlogAuditLogger())

	err := svc.Revert(context.Background(), 1, 99)
	require.Error(t, err)
	var appErr *errors.ApplicationError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "NOT_APPLIED", appErr.Reason)
	// 没有任何计费写入
	assert.False(t, ch.called)
	assert.False(t, grp.called)
	assert.False(t, repo.reverted.called)
}

func TestRevert_MissingAppliedSnapshot_ReturnsError(t *testing.T) {
	repo := newApplyFakeRepo()
	c := newPendingChange(1)
	c.Status = UpstreamPriceChangeStatusApplied
	c.AppliedTarget = appliedTargetChannelPricing
	c.AppliedTargetID = 10
	// AppliedChannelID 留 nil（模拟 apply 时 snapshot 读取失败 / 旧数据）
	repo.changes[1] = c
	ch := &fakeChannelWriter{}
	grp := &fakeGroupWriter{}
	svc := NewUpstreamPriceApplyService(repo, ch, grp, nil, NewSlogAuditLogger())

	err := svc.Revert(context.Background(), 1, 99)
	require.Error(t, err)
	var appErr *errors.ApplicationError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "MISSING_APPLIED_SNAPSHOT", appErr.Reason)
	// 没有任何计费写入
	assert.False(t, ch.called)
	assert.False(t, grp.called)
	assert.False(t, repo.reverted.called)
}

func TestRevert_BatchMultiChannel_RestoresAllChannels(t *testing.T) {
	repo := newApplyFakeRepo()
	c := newPendingChange(1)
	c.Status = UpstreamPriceChangeStatusApplied
	c.AppliedTarget = appliedTargetChannelPricing
	// applied_channel_id 留 nil（批量 apply 不填单值），用 channels snapshot
	repo.changes[1] = c
	// 模拟批量 apply 记录了 2 个 channel 的 prev 价
	repo.channelsSnapshot = []AppliedChannelSnapshot{
		{ChannelID: 10, PrevInputPrice: 0.012, PrevOutputPrice: 0.060},
		{ChannelID: 11, PrevInputPrice: 0.011, PrevOutputPrice: 0.055},
	}
	ch := &fakeChannelWriter{}
	svc := NewUpstreamPriceApplyService(repo, ch, &fakeGroupWriter{}, nil, NewSlogAuditLogger())

	err := svc.Revert(context.Background(), 1, 99)
	require.NoError(t, err)

	// 两个 channel 都被恢复
	require.Len(t, ch.calls, 2)
	assert.Equal(t, int64(10), ch.calls[0].channelID)
	assert.Equal(t, 0.012, ch.calls[0].inputPrice)
	assert.Equal(t, int64(11), ch.calls[1].channelID)
	assert.Equal(t, 0.055, ch.calls[1].outputPrice)
	assert.True(t, repo.reverted.called)
}

// ---- ApplyAllPendingFollowCost (multi-channel batch) ----

func TestApplyAllPendingFollowCost_UpdatesAllChannelsAndRecordsSnapshot(t *testing.T) {
	repo := newApplyFakeRepo()
	repo.changes[1] = newPendingChange(1) // model claude-opus-4, CurrInputPrice 0.015, CurrOutputPrice 0.075
	targets := &fakeApplyTargetReader{
		channels:    []ChannelApplyTarget{{ID: 10, Name: "ch-a"}, {ID: 11, Name: "ch-b"}},
		groups:      []GroupApplyTarget{},
		modelCounts: map[int64]int{},
	}
	ch := &fakeChannelWriter{curInputPrice: 0.012, curOutputPrice: 0.060} // apply 前 channel 实际价
	svc := NewUpstreamPriceApplyService(repo, ch, &fakeGroupWriter{}, targets, NewSlogAuditLogger())

	res, err := svc.ApplyAllPendingFollowCost(context.Background(), nil, 99)
	require.NoError(t, err)
	// change 级别成功（1 change 处理完毕）
	assert.Len(t, res.Succeeded, 1)
	assert.Empty(t, res.Failed)
	// 2 个 channel 都被写新价
	require.Len(t, ch.calls, 2)
	assert.Equal(t, int64(10), ch.calls[0].channelID)
	assert.Equal(t, int64(11), ch.calls[1].channelID)
	assert.Equal(t, 0.015, ch.calls[0].inputPrice) // 写的是 change 的 curr 价
	// channels snapshot 记录了 2 个 channel 的 prev 价（供撤销）
	require.Len(t, repo.channelsSnapshot, 2)
	assert.Equal(t, int64(10), repo.channelsSnapshot[0].ChannelID)
	assert.Equal(t, 0.012, repo.channelsSnapshot[0].PrevInputPrice) // prev 价
	assert.Equal(t, int64(11), repo.channelsSnapshot[1].ChannelID)
	// change 翻 applied
	assert.True(t, repo.appliedChange.called)
}

func TestApplyAllPendingFollowCost_NoChannelRecordsFailed(t *testing.T) {
	repo := newApplyFakeRepo()
	repo.changes[1] = newPendingChange(1)
	targets := &fakeApplyTargetReader{
		channels:    []ChannelApplyTarget{}, // 0 channels
		groups:      []GroupApplyTarget{},
		modelCounts: map[int64]int{},
	}
	svc := NewUpstreamPriceApplyService(repo, &fakeChannelWriter{}, &fakeGroupWriter{}, targets, NewSlogAuditLogger())

	res, err := svc.ApplyAllPendingFollowCost(context.Background(), nil, 99)
	require.NoError(t, err)
	assert.Empty(t, res.Succeeded)
	require.Contains(t, res.Failed, int64(1))
}
