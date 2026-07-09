# 渠道定价：图片/视频计费层级下拉框化 + 匹配闭环 — 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把渠道定价配置中图片/视频的「分辨率」从手输文本框改为固定枚举下拉框，修复后端 tier_label 大小写敏感匹配 bug，并新增 `video` 计费模式骨架（配置可保存 + 解析链路就绪）。

**Architecture:** 前后端共享同一套分辨率档位 key（图片 1K/2K/4K，视频 480P/720P/1080P/4K）。前端把档位下拉框化 + 校验加严；后端修复 `GetRequestTierPrice` 大小写归一化、新增 `video` 模式常量与计费分发骨架。视频请求金额入口本轮不改（属后续专项）。

**Tech Stack:** Go 1.26 (Gin/Ent)、Vue 3 + TypeScript、自研 Tailwind 组件（`Select.vue`）、vitest 2.1.9 + @vue/test-utils 2.4.6、Go 单测 `-tags=unit`。

## Global Constraints

- 前端必须用 **pnpm**（非 npm）；改 `package.json` 后跑 `pnpm install` 并提交 lockfile（本计划不改 package.json）。
- 图片档位固定枚举 **`1K`/`2K`/`4K`**；视频档位固定枚举 **`480P`/`720P`/`1080P`/`4K`**。下拉框严格枚举，不允许手输。
- `per_request`（通用按次）模式保持自由文本「层级」不动。
- `video` 模式本轮是骨架：常量 + `IsValid` + `Resolve`/`applyChannelOverrides` 识别 + `CalculateCostUnified` 分发；**不改**视频请求金额入口（`calculateRecordUsageCost`/`calculateOpenAIRecordUsageCost`）。
- **无 migration、无 ent schema 变更、无 Wire 变更。** `channel_pricing_intervals.tier_label VARCHAR(50)` 已支持任意字符串。
- 后端 `golangci-lint` 必须通过（depguard：service 不 import repository/gorm/redis）。
- 计费精度不变（本计划只动 tier_label 匹配与枚举，不触碰 per-token 价格精度）。

---

## File Structure

**后端（3 文件）：**
- `backend/internal/service/channel.go` — `BillingModeVideo` 常量、`IsValid()`、`ValidateIntervals` video 分支
- `backend/internal/service/model_pricing_resolver.go` — `GetRequestTierPrice` 大小写归一化、`Resolve`/`applyChannelOverrides` 识别 video
- `backend/internal/service/billing_service.go` — `CalculateCostUnified` mode 分发加 video

**前端（6 文件）：**
- `frontend/src/constants/channel.ts` — `BILLING_MODE_VIDEO`、`IMAGE_RESOLUTION_OPTIONS`、`VIDEO_RESOLUTION_OPTIONS`
- `frontend/src/api/admin/channels.ts` — `BillingMode` 类型并入 `video`
- `frontend/src/components/admin/channel/types.ts` — 新增纯函数 `resolutionOptionsForMode`/`nextDefaultTierLabel`、`validateIntervals` 加严
- `frontend/src/components/admin/channel/IntervalRow.vue` — image/video 下拉框化、移除 Min/Max、标红
- `frontend/src/components/admin/channel/PricingEntryCard.vue` — video 模式分支、`addImageTier` 泛化、`billingModeOptions`
- `frontend/src/i18n/locales/zh.ts` / `en.ts` — video 模式与层级文案

**无新建文件**（测试追加到既有 `*_test.go` / 新建 `.spec.ts`）。

---

## Task 1: 后端 — `BillingModeVideo` 常量 + `IsValid` + `ValidateIntervals`

**Files:**
- Modify: `backend/internal/service/channel.go`（常量块 L13-17、`IsValid` L20-26、`ValidateIntervals` L302-305）
- Test: `backend/internal/service/channel_test.go`（追加）

**Interfaces:**
- Produces: 常量 `BillingModeVideo BillingMode = "video"`；`BillingModeVideo.IsValid() == true`；`ValidateIntervals(_, BillingModeVideo)` 跳过 token 区间重叠校验。

- [ ] **Step 1: 写失败测试**（追加到 `channel_test.go` 末尾）

