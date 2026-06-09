/**
 * Admin Bundle API endpoints
 * Handles bundle plan and subscription management for administrators
 */

import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'
import type { BundleTier } from '@/constants/bundleTiers'
import type {
  BundlePlan,
  BundleSubscription,
  CreateBundlePlanRequest,
  CreateGroupQuotaRequest
} from '@/types/bundle'

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

/**
 * Get detail of a specific bundle plan
 * @param id - Plan ID
 * @returns Plan details
 */
export async function getPlanDetail(id: number): Promise<BundlePlan> {
  const { data } = await apiClient.get<BundlePlan>(`/admin/bundle/plans/${id}`)
  return data
}

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

export const bundlesAPI = {
  createPlan,
  updatePlan,
  listPlans,
  getPlanDetail,
  disablePlan,
  listSubscriptions,
  revokeSubscription,
  extendSubscription
}

export default bundlesAPI
