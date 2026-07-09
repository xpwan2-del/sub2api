package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"

	"github.com/gin-gonic/gin"
)

func RegisterPublicModelRoutes(v1 *gin.RouterGroup, h *handler.Handlers) {
	public := v1.Group("/public")
	{
		models := public.Group("/models")
		models.GET("/catalog", h.PublicModelCatalog.List)
	}
}
