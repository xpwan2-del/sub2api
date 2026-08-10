# 设计:同步上游模型价格(new-api 上游)

- **日期**: 2026-08-10
- **分支**: feat/sync-upstream-model-price
- **状态**: 已与用户确认方向与决策,待写实现计划
- **节奏**: 分两阶段(P1 主体 = 本规格;P2 = roadmap)

---

## 1. 背景与目标

本平台(sub2api)当前直接对接官方 API(Anthropic/OpenAI/Gemini/xAI)。运营上还会在**上游 new-api 中转平台**注册账号、充值,并以「上游的分组和模型」作为定价参考与(未来)转发来源。多个上游平台的模型价格/分组倍率各不相同,且会随上游调整而变化。

**问题**: 目前没有机制感知上游价格变化。上游调价后,本平台 `Channel`/`Group` 的定价可能滞后,导致成本与定价脱节。

**目标(本规格 P1)**: 针对「以 new-api 为上游」的数据源账号,提供**手动触发**的模型价格同步:从上游拉取定价 → 与本平台目标渠道现有定价做 diff → 生成**逐条审批单** → 管理员可查看上游原始值/还原值/本平台当前值,并可**修改/批准/忽略** → 批准后自动写入 `channel_model_pricing`。全程留痕、可审计、可回滚(通过渠道定价历史)。

**长期目标(混合模式)**: new-api 上游未来也会作为请求转发目标(复用同一 `Account`,本次不做转发)。本设计与之兼容。

## 2. 现状分析(关键事实)

### 2.1 sub2api 定价结构(已核对源码)
- 计费口径: `渠道模型单价(USD/token) × Group.rate_multiplier`(`billing_service.go:1079` `actualCost := totalCost * input.RateMultiplier`)。
- 渠道定价: `ChannelModelPricing`(`channel.go:86-101`),字段为 **USD 绝对单价**(`InputPrice`/`OutputPrice`/`CacheWritePrice`/`CacheReadPrice` per-token、`PerRequestPrice` 按次 USD),计费模式 `BillingMode`(token/per_request/image/video/per_second)。
- 分组倍率: `Group.rate_multiplier`(decimal,`group.go:45`)。
- 渠道定价存储: 原生 SQL 表 `channel_model_pricing` + `channel_pricing_intervals`(**不经 ent**),仓库 `channel_repo.go`,服务 `channel_service.go`。
- 模型广场: `model_catalog_display` 表,基于渠道定价展示。

### 2.2 new-api 接口事实(源码核对)
- **模型定价 + 分组倍率**: 
  - `GET /api/ratio_config`(需上游开 `expose_ratio_enabled`,匿名):返回原始完整 `ModelRatio`/`CompletionRatio`/`CacheRatio`/`CreateCacheRatio`/`ModelPrice`/`GroupRatio` map。**首选,解析最简**。
  - `GET /api/pricing`(默认匿名公开,可被上游关停/强制登录):返回产品化的 `data[]`(含 `model_ratio`/`completion_ratio`/`cache_ratio`/`create_cache_ratio`/`model_price`/`quota_type`/`enable_groups`)+ 全局 `group_ratio`/`usable_group`。**默认兜底**。
  - `GET /api/option/`(需 RootAuth):全量原始倍率(JSON 字符串 option)。P1 可不实现。
- **余额(P2)**: `GET /api/user/self`(需 **dashboard access token / 登录态**,**`sk-xxx` 不可用**)→ `quota`(剩余,quota 点)/`used_quota`。`QuotaPerUnit=500000` ⇒ `500000 quota = $1`,即基准 `$0.002/1K tokens`。
- **模型清单**: `GET /v1/models`(`sk-xxx`,OpenAI 兼容),不含价格。
- new-api 自带「从上游同步倍率」`/api/ratio_sync/*`(RootAuth),设计可借鉴。

### 2.3 口径关系(关键)
两边定价**结构一致,都是「模型价 × 分组倍率」**:
- new-api: `model_ratio × group_ratio × 基准(0.002/1K)`
- sub2api: `渠道USD单价 × rate_multiplier`

⇒ **分组倍率层两边都是无量纲倍率,直接同步**(P2:`group_ratio → Group.rate_multiplier`)。
⇒ **模型价层是同一个 USD 价的两种写法**:new-api `model_ratio`(相对 0.002/1K 的倍率)与 sub2api `InputPrice`(USD/token 绝对值)。同步时做一步确定性还原即可,**不是两套互不兼容口径**。按次价(`model_price`,USD/次)直接传。

