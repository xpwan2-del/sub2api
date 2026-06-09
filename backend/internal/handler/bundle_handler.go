// bundle_handler.go 用户端套餐 Handler
// 提供面向普通用户的套餐浏览和用量查询 API：
// - 浏览在售套餐计划列表和详情
// - 查看当前用户的活跃套餐订阅
// - 查看套餐用量进度
// - 发起套餐购买（checkout，暂未实现）

package handler

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// BundleHandler 用户端套餐 Handler
// BundleHandler handles user-facing bundle operations.
type BundleHandler struct {
	bundlePlanService         *service.BundlePlanService
	bundleSubscriptionService *service.BundleSubscriptionService
}

// NewBundleHandler 创建用户端套餐 Handler
// NewBundleHandler creates a new user-facing bundle handler.
func NewBundleHandler(
	bundlePlanService *service.BundlePlanService,
	bundleSubscriptionService *service.BundleSubscriptionService,
) *BundleHandler {
	return &BundleHandler{
		bundlePlanService:         bundlePlanService,
		bundleSubscriptionService: bundleSubscriptionService,
	}
}

// ListPlans 获取所有在售套餐计划
// ListPlans returns all plans currently for sale.
// GET /bundles/plans
func (h *BundleHandler) ListPlans(c *gin.Context) {
	plans, err := h.bundlePlanService.ListForSale(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, plans)
}

// GetPlanDetail 获取套餐计划详情
// GetPlanDetail returns a single bundle plan by ID.
// GET /bundles/plans/:id
func (h *BundleHandler) GetPlanDetail(c *gin.Context) {
	planID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID")
		return
	}

	plan, err := h.bundlePlanService.GetPlanDetail(c.Request.Context(), planID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, plan)
}

// GetMyBundle 获取当前用户的活跃套餐订阅
// GetMyBundle returns the current user's active bundle subscription.
// GET /bundles/subscription
func (h *BundleHandler) GetMyBundle(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	bundles, err := h.bundleSubscriptionService.GetUserActiveBundle(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, bundles)
}

// GetMyUsage 获取当前用户的套餐用量进度
// GetMyUsage returns the current user's bundle usage progress.
// GET /bundles/subscription/usage
func (h *BundleHandler) GetMyUsage(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	bundles, err := h.bundleSubscriptionService.GetUserActiveBundle(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	if len(bundles) == 0 {
		response.NotFound(c, "No active bundle subscription found")
		return
	}

	progress, err := h.bundleSubscriptionService.GetBundleUsageProgress(c.Request.Context(), bundles[0].ID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, progress)
}

// Checkout 发起套餐购买（暂未实现）
// Checkout initiates a bundle purchase. Not yet implemented.
// POST /bundles/checkout
func (h *BundleHandler) Checkout(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "Checkout is not yet implemented",
	})
}
