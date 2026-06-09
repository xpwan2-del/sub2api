// bundle.go 用户端套餐路由注册
// 注册 /bundles 下的所有用户端路由，均需要认证（requireAuth）。

package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterBundleRoutes registers user-facing bundle routes.
func RegisterBundleRoutes(v1 *gin.RouterGroup, h *handler.Handlers, requireAuth gin.HandlerFunc) {
	bundles := v1.Group("/bundles")
	bundles.Use(requireAuth)
	{
		bundles.GET("/plans", h.Bundle.ListPlans)
		bundles.GET("/plans/:id", h.Bundle.GetPlanDetail)
		bundles.GET("/subscription", h.Bundle.GetMyBundle)
		bundles.GET("/subscription/usage", h.Bundle.GetMyUsage)
		bundles.POST("/checkout", h.Bundle.Checkout)
	}
}
