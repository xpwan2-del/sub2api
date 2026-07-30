# 模型广场套餐区重新设计 · Design Spec

- **日期**: 2026-07-30
- **分支**: feat/model-catalog-ops
- **范围**: 仅公开首页「模型广场」(`ModelCatalogView`)顶部的套餐展示区(`BundleShowcaseSection`)
- **状态**: 待评审

---

## 1. 背景与目标

公开首页模型广场(`src/views/public/ModelCatalogView.vue`,路由 `/`)顶部嵌有一个**套餐展示区**(`BundleShowcaseSection`),用于向访客(含未登录用户)展示可购套餐并引导进入购买流程。

### 问题
模型广场主体(`ModelCard` 模型卡 + `ModelCatalogHeader` + `ModelCatalogFilters`)是一套精心设计的**「深色 HUD / 赛博终端」视觉语言**:常黑深蓝渐变底、mint/teal 半透明描边、`clip-path` 切角、内发光 + 重投影、monospace 数据字体、teal/cyan 单色系、饱和渐变运营 badge。

但套餐区当前**直接复用** `src/components/bundles/BundlePlanCard.vue`——一个为「白底圆角浅色 + tier 多色(blue/purple/amber) + sans-serif」设计的标准卡片。两者视觉语言几乎完全不同,套餐区像一张被插进科技感页面里的普通电商卡,割裂感明显。

### 目标
把模型广场顶部套餐区**重新设计为与主体一致的深色 HUD 风格**,让整页视觉统一,同时保留套餐作为「阶梯定价产品」该有的信息传达与转化能力。

---

## 2. 范围

### In scope
- `src/components/models/BundleShowcaseSection.vue`(区块 header + 网格容器)整体重新设计。
- 新建 `src/components/models/ShowcaseBundleCard.vue`(模型广场专属套餐卡),替换对 `BundlePlanCard` 的复用。
- 区块 header 对齐 `ModelCatalogHeader` 语言。

### Out of scope(明确不动)
- `src/components/bundles/BundlePlanCard.vue` **不改**(用户中心 `/bundles` 仍在用,保持现状)。
- `src/views/user/BundlesView.vue`、`BundleUsageView.vue` 不改。
- 后端接口不改(沿用 `getPublicBundlePlans` 返回的 `PublicBundlePlan`)。
- 模型卡 `ModelCard` 及其它模型广场组件不改。
- `/bundles` 路由行为不改。

---

## 3. 现状关键事实

### 3.1 数据源:`PublicBundlePlan`(公开裁剪版,无鉴权)
来自 `src/api/publicBundles.ts` 的 `getPublicBundlePlans()`。可用展示字段:
`id`、`name`、`description`、`tier`(`starter`/`pro`/`enterprise`)、`price`、`original_price`、`currency`、`validity_days`、`features: string[]`、`platforms: string[]`(后端从 group_quotas 去重聚合)。

**关键约束**:`PublicBundlePlan` **不返回** `concurrency_limit`、`rpm_limit`、`group_quotas` 等敏感/精确字段(这些只在登录后的 `/bundles` 页有)。因此公开套餐卡**不展示**并发/RPM/精确额度。

### 3.2 现状组件链
`ModelCatalogView` → `BundleShowcaseSection`(薄壳:header + 网格)→ 复用 `BundlePlanCard`(`showActions=false`)。`BundleShowcaseSection` 把 `PublicBundlePlan` 经 `toCardData()` 映射成 `BundlePlanCardData` 后渲染;整卡点击跳 `/bundles`;"查看全部" CTA 跳 `/bundles`。

### 3.3 模型广场视觉 DNA(精确基准值,新组件直接复用)
| 项 | 值 |
|---|---|
| 卡片底 | `linear-gradient(145deg, rgba(15,23,42,0.94), rgba(3,7,18,0.86))` |
| 描边 | `1px solid rgba(94,234,212,0.20)` |
| 切角 | `clip-path: polygon(0 0, calc(100% - 18px) 0, 100% 18px, 100% 100%, 18px 100%, 0 calc(100% - 18px))` |
| 顶部高光 | `::before { background: radial-gradient(circle at 20% 0%, rgba(34,211,238,0.16), transparent 38%) }` |
| 投影 | `0 26px 60px rgba(0,0,0,0.32), inset 0 0 32px rgba(45,212,191,0.04)` |
| 主文字色 | `#f8fafc` |
| 次文字色 | `rgba(203,213,225,0.78)` / 弱 `rgba(226,232,240,0.62)` |
| 数据字体 | `ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace` |
| 强调渐变 | `linear-gradient(135deg, rgba(20,184,166,0.96), rgba(34,211,238,0.88))`,渐变上文字 `#022622` |
| 网格 gap | `18px`(与模型卡网格一致) |
| 卡 padding | `20–22px` |

