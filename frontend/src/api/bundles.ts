/**
 * User Bundle API
 * API for regular users to view bundle plans and manage their bundle subscriptions
 */

import { apiClient } from './client'
import type { BundlePlan, BundleSubscription, BundleUsageProgress } from '@/types/bundle'

/**
 * Get list of available bundle plans
 */
export async function getPlans(): Promise<BundlePlan[]> {
  const { data } = await apiClient.get<BundlePlan[]>('/bundles/plans')
  return data
}

/**
 * Get detail of a specific bundle plan
 * @param id - Plan ID
 */
export async function getPlanDetail(id: number): Promise<BundlePlan> {
  const { data } = await apiClient.get<BundlePlan>(`/bundles/plans/${id}`)
  return data
}

/**
 * Get current user's active bundle subscription
 */
export async function getMyBundle(): Promise<BundleSubscription | null> {
  const { data } = await apiClient.get<BundleSubscription | null>('/bundles/subscription')
  return data
}

/**
 * Get current user's bundle usage progress
 */
export async function getMyUsage(): Promise<BundleUsageProgress[]> {
  const { data } = await apiClient.get<BundleUsageProgress[]>('/bundles/subscription/usage')
  return data
}

/**
 * Initiate checkout for a bundle plan
 * @param planId - Plan ID to purchase
 */
export async function checkout(planId: number): Promise<{ checkout_url: string; order_id: string }> {
  const { data } = await apiClient.post<{ checkout_url: string; order_id: string }>('/bundles/checkout', {
    plan_id: planId
  })
  return data
}

export default {
  getPlans,
  getPlanDetail,
  getMyBundle,
  getMyUsage,
  checkout
}
