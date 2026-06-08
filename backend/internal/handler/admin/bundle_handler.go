package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// BundleAdminHandler handles admin bundle plan and subscription management.
type BundleAdminHandler struct {
	bundlePlanService         *service.BundlePlanService
	bundleSubscriptionService *service.BundleSubscriptionService
}

// NewBundleAdminHandler creates a new admin bundle handler.
func NewBundleAdminHandler(
	bundlePlanService *service.BundlePlanService,
	bundleSubscriptionService *service.BundleSubscriptionService,
) *BundleAdminHandler {
	return &BundleAdminHandler{
		bundlePlanService:         bundlePlanService,
		bundleSubscriptionService: bundleSubscriptionService,
	}
}

// CreatePlanRequest represents the request body for creating a bundle plan.
type CreatePlanRequest = service.CreateBundlePlanRequest

// UpdatePlanRequest represents the request body for updating a bundle plan.
type UpdatePlanRequest = service.UpdateBundlePlanRequest

// ExtendSubscriptionRequest represents the request body for extending a bundle subscription.
type ExtendSubscriptionRequest struct {
	Days int `json:"days" binding:"required,min=1"`
}

// CreatePlan creates a new bundle plan.
// POST /admin/bundle/plans
func (h *BundleAdminHandler) CreatePlan(c *gin.Context) {
	var req CreatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	plan, err := h.bundlePlanService.CreatePlan(c.Request.Context(), &req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, plan)
}

// UpdatePlan updates an existing bundle plan.
// PUT /admin/bundle/plans/:id
func (h *BundleAdminHandler) UpdatePlan(c *gin.Context) {
	planID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID")
		return
	}

	var req UpdatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	plan, err := h.bundlePlanService.UpdatePlan(c.Request.Context(), planID, &req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, plan)
}

// ListPlans returns a paginated list of bundle plans.
// GET /admin/bundle/plans
func (h *BundleAdminHandler) ListPlans(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	tier := c.Query("tier")
	status := c.Query("status")

	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}

	plans, pag, err := h.bundlePlanService.ListPlans(c.Request.Context(), params, tier, status)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.PaginatedWithResult(c, plans, toResponsePagination(pag))
}

// GetPlanDetail returns a single bundle plan by ID.
// GET /admin/bundle/plans/:id
func (h *BundleAdminHandler) GetPlanDetail(c *gin.Context) {
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

// DisablePlan disables a bundle plan by setting its status to "disabled".
// DELETE /admin/bundle/plans/:id
func (h *BundleAdminHandler) DisablePlan(c *gin.Context) {
	planID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID")
		return
	}

	disabled := "disabled"
	_, err = h.bundlePlanService.UpdatePlan(c.Request.Context(), planID, &service.UpdateBundlePlanRequest{
		Status: &disabled,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Plan disabled successfully"})
}

// ListSubscriptions returns a paginated list of bundle subscriptions.
// GET /admin/bundle/subscriptions
func (h *BundleAdminHandler) ListSubscriptions(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	status := c.Query("status")

	var userID *int64
	if userIDStr := c.Query("user_id"); userIDStr != "" {
		if id, err := strconv.ParseInt(userIDStr, 10, 64); err == nil {
			userID = &id
		}
	}

	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}

	subs, pag, err := h.bundleSubscriptionService.List(c.Request.Context(), params, userID, status)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.PaginatedWithResult(c, subs, toResponsePagination(pag))
}

// RevokeSubscription revokes an active bundle subscription.
// POST /admin/bundle/subscriptions/:id/revoke
func (h *BundleAdminHandler) RevokeSubscription(c *gin.Context) {
	subID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	if err := h.bundleSubscriptionService.RevokeBundle(c.Request.Context(), subID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Bundle subscription revoked successfully"})
}

// ExtendSubscription extends a bundle subscription by a number of days.
// POST /admin/bundle/subscriptions/:id/extend
func (h *BundleAdminHandler) ExtendSubscription(c *gin.Context) {
	subID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	var req ExtendSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if err := h.bundleSubscriptionService.ExtendBundle(c.Request.Context(), subID, req.Days); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Bundle subscription extended successfully"})
}
