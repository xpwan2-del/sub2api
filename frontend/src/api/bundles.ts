/**
 * User Bundle API
 * API for regular users to view bundle plans and manage their bundle subscriptions
 */

/**
 * 用户端套餐 API
 * 提供套餐计划浏览、当前订阅查询、用量进度查看等用户端接口
 */

import { apiClient } from './client'
import type { BundlePlan, BundleSubscription, BundleUsageProgress } from '@/types/bundle'
import type { CreateOrderResult } from '@/types/payment'

/** 获取所有在售套餐计划 */
/**
 * Get list of available bundle plans
 */
export async function getPlans(): Promise<BundlePlan[]> {
  const { data } = await apiClient.get<BundlePlan[]>('/bundles/plans')
  return data
}

/** 获取单个套餐计划详情 */
/**
 * Get detail of a specific bundle plan
 * @param id - Plan ID
 */
export async function getPlanDetail(id: number): Promise<BundlePlan> {
  const { data } = await apiClient.get<BundlePlan>(`/bundles/plans/${id}`)
  return data
}

/** 获取当前用户的活跃套餐订阅 */
/**
 * Get current user's active bundle subscription
 */
export async function getMyBundle(): Promise<BundleSubscription | null> {
  const { data } = await apiClient.get<BundleSubscription | null>('/bundles/subscription')
  return data
}

/** 获取当前用户的套餐用量进度 */
/**
 * Get current user's bundle usage progress
 */
export async function getMyUsage(): Promise<BundleUsageProgress[]> {
  const { data } = await apiClient.get<BundleUsageProgress[]>('/bundles/subscription/usage')
  return data
}

/** 发起套餐购买（获取支付链接） */
/**
 * Initiate checkout for a bundle plan
 * @param planId - Plan ID to purchase
 * @param paymentType - Payment method type (e.g. 'alipay', 'wxpay', 'stripe')
 * @param returnUrl - Optional return URL after payment
 */
export async function checkout(planId: number, paymentType: string, returnUrl?: string): Promise<CreateOrderResult> {
  const { data } = await apiClient.post<CreateOrderResult>('/bundles/checkout', {
    plan_id: planId,
    payment_type: paymentType,
    return_url: returnUrl,
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
