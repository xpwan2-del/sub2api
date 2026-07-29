# Design: 模型广场可运营排序（置顶 + 标签权重 + 隐藏）

日期: 2026-07-29
分支: 待开 `feat/model-catalog-ops`（当前在 `feat/models-price`）
状态: 待用户审阅

## 1. 背景与目标

模型广场（公开页 `/models`，组件 `frontend/src/views/public/ModelCatalogView.vue`）当前排序**完全硬编码、无任何可配置项**：

- 数据从「渠道 + 渠道定价」实时推导、按 `(platform, model_name)` 去重，**模型不是独立实体**。
- 后端给稳定基线（Provider→平台→名字，`public_model_catalog_handler.go:233`），前端默认**按价格升序**覆盖（`utils/modelCatalog.ts:312`、`ModelCatalogView.vue:83` 默认 `sortBy:'price'`）。
- **没有任何后台排序入口**：无管理页、无拖拽、无排序权重字段。已有 6 个**自动推断**的能力标签（`reasoning/coding/longContext/lowCost/multimodal/fast`，`modelCatalog.ts:257`），但只能筛选、不能当排序权重；也没有「新模型 / 特色」这类可运营标签；广场页不展示套餐。

**目标**：插入一层「模型展示配置」，让运营能对每个去重后的模型卡片单独控制 **置顶 / 标签 / 隐藏**，把广场排序主轴从「价格」换成「运营意图」。具体支持：

- 新出的模型 → 自动或手动置顶/靠前；
- 多模态、有特点的模型 → 靠标签权重靠前；
- 任意模型 → 可隐藏不展示、可设特色并带过期。

**非目标**（明确排除，本期不做）：
- **套餐展示仅做顶部独立套餐区**：不做「模型卡内嵌可用套餐标签」（需模型↔套餐 glob 匹配推导，技术复杂且易泄密，YAGNI；见 §15）。
- **不改渠道/定价表结构**：排序配置落在去重后的「模型卡片」层，不污染渠道表。
- **不动计费链路**：本功能只影响公开展示顺序，与请求路由/计费完全解耦。

## 2. 需求决策（已与用户确认）

| 维度 | 决策 |
|---|---|
| 范围 | 仅「模型可运营排序」子系统；套餐展示另议 |
| 控制方式 | **混合**：运营标签权重 + 手动置顶 |
| 标签来源 | **自动推断 + 手动补充**：复用 6 个能力标签（含 multimodal）+ 新增「新模型」(按时间自动) + 「特色/推荐」(手动) + 管理员可覆盖/补充 |
| 排序链 | 置顶区 → 标签得分**累加** → 价格升序兜底；默认键 `price`→`recommended`，保留 price/name/provider 切换 |
| 扩展能力 | 全部要：**隐藏模型** + **特色有效期(featured_until)** + **新模型窗口可配** |
| 数据层 | **ent ORM**（新建 `model_catalog_display` schema + 手写迁移） |
| 后台入口 | admin 侧边栏**独立页** `/admin/catalog` |
| 套餐展示 | **顶部独立套餐区**，复用套餐卡，按 `sort_order` 排，不加 featured |
| 套餐位置 | 广场**顶部** |
| 功能开关 | `model_catalog.ops_enabled` 同时管控排序 + 套餐区（默认开） |

## 3. 架构与数据流

在现有「渠道 → 去重模型卡片」链路中间插入展示配置层：

```
渠道 + 渠道定价 (现有)
   ↓ ChannelService.ListAvailable (现有)
去重出模型卡片 (现有: handler buildPublicModelCatalog)
   ↓ 【新增】按 (platform, model_name) 批量读取 model_catalog_display 配置
   ↓ 【新增】merge 运营字段 + 实时计算 tag_score / recommended_rank
   ↓ 【新增】过滤 hidden=true
带运营字段的 catalog → GET /api/v1/public/models/catalog
   ↓ 前端 modelCatalog.ts
   ↓ 【改】默认排序键 price → recommended（置顶→tag_score→价格）
模型广场展示
```

