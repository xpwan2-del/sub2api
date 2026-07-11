import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import admin from './admin'
import misc from './misc'
import custom from './custom'

// 深合并：将 custom（二次开发独有字段）合入 main 拆分结构。
// custom 仅含 main 缺失的 key，已存在的 key 保留 main 值，避免覆盖上游迭代。
function mergeLocaleMessages(base: Record<string, any>, override: Record<string, any>): Record<string, any> {
  const out: Record<string, any> = { ...base }
  for (const key of Object.keys(override)) {
    const bv = out[key]
    const ov = override[key]
    if (!(key in out)) {
      out[key] = ov
    } else if (bv && typeof bv === 'object' && !Array.isArray(bv) && ov && typeof ov === 'object' && !Array.isArray(ov)) {
      out[key] = mergeLocaleMessages(bv, ov)
    }
  }
  return out
}

export default mergeLocaleMessages({
  ...landing,
  ...common,
  ...dashboard,
  admin,
  ...misc,
}, custom)
