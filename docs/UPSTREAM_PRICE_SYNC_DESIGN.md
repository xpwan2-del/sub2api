# 上游价格同步与变动告警 — 设计文档

| 项 | 值 |
|---|---|
| 日期 | 2026-06-14 |
| 状态 | Draft（待复审） |
| 方案层次 | 方案 B（完整闭环）+ admin 站内通知 |
| 预计工作量 | ~7.5 人天 |

---

## 1. 背景与问题

Sub2API 作为 AI API 网关（中转站），其上游往往也是中转站而非官方源。上游中转站的报价**频繁变动**（今天一个价、明天一个价），而本系统的模型单价（`channel_model_pricing`）和倍率（`group.rate_multiplier`）目前完全依赖管理员**手工维护**。

现状缺口：
- 现有的远程价格同步（`PricingService`，`internal/service/pricing_service.go:157-281`）只拉取 **LiteLLM 行业公开目录价**（官方价），不是"实际接入的上游中转站报价"。
- 该同步**落地为文件、不入库、无 diff、无告警**（哈希变化时只打日志）。
- 系统没有任何调用上游 `/v1/models` 或读取上游定价的代码。
- 通知渠道目前只有邮件，无站内通知。

风险：管理员无法及时感知上游涨价 → 沿用旧成本核算 → 毛利被压缩甚至亏损。

## 2. 目标与非目标

### 目标
1. **自动拉取**上游中转站的定价接口，解析为标准 per-token 价格。
2. **检测变动**（涨价/降价/新增模型/下架），与本地现状对比。
3. **双通道告警**：邮件 + 站内通知（admin 专属）。
4. **建议值 + 一键应用**：系统预算两种调价策略，管理员点确认才写入计费链路（人工审计闸门）。
5. **历史可追溯**：保留参考价快照与变动记录。

### 非目标（二期，方案 C）
- 多上游自动比价 / 选最优路由。
- 毛利率护城河自动规则（上游涨超阈值自动提价，无人工）。
- webhook / Telegram 通知渠道。

## 3. 已对齐的核心决策

| 维度 | 决策 |
|---|---|
| 价格来源 | 上游有定价接口 → **定时自动拉取 + 解析适配器** |
| 价格落点 | **独立旁路参考价表**，不直接进计费链路；算出建议值，**管理员点确认才写库** |
| 告警范围 | **全部变动都告警**（配"一次同步聚合成一条 + 同模型短时重复限流"防刷屏；阈值可配，默认 0% = 全报） |
| 站内通知 | **新建 `admin_notifications` 表 + admin 专属铃铛**（不复用 announcement，避免内部成本信息泄露给终端用户） |

### 工程默认值
- 多上游支持（参考价按"上游×模型"维度存储）。
- 模型名映射：精确匹配优先 + 可配别名表（`model_alias_map`）。
- 同步频率：可配置，默认 6 小时（每源可单独覆盖）。
- 一键应用目标：支持「改 channel 单价」和「改 group 倍率」。
- 通知渠道：邮件（复用 `NotificationEmailService`）+ 站内（新建 admin 通知）。
- 接口格式适配：预置 one-api/new-api 系 `/api/pricing`、LiteLLM、自定义 JSONPath 三种解析器。

## 4. 价格体系认知（设计地基）

项目里"价格"是**两条正交的轴**：

- **轴 A — 模型单价**（per-token USD）：来自 `channel_model_pricing`（DB）或 LiteLLM 远程目录，决定**上游成本**。解析链 `ModelPricingResolver.Resolve`（`internal/service/model_pricing_resolver.go:66-107`）：Channel → LiteLLM → Fallback。
- **轴 B — 倍率**（`rate_multiplier`）：挂在 `group`（`ent/schema/group.go:45`）、`account`（`account.go:112`）、`user_group_rate` 上。最终 `ActualCost = TotalCost × rateMultiplier`（`billing_service.go:608`）。

本功能同步的对象是**轴 A 的上游成本单价**；管理员"手动调整"的对象是**轴 A 单价**或**轴 B 倍率**。Bundle 套餐只有配额（`group_quotas`）和购买价（`price`），**不在价格链路内**，本功能不涉及。

**关键边界**：参考价表完全独立于计费链路，`ModelPricingResolver` **不读**参考价表。只有管理员点"应用"后，数据才经 `ChannelService.ReplaceModelPricing` 或 group update 进入计费链路。

