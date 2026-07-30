// public_bundle_plan_handler.go 公开套餐计划 Handler
// 提供无鉴权的在售套餐列表，供模型广场顶部套餐展示区使用。
// DTO 刻意收窄：剥离 group_id/group_name/精确额度/concurrency/rpm 等内部细节，
// 仅保留展示字段 + 由 group_quotas 聚合的去重 platform 列表。

package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// bundlePlanForSaleReader 是 PublicBundlePlanHandler 的窄读取依赖。
// *service.BundlePlanService 满足该接口；用接口而非具体类型，使 handler 可脱离
// 真实 repository/cache 做隔离测试，且不破坏 handler→service 的分层约束。
type bundlePlanForSaleReader interface {
	ListForSale(ctx context.Context) ([]service.BundlePlan, error)
}

// PublicBundlePlanHandler 暴露只读的公开在售套餐列表。
//
// 该 DTO 刻意收窄：不暴露渠道组 ID/名称、精确额度（*_limit_usd/*_limit_count）、
// 并发/RPM 限制等内部细节——仅保留展示字段，外加由 group_quotas 聚合的 platform 列表。
type PublicBundlePlanHandler struct {
	bundlePlanSvc   bundlePlanForSaleReader
	modelCatalogSvc service.ModelCatalogService
}

// NewPublicBundlePlanHandler 创建公开套餐计划 Handler。
func NewPublicBundlePlanHandler(bundlePlanSvc *service.BundlePlanService, modelCatalogSvc service.ModelCatalogService) *PublicBundlePlanHandler {
	return &PublicBundlePlanHandler{
		bundlePlanSvc:   bundlePlanSvc,
		modelCatalogSvc: modelCatalogSvc,
	}
}

// PublicBundlePlan 是面向公网的套餐计划 DTO。
// 敏感/内部字段已剥离：group_id、group_name、精确额度（*_limit_usd/*_limit_count）、
// concurrency_limit、rpm_limit、quota_scope、model_pattern 均不公开。
// Platforms 由该套餐 group_quotas 的 group_platform 去重聚合而来。
type PublicBundlePlan struct {
	Name          string   `json:"name"`
	Tier          string   `json:"tier"`
	Description   string   `json:"description"`
	Price         float64  `json:"price"`
	OriginalPrice float64  `json:"original_price"`
	Currency      string   `json:"currency"`
	ValidityDays  int      `json:"validity_days"`
	Features      []string `json:"features"`
	SortOrder     int      `json:"sort_order"`
	Featured      bool     `json:"featured"`
	Platforms     []string `json:"platforms"`
}

// List 返回公开的在售套餐计划。
// GET /api/v1/public/bundles/plans
//
// 当模型广场运营总开关（ops_enabled）关闭时，不暴露任何套餐（返回空数组）；
// 开关由后端统一控制，前端无需读取。空结果返回 []（非 null）。
func (h *PublicBundlePlanHandler) List(c *gin.Context) {
	out, err := h.list(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, out)
}

// list 封装与 gin 无关的核心逻辑，便于隔离测试。
// 始终返回非 nil 切片（可能为空），确保 JSON 序列化为 [] 而非 null。
func (h *PublicBundlePlanHandler) list(ctx context.Context) ([]PublicBundlePlan, error) {
	// ops_enabled=false → 不暴露套餐。modelCatalogSvc 未注入（降级场景）时同样不暴露，
	// 避免 nil 解引用；正常 Wire 装配下该服务恒被注入。
	if h == nil || h.modelCatalogSvc == nil || !h.modelCatalogSvc.IsEnabled(ctx) {
		return []PublicBundlePlan{}, nil
	}
	plans, err := h.bundlePlanSvc.ListForSale(ctx)
	if err != nil {
		return nil, err
	}
	return toPublicBundlePlans(plans), nil
}

// toPublicBundlePlans 将 service 层套餐映射为公开 DTO，保持 ListForSale 返回的
// （按 sort_order 的）顺序。敏感字段被丢弃，Platforms 按 plan 聚合。
// Features 为 nil 时归一为空切片，避免 JSON null。
func toPublicBundlePlans(plans []service.BundlePlan) []PublicBundlePlan {
	out := make([]PublicBundlePlan, 0, len(plans))
	for i := range plans {
		features := plans[i].Features
		if features == nil {
			features = []string{}
		}
		out = append(out, PublicBundlePlan{
			Name:          plans[i].Name,
			Tier:          plans[i].Tier,
			Description:   plans[i].Description,
			Price:         plans[i].Price,
			OriginalPrice: plans[i].OriginalPrice,
			Currency:      plans[i].Currency,
			ValidityDays:  plans[i].ValidityDays,
			Features:      features,
			SortOrder:     plans[i].SortOrder,
			Featured:      plans[i].Featured,
			Platforms:     dedupPlatforms(plans[i].GroupQuotas),
		})
	}
	return out
}

// dedupPlatforms 从套餐的 group_quotas 中取 group_platform 去重（保序）。
// 空白 platform 跳过；无任何平台时返回非 nil 空切片。
func dedupPlatforms(quotas []service.BundlePlanGroupQuota) []string {
	seen := make(map[string]struct{}, len(quotas))
	out := make([]string, 0, len(quotas))
	for _, q := range quotas {
		p := q.GroupPlatform
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}