### 2.4 可复用模式(已核对源码)
- **CRS Sync 三段式**(`crs_sync_service.go`):`fetchExport → Preview(纯读 diff)→ Apply(逐条隔离 upsert)`;纯手动、无定时;`httpclient.GetClient` + `urlvalidator` 白名单(仿 `CRSHosts`)。
- **周期任务**(`usage_cleanup_service.go`):`timing_wheel.ScheduleRecurring` + Wire `ProvideXxx` + `provideCleanup` + Config 四步。⚠️ `timing_wheel` 单次延迟上限约 1h,P2 的 6h 周期需评估(可能拆分或换机制)。
- **余额告警**(`balance_notify_service.go`):邮件 + 穿越+每日 `ReminderKey` 去重;admin 告警走邮件(announcement 不支持 admin 定向)。P2 扩展。
- **admin 路由/handler 注册**: `admin.go` 加路由 + `AccountHandler`(或新 handler)加方法 + `handler/wire.go` 聚合。
- **迁移**: `migrations/NNN_xxx.sql`,幂等(IF NOT EXISTS),`//go:embed` 嵌入,启动自动执行。

## 3. 已确认决策

| 维度 | 决策 |
|---|---|
| 上游角色 | 混合模式(官方直连 + new-api 上游并存);**本次不含转发链路** |
| 上游账号定位 | 数据源账号(拉价格/倍率/余额用),`schedulable=false`,不进转发池 |
| 上游类型 | new-api(QuantumNous/new-api) |
| 模型价格同步 | 拉取 → 还原 USD → 写入目标渠道 `channel_model_pricing` |
| 分组倍率同步 | `group_ratio → Group.rate_multiplier`(P2) |
| 应用工作流 | **逐条审批单**:diff → 管理员改值/批准/忽略 → 批准后自动写入 → 全程留痕可审计 |
| 账户余额 | 账号详情展示 + 低于阈值告警(对齐 `balance_notify_service`)(P2) |
| 触发 | P1 手动;P2 加定时 |
| 架构 | 方案1:`Account(platform=newapi)` 做凭证载体 + 独立 `upstream_source_configs` 表 + 审批单双表 |
| 节奏 | 分两阶段:P1 价格同步审批;P2 分组倍率+余额+告警+定时 |

## 4. 非目标(out of scope)

- **请求转发到 new-api 上游**(混合模式的转发链路,未来另做,但 Account 建模为之预留)。
- 自动定时同步(P2)。
- 分组倍率同步、余额同步与展示、低额告警(P2)。
- 多上游同模型价格冲突的自动裁决:P1 每个「上游账号 → 目标 Channel」天然隔离,不做合并。

## 5. 架构与组件

```
[new-api 上游] ──HTTP──▶ UpstreamPricingClient ──▶ PricingSnapshot
                                                       │ (ratio_config 或 pricing)
              UpstreamPriceSyncService ◀───────────────┘
              ├─ Convert(ratio → USD 还原, base_price_per_1k)
              ├─ PlatformInfer(model_name → anthropic/openai/gemini/grok)
              ├─ Diff(snapshot vs 目标 Channel 现有 channel_model_pricing)
              └─ 建审批单(request + items, 全 pending) ──▶ 邮件提醒管理员(可选)
                                                       │
admin 审批页 ◀────────────────────────────────────────┘
  逐条: 改 apply_value / approve / reject / ignore  (支持批量)
   └─ ApplyItem ──▶ ChannelService upsert channel_model_pricing ──▶ item=applied
```

### 新增后端组件

**`UpstreamPricingClient`**(新文件 `internal/service/upstream_pricing_client.go`):
```go
type PricingSource string // "ratio_config" | "pricing" | "auto"
type UpstreamPricingClient interface {
    FetchPricing(ctx context.Context, baseURL string, source PricingSource) (*PricingSnapshot, error)
    // P2: FetchBalance(ctx, baseURL, dashboardToken) (*BalanceSnapshot, error)
}
type PricingSnapshot struct {
    Version     string                   // pricing_version(用于变更检测)
    Models      []UpstreamModelPricing
    GroupRatio  map[string]float64       // P2
    UsableGroup map[string]string        // P2
    FetchedAt   time.Time
    Source      string
}
type UpstreamModelPricing struct {
    ModelName        string
    ModelRatio       float64
    CompletionRatio  float64
    CacheRatio       *float64
    CreateCacheRatio *float64
    ModelPrice       *float64  // 按次 USD;<0 或 nil = 未启用按次
    QuotaType        int       // 0=ratio, 1=按次
    EnableGroups     []string
}
```
- 复用 `httpclient.GetClient` + `urlvalidator`(白名单取自新 config `URLAllowlist.NewAPIHosts`)。
- `source=auto`:先试 `/api/ratio_config`(检测 `success`/`expose_ratio_enabled`),失败回退 `/api/pricing`。
- 非强制认证:P1 价格走匿名接口,不依赖 dashboard token。