**零配置即保持现状**：表建好但不配任何东西时，所有模型无配置 → 全部落入基础区按价格排，广场与现在一模一样。灰度安全。

**排序逻辑放前端**（沿用现有架构）：运营字段透传到 API 响应，前端在已有 `compareCatalogCards` 框架内加 `recommended` 分支，不破坏现有排序设计。

## 4. 数据层

### 4.1 Ent Schema（新增）
- 新建 `backend/ent/schema/model_catalog_display.go`：
  ```go
  // 业务键是 (platform, model_name) 复合；ent 保留默认自增 id + 加 unique 组合索引
  field.String("platform"),
  field.String("model_name"),
  field.Bool("pinned").Default(false),
  field.Int("sort_weight").Default(0),           // 置顶区内排序，越大越靠前
  field.JSON("custom_tags", []string{}).Default([]string{}), // 运营手动标签
  field.Time("featured_until").Optional().Nillable(),        // 特色过期；nil=不过期
  field.Bool("hidden").Default(false),
  field.Time("first_seen_at").Default(time.Now),             // 首次进广场时间(新模型判定)
  // timestamps（按项目惯例 created_at/updated_at）
  ```
  - Index：`unique` 组合索引 `(platform, model_name)`（业务查询键）。
- 改后执行 `go generate ./ent`，产出 `ent/modelcatalogdisplay*.go`、`ent/mutation.go`、`ent/migrate/schema.go` 变体。

### 4.2 数据库迁移（新增）
- 新建 `backend/migrations/176_create_model_catalog_display.sql`：
  ```sql
  SET LOCAL lock_timeout = '5s';
  SET LOCAL statement_timeout = '10min';
  CREATE TABLE IF NOT EXISTS model_catalog_display (
      id              BIGSERIAL    PRIMARY KEY,
      platform        VARCHAR(64)  NOT NULL,
      model_name      VARCHAR(128) NOT NULL,
      pinned          BOOLEAN      NOT NULL DEFAULT FALSE,
      sort_weight     INTEGER      NOT NULL DEFAULT 0,
      custom_tags     JSONB        NOT NULL DEFAULT '[]',
      featured_until  TIMESTAMPTZ,
      hidden          BOOLEAN      NOT NULL DEFAULT FALSE,
      first_seen_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
      created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
      updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
      UNIQUE (platform, model_name)
  );
  COMMENT ON TABLE model_catalog_display IS '模型广场展示配置（置顶/标签/隐藏），按平台+模型名';
  ```
- 幂等（IF NOT EXISTS）。验证：起一次性 `postgres:18-alpine` 容器、预建 `schema_migrations`、从零按序跑全部迁移（CLAUDE.md 要求）。

## 5. 标签体系

三类标签，UI 上视觉区分（自动=只读灰 chip，手动=可编辑）：

| 标签 | 来源 | 权重(代码常量) |
|---|---|---|
| `new`（新模型） | 自动：`now() - first_seen_at < new_model_days` | 100 |
| `featured`（特色） | 手动（`custom_tags`），可带 `featured_until` 过期 | 80 |
| `recommended`（推荐） | 手动（`custom_tags`） | 60 |
| `multimodal` | 自动（复用现有关键词推断） | 40 |
| `reasoning` | 自动（复用） | 20 |
| `coding` | 自动（复用） | 15 |
| `longContext` | 自动（复用） | 10 |
| `fast` / `lowCost` | 自动（复用） | 5 / 5 |

- **标签权重写死为前端 `modelCatalog.ts` 常量**（排序逻辑在前端，见 §6；YAGNI）；仅「新模型窗口」`new_model_days` 放系统设置可配（用户明确要求）。**后端只透传 tags 数组，不算分**；权重常量表里没有的标签（如管理员自定义的「限时免费」）权重为 0，仅展示、不参与排序。
- **featured 过期**：查询时实时判定 `featured_until != nil && now() > featured_until` → 该模型本轮不算 featured、不参与 featured 权重（不改库）。
- **管理员手动覆盖**：`custom_tags` 可任意增删；自动标签不可改，但管理员可加自定义标签（如"限时免费"等，需同步在权重常量表登记权重，未登记的标签权重为 0）。

