/**
 * 公开套餐接口（无鉴权）。
 *
 * GET /api/v1/public/bundles/plans —— 返回裁剪后的在售套餐计划。
 * 字段与后端 `PublicBundlePlan` DTO 对齐（snake_case JSON tag）：
 * 敏感/内部字段（group_id、精确额度、concurrency/rpm 限额等）已剥离，
 * 仅保留展示所需信息；`platforms` 由该套餐 group_quotas 的 group_platform 去重聚合。
 *
 * 运营总开关 ops_enabled=false 时后端统一返回空数组 []（前端无需读取开关，
 * 据此自然不渲染套餐区）。空结果序列化为 [] 而非 null。
 */
import { apiClient } from './client'

/** 公网套餐计划 DTO —— 对齐后端 PublicBundlePlan（裁剪版，无 group_quotas / 精确额度）。 */
export interface PublicBundlePlan {
  name: string
  tier: string
  description: string
  price: number
  original_price: number
  currency: string
  validity_days: number
  features: string[]
  sort_order: number
  /** 覆盖平台列表（由后端 group_quotas 去重聚合，如 ['openai','anthropic']） */
  platforms: string[]
}

/**
 * 拉取公开在售套餐计划。
 * @param signal 可选 AbortSignal，用于组件卸载时取消请求。
 */
export const getPublicBundlePlans = (signal?: AbortSignal): Promise<PublicBundlePlan[]> =>
  apiClient.get<PublicBundlePlan[]>('/public/bundles/plans', { signal }).then(r => r.data)