**`UpstreamPriceSyncService`**(新文件 `internal/service/upstream_price_sync_service.go`):
```go
type ReviewAction string // "apply" | "reject" | "ignore"
type UpstreamPriceSyncService interface {
    SyncNow(ctx, configID int64, trigger Trigger) (requestID int64, err error) // 拉→还原→diff→建单
    ListRequests(ctx, filter RequestFilter) ([]PriceChangeRequest, total, err error)
    GetRequest(ctx, requestID) (*PriceChangeRequest, items []PriceChangeItem, err error)
    ReviewItem(ctx, itemID int64, action ReviewAction, applyValue *ConvertedPrice, note string) error
    ReviewItems(ctx, itemIDs []int64, action ReviewAction, ...) (BatchResult, error) // 批量
    CloseRequest(ctx, requestID) error
}
```
- `ReviewItem(action=apply)`:校验 `apply_value`(默认=`upstream_converted`)→ 经 `ChannelService` upsert `channel_model_pricing` → 置 item `applied`;失败置 `failed`(隔离,不中断)。
- `SyncNow` 重复触发:同一 `source_config` 已有 `open` 审批单时,默认**新建本批次**并将旧 `open` 单标记为 `expired`(旧 items 保留留痕、不删除);接口返回新 `request_id` 并在响应中附「被取代的旧单」提示。

**admin handler**(新文件 `internal/handler/admin/upstream_price_sync_handler.go`):
- 路由(挂 `/admin`,需 admin 鉴权):
```
# 上游源配置
GET    /admin/upstream-sources                 列表
POST   /admin/upstream-sources                 创建
GET    /admin/upstream-sources/:id             
PUT    /admin/upstream-sources/:id             
DELETE /admin/upstream-sources/:id             
POST   /admin/upstream-sources/:id/sync        立即同步 → {request_id}

# 审批单
GET    /admin/price-change-requests            列表(按 status/source 筛选)
GET    /admin/price-change-requests/:id        详情 + items
POST   /admin/price-change-requests/:id/items/:itemID/review   {action, apply_value, note}
POST   /admin/price-change-requests/:id/items/batch-review    {item_ids[], action}
POST   /admin/price-change-requests/:id/close
```

### 前端(Vue3,新页面)
- **`UpstreamSourcesView.vue`**(admin):列表/新建/编辑 `upstream_source_config`(选关联的 newapi `Account`、目标 `Channel`、`base_price_per_1k`、`pricing_source`、`sync_model_price` 开关);每行「立即同步」按钮 → 跳转/刷新审批单。
- **`PriceChangeRequestsView.vue`**(admin):审批单列表;详情页逐条 items 表:模型 / platform / 上游原始值 / 还原 USD / 本平台当前值 / **可编辑 apply_value** / 操作(approve·reject·ignore)+ 批量栏;状态计数。
- Account 详情/列表:展示 newapi 账号的 `last_sync_at`/`last_error`(P1);余额展示 P2。

## 6. 数据模型

### 6.1 Account 约定(无 schema 改动)
- `platform = "newapi"`、`schedulable = false`、`status = "active"`
- `credentials = { "base_url": "...", "api_key": "sk-xxx", "dashboard_token": "..." }`(`dashboard_token` P1 可空,P2 余额必需)
- Account 创建走现有流程;`AccountHandler` 允许 `platform=newapi`。
- config 新增 `URLAllowlist.NewAPIHosts []string`(白名单,仿 `CRSHosts`)。

### 6.2 新表(迁移文件,幂等)

**`upstream_source_configs`**(`NNN_create_upstream_source_configs.sql`):
| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | BIGSERIAL PK | |
| `account_id` | BIGINT NOT NULL UNIQUE | FK accounts(newapi 账号,一对一) |
| `target_channel_id` | BIGINT NOT NULL | FK channels(价格写入目标) |
| `enabled` | BOOL DEFAULT true | 总开关 |
| `base_price_per_1k` | NUMERIC(10,6) DEFAULT 0.002 | 还原基准 |
| `pricing_source` | TEXT DEFAULT 'auto' | auto/ratio_config/pricing |
| `sync_model_price` | BOOL DEFAULT true | P1 |
| `sync_group_ratio` | BOOL DEFAULT false | P2 |
| `group_mapping` | JSONB DEFAULT '{}' | P2:`{upstream_group_key: local_group_id}` |
| `balance_threshold_usd` | NUMERIC(12,4) | P2 |
| `last_sync_at` | TIMESTAMPTZ | |
| `last_pricing_version` | TEXT | |
| `last_error` | TEXT | |
| `created_at`/`updated_at` | TIMESTAMPTZ | |

