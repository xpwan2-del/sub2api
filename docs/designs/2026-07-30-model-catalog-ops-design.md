# 模型广场运营优化设计

- 日期：2026-07-30
- 分支：`feat/model-catalog-ops`
- 状态：设计已确认，待实现

## 1. 背景与目标

模型广场（model catalog）分两端：

- **admin 管理页** `frontend/src/views/admin/CatalogManageView.vue` —— 运营配置入口，经 `GET/PUT /admin/catalog/config` 管理置顶/排序/标签/精选/隐藏。
- **公开页** `frontend/src/views/public/ModelCatalogView.vue` —— 面向最终用户展示，经 `GET /public/models/catalog`（+ `GET /public/bundles/plans`）取数。

底层架构是「渠道为源、display 表为运营覆盖层」：模型清单由活跃渠道（`ChannelService.ListAvailable`）动态聚合，`model_catalog_displays` 表只存每条 `(platform, model_name)` 的运营元数据（`pinned / sort_weight / custom_tags / featured_until / hidden / first_seen_at`）。admin 与公开页的模型集合因此会因「隐藏 / 渠道状态 / 孤儿行」产生差异，这是设计使然。

本次要解决用户提出的 4 个问题：

| # | 需求 | 处置 |
|---|---|---|
| 1 | admin 控制全部模型排序（含置顶）+ NEW 标签手动控制（默认不显示） | 实现 |
| 2 | 公开页筛选失效 + 展示模型与 admin 不一致 | 修复筛选 bug；差异为设计使然，admin 已标注隐藏项 |
| 3 | admin 的 featured 在公开页无特殊显示 | 修复（统一为手动开关）|
| 4 | 无套餐时隐藏首页套餐区块 | 跳过（代码已用 `v-if="bundlePlans.length>0"` 守卫）|

## 2. 已确认的根因

### 2.1 NEW 标签不可控（需求 1）
`is_new` 是后端**派生只读**字段：`classifyDisplay`（`backend/internal/service/model_catalog_service.go:259-270`）按 `first_seen_at` 是否落在 `new_model_days`（默认 30 天）窗口内判定。admin 只能调窗口天数（顶部 `new_model_days` 设置条），不能逐模型控制。

### 2.2 排序仅置顶区可控（需求 1）
admin 仅"置顶区"可拖拽（`onPinnedDragEnd` → 写 `sort_weight`，`CatalogManageView.vue:327-333`），非置顶模型在公开页按 `compareCatalogCards`（`frontend/src/utils/modelCatalog.ts:347-367`）的 `tagScore → 价格` 自动排，admin 无法手动控制全部模型顺序。

### 2.3 筛选失效（需求 2）
`ModelCatalogView.vue:20` 的 `v-model:filters="filters"` 绑定在 `const filters = reactive(...)`（line 90）上；子组件 `ModelCatalogFilters.vue:60-64` 的 `update()` emit **整体替换对象** `{...props.filters, [key]: value}`。Vue 把 `v-model:filters` 编译为 setter `(filters) = $event`，对 `const` 赋值抛 `Assignment to constant variable`，错误被吞，**所有筛选控件静默失效**。仓库其它 admin 页用 `v-model="filters.xxx"`（属性赋值）才幸免。

### 2.4 featured 断链（需求 3）
admin 勾"featured"经 `toggleTag(item,'featured')` 写入 `custom_tags`（`CatalogManageView.vue:177`）；后端 `classifyDisplay` 的 `featured` **只由 `featured_until` 日期驱动**（`model_catalog_service.go:266-268`），与 custom_tags 无关；公开页 `ModelCard.vue:89` 的 badge 只认派生 `model.featured` 布尔。**只勾选不填日期 → 公开页不显示**。两套机制断链。

