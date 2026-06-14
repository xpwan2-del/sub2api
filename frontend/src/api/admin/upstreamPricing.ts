/**
 * Admin Upstream Price API
 * 上游价格同步子系统管理端 API
 *
 * 对应后端路由：
 *   POST   /admin/upstream-price/sources
 *   GET    /admin/upstream-price/sources
 *   PUT    /admin/upstream-price/sources/:id
 *   DELETE /admin/upstream-price/sources/:id
 *   POST   /admin/upstream-price/sources/:id/test
 *   POST   /admin/upstream-price/sources/:id/sync
 *   GET    /admin/upstream-price/changes
 *   POST   /admin/upstream-price/changes/:id/apply
 *   POST   /admin/upstream-price/changes/:id/dismiss
 *   GET    /admin/upstream-price/compare
 */

import { apiClient } from '../client'
import type {
  UpstreamPriceSource,
  CreateUpstreamPriceSourceRequest,
  UpdateUpstreamPriceSourceRequest,
  UpstreamSourceTestResult,
  UpstreamPriceChange,
  ListUpstreamPriceChangesParams,
  ApplyUpstreamPriceChangeRequest,
  UpstreamPriceCompareRow
} from '@/types/upstreamPricing'

const BASE = '/admin/upstream-price'

// ==================== Sources ====================

/** 列出全部价格源（后端始终返回数组，空时返回 []） */
export async function listSources(): Promise<UpstreamPriceSource[]> {
  const { data } = await apiClient.get<UpstreamPriceSource[]>(`${BASE}/sources`)
  return data ?? []
}

/** 创建价格源 */
export async function createSource(
  payload: CreateUpstreamPriceSourceRequest
): Promise<UpstreamPriceSource> {
  const { data } = await apiClient.post<UpstreamPriceSource>(`${BASE}/sources`, payload)
  return data
}

/** 更新价格源（支持部分字段） */
export async function updateSource(
  id: number,
  payload: UpdateUpstreamPriceSourceRequest
): Promise<UpstreamPriceSource> {
  const { data } = await apiClient.put<UpstreamPriceSource>(
    `${BASE}/sources/${id}`,
    payload
  )
  return data
}

/** 删除价格源 */
export async function deleteSource(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`${BASE}/sources/${id}`)
  return data
}

/** 测试连接，返回 reachable 与 model_count */
export async function testSource(id: number): Promise<UpstreamSourceTestResult> {
  const { data } = await apiClient.post<UpstreamSourceTestResult>(
    `${BASE}/sources/${id}/test`
  )
  return data
}

/** 手动触发某个源的同步 */
export async function syncSource(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>(`${BASE}/sources/${id}/sync`)
  return data
}

// ==================== Changes ====================

/** 列出价格变动（默认 pending，可按 source_id / status 过滤，空时返回 []） */
export async function listChanges(
  params?: ListUpstreamPriceChangesParams
): Promise<UpstreamPriceChange[]> {
  const { data } = await apiClient.get<UpstreamPriceChange[]>(`${BASE}/changes`, {
    params
  })
  return data ?? []
}

/** 应用变动：follow_cost（跟随成本）或 lock_price（锁死售价） */
export async function applyChange(
  id: number,
  payload: ApplyUpstreamPriceChangeRequest
): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>(
    `${BASE}/changes/${id}/apply`,
    payload
  )
  return data
}

/** 忽略变动 */
export async function dismissChange(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>(
    `${BASE}/changes/${id}/dismiss`
  )
  return data
}

// ==================== Compare ====================

/** 价格对比（需传 source_id，未指定时返回 []） */
export async function comparePrices(sourceId?: number): Promise<UpstreamPriceCompareRow[]> {
  const { data } = await apiClient.get<UpstreamPriceCompareRow[]>(`${BASE}/compare`, {
    params: sourceId ? { source_id: sourceId } : undefined
  })
  return data ?? []
}

export const upstreamPricingAPI = {
  listSources,
  createSource,
  updateSource,
  deleteSource,
  testSource,
  syncSource,
  listChanges,
  applyChange,
  dismissChange,
  comparePrices
}

export default upstreamPricingAPI
