// bundle_route_resolver.go 套餐路由解析器
// 根据用户请求的模型名称，在套餐计划包含的渠道组中解析出应使用的 Group。
// 解析策略：优先匹配模型级（glob 模式），回退到平台级匹配。

package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// BundleRouteResolver 套餐路由解析器，将模型请求映射到具体的渠道组
// BundleRouteResolver resolves which group should handle a model request
// for a user with an active bundle subscription.
type BundleRouteResolver struct {
	bundleSubRepo BundleSubscriptionRepository
	planRepo      BundlePlanRepository
	groupRepo     GroupRepository
}

// NewBundleRouteResolver 创建路由解析器实例
// NewBundleRouteResolver creates a new BundleRouteResolver.
func NewBundleRouteResolver(
	bundleSubRepo BundleSubscriptionRepository,
	planRepo BundlePlanRepository,
	groupRepo GroupRepository,
) *BundleRouteResolver {
	return &BundleRouteResolver{
		bundleSubRepo: bundleSubRepo,
		planRepo:      planRepo,
		groupRepo:     groupRepo,
	}
}

// ResolvedGroup 路由解析结果，包含目标渠道组ID、平台和额度信息
// ResolvedGroup holds the result of a bundle route resolution.
type ResolvedGroup struct {
	GroupID  int64
	Platform string
	Quota    BundlePlanGroupQuota
}

// ResolveGroup 解析模型请求应使用的渠道组：先尝试模型级 glob 匹配，再回退到平台级匹配
// ResolveGroup determines which group should handle a model request for a bundle subscriber.
// It first tries model-level matching (glob), then falls back to platform-level matching.
func (r *BundleRouteResolver) ResolveGroup(ctx context.Context, modelName string, bundleSubID int64) (*ResolvedGroup, error) {
	// Load bundle subscription.
	bundleSub, err := r.bundleSubRepo.GetByID(ctx, bundleSubID)
	if err != nil {
		return nil, fmt.Errorf("load bundle subscription: %w", err)
	}
	if bundleSub.Status != BundleStatusActive {
		return nil, ErrBundleExpired
	}

	// Load plan with group quotas.
	plan, err := r.planRepo.GetByID(ctx, bundleSub.PlanID)
	if err != nil {
		return nil, fmt.Errorf("load bundle plan: %w", err)
	}

	platform := resolveModelPlatform(modelName)

	// Phase 1: Try model-level matching (glob patterns).
	for _, gq := range plan.GroupQuotas {
		if gq.QuotaScope != QuotaScopeModel || gq.ModelPattern == "" {
			continue
		}
		if matchGlob(gq.ModelPattern, modelName) {
			return &ResolvedGroup{
				GroupID:  gq.GroupID,
				Platform: platform,
				Quota:    gq,
			}, nil
		}
	}

	// Phase 2: Fallback to platform-level matching.
	// We need to find which group matches the resolved platform.
	for _, gq := range plan.GroupQuotas {
		if gq.QuotaScope != QuotaScopePlatform {
			continue
		}
		group, err := r.groupRepo.GetByID(ctx, gq.GroupID)
		if err != nil {
			continue
		}
		if group.Platform == platform {
			return &ResolvedGroup{
				GroupID:  gq.GroupID,
				Platform: platform,
				Quota:    gq,
			}, nil
		}
	}

	return nil, ErrBundleModelNotIncluded
}

// resolveModelPlatform 根据模型名称前缀推断所属平台（openai/anthropic/gemini 等）
// resolveModelPlatform maps a model name prefix to a platform constant.
func resolveModelPlatform(modelName string) string {
	prefixes := map[string]string{
		"gpt-":     domain.PlatformOpenAI,
		"o1-":      domain.PlatformOpenAI,
		"o3-":      domain.PlatformOpenAI,
		"chatgpt-": domain.PlatformOpenAI,
		"dall-":    domain.PlatformOpenAI,
		"claude-":  domain.PlatformAnthropic,
		"gemini-":  domain.PlatformGemini,
		"deepseek-": domain.PlatformOpenAI, // compatible protocol
	}

	lower := strings.ToLower(modelName)
	for prefix, platform := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			return platform
		}
	}

	// Default to openai for unknown models (compatible protocol).
	return domain.PlatformOpenAI
}

// matchGlob 简易 glob 匹配，仅支持 '*' 通配符
// matchGlob performs simple glob matching with only '*' wildcard support.
func matchGlob(pattern, s string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}

	// Split pattern by '*' and verify each segment appears in order.
	segments := strings.Split(pattern, "*")
	if len(segments) == 1 {
		// No wildcard, exact match.
		return pattern == s
	}

	idx := 0
	for i, seg := range segments {
		if seg == "" {
			continue
		}
		pos := strings.Index(s[idx:], seg)
		if pos < 0 {
			return false
		}
		// First segment must match at the start.
		if i == 0 && pos != 0 {
			return false
		}
		idx += pos + len(seg)
	}

	// Last segment must match at the end if pattern doesn't end with '*'.
	if !strings.HasSuffix(pattern, "*") {
		return strings.HasSuffix(s, segments[len(segments)-1])
	}
	return true
}
