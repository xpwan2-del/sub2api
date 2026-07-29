package handler

import (
	"context"
	"log/slog"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminModelCatalogHandler 暴露模型广场可运营配置的管理端读写接口。
//
// 与 PublicModelCatalogHandler 同源：currentModelKeys 复用 ChannelService.ListAvailable
// 聚合当前所有去重模型键，使 EnsureFirstSeen 登记"用户可见模型"的首见时间。管理页通过
// List 读取全部运营配置（含 hidden），通过 Update 批量保存 pinned/sort_weight/custom_tags/
// featured_until/hidden（first_seen_at 只读，BatchSave 不写回）。
type AdminModelCatalogHandler struct {
	modelCatalogSvc service.ModelCatalogService
	channelService  *service.ChannelService
}

// NewAdminModelCatalogHandler 创建模型广场运营配置管理 handler。
func NewAdminModelCatalogHandler(channelService *service.ChannelService, modelCatalogSvc service.ModelCatalogService) *AdminModelCatalogHandler {
	return &AdminModelCatalogHandler{
		channelService:  channelService,
		modelCatalogSvc: modelCatalogSvc,
	}
}

// List 返回全部运营配置供管理页展示。
// GET /api/v1/admin/catalog/config
//
// 先 EnsureFirstSeen 登记当前可见模型的首见时间（best-effort：失败仅告警不阻断），
// 再 ListAllForAdmin 取全部配置行。无数据时返回非 nil 空切片（JSON `[]`，非 `null`）。
func (h *AdminModelCatalogHandler) List(c *gin.Context) {
	ctx := c.Request.Context()

	if err := h.modelCatalogSvc.EnsureFirstSeen(ctx, h.currentModelKeys(ctx)); err != nil {
		// 首见登记失败不应阻断管理页加载：仍返回已持久化的配置。
		slog.WarnContext(ctx, "model catalog: ensure first-seen failed on admin list", "err", err)
	}

	cfgs, err := h.modelCatalogSvc.ListAllForAdmin(ctx)
	if err != nil {
		response.InternalError(c, "load catalog config failed")
		return
	}
	if cfgs == nil {
		cfgs = []service.AdminCatalogConfig{}
	}
	response.Success(c, cfgs)
}

// Update 批量保存管理员提交的运营配置。
// PUT /api/v1/admin/catalog/config
func (h *AdminModelCatalogHandler) Update(c *gin.Context) {
	var req []service.AdminCatalogConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid catalog config payload")
		return
	}
	if err := h.modelCatalogSvc.BatchSave(c.Request.Context(), req); err != nil {
		response.InternalError(c, "save catalog config failed")
		return
	}
	response.Success(c, gin.H{"updated": len(req)})
}

// currentModelKeys 聚合当前所有去重模型键 (platform, model_name)，与 public catalog 同源
// （ChannelService.ListAvailable → SupportedModels）。channelService 未注入或读取失败时返回
// nil，EnsureFirstSeen 收到空键即 no-op，保证管理页不因渠道读取失败而不可用。
func (h *AdminModelCatalogHandler) currentModelKeys(ctx context.Context) []service.ModelKey {
	if h == nil || h.channelService == nil {
		return nil
	}
	channels, err := h.channelService.ListAvailable(ctx)
	if err != nil {
		slog.WarnContext(ctx, "model catalog: list available channels failed, skipping first-seen", "err", err)
		return nil
	}
	return modelKeysFromChannels(channels)
}

// modelKeysFromChannels 从渠道清单聚合去重的模型键。
// 与 buildPublicModelCatalog 同源：仅取 StatusActive 渠道，跳过空 platform/model_name，
// 按 (lowercase platform, lowercase model_name) 去重——即"用户在广场可见"的模型集合。
func modelKeysFromChannels(channels []service.AvailableChannel) []service.ModelKey {
	seen := make(map[string]struct{})
	out := make([]service.ModelKey, 0)
	for _, ch := range channels {
		if ch.Status != service.StatusActive {
			continue
		}
		for _, m := range ch.SupportedModels {
			name := strings.TrimSpace(m.Name)
			platform := strings.TrimSpace(m.Platform)
			if name == "" || platform == "" {
				continue
			}
			key := strings.ToLower(platform) + "\x00" + strings.ToLower(name)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, service.ModelKey{Platform: platform, ModelName: name})
		}
	}
	return out
}