---

## 4. 设计决策

### 4.1 组件方案:新建 `ShowcaseBundleCard`,不覆盖 `BundlePlanCard`
- `BundlePlanCard` 的 DOM 结构是「白底语义」(顶部 tier 色条、灰底 features 块、浅底 tier badge),用 `:deep()` / scoped CSS 把它强行覆盖成「深色切角 HUD」非常脆弱,且任何对 `BundlePlanCard` 的改动都会同时影响 `/bundles`。
- 新建 `ShowcaseBundleCard.vue`,专注模型广场场景,接口单一、可独立测试、零副作用。
- `BundleShowcaseSection` 改用 `ShowcaseBundleCard`,移除对 `BundlePlanCard` 与 `toCardData()` 映射的依赖。

### 4.2 视觉规范:1:1 对齐模型广场 DNA
`ShowcaseBundleCard` 用 `<style scoped>` + `isDark` prop 实现深色 HUD(与 `ModelCard` 同机制),**不**用 Tailwind `dark:` 变体(避免引入第二套暗色实现)。具体 CSS 值见 §3.3。

### 4.3 tier 配色策略:方案 C(teal 单色 + 推荐 badge)
- 所有套餐卡统一 teal/cyan 主调(mint 描边 + teal 顶条/glow + cyan 价格字),与 `ModelCard` 同源。
- tier 身份靠 **monospace 文字标签**(`STARTER · 入门版` / `PRO · 专业版` / `ENTERPRISE · 企业版`)+ 价格 + 卡片排序传达,**不**用 blue/purple/amber 多色。
- **主推套餐**贴饱和渐变 `★ 推荐` badge(复用模型广场运营 badge 语言:new/featured/recommended),并把该卡描边/glow 略加强、CTA 用实心渐变(其余卡 CTA 为描边 ghost)。
- **featured 判定**:`BundleShowcaseSection` 计算,默认 `tier === 'pro'` 的套餐为 featured;若不存在 pro,回落到按价格升序的中间档;再回落则无 featured。判定结果通过 `:featured` prop 传给 `ShowcaseBundleCard`。

### 4.4 卡片信息结构(自上而下)
1. mono tier 标签(描边 chip)
2. 套餐名(19px / weight 850)
3. 一句话描述(`description`,`line-clamp-2`,次文字色)
4. 价格行:mono `price`(cyan-300 大字)+ 划线 `original_price`(`original_price > price` 时)+ `/ validity_days 天`(右对齐弱色)
5. 卖点 `features`(每条前一个 teal 菱形 ◆ check)
6. 覆盖平台 `platforms` chips(mono + 平台色圆点,复用 `src/utils/platformColors.ts` 的平台色映射)
7. CTA「查看详情」(直角渐变描边按钮;featured 卡为实心渐变;文案可按运营偏好改为「立即购买」,跳转不变)

**不展示**:并发、RPM、精确额度、`group_quotas` tooltip(`PublicBundlePlan` 无此数据)。

### 4.5 区块 header(对齐 `ModelCatalogHeader`)
- mono kicker:`// 订阅套餐`
- 巨标(可沿用现有 i18n key 或新增):「选择适合你的套餐」
- 副标题:一行说明
- mono 统计:`{N} 个套餐 · 覆盖 {M} 个平台`(N=plans.length,M=plans 下 platforms 去重计数)
- 右侧「查看全部 →」直角描边按钮 → 跳 `/bundles`(沿用现有行为)

### 4.6 CTA 与点击行为
- 卡内 CTA「查看详情」与整卡点击均跳 `/bundles`(套餐浏览/购买入口,沿用现有路由;未登录由 `/bundles` 路由守卫处理)。**不**直接跳 `/purchase`,降低改动面与未登录态风险。
- 文案「查看详情」与落地页(`/bundles` 列表)语义匹配;若运营更重转化,可改为「立即购买」,跳转不变。
- 保留 header「查看全部套餐」CTA(复用现有 `viewAllBundles` key)。

### 4.7 边界处理
- **暗色模式**:接收 `isDark` prop。当前 `ModelCatalogView` **未**把 `isDark` 传给 `BundleShowcaseSection`(仅传 `:plans`),需新增透传链:`ModelCatalogView` → `BundleShowcaseSection`(`:is-dark`)→ `ShowcaseBundleCard`(`:is-dark`)。scoped CSS 用 `.is-dark` 选择器切换(与模型广场主体一致)。
- **加载/失败**:沿用 `BundleShowcaseSection` 现有 `loadBundlePlans()` 静默降级——无套餐或请求失败则整区不渲染(`v-if="bundlePlans.length"`),不打断模型列表。
- **价格折扣**:`original_price > price` 才显示划线价;否则只显示 `price`。
- **features / platforms 为空**:`features` 空则省略该块;`platforms` 空则省略平台 chips 行。

