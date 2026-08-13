package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// pricingEntryRepo — 仅用于 ApplyUpstreamPricingEntry 行为测试的最小 ChannelRepository。
// 只实现 ListModelPricing / CreateModelPricing / UpdateModelPricing 的语义,其余返回零值。
type pricingEntryRepo struct {
	existing []ChannelModelPricing
	created  []*ChannelModelPricing
	updated  []*ChannelModelPricing
	deleted  []int64
}

func (r *pricingEntryRepo) ListModelPricing(_ context.Context, _ int64) ([]ChannelModelPricing, error) {
	// 返回拷贝,避免被 service 原地修改污染测试断言。
	out := make([]ChannelModelPricing, len(r.existing))
	copy(out, r.existing)
	return out, nil
}
func (r *pricingEntryRepo) CreateModelPricing(_ context.Context, p *ChannelModelPricing) error {
	r.created = append(r.created, p)
	return nil
}
func (r *pricingEntryRepo) UpdateModelPricing(_ context.Context, p *ChannelModelPricing) error {
	r.updated = append(r.updated, p)
	return nil
}

// 其余 ChannelRepository 方法测试不涉及,返回零值。
func (r *pricingEntryRepo) Create(context.Context, *Channel) error { return nil }
func (r *pricingEntryRepo) GetByID(context.Context, int64) (*Channel, error) {
	return nil, nil
}
func (r *pricingEntryRepo) Update(context.Context, *Channel) error  { return nil }
func (r *pricingEntryRepo) Delete(context.Context, int64) error     { return nil }
func (r *pricingEntryRepo) List(context.Context, pagination.PaginationParams, string, string) ([]Channel, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *pricingEntryRepo) ListAll(context.Context) ([]Channel, error) { return nil, nil }
func (r *pricingEntryRepo) ExistsByName(context.Context, string) (bool, error) {
	return false, nil
}
func (r *pricingEntryRepo) ExistsByNameExcluding(context.Context, string, int64) (bool, error) {
	return false, nil
}
func (r *pricingEntryRepo) GetGroupIDs(context.Context, int64) ([]int64, error) { return nil, nil }
func (r *pricingEntryRepo) SetGroupIDs(context.Context, int64, []int64) error    { return nil }
func (r *pricingEntryRepo) GetChannelIDByGroupID(context.Context, int64) (int64, error) {
	return 0, nil
}
func (r *pricingEntryRepo) GetGroupsInOtherChannels(context.Context, int64, []int64) ([]int64, error) {
	return nil, nil
}
func (r *pricingEntryRepo) GetGroupPlatforms(context.Context, []int64) (map[int64]string, error) {
	return nil, nil
}
func (r *pricingEntryRepo) DeleteModelPricing(_ context.Context, id int64) error {
	r.deleted = append(r.deleted, id)
	return nil
}
func (r *pricingEntryRepo) ReplaceModelPricing(context.Context, int64, []ChannelModelPricing) error {
	return nil
}

