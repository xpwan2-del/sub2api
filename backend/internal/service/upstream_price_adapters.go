package service

import (
	"context"
	"strings"
)

// This file bridges the upstream-price subsystem (Tasks 9/10) with the
// existing ChannelService / GroupService / OpsService.
//
// All five adapter types below live in the service package so they can reach
// the private fields (repo, groupRepo) and private method (invalidateCache)
// of ChannelService, satisfying the interfaces declared by the apply/sync
// services without leaking repository dependencies upward to handlers.

// ===== ChannelPricingWriter + groupChannelResolver adapter =====

// channelPricingWriterAdapter wraps ChannelService to implement both
// ChannelPricingWriter (ReplaceModelPricingForModel + InvalidateChannelCache)
// and the optional groupChannelResolver (GetChannelIDForGroup) declared inside
// UpstreamPriceApplyService.resolveChannelForGroup.
//
// ChannelService itself only exposes the private invalidateCache() and the
// repository already has ReplaceModelPricingForModel / GetChannelIDByGroupID,
// so this adapter delegates to ChannelService.repo / .groupRepo and exposes a
// public InvalidateChannelCache alias.
type channelPricingWriterAdapter struct {
	channel *ChannelService
}

// NewChannelPricingWriterAdapter wires ChannelService into a ChannelPricingWriter.
func NewChannelPricingWriterAdapter(channel *ChannelService) ChannelPricingWriter {
	return channelPricingWriterAdapter{channel: channel}
}

// NewApplyTargetReaderAdapter wires ChannelService.repo into an ApplyTargetReader.
// repo 为 nil 时返回 nil（ApplyService 会跳过目标查询，前端退化为输入框）。
func NewApplyTargetReaderAdapter(channel *ChannelService) ApplyTargetReader {
	if channel == nil || channel.repo == nil {
		return nil
	}
	return applyTargetReaderAdapter{repo: channel.repo}
}

// applyTargetReaderAdapter 把 ChannelRepository 的两个查询方法适配为 ApplyTargetReader。
type applyTargetReaderAdapter struct {
	repo ChannelRepository
}

func (a applyTargetReaderAdapter) ListChannelsByModel(ctx context.Context, modelName string) ([]ChannelApplyTarget, error) {
	return a.repo.ListChannelsByModel(ctx, modelName)
}

func (a applyTargetReaderAdapter) ListGroupsByChannels(ctx context.Context, channelIDs []int64) ([]GroupApplyTarget, error) {
	return a.repo.ListGroupsByChannels(ctx, channelIDs)
}

func (a applyTargetReaderAdapter) CountDistinctModelsByGroups(ctx context.Context, groupIDs []int64) (map[int64]int, error) {
	return a.repo.CountDistinctModelsByGroups(ctx, groupIDs)
}

// ReplaceModelPricingForModel delegates to ChannelRepository.
func (a channelPricingWriterAdapter) ReplaceModelPricingForModel(ctx context.Context, channelID int64, modelName string, inputPrice, outputPrice float64) error {
	if a.channel == nil || a.channel.repo == nil {
		return nil
	}
	return a.channel.repo.ReplaceModelPricingForModel(ctx, channelID, modelName, inputPrice, outputPrice)
}

// InvalidateChannelCache exposes ChannelService.invalidateCache (private) under
// the public name required by ChannelPricingWriter.
func (a channelPricingWriterAdapter) InvalidateChannelCache() {
	if a.channel != nil {
		a.channel.invalidateCache()
	}
}

// GetChannelIDForGroup bridges the lock_price resolver. The existing
// ChannelRepository names it GetChannelIDByGroupID; this adapter renames it to
// what UpstreamPriceApplyService.resolveChannelForGroup looks up via the
// optional groupChannelResolver interface (declared inline in apply_service.go).
func (a channelPricingWriterAdapter) GetChannelIDForGroup(ctx context.Context, groupID int64) (int64, error) {
	if a.channel == nil || a.channel.repo == nil {
		return 0, nil
	}
	return a.channel.repo.GetChannelIDByGroupID(ctx, groupID)
}

// GetCurrentPriceForModel reads the current per-token price for a model on a
// channel, used to snapshot apply-prev values (coverage protection + revert).
// Delegates to ChannelRepository; satisfies the optional
// channelPricingSnapshotReader interface via type assertion.
func (a channelPricingWriterAdapter) GetCurrentPriceForModel(ctx context.Context, channelID int64, modelName string) (float64, float64, error) {
	if a.channel == nil || a.channel.repo == nil {
		return 0, 0, nil
	}
	return a.channel.repo.GetCurrentPriceForModel(ctx, channelID, modelName)
}

// ===== GroupRateWriter adapter =====

// groupRateWriterAdapter wraps GroupRepository.UpdateRateMultiplier to satisfy
// GroupRateWriter (used by ApplyService for lock_price mode).
type groupRateWriterAdapter struct {
	groupRepo GroupRepository
}

// NewGroupRateWriterAdapter wires GroupRepository into a GroupRateWriter.
func NewGroupRateWriterAdapter(groupRepo GroupRepository) GroupRateWriter {
	return groupRateWriterAdapter{groupRepo: groupRepo}
}

// UpdateRateMultiplier delegates to GroupRepository.
func (a groupRateWriterAdapter) UpdateRateMultiplier(ctx context.Context, groupID int64, multiplier float64) error {
	return a.groupRepo.UpdateRateMultiplier(ctx, groupID, multiplier)
}