```go
func TestBillingModeVideoIsValid(t *testing.T) {
	if !BillingModeVideo.IsValid() {
		t.Fatal("BillingModeVideo should be a valid billing mode")
	}
}

func TestValidateIntervalsVideoSkipsOverlap(t *testing.T) {
	price := func(v float64) *float64 { return &v }
	// 两个区间 token 范围重叠，但 video 模式按 tier_label 分层，应跳过重叠校验
	intervals := []PricingInterval{
		{TierLabel: "480P", MinTokens: 0, MaxTokens: nil, PerRequestPrice: price(0.1)},
		{TierLabel: "720P", MinTokens: 0, MaxTokens: nil, PerRequestPrice: price(0.2)},
	}
	if err := ValidateIntervals(intervals, BillingModeVideo); err != nil {
		t.Fatalf("video mode should skip token-overlap check, got err: %v", err)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test -tags=unit -run 'TestBillingModeVideoIsValid|TestValidateIntervalsVideoSkipsOverlap' ./internal/service/`
Expected: FAIL / 编译失败（`BillingModeVideo` undefined）。

- [ ] **Step 3: 实现**（`channel.go`）

常量块（在 `BillingModeImage` 后追加）：
```go
const (
	BillingModeToken      BillingMode = "token"       // 按 token 区间计费
	BillingModePerRequest BillingMode = "per_request" // 按次计费（支持上下文窗口分层）
	BillingModeImage      BillingMode = "image"       // 图片计费（当前按次，预留 token 计费）
	BillingModeVideo      BillingMode = "video"       // 视频计费（按次，预留分辨率分层）
)
```

`IsValid` switch case 加 `BillingModeVideo`：
```go
func (m BillingMode) IsValid() bool {
	switch m {
	case BillingModeToken, BillingModePerRequest, BillingModeImage, BillingModeVideo, "":
		return true
	}
	return false
}
```

`ValidateIntervals` 跳过重叠分支加 video：
```go
	// per_request / image / video 模式按 tier_label 匹配，不做 token 区间重叠校验
	if mode == BillingModePerRequest || mode == BillingModeImage || mode == BillingModeVideo {
		return nil
	}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test -tags=unit -run 'TestBillingModeVideoIsValid|TestValidateIntervalsVideoSkipsOverlap' ./internal/service/`
Expected: PASS。

- [ ] **Step 5: lint + commit**

