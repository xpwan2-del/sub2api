/**
 * 模型广场套餐展示区的纯展示逻辑。
 *
 * pickFeaturedPlan:选出「主推」套餐（贴 ★推荐 badge + 加强 glow）。
 *   规则：返回运营在管理后台勾选 featured 的套餐；多个取第一个（按 sort_order）；无则 null。
 * platformDotColor:平台 → 圆点 CSS 颜色（深色 HUD scoped CSS 用；
 *   platformColors.ts 只提供 Tailwind 类，此处用不上）。
 */
import type { PublicBundlePlan } from '@/api/publicBundles'

/** 选出主推套餐。套餐数 < 2 时返回 null。 */
export function pickFeaturedPlan(plans: PublicBundlePlan[]): PublicBundlePlan | null {
  if (!plans || plans.length === 0) return null
  // 运营在管理后台勾选 featured 的套餐即主推；多个 featured 取第一个（plans 已按 sort_order 排序）
  return plans.find(p => p.featured) ?? null
}

/** 平台 → 圆点 CSS 颜色。未知平台回落 slate-500。 */
export function platformDotColor(platform: string): string {
  switch (platform) {
    case 'anthropic': return '#f97316'   // orange-500
    case 'openai': return '#10b981'      // emerald-500
    case 'antigravity': return '#a855f7' // purple-500
    case 'gemini': return '#3b82f6'      // blue-500
    case 'grok': return '#71717a'        // zinc-500
    default: return '#64748b'            // slate-500
  }
}
