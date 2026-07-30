/**
 * 模型广场套餐展示区的纯展示逻辑。
 *
 * pickFeaturedPlan:选出「主推」套餐（贴 ★推荐 badge + 加强 glow）。
 *   规则：套餐数 < 2 → null；否则优先 tier==='pro'；否则取价格升序中间档。
 * platformDotColor:平台 → 圆点 CSS 颜色（深色 HUD scoped CSS 用；
 *   platformColors.ts 只提供 Tailwind 类，此处用不上）。
 */
import type { PublicBundlePlan } from '@/api/publicBundles'

/** 选出主推套餐。套餐数 < 2 时返回 null。 */
export function pickFeaturedPlan(plans: PublicBundlePlan[]): PublicBundlePlan | null {
  if (!plans || plans.length < 2) return null
  const pro = plans.find(p => p.tier === 'pro')
  if (pro) return pro
  const sorted = [...plans].sort((a, b) => a.price - b.price)
  return sorted[Math.floor((sorted.length - 1) / 2)]
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