// GetRateMultiplierByGroupID reads the current rate_multiplier for a group,
// used to snapshot apply-prev values (coverage protection + revert). Satisfies
// the optional groupRateSnapshotReader interface via type assertion.
//
// Uses GetByIDLite (not GetByID) because only RateMultiplier is read — the
// account-count aggregate that GetByID performs is wasted work here.
func (a groupRateWriterAdapter) GetRateMultiplierByGroupID(ctx context.Context, groupID int64) (float64, error) {
	if a.groupRepo == nil {
		return 0, nil
	}
	g, err := a.groupRepo.GetByIDLite(ctx, groupID)
	if err != nil {
		return 0, err
	}
	return g.RateMultiplier, nil
}

// ===== GroupRateReader adapter =====

// groupRateReaderAdapter implements GroupRateReader (sync suggestion value).
//
// There is no direct "rate_multiplier for a model" lookup, so this resolves a
// representative multiplier by finding the first group whose platform matches
// the model family and returning its rate_multiplier. Returning 1.0 when no
// group matches is the documented fail-open behavior of the sync service.
type groupRateReaderAdapter struct {
	groupRepo GroupRepository
}

// NewGroupRateReaderAdapter wires GroupRepository into a GroupRateReader.
func NewGroupRateReaderAdapter(groupRepo GroupRepository) GroupRateReader {
	return groupRateReaderAdapter{groupRepo: groupRepo}
}

// RateMultiplierForModel returns a representative group rate_multiplier for the
// given model. It picks the first active group whose platform matches the
// model's inferred platform; falls back to 1.0 when none is found.
func (a groupRateReaderAdapter) RateMultiplierForModel(ctx context.Context, localModelName string) (float64, error) {
	if a.groupRepo == nil {
		return 1.0, nil
	}
	groups, err := a.groupRepo.ListActive(ctx)
	if err != nil {
		return 1.0, nil
	}
	platform := inferPlatformFromModelName(localModelName)
	for i := range groups {
		if platform == "" || strings.EqualFold(groups[i].Platform, platform) {
			if groups[i].RateMultiplier > 0 {
				return groups[i].RateMultiplier, nil
			}
		}
	}
	return 1.0, nil
}

// inferPlatformFromModelName best-effort maps a model name to a platform.
// Used only to pick a representative group for suggestion-value calculation.
func inferPlatformFromModelName(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(m, "claude"), strings.Contains(m, "anthropic"):
		return PlatformAnthropic
	case strings.Contains(m, "gpt"), strings.Contains(m, "o1"), strings.Contains(m, "o3"), strings.Contains(m, "openai"):
		return PlatformOpenAI
	case strings.Contains(m, "gemini"):
		return PlatformGemini
	default:
		return ""
	}
}

// ===== AlertRecipientReader adapter =====

// alertRecipientReaderAdapter reads upstream-price alert recipients from the
// ops email-notification config (same source OpsAlertEvaluator uses), exposing
// only the recipient list to the sync service.
type alertRecipientReaderAdapter struct {
	opsService *OpsService
}

// NewAlertRecipientReaderAdapter wires OpsService into an AlertRecipientReader.
// opsService may be nil; ListAlertRecipients then returns an empty list so
// email sending is skipped while in-site admin notifications still fire.
func NewAlertRecipientReaderAdapter(opsService *OpsService) AlertRecipientReader {
	return alertRecipientReaderAdapter{opsService: opsService}
}

// ListAlertRecipients returns the ops alert recipient email list. Empty list on
// any error or missing config (sync service tolerates this gracefully).
func (a alertRecipientReaderAdapter) ListAlertRecipients(ctx context.Context) ([]string, error) {
	if a.opsService == nil {
		return []string{}, nil
	}
	cfg, err := a.opsService.GetEmailNotificationConfig(ctx)
	if err != nil || cfg == nil {
		return []string{}, nil
	}
	if len(cfg.Alert.Recipients) == 0 {
		return []string{}, nil
	}
	out := make([]string, 0, len(cfg.Alert.Recipients))
	for _, r := range cfg.Alert.Recipients {
		if t := strings.TrimSpace(r); t != "" {
			out = append(out, t)
		}
	}
	return out, nil
}

// Compile-time assertions that adapters satisfy their interfaces.
var (
	_ ChannelPricingWriter          = channelPricingWriterAdapter{}
	_ GroupRateWriter               = groupRateWriterAdapter{}
	_ GroupRateReader               = groupRateReaderAdapter{}
	_ AlertRecipientReader          = alertRecipientReaderAdapter{}
	_ ApplyTargetReader             = applyTargetReaderAdapter{}
	_ groupChannelResolver          = channelPricingWriterAdapter{}
	_ channelPricingSnapshotReader  = channelPricingWriterAdapter{}
	_ groupRateSnapshotReader       = groupRateWriterAdapter{}
)

// groupChannelResolver is mirrored from apply_service.go for the compile-time
// assertion above. The real interface is declared inline inside
// UpstreamPriceApplyService.resolveChannelForGroup; this local redeclaration
// keeps the assertion self-contained without exporting the apply-service helper.
type groupChannelResolver interface {
	GetChannelIDForGroup(ctx context.Context, groupID int64) (int64, error)
}
