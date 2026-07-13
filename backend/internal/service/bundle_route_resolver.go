// bundle_route_resolver.go 套餐路由解析器
// 根据用户请求的模型名称，在套餐计划包含的渠道组中解析出应使用的 Group。
// 解析策略：优先匹配模型级（glob 模式），回退到平台级匹配。

package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// BundleRouteResolver 套餐路由解析器，将模型请求映射到具体的渠道组
// BundleRouteResolver resolves which group should handle a model request
// for a user with an active bundle subscription.
type BundleRouteResolver struct {
	bundleSubRepo BundleSubscriptionRepository
	planRepo      BundlePlanRepository
	groupRepo     GroupRepository
	cache         GatewayCache
}

// NewBundleRouteResolver 创建路由解析器实例
// NewBundleRouteResolver creates a new BundleRouteResolver.
func NewBundleRouteResolver(
	bundleSubRepo BundleSubscriptionRepository,
	planRepo BundlePlanRepository,
	groupRepo GroupRepository,
	cache GatewayCache,
) *BundleRouteResolver {
	return &BundleRouteResolver{
		bundleSubRepo: bundleSubRepo,
		planRepo:      planRepo,
		groupRepo:     groupRepo,
		cache:         cache,
	}
}

// ResolvedGroup 路由解析结果，包含目标渠道组ID、平台、额度信息和套餐级限制
// ResolvedGroup holds the result of a bundle route resolution.
type ResolvedGroup struct {
	GroupID          int64
	Platform         string
	Quota            BundlePlanGroupQuota
	BundleSubID      int64  // 套餐订阅 ID，供中间件层进行 RPM/并发检查
	ConcurrencyLimit int    // 快照：套餐并发上限（0=不限）
	RPMLimit         int    // 快照：套餐 RPM 上限（0=不限）
	Group            *Group // 完整的渠道组对象，供中间件注入到 apiKey 以供下游 handler 使用
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
		slog.Warn("bundle route resolver: subscription not active",
			"bundle_sub_id", bundleSubID,
			"status", bundleSub.Status,
			"model", modelName,
		)
		return nil, ErrBundleExpired
	}
	// 实时过期兜底：后台 BundleExpiryService 约 1 分钟一轮翻转 status。若扫描器延迟或故障，
	// 仍按 ExpiresAt 拦截，避免过期套餐在下次扫描前继续放行。
	// Real-time expiry fallback: gate on ExpiresAt so an expired bundle cannot keep
	// serving if the background scanner lags or stalls. Skip when ExpiresAt is zero
	// (test-only); ActivateBundle always sets a future expiry in production.
	if !bundleSub.ExpiresAt.IsZero() && time.Now().After(bundleSub.ExpiresAt) {
		slog.Warn("bundle route resolver: subscription past expiry",
			"bundle_sub_id", bundleSubID,
			"expires_at", bundleSub.ExpiresAt,
			"model", modelName,
		)
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
		if matchAnyGlob(gq.ModelPattern, modelName) {
			resolved := makeResolvedGroup(gq, platform, bundleSubID, bundleSub)
			// 加载完整 Group 对象，供下游 handler 做路由决策
			group, groupErr := r.groupRepo.GetByIDLite(ctx, gq.GroupID)
			if groupErr != nil {
				return nil, fmt.Errorf("load resolved group %d: %w", gq.GroupID, groupErr)
			}
			resolved.Group = group
			return resolved, nil
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
			resolved := makeResolvedGroup(gq, platform, bundleSubID, bundleSub)
			resolved.Group = group
			return resolved, nil
		}
	}

	// Log the full quota configuration for debugging when no match is found.
	slog.Warn("bundle route resolver: model not included in bundle plan",
		"model", modelName,
		"resolved_platform", platform,
		"bundle_sub_id", bundleSubID,
		"plan_id", plan.ID,
		"plan_name", plan.Name,
	)
	for i, gq := range plan.GroupQuotas {
		slog.Warn("bundle route resolver: available quota entry",
			"index", i,
			"group_id", gq.GroupID,
			"quota_scope", gq.QuotaScope,
			"model_pattern", gq.ModelPattern,
			"group_name", gq.GroupName,
		)
	}

	return nil, ErrBundleModelNotIncluded
}

