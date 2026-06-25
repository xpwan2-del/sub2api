// bundle_handler.go 用户端套餐 Handler
// 提供面向普通用户的套餐浏览、用量查询和购买 API：
// - 浏览在售套餐计划列表和详情
// - 查看当前用户的活跃套餐订阅
// - 查看套餐用量进度
// - 发起套餐购买（checkout）

package handler

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

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
	paymentService            *service.PaymentService
}

// NewBundleHandler 创建用户端套餐 Handler
// NewBundleHandler creates a new user-facing bundle handler.
func NewBundleHandler(
	bundlePlanService *service.BundlePlanService,
	bundleSubscriptionService *service.BundleSubscriptionService,
	paymentService *service.PaymentService,
) *BundleHandler {
	return &BundleHandler{
		bundlePlanService:         bundlePlanService,
		bundleSubscriptionService: bundleSubscriptionService,
		paymentService:            paymentService,
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

// CheckoutRequest is the request body for bundle checkout.
type CheckoutRequest struct {
	PlanID      int64  `json:"plan_id" binding:"required"`
	PaymentType string `json:"payment_type"` // 纯余额支付时可为空
	ReturnURL   string `json:"return_url"`
	UseBalance  bool   `json:"use_balance"` // 是否使用账户余额抵扣
}

// Checkout 发起套餐购买，创建支付订单
// Checkout initiates a bundle purchase by creating a payment order.
// POST /bundles/checkout
func (h *BundleHandler) Checkout(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	var req CheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

		// Validate ReturnURL: only allow relative paths (starting with "/" and not "//")
		// to prevent open redirect attacks. Empty URLs are allowed.
		if req.ReturnURL != "" && !isValidReturnURL(req.ReturnURL) {
			response.BadRequest(c, "Invalid return_url: only relative paths are allowed")
			return
		}

	// Load plan to validate and get price.
	plan, err := h.bundlePlanService.GetPlanDetail(c.Request.Context(), req.PlanID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !plan.ForSale || plan.Status != "active" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "bundle_plan_disabled",
				"message": "该套餐已下架",
			},
		})
		return
	}

	// Create payment order via PaymentService.
	result, err := h.paymentService.CreateOrder(c.Request.Context(), service.CreateOrderRequest{
		UserID:      subject.UserID,
		Amount:      plan.Price,
		PaymentType: req.PaymentType,
		ClientIP:    c.ClientIP(),
		IsMobile:    isMobile(c),
		SrcHost:     c.Request.Host,
		SrcURL:      c.Request.Referer(),
		ReturnURL:   req.ReturnURL,
		OrderType:   "bundle",
		PlanID:      req.PlanID,
		Locale:      c.GetHeader("Accept-Language"),
		UseBalance:  req.UseBalance,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// isValidReturnURL validates that a return URL is a relative path (not an absolute URL)
// to prevent open redirect attacks. Only paths starting with "/" and not "//" are allowed.
func isValidReturnURL(rawURL string) bool {
	if rawURL == "" {
		return true
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	// Only allow relative paths: no scheme, no host, starts with "/".
	if u.Scheme != "" || u.Host != "" {
		return false
	}
	return strings.HasPrefix(rawURL, "/") && !strings.HasPrefix(rawURL, "//")
}
