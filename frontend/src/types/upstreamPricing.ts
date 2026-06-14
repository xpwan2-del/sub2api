/**
 * 上游价格同步子系统类型定义
 * 对应后端 admin/upstream_price_handler.go 与 admin_notification_handler.go 的 DTO
 */

// ==================== Source（价格源配置） ====================

export type UpstreamPriceParserType = 'openai' | 'anthropic' | 'gemini' | 'custom'

/** 价格源返回结构（sourceResponse） */
export interface UpstreamPriceSource {
  id: number
  name: string
  platform: string
  base_url: string
  pricing_endpoint: string
  parser_type: UpstreamPriceParserType | string
  parser_config: Record<string, unknown>
  model_alias_map: Record<string, string>
  sync_interval_minutes: number
  enabled: boolean
  last_sync_at?: string
  last_sync_status?: string
  last_sync_error?: string
  created_at: string
  updated_at: string
}

/** 创建价格源请求（CreateSourceRequest） */
export interface CreateUpstreamPriceSourceRequest {
  name: string
  platform?: string
  base_url: string
  pricing_endpoint: string
  api_key?: string
  parser_type: string
  parser_config?: Record<string, unknown>
  model_alias_map?: Record<string, string>
  sync_interval_minutes?: number
  enabled?: boolean
}

/** 更新价格源请求（UpdateSourceRequest，全部可选） */
export interface UpdateUpstreamPriceSourceRequest {
  name?: string
  platform?: string
  base_url?: string
  pricing_endpoint?: string
  api_key?: string
  parser_type?: string
  parser_config?: Record<string, unknown>
  model_alias_map?: Record<string, string>
  sync_interval_minutes?: number
  enabled?: boolean
}

/** TestConnection 返回 */
export interface UpstreamSourceTestResult {
  reachable: boolean
  model_count: number
  error?: string
}

// ==================== Change（价格变动） ====================

export type UpstreamPriceChangeStatus = 'pending' | 'applied' | 'dismissed' | 'failed'

export type UpstreamPriceApplyMode = 'follow_cost' | 'lock_price'

/** 变动行（changeResponse） */
export interface UpstreamPriceChange {
  id: number
  source_id: number
  model_name: string
  local_model_name: string
  change_type: string
  prev_input_price: number | null
  prev_output_price: number | null
  curr_input_price: number
  curr_output_price: number
  input_delta_pct: number
  output_delta_pct: number
  suggested_input_price: number
  suggested_multiplier: number | null
  status: UpstreamPriceChangeStatus | string
  detected_at: string
}

export interface ListUpstreamPriceChangesParams {
  source_id?: number
  status?: string
}

export interface ApplyUpstreamPriceChangeRequest {
  mode: UpstreamPriceApplyMode
  target_id?: number
}

// ==================== Compare（对比） ====================

/** 单个模型的 上游 vs 本地 价格对比行（service.PriceCompareRow） */
export interface UpstreamPriceCompareRow {
  model_name: string
  local_model_name?: string
  upstream_input_price: number
  upstream_output_price: number
  local_input_price: number
  local_output_price: number
  local_multiplier: number
  suggested_price: number
  diff_pct: number
}
