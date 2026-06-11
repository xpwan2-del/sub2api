// bundle_resolver.go 套餐路由解析中间件
// 在网关请求处理链中，为携带 bundle_subscription_id 的 API Key
// 解析出应使用的渠道组（Group），注入到 Gin 上下文中。
// 必须放在 APIKeyAuth 中间件之后、RequireGroupAssignment 之前。
// 同时执行套餐级的 RPM 和并发数限制检查。

package middleware

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// BundleRouteResolverMiddleware 套餐路由解析中间件
// BundleRouteResolverMiddleware resolves which group should handle a request
// for bundle API keys (keys with no group assignment but an active bundle subscription).
// It also enforces bundle-level RPM and concurrency limits.
type BundleRouteResolverMiddleware struct {
	resolver          *service.BundleRouteResolver
	rpmCache          service.BundleRPMCache
	concurrencyCache  service.BundleConcurrencyCache
}

// NewBundleRouteResolverMiddleware 创建套餐路由解析中间件
// NewBundleRouteResolverMiddleware creates a new BundleRouteResolverMiddleware.
func NewBundleRouteResolverMiddleware(
	resolver *service.BundleRouteResolver,
	rpmCache service.BundleRPMCache,
	concurrencyCache service.BundleConcurrencyCache,
) *BundleRouteResolverMiddleware {
	return &BundleRouteResolverMiddleware{
		resolver:         resolver,
		rpmCache:         rpmCache,
		concurrencyCache: concurrencyCache,
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
		if modelName == "" {
			slog.Warn("bundle resolver: bundle key request has no model field, skipping",
				"api_key_id", apiKey.ID,
				"bundle_sub_id", *apiKey.BundleSubscriptionID,
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
			)
			c.Next()
			return
		}

		slog.Info("bundle resolver: resolving group for bundle key",
			"api_key_id", apiKey.ID,
			"bundle_sub_id", *apiKey.BundleSubscriptionID,
			"model", modelName,
			"path", c.Request.URL.Path,
		)

		// Resolve group.
		resolved, err := m.resolver.ResolveGroup(c.Request.Context(), modelName, *apiKey.BundleSubscriptionID)
		if err != nil {
			status := http.StatusForbidden
			errType := "bundle_error"
			msg := err.Error()
			if err == service.ErrBundleExpired {
				errType = "bundle_expired"
			} else if err == service.ErrBundleModelNotIncluded {
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

		// Inject resolved group_id into context for downstream middleware/handlers.
		c.Set("bundle_resolved_group_id", resolved.GroupID)
		c.Next()
	}
}

// extractModelFromRequest 从请求 query 参数或 body 中提取模型名称
func extractModelFromRequest(c *gin.Context) string {
	if model := c.Query("model"); model != "" {
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
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		return ""
	}
	return req.Model
}
