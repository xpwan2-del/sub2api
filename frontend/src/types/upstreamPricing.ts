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

// ==================== Apply Targets（下拉目标） ====================

/** apply 弹窗的渠道下拉项（follow_cost 模式） */
export interface ApplyTargetChannel {
  id: number
  name: string
}

/** apply 弹窗的分组下拉项（lock_price 模式） */
export interface ApplyTargetGroup {
  id: number
  name: string
  rate_multiplier: number
  model_count: number
}

/** apply 弹窗中某个 group 的告警（如该分组下无对应模型，无法应用） */
export interface ApplyTargetWarning {
  group_id: number
  group_name: string
  message: string
}

/** apply 弹窗下拉数据：GET /changes/:id/targets */
export interface ApplyTargetsResponse {
  channels: ApplyTargetChannel[]
  groups: ApplyTargetGroup[]
  warnings?: ApplyTargetWarning[]
}

/** 批量操作结果：批量 follow_cost / 批量撤销 等接口的统一返回 */
export interface BatchOperationResult {
  total: number
  success: number
  failed: number
  errors?: Array<{ change_id: number; error: string }>
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
