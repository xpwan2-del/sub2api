package service

import (
	"context"
	"log/slog"
	"time"
)

// CatalogDisplayInfo 是模型广场卡片合并运营配置后的展示信息（供 public handler 输出）。
type CatalogDisplayInfo struct {
	Pinned     bool
	SortWeight int
	Tags       []string // 自动能力标签 + 手动 custom_tags + 条件 new/featured，去重保序
	IsNew      bool
	Featured   bool
}

// CatalogItem 是模型广场卡片的 service 层契约。
// public handler 从渠道清单构建后传入 MergeDisplayConfig 合并运营配置；
// 定价/健康等其余展示字段由 handler 自行承载，merge 不改写。
type CatalogItem struct {
	Platform     string              // 渠道平台（与渠道 platform 对齐）
	ModelName    string              // 模型名（与渠道 model_name 对齐）
	Name         string              // 展示名
	Capabilities []string            // 调用方/前端推断的自动能力标签（如 multimodal），合并进 Tags
	Display      *CatalogDisplayInfo // merge 后填充；nil=无配置（降级或未配置）
}

// ModelKey 标识一个 (platform, model_name) 二元组，用于运营配置的批量读写。
// 与 repository.ModelKey 字段对齐，由 wire adapter 互转。
type ModelKey struct {
	Platform  string
	ModelName string
}

// AdminCatalogConfig 是管理页 GET/PUT 的运营配置：持久化字段 + 只读派生展示字段。
// json tag 固定 snake_case 线上契约，与 public catalog 字段名（platform/pinned/sort_weight/
// tags/is_new/featured）对齐；admin 专有的复合键与只读字段为 model_name/custom_tags/
// featured_until/hidden/first_seen_at。前端 (A9) 与本结构体字段名严格一致。
type AdminCatalogConfig struct {
	Platform      string     `json:"platform"`
	ModelName     string     `json:"model_name"`
	Pinned        bool       `json:"pinned"`
	SortWeight    int        `json:"sort_weight"`
	CustomTags    []string   `json:"custom_tags"`
	FeaturedUntil *time.Time `json:"featured_until"`
	Hidden        bool       `json:"hidden"`
	FirstSeenAt   time.Time  `json:"first_seen_at"` // 只读：一经设定不可变，BatchSave 不写回
	Tags          []string   `json:"tags"`          // 派生：自动+手动合并（只读展示）
	IsNew         bool       `json:"is_new"`        // 手动 NEW 开关（持久化）
	Featured      bool       `json:"featured"`      // 手动精选开关（持久化；可选 featured_until 到期）
}

// ModelCatalogDisplay 是运营配置的 service 层内部 DTO，与 repository.ModelCatalogDisplay
// 字段对齐，供 wire adapter（service/wire.go）转换。service 不直接 import repository（depguard）。
type ModelCatalogDisplay struct {
	Platform      string
	ModelName     string
	Pinned        bool
	SortWeight    int
	CustomTags    []string
	FeaturedUntil *time.Time
	Hidden        bool
	IsNew         bool
	Featured      bool
	FirstSeenAt   time.Time
}

// ModelCatalogRepo 是模型广场运营配置的数据访问接口（service 包内定义）。
// repository.ModelCatalogRepo 的实现由 wire adapter 适配进此接口——两者方法签名等价但
// 使用 service 包内的 DTO/ModelKey 名义类型，故需在 service/wire.go（depguard 白名单）做转换。
type ModelCatalogRepo interface {
	// GetByModelKeys 按 (platform, model_name) 复合键批量查询，返回 map。
	// map key 为 platform + "\x00" + model_name；未命中的 key 不出现在结果中。
	GetByModelKeys(ctx context.Context, keys []ModelKey) (map[string]*ModelCatalogDisplay, error)
	// UpsertMissing 为尚未存在的 (platform, model_name) 插入默认行（不覆盖已存在行）。
	UpsertMissing(ctx context.Context, keys []ModelKey) error
	// ListAll 返回全部展示配置（按 platform、model_name 升序），无数据时返回非 nil 空切片。
	ListAll(ctx context.Context) ([]*ModelCatalogDisplay, error)
	// BatchUpsert 按 (platform, model_name) 复合键 upsert 可运营字段，不改动 first_seen_at。
	BatchUpsert(ctx context.Context, cfgs []*ModelCatalogDisplay) error
}

