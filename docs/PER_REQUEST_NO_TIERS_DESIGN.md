# per_request（按次）计费模式移除层级 — 设计文档

日期：2026-07-10

## 背景

渠道创建/编辑弹框中，"模型定价"的每条定价配置（`PricingEntryCard`）可选三种计费模式：

- `token` — Token 区间计费
- `per_request` — 按次计费
- `image` — 图片计费（按次）

当前 `per_request` 模式除"默认单次价格"外，还支持添加"按次计费层级"（tier）。业务上 per_request 模式不需要分层，层级能力应仅保留给 image 模式。

后端只认这三种合法 `BillingMode`，不存在独立的"视频按次"模式——用户提到的"图片按次和视频按次不影响"，实际对应 `image` 模式保持不变。

## 目标

1. `per_request` 模式移除层级能力，仅保留"默认单次价格"输入框
2. 文案"默认单次价格（未命中层级时使用）"→"默认单次价格"（层级已不存在，括号说明失效）
3. `image`（图片按次）模式完全不变
4. 编辑已有渠道时，per_request 历史层级数据隐藏不展示，保存时清空

## 非目标

- 不改动 token 模式
- 不改动 image 模式
- 不改动后端计费逻辑

## 改动

### 1. `frontend/src/components/admin/channel/PricingEntryCard.vue`

模板 `per_request` 分支（第 156-190 行）删除层级区域，仅保留默认单次价格 label + 输入框：

- 删除：「按次计费层级」标题 +「+ 添加层级」按钮（168-176）
- 删除：IntervalRow 层级列表（177-186）
- 删除：「暂无层级…」空状态（187-189）

脚本：`addInterval`（token 用）、`addImageTier`（image 用）保留不动。

### 2. `frontend/src/views/admin/ChannelsView.vue`

两个提交转换点对 per_request 强制清空 intervals：

- `accountStatsRulesToAPI`（第 1071 行）
- `formToAPI`（第 1111 行）

```ts
intervals: entry.billing_mode === 'per_request' ? [] : formIntervalsToAPI(entry.intervals || [])
```

校验逻辑（1484-1495）不改：per_request 提交时 intervals 为空，校验自然要求填默认价格。

### 3. i18n（`frontend/src/i18n/locales/zh.ts` + `en.ts`）

- `defaultPerRequestPrice`：去括号
  - zh: `'默认单次价格（未命中层级时使用）'` → `'默认单次价格'`
  - en: `'Default per-request price (fallback when no tier matches)'` → `'Default per-request price'`
- `defaultImagePrice` 不动
- 清理 dead key：`requestTiers`、`noTiersYet`（仅 per_request 用过，现已无引用）；`addTier` 保留（image 用）

## 依赖与不变项

- **后端无需改动**：`model_pricing_resolver.go:266-277` + `billing_service.go:994` 在 per_request 无 tier 时回退到 `DefaultPerRequestPrice`，清空 intervals 后按默认单次价格计费
- 后端校验 `channel_service.go:629`（per_request 须有价格或层级）与前端一致
- image 模式计费、层级、文案均不变

## 测试

- 新建渠道：添加定价 → 选"按次"→ 仅显示单次价格输入框，无层级入口；选"图片（按次）"→ 层级入口仍在
- 编辑渠道：历史 per_request 层级渠道，打开编辑不显示层级，保存后后端 intervals 清空、按默认单次价格计费
- 同步更新 `PricingEntryCard` / `ChannelsView` 相关现有 spec（若有断言 per_request 层级行为）