## 5. 整体数据流

```
┌─────────────────────────────────────────────────────────────────┐
│  定时调度层（复用 time.Timer + Redis SetNX leader lock 模式）      │
│  UpstreamPriceSyncService  ← 注册: service/wire.go + cmd/wire.go  │
│         │  每源独立间隔 (默认 6h)，分布式单实例                     │
└─────────┬───────────────────────────────────────────────────────┘
          ▼
① HTTP 拉取    GET source.base_url + pricing_endpoint (带 api_key)
               复用 httpUpstream.DoWithTLS 模式
          ▼
② Parser 适配器   one_api / new_api / litellm / custom(JSONPath)
                 原始 JSON → []UpstreamModelPrice（经 alias_map 映射本地名）
          ▼
③ DiffEngine   新快照 vs upstream_model_prices(上次)
               产出变动: price_up/down/new_model/removed + delta_pct
          ▼
④ 持久化       更新 upstream_model_prices（最新快照）
               插入 upstream_price_changes（变动 + 待应用清单）
          ▼
⑤ SuggestionCalculator  查本地 channel_model_pricing / group.rate_multiplier
                        算 suggested_price（跟随成本）+ suggested_multiplier（锁死售价）
          ▼
⑥ AlertAggregator   一次 Sync 全部变动聚合 →
                    1 条 AdminNotification + 1 封邮件（event=ops.price_change）
                    限流：source.last_hash 未变 / 在 cooldown 内 → 不重发
          ▼
⑦ 前端管理页   管理员看变动列表 + 建议值 → admin 铃铛红点提示
          ▼
⑧ ApplyService（人工确认闸门）
               follow_cost → ChannelService.ReplaceModelPricing（轴A单价）
               lock_price  → 改单价 + GroupService.UpdateRateMultiplier（轴B倍率）
               标记 change.applied + admin 审计日志
```

## 6. 数据模型（5 张 ent entity）

字段命名对齐现有 `channel_model_pricing`（`migrations/081_create_channels.sql:33-44`）：`input_price / output_price / cache_write_price / cache_read_price / image_output_price`，全部 per-token USD。

### 6.1 `UpstreamPriceSource` → `upstream_price_sources`（上游定价源配置）
| 字段 | 类型 | 说明 |
|---|---|---|
| `name` | string | 显示名，如 "上游A-某中转站" |
| `platform` | string | openai/anthropic/gemini/mixed |
| `base_url` | string | 如 `https://relay.xxx.com` |
| `pricing_endpoint` | string | 如 `/api/pricing` 或 `/v1/models` |
| `api_key` | string(加密) | 拉取定价用，可空 |
| `parser_type` | string | `one_api` / `new_api` / `litellm` / `custom` |
| `parser_config` | JSON | 自定义解析 JSONPath 规则 |
| `model_alias_map` | JSON | `{"claude-3-opus":"claude-opus-4-6"}` 别名映射 |
| `sync_interval_minutes` | int | 默认 360 |
| `alert_threshold_pct` | float64 | 告警阈值，默认 0（全报） |
| `cooldown_minutes` | int | 同源告警冷却，默认 60 |
| `enabled` | bool | |
| `last_sync_at` | time | |
| `last_sync_status` | string | `success` / `failed` / `running` |
| `last_sync_error` | string | |
| `last_hash` | string | 上次内容哈希，快速判变 |
| `created_at` / `updated_at` | time | |

### 6.2 `UpstreamModelPrice` → `upstream_model_prices`（最新参考价快照）
| 字段 | 类型 | 说明 |
|---|---|---|
| `source_id` | int(FK) | |
| `model_name` | string | 上游原始模型名 |
| `local_model_name` | string | 经 alias_map 映射后的本地名 |
| `input_price` / `output_price` | float64 | per-token USD（核心两字段） |
| `cache_write_price` / `cache_read_price` | *float64 | 可空 |
| `image_output_price` / `per_request_price` | *float64 | 可空 |
| `currency` | string | USD/CNY |
| `raw_payload` | JSON | 原始数据，审计用 |
| `fetched_at` | time | |
| **UNIQUE** | `(source_id, model_name)` | |