Run: `cd backend && golangci-lint run ./internal/service/...`（应通过）
```bash
git add backend/internal/service/channel.go backend/internal/service/channel_test.go
git commit -m "feat(billing): 新增 video 计费模式常量与校验

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 2: 后端 — `GetRequestTierPrice` 大小写归一化（图片匹配 bug 修复）

**Files:**
- Modify: `backend/internal/service/model_pricing_resolver.go`（`GetRequestTierPrice` L264-271，可能需补 `strings` import）
- Test: `backend/internal/service/model_pricing_resolver_test.go`（追加）

**Interfaces:**
- Produces: `GetRequestTierPrice(resolved, tierLabel)` 改为大小写不敏感匹配（`strings.EqualFold`），与 `GetTierByLabel` 一致。

- [ ] **Step 1: 写失败测试**（追加到 `model_pricing_resolver_test.go`）

```go
func TestGetRequestTierPriceCaseInsensitive(t *testing.T) {
	price := func(v float64) *float64 { return &v }
	resolved := &ResolvedPricing{
		RequestTiers: []PricingInterval{
			{TierLabel: "1K", PerRequestPrice: price(0.04)},
			{TierLabel: "2K", PerRequestPrice: price(0.08)},
		},
	}
	r := &ModelPricingResolver{}
	cases := []struct {
		label string
		want  float64
	}{
		{"1K", 0.04},  // 完全匹配
		{"1k", 0.04},  // 小写请求应命中（修复前返回 0）
		{"2k", 0.08},
		{"4K", 0},     // 无此档
	}
	for _, c := range cases {
		if got := r.GetRequestTierPrice(resolved, c.label); got != c.want {
			t.Errorf("GetRequestTierPrice(%q) = %v, want %v", c.label, got, c.want)
		}
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test -tags=unit -run 'TestGetRequestTierPriceCaseInsensitive' ./internal/service/`
Expected: FAIL（`1k` 返回 0，期望 0.04）。

- [ ] **Step 3: 实现**（`model_pricing_resolver.go`）

先确认顶部 import 是否含 `"strings"`（该文件若未 import 则在 import 块加入）。然后将 `GetRequestTierPrice` 改为：
```go
// GetRequestTierPrice 根据层级标签获取按次价格（大小写不敏感，与 GetTierByLabel 一致）
func (r *ModelPricingResolver) GetRequestTierPrice(resolved *ResolvedPricing, tierLabel string) float64 {
	for _, tier := range resolved.RequestTiers {
		if strings.EqualFold(tier.TierLabel, tierLabel) && tier.PerRequestPrice != nil {
			return *tier.PerRequestPrice
		}
	}
	return 0
}
```

- [ ] **Step 4: 跑测试确认通过 + 全包编译**

Run: `cd backend && go test -tags=unit -run 'TestGetRequestTierPriceCaseInsensitive' ./internal/service/`
Expected: PASS。

- [ ] **Step 5: lint + commit**

Run: `cd backend && golangci-lint run ./internal/service/...`
```bash
git add backend/internal/service/model_pricing_resolver.go backend/internal/service/model_pricing_resolver_test.go
git commit -m "fix(billing): GetRequestTierPrice 层级标签大小写不敏感匹配

根除 DB 存小写 tier_label 时请求大写匹配失败、静默落兜底价的问题。

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 3: 后端 — video 计费分发骨架（resolver + billing 端到端）

**Files:**
- Modify: `backend/internal/service/model_pricing_resolver.go`（`Resolve` L75、`applyChannelOverrides` L137）
- Modify: `backend/internal/service/billing_service.go`（`CalculateCostUnified` switch L818-823）
- Test: `backend/internal/service/billing_service_unified_test.go`（追加）

**Interfaces:**
- Consumes: Task 1 的 `BillingModeVideo`。
- Produces: `video` 模式经 `Resolve` 走 `applyRequestTierOverrides`（填充 `RequestTiers`/`DefaultPerRequestPrice`），`CalculateCostUnified` 把 `video` 分发到 `calculatePerRequestCost`。

- [ ] **Step 1: 写失败测试**（追加到 `billing_service_unified_test.go`，复制 `TestCalculateCostUnified_ImageMode` 模式）

```go
func TestCalculateCostUnified_VideoMode(t *testing.T) {
	cs := newTestChannelServiceWithCache(t, &channelCache{
		pricingByGroupModel: map[channelModelKey]*ChannelModelPricing{
			{groupID: 3, model: "sora-video"}: {
				BillingMode:     BillingModeVideo,
				PerRequestPrice: testPtrFloat64(0.20),
			},
		},
		channelByGroupID: map[int64]*Channel{
			3: {ID: 3, Status: StatusActive},
		},
		groupPlatform:           map[int64]string{3: ""},
		wildcardByGroupPlatform: map[channelGroupPlatformKey][]*wildcardPricingEntry{},
		mappingByGroupModel:     map[channelModelKey]string{},
		wildcardMappingByGP:     map[channelGroupPlatformKey][]*wildcardMappingEntry{},
		byID:                    map[int64]*Channel{},
	})

	bs := &BillingService{
		cfg:            &config.Config{},
		fallbackPrices: map[string]*ModelPricing{},
	}
	resolver := NewModelPricingResolver(cs, bs)
	groupID := int64(3)

	input := CostInput{
		Ctx:            context.Background(),
		Model:          "sora-video",
		GroupID:        &groupID,
		Tokens:         UsageTokens{},
		RequestCount:   2,
		RateMultiplier: 1.0,
		Resolver:       resolver,
	}
	cost, err := bs.CalculateCostUnified(input)
	require.NoError(t, err)
	require.NotNil(t, cost)

	// video 走 calculatePerRequestCost，用默认兜底价：2 * $0.20 = $0.40
	require.InDelta(t, 0.40, cost.TotalCost, 1e-10)
	require.Equal(t, string(BillingModeVideo), cost.BillingMode)
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test -tags=unit -run 'TestCalculateCostUnified_VideoMode' ./internal/service/`
Expected: FAIL（video 未被识别 → 走 token 路径 → `TotalCost=0` 或 `BillingMode != "video"`）。

- [ ] **Step 3: 实现 — `Resolve`（`model_pricing_resolver.go` L75）**

```go
		if mode == BillingModePerRequest || mode == BillingModeImage || mode == BillingModeVideo {
```

- [ ] **Step 4: 实现 — `applyChannelOverrides`（`model_pricing_resolver.go` L137）**

```go
	case BillingModePerRequest, BillingModeImage, BillingModeVideo:
		r.applyRequestTierOverrides(chPricing, resolved)
```

- [ ] **Step 5: 实现 — `CalculateCostUnified`（`billing_service.go` L818-823）**

```go
	switch resolved.Mode {
	case BillingModePerRequest, BillingModeImage, BillingModeVideo:
		breakdown, err = s.calculatePerRequestCost(resolved, input)
	default: // BillingModeToken
		breakdown, err = s.calculateTokenCost(resolved, input)
	}
```

- [ ] **Step 6: 跑测试确认通过**

Run: `cd backend && go test -tags=unit -run 'TestCalculateCostUnified_VideoMode|TestCalculateCostUnified_ImageMode|TestCalculateCostUnified_PerRequestMode' ./internal/service/`
Expected: PASS（image/per_request 回归不破）。

- [ ] **Step 7: 全包测试 + lint + commit**

Run: `cd backend && go test -tags=unit ./internal/service/` 与 `cd backend && golangci-lint run ./internal/service/...`
```bash
git add backend/internal/service/model_pricing_resolver.go backend/internal/service/billing_service.go backend/internal/service/billing_service_unified_test.go
git commit -m "feat(billing): video 模式接入计费分发骨架

Resolve/applyChannelOverrides 识别 video，CalculateCostUnified 分发到
calculatePerRequestCost。视频请求金额入口本轮不改，待分辨率解析专项。

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 4: 前端 — 计费模式与分辨率档位枚举

**Files:**
- Modify: `frontend/src/constants/channel.ts`（L6-13）
- Modify: `frontend/src/api/admin/channels.ts`（`BillingMode` 类型定义处）
- Test: `cd frontend && pnpm run typecheck`

**Interfaces:**
- Produces: `BILLING_MODE_VIDEO`、`IMAGE_RESOLUTION_OPTIONS`、`VIDEO_RESOLUTION_OPTIONS`，供 Task 5/6/7 使用。

- [ ] **Step 1: 实现 — `constants/channel.ts`**

把 L6-13 的 BillingMode 块改为：
```ts
/** Billing mode values (must match service.BillingMode* constants in Go). */
export const BILLING_MODE_TOKEN = 'token' as const
export const BILLING_MODE_PER_REQUEST = 'per_request' as const
export const BILLING_MODE_IMAGE = 'image' as const
export const BILLING_MODE_VIDEO = 'video' as const
export type BillingMode =
  | typeof BILLING_MODE_TOKEN
  | typeof BILLING_MODE_PER_REQUEST
  | typeof BILLING_MODE_IMAGE
  | typeof BILLING_MODE_VIDEO

/** 图片计费分辨率档位（与后端 ClassifyImageBillingTier 输出对齐）。 */
export const IMAGE_RESOLUTION_OPTIONS = [
  { value: '1K', label: '1K' },
  { value: '2K', label: '2K' },
  { value: '4K', label: '4K' },
] as const

/** 视频计费分辨率档位（后端解析链路待建，本轮仅前端配置）。 */
export const VIDEO_RESOLUTION_OPTIONS = [
  { value: '480P', label: '480P' },
  { value: '720P', label: '720P' },
  { value: '1080P', label: '1080P' },
  { value: '4K', label: '4K' },
] as const
```

- [ ] **Step 2: 实现 — `api/admin/channels.ts`**

搜索该文件中 `BillingMode` 类型定义（若 re-export 自 `constants/channel.ts` 则无需改；若是独立定义，把 `'video'` 并入联合类型，与 `constants/channel.ts` 保持一致）。判断方式：`grep -n "BillingMode" frontend/src/api/admin/channels.ts`。

- [ ] **Step 3: typecheck**

Run: `cd frontend && pnpm run typecheck`
Expected: 无错误（此时 `video` 还没被组件消费，但类型已就绪）。

- [ ] **Step 4: commit**

```bash
git add frontend/src/constants/channel.ts frontend/src/api/admin/channels.ts
git commit -m "feat(channel-pricing): 前端计费模式 video 与分辨率档位枚举

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 5: 前端 — `types.ts` 档位纯函数 + `validateIntervals` 加严

**Files:**
- Modify: `frontend/src/components/admin/channel/types.ts`
- Create: `frontend/src/components/admin/channel/__tests__/types.spec.ts`
- Modify: `frontend/src/i18n/locales/zh.ts` / `en.ts`

**Interfaces:**
- Produces: `resolutionOptionsForMode(mode)` 返回该模式的 Select 选项；`nextDefaultTierLabel(mode, count)` 返回第 `count+1` 个预填档位；`validateIntervals` 对 image/video 校验「合法枚举 + 不重复」。

- [ ] **Step 1: 写失败测试**（`__tests__/types.spec.ts`）

```ts
import { describe, it, expect } from 'vitest'
import {
  resolutionOptionsForMode,
  nextDefaultTierLabel,
  validateIntervals,
  type IntervalFormEntry,
} from '../types'
import { BILLING_MODE_IMAGE, BILLING_MODE_VIDEO, BILLING_MODE_PER_REQUEST, BILLING_MODE_TOKEN } from '@/constants/channel'

function tier(label: string): IntervalFormEntry {
  return {
    min_tokens: 0, max_tokens: null, tier_label: label,
    input_price: null, output_price: null, cache_write_price: null,
    cache_read_price: null, per_request_price: 0.1, sort_order: 0,
  }
}

describe('resolutionOptionsForMode', () => {
  it('image 返回 1K/2K/4K', () => {
    const vals = resolutionOptionsForMode(BILLING_MODE_IMAGE).map(o => o.value)
    expect(vals).toEqual(['1K', '2K', '4K'])
  })
  it('video 返回 480P/720P/1080P/4K', () => {
    const vals = resolutionOptionsForMode(BILLING_MODE_VIDEO).map(o => o.value)
    expect(vals).toEqual(['480P', '720P', '1080P', '4K'])
  })
  it('per_request / token 返回空数组（自由文本/不适用）', () => {
    expect(resolutionOptionsForMode(BILLING_MODE_PER_REQUEST)).toEqual([])
    expect(resolutionOptionsForMode(BILLING_MODE_TOKEN)).toEqual([])
  })
})

describe('nextDefaultTierLabel', () => {
  it('image 按 1K/2K/4K 顺序预填', () => {
    expect(nextDefaultTierLabel(BILLING_MODE_IMAGE, 0)).toBe('1K')
    expect(nextDefaultTierLabel(BILLING_MODE_IMAGE, 1)).toBe('2K')
    expect(nextDefaultTierLabel(BILLING_MODE_IMAGE, 3)).toBe('') // 超出预设
  })
  it('video 按 480P/720P/1080P/4K 顺序预填', () => {
    expect(nextDefaultTierLabel(BILLING_MODE_VIDEO, 0)).toBe('480P')
    expect(nextDefaultTierLabel(BILLING_MODE_VIDEO, 3)).toBe('4K')
    expect(nextDefaultTierLabel(BILLING_MODE_VIDEO, 4)).toBe('')
  })
})

describe('validateIntervals image/video', () => {
  it('image 非白名单 tier_label 报错', () => {
    expect(validateIntervals([tier('HD')], BILLING_MODE_IMAGE)).not.toBeNull()
  })
  it('image 合法档位通过', () => {
    expect(validateIntervals([tier('1K')], BILLING_MODE_IMAGE)).toBeNull()
  })
  it('image 同档位重复报错', () => {
    expect(validateIntervals([tier('1K'), tier('1K')], BILLING_MODE_IMAGE)).not.toBeNull()
  })
  it('video 非白名单报错', () => {
    expect(validateIntervals([tier('5K')], BILLING_MODE_VIDEO)).not.toBeNull()
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd frontend && pnpm run test:run -- types.spec`
Expected: FAIL（`resolutionOptionsForMode`/`nextDefaultTierLabel` 未定义）。

- [ ] **Step 3: 实现 — `types.ts`**

在文件顶部 import 后追加：
```ts
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_VIDEO,
  IMAGE_RESOLUTION_OPTIONS,
  VIDEO_RESOLUTION_OPTIONS,
  type BillingMode,
} from '@/constants/channel'
```
（若 `BillingMode` 已从 `@/api/admin/channels` import，保持原样即可；新常量从 `@/constants/channel` 引入。）

在文件合适位置（如 `validateIntervals` 之前）追加纯函数：
```ts
/** 返回指定模式可用的分辨率档位选项（image/video）；其他模式返回空数组。 */
export function resolutionOptionsForMode(mode: BillingMode): { value: string; label: string }[] {
  switch (mode) {
    case BILLING_MODE_IMAGE:
      return [...IMAGE_RESOLUTION_OPTIONS]
    case BILLING_MODE_VIDEO:
      return [...VIDEO_RESOLUTION_OPTIONS]
    default:
      return []
  }
}

/** 按 mode 的预设档位顺序，返回第 count+1 个预填 label；超出范围返回空串。 */
export function nextDefaultTierLabel(mode: BillingMode, count: number): string {
  const opts = resolutionOptionsForMode(mode)
  return count >= 0 && count < opts.length ? opts[count].value : ''
}
```

修改 `validateIntervals`：在 `if (mode !== 'token') return null`（约 L140）**之前**插入 image/video 枚举与重复校验：
```ts
  // image / video 模式：tier_label 必须是合法枚举值，且不可重复
  if (mode === 'image' || mode === 'video') {
    const allowed = resolutionOptionsForMode(mode).map(o => o.value)
    const seen = new Set<string>()
    for (let i = 0; i < sorted.length; i++) {
      const label = sorted[i].tier_label
      if (!allowed.includes(label)) {
        return `层级 #${i + 1}: 分辨率「${label || '空'}」不在可选范围（${allowed.join('/')}）`
      }
      if (seen.has(label)) {
        return `层级 #${i + 1}: 分辨率「${label}」重复`
      }
      seen.add(label)
    }
    return null
  }
