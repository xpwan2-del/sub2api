# 套餐推荐标识 + 可购套餐卡片重构 设计文档

> 日期: 2026-07-30 · 分支: `feat/model-catalog-ops`
> 范围: 前端 UI 优化,共 3 个文件,无后端 / 数据库 / API 变更。

## 1. 目标

两处独立优化:

1. **管理端列表推荐标识**:在「套餐方案」「套餐订阅」两个列表的「套餐名称」后追加「推荐」标识(不新增列,直接跟在名称后)。仅当该套餐 `featured === true` 时显示。
2. **可购套餐卡片重构**:重构用户端「我的账户 → 套餐订阅 → 可购套餐」卡片(`BundlePlanCard.vue`)的视觉风格。**字段 / 数据 / 交互一律保持不变**,只改视觉。方向:**现代玻璃渐变(浅色玻璃 + 品牌渐变)**。

## 2. 现状诊断

- `featured`(boolean)字段后端有、类型有、管理端表单可配(琥珀色开关),但**两个列表与用户端卡片均未在视觉上展示**。
- 套餐方案列表 `BundlePlansView.vue` 的 `name` 列当前无 `#cell-name` 插槽,走 `DataTable` 纯文本默认渲染。
- 套餐订阅列表 `BundleSubscriptionsView.vue` 的名称走 `#cell-plan_id` 插槽,显示 `row.plan?.name`。
- 用户端卡片 `BundlePlanCard.vue`(被 `BundlesView.vue` 消费)的问题:
  1. tier 配色被撒到 7 处(顶部色条 / 边框 / 徽章 / 价格 / 勾选图标 / 折扣徽章 / 按钮),视觉嘈杂;
  2. 无静态阴影(仅 hover 出阴影),平铺时层次扁平;
  3. `p-4` 留白不足,信息密度高;
  4. 没用项目既定设计系统(主色 teal `#14b8a6`、`bg-gradient-primary`、`shadow-glass/card` token),底部按钮是父级注入的 per-tier 蓝/紫/金纯色,与全站主色割裂。

## 3. 设计

### 3.1 任务 1 — 管理端「推荐」标识

| 文件 | 改动 | 数据源 |
|---|---|---|
| `views/admin/bundles/BundlePlansView.vue` | 新增 `#cell-name` 插槽,名称后追加 `v-if="row.featured"` 徽章 | `row.featured` |
| `views/admin/bundles/BundleSubscriptionsView.vue` | 改 `#cell-plan_id` 插槽,名称后追加 `v-if="row.plan?.featured"` 徽章 | `row.plan?.featured` |

徽章样式(主色青文字药丸):

```
inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium
bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300
```

文案复用现有 i18n 键 `bundles.admin.featured`(推荐 / Featured),不新增翻译。尺寸对齐现有 status/source 徽章;`v-if` 守卫,非推荐套餐零影响。

### 3.2 任务 2 — 可购套餐卡片重构(玻璃渐变)

涉及 `BundlePlanCard.vue`(卡片本体)+ `BundlesView.vue`(底部 4 种按钮)。核心原则:**收敛 tier 配色噪声 → tier 身份仅保留在徽章;其余统一回全站青色主色;引入玻璃质感 + 品牌渐变。**

| 区块 | 现状 | 重构后 |
|---|---|---|
| 容器 | 实色 `bg-white` + per-tier 边框,无静态阴影 | 玻璃:`bg-white/70 dark:bg-dark-800/70 backdrop-blur-sm` + 中性玻璃边 `border-white/60` + 静态 `shadow-glass` / 悬停 `hover:shadow-card-hover` |
| 渐变层 | 无 | 顶部叠加柔和品牌 wash(子 div):`absolute inset-x-0 top-0 h-24 bg-gradient-to-br from-primary-500/10 via-primary-400/5 to-transparent` |
| 顶部色条 | per-tier 渐变(蓝/紫/金) | 统一品牌 `bg-gradient-primary`(青) |
| 套餐名 | `text-base font-bold` | `text-lg font-bold`(强化标题层级) |
| tier 徽章 | per-tier 色 | **保留 per-tier 色**(tier 身份唯一载体) |
| 价格 | `text-2xl` + tier 色 | `text-2xl` + 中性 `text-gray-900 dark:text-white`(横向比价不被染色) |
| 折扣徽章 | tier 色 | 主色 `bg-primary-100 text-primary-700` |
| Features 区 | `bg-gray-50` 包裹盒 + tier 色勾选 | 去灰底盒 + 顶部细分隔线 + 主色勾选 `text-primary-500`,`space-y-2` |
| 配额/平台 chips | `bg-white border-gray-100`(白底白卡无边界) | 玻璃 `bg-white/60 dark:bg-dark-700/40 border-primary-100/60` |
| 并发/RPM 图标 | 灰 | 主色淡染 |
| 底部 CTA(父级) | per-tier 蓝/紫/金纯色按钮 | 可点按钮统一 `bg-gradient-primary text-white shadow-lg shadow-primary-500/25 hover:shadow-glow`;置灰按钮统一中性 `bg-gray-100 text-gray-400` |

