package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// modelCatalogServiceAdapter 将 repository 层的 ModelCatalogRepo 适配为
// service.ModelCatalogRepo 接口（返回 *service.ModelCatalogDisplay）。
//
// 设计与 user_platform_quota_service_adapter 一致：service 不反向 import repository
// （会与 repository→service 成环），故 DTO 互转的 adapter 写在 repository 包内。
// service.ModelCatalogDisplay / service.ModelKey 为导出类型，供此 adapter 命名转换。
type modelCatalogServiceAdapter struct {
	inner ModelCatalogRepo
}

// NewModelCatalogServiceAdapter 将 ModelCatalogRepo 实现包装为满足
// service.ModelCatalogRepo 接口的适配器。
func NewModelCatalogServiceAdapter(repo ModelCatalogRepo) service.ModelCatalogRepo {
	return &modelCatalogServiceAdapter{inner: repo}
}

// serviceKeysToRepo 将 service.ModelKey 切片转换为 repository.ModelKey 切片。
func serviceKeysToRepo(keys []service.ModelKey) []ModelKey {
	out := make([]ModelKey, len(keys))
	for i, k := range keys {
		out[i] = ModelKey{Platform: k.Platform, ModelName: k.ModelName}
	}
	return out
}

// repoDisplayToService 将 repository DTO 转为 service DTO（字段一一对应）。
// CustomTags 切片按 entToModelCatalogDisplay 惯例共享引用——service 侧只读不写。
func repoDisplayToService(d *ModelCatalogDisplay) *service.ModelCatalogDisplay {
	return &service.ModelCatalogDisplay{
		Platform:      d.Platform,
		ModelName:     d.ModelName,
		Pinned:        d.Pinned,
		SortWeight:    d.SortWeight,
		CustomTags:    d.CustomTags,
		FeaturedUntil: d.FeaturedUntil,
		Hidden:        d.Hidden,
		FirstSeenAt:   d.FirstSeenAt,
	}
}

// serviceDisplayToRepo 将 service DTO 转为 repository DTO。
func serviceDisplayToRepo(d *service.ModelCatalogDisplay) *ModelCatalogDisplay {
	return &ModelCatalogDisplay{
		Platform:      d.Platform,
		ModelName:     d.ModelName,
		Pinned:        d.Pinned,
		SortWeight:    d.SortWeight,
		CustomTags:    d.CustomTags,
		FeaturedUntil: d.FeaturedUntil,
		Hidden:        d.Hidden,
		FirstSeenAt:   d.FirstSeenAt,
	}
}

// GetByModelKeys 按 (platform, model_name) 批量查询。
//
// map key 不透传 repository 返回的内部 string key，而是用记录自身的 Platform/ModelName
// 字段按同算法（modelKey = platform + "\x00" + model_name）重新计算——与 service 查找侧
// （modelCatalogMapKey）严格对齐。两套算法当前等价，但重建确保边界解耦。
func (a *modelCatalogServiceAdapter) GetByModelKeys(ctx context.Context, keys []service.ModelKey) (map[string]*service.ModelCatalogDisplay, error) {
	repoMap, err := a.inner.GetByModelKeys(ctx, serviceKeysToRepo(keys))
	if err != nil {
		return nil, err
	}
	out := make(map[string]*service.ModelCatalogDisplay, len(repoMap))
	for _, d := range repoMap {
		out[modelKey(d.Platform, d.ModelName)] = repoDisplayToService(d)
	}
	return out, nil
}

func (a *modelCatalogServiceAdapter) UpsertMissing(ctx context.Context, keys []service.ModelKey) error {
	return a.inner.UpsertMissing(ctx, serviceKeysToRepo(keys))
}

func (a *modelCatalogServiceAdapter) ListAll(ctx context.Context) ([]*service.ModelCatalogDisplay, error) {
	rows, err := a.inner.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*service.ModelCatalogDisplay, len(rows))
	for i, r := range rows {
		out[i] = repoDisplayToService(r)
	}
	return out, nil
}

// BatchUpsert 将 service DTO 切片转为 repository DTO 切片后下沉。
// FirstSeenAt 在 service→repo 方向恒为零值（service.BatchSave 从不写该只读字段）。
func (a *modelCatalogServiceAdapter) BatchUpsert(ctx context.Context, cfgs []*service.ModelCatalogDisplay) error {
	repoCfgs := make([]*ModelCatalogDisplay, len(cfgs))
	for i, c := range cfgs {
		repoCfgs[i] = serviceDisplayToRepo(c)
	}
	return a.inner.BatchUpsert(ctx, repoCfgs)
}
