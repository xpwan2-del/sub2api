# 设计:渠道视频按秒计费(per_second 模式)

- **日期**: 2026-07-28
- **分支**: feat/video-price
- **状态**: 已与用户确认方向与改动清单,待写实现计划

## 1. 背景与目标

渠道层(`channel_model_pricing`)当前对视频模型支持 `token` / `per_request` / `image` / `video` 四种计费模式。其中 `per_request` / `image` / `video` 在视频路径下都走**按次计费**(不乘时长),导致用户生成 5 秒与 15 秒视频收费相同。而中转站上游成本是按秒计的(xAI Grok 按秒收),按次对用户收费会让中转站亏本。

**目标**: 新增 `per_second`(按秒)计费模式,总价 = 每秒单价 × 时长 × 段数,使不同时长视频收费不同,与上游成本口径对齐。该模式与现有按次模式并存,由管理员按模型主动选择。

## 2. 现状分析(关键事实)

### 计费三级回退(USD 维度)
- **L1 分组层每秒价**(`Group.VideoPrice480P/720P/1080P`)→ 已按秒
- **L2 渠道层**(`channel_model_pricing`)→ 当前 `per_request` / `image` / `video` 均按次 ← **问题在此层**
- **L3 默认价**(xAI Grok 官方每秒价,`billing_service.go:1515`)→ 已按秒

### 问题的精确位置
- `internal/service/openai_gateway_usage.go:522-543` `calculateOpenAIVideoCost` 的 L2 分支:`per_request` / `image` / `video` 都走 `CalculateCostUnified` 用 `RequestCount: videoCount`,注释 `:524` 明确"不乘视频时长"。
- `internal/service/billing_service.go:1057-1065` `calculatePerRequestCost` = `GetRequestTierPrice(resolved, sizeTier) × RequestCount`。

### 数据基础已具备(无需新增采集)
- **时长**: `result.VideoDurationSeconds` 已从用户请求解析(`grok_media.go:539`),归一化 1-15 秒、默认 8(`video_billing_resolution.go`)
- **段数**: `result.VideoCount`(=1)
- **分辨率**: `result.VideoResolution` 归一化 480p/720p/1080p
- **渠道 interval** 已有 `tier_label`(分辨率档)+ `per_request_price` 结构

### 数据层约束(决定零迁移可行性)
- `channel_model_pricing.billing_mode` 是 `VARCHAR(20)` **无 CHECK 约束**(`082` 迁移)
- `channel_model_pricing` 是**原生 SQL 表,不经 ent 管理**
- `usage_log.billing_mode` 是 `field.String().MaxLen(20)` 自由字符串、无 Enum 约束(`usage_log.go:59`)
- → 新增 `per_second` **无需 SQL 迁移、无需 `go generate ./ent`**
- tier 匹配大小写不敏感(`channel.go:172-179` `GetTierByLabel` 用 `ToLower`),`tier_label="720P"` 可命中 `resolution="720p"`

## 3. 设计决策(均已与用户确认)

| 决策点 | 选择 | 理由 |
|---|---|---|
| 方向 | 新增 `per_second` 计费模式,与现有按次并存 | 语义清晰,不破坏现有按次配置 |
| 价格字段 | 复用 `channel_pricing_intervals.per_request_price`(per_second 下语义=每秒价) | 零迁移;interval 的 tier_label=resolution 结构够用 |
| 代码路径 | 视频路径特判(`calculateOpenAIVideoCost`),不塞进 `CalculateCostUnified` | per_second 只对视频有意义;避免给 `CostInput` 加 `DurationSeconds` 波及全链路 |
| 兜底 | per_second 命中但该分辨率档未配价 → 回退 L3 默认每秒价 | 保证计费不中断 |
| 渠道成本统计 | `account_stats_pricing` 一并按秒改 | 上游按秒收,运营成本看板才不失真 |
| 适用范围 | 仅视频;图片路径(`openai_gateway_usage.go:477`)不含 per_second,不受影响 | 图片无时长概念 |

## 4. 计费公式

**per_second 模式**:
```
perSecondPrice  = GetRequestTierPrice(resolved, resolution)          // 复用 per_request_price
durationSeconds = NormalizeVideoBillingDurationSecondsOrDefault(...) // 1-15,默认 8
totalCost       = perSecondPrice × durationSeconds × videoCount
actualCost      = totalCost × rateMultiplier                          // 负数倍率归零
```

**对比现有 video 模式**: `per_request_price × videoCount`(不乘时长)。

**示例**: 720p 每秒价 $0.10 → 5s = $0.50,15s = $1.50(原按次两者同价)。

## 5. 计费链路改造(核心)

`calculateOpenAIVideoCost`(`internal/service/openai_gateway_usage.go:522-543`)的 L2 分支:

