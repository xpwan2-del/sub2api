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
  dashboard_auth_mode?: 'auto' | 'bearer' | 'raw' | 'raw_user' | 'bearer_user'
  dashboard_user_id?: number | null
  proxy_id?: number | null
  target_channel_id: number
  enabled: boolean
  base_price_per_1k: number
  pricing_source: 'auto' | 'ratio_config' | 'pricing'
  sync_model_price: boolean
  target_upstream_group?: string
  balance_threshold_usd?: number | null
  last_balance_quota?: number | null
  last_used_quota?: number | null
  last_balance_usd?: number | null
  last_balance_at?: string | null
  last_balance_checked_at?: string | null
  last_balance_error?: string | null
  balance_status?: 'not_configured' | 'unknown' | 'error' | 'low' | 'healthy'
  last_sync_at?: string | null
  last_error?: string | null
}

export interface PriceChangeItem {
  id: number
  request_id: number
  kind: 'model_price' | 'model_added' | 'model_removed' | 'model_unchanged'
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

export async function refreshBalance(id: number) {
  const { data } = await apiClient.post<UpstreamSourceConfig>(
    `${base}/upstream-sources/${id}/balance/refresh`,
  )
  return data
}

export interface BatchBalanceRefreshResult {
  total: number
  success: number
  failed: number
  skipped: number
  errors: Array<{ source_id: number; source_name: string; error: string }>
}

export async function refreshAllBalances() {
  const { data } = await apiClient.post<BatchBalanceRefreshResult>(
    `${base}/upstream-sources/balance/refresh`,
    undefined,
    { timeout: 120000 },
  )
  return data
}

export async function syncNow(id: number) {
  const { data } = await apiClient.post<{ request_id: number }>(`${base}/upstream-sources/${id}/sync`)
  return data
}

/** 拉取上游可用分组字典({key: 展示名}),供 source 配置下拉。 */
export async function listUpstreamGroups(id: number) {
  const { data } = await apiClient.get<{ groups: Record<string, string> }>(
    `${base}/upstream-sources/${id}/groups`,
  )
  return data
}

/** 按 base_url 预览可用分组(新建 source 尚未保存时用)。 */
export async function previewUpstreamGroups(baseURL: string, proxyId?: number | null, dashboardToken?: string, apiKey?: string, authMode?: string, userID?: number | null) {
  const { data } = await apiClient.post<{ groups: Record<string, string> }>(
    `${base}/upstream-sources/groups/preview`,
    { base_url: baseURL, proxy_id: proxyId ?? null, dashboard_token: dashboardToken ?? '', api_key: apiKey ?? '', dashboard_auth_mode: authMode ?? 'auto', dashboard_user_id: userID ?? null },
  )
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
  refreshBalance,
  refreshAllBalances,
  syncNow,
  listUpstreamGroups,
  previewUpstreamGroups,
  listRequests,
  getRequest,
  reviewItem,
  closeRequest,
}

export default upstreamPriceSyncAPI