### 3.3 不变项(明确承诺)

- `BundlePlanCard` 的 props / emits / `BundlePlanCardData` 字段;
- `#actions` 插槽契约与 4 种按钮的判定逻辑(`isCurrentPlan` / `isUpgradeTarget` / `handlePurchase` / `handleUpgradeClick`);
- 所有 `v-if` 数据守卫、quota tooltip、platform 徽章、`group_quotas` ↔ `platforms` 回落逻辑;
- `bundleTiers.ts` 共享主题文件**不动**(管理端、活跃套餐卡不受影响);
- 网格列数 `planGridClass`(≤2 双列 / ≥3 三列);
- 无新增/删除字段,无 API / i18n key 新增(任务 1 复用现有 key)。

## 4. 实施步骤

1. **任务 1**:`BundlePlansView.vue` 加 `#cell-name` 插槽;`BundleSubscriptionsView.vue` 改 `#cell-plan_id` 插槽。
2. **任务 2 - 卡片**:`BundlePlanCard.vue` 重写 template class(容器玻璃化、加渐变层、顶部色条统一、名/价格/勾选/折扣/chip 换色、去灰底盒)。卡片仅保留 `tierBadgeClass`(tier 徽章 per-tier 色);顶部色条、边框、价格、勾选、折扣、chip 改为内联品牌渐变/主色/中性 class,不再调用 `tierAccentClass` / `tierBorderClass` / `tierTextClass` / `tierIconClass` / `tierDiscountClass`。
3. **任务 2 - 按钮**:`BundlesView.vue` 4 种按钮 class 统一为品牌渐变 / 中性置灰;清理因此变为未使用的 tier 按钮工具函数(若仅此处引用)。
4. **验证**:`pnpm run typecheck` + `pnpm run lint:check`;`pnpm exec vitest run` 跑相关组件单测(`BundlePlanCard` 若有);目视核查浅色/深色两套主题。

### 实现注记

- **顶部色条统一为品牌渐变**:卡片内不再依赖 `tierAccentClass`(该函数仍保留在 `bundleTiers.ts` 供活跃套餐卡等使用),直接用 `bg-gradient-primary`。
- **深色模式**:`darkMode: 'class'`,所有新增 class 需带 `dark:` 变体(玻璃背景、wash、chips 边框、置灰按钮)。
- **未引用清理**:`BundlePlanCard.vue` 停用的 tier 工具函数导入与本地包装函数(`tierBorderClass`/`tierTextClass`/`tierIconClass`/`tierDiscountClass`)需一并删除,避免 ESLint 未使用告警;`BundlesView.vue` 同理处理 `tierBtnClass`/`tierDisabledBtnClass`(确认活跃套餐卡未引用后删除)。
- **不新增 scoped CSS**:玻璃 wash 用绝对定位子 div + Tailwind 实现,保持该文件「纯 utility class」现状。

## 5. 风险

- **低**:tier 视觉差异减弱(仅徽章区分)。可接受——这正是「降噪」目标;tier 文本仍清晰可读。
- **低**:玻璃 `backdrop-blur` 在纯色浅背景上效果有限,纵深主要由 `shadow-glass` + 渐变 wash 承担。
- **可规避**:删除 tier 工具函数前需 grep 确认无其它引用,避免编译错误。
