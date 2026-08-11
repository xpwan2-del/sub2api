package admin

import (
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// adminUserID 取当前认证 admin 的 user id;缺失返回 0。
// 复用 middleware.GetAuthSubjectFromContext(由 admin auth 中间件注入)。
func adminUserID(c *gin.Context) int64 {
	if subject, ok := middleware.GetAuthSubjectFromContext(c); ok {
		return subject.UserID
	}
	return 0
}

// --- 上游源配置 ---

// ListUpstreamSources 列出全部上游 new-api 同步源配置。
// GET /api/v1/admin/channels/upstream-sources
func (h *ChannelHandler) ListUpstreamSources(c *gin.Context) {
	list, err := h.upstreamPriceSyncService.ListConfigs(c.Request.Context())
	if err != nil {
		response.InternalError(c, "list upstream sources: "+err.Error())
		return
	}
	if list == nil {
		list = []service.UpstreamSourceConfig{}
	}
	response.Success(c, list)
}

// CreateUpstreamSource 新建上游同步源配置。
// POST /api/v1/admin/channels/upstream-sources
func (h *ChannelHandler) CreateUpstreamSource(c *gin.Context) {
	var req service.UpstreamSourceConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	if err := h.upstreamPriceSyncService.CreateConfig(c.Request.Context(), &req); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, req)
}

// GetUpstreamSource 按 id 取单个上游同步源配置。
// GET /api/v1/admin/channels/upstream-sources/:id
func (h *ChannelHandler) GetUpstreamSource(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid id")
		return
	}
	cfg, err := h.upstreamPriceSyncService.GetConfig(c.Request.Context(), id)
	if err != nil {
		response.InternalError(c, "get upstream source: "+err.Error())
		return
	}
	response.Success(c, cfg)
}

// UpdateUpstreamSource 更新上游同步源配置(整体替换)。
// PUT /api/v1/admin/channels/upstream-sources/:id
func (h *ChannelHandler) UpdateUpstreamSource(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid id")
		return
	}
	var req service.UpstreamSourceConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	req.ID = id
	if err := h.upstreamPriceSyncService.UpdateConfig(c.Request.Context(), &req); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, req)
}

// DeleteUpstreamSource 按 id 删除上游同步源配置。
// DELETE /api/v1/admin/channels/upstream-sources/:id
func (h *ChannelHandler) DeleteUpstreamSource(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid id")
		return
	}
	if err := h.upstreamPriceSyncService.DeleteConfig(c.Request.Context(), id); err != nil {
		response.InternalError(c, "delete upstream source: "+err.Error())
		return
	}
	response.Success(c, gin.H{"deleted": id})
}

// SyncUpstreamNow 立即触发某个上游源的定价同步,返回新建审批单 id(无变更时为 0)。
// POST /api/v1/admin/channels/upstream-sources/:id/sync
func (h *ChannelHandler) SyncUpstreamNow(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid id")
		return
	}
	reqID, err := h.upstreamPriceSyncService.SyncNow(c.Request.Context(), id, adminUserID(c))
	if err != nil {
		response.InternalError(c, "sync upstream: "+err.Error())
		return
	}
	response.Success(c, gin.H{"request_id": reqID})
}

// ListUpstreamGroups 拉取上游可用分组字典({key: 展示名}),供 source 配置下拉。
// GET /api/v1/admin/channels/upstream-sources/:id/groups
func (h *ChannelHandler) ListUpstreamGroups(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid id")
		return
	}
	groups, err := h.upstreamPriceSyncService.ListUpstreamGroups(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"groups": groups})
}

