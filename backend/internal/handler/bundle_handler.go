package handler

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// BundleHandler handles user-facing bundle operations.
type BundleHandler struct {
	bundlePlanService         *service.BundlePlanService
	bundleSubscriptionService *service.BundleSubscriptionService
}

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

// Checkout initiates a bundle purchase. Not yet implemented.
// POST /bundles/checkout
func (h *BundleHandler) Checkout(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "Checkout is not yet implemented",
	})
}
