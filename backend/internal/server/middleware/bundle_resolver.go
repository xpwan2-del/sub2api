package middleware

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// BundleRouteResolverMiddleware resolves which group should handle a request
// for bundle API keys (keys with no group assignment but an active bundle subscription).
type BundleRouteResolverMiddleware struct {
	resolver *service.BundleRouteResolver
}

// NewBundleRouteResolverMiddleware creates a new BundleRouteResolverMiddleware.
func NewBundleRouteResolverMiddleware(resolver *service.BundleRouteResolver) *BundleRouteResolverMiddleware {
	return &BundleRouteResolverMiddleware{resolver: resolver}
}

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
			c.Next()
			return
		}

		// Extract model name from request body.
		modelName := extractModelFromRequest(c)
		if modelName == "" {
			c.Next()
			return
		}

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

		// Inject resolved group_id into context for downstream middleware/handlers.
		c.Set("bundle_resolved_group_id", resolved.GroupID)
		c.Next()
	}
}

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
