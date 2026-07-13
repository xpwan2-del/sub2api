// bundle_resolver.go 套餐路由解析中间件
// 在网关请求处理链中，为携带 bundle_subscription_id 的 API Key
// 解析出应使用的渠道组（Group），注入到 Gin 上下文中。
// 必须放在 APIKeyAuth 中间件之后、RequireGroupAssignment 之前。
// 同时执行套餐级的 RPM 和并发数限制检查。

package middleware

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// BundleRouteResolverMiddleware 套餐路由解析中间件
// BundleRouteResolverMiddleware resolves which group should handle a request
// for bundle API keys (keys with no group assignment but an active bundle subscription).
// It also enforces bundle-level RPM and concurrency limits.
// After resolving the group, it loads the bridged UserSubscription and injects
// it into the gin context so downstream billing tracks bundle usage correctly.
type BundleRouteResolverMiddleware struct {
	resolver         *service.BundleRouteResolver
	rpmCache         service.BundleRPMCache
	concurrencyCache service.BundleConcurrencyCache
	subscriptionSvc  *service.SubscriptionService
	usageSvc         *service.BundleUsageService
}

// NewBundleRouteResolverMiddleware 创建套餐路由解析中间件
// NewBundleRouteResolverMiddleware creates a new BundleRouteResolverMiddleware.
func NewBundleRouteResolverMiddleware(
	resolver *service.BundleRouteResolver,
	rpmCache service.BundleRPMCache,
	concurrencyCache service.BundleConcurrencyCache,
	subscriptionSvc *service.SubscriptionService,
	usageSvc *service.BundleUsageService,
) *BundleRouteResolverMiddleware {
	return &BundleRouteResolverMiddleware{
		resolver:         resolver,
		rpmCache:         rpmCache,
		concurrencyCache: concurrencyCache,
		subscriptionSvc:  subscriptionSvc,
		usageSvc:         usageSvc,
	}
}