```go
if resolved := s.resolveOpenAIChannelPricing(ctx, billingModel, apiKey); resolved != nil {
    switch resolved.Mode {
    case BillingModePerSecond: // 【新增】
        perSecond := s.resolver.GetRequestTierPrice(resolved, resolution)
        if perSecond <= 0 {
            // 兜底:回退 L3 默认每秒价(groupConfig=nil 让 CalculateVideoCost 走默认)
            return s.billingService.CalculateVideoCost(billingModel, resolution, videoCount, durationSeconds, nil, multiplier)
        }
        total := perSecond * float64(durationSeconds) * float64(videoCount)
        if multiplier < 0 { multiplier = 0 }
        return &CostBreakdown{
            TotalCost:   total,
            ActualCost:  total * multiplier,
            BillingMode: string(BillingModePerSecond),
        }
    case BillingModePerRequest, BillingModeImage, BillingModeVideo: // 【不变】按次
        // ...原 CalculateCostUnified(RequestCount: videoCount) 逻辑保持...
    }
}
```

## 6. 改动清单

### 后端核心(2 处)
1. `channel.go:17` 后 —— 新增常量 `BillingModePerSecond = "per_second"`,注释"视频按秒计费(每秒单价×时长×段数)"
2. `openai_gateway_usage.go:522-543` —— `calculateOpenAIVideoCost` L2 分支加 `case BillingModePerSecond`(见第 5 节)

### 后端模式注册/校验(3 处,否则 per_second 被判非法)
3. `channel.go:23` `IsValid()` 加 `BillingModePerSecond`
4. `channel.go:32` `IsValidUsageFilter()` 加(否则使用记录筛不出 per_second)
5. `channel.go:296-313` `ValidateIntervals`:per_second 与 image/video 同样按 `tier_label` 校验区间

### 后端渠道定价解析(3 处,否则取不到价)
6. `channel_service.go:628` per_second 加入"按 tier 匹配区间"判断
7. `model_pricing_resolver.go:76` resolver 将 per_second 视为 tier 模式
8. `model_pricing_resolver.go:138` 同上

### 后端辅助(2 处,保证一致性)
9. `channel_available.go:185` per_second 归入"按量展示"类
10. `account_stats_pricing.go:167` per_second 按秒核算渠道成本(上游按秒收,运营看板才准)

### 后端明确不改(1 处)
- `billing_service.go:906` `CalculateCostUnified` switch —— per_second 在 #2 命中后直接 return,不会走到统一入口;改了反而要给 `CostInput` 加 `DurationSeconds` 波及全链路

### 数据层(0 改动)
- 无 SQL 迁移、无 ent schema 改动、无 `go generate`(理由见第 2 节)

### 前端(8 处)
11. `utils/billingMode.ts` —— 新增 per_second 常量与工具函数
12. `constants/channel.ts` —— 计费模式选项加 per_second
13. `components/admin/channel/PricingEntryCard.vue` —— 模式下拉加"按秒计费";per_second 选中时单位文案 → "每秒价格(USD/s)"
14. `components/admin/channel/IntervalRow.vue` —— per_second 时输入框 label/placeholder 改"每秒价格"
15. `components/admin/channel/types.ts` —— 类型加 per_second
16. `views/admin/ChannelsView.vue` —— 表单 billing_mode 选项、默认 tier(480P/720P/1080P)、提交逻辑
17. `components/models/ModelPriceSummary.vue` —— 价格摘要按 per_second 展示"/秒"
18. `i18n/locales/{zh,en}.ts` —— "按秒计费"/"每秒价格"等文案

## 7. 测试

### 后端单测(`-tags=unit`)
- `calculateOpenAIVideoCost`:per_second 命中按秒;5s vs 15s 不同价;duration 默认 8;分辨率分层取价;图片路径不受影响;未配价兜底回退 L3
- `IsValid` / `ValidateIntervals`:per_second 合法;区间按 tier 校验

### 集成(`-tags=integration`)
- 端到端:per_second 视频请求扣费正确;`usage_log` 落库 `billing_mode=per_second`、`video_duration_seconds` 正确

### 前端 vitest
- per_second 选项渲染;选中后单位"每秒价格";提交 payload 含 per_second + intervals

## 8. 向后兼容与风险

- 现有 `video` / `per_request` 配置不变(仍按次),无破坏性
- per_second 是纯增量模式,管理员主动选择
- **风险**:管理员把 per_second 配给非视频模型 → 图片路径(`openai_gateway_usage.go:477`)不含 per_second,图片不会误走按秒,安全
- **风险**:per_second 命中但该分辨率未配价 → 兜底回退 L3 默认每秒价,计费不中断
- **回滚**:per_second 是纯增量,删除新增分支 + 常量即回滚;无数据迁移不可逆操作

## 9. 关键文件清单

| 文件 | 改动类型 |
|---|---|
| `backend/internal/service/channel.go` | 常量 + IsValid + IsValidUsageFilter + ValidateIntervals |
| `backend/internal/service/openai_gateway_usage.go` | calculateOpenAIVideoCost 核心分支 |
| `backend/internal/service/channel_service.go` | 区间匹配判断 |
| `backend/internal/service/model_pricing_resolver.go` | resolver tier 模式判断(2 处) |
| `backend/internal/service/channel_available.go` | 可用性展示归类 |
| `backend/internal/service/account_stats_pricing.go` | 渠道成本按秒核算 |
| `frontend/src/utils/billingMode.ts` 等 8 处 | 模式选项 + 单位文案 + 表单 + 摘要 |