### 2.5 模型集合差异（需求 2）
公开页只展示「活跃渠道 + 非隐藏」模型（`MergeDisplayConfig` 过滤 `hidden=true`，`model_catalog_service.go:150-152`；`buildPublicModelCatalog` 仅取 `StatusActive` 渠道）；admin 的 `ListAllForAdmin` 返回全部 display 行（含隐藏、含已无活跃渠道的孤儿行）。filters 修复后用户可看到全部模型，隐藏项 admin 已有"隐藏"标签明示。

## 3. 设计决策（已与用户确认）

1. **排序**：admin 改为**单一可拖拽列表**展示全部模型，拖拽按位置重写 `sort_weight`；公开页 `置顶 → sort_weight → tagScore → 价格`。
2. **NEW**：改为**纯手动开关**，新增持久化 `is_new` bool（默认 false）；移除 `first_seen_at` 时间窗派生；`new_model_days` 设置项下线。
3. **featured**：**统一为手动开关**，新增持久化 `featured` bool（默认 false）；勾选即公开页显示，无需填日期。`featured_until` 保留为**可选"到期自动隐藏"高级项**（不填则永久）。
4. **featured_until 保留**：`featured` 展示值 = `featured==true && (featured_until 为空 ‖ now ≤ featured_until)`。
5. **保存策略**：所有改动（含拖拽）统一置 dirty，由"保存全部"按钮落库（与现有开关一致，避免拖拽自动存而开关不存的割裂）。
6. **套餐区块（需求 4）**：跳过。

## 4. 数据模型变更

`model_catalog_displays` 表（`backend/ent/schema/model_catalog_display.go`）：

| 字段 | 变更 |
|---|---|
| `is_new` | **新增** `field.Bool("is_new").Default(false)` |
| `featured` | **新增** `field.Bool("featured").Default(false)` |
| `featured_until` | 保留，语义降级为可选到期 |
| `first_seen_at` | 保留，**不再驱动 NEW**，仅作只读信息 |
| 其余（pinned/sort_weight/hidden/custom_tags） | 不变 |

迁移 `migrations/NNN_model_catalog_manual_flags.sql`（NNN 取下一序号）：

```sql
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE model_catalog_displays ADD COLUMN IF NOT EXISTS is_new BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE model_catalog_displays ADD COLUMN IF NOT EXISTS featured BOOLEAN NOT NULL DEFAULT FALSE;

-- 平滑回填：旧 featured_until 未到期的行置 featured=true
UPDATE model_catalog_displays
SET featured = TRUE
WHERE featured_until IS NOT NULL AND featured_until > now();
```

不动 `featured_until` / `first_seen_at` 列。改完 ent schema 后 `go generate ./ent` 并提交生成代码。

## 5. 后端改动

### 5.1 service（`model_catalog_service.go`）
- `AdminCatalogConfig`：`IsNew`/`Featured` 由派生改为**可写持久化字段**；保留 `FeaturedUntil`（可选）；`FirstSeenAt` 只读；`Tags` 仍派生。
- `ModelCatalogDisplay`（内部 DTO）：新增 `IsNew`/`Featured`。
- `classifyDisplay(now, cfg)`：返回 `(cfg.IsNew, cfg.Featured && (cfg.FeaturedUntil==nil ‖ !now.After(*cfg.FeaturedUntil)))`；**移除 `newModelDays` 参数与时间窗逻辑**。
- `mergeDisplay`：同步去掉 `newModelDays` 形参；Tags 合并里 `new`/`featured` 按上述布尔注入（保留展示一致性）。
- `displayToAdminConfig`：`IsNew`/`Featured` 直接取行值（叠加 featured_until 到期）。
- `BatchSave`：把 `IsNew`/`Featured` 写入 `ModelCatalogDisplay`。
- `MergeDisplayConfig`：不再读 `new_model_days`（可从 `modelCatalogSettings` 接口移除 `GetModelCatalogNewModelDays`，或保留方法但不再调用）。

### 5.2 repository + adapter
- `model_catalog_repo.go`：`BatchUpsert` 的 upsert 集合增加 `is_new`/`featured`；DTO 映射新增两字段。
- `model_catalog_service_adapter.go`：repo DTO ↔ service DTO 互转补两字段。