```
（保留其后 `if (mode !== 'token') return null` 与 token 重叠校验不变。）

- [ ] **Step 4: 跑测试确认通过**

Run: `cd frontend && pnpm run test:run -- types.spec`
Expected: PASS。

- [ ] **Step 5: i18n — `zh.ts` / `en.ts`**

在 `admin.channels.billingMode` 下补 video 标签；在 `admin.channels.form` 下补 video 层级与提示文案。

`zh.ts`（`billingMode` 块，参照现有 `image` 行追加）：
```ts
video: '视频（按次）',
```
`zh.ts`（`form` 块追加）：
```ts
videoTiers: '视频计费层级（按次）',
resolutionNotMatched: '该档位无法匹配，请重新选择',
```
`en.ts` 对应：
```ts
video: 'Video (per request)',
videoTiers: 'Video billing tiers (per request)',
resolutionNotMatched: 'This tier cannot be matched, please reselect',
```
（具体行号执行时用 `grep -n "billingMode:" frontend/src/i18n/locales/zh.ts` 定位。）

- [ ] **Step 6: typecheck + commit**

Run: `cd frontend && pnpm run typecheck`
```bash
git add frontend/src/components/admin/channel/types.ts frontend/src/components/admin/channel/__tests__/types.spec.ts frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "feat(channel-pricing): 分辨率档位纯函数与 validateIntervals 校验加严

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 6: 前端 — `IntervalRow.vue` 分辨率下拉框化