// BundleResolver 返回 Gin 中间件，为套餐 Key 解析目标渠道组
// BundleResolver returns a gin middleware that resolves the group for bundle keys.
// Must be placed after APIKeyAuth middleware and before RequireGroupAssignment.
func (m *BundleRouteResolverMiddleware) BundleResolver() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey, ok := GetAPIKeyFromContext(c)
		if !ok {
			c.Next()
			return
		}
		// Only handle unassigned keys (GroupID nil) — bundle keys have no fixed group.
		if apiKey.GroupID != nil {
			c.Next()
			return
		}
		// Bundle keys carry BundleSubscriptionID from the database.
		if apiKey.BundleSubscriptionID == nil || *apiKey.BundleSubscriptionID <= 0 {
			slog.Debug("bundle resolver: skipping key without BundleSubscriptionID",
				"api_key_id", apiKey.ID,
			)
			c.Next()
			return
		}

		// Extract model name from request body.
		modelName := extractModelFromRequest(c)

		// Resolve group: 有 model 走标准 model 路由；GET /v1/videos/:id 与 /content 按
		// OpenAI 规范不带 model，改用 taskID 反查创建时写入的 group 绑定。
		resolved, err := m.resolveBundleGroup(c, modelName, *apiKey.BundleSubscriptionID)
		if err != nil {
			status := http.StatusForbidden
			errType := "bundle_error"
			msg := err.Error()
			switch err {
			case service.ErrBundleExpired:
				errType = "bundle_expired"
			case service.ErrBundleModelNotIncluded:
				status = http.StatusBadRequest
				errType = "bundle_model_not_included"
			}
			c.JSON(status, gin.H{
				"error": gin.H{
					"type":    errType,
					"message": msg,
				},
			})
			c.Abort()
			return
		}
		if resolved == nil {
			// 无 model 且 task 反查未命中（非 videos 请求，或 task 不属于本订阅/已过期）：
			// 跳过 group 注入，交由下游路由门控处理。
			slog.Warn("bundle resolver: bundle key request has no model field and no video task binding, skipping",
				"api_key_id", apiKey.ID,
				"bundle_sub_id", *apiKey.BundleSubscriptionID,
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
			)
			c.Next()
			return
		}

		slog.Info("bundle resolver: resolved group for bundle key",
			"api_key_id", apiKey.ID,
			"bundle_sub_id", *apiKey.BundleSubscriptionID,
			"model", modelName,
			"group_id", resolved.GroupID,
			"path", c.Request.URL.Path,
		)

		// 先加载桥接 UserSubscription：既供下游计费进入订阅扣减路径，又用其激活时快照的
		// limit 构造注入 ctx 的 quota —— 使 pre-flight 额度检查、post 计费累加、用量进度展示
		// 三处 limit 同源（激活快照），admin 后续修改 plan 额度不影响已购用户。
		var bridgedSub *service.UserSubscription
		if m.subscriptionSvc != nil && apiKey.User != nil && resolved.Group != nil && resolved.Group.IsSubscriptionType() {
			sub, subErr := m.subscriptionSvc.GetActiveSubscription(c.Request.Context(), apiKey.User.ID, resolved.GroupID)
			if subErr != nil {
				slog.Error("bundle resolver: failed to load bridged subscription",
					"user_id", apiKey.User.ID,
					"group_id", resolved.GroupID,
					"bundle_sub_id", resolved.BundleSubID,
					"error", subErr,
				)
				// Do not abort — the request can still proceed via balance billing.
			} else {
				bridgedSub = sub
				c.Set(string(ContextKeySubscription), sub)
			}
		}

		// 构造注入 ctx 的 quota：ModelPattern 取路由 glob 命中的正确 pattern（避免
		// CheckQuotaEligibility 的 resolveMatchingQuota 按 GroupID 取首条导致取错），
		// limit 优先取桥接 userSub 的激活快照；无快照时回退 plan 当前值。
		// detachedBillingContext 用 WithoutCancel 保留 values，计费 worker 可读。
		resolvedQuota := resolved.Quota
		if bridgedSub != nil {
			resolvedQuota.DailyLimitUSD = bridgedSub.DailyLimitUSD
			resolvedQuota.WeeklyLimitUSD = bridgedSub.WeeklyLimitUSD
			resolvedQuota.MonthlyLimitUSD = bridgedSub.MonthlyLimitUSD
			resolvedQuota.DailyImageLimitCount = bridgedSub.DailyImageLimitCount
			resolvedQuota.WeeklyImageLimitCount = bridgedSub.WeeklyImageLimitCount
			resolvedQuota.MonthlyImageLimitCount = bridgedSub.MonthlyImageLimitCount
			resolvedQuota.DailyVideoLimitCount = bridgedSub.DailyVideoLimitCount
			resolvedQuota.WeeklyVideoLimitCount = bridgedSub.WeeklyVideoLimitCount
			resolvedQuota.MonthlyVideoLimitCount = bridgedSub.MonthlyVideoLimitCount
		}
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.BundleResolvedQuota, &resolvedQuota))

		// --- Bundle-level concurrency check ---
		// Fail-closed: concurrency limits protect backend resources from overload.
		// If Redis is unavailable, reject the request rather than risk unbounded
		// concurrency on upstream AI provider accounts.
		if resolved.ConcurrencyLimit > 0 {
			count, incErr := m.concurrencyCache.Increment(c.Request.Context(), resolved.BundleSubID)
			if incErr != nil {
				slog.Error("bundle concurrency check failed, rejecting request",
					"bundle_sub_id", resolved.BundleSubID, "error", incErr)
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"error": gin.H{
						"type":    "bundle_concurrency_unavailable",
						"message": "并发限制检查暂不可用，请稍后重试",
					},
				})
				c.Abort()
				return
			}
			if count > int64(resolved.ConcurrencyLimit) {
				// Exceeded: decrement and reject.
				_, _ = m.concurrencyCache.Decrement(c.Request.Context(), resolved.BundleSubID)
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error": gin.H{
						"type":    "bundle_concurrency_exceeded",
						"message": "当前并发请求数已达套餐上限",
					},
				})
				c.Abort()
				return
			}
			// Ensure decrement on request completion.
			defer func() {
				_, decErr := m.concurrencyCache.Decrement(c.Request.Context(), resolved.BundleSubID)
				if decErr != nil {
					slog.Error("bundle concurrency decrement failed", "bundle_sub_id", resolved.BundleSubID, "error", decErr)
				}
			}()
		}

		// --- Bundle-level RPM check ---
		// Fail-open by design: consistent with the existing RPM pattern in the codebase
		// ("失败开放：GetRPM 错误时允许调度"). RPM is a soft limit for rate smoothing,
		// not a resource-protection boundary. Availability is preferred over strictness.
		if resolved.RPMLimit > 0 {
			rpmCount, rpmErr := m.rpmCache.IncrementBundleRPM(c.Request.Context(), resolved.BundleSubID)
			if rpmErr != nil {
				slog.Warn("bundle rpm check failed, allowing request (fail-open)",
					"bundle_sub_id", resolved.BundleSubID, "error", rpmErr)
			} else if rpmCount > resolved.RPMLimit {
				// 被拒请求归还其占用的 RPM 槽位，避免持续打满时压榨合法请求配额。
				if decErr := m.rpmCache.DecrementBundleRPM(c.Request.Context(), resolved.BundleSubID); decErr != nil {
					slog.Warn("bundle rpm decrement failed on reject", "bundle_sub_id", resolved.BundleSubID, "error", decErr)
				}
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error": gin.H{
						"type":    "bundle_rpm_exceeded",
						"message": "请求频率已达套餐上限",
					},
				})
				c.Abort()
				return
			}
		}

		// --- Bundle-level quota precheck (USD + count) ---
		// Fail-open by design (consistent with RPM above): quota is a soft
		// read-only check of "used vs limit"; transient usageSvc errors must
		// not block requests. Concurrent over-issuance is acceptable here
		// (see spec 10.1) — strictness is enforced post-billing.
		//
		// 跳过只读视频任务查询（无 model 参数、走 task binding 反查的 GET /v1/videos/:id 与
		// /content）：视频任务在 POST 创建时已通过配额检查（合法创建），轮询进度/取内容是
		// 只读操作、不消耗配额。带 ?model= 的 GET 走 model 路由（ResolveGroup），语义上不属
		// "查询已创建任务"，不享受豁免。并发/RPM 检查已在上游执行，频率仍受控。
		if m.usageSvc != nil && !isReadOnlyVideoTaskQuery(c.Request.Method, c.Request.URL.Path, modelName) {
			// 按请求路径粗略推断媒体维度,决定 pre-flight 校验哪条 count 轨道。
			// fail-open:推断不精确也安全(严格扣减在 post-billing)。
			path := c.Request.URL.Path
			var modality service.UsageModality
			switch {
			case strings.Contains(path, "/videos"):
				modality = service.ModalityVideo
			case strings.Contains(path, "/images"):
				modality = service.ModalityImage
			default:
				modality = service.ModalityAny
			}
			elig, qErr := m.usageSvc.CheckQuotaEligibility(c.Request.Context(), resolved.BundleSubID, resolved.GroupID, modality)
			if qErr != nil {
				slog.Warn("bundle quota check failed, allowing (fail-open)",
					"bundle_sub_id", resolved.BundleSubID, "error", qErr)
			} else if elig != nil && !elig.Eligible {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error": gin.H{
						"type":    "BUNDLE_GROUP_QUOTA_EXCEEDED",
						"message": "套餐额度已达上限",
					},
				})
				c.Abort()
				return
			}
		}

		// Inject resolved group_id into context for downstream middleware/handlers.
		c.Set("bundle_resolved_group_id", resolved.GroupID)

		// VIDEO_DIAG: 诊断 bundle 解析结果。resolved.Group 是否为 nil 决定 apiKey.GroupID 是否被注入
		// (下方 if resolved.Group != nil)，进而决定 POST BindVideoTask 写入 binding 的 group 维度。
		// 若 resolved_group_nil=true 而 ctx 已 set bundle_resolved_group_id，说明 apiKey.GroupID 未被
		// 注入（合并前 handler 靠 getGroupPlatform 不受影响；合并后靠 bundleRouteResolved 反查会 miss）。
		slog.Info("VIDEO_DIAG: bundle resolved",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"model", modelName,
			"resolved_group_id", resolved.GroupID,
			"resolved_group_nil", resolved.Group == nil,
			"bundle_sub_id", *apiKey.BundleSubscriptionID,
			"api_key_id", apiKey.ID,
		)

		// 将解析出的 Group 注入到 apiKey 对象，使下游 handler 自动获得正确的 group 信息。
		// apiKey 是指针，修改其字段对后续所有中间件和 handler 可见。
		if resolved.Group != nil {
			groupID := resolved.GroupID
			apiKey.GroupID = &groupID
			apiKey.Group = resolved.Group
			setGroupContext(c, resolved.Group)
		}

		// (桥接 UserSubscription 已在上方提前加载并注入 ctx，供额度检查与下游计费复用。)

		c.Next()
	}
}