// modelCatalogSettings 抽象 ModelCatalogService 所需的两个设置读取，使其可被 stub 测试。
// *SettingService 结构化实现该接口（IsModelCatalogOpsEnabled / GetModelCatalogNewModelDays）。
type modelCatalogSettings interface {
	IsModelCatalogOpsEnabled(ctx context.Context) bool
}

// ModelCatalogService 提供模型广场可运营展示配置的合并、首见记录与管理。
type ModelCatalogService interface {
	// MergeDisplayConfig 按 (platform, model_name) 取配置 merge 进 items：
	// 过滤 hidden、判定 new/featured、合并 tags。配置层故障降级返回原 items（不阻断展示）。
	MergeDisplayConfig(ctx context.Context, items []CatalogItem) ([]CatalogItem, error)
	// EnsureFirstSeen 为未见过的模型键登记首见时间（不覆盖已存在行）。
	EnsureFirstSeen(ctx context.Context, keys []ModelKey) error
	// ListAllForAdmin 返回全部运营配置（含派生的 Tags/IsNew/Featured），供管理页展示。
	ListAllForAdmin(ctx context.Context) ([]AdminCatalogConfig, error)
	// BatchSave 批量保存管理员提交的运营配置（不改动只读的 first_seen_at）。
	BatchSave(ctx context.Context, cfgs []AdminCatalogConfig) error
	// IsEnabled 返回运营功能总开关状态（默认开）。
	IsEnabled(ctx context.Context) bool
}

type modelCatalogServiceImpl struct {
	repo     ModelCatalogRepo
	settings modelCatalogSettings
}

// NewModelCatalogService 创建模型广场运营配置服务。
func NewModelCatalogService(repo ModelCatalogRepo, settings modelCatalogSettings) ModelCatalogService {
	return &modelCatalogServiceImpl{repo: repo, settings: settings}
}

func (s *modelCatalogServiceImpl) IsEnabled(ctx context.Context) bool {
	return s.settings.IsModelCatalogOpsEnabled(ctx)
}

// MergeDisplayConfig 合并运营配置到去重后的模型卡片。
//
// 处理流程：按 (platform, model_name) 批量取配置 → hidden 过滤 → 填充 Display
// （Pinned/SortWeight/Tags/IsNew/Featured）。repo 报错时降级：记日志并原样返回 items
// （Display=nil），不阻断展示链路。settings 读取内部已降级，不会抛错。
func (s *modelCatalogServiceImpl) MergeDisplayConfig(ctx context.Context, items []CatalogItem) ([]CatalogItem, error) {
	if len(items) == 0 {
		return items, nil
	}
	// 运营功能总开关关闭时回归原始展示：不 merge 任何运营配置
	// （不过滤 hidden、不置顶、不判 new/featured），与配置层故障降级语义一致。
	if !s.settings.IsModelCatalogOpsEnabled(ctx) {
		return items, nil
	}

	keys := make([]ModelKey, 0, len(items))
	for _, it := range items {
		keys = append(keys, ModelKey{Platform: it.Platform, ModelName: it.ModelName})
	}
	cfgs, err := s.repo.GetByModelKeys(ctx, keys)
	if err != nil {
		slog.WarnContext(ctx, "model catalog: get display config failed, degrading to unmerged items", "err", err)
		return items, nil
	}

	now := time.Now()
	out := make([]CatalogItem, 0, len(items))
	for _, it := range items {
		cfg := cfgs[modelCatalogMapKey(it.Platform, it.ModelName)]
		// hidden=true 的模型从广场过滤掉（不返回）。
		if cfg != nil && cfg.Hidden {
			continue
		}
		merged := it
		merged.Display = mergeDisplay(now, it, cfg)
		out = append(out, merged)
	}
	return out, nil
}

func (s *modelCatalogServiceImpl) EnsureFirstSeen(ctx context.Context, keys []ModelKey) error {
	return s.repo.UpsertMissing(ctx, keys)
}

func (s *modelCatalogServiceImpl) ListAllForAdmin(ctx context.Context) ([]AdminCatalogConfig, error) {
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := make([]AdminCatalogConfig, 0, len(rows))
	for _, r := range rows {
		out = append(out, displayToAdminConfig(now, r))
	}
	return out, nil
}