### 5.3 handler
- `public_model_catalog_handler.go`：`mergeCatalog` 透传 `Display.IsNew/Featured` 即可，改动很小。
- `admin_model_catalog_handler.go`：无结构变化（透传）。

### 5.4 interface 变更（CLAUDE.md 强制）
- `classifyDisplay`/`mergeDisplay` 签名变更后，搜索 `type.*Stub.*struct` / `type.*Mock.*struct` 补全所有实现与测试。

## 6. 前端改动

### 6.1 admin 页（`views/admin/CatalogManageView.vue`）
- 移除顶部 `new_model_days` 设置条及相关状态/函数（`newModelDays`/`saveNewModelDays`/`cachedSettings`/`getSettings` 调用）。
- 移除独立"置顶区"拖拽块，合并为**单一可拖拽列表**（`VueDraggable`），展示全部模型，初始排序 `(pinned desc, sort_weight desc)`。
- 每行控件：拖拽手柄 + 模型名/badges + `pin` 开关 + **`NEW` 开关（写 `item.is_new`）** + **`featured` 开关（写 `item.featured`，不再写 custom_tags）** + `recommended` 开关（仍写 custom_tags） + 可选 `featured_until` 日期 + `hidden` 开关 + 只读自动 tags。
- 拖拽结束：按可见位置重写所有 `sort_weight`，置 dirty（不自动保存）。
- 统一由"保存全部"按钮 `saveCatalogConfig(items.value)` 落库。

### 6.2 admin API 类型（`api/adminCatalog.ts`）
- `CatalogConfigItem`：`is_new`/`featured` 改为可写布尔；保留 `featured_until`（可选）；`first_seen_at` 只读。

### 6.3 公开页筛选修复（`views/public/ModelCatalogView.vue`）
- `const filters = reactive(...)` → `const filters = ref<CatalogFilters>({...})`。
- `filteredModels = computed(() => filterModelCatalog(catalog.value.items, filters.value))`。
- `watch` 内 `filters.sortBy` → `filters.value.sortBy`。
- 模板 `v-model:filters="filters"` 不变（ref 的 v-model setter 正确赋值 `.value`）。
- 子组件 `ModelCatalogFilters.vue` **无需改动**。

### 6.4 公开页排序（`utils/modelCatalog.ts`）
- `compareCatalogCards`：非置顶模型排序改为 `sort_weight 降序 → tagScore → 价格`（置顶仍优先，内置顶区间按 sort_weight）。`sort_weight=0` 自然回退 tagScore，向后兼容。

### 6.5 公开页卡片（`components/models/ModelCard.vue`）
- **无需改动**：badge 本就认 `model.is_new`/`model.featured`；后端修好派生源即生效。

### 6.6 i18n（`i18n/locales/{zh,en}/custom.ts`）
- 新增 key：NEW 开关文案、统一排序提示、可选到期说明等；移除/弃用 `newModelDays` 相关 key（按 [[i18n-audit-and-custom-ts-fix]] 惯例在 custom.ts 深合并补缺，不改 main）。

## 7. 测试

- **后端 unit**：`classifyDisplay`（纯 bool + featured_until 到期边界）、`mergeDisplay`、`BatchSave` 字段映射、`MergeDisplayConfig` 不再依赖 newModelDays。
- **前端**：`utils/__tests__/modelCatalog.spec.ts` 增 `compareCatalogCards` 的 sort_weight 用例（置顶优先、sort_weight 降序、0 回退 tagScore）。
- 更新引用 `featured_until`/`newModelDays` 的现有 stub/mock 与测试断言。

## 8. 不在范围内

- 孤儿行（display 表有、活跃渠道无）清理 —— 差异为设计使然，仅靠隐藏标签明示。
- 套餐区块显隐（需求 4）—— 已由 `v-if` 守卫，跳过。
- recommended 机制迁移 —— 维持 custom_tags。