## 6. 排序规则（recommended 键精确定义）

前端 `compareCatalogCards` 新增 `recommended` 分支，比较函数：

1. **hidden 过滤**：`hidden=true` 的卡片在 handler 层直接不返回（不到前端）。
2. **pinned 优先**：`pinned=true` 进置顶区，区内按 `sort_weight` 降序。
3. **tag_score 降序**：剩余卡片按有效标签权重**累加**降序（自动 + 手动标签都计入；featured 过期则不计 featured 权重）。
4. **价格升序兜底**：tag_score 并列 → `modelPriceScore`（input+output+cache 求和）升序，无定价 = `Infinity` 垫底。
5. **名字兜底**：价格也并列 → `name.localeCompare`。
- 默认 `sortBy` 从 `price` 改为 `recommended`；排序下拉新增「推荐」选项，保留「价格 / 名字 / 厂商」。

## 7. 后端改动清单

### 7.1 Repository（新增）
- `backend/internal/repository/model_catalog_repo.go`（ent client 操作）：
  - `GetByModelKeys(ctx, keys [][2]string) (map[string]*ModelCatalogDisplay, error)`：批量按 `(platform, model_name)` 取配置。
  - `UpsertMissing(ctx, models []ModelKey) error`：管理页加载时，给缺失的模型插 `first_seen_at=now()`（只插缺失，不覆盖已有）。
  - `ListAll(ctx)`：管理页列表。
  - `BatchUpsert(ctx, configs []ModelCatalogDisplay)`：管理页保存（置顶/标签/隐藏/有效期）。
- 遵循现有 repository 模式（参考 `channel_repo.go` 的事务/错误处理风格）。

### 7.2 Service
- 新增 `backend/internal/service/model_catalog_service.go`：
  - `MergeDisplayConfig(items []PublicModelCatalogItem) ([]PublicModelCatalogItem, error)`：批量取配置 → merge 字段（pinned/sort_weight/tags/is_new/featured）→ 过滤 hidden → 判定 new/featured 过期。**不算 tag_score**（权重常量在前端，排序在前端做）。
  - `EnsureFirstSeen(ctx)`：调用 `UpsertMissing`（管理页触发）。
  - 系统设置读取 `model_catalog.new_model_days`（默认 30）。
- Wire DI：`backend/cmd/server/wire.go` 加 Provider，重新生成 `wire_gen.go`。

### 7.3 Handler
- `backend/internal/handler/public_model_catalog_handler.go`：
  - `buildPublicModelCatalog` 去 `hidden=true`（MergeDisplayConfig 内过滤）。
  - `PublicModelCatalogItem` 响应结构加字段：`pinned bool`、`sort_weight int`、`tags []string`（合并自动+手动）、`is_new bool`、`featured bool`。（排序由前端按权重常量计算，无需后端给 rank。）
- 新增 `backend/internal/handler/admin_model_catalog_handler.go`：
  - `GET /api/v1/admin/catalog/config`：返回所有模型卡片 + 其展示配置（调用 EnsureFirstSeen 先填充）。
  - `PUT /api/v1/admin/catalog/config`：批量保存（置顶/标签/有效期/隐藏）。**列表端点空结果返回 `[]`**（CLAUDE.md 规范）。
- 路由注册：`backend/internal/server/routes/` 下 public 与 admin 各加一条。

### 7.4 Interface / Mock 同步（CLAUDE.md 要求）
- 新增 repository/service interface 方法后，搜索 `type.*Stub.*struct` / `type.*Mock.*struct` 补全所有测试 stub，否则编译失败。

## 8. 前端改动清单