// extractModelFromRequest 从请求 query 参数或 body 中提取模型名称
func extractModelFromRequest(c *gin.Context) string {
	if model := c.Query("model"); model != "" {
		return model
	}
	// Gemini 原生等入口的 model 在 URL path（/models/{model}:action）。
	if model := extractModelFromPath(c.Request.URL.Path); model != "" {
		return model
	}
	if c.Request.Body == nil {
		return ""
	}
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil || len(bodyBytes) == 0 {
		return ""
	}
	// Restore the body so downstream handlers can read it.
	c.Request.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))

	// multipart/form-data（如 /v1/videos、/v1/images/edits）不能用 JSON 解析：
	// bundle key 若取不到 model 会跳过 group 注入，导致下游平台门控误判为不支持的平台。
	contentType := c.GetHeader("Content-Type")
	if strings.HasPrefix(strings.ToLower(contentType), "multipart/form-data") {
		return httputil.ExtractModelFromMultipart(bodyBytes, contentType)
	}

	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		return ""
	}
	return req.Model
}

// extractModelFromPath 从 URL path 提取模型名，支持 Gemini 风格 /models/{model}:action
// 与 /models/{model}（取冒号 / 斜杠前的片段）。找不到返回空。
func extractModelFromPath(path string) string {
	const prefix = "/models/"
	idx := strings.Index(path, prefix)
	if idx < 0 {
		return ""
	}
	rest := path[idx+len(prefix):]
	if colon := strings.Index(rest, ":"); colon >= 0 {
		rest = rest[:colon]
	}
	if slash := strings.Index(rest, "/"); slash >= 0 {
		rest = rest[:slash]
	}
	return rest
}