---

## 5. 技术实现

### 5.1 文件清单
| 文件 | 动作 | 说明 |
|---|---|---|
| `src/components/models/ShowcaseBundleCard.vue` | **新建** | 深色 HUD 套餐卡,scoped CSS + isDark prop |
| `src/components/models/BundleShowcaseSection.vue` | **改写** | 新 header + 网格 + featured 判定,改用 ShowcaseBundleCard,接收并透传 `isDark` |
| `src/views/public/ModelCatalogView.vue` | **小改** | 给 `BundleShowcaseSection` 传 `:is-dark="isDark"`(现仅传 `:plans`) |
| `src/i18n/locales/{zh,en}/custom.ts` | **新增 key** | 在 `modelCatalog` 命名空间新增 CTA 文案 key(如 `bundleViewDetails`);标题/副标题/查看全部复用现有 `bundleSectionTitle`/`bundleSectionSubtitle`/`viewAllBundles` |

### 5.2 `ShowcaseBundleCard` 接口
```ts
// props
plan: PublicBundlePlan   // 直接接收公开 DTO,无需中间映射
featured?: boolean       // 主推位:加强 glow + 实心 CTA + 推荐 badge
isDark?: boolean         // 暗色模式(默认 false)
// emits
(e: 'click', plan: PublicBundlePlan): void
```
展示字段全部直接读 `plan.*`,无 `toCardData` 中间层。

### 5.3 平台色复用
平台 chip 圆点色复用 `src/utils/platformColors.ts` 现有映射(anthropic/openai/gemini/antigravity/grok);若该工具未导出「圆点色」,在 `ShowcaseBundleCard` 内本地定义并加注释指向未来下沉点(与现有 `BundlePlanCard` 的本地 `platformDotClass` 处理一致,不在此 spec 扩大重构范围)。

### 5.4 暗色机制一致性
模型广场主体用 `isDark` prop + scoped CSS(非 Tailwind `dark:`)。新组件**必须沿用** `isDark` + `.is-dark` 选择器,禁止混入 `dark:` 变体,以消除 §3 现存的「两套暗色实现」问题在新代码中重现。

---

## 6. 验收标准

1. 模型广场(`/`)顶部套餐区的卡片与下方模型卡视觉语言一致:深色渐变底、mint 描边、切角、内发光、monospace 数据、teal/cyan 单色。
2. `tier` 不再以 blue/purple/amber 多色表达,改用 mono 文字标签 + 推荐 badge。
3. 主推套餐(`tier==='pro'` 或中间档)显示 `★ 推荐` badge 且 CTA 为实心渐变。
4. 卡片信息仅来自 `PublicBundlePlan` 字段;不含并发/RPM/精确额度。
5. `BundlePlanCard.vue`、`BundlesView.vue`、`BundleUsageView.vue` 零改动,`/bundles` 页外观不变。
6. `isDark` 经 `ModelCatalogView → BundleShowcaseSection → ShowcaseBundleCard` 正确透传;亮/暗双主题下套餐区与模型广场主体表现一致(单套 `isDark` 机制)。
7. 无套餐 / 接口失败时,套餐区不渲染,模型列表正常。
8. `pnpm run typecheck` 通过;新增/改动组件有 Vitest 单测覆盖 featured 判定与空数据降级。
9. 整卡与 CTA 点击跳转 `/bundles` 行为正常。

---

## 7. 风险与权衡

- **tier 多色身份丢失**(方案 C 取舍):tier 不再用颜色区分,仅靠文字标签 + 价格 + 排序。权衡:换来与模型广场最高的视觉统一度;tier 身份信息通过文字 + 价格仍可传达。若后续运营反馈需要强色彩区分,可低成本切回方案 A(仅改 `ShowcaseBundleCard` 内 tier class)。
- **featured 判定依赖 tier/价格**:若套餐 tier 配置不规范(如全是 starter),featured 回落到中间价格档或无 badge。可接受。
- **新增 i18n key**:需同步所有语言文件,否则 fallback 到 key 字符串。
- **未改动 `BundlePlanCard` 的重复平台色定义**:本 spec 不扩大重构范围,仅在新组件内本地定义并注释;下沉到 `platformColors.ts` 留作后续清理。
