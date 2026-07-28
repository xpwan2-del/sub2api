# 渠道视频按秒计费(per_second 模式)实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增 `per_second` 计费模式,让渠道层视频定价按秒收费(每秒单价 × 时长 × 段数),解决 5 秒与 15 秒视频同价导致中转站亏本的问题。

**Architecture:** 在 `calculateOpenAIVideoCost` 的 L2 渠道分支内特判 `per_second` 模式,复用 `channel_pricing_intervals.per_request_price`(语义=每秒价)与现成的 `GetRequestTierPrice`/`VideoDurationSeconds`,提取纯函数 `computePerSecondVideoCost` 便于 TDD。`per_second` 不走 `CalculateCostUnified` 统一入口。零 SQL 迁移、零 ent 改动。

**Tech Stack:** Go 1.26.4 / Gin / Ent(本任务 ent 零改);Vue 3 / TypeScript / Tailwind / pnpm。

## Global Constraints

- **后端 lint 必须过**:`cd backend && golangci-lint run ./...`(depguard 强制 handler↔service↔repository 分层)。
- **前端必须 pnpm**(非 npm);`pnpm.overrides` 安全补丁条目禁止删除。
- **channel_model_pricing 是原生 SQL 表(非 ent)**;`billing_mode` 是 `VARCHAR(20)` 无 CHECK 约束 → 新增 `'per_second'` **无需 SQL 迁移、无需 `go generate ./ent`**。
- **测试命令**:后端 `cd backend && go test -tags=unit ./...`;前端 `cd frontend && pnpm run test:run`。
- **已知范围限制**:`account_stats_pricing.go:162` `calculateStatsCost` 无 `durationSeconds` 参数,真正按秒需透传(见 Task 5 的 stretch)。
- **提交规范**:Conventional Commits;提交信息结尾加 `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。

## File Structure

| 文件 | 职责 | 改动 |
|---|---|---|
| `backend/internal/service/channel.go` | 计费模式常量 + 校验 | 新增常量 + 3 处校验 |
| `backend/internal/service/billing_service.go` | 计费核心 | 新增纯函数 `computePerSecondVideoCost` |
| `backend/internal/service/model_pricing_resolver.go` | 渠道定价解析 | 2 处把 per_second 视为 tier 模式 |
| `backend/internal/service/channel_service.go` | 区间匹配 | 1 处判断 |
| `backend/internal/service/openai_gateway_usage.go` | 视频计费路由 | `calculateOpenAIVideoCost` 加 per_second 分支 |
| `backend/internal/service/channel_available.go` | 展示合成 | 1 处归类 |
| `backend/internal/service/account_stats_pricing.go` | 运营统计 | 模式合法化(按秒见 stretch) |
| `frontend/src/utils/billingMode.ts` 等 8 处 | 模式选项/单位/表单/展示 | 见 Task 6-8 |

---

### Task 1: 注册 per_second 计费模式与校验

**Files:**
- Modify: `backend/internal/service/channel.go:14-17`(常量)、`:20-27`(IsValid)、`:29-35`(IsValidUsageFilter)、`:296-320`(ValidateIntervals)
- Test: `backend/internal/service/channel_test.go`

**Interfaces:**
- Produces: 常量 `BillingModePerSecond BillingMode = "per_second"`;`BillingModePerSecond.IsValid()` 返回 true。

- [ ] **Step 1: 写失败测试**(追加到 `channel_test.go`)

```go
func TestBillingModePerSecond_IsValid(t *testing.T) {
	require.True(t, BillingModePerSecond.IsValid(), "per_second should be a valid billing mode")
	require.True(t, BillingModePerSecond.IsValidUsageFilter(), "per_second should be valid usage filter")
}