// resolveBundleGroup 解析 bundle key 的目标渠道组：有 model 走标准 model 路由；
// 无 model 时（如 GET /v1/videos/:id）尝试用 taskID 反查。返回 (nil, nil) 表示两种路径
// 都无法解析（调用方应跳过 group 注入），返回非 nil error 表示订阅/计划级失败。
func (m *BundleRouteResolverMiddleware) resolveBundleGroup(c *gin.Context, modelName string, bundleSubID int64) (*service.ResolvedGroup, error) {
	if strings.TrimSpace(modelName) != "" {
		return m.resolver.ResolveGroup(c.Request.Context(), modelName, bundleSubID)
	}
	if resolved := m.tryResolveBundleVideoTask(c, bundleSubID); resolved != nil {
		return resolved, nil
	}
	return nil, nil
}

// tryResolveBundleVideoTask 仅对 GET /v1/videos/:id(/content) 用 taskID 反查创建时绑定的 group。
// 非 GET、非 videos 路径、无 taskID 或反查未命中均返回 nil。
func (m *BundleRouteResolverMiddleware) tryResolveBundleVideoTask(c *gin.Context, bundleSubID int64) *service.ResolvedGroup {
	if c.Request.Method != http.MethodGet {
		return nil
	}
	taskID := extractVideoTaskIDFromPath(c.Request.URL.Path)
	if taskID == "" {
		return nil
	}
	resolved, err := m.resolver.ResolveGroupByVideoTask(c.Request.Context(), bundleSubID, taskID)
	if err != nil || resolved == nil {
		return nil
	}
	return resolved
}

// extractVideoTaskIDFromPath 从 /v1/videos/:id 或 /v1/videos/:id/content 提取 task id 段。
// 非 videos 路径或无 id 段返回空串。
func extractVideoTaskIDFromPath(path string) string {
	const marker = "/videos/"
	idx := strings.Index(path, marker)
	if idx < 0 {
		return ""
	}
	rest := path[idx+len(marker):]
	if cut, ok := strings.CutSuffix(rest, "/content"); ok {
		rest = cut
	}
	rest = strings.TrimRight(rest, "/")
	if slash := strings.Index(rest, "/"); slash >= 0 {
		rest = rest[:slash]
	}
	return strings.TrimSpace(rest)
}

// isReadOnlyVideoTaskQuery reports whether the request is a read-only video task query
// that resolved its group via task-binding lookup (the no-model path in resolveBundleGroup):
// GET /v1/videos/:id or /v1/videos/:id/content, WITHOUT a model parameter.
//
// Such requests do not consume quota — the task already passed the quota check when created
// via POST — so they skip the pre-flight quota pre-check. Otherwise, once a bundle hits its
// limit, users could no longer poll progress or fetch content of tasks they legitimately
// created. A GET that carries ?model= routes by model (ResolveGroup) and stays under the
// standard quota check; only the task-binding lookup path qualifies for the exemption.
// Concurrency/RPM limits above still apply, so query frequency remains controlled.
func isReadOnlyVideoTaskQuery(method, path, modelName string) bool {
	if method != http.MethodGet {
		return false
	}
	// 带 model 参数 → 走 model 路由（resolveBundleGroup 优先 ResolveGroup），不属于纯只读
	// 查询，不豁免。仅无 model、走 task binding 反查的请求才豁免。
	if strings.TrimSpace(modelName) != "" {
		return false
	}
	return videoTaskQueryID(path) != ""
}

// videoTaskQueryID 精确提取只读视频查询的 taskID：仅接受 ".../videos/{id}" 与
// ".../videos/{id}/content" 两种形态，拒绝额外 segment（如 ".../videos/{id}/extra"），
// 避免未来 wildcard 路由复用时扩大配额豁免面。与 extractVideoTaskIDFromPath 的宽泛截断
// （供 task 反查用，路由层已过滤不可达路径）不同，此函数用于配额豁免门控，必须严格。
func videoTaskQueryID(path string) string {
	const marker = "/videos/"
	idx := strings.Index(path, marker)
	if idx < 0 {
		return ""
	}
	rest := strings.Trim(path[idx+len(marker):], "/")
	if rest == "" {
		return ""
	}
	segments := strings.Split(rest, "/")
	switch len(segments) {
	case 1:
		return segments[0]
	case 2:
		if segments[1] == "content" {
			return segments[0]
		}
		return ""
	default:
		return ""
	}
}