**Files:**
- Modify: `frontend/src/components/admin/channel/IntervalRow.vue`（template L38-62、script import）

**Interfaces:**
- Consumes: Task 5 的 `resolutionOptionsForMode`、Task 4 的 `Select.vue`（既有）。
- Produces: image/video 模式渲染 `Select`（选项来自 `resolutionOptionsForMode(mode)`）并传 `:error` 标红；移除该分支的 Min/Max 框；per_request 保持自由文本。

- [ ] **Step 1: 实现 — template（替换 L38-62 的 `<template v-else>` 整块）**

```vue
    <!-- Per-request mode: 自由文本层级（语义通用，保持手输） -->
    <template v-else-if="mode === 'per_request'">
      <div class="w-24">
        <label class="text-xs text-gray-400">{{ t('admin.channels.form.tierLabel', '层级') }}</label>
        <input :value="interval.tier_label" @input="emitField('tier_label', ($event.target as HTMLInputElement).value)"
          type="text" class="input mt-0.5 text-xs" />
      </div>
      <div class="flex-1">
        <label class="text-xs text-gray-400">{{ t('admin.channels.form.perRequestPrice', '单次价格') }} <span v-if="isEmpty" class="text-red-500">*</span> <span class="text-gray-300">$</span></label>
        <input :value="interval.per_request_price" @input="emitField('per_request_price', ($event.target as HTMLInputElement).value)"
          type="number" step="any" min="0" class="input mt-0.5 text-xs" />
      </div>
    </template>

    <!-- Image / Video mode: 分辨率下拉框（固定枚举）+ 单次价格 -->
    <template v-else>
      <div class="w-28">
        <label class="text-xs text-gray-400">
          {{ t('admin.channels.form.resolution', '分辨率') }}
          <span v-if="isTierInvalid" class="text-red-500">*</span>
        </label>
        <Select
          :modelValue="interval.tier_label"
          @update:modelValue="emitField('tier_label', ($event as string) ?? '')"
          :options="resolutionOptionsForMode(mode)"
          :error="isTierInvalid"
          :placeholder="t('admin.channels.form.resolutionNotMatched', '该档位无法匹配，请重新选择')"
          class="mt-0.5"
        />
      </div>
      <div class="flex-1">
        <label class="text-xs text-gray-400">{{ t('admin.channels.form.perRequestPrice', '单次价格') }} <span v-if="isEmpty" class="text-red-500">*</span> <span class="text-gray-300">$</span></label>
        <input :value="interval.per_request_price" @input="emitField('per_request_price', ($event.target as HTMLInputElement).value)"
          type="number" step="any" min="0" class="input mt-0.5 text-xs" />
      </div>
    </template>
```