### 6.3 `UpstreamPriceChange` → `upstream_price_changes`（变动记录 + 待应用清单，身兼二职）
| 字段 | 类型 | 说明 |
|---|---|---|
| `source_id` | int(FK) | |
| `model_name` / `local_model_name` | string | |
| `change_type` | string | `price_up` / `price_down` / `new_model` / `removed` |
| `prev_input_price` / `prev_output_price` | *float64 | |
| `curr_input_price` / `curr_output_price` | float64 | |
| `input_delta_pct` / `output_delta_pct` | float64 | |
| `detected_at` | time | |
| `notified` | bool | 是否已通知 |
| `status` | string | `pending` / `applied` / `dismissed` |
| `suggested_input_price` / `suggested_output_price` | float64 | 建议值（跟随成本） |
| `suggested_multiplier` | *float64 | 建议倍率（锁死售价） |
| `applied_at` | *time | |
| `applied_by` | *int | admin user id |
| `applied_target` | string | `channel_pricing` / `group_multiplier` |
| `applied_target_id` | int | channel_id 或 group_id |

### 6.4 `AdminNotification` → `admin_notifications`（admin 站内通知）
| 字段 | 类型 | 说明 |
|---|---|---|
| `type` | string | `upstream_price_change` / `system`（预留） |
| `title` | string | 如 "上游A 价格变动：3 涨 1 跌" |
| `content` | text(Markdown) | 含变动明细表 |
| `severity` | string | `info` / `warning` / `critical`（>20% critical, >5% warning） |
| `target_link` | string | 点击跳转 `/admin/upstream-pricing/changes` |
| `related_ids` | JSON | 关联的 change_ids |
| `created_at` | time | |

### 6.5 `AdminNotificationRead` → `admin_notification_reads`（多 admin 独立已读，对齐 `announcement_read`）
| 字段 | 类型 | 说明 |
|---|---|---|
| `notification_id` | int(FK) | |
| `user_id` | int | admin user id |
| `read_at` | time | |
| **UNIQUE** | `(notification_id, user_id)` | |

## 7. 核心组件（后端）

### 7.1 新增文件
| 层 | 文件 | 职责 |
|---|---|---|
| ent schema | `upstream_price_source.go` / `upstream_model_price.go` / `upstream_price_change.go` / `admin_notification.go` / `admin_notification_read.go` | 5 张表 |
| repository | `upstream_price_repo.go` | source/model_price/change 三表 CRUD |
| repository | `admin_notification_repo.go` | 通知 + reads CRUD |
| service | `upstream_price_source_service.go` | 源配置 CRUD + 测试连接 |
| service | `upstream_price_sync_service.go` ⭐ | 定时调度 + 拉取 + diff + 持久化 + 触发告警 |
| service | `upstream_price_parser.go` | 解析适配器（interface + 3 实现） |
| service | `upstream_price_suggestion_service.go` | 建议值计算 |
| service | `upstream_price_apply_service.go` | 一键应用 + 审计 |
| service | `admin_notification_service.go` | admin 通知 CRUD + 未读 |
| handler | `admin/upstream_price_handler.go` | 源配置 / 变动列表 / 对比 / 应用 / 手动同步 |
| handler | `admin/admin_notification_handler.go` | 未读列表 / 标记已读 / 未读数 |

### 7.2 修改文件
| 文件 | 改动 |
|---|---|
| `internal/service/wire.go` | ProviderSet 加 `ProvideUpstreamPriceSyncService` 等 |
| `cmd/server/wire.go` | `provideCleanup` 加新服务 `Stop()` |
| `internal/server/routes/admin.go` | 注册新路由组 |
| `internal/service/notification_email_service.go` | 新增 event `NotificationEmailEventUpstreamPriceChange = "ops.price_change"` + 模板 |
| `internal/config/config.go` | 加 `upstream_price.default_interval_minutes` 默认值 |

## 8. 关键算法

### 8.1 Parser 适配器
```go
type PriceParser interface {
    Parse(ctx context.Context, raw []byte, cfg ParserConfig) ([]UpstreamModelPrice, error)
}
// 预置实现：
//   OneAPIParser   — new-api/one-api 系 /api/pricing 返回格式
//   LiteLLMParser  — LiteLLM model_prices JSON 格式
//   CustomJSONPathParser — 用户配 JSONPath 抽取（最大灵活性）
// 每个 price 经 source.model_alias_map 映射到 local_model_name
```