func (s *modelCatalogServiceImpl) BatchSave(ctx context.Context, cfgs []AdminCatalogConfig) error {
	rows := make([]*ModelCatalogDisplay, 0, len(cfgs))
	for _, c := range cfgs {
		// FirstSeenAt 不可由管理员修改：不写入 BatchUpsert（repo 侧也不会更新该列）。
		rows = append(rows, &ModelCatalogDisplay{
			Platform:      c.Platform,
			ModelName:     c.ModelName,
			Pinned:        c.Pinned,
			SortWeight:    c.SortWeight,
			CustomTags:    c.CustomTags,
			FeaturedUntil: c.FeaturedUntil,
			Hidden:        c.Hidden,
			IsNew:         c.IsNew,
			Featured:      c.Featured,
		})
	}
	return s.repo.BatchUpsert(ctx, rows)
}

// mergeDisplay 将单条运营配置 merge 进 CatalogItem，生成展示信息。
// cfg 为 nil 表示无配置（模型未登记），此时仅保留自动能力标签 + 可能的 new/featured=false。
func mergeDisplay(now time.Time, it CatalogItem, cfg *ModelCatalogDisplay) *CatalogDisplayInfo {
	isNew, featured := classifyDisplay(now, cfg)

	info := &CatalogDisplayInfo{
		IsNew:    isNew,
		Featured: featured,
	}
	if cfg != nil {
		info.Pinned = cfg.Pinned
		info.SortWeight = cfg.SortWeight
	}

	// Tags 合并：自动能力标签（调用方传入）+ 手动 custom_tags + 条件 new/featured，去重保序。
	capHint := len(it.Capabilities) + 2
	if cfg != nil {
		capHint += len(cfg.CustomTags)
	}
	tags := make([]string, 0, capHint)
	tags = append(tags, it.Capabilities...)
	if cfg != nil {
		tags = append(tags, cfg.CustomTags...)
	}
	if isNew {
		tags = append(tags, "new")
	}
	if featured {
		tags = append(tags, "featured")
	}
	info.Tags = dedupStringsPreserveOrder(tags)
	return info
}

// displayToAdminConfig 将运营配置行转为管理页 DTO，附带派生展示字段。
func displayToAdminConfig(now time.Time, r *ModelCatalogDisplay) AdminCatalogConfig {
	isNew, featured := classifyDisplay(now, r)
	// 管理页无调用方传入的能力标签：Tags = custom_tags + 条件 new/featured。
	tags := make([]string, 0, len(r.CustomTags)+2)
	tags = append(tags, r.CustomTags...)
	if isNew {
		tags = append(tags, "new")
	}
	if featured {
		tags = append(tags, "featured")
	}
	return AdminCatalogConfig{
		Platform:      r.Platform,
		ModelName:     r.ModelName,
		Pinned:        r.Pinned,
		SortWeight:    r.SortWeight,
		CustomTags:    r.CustomTags,
		FeaturedUntil: r.FeaturedUntil,
		Hidden:        r.Hidden,
		FirstSeenAt:   r.FirstSeenAt,
		Tags:          dedupStringsPreserveOrder(tags),
		IsNew:         isNew,
		Featured:      featured,
	}
}

// classifyDisplay 判定 new/featured：
//   - new：手动开关 cfg.IsNew（first_seen_at 不再驱动）
//   - featured：手动开关 cfg.Featured，且（无 featured_until 或 featured_until 未过期）
func classifyDisplay(now time.Time, cfg *ModelCatalogDisplay) (isNew bool, featured bool) {
	if cfg == nil {
		return false, false
	}
	isNew = cfg.IsNew
	featured = cfg.Featured
	if featured && cfg.FeaturedUntil != nil && now.After(*cfg.FeaturedUntil) {
		featured = false
	}
	return isNew, featured
}

// dedupStringsPreserveOrder 去重保序，跳过空串。
func dedupStringsPreserveOrder(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// modelCatalogMapKey 生成 map 复合键：platform + NUL + model_name。
// 与 repository 层 modelKey 算法一致（NUL 不会出现在正常字符串中，避免歧义碰撞）。
func modelCatalogMapKey(platform, modelName string) string {
	return platform + "\x00" + modelName
}