**`upstream_price_change_requests`**:
| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | BIGSERIAL PK | |
| `source_config_id` | BIGINT NOT NULL | FK |
| `trigger` | TEXT NOT NULL | manual/scheduled |
| `status` | TEXT NOT NULL DEFAULT 'open' | open/partially_applied/closed/expired |
| `upstream_pricing_version` | TEXT | |
| `summary` | JSONB | `{pending,approved,rejected,ignored,applied,failed}` 计数 |
| `created_by` | BIGINT | admin user id |
| `created_at`/`closed_at` | TIMESTAMPTZ | |
| 索引 | `(source_config_id, status)`,`(created_at)` | |

**`upstream_price_change_items`**:
| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | BIGSERIAL PK | |
| `request_id` | BIGINT NOT NULL | FK(级联删) |
| `kind` | TEXT NOT NULL | model_price/model_added/model_removed(P2: group_ratio) |
| `platform` | TEXT | anthropic/openai/gemini/grok |
| `model_name` | TEXT | |
| `target_channel_id` | BIGINT | |
| `upstream_raw` | JSONB | 原始 ratio/completion_ratio/cache_ratio/create_cache_ratio/model_price/quota_type |
| `upstream_converted` | JSONB | 还原后 `{input,output,cache_read,cache_write,per_request}` USD |
| `local_current` | JSONB | 本平台当前值(同结构) |
| `apply_value` | JSONB | 管理员确认/修改值(默认=upstream_converted) |
| `status` | TEXT NOT NULL DEFAULT 'pending' | pending/approved/rejected/ignored/applied/failed |
| `reviewer_id` | BIGINT | |
| `review_note` | TEXT | |
| `reviewed_at`/`applied_at` | TIMESTAMPTZ | |
| `created_at` | TIMESTAMPTZ | |
| 索引 | `(request_id, status)` | |

> 三张表均为原生 SQL 表(与 `channel_model_pricing` 一致,**不经 ent**),仓库实现于 `internal/repository/upstream_price_sync_repo.go`(实现 service 层接口)。

## 7. 口径还原与 diff 规则

### 7.1 还原公式(一步乘法)
```
base = base_price_per_1k               // 默认 0.002
input        = model_ratio × base / 1000            // USD/token
output       = input × completion_ratio
cache_read   = input × cache_ratio                  // *float64,缺省 nil
cache_write  = input × create_cache_ratio           // *float64,缺省 nil
per_request  = model_price                          // USD/次,仅当 quota_type=1 且 model_price≥0
```
- **cache 语义说明**: `cache_ratio`/`create_cache_ratio` 假设为相对 prompt 基准的倍数(以 new-api 上游版本为准,实现时验证)。**审批单的可编辑性是口径不确定性的安全网**:管理员审批时可见上游原始值与还原值,可手动修正 `apply_value`。
- 按次模型(`quota_type=1`):生成 `BillingMode=per_request` 定价,`PerRequestPrice=per_request`。
- 按量模型(`quota_type=0`):生成 `BillingMode=token` 定价,填 `input/output/cache_*`。

### 7.2 平台推断(`PlatformInfer`)
按 model_name 前缀映射:`gpt-/chatgpt-/o1-/o3-/text-` → `openai`;`claude-` → `anthropic`;`gemini-` → `gemini`;`grok-` → `grok`。**未识别** → 标记 `platform=""`,在审批单中高亮「需人工指定 platform 或忽略」。P1 可在配置中限定只同步特定平台。

### 7.3 diff 算法
- 以 `(platform, model_name)` 为键,对比 `upstream_converted` 与目标渠道现有 `channel_model_pricing`。
- 数值容差 `1e-9`:任一字段超过容差 → `kind=model_price`,记录新旧值。
- 上游有、本地无 → `model_added`(默认 `apply_value=upstream_converted`,待审批新增)。
- 本地有、上游无 → `model_removed`(默认 `status=ignored`,**不自动删**,仅提示)。
- 无变化:不生成 item。

## 8. 核心数据流(P1,手动)

