/**
 * Upstream new-api Price Sync API.
 *
 * 同步上游 new-api 的模型与价格变动:
 * - UpstreamSourceConfig: 上游源配置(目标 channel、价格来源、开关等)
 * - PriceChangeRequest / PriceChangeItem: 一次同步产生的变更请求与逐项审查条目
 */

import { apiClient } from '../client'

export interface UpstreamSourceConfig {
  id?: number
  name: string
  base_url: string
  api_key: string
  dashboard_token?: string
  target_channel_id: number
  enabled: boolean
  base_price_per_1k: number
  pricing_source: 'auto' | 'ratio_config' | 'pricing'
  sync_model_price: boolean
  last_sync_at?: string | null
  last_error?: string | null
}

export interface PriceChangeItem {
  id: number
  request_id: number
  kind: 'model_price' | 'model_added' | 'model_removed'
  platform: string
  model_name: string
  target_channel_id: number
  upstream_converted: any
  local_current: any
  apply_value: any
  status: string
}

export interface PriceChangeRequest {
  id: number
  source_config_id: number
  status: string
  summary: any
  created_at: string
}

const base = '/admin/channels'

export async function listSources() {
  const { data } = await apiClient.get<UpstreamSourceConfig[]>(`${base}/upstream-sources`)
  return data
}

export async function createSource(p: UpstreamSourceConfig) {
  const { data } = await apiClient.post<UpstreamSourceConfig>(`${base}/upstream-sources`, p)
  return data
}

export async function updateSource(id: number, p: UpstreamSourceConfig) {
  const { data } = await apiClient.put<UpstreamSourceConfig>(`${base}/upstream-sources/${id}`, p)
  return data
}

export async function deleteSource(id: number) {
  await apiClient.delete(`${base}/upstream-sources/${id}`)
}

export async function syncNow(id: number) {
  const { data } = await apiClient.post<{ request_id: number }>(`${base}/upstream-sources/${id}/sync`)
  return data
}

/** 后端 `response.Paginated` 信封(apiClient 拦截器已剥离外层 `data`)。 */
export interface PaginatedPriceChangeRequests {
  items: PriceChangeRequest[]
  total: number
  page: number
  page_size: number
  pages: number
}

export async function listRequests(params: { status?: string }) {
  const { data } = await apiClient.get<PaginatedPriceChangeRequests>(
    `${base}/price-change-requests`,
    { params },
  )
  return data
}

export async function getRequest(id: number) {
  const { data } = await apiClient.get<{ request: PriceChangeRequest; items: PriceChangeItem[] }>(
    `${base}/price-change-requests/${id}`,
  )
  return data
}

export async function reviewItem(
  reqId: number,
  itemId: number,
  body: { action: string; apply_value?: any; note?: string },
) {
  const { data } = await apiClient.post(
    `${base}/price-change-requests/${reqId}/items/${itemId}/review`,
    body,
  )
  return data
}

export async function closeRequest(id: number) {
  await apiClient.post(`${base}/price-change-requests/${id}/close`)
}

export const upstreamPriceSyncAPI = {
  listSources,
  createSource,
  updateSource,
  deleteSource,
  syncNow,
  listRequests,
  getRequest,
  reviewItem,
  closeRequest,
}

export default upstreamPriceSyncAPI
