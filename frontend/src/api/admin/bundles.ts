/**
 * Admin Bundle API endpoints
 * Handles bundle plan and subscription management for administrators
 */

/**
 * 管理后台套餐 API
 * 提供套餐计划的 CRUD、订阅的查询/撤销/延期等管理端接口
 */

import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'
import type { BundleTier } from '@/constants/bundleTiers'
import type {
  BundlePlan,
  BundleSubscription,
  BundleUsageProgress,
  CreateBundlePlanRequest,
  CreateGroupQuotaRequest
} from '@/types/bundle'

/** 创建新的套餐计划 */
/**
 * Create a new bundle plan
 * @param data - Plan creation request
 * @returns Created plan
 */
export async function createPlan(
  data: CreateBundlePlanRequest
): Promise<BundlePlan> {
  const { data: result } = await apiClient.post<BundlePlan>(
    '/admin/bundle/plans',
    data
  )
  return result
}

/** 更新已有套餐计划（支持部分字段更新） */
/**
 * Update an existing bundle plan
 * @param id - Plan ID
 * @param data - Plan update payload (partial fields + group_quotas)
 * @returns Updated plan
 */
export async function updatePlan(
  id: number,
  data: Partial<CreateBundlePlanRequest> & { group_quotas?: CreateGroupQuotaRequest[] }
): Promise<BundlePlan> {
  const { data: result } = await apiClient.put<BundlePlan>(
    `/admin/bundle/plans/${id}`,
    data
  )
  return result
}

/** 分页查询套餐计划列表，支持层级和状态过滤 */
/**
 * List all bundle plans with optional filters
 * @param params - Query parameters
 * @returns Paginated list of plans
 */
export async function listPlans(
  params?: {
    page?: number
    page_size?: number
    status?: 'active' | 'disabled'
    tier?: BundleTier
  }
): Promise<PaginatedResponse<BundlePlan>> {
  const { data } = await apiClient.get<PaginatedResponse<BundlePlan>>(
    '/admin/bundle/plans',
    { params }
  )
  return data
}

/** 获取单个套餐计划详情 */
/**
 * Get detail of a specific bundle plan
 * @param id - Plan ID
 * @returns Plan details
 */
export async function getPlanDetail(id: number): Promise<BundlePlan> {
  const { data } = await apiClient.get<BundlePlan>(`/admin/bundle/plans/${id}`)
  return data
}

/** 停用（软删除）套餐计划 */
/**
 * Disable (soft-delete) a bundle plan
 * @param id - Plan ID
 * @returns Success confirmation
 */
export async function disablePlan(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(
    `/admin/bundle/plans/${id}`
  )
  return data
}

/** 分页查询套餐订阅列表，支持状态和用户ID过滤 */
/**
 * List all bundle subscriptions with optional filters
 * @param params - Query parameters
 * @returns Paginated list of subscriptions
 */
export async function listSubscriptions(
  params?: {
    page?: number
    page_size?: number
    status?: 'active' | 'expired' | 'revoked'
    user_id?: number
    plan_id?: number
  }
): Promise<PaginatedResponse<BundleSubscription>> {
  const { data } = await apiClient.get<PaginatedResponse<BundleSubscription>>(
    '/admin/bundle/subscriptions',
    { params }
  )
  return data
}

/** 撤销套餐订阅 */
/**
 * Revoke a bundle subscription
 * @param id - Subscription ID
 * @returns Updated subscription
 */
export async function revokeSubscription(
  id: number
): Promise<BundleSubscription> {
  const { data } = await apiClient.post<BundleSubscription>(
    `/admin/bundle/subscriptions/${id}/revoke`
  )
  return data
}

/** 延长套餐订阅有效期 */
/**
 * Extend a bundle subscription by given days
 * @param id - Subscription ID
 * @param days - Number of days to extend
 * @returns Updated subscription
 */
export async function extendSubscription(
  id: number,
  days: number
): Promise<BundleSubscription> {
  const { data } = await apiClient.post<BundleSubscription>(
    `/admin/bundle/subscriptions/${id}/extend`,
    { days }
  )
  return data
}

/** 获取单个订阅的渠道组用量详情（展开行按需调用） */
/**
 * Get usage progress for a single bundle subscription
 * @param id - Subscription ID
 * @returns Usage progress per channel group
 */
export async function getSubscriptionUsageProgress(
  id: number
): Promise<BundleUsageProgress[]> {
  const { data } = await apiClient.get<BundleUsageProgress[]>(
    `/admin/bundle/subscriptions/${id}/usage-progress`
  )
  return data
}

export const bundlesAPI = {
  createPlan,
  updatePlan,
  listPlans,
  getPlanDetail,
  disablePlan,
  listSubscriptions,
  revokeSubscription,
  extendSubscription,
  getSubscriptionUsageProgress
}

export default bundlesAPI
