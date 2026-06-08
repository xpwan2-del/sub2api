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

		// Check if the user has an active bundle subscription ID stored in context.
		// This is set by the auth middleware when the API key has BundleSubscriptionID set.
		bundleSubID, exists := c.Get("bundle_subscription_id")
		if !exists {
			c.Next()
			return
		}
		subID, ok := bundleSubID.(int64)
		if !ok || subID <= 0 {
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
		groupID, err := m.resolver.ResolveGroup(c.Request.Context(), modelName, subID)
		if err != nil {
			status := http.StatusBadRequest
			code := "BUNDLE_MODEL_NOT_INCLUDED"
			msg := err.Error()
			if err == service.ErrBundleExpired {
				status = http.StatusForbidden
				code = "BUNDLE_EXPIRED"
			}
			c.JSON(status, gin.H{"error": gin.H{"type": code, "message": msg}})
			c.Abort()
			return
		}

		// Inject resolved group_id into context for downstream use.
		c.Set("bundle_resolved_group_id", groupID.GroupID)
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