// ResolveGroupByVideoTask 为 bundle key 的视频任务查询请求（GET /v1/videos/:id 与 /content）
// 解析目标渠道组。这类请求按 OpenAI 规范不带 model，无法走 ResolveGroup 的 model 匹配；
// 改为用 taskID 在订阅 plan 覆盖的渠道组里反查 video task 绑定，命中即恢复创建时选定的 group。
//
// 复用既有 (groupID, taskID) 绑定存储，无需为 bundle 维护额外索引。plan 覆盖的 group 数
// 通常很少（个位数），每次 GET 仅做相应次数的 cache 查询，可接受。
// 任意一步失败（订阅失效、task 不属于本订阅、绑定已过期）返回 ErrBundleModelNotIncluded，
// 由调用方决定回退行为。
func (r *BundleRouteResolver) ResolveGroupByVideoTask(ctx context.Context, bundleSubID int64, taskID string) (*ResolvedGroup, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || r.cache == nil {
		return nil, ErrBundleModelNotIncluded
	}
	// 与 ResolveGroup 一致的订阅有效性校验。
	bundleSub, err := r.bundleSubRepo.GetByID(ctx, bundleSubID)
	if err != nil {
		return nil, fmt.Errorf("load bundle subscription: %w", err)
	}
	if bundleSub.Status != BundleStatusActive {
		return nil, ErrBundleExpired
	}
	if !bundleSub.ExpiresAt.IsZero() && time.Now().After(bundleSub.ExpiresAt) {
		return nil, ErrBundleExpired
	}
	plan, err := r.planRepo.GetByID(ctx, bundleSub.PlanID)
	if err != nil {
		return nil, fmt.Errorf("load bundle plan: %w", err)
	}
	// 遍历 plan 覆盖的 group（去重），用 taskID 反查 video 绑定；命中即返回。
	seen := make(map[int64]struct{})
	for _, gq := range plan.GroupQuotas {
		if _, ok := seen[gq.GroupID]; ok {
			continue
		}
		seen[gq.GroupID] = struct{}{}
		binding, gErr := r.cache.GetVideoTaskBinding(ctx, gq.GroupID, taskID)
		// VIDEO_DIAG: 反查遍历的每个 plan group 及其命中情况。对照 POST BindVideoTask 写入的
		// bind_group_id 即可判定维度是否一致（一致则此处必然 found=true 命中一次）。
		slog.Info("VIDEO_DIAG: ResolveGroupByVideoTask probe",
			"task_id", taskID,
			"probed_group_id", gq.GroupID,
			"found", gErr == nil && strings.TrimSpace(binding.Model) != "",
		)
		if gErr != nil || strings.TrimSpace(binding.Model) == "" {
			continue
		}
		// scope 到订阅：仅接受本订阅创建的 task，防止跨订阅越权查询/取内容（IDOR）。
		// 多个订阅共享同一 group（同一上游账号池）时，(groupID, taskID) 维度的绑定不带
		// 归属即可被任意订阅命中，故必须用 BundleSubID 收紧到归属订阅。
		if binding.BundleSubID == nil || *binding.BundleSubID != bundleSubID {
			continue
		}
		group, groupErr := r.groupRepo.GetByIDLite(ctx, gq.GroupID)
		if groupErr != nil || group == nil {
			continue
		}
		return &ResolvedGroup{
			GroupID:          gq.GroupID,
			Platform:         group.Platform,
			Quota:            gq,
			BundleSubID:      bundleSubID,
			ConcurrencyLimit: bundleSub.ConcurrencyLimit,
			RPMLimit:         bundleSub.RPMLimit,
			Group:            group,
		}, nil
	}
	// VIDEO_DIAG: 反查全部 miss。若此处触发，对照 POST 的 bind_group_id 检查：
	// bind_group_id 不在 probed_group_id 列表里 → POST 写入维度错误（apiKey.GroupID 未注入）。
	slog.Warn("VIDEO_DIAG: ResolveGroupByVideoTask MISS",
		"task_id", taskID,
		"bundle_sub_id", bundleSubID,
		"probed_group_count", len(seen),
	)
	return nil, ErrBundleModelNotIncluded
}

// makeResolvedGroup builds a ResolvedGroup from quota and bundle subscription snapshot limits.
func makeResolvedGroup(gq BundlePlanGroupQuota, platform string, bundleSubID int64, sub *BundleSubscription) *ResolvedGroup {
	return &ResolvedGroup{
		GroupID:          gq.GroupID,
		Platform:         platform,
		Quota:            gq,
		BundleSubID:      bundleSubID,
		ConcurrencyLimit: sub.ConcurrencyLimit,
		RPMLimit:         sub.RPMLimit,
	}
}

// resolveModelPlatform 根据模型名称前缀推断所属平台（openai/anthropic/gemini 等）
// resolveModelPlatform maps a model name prefix to a platform constant.
func resolveModelPlatform(modelName string) string {
	prefixes := map[string]string{
		"gpt-":            domain.PlatformOpenAI,
		"o1-":             domain.PlatformOpenAI,
		"o3-":             domain.PlatformOpenAI,
		"o4-":             domain.PlatformOpenAI,
		"chatgpt-":        domain.PlatformOpenAI,
		"dall-":           domain.PlatformOpenAI,
		"gpt-image-":      domain.PlatformOpenAI,
		"sora-":           domain.PlatformOpenAI,
		"text-embedding-": domain.PlatformOpenAI,
		"embedding-":      domain.PlatformOpenAI,
		"claude-":         domain.PlatformAnthropic,
		"gemini-":         domain.PlatformGemini,
		"veo-":            domain.PlatformGemini,
		"imagen-":         domain.PlatformGemini,
	}

	lower := strings.ToLower(modelName)
	for prefix, platform := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			return platform
		}
	}

	// Default to anthropic for unknown models.
	return domain.PlatformAnthropic
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

// matchAnyGlob 按逗号拆分 pattern 字段为多个 glob 子模式,任一命中即返回 true。
// 空字段返回 false(语义「未配置」,由 ResolveGroup 的 ModelPattern=="" guard 拦截,
// 该路径下本函数不会被空串调用);纯逗号/全空格拆分后无有效段亦返回 false;
// 单 pattern 等价于旧 matchGlob。与旧 matchGlob("")==true 不同,本函数空串返回 false,
// 这是刻意设计以避免空配置意外匹配全部模型。resolve 路径靠外层 guard 保证兼容。
func matchAnyGlob(patternField, s string) bool {
	if patternField == "" {
		return false
	}
	for _, p := range strings.Split(patternField, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if matchGlob(p, s) {
			return true
		}
	}
	return false
}