### 8.2 DiffEngine（处理浮点抖动）
```
输入: newPrices map[model]Price, oldPrices map[model]Price
对 newPrices 每个 model:
    old, ok := oldPrices[model]
    !ok                              → new_model (prev=nil, curr=new)
    |curr-old| > 1e-9 且 curr>old    → price_up,   delta=(curr-old)/old
    |curr-old| > 1e-9 且 curr<old    → price_down, delta=(curr-old)/old
对 oldPrices 中不在 newPrices 的      → removed
```

### 8.3 SuggestionCalculator（两种调价策略，预算好让管理员选）

售价 ∝ 单价 × 倍率。上游成本从 `oldCost` 变为 `newCost`：

| 策略 | 含义 | 计算 | 改动对象 |
|---|---|---|---|
| **跟随成本** `follow_cost` | 维持毛利率%，售价随成本自然浮动 | `suggested_price = 上游最新价`，倍率不动 | 仅 channel 单价（轴A） |
| **锁死售价** `lock_price` | 维持对用户售价不变，毛利额被压缩/扩大 | `suggested_multiplier = 旧倍率 × (oldCost/newCost)`，单价同步更新为上游价 | channel 单价 + group 倍率 |

> 锁死售价数学验证：售价不变 ⟺ `新单价×新倍率 = 旧单价×旧倍率` ⟺ `新倍率 = 旧倍率 × (旧单价/新单价) = 旧倍率 × (oldCost/newCost)`。成本涨 → 新倍率降（毛利压缩）。

### 8.4 AlertAggregator（聚合 + 限流）
```
一次 Sync 的全部 changes 聚合：
  severity = max(各 change 涨幅): >20% → critical, >5% → warning, else info
  → 创建 1 条 AdminNotification(content=变动明细 Markdown 表)
  → 发 1 封聚合邮件(复用 NotificationEmailService, event=ops.price_change)
限流: source.last_hash 未变 或 在 cooldown_minutes 内 → 跳过
```

### 8.5 ApplyService（人工审计闸门）
```
Apply(change_id, mode, target_id):
    校验 change.status == pending
    follow_cost:
        ChannelService.ReplaceModelPricing(target_id, 上游价)        // 轴A
    lock_price:
        ChannelService.ReplaceModelPricing(target_id, 上游价)        // 先更新成本
        GroupService.UpdateRateMultiplier(target_id, suggested_multiplier)  // 轴B反推
    事务内: change.status=applied, applied_by/at/target
    adminAudit.Log(...)   // 复用现有 admin 操作审计
支持批量应用（一次处理一批 change）
```
> target 由管理员在 UI 上选定（该 model 涉及的 channel / group），前端列出所有受影响项。

## 9. 前端

| 文件 | 职责 |
|---|---|
| `src/views/admin/upstream-pricing/UpstreamSourcesView.vue` | 源配置 CRUD + 测试连接 + 手动触发同步 |
| `src/views/admin/upstream-pricing/UpstreamPriceChangesView.vue` | 变动列表（筛选 source/状态/类型）+ 建议值 + 一键/批量应用 + 忽略 |
| `src/views/admin/upstream-pricing/UpstreamPriceCompareView.vue` | 参考价 vs 本地价对比表 |
| `src/components/admin/AdminNotificationBell.vue` | 参考 `AnnouncementBell.vue` 改数据源，仅 admin 布局显示 |
| `src/api/admin/upstreamPricing.ts` | API 客户端 |
| `src/stores/adminNotifications.ts` | Pinia store |
| 路由 + 侧边栏菜单 | 注册入口 |

## 10. 配置 / 错误处理 / 可观测性

- **配置**：`upstream_price.default_interval_minutes`（默认 360）；每源可单独覆盖 `sync_interval_minutes`。
- **HTTP/解析失败**：记 `last_sync_status=failed` + `last_sync_error`，不产生变动，下次重试；连续失败超阈值时发一条"同步异常"admin 通知。
- **应用失败**：事务回滚，change 保持 `pending`。
- **并发**：Redis leader lock 保证多实例下单实例运行（仿 `ops_alert_evaluator_service.go:906-943`）。
- **心跳**：复用 `OpsRepository.UpsertJobHeartbeat` 记录同步任务状态，可观测。
- **审计**：所有 Apply 操作写 admin 审计日志；`raw_payload` 保留上游原始数据备查。