func TestValidateIntervals_PerSecondMode_TierLabels(t *testing.T) {
	// per_second 与 image/video 同样按 tier_label 分层(分辨率档)
	intervals := []PricingInterval{{TierLabel: "720p", PerRequestPrice: testPtrFloat64(0.10)}}
	require.NoError(t, ValidateIntervals(intervals, BillingModePerSecond))
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test -tags=unit -run 'TestBillingModePerSecond_IsValid|TestValidateIntervals_PerSecondMode' ./internal/service/`
Expected: 编译失败 / FAIL —— `BillingModePerSecond` 未定义。

- [ ] **Step 3: 新增常量**(`channel.go:17` 后)

```go
	BillingModeVideo      BillingMode = "video"       // 视频生成计费（按次，含分辨率分层定价 480P/720P/1080P）
	BillingModePerSecond  BillingMode = "per_second"  // 视频按秒计费（每秒单价 × 时长 × 段数，分辨率分层）
```

- [ ] **Step 4: 注册到校验函数**

`IsValid()`(约 `:23`)的 case 加 `BillingModePerSecond`:
```go
	case BillingModeToken, BillingModePerRequest, BillingModeImage, BillingModeVideo, BillingModePerSecond, "":
```
`IsValidUsageFilter()`(约 `:32`)同样加 `BillingModePerSecond`。
`ValidateIntervals`(`:313`)的判断加 per_second:
```go
	if mode == BillingModePerRequest || mode == BillingModeImage || mode == BillingModeVideo || mode == BillingModePerSecond {
```

- [ ] **Step 5: 跑测试确认通过**

Run: `cd backend && go test -tags=unit -run 'TestBillingModePerSecond_IsValid|TestValidateIntervals_PerSecondMode' ./internal/service/`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add backend/internal/service/channel.go backend/internal/service/channel_test.go
git commit -m "feat(billing): 注册 per_second 视频按秒计费模式" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: per_second 按秒计费纯函数

**Files:**
- Modify: `backend/internal/service/billing_service.go`(在 `CalculateVideoCost` 后,约 `:1394`)
- Test: `backend/internal/service/billing_service_test.go`

**Interfaces:**
- Produces: `computePerSecondVideoCost(perSecondPrice float64, durationSeconds, videoCount int, rateMultiplier float64) *CostBreakdown`
- Consumes: `BillingModePerSecond`(Task 1)、`CostBreakdown`(`billing_service.go:165`)

**设计:** 把"每秒价 × 时长 × 段数 × 倍率"抽成纯函数,无需构造 service 即可 TDD。`perSecondPrice<=0` 由调用方(Task 4)在兜底前判断,纯函数假定入参有效。

- [ ] **Step 1: 写失败测试**(追加到 `billing_service_test.go`)

```go
func TestComputePerSecondVideoCost(t *testing.T) {
	t.Run("5s vs 15s different price", func(t *testing.T) {
		c5 := computePerSecondVideoCost(0.10, 5, 1, 1.0)
		c15 := computePerSecondVideoCost(0.10, 15, 1, 1.0)
		require.InDelta(t, 0.50, c5.TotalCost, 1e-10)
		require.InDelta(t, 1.50, c15.TotalCost, 1e-10) // 15s ≠ 5s,核心诉求
	})
	t.Run("applies rate multiplier", func(t *testing.T) {
		c := computePerSecondVideoCost(0.10, 10, 1, 2.0)
		require.InDelta(t, 1.0, c.TotalCost, 1e-10)
		require.InDelta(t, 2.0, c.ActualCost, 1e-10)
	})
	t.Run("negative multiplier clamps to zero", func(t *testing.T) {
		c := computePerSecondVideoCost(0.10, 10, 1, -1.0)
		require.InDelta(t, 1.0, c.TotalCost, 1e-10)
		require.InDelta(t, 0.0, c.ActualCost, 1e-10)
	})
	t.Run("zero video count returns empty", func(t *testing.T) {
		require.Equal(t, &CostBreakdown{}, computePerSecondVideoCost(0.10, 10, 0, 1.0))
	})
	t.Run("billing mode is per_second", func(t *testing.T) {
		c := computePerSecondVideoCost(0.10, 10, 1, 1.0)
		require.Equal(t, string(BillingModePerSecond), c.BillingMode)
	})
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test -tags=unit -run 'TestComputePerSecondVideoCost' ./internal/service/`
Expected: FAIL —— `computePerSecondVideoCost` undefined。

- [ ] **Step 3: 实现纯函数**(`billing_service.go`,`CalculateVideoCost` 函数后)

```go
// computePerSecondVideoCost 渠道 per_second 模式按秒计费:每秒单价 × 时长(秒) × 段数 × 倍率。
// 调用方负责保证 perSecondPrice > 0(未配价时走兜底),durationSeconds 已归一化(1-15,默认 8)。
func computePerSecondVideoCost(perSecondPrice float64, durationSeconds, videoCount int, rateMultiplier float64) *CostBreakdown {
	if videoCount <= 0 {
		return &CostBreakdown{}
	}
	if rateMultiplier < 0 {
		rateMultiplier = 0
	}
	total := perSecondPrice * float64(durationSeconds) * float64(videoCount)
	return &CostBreakdown{
		TotalCost:   total,
		ActualCost:  total * rateMultiplier,
		BillingMode: string(BillingModePerSecond),
	}
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test -tags=unit -run 'TestComputePerSecondVideoCost' ./internal/service/`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add backend/internal/service/billing_service.go backend/internal/service/billing_service_test.go
git commit -m "feat(billing): 新增 per_second 按秒计费纯函数" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: resolver 与 channel_service 把 per_second 视为 tier 模式

让 `Resolve` 在 per_second 模式下把 interval 填入 `RequestTiers`,使 `GetRequestTierPrice(resolved, resolution)` 能取到每秒价。

**Files:**
- Modify: `backend/internal/service/model_pricing_resolver.go:76`、`:138`;`backend/internal/service/channel_service.go:628`
- Test: `backend/internal/service/model_pricing_resolver_test.go`

**Interfaces:**
- Consumes: `BillingModePerSecond`(Task 1)
- Produces: per_second 模式下 `ResolvedPricing.RequestTiers` 被填充,`GetRequestTierPrice` 命中。

- [ ] **Step 1: 写失败测试**(追加到 `model_pricing_resolver_test.go`,参考 `TestCalculateCostUnified_VideoMode` 的 cache 构造)

```go
func TestResolve_PerSecondMode_PopulatesRequestTiers(t *testing.T) {
	cs := newTestChannelServiceWithCache(t, &channelCache{
		pricingByGroupModel: map[channelModelKey]*ChannelModelPricing{
			{groupID: 7, model: "grok-imagine-video"}: {
				BillingMode: BillingModePerSecond,
				Intervals:   []PricingInterval{{TierLabel: "720p", PerRequestPrice: testPtrFloat64(0.10)}},
			},
		},
		channelByGroupID:         map[int64]*Channel{7: {ID: 7, Status: StatusActive}},
		groupPlatform:            map[int64]string{7: ""},
		wildcardByGroupPlatform:  map[channelGroupPlatformKey][]*wildcardPricingEntry{},
		mappingByGroupModel:      map[channelModelKey]string{},
		wildcardMappingByGP:      map[channelGroupPlatformKey][]*wildcardMappingEntry{},
		byID:                     map[int64]*Channel{},
	})
	bs := &BillingService{cfg: &config.Config{}, fallbackPrices: map[string]*ModelPricing{}}
	resolver := NewModelPricingResolver(cs, bs)
	gid := int64(7)

	resolved := resolver.Resolve(context.Background(), PricingInput{Model: "grok-imagine-video", GroupID: &gid})
	require.Equal(t, BillingModePerSecond, resolved.Mode)
	require.Equal(t, 0.10, resolver.GetRequestTierPrice(resolved, "720p"))
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test -tags=unit -run 'TestResolve_PerSecondMode_PopulatesRequestTiers' ./internal/service/`
Expected: FAIL —— resolved.Mode 不是 per_second(被当 token)或取价为 0。

- [ ] **Step 3: 改 resolver 两处判断**

`model_pricing_resolver.go:76` 加 per_second:
```go
		if mode == BillingModePerRequest || mode == BillingModeImage || mode == BillingModeVideo || mode == BillingModePerSecond {
```
`:138` 的 case 加 per_second:
```go
	case BillingModePerRequest, BillingModeImage, BillingModeVideo, BillingModePerSecond:
```

- [ ] **Step 4: 改 channel_service.go:628 区间匹配判断**

```go
	if p.BillingMode == BillingModePerRequest || p.BillingMode == BillingModeImage || p.BillingMode == BillingModeVideo || p.BillingMode == BillingModePerSecond {
```

- [ ] **Step 5: 跑测试确认通过**

Run: `cd backend && go test -tags=unit -run 'TestResolve_PerSecondMode' ./internal/service/`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add backend/internal/service/model_pricing_resolver.go backend/internal/service/channel_service.go backend/internal/service/model_pricing_resolver_test.go
git commit -m "feat(billing): resolver 支持 per_second 模式按 tier 解析" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: calculateOpenAIVideoCost 接入 per_second 分支(核心)

**Files:**
- Modify: `backend/internal/service/openai_gateway_usage.go:522-543`(`calculateOpenAIVideoCost` 的 L2 分支)
- Test: `backend/internal/service/openai_gateway_record_usage_test.go`

**Interfaces:**
- Consumes: `computePerSecondVideoCost`(Task 2)、`GetRequestTierPrice`、`CalculateVideoCost`(兜底)、resolver/channelService(Task 3)
- Produces: per_second 渠道视频请求按秒计费。

- [ ] **Step 1: 写失败集成测试**(追加到 `openai_gateway_record_usage_test.go`)

```go
func TestCalculateOpenAIVideoCost_PerSecondMode_DifferentDuration(t *testing.T) {
	cs := newTestChannelServiceWithCache(t, &channelCache{
		pricingByGroupModel: map[channelModelKey]*ChannelModelPricing{
			{groupID: 5, model: "grok-imagine-video"}: {
				BillingMode: BillingModePerSecond,
				Intervals:   []PricingInterval{{TierLabel: "720p", PerRequestPrice: testPtrFloat64(0.10)}},
			},
		},
		channelByGroupID:         map[int64]*Channel{5: {ID: 5, Status: StatusActive}},
		groupPlatform:            map[int64]string{5: ""},
		wildcardByGroupPlatform:  map[channelGroupPlatformKey][]*wildcardPricingEntry{},
		mappingByGroupModel:      map[channelModelKey]string{},
		wildcardMappingByGP:      map[channelGroupPlatformKey][]*wildcardMappingEntry{},
		byID:                     map[int64]*Channel{},
	})
	bs := &BillingService{cfg: &config.Config{}, fallbackPrices: map[string]*ModelPricing{}}
	resolver := NewModelPricingResolver(cs, bs)
	svc := &OpenAIGatewayService{resolver: resolver, billingService: bs}
	groupID := int64(5)
	// VideoRateIndependent=true 避免 apiKeyWithFreshGroupMediaPricing 回源(nil channelService 不 panic)
	apiKey := &APIKey{GroupID: &groupID, Group: &Group{ID: 5, VideoRateIndependent: true}}

	t.Run("5s vs 15s", func(t *testing.T) {
		r5 := &OpenAIForwardResult{VideoCount: 1, VideoResolution: "720p", VideoDurationSeconds: 5, BillingModel: "grok-imagine-video"}
		r15 := &OpenAIForwardResult{VideoCount: 1, VideoResolution: "720p", VideoDurationSeconds: 15, BillingModel: "grok-imagine-video"}
		c5 := svc.calculateOpenAIVideoCost(context.Background(), "grok-imagine-video", apiKey, r5, 1.0)
		c15 := svc.calculateOpenAIVideoCost(context.Background(), "grok-imagine-video", apiKey, r15, 1.0)
		require.InDelta(t, 0.50, c5.TotalCost, 1e-10)  // 0.10 × 5
		require.InDelta(t, 1.50, c15.TotalCost, 1e-10) // 0.10 × 15
		require.Equal(t, string(BillingModePerSecond), c15.BillingMode)
	})

	t.Run("default duration 8 when unset", func(t *testing.T) {
		r := &OpenAIForwardResult{VideoCount: 1, VideoResolution: "720p", BillingModel: "grok-imagine-video"}
		c := svc.calculateOpenAIVideoCost(context.Background(), "grok-imagine-video", apiKey, r, 1.0)
		require.InDelta(t, 0.80, c.TotalCost, 1e-10) // 0.10 × 8
	})

	t.Run("fallback to default price when tier unconfigured", func(t *testing.T) {
		// 1080p 未配价 → 回退 L3 默认每秒价(grok-imagine-video 1080p=$0.07)
		r := &OpenAIForwardResult{VideoCount: 1, VideoResolution: "1080p", VideoDurationSeconds: 10, BillingModel: "grok-imagine-video"}
		c := svc.calculateOpenAIVideoCost(context.Background(), "grok-imagine-video", apiKey, r, 1.0)
		require.InDelta(t, 0.70, c.TotalCost, 1e-10) // 0.07 × 10
	})
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test -tags=unit -run 'TestCalculateOpenAIVideoCost_PerSecondMode' ./internal/service/`
Expected: FAIL —— 当前 per_second 落到 `case PerRequest/Image/Video`(不存在),走按次或 default。

- [ ] **Step 3: 在 L2 分支加 per_second 特判**(`openai_gateway_usage.go:522-543`)

把现有 L2 判断改为先特判 per_second:
```go
	if resolved := s.resolveOpenAIChannelPricing(ctx, billingModel, apiKey); resolved != nil {
		if resolved.Mode == BillingModePerSecond {
			perSecond := s.resolver.GetRequestTierPrice(resolved, resolution)
			if perSecond > 0 {
				return computePerSecondVideoCost(perSecond, durationSeconds, videoCount, multiplier)
			}
			// 兜底:该分辨率档未配价 → 回退 L3 默认每秒价(groupConfig=nil 走 getDefaultVideoPrice)
			return s.billingService.CalculateVideoCost(billingModel, resolution, videoCount, durationSeconds, nil, multiplier)
		}
		if resolved.Mode == BillingModePerRequest || resolved.Mode == BillingModeImage || resolved.Mode == BillingModeVideo {
			// ...原 CalculateCostUnified 按次逻辑保持不变...
		}
	}
```
(注:`videoCount`/`resolution`/`durationSeconds` 在函数上方 `:505-510` 已归一化,直接复用。)

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test -tags=unit -run 'TestCalculateOpenAIVideoCost_PerSecondMode' ./internal/service/`
Expected: PASS

- [ ] **Step 5: 回归现有视频/图片计费测试,确认未破坏按次路径**

Run: `cd backend && go test -tags=unit -run 'TestCalculateCostUnified_VideoMode|TestCalculateCostUnified_ImageMode|TestCalculateCostUnified_PerRequestMode' ./internal/service/`
Expected: PASS(按次路径未受影响)

- [ ] **Step 6: 全量后端单测 + lint**

Run: `cd backend && go test -tags=unit ./... && golangci-lint run ./...`
Expected: 全绿

- [ ] **Step 7: 提交**

```bash
git add backend/internal/service/openai_gateway_usage.go backend/internal/service/openai_gateway_record_usage_test.go
git commit -m "feat(billing): 视频渠道 per_second 模式按秒计费" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: 辅助路径一致性(channel_available + account_stats)

**Files:**
- Modify: `backend/internal/service/channel_available.go:185`;`backend/internal/service/account_stats_pricing.go:166-171`
- Test: `backend/internal/service/account_stats_pricing_test.go`(若无则新建)

**说明 / 范围界定:** `calculateStatsCost(pricing, tokens, requestCount)` 无 `durationSeconds` 参数。本期仅做**模式合法化**(per_second 不被当 token 误算、不 panic);真正按秒核算需透传 duration,列为 **stretch(可选)**,不阻塞主功能。

- [ ] **Step 1: channel_available.go:185 归类**

```go
	if mode == BillingModeImage || mode == BillingModePerRequest || mode == BillingModePerSecond {
```
(让 per_second 从 LiteLLM 合成时落到按次展示分支;per_second 无 LiteLLM 来源,仅保证 existing 配置不被当 token。)

- [ ] **Step 2: account_stats_pricing.go:167 模式合法化**

```go
	switch pricing.BillingMode {
	case BillingModePerRequest, BillingModeImage, BillingModeVideo, BillingModePerSecond:
		return calculatePerRequestStatsCost(pricing, requestCount)
	default:
		return calculateTokenStatsCost(pricing, tokens)
	}
```

- [ ] **Step 3: 写测试锁定"不 panic + 走按次"**(追加到 account_stats_pricing_test.go)

```go
func TestCalculateStatsCost_PerSecondMode_DoesNotPanic(t *testing.T) {
	price := 0.10
	pricing := &ChannelModelPricing{BillingMode: BillingModePerSecond, PerRequestPrice: &price}
	cost := calculateStatsCost(pricing, UsageTokens{}, 2)
	// 本期按次统计(已知限制:真正按秒见 stretch);只保证不 panic 且返回非 nil
	require.NotNil(t, cost)
}
```

- [ ] **Step 4: 跑测试 + lint**

Run: `cd backend && go test -tags=unit -run 'TestCalculateStatsCost_PerSecondMode' ./internal/service/ && golangci-lint run ./...`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add backend/internal/service/channel_available.go backend/internal/service/account_stats_pricing.go backend/internal/service/account_stats_pricing_test.go
git commit -m "feat(billing): 辅助路径识别 per_second 模式(统计暂按次)" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

> **Stretch(可选,单独评估):** 让 `account_stats_pricing` 真正按秒 —— 需给 `calculateStatsCost` 加 `durationSeconds int` 参数,并让所有调用方从 `usage_log.video_duration_seconds` 聚合透传。涉及账号统计聚合链路,建议单独 PR。

---

### Task 6: 前端模式常量与类型

**Files:**
- Modify: `frontend/src/utils/billingMode.ts`;`frontend/src/constants/channel.ts`;`frontend/src/components/admin/channel/types.ts`
- Test: `frontend/src/components/admin/channel/__tests__/types.spec.ts`

**Interfaces:**
- Produces: `BILLING_MODE_PER_SECOND = 'per_second'` 常量;`getBillingModeLabel` 支持 per_second。

- [ ] **Step 1: 写失败测试**(追加到 `types.spec.ts` 或 `billingMode` 相关 spec)

```ts
import { getBillingModeLabel, BILLING_MODE_PER_SECOND } from '@/utils/billingMode'

describe('per_second billing mode', () => {
  it('exposes constant', () => {
    expect(BILLING_MODE_PER_SECOND).toBe('per_second')
  })
  it('renders label via i18n key', () => {
    const t = (k: string) => k
    expect(getBillingModeLabel(BILLING_MODE_PER_SECOND, t)).toBe('admin.usage.billingModePerSecond')
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd frontend && pnpm vitest run src/components/admin/channel/__tests__/types.spec.ts`
Expected: FAIL —— `BILLING_MODE_PER_SECOND` 未导出。

- [ ] **Step 3: `billingMode.ts` 加常量与分支**

```ts
export const BILLING_MODE_PER_SECOND = 'per_second'
```
`getBillingModeLabel` switch 加:
```ts
    case BILLING_MODE_PER_SECOND: return t('admin.usage.billingModePerSecond')
```
`getBillingModeBadgeClass` 加:
```ts
    case BILLING_MODE_PER_SECOND: return 'bg-teal-100 text-teal-700 dark:bg-teal-900/30 dark:text-teal-300'
```
`isImageUsage` 的排除条件加 `&& row?.billing_mode !== BILLING_MODE_PER_SECOND`(视频不计入图片)。

- [ ] **Step 4: `constants/channel.ts` 与 `types.ts`**

在计费模式选项列表与 `BillingMode` 类型加入 `'per_second'`(跟随现有 token/per_request/image/video 的写法)。

- [ ] **Step 5: 跑测试确认通过**

Run: `cd frontend && pnpm vitest run src/components/admin/channel/__tests__/types.spec.ts`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add frontend/src/utils/billingMode.ts frontend/src/constants/channel.ts frontend/src/components/admin/channel/types.ts frontend/src/components/admin/channel/__tests__/types.spec.ts
git commit -m "feat(frontend): 新增 per_second 计费模式常量与类型" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: 前端渠道定价配置 UI(模式选项 + 每秒价输入)

**Files:**
- Modify: `frontend/src/components/admin/channel/PricingEntryCard.vue`(`:264` billingModeOptions、`:203` 后加 per_second 模式块);`frontend/src/components/admin/channel/IntervalRow.vue`
- Test: `frontend/src/components/admin/channel/__tests__/`(新增 PricingEntryCard spec)

**Interfaces:**
- Consumes: `BILLING_MODE_PER_SECOND`(Task 6)

- [ ] **Step 1: 写失败测试**(验证 per_second 选项存在 + 选中后渲染每秒价输入)

```ts
it('shows per_second option and per-second price input when selected', async () => {
  const { getByRole, getByText } = render(PricingEntryCard, {
    props: { entry: { billing_mode: 'per_second', models: ['grok-imagine-video'], intervals: [], per_request_price: null }, platform: 'openai' },
    global: { plugins: [i18n] },
  })
  // 选中 per_second 时,渲染"每秒价格"输入区
  expect(getByText('admin.channels.form.perSecondPrice')).toBeTruthy()
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd frontend && pnpm vitest run src/components/admin/channel`
Expected: FAIL。

- [ ] **Step 3: `PricingEntryCard.vue` script 的 `billingModeOptions`(`:264`)加选项**

```ts
const billingModeOptions = computed(() => [
  // ...现有 token/per_request/image/video...
  { label: t('admin.channels.form.billingModePerSecond'), value: 'per_second' },
])
```

- [ ] **Step 4: 模板加 per_second 模式块**(仿 `:203` 的 video 块,在 video 块后)

```vue
<div v-else-if="entry.billing_mode === 'per_second'">
  <label class="mt-3 block text-xs font-medium text-gray-500 dark:text-gray-400">
    {{ t('admin.channels.form.defaultPerSecondPrice') }}
    <span class="ml-1 font-normal text-gray-400">USD/s</span>
  </label>
  <input :value="entry.per_request_price" @input="emitField('per_request_price', ($event.target as HTMLInputElement).value)"
    type="number" step="any" min="0" class="input mt-0.5 text-sm" :placeholder="t('admin.channels.form.perSecondPricePlaceholder')" />
  <!-- 分辨率分层 intervals(复用 video 的 IntervalRow 渲染) -->
  <IntervalRow ... />  <!-- 同 video 块,tier_label 用 480P/720P/1080P -->
</div>
```
(注:`per_request_price` 字段在 per_second 下语义=每秒价;`IntervalRow` 的单位文案随模式切换 —— 见 Step 5。)

- [ ] **Step 5: `IntervalRow.vue` 单位文案随模式切换**

在 IntervalRow 的价格 label 处,按父传入的 `billingMode` 显示"每秒价格(USD/s)"(per_second)或"每次价格"(其他)。

- [ ] **Step 6: 跑测试确认通过 + typecheck**

Run: `cd frontend && pnpm vitest run src/components/admin/channel && pnpm run typecheck`
Expected: PASS

- [ ] **Step 7: 提交**

```bash
git add frontend/src/components/admin/channel/PricingEntryCard.vue frontend/src/components/admin/channel/IntervalRow.vue frontend/src/components/admin/channel/__tests__/
git commit -m "feat(frontend): 渠道定价配置支持 per_second 每秒价输入" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 8: 前端表单提交、价格摘要展示、i18n 文案

**Files:**
- Modify: `frontend/src/views/admin/ChannelsView.vue`(表单默认值/提交,`:853`/`:1064` 等);`frontend/src/components/models/ModelPriceSummary.vue`;`frontend/src/i18n/locales/zh.ts`、`en.ts`

- [ ] **Step 1: i18n 文案(先加,供 UI 引用)**

`zh.ts` 的 `admin.usage` 与 `admin.channels.form` 加:
```ts
billingModePerSecond: '按秒',
// channels.form:
billingModePerSecond: '按秒计费(视频)',
defaultPerSecondPrice: '默认每秒价格',
perSecondPricePlaceholder: '每秒单价',
perSecondPrice: '每秒价格',
```
`en.ts` 对应:`Per second` / `Per-second billing (video)` / `Default per-second price` / `Per-second price` 等。

- [ ] **Step 2: `ChannelsView.vue` 表单**

- 新建定价条目默认值(`:853` 等附近):`per_second` 模式无需新字段(复用 `per_request_price`),确保 `formIntervalsToAPI` 对 per_second 也按 tier 提交(沿用 video 分支逻辑,`:1071`)。
- 编辑回填(`:1064`):per_second 条目正常加载(字段相同)。

- [ ] **Step 3: `ModelPriceSummary.vue` 展示**

按 `billing_mode === 'per_second'` 展示价格单位为"/秒"(参考现有 image 的"/张"、video 的"/次"分支)。

- [ ] **Step 4: 前端测试 + lint + typecheck**

Run: `cd frontend && pnpm run test:run && pnpm run lint:check && pnpm run typecheck`
Expected: 全绿

- [ ] **Step 5: 提交**

```bash
git add frontend/src/views/admin/ChannelsView.vue frontend/src/components/models/ModelPriceSummary.vue frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "feat(frontend): per_second 表单提交/价格展示/文案" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Self-Review(计划自检)

**1. Spec coverage**(对照设计文档 §6 改动清单):
- 后端核心 #1→Task 1;#2→Task 4 ✓
- 模式注册 #3-5→Task 1 ✓
- 渠道解析 #6-8→Task 3 ✓
- 辅助 #9→Task 5(channel_available);#10→Task 5(模式合法化)+ stretch(真正按秒,已标注范围偏差)⚠️ 已诚实标注
- 不改项(统一入口)→ 计划未触碰 `CalculateCostUnified` switch ✓
- 数据层 0 改动 → 计划无迁移/ent 任务 ✓
- 前端 #11-18→Task 6-8 ✓
- 测试 → 每个 Task 含 TDD 步骤 ✓

**2. Placeholder scan:** 无 TBD/TODO;前端模板代码用 `...(沿用 video 分支)` 处标注了复用来源,实现者可定位。i18n key 与组件 prop 名前后一致。

**3. Type consistency:** `BillingModePerSecond`(后端常量)↔ `'per_second'`(前端字面量)↔ `BILLING_MODE_PER_SECOND`(前端常量);`computePerSecondVideoCost` 签名在 Task 2 定义、Task 4 调用一致;`testPtrFloat64`/`newTestChannelServiceWithCache` 沿用现有测试 helper。

**已知范围偏差(需用户在 review 时确认):** `account_stats_pricing` 真正按秒需透传 `durationSeconds`(stretch),本期仅模式合法化。这是计划阶段发现的设计 §3 假设与代码现实(`calculateStatsCost` 无 duration 参数)的差异。