- `frontend/src/utils/modelCatalog.ts`：
  - `compareCatalogCards` 加 `recommended` 分支（置顶→tag_score 累加→价格→名字）。
  - `buildModelCatalog` 排序选项数组前置加 `'recommended'`。
- `frontend/src/views/public/ModelCatalogView.vue`：默认 `sortBy: 'recommended'`；下拉 i18n 加「推荐」。
- `frontend/src/components/models/ModelCard.vue`：渲染运营标签 badge（`NEW` / `特色` / `推荐`）+ 置顶角标；自动标签 vs 手动标签视觉区分。
- 新增 `frontend/src/views/admin/CatalogManageView.vue`（管理页，见 §9）。
- 新增 `frontend/src/api/adminCatalog.ts`：`getConfig` / `saveConfig`。
- `frontend/src/router/index.ts`：admin 路由加 `/admin/catalog`，侧边栏菜单项 + 权限。
- `frontend/src/i18n/locales/{zh,en}/custom.ts`：新增 key（按项目 i18n 惯例在 custom.ts 深合并补缺，不改 main）。

## 9. 后台管理页交互（`/admin/catalog`）

- **置顶区**（上方）：`pinned=true` 的模型卡片，**可拖拽排序**；拖拽即批量重排 `sort_weight`（置顶区内赋 100/99/98…，简单无冲突）。
- **全部模型列表**（下方）：所有去重模型，每个卡片/行可：
  - 切换「置顶」（加入/移出置顶区）；
  - 勾选运营标签（`特色` / `推荐` → `custom_tags`）；
  - 设「特色有效期」（日期选择器 → `featured_until`）；
  - 切换「隐藏」（`hidden`）；
  - 只读展示自动标签（`new` / `multimodal` / `reasoning`…，灰 chip，不可编辑）。
- **顶部设置条**：「新模型窗口」`new_model_days` 数字输入 + 保存（写系统设置）。
- **页面加载**：先调 `EnsureFirstSeen`（给新模型填 `first_seen_at`），再拉配置列表。

## 10. API 一览

| 端点 | 方法 | 用途 |
|---|---|---|
| `/api/v1/public/models/catalog` | GET | 现有，响应加运营字段，过滤 hidden |
| `/api/v1/admin/catalog/config` | GET | 管理页：列表 + 触发 EnsureFirstSeen |
| `/api/v1/admin/catalog/config` | PUT | 管理页：批量保存展示配置 |
| `/api/v1/admin/settings`（现有） | PUT | 保存 `model_catalog.new_model_days` |
| `/api/v1/public/bundles/plans` | GET | **新增**：无鉴权公开套餐列表（裁剪敏感字段，见 §15） |

## 11. 系统设置

- `setting_service` 加键 `model_catalog.new_model_days`（int，默认 30）。
- 标签权重为代码常量（不进设置）；如未来需运营调权重，再扩为 `model_catalog.tag_weights` JSON。

## 12. 错误处理

- public catalog 读取展示配置失败 → **降级**：记日志，按无配置处理（全部落基础区按价格），不阻断广场展示（展示是公网核心，配置层故障不应让广场挂掉）。
- `EnsureFirstSeen` 失败 → 管理页提示「首次发现时间刷新失败，不影响配置编辑」，不阻塞页面。
- `BatchUpsert` 用事务，部分失败整体回滚。

## 13. 测试策略

- **后端单元**（`-tags=unit`）：
  - `MergeDisplayConfig`：merge 正确性、tag_score 累加、hidden 过滤、featured 过期（构造 `featured_until` 在过去）、new 时间判定（构造 `first_seen_at` + 不同 `new_model_days`）。
  - 排序顺序：置顶 > tag_score > 价格 > 名字 的多组用例。
- **后端集成**（`-tags=integration`，Postgres）：`UpsertMissing` 只插缺失不覆盖、`BatchUpsert` 事务、unique 键冲突。
- **前端 vitest**：`compareCatalogCards('recommended')` 排序顺序正确性。
- **迁移验证**：postgres 容器从零跑全部迁移至 176。
- **类型/lint**：`golangci-lint run ./...`、`pnpm typecheck`、`pnpm lint:check`。