## 11. 测试策略

| 测试 | 范围 |
|---|---|
| Parser 单测 | one_api / litellm / custom 三种格式 fixture |
| DiffEngine 单测 | new/up/down/removed、浮点边界、空快照 |
| SuggestionCalculator 单测 | 跟随成本 / 锁死售价的毛利数学验证 |
| ApplyService 集成测 | 写 channel / group + 审计 + 状态机 |
| AlertAggregator 单测 | 聚合、severity 定级、限流/冷却 |
| 同步 E2E | mock 上游接口，全链路验证 |

## 12. 与现有代码集成点（已验证）

| 复用对象 | 位置 | 用法 |
|---|---|---|
| 定时 + 分布式锁 | `ops_*` 的 `time.Timer + Redis SetNX`（`ops_alert_evaluator_service.go:906-943`） | 新服务照搬 |
| 任务注册 | `internal/service/wire.go` ProviderSet + `cmd/server/wire.go` provideCleanup | 加 Provider + Stop |
| HTTP 上游调用 | `openai_apikey_responses_probe.go` 的 `httpUpstream.DoWithTLS` | 拉取定价客户端 |
| 邮件通知 | `NotificationEmailService.Send` | 新增 event 常量 |
| 应用-单价 | `ChannelService.ReplaceModelPricing`（`channel_repo_pricing.go:83`） | 写 channel_model_pricing |
| 应用-倍率 | group update（`group.go:45` rate_multiplier） | 写 group.rate_multiplier |
| 站内通知模式 | `announcement_read`（多用户独立已读） | admin_notification_read 对齐 |
| 前端铃铛 | `AnnouncementBell.vue` | 改数据源 |

## 13. 关键设计边界与权衡

1. **参考价纯旁路**：`ModelPricingResolver` 不读参考价表。只有 Apply 才进入计费链路 → 上游脏数据无法直接污染线上账单（金融"结算前对账"思路）。
2. **admin 通知独立表**：不复用 announcement（其 targeting 仅 subscription/balance，面向终端用户），避免内部成本信息泄露给终端用户。
3. **建议值双策略**：跟随成本 vs 锁死售价，覆盖两种经营决策，预算好让管理员选，不现场算。
4. **聚合 + 限流**：一次 Sync 一条通知，`last_hash` + cooldown 防同源刷屏。
5. **不自动改价**：方案 C 的"毛利率护城河自动规则"风险高（算错直接动用户售价），留二期。

## 14. 工作量拆解

| 模块 | 估时 |
|---|---|
| ent schema(5表) + repository | 1.0 天 |
| sync service + parser + diff + suggestion | 1.5 天 |
| apply service + handler + 路由 | 1.0 天 |
| admin notification service + handler + 铃铛 API | 0.5 天 |
| 邮件模板 | 0.5 天 |
| 前端 3 页面 + 铃铛 + store | 1.5 天 |
| 测试 | 1.0 天 |
| wire 注册 + 联调 | 0.5 天 |
| **合计** | **~7.5 天** |

## 15. 未来扩展（二期 / 方案 C）

- 多上游比价：同模型多源报价对比，辅助选路。
- 毛利率护城河自动规则：上游涨超阈值自动生成提价建议（仍人工确认或全自动可选）。
- webhook / Telegram 通知渠道。
- 模型名智能映射（模糊匹配 + 历史学习）。
- 参考价历史曲线图（独立 `upstream_price_history` 表）。

---

## 附录：决策溯源

- 价格/倍率两轴分离 → `billing_service.go:608`（ActualCost=TotalCost×mult）、`model_pricing_resolver.go:66-107`。
- 套餐无倍率 → `bundle_plan.go:31-49` 仅有 price/original_price + group_quotas。
- 现有 LiteLLM 同步无 diff/告警 → `pricing_service.go:243-281` 哈希变化仅打日志。
- 上游无定价发现 → 全仓无 `/v1/models` 价格读取代码；唯一上游探测 `openai_apikey_responses_probe.go` 不读价格。
- announcement targeting 局限 → `domain/announcement.go:50-64` 仅 subscription/balance。