// TestApplyUpstreamPricingEntry_PartialOverlapCreatesNewRow 验证 I1 修复:
// 对单模型 [a] 应用新价时,不得改写已存在的多模型行 [a,b];应新建一条单模型行,
// 且原 [a,b] 行的 UpdateModelPricing 不被调用。
func TestApplyUpstreamPricingEntry_PartialOverlapCreatesNewRow(t *testing.T) {
	originalInput := 5e-6
	repo := &pricingEntryRepo{
		existing: []ChannelModelPricing{{
			ID: 10, ChannelID: 1, Platform: PlatformAnthropic,
			Models:      []string{"claude-opus", "claude-sonnet"},
			BillingMode: BillingModeToken, InputPrice: floatPtr(originalInput),
		}},
	}
	svc := NewChannelService(repo, nil, nil, nil)

	newPrice := ConvertedPrice{BillingMode: BillingModeToken, InputPrice: floatPtr(9e-9)}
	got, err := svc.ApplyUpstreamPricingEntry(context.Background(), 1, PlatformAnthropic, []string{"claude-opus"}, newPrice)
	if err != nil {
		t.Fatalf("ApplyUpstreamPricingEntry err: %v", err)
	}

	// 不得更新原多模型行。
	if len(repo.updated) != 0 {
		t.Fatalf("expected NO UpdateModelPricing call on partial overlap, got %d (updated[0].ID=%d)", len(repo.updated), func() int64 {
			if len(repo.updated) > 0 {
				return repo.updated[0].ID
			}
			return 0
		}())
	}
	// 应新建单模型行。
	if len(repo.created) != 1 {
		t.Fatalf("expected 1 CreateModelPricing call, got %d", len(repo.created))
	}
	created := repo.created[0]
	if len(created.Models) != 1 || created.Models[0] != "claude-opus" {
		t.Fatalf("created row models = %v, want [claude-opus]", created.Models)
	}
	if created.InputPrice == nil || *created.InputPrice != 9e-9 {
		t.Fatalf("created row input = %v, want 9e-9", created.InputPrice)
	}
	// 返回值应为新建行(Models 仅含 claude-opus)。
	if got == nil || len(got.Models) != 1 || got.Models[0] != "claude-opus" {
		t.Fatalf("returned pricing models = %v, want [claude-opus]", got.Models)
	}
	// 原行价格保持不变(未被原地改写)。
	if repo.existing[0].InputPrice == nil || *repo.existing[0].InputPrice != originalInput {
		t.Fatalf("original [a,b] row input changed: got %v, want %v", repo.existing[0].InputPrice, originalInput)
	}
}

// TestApplyUpstreamPricingEntry_ExactSetUpdates 验证模型集合完全一致时走就地更新分支。
func TestApplyUpstreamPricingEntry_ExactSetUpdates(t *testing.T) {
	repo := &pricingEntryRepo{
		existing: []ChannelModelPricing{{
			ID: 11, ChannelID: 1, Platform: PlatformAnthropic,
			Models:      []string{"claude-opus"},
			BillingMode: BillingModeToken, InputPrice: floatPtr(5e-6),
		}},
	}
	svc := NewChannelService(repo, nil, nil, nil)

	newPrice := ConvertedPrice{BillingMode: BillingModeToken, InputPrice: floatPtr(7e-9), OutputPrice: floatPtr(14e-9)}
	got, err := svc.ApplyUpstreamPricingEntry(context.Background(), 1, PlatformAnthropic, []string{"claude-opus"}, newPrice)
	if err != nil {
		t.Fatalf("ApplyUpstreamPricingEntry err: %v", err)
	}
	if len(repo.updated) != 1 {
		t.Fatalf("expected 1 UpdateModelPricing call, got %d", len(repo.updated))
	}
	if repo.updated[0].ID != 11 {
		t.Fatalf("updated row id = %d, want 11", repo.updated[0].ID)
	}
	if repo.updated[0].InputPrice == nil || *repo.updated[0].InputPrice != 7e-9 {
		t.Fatalf("updated row input = %v, want 7e-9", repo.updated[0].InputPrice)
	}
	if len(repo.created) != 0 {
		t.Fatalf("expected NO CreateModelPricing on exact-set match, got %d", len(repo.created))
	}
	if got == nil || got.ID != 11 {
		t.Fatalf("returned pricing id = %v, want 11", got)
	}
}

// TestApplyUpstreamPricingEntry_ExactSetCaseInsensitive 验证集合相等的大小写不敏感。
func TestApplyUpstreamPricingEntry_ExactSetCaseInsensitive(t *testing.T) {
	repo := &pricingEntryRepo{
		existing: []ChannelModelPricing{{
			ID: 12, ChannelID: 1, Platform: PlatformAnthropic,
			Models: []string{"Claude-Opus", "Claude-Sonnet"},
		}},
	}
	svc := NewChannelService(repo, nil, nil, nil)

	newPrice := ConvertedPrice{BillingMode: BillingModeToken, InputPrice: floatPtr(3e-9)}
	if _, err := svc.ApplyUpstreamPricingEntry(context.Background(), 1, PlatformAnthropic, []string{"claude-opus", "claude-sonnet"}, newPrice); err != nil {
		t.Fatalf("ApplyUpstreamPricingEntry err: %v", err)
	}
	if len(repo.updated) != 1 || repo.updated[0].ID != 12 {
		t.Fatalf("expected update of row 12 (case-insensitive set match), got updated=%v", repo.updated)
	}
	if len(repo.created) != 0 {
		t.Fatalf("expected no create on case-insensitive exact-set match, got %d", len(repo.created))
	}
}