## 14. CLAUDE.md 合规检查清单

- [ ] ent schema 改后 `go generate ./ent` + 提交生成代码
- [ ] 手写迁移 `176_*.sql`，幂等，文件头 `SET LOCAL lock_timeout/statement_timeout`
- [ ] Wire DI 新 service 加 Provider + 重新生成 `wire_gen.go`
- [ ] 新增 interface 方法 → 补全所有测试 Stub/Mock
- [ ] 列表端点空结果返回 `[]`（非 null）
- [ ] handler 不 import repository/gorm/redis（depguard）
- [ ] 前端用 pnpm；i18n 在 custom.ts 补缺

## 15. 套餐展示子系统（顶部独立套餐区）

用户在套餐管理新建的套餐，在模型广场**顶部**以独立套餐区展示，引导购买。

### 15.1 数据来源（复用，不加字段）
- 复用 `bundle_plan_service.ListForSale`（`status=active AND for_sale=true`，按 `sort_order` 升序）。
- **不给 bundle_plan 加字段**（用户确认不要 featured/推荐位，排序复用 sort_order）。
- 套餐不直接绑模型（绑 Group + `model_pattern` glob），故套餐卡只展示「覆盖平台」粒度，不给精确模型清单（避免 glob 推导的复杂度与泄密）。

### 15.2 公开套餐接口（新增——最大缺口）
现有 `/bundles/*` 全在 `requireAuth` 之后，而模型广场是无登录公开页。新增无鉴权端点：
- `GET /api/v1/public/bundles/plans` → `PublicBundlePlanHandler.List`，注册到 `routes/public_models.go` 的 public 路由组。
- **DTO 裁剪敏感字段**（参照 `PublicModelCatalogHandler` 的 narrow DTO 原则）：
  - 保留：`name / tier / description / price / original_price / currency / validity_days / features / sort_order`
  - 新增「覆盖平台」聚合：从 `group_quotas` 推导出去重的 platform 列表（如 `[openai, anthropic, gemini]`）作 chip 展示
  - **去掉**：`group_id / group_name / 精确额度(*_limit_usd/*_limit_count) / concurrency_limit / rpm_limit`（内部细节不公开）
- 复用 `ListForSale` 查询，handler 层裁剪 + platform 聚合。

### 15.3 前端
- `ModelCatalogView.vue` 顶部新增套餐区组件 `BundleShowcaseSection`。
- 套餐卡抽公共组件 `BundlePlanCard`（供广场 + `/bundles` 共用）：tier 主题色、price + 划线价 + 折扣、features 打勾、覆盖平台 chips。
- 卡片点击 → 跳 `/bundles`（未登录由 router `requireAuth` 拦截 → 登录后回跳）。
- 数据加载：`ModelCatalogView` 并行 `getCatalog()` + `getPublicBundlePlans()`。
- i18n + 类型（`frontend/src/types/bundle.ts` 加公开 DTO 类型）。

### 15.4 开关联动
`model_catalog.ops_enabled=false` 时，套餐区**也不显示**（与排序一起回滚到纯模型广场）。

### 15.5 测试
- public bundle 接口：只返回 active+for_sale、字段裁剪正确（无 group_id/额度）、按 sort_order、platform 聚合正确、空结果返回 `[]`。
- 前端 vitest：套餐区渲染、点击跳转。

## 16. 风险与回滚

- **风险**：模型去重逻辑改动可能导致现有展示顺序细微变化。**缓解**：零配置保持现状（§3），灰度先不配任何模型验证一致性。
- **回滚**：功能开关——加系统设置 `model_catalog.ops_enabled`（默认 true），关闭后前端退回 `price` 默认 + **套餐区隐藏**、后端跳过 merge，等同回滚。或直接还原迁移 176（DROP TABLE，表内无生产数据依赖）。