- [ ] **Step 2: 实现 — script（import Select + 纯函数 + 派生状态）**

import 区追加：
```ts
import Select from '@/components/common/Select.vue'
import { resolutionOptionsForMode } from './types'
```
在 `const isEmpty = computed(...)` 之后追加：
```ts
// 当前 tier_label 是否不在该模式合法枚举内（旧非标数据 → 标红提示）
const isTierInvalid = computed(() => {
  const opts = resolutionOptionsForMode(props.mode)
  if (opts.length === 0) return false // per_request/token 不校验
  return !opts.some(o => o.value === props.interval.tier_label)
})
```

- [ ] **Step 3: typecheck**

Run: `cd frontend && pnpm run typecheck`
Expected: 无错误。

- [ ] **Step 4: 跑既有前端测试确认无回归**

Run: `cd frontend && pnpm run test:run`
Expected: 既有测试全绿（本任务不改纯函数逻辑）。

- [ ] **Step 5: commit**

```bash
git add frontend/src/components/admin/channel/IntervalRow.vue
git commit -m "feat(channel-pricing): IntervalRow 图片/视频分辨率下拉框化

移除 image/video 模式无用的 Min/Max，非标 tier_label 标红提示。

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 7: 前端 — `PricingEntryCard.vue` video 模式分支 + `addImageTier` 泛化

**Files:**
- Modify: `frontend/src/components/admin/channel/PricingEntryCard.vue`（`billingModeOptions` L256-260、image 分支后新增 video 分支 L223 处、`addImageTier` L282-292）

**Interfaces:**
- Consumes: Task 4 的 `BILLING_MODE_VIDEO`、Task 5 的 `nextDefaultTierLabel`。
- Produces: 计费模式下拉含「视频（按次）」；video 模式渲染默认价 + 层级列表；新增层级按模式预填合法档位（image 1K/2K/4K，video 480P/720P/1080P/4K，去掉 HD）。

- [ ] **Step 1: 实现 — `billingModeOptions`（L256-260）**

```ts
const billingModeOptions = computed(() => [
  { value: 'token', label: 'Token' },
  { value: 'per_request', label: t('admin.channels.billingMode.perRequest', '按次') },
  { value: 'image', label: t('admin.channels.billingMode.image', '图片（按次）') },
  { value: 'video', label: t('admin.channels.billingMode.video', '视频（按次）') },
])
```

- [ ] **Step 2: 实现 — 新增 video 模板分支（紧跟 image 分支 `<div v-else-if="entry.billing_mode === 'image'">...</div>` 之后，即原 L223 `</div>` 后）**

```vue
        <!-- Video mode -->
        <div v-else-if="entry.billing_mode === 'video'">
          <label class="mt-3 block text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('admin.channels.form.defaultImagePrice', '默认单次价格（未命中层级时使用）') }}
            <span class="ml-1 font-normal text-gray-400">$</span>
          </label>
          <div class="mt-1 w-48">
            <input :value="entry.per_request_price" @input="emitField('per_request_price', ($event.target as HTMLInputElement).value)"
              type="number" step="any" min="0" class="input text-sm" :placeholder="t('admin.channels.form.pricePlaceholder', '默认')" />
          </div>

          <div class="mt-3 flex items-center justify-between">
            <label class="text-xs font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.channels.form.videoTiers', '视频计费层级（按次）') }}
            </label>
            <button type="button" @click="addVideoTier" class="text-xs text-primary-600 hover:text-primary-700">
              + {{ t('admin.channels.form.addTier', '添加层级') }}
            </button>
          </div>
          <div v-if="entry.intervals && entry.intervals.length > 0" class="mt-2 space-y-2">
            <IntervalRow
              v-for="(iv, idx) in entry.intervals"
              :key="idx"
              :interval="iv"
              :mode="entry.billing_mode"
              @update="updateInterval(idx, $event)"
              @remove="removeInterval(idx)"
            />
          </div>
        </div>