// TestRemoveUpstreamPricingEntry_RemovesModelFromRow 验证从多模型行中移除单个模型:
// 应就地更新该行(Models 去掉目标模型),不删除整行。
func TestRemoveUpstreamPricingEntry_RemovesModelFromRow(t *testing.T) {
	repo := &pricingEntryRepo{
		existing: []ChannelModelPricing{{
			ID: 20, ChannelID: 1, Platform: PlatformAnthropic,
			Models: []string{"claude-opus", "claude-sonnet"},
		}},
	}
	svc := NewChannelService(repo, nil, nil, nil)

	if err := svc.RemoveUpstreamPricingEntry(context.Background(), 1, PlatformAnthropic, "claude-opus"); err != nil {
		t.Fatalf("RemoveUpstreamPricingEntry err: %v", err)
	}
	if len(repo.updated) != 1 || repo.updated[0].ID != 20 {
		t.Fatalf("expected update of row 20, got updated=%v", repo.updated)
	}
	if len(repo.updated[0].Models) != 1 || repo.updated[0].Models[0] != "claude-sonnet" {
		t.Fatalf("remaining models = %v, want [claude-sonnet]", repo.updated[0].Models)
	}
	if len(repo.deleted) != 0 {
		t.Fatalf("expected no delete for multi-model row, got deleted=%v", repo.deleted)
	}
}

// TestRemoveUpstreamPricingEntry_DeletesEmptyRow 验证移除单模型行中最后一个模型:
// 应删除整行,而非更新为空。
func TestRemoveUpstreamPricingEntry_DeletesEmptyRow(t *testing.T) {
	repo := &pricingEntryRepo{
		existing: []ChannelModelPricing{{
			ID: 21, ChannelID: 1, Platform: PlatformAnthropic,
			Models: []string{"claude-opus"},
		}},
	}
	svc := NewChannelService(repo, nil, nil, nil)

	if err := svc.RemoveUpstreamPricingEntry(context.Background(), 1, PlatformAnthropic, "claude-opus"); err != nil {
		t.Fatalf("RemoveUpstreamPricingEntry err: %v", err)
	}
	if len(repo.updated) != 0 {
		t.Fatalf("expected no update, got updated=%v", repo.updated)
	}
	if len(repo.deleted) != 1 || repo.deleted[0] != 21 {
		t.Fatalf("expected delete of row 21, got deleted=%v", repo.deleted)
	}
}

// TestRemoveUpstreamPricingEntry_NoMatchIsIdempotent 验证模型不存在时幂等返回 nil。
func TestRemoveUpstreamPricingEntry_NoMatchIsIdempotent(t *testing.T) {
	repo := &pricingEntryRepo{
		existing: []ChannelModelPricing{{
			ID: 22, ChannelID: 1, Platform: PlatformAnthropic,
			Models: []string{"claude-opus"},
		}},
	}
	svc := NewChannelService(repo, nil, nil, nil)

	if err := svc.RemoveUpstreamPricingEntry(context.Background(), 1, PlatformAnthropic, "missing-model"); err != nil {
		t.Fatalf("RemoveUpstreamPricingEntry err: %v", err)
	}
	if len(repo.updated) != 0 || len(repo.deleted) != 0 {
		t.Fatalf("expected no-op on missing model, got updated=%v deleted=%v", repo.updated, repo.deleted)
	}
}