1. **配置**: admin 建 `Account(platform=newapi, schedulable=false, credentials)` → 建 `upstream_source_config`(绑定 `target_channel_id`)。
2. **同步**: 审批页/源配置页点「立即同步」→ `SyncNow(configID, manual)`:
   a. `UpstreamPricingClient.FetchPricing`(ratio_config/pricing)→ `PricingSnapshot`
   b. `Convert`(按 `base_price_per_1k` 还原)+ `PlatformInfer`
   c. `Diff` vs 目标渠道现有 `channel_model_pricing` → `[]ChangeItem`
   d. 建 `request`(open)+ `items`(全 pending);更新 `last_sync_at`/`last_pricing_version`/`last_error`
   e. 返回 `request_id`
3. **审批**: admin 在 `PriceChangeRequestsView` 查看 diff,逐条(或批量)`review`:
   - `apply`:可先改 `apply_value` → `ApplyItem` → `ChannelService` upsert `channel_model_pricing` → item `applied`
   - `reject`/`ignore`:仅置状态
4. **关闭**: 所有 items 到终态 → `request.status=closed`(或 admin 手动 `close`)。

## 9. 错误处理

- 上游不可达 / 认证失败 / 接口关停: `SyncNow` 返回明确错误,写 `last_error`,**不建空审批单**。
- 单 item apply 失败: 置 `failed` + 错误信息,不中断批次(抄 CRS 逐条隔离),`summary` 实时更新。
- `dashboard_token` 缺失: P1 不影响(价格走匿名接口);P2 余额降级跳过并提示。
- 并发: 同一 `source_config` 同时 `SyncNow` 加锁(防重复建单);`ReviewItem` 用乐观锁/行锁防重复应用。
- 应用幂等: `ApplyItem` 重复调用对已 `applied` 的 item 直接返回成功(不重复写)。

## 10. 测试策略

- **单元**:
  - 还原公式(含 completion/cache/quota_type 各分支、缺省 nil)
  - `PlatformInfer`(各前缀 + 未识别)
  - diff(数值容差 / model_added / model_removed / 无变化)
  - `ApplyItem` 幂等 + 状态机(pending→approved→applied;reject/ignore 终态)
  - `auto` 数据源降级(ratio_config 失败 → pricing)
- **集成**: `httptest` mock new-api(`/api/ratio_config`、`/api/pricing`),跑 配置→SyncNow→review→apply→channel_model_pricing 落库 全流程;覆盖错误路径。
- 复用现有测试布局(`*_test.go` 同包)。

## 11. 前置条件与约束

- new-api 上游需满足其一:① `/api/pricing` 默认公开(常见);② 开启 `expose_ratio_enabled`(走 `/api/ratio_config`,字段更全);③ 否则需 root token 走 `/api/option`(P1 非必须)。
- 余额同步(P2)需 `dashboard access token`(`sk-xxx` 无法查余额)。
- `base_price_per_1k` 默认 0.002(new-api `QuotaPerUnit=500000` 固定),分叉基准不同时可配。

## 12. P2 roadmap(后续阶段,本规格仅记录)

1. **分组倍率同步**: `group_ratio → Group.rate_multiplier`,经 `group_mapping`(`upstream_group_key → local_group_id`);审批单 `kind=group_ratio`。
2. **余额同步 + 展示 + 告警**: `FetchBalance`(`/api/user/self`,dashboard token)→ `upstream_source_configs.last_balance_*` 快照 → 账号详情展示;扩展 `BalanceNotifyService`(新 `NotificationEmailEvent` + `SettingKey` + 方法,复用 `NotificationEmailService.Send` + 每日 `ReminderKey` 去重 + 管理员邮件列表)。
3. **定时自动化**: `timing_wheel.ScheduleRecurring` 注册(⚠️ 评估 1h 延迟上限 vs 6h 周期);Config `UpstreamPriceSyncConfig{Enabled, IntervalSeconds}`。
4. **模型广场联动**: 审批应用后 `model_catalog_display` 自动可见;可选「上游新增模型」提醒。
5. **邮件提醒**: 审批单生成时通知管理员(复用 `NotificationEmailService`)。

## 13. 实现顺序建议(P1)

1. Config(`NewAPIHosts`)+ 迁移(3 表)
2. `UpstreamPricingClient`(含 `auto` 降级)+ 单元测试
3. 还原 + `PlatformInfer` + diff + 单元测试
4. `upstream_price_sync_repo.go` + `UpstreamPriceSyncService`(SyncNow/Review/Apply)
5. admin handler + 路由 + Wire 注入
6. 前端两个页面 + API client
7. 集成测试 + 手动联调(mock new-api)