```

- [ ] **Step 3: 实现 — `addImageTier` 泛化 + 新增 `addVideoTier`（替换 L282-292）**

```ts
function addImageTier() {
  addTierForMode('image')
}

function addVideoTier() {
  addTierForMode('video')
}

// 按模式预填下一个合法档位（image: 1K/2K/4K；video: 480P/720P/1080P/4K）
function addTierForMode(mode: 'image' | 'video') {
  const intervals = [...(props.entry.intervals || [])]
  const tierLabel = nextDefaultTierLabel(mode as BillingMode, intervals.length)
  intervals.push({
    min_tokens: 0, max_tokens: null, tier_label: tierLabel,
    input_price: null, output_price: null, cache_write_price: null,
    cache_read_price: null, per_request_price: null,
    sort_order: intervals.length,
  })
  emit('update', { ...props.entry, intervals })
}
```
并在 import 区追加：
```ts
import { nextDefaultTierLabel } from './types'
import { BILLING_MODE_VIDEO } from '@/constants/channel'
import type { BillingMode } from '@/api/admin/channels'
```
（`BillingMode` 已 import 则不重复；`BILLING_MODE_VIDEO` 若未用到可省，但 typecheck 会提示。）

- [ ] **Step 4: typecheck + lint**

Run: `cd frontend && pnpm run typecheck` 与 `cd frontend && pnpm run lint:check`
Expected: 无错误。

- [ ] **Step 5: 跑前端测试确认无回归**

Run: `cd frontend && pnpm run test:run`
Expected: 全绿。

- [ ] **Step 6: commit**

```bash
git add frontend/src/components/admin/channel/PricingEntryCard.vue
git commit -m "feat(channel-pricing): PricingEntryCard 新增 video 模式分支与档位预填