// PreviewUpstreamGroups 按 base_url 直接拉取可用分组(新建 source 尚未保存时用)。
// POST /api/v1/admin/channels/upstream-sources/groups/preview
func (h *ChannelHandler) PreviewUpstreamGroups(c *gin.Context) {
	var body struct {
		BaseURL           string `json:"base_url" binding:"required"`
		ProxyID           *int64 `json:"proxy_id"`
		DashboardToken    string `json:"dashboard_token"`
		APIKey            string `json:"api_key"`
		DashboardAuthMode string `json:"dashboard_auth_mode"`
		DashboardUserID   *int64 `json:"dashboard_user_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	groups, err := h.upstreamPriceSyncService.PreviewUpstreamGroups(c.Request.Context(), body.BaseURL, body.ProxyID, body.DashboardToken, body.APIKey, body.DashboardAuthMode, body.DashboardUserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"groups": groups})
}

// RefreshUpstreamBalance 手动刷新单个上游源的余额快照。
// POST /api/v1/admin/channels/upstream-sources/:id/balance/refresh
func (h *ChannelHandler) RefreshUpstreamBalance(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid id")
		return
	}
	cfgRec, err := h.upstreamPriceSyncService.RefreshBalance(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfgRec)
}

// BatchRefreshUpstreamBalances 批量刷新所有可用上游源的余额快照。
// POST /api/v1/admin/channels/upstream-sources/balance/refresh
func (h *ChannelHandler) BatchRefreshUpstreamBalances(c *gin.Context) {
	ctx := c.Request.Context()
	configs, err := h.upstreamPriceSyncService.ListConfigs(ctx)
	if err != nil {
		response.InternalError(c, "list upstream sources: "+err.Error())
		return
	}

	eligible := make([]service.UpstreamSourceConfig, 0, len(configs))
	for i := range configs {
		if configs[i].Enabled && strings.TrimSpace(configs[i].DashboardToken) != "" {
			eligible = append(eligible, configs[i])
		}
	}

	const maxConcurrency = 10
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrency)

	var mu sync.Mutex
	successCount := 0
	errors := make([]gin.H, 0)
	for i := range eligible {
		cfgRec := eligible[i]
		g.Go(func() error {
			_, refreshErr := h.upstreamPriceSyncService.RefreshBalance(gctx, cfgRec.ID)
			mu.Lock()
			defer mu.Unlock()
			if refreshErr != nil {
				errors = append(errors, gin.H{
					"source_id":   cfgRec.ID,
					"source_name": cfgRec.Name,
					"error":       refreshErr.Error(),
				})
				return nil
			}
			successCount++
			return nil
		})
	}
	_ = g.Wait()

	response.Success(c, gin.H{
		"total":   len(eligible),
		"success": successCount,
		"failed":  len(errors),
		"skipped": len(configs) - len(eligible),
		"errors":  errors,
	})
}

// --- 审批单 ---

// priceChangeRequestDetail GetPriceChangeRequest 的响应结构:批次 + 条目。
type priceChangeRequestDetail struct {
	*service.PriceChangeRequest
	Items []service.PriceChangeItem `json:"items"`
}

// ListPriceChangeRequests 列审批批次,支持 status / source_config_id / page / page_size 过滤分页。
// GET /api/v1/admin/channels/price-change-requests
func (h *ChannelHandler) ListPriceChangeRequests(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	f := service.RequestFilter{
		Status:   c.Query("status"),
		Page:     page,
		PageSize: pageSize,
	}
	if v := c.Query("source_config_id"); v != "" {
		if sid, err := strconv.ParseInt(v, 10, 64); err == nil && sid > 0 {
			f.SourceConfigID = &sid
		}
	}
	list, total, err := h.upstreamPriceSyncService.ListRequests(c.Request.Context(), f)
	if err != nil {
		response.InternalError(c, "list price change requests: "+err.Error())
		return
	}
	if list == nil {
		list = []service.PriceChangeRequest{}
	}
	response.Paginated(c, list, total, page, pageSize)
}

// GetPriceChangeRequest 取审批批次详情(批次元信息 + 全部条目)。
// GET /api/v1/admin/channels/price-change-requests/:id
func (h *ChannelHandler) GetPriceChangeRequest(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid id")
		return
	}
	req, err := h.upstreamPriceSyncService.GetRequest(c.Request.Context(), id)
	if err != nil {
		response.InternalError(c, "get price change request: "+err.Error())
		return
	}
	items, err := h.upstreamPriceSyncService.ListItems(c.Request.Context(), id)
	if err != nil {
		response.InternalError(c, "list price change items: "+err.Error())
		return
	}
	if items == nil {
		items = []service.PriceChangeItem{}
	}
	response.Success(c, priceChangeRequestDetail{PriceChangeRequest: req, Items: items})
}

// reviewPriceChangeItemBody ReviewPriceChangeItem 请求体。
type reviewPriceChangeItemBody struct {
	Action     string                  `json:"action" binding:"required,oneof=apply reject ignore"`
	ApplyValue *service.ConvertedPrice `json:"apply_value"`
	Note       string                  `json:"note"`
}

// ReviewPriceChangeItem 对单条审批条目执行 apply / reject / ignore。
// POST /api/v1/admin/channels/price-change-requests/:id/items/:itemId/review
func (h *ChannelHandler) ReviewPriceChangeItem(c *gin.Context) {
	// :id 现仅作路径占位,实际以 :itemId 为准;仍校验其合法性以保持 URL 一致性。
	if _, err := strconv.ParseInt(c.Param("id"), 10, 64); err != nil {
		response.BadRequest(c, "invalid request id")
		return
	}
	itemID, err := strconv.ParseInt(c.Param("itemId"), 10, 64)
	if err != nil || itemID <= 0 {
		response.BadRequest(c, "invalid item id")
		return
	}
	var body reviewPriceChangeItemBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	if err := h.upstreamPriceSyncService.ReviewItem(c.Request.Context(), itemID, service.ReviewAction(body.Action), body.ApplyValue, adminUserID(c), body.Note); err != nil {
		response.InternalError(c, "review price change item: "+err.Error())
		return
	}
	response.Success(c, gin.H{"reviewed": itemID})
}

// ClosePriceChangeRequest 关闭审批批次(仅当无 pending 条目时)。
// POST /api/v1/admin/channels/price-change-requests/:id/close
func (h *ChannelHandler) ClosePriceChangeRequest(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid id")
		return
	}
	if err := h.upstreamPriceSyncService.CloseRequest(c.Request.Context(), id); err != nil {
		response.InternalError(c, "close price change request: "+err.Error())
		return
	}
	response.Success(c, gin.H{"closed": id})
}
