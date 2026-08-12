/**
 * Upstream new-api Price Sync API.
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
  sync_group_ratio: boolean
  excluded_group_ids: number[]
  target_upstream_group?: string
  group_ratio_baseline_key?: string | null
  group_ratio_baseline_value?: number | null
  group_ratio_baseline_observed_at?: string | null
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

export interface ConvertedPrice {
  BillingMode?: string
  billing_mode?: string
  InputPrice?: number | null
  input_price?: number | null
  OutputPrice?: number | null
  output_price?: number | null
  CacheReadPrice?: number | null
  cache_read_price?: number | null
  CacheWritePrice?: number | null
  cache_write_price?: number | null
  PerRequestPrice?: number | null
  per_request_price?: number | null
}

interface PriceChangeItemBase {
  id: number
  request_id: number
  platform: string
  target_channel_id: number
  status: string
  note?: string | null
}

export interface ModelPriceChangeItem extends PriceChangeItemBase {
  kind: 'model_price' | 'model_added' | 'model_removed' | 'model_unchanged'
  model_name: string
  upstream_converted: ConvertedPrice | null
  local_current: ConvertedPrice | null
  apply_value?: ConvertedPrice | null
}

export interface GroupRateChange {
  strategy: string
  upstream_group_key: string
  upstream_group_name: string
  upstream_old_ratio: number
  upstream_new_ratio: number
  local_group_id: number
  local_group_name: string
  local_group_sort_order: number
  local_current_rate: number
  suggested_rate: number
}

export interface GroupRatioChangeItem extends PriceChangeItemBase {
  kind: 'group_ratio'
  target_group_id: number
  group_rate_change: GroupRateChange
  apply_rate?: number | null
}

export type PriceChangeItem = ModelPriceChangeItem | GroupRatioChangeItem

export interface PriceChangeRequest {
  id: number
  source_config_id: number
  status: string
  summary: Record<string, number>
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
  const { data } = await apiClient.post<UpstreamSourceConfig>(`${base}/upstream-sources/${id}/balance/refresh`)
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

export async function listUpstreamGroups(id: number) {
  const { data } = await apiClient.get<{ groups: Record<string, string> }>(`${base}/upstream-sources/${id}/groups`)
  return data
}

export async function previewUpstreamGroups(baseURL: string, proxyId?: number | null, dashboardToken?: string, apiKey?: string, authMode?: string, userID?: number | null) {
  const { data } = await apiClient.post<{ groups: Record<string, string> }>(
    `${base}/upstream-sources/groups/preview`,
    { base_url: baseURL, proxy_id: proxyId ?? null, dashboard_token: dashboardToken ?? '', api_key: apiKey ?? '', dashboard_auth_mode: authMode ?? 'auto', dashboard_user_id: userID ?? null },
  )
  return data
}

export interface PaginatedPriceChangeRequests {
  items: PriceChangeRequest[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface PriceChangeRequestListParams {
  status?: string
  page?: number
  page_size?: number
}

export async function listRequests(params: PriceChangeRequestListParams = {}) {
  const { data } = await apiClient.get<PaginatedPriceChangeRequests>(`${base}/price-change-requests`, { params })
  return data
}

export type PriceChangeRequestDetail = PriceChangeRequest & { items: PriceChangeItem[] }

export async function getRequest(id: number) {
  const { data } = await apiClient.get<PriceChangeRequestDetail>(`${base}/price-change-requests/${id}`)
  return data
}

export type ReviewItemBody =
  | { action: 'apply'; apply_value: ConvertedPrice; note?: string }
  | { action: 'apply'; apply_rate: number; note?: string }
  | { action: 'reject' | 'ignore'; note?: string }

export async function reviewItem(reqId: number, itemId: number, body: ReviewItemBody) {
  const { data } = await apiClient.post(`${base}/price-change-requests/${reqId}/items/${itemId}/review`, body)
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