addImageTier/addVideoTier 按模式预填合法分辨率档位（去掉 HD）。

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## 收尾验证

- [ ] **后端全量**：`cd backend && go test -tags=unit ./...` 与 `cd backend && golangci-lint run ./...`
- [ ] **前端全量**：`cd frontend && pnpm run typecheck && pnpm run lint:check && pnpm run test:run`
- [ ] **人工核对**（构建后或 dev server）：渠道编辑页 → 新增定价配置 → 切 image 模式看到分辨率下拉框（1K/2K/4K，无 HD）；切 video 模式看到 480P/720P/1080P/4K；切 per_request 仍是自由文本；旧非标 tier_label 标红。

## Self-Review 结论

- **Spec 覆盖**：5.1（video 常量）→ Task 1/4；5.2（档位枚举）→ Task 4/5；5.3 前端（IntervalRow/PricingEntryCard/types/i18n）→ Task 5/6/7；5.4 后端（大小写 Task 2、video 骨架 Task 3）；5.5 旧数据（标红 Task 6 `isTierInvalid` + 后端归一化见下注）。无遗漏。
- **注（后端 tier_label 归一化）**：spec 5.5 提到「后端加载 intervals 时 ToUpper 归一化」。Task 2 的 `strings.EqualFold` 已使匹配大小写不敏感，等效覆盖了「DB 小写 → 仍命中」的核心诉求；DB 写入侧的强制 ToUpper 为可选增强，未单列任务（避免触碰 repo 读写层），由 `EqualFold` 兜底。
- **类型一致**：`BillingModeVideo`/`BILLING_MODE_VIDEO`/`resolutionOptionsForMode`/`nextDefaultTierLabel` 在产出与消费任务间命名一致。
- **无占位符**：所有步骤含完整代码与确切命令。
