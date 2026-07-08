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

/** 获取当前用户的活跃套餐订阅（后端返回数组） */
/**
 * Get current user's active bundle subscriptions
 * Backend returns an array (may be empty when no active subscription)
 */
export async function getMyBundle(): Promise<BundleSubscription[]> {
  const { data } = await apiClient.get<BundleSubscription[]>('/bundles/subscription')
  return data ?? []
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
export async function checkout(planId: number, paymentType: string, returnUrl?: string, useBalance?: boolean): Promise<CreateOrderResult> {
  const { data } = await apiClient.post<CreateOrderResult>('/bundles/checkout', {
    plan_id: planId,
    payment_type: paymentType,
    return_url: returnUrl,
    use_balance: useBalance ?? false,
  })
  return data
}

/** 套餐升级试算结果 —— 旧套餐剩余价值抵扣后需补的差价等信息 */
export interface UpgradePreview {
  /** 旧套餐剩余价值（抵扣金额） */
  credit: number
  /** 目标套餐价格 */
  target_price: number
  /** 升级需补差价 */
  due_amount: number
  /** 升级后新有效期（天） */
  validity_days: number
  /** 是否可升级（目标套餐价值更高时为 true） */
  upgradeable: boolean
  /** 旧套餐名称 */
  old_plan_name: string
  /** 新套餐名称 */
  new_plan_name: string
}

/**
 * 预览套餐升级（试算差价与抵扣）
 * @param sourceBundleSubscriptionId - 当前生效的套餐订阅 ID
 * @param targetPlanId - 目标套餐计划 ID
 */
export async function previewBundleUpgrade(
  sourceBundleSubscriptionId: number,
  targetPlanId: number,
): Promise<UpgradePreview> {
  const { data } = await apiClient.post<UpgradePreview>('/bundles/upgrade/preview', {
    source_bundle_subscription_id: sourceBundleSubscriptionId,
    target_plan_id: targetPlanId,
  })
  return data
}

/**
 * 发起套餐升级订单（返回与 checkout 同结构的支付订单结果）
 */
export async function createBundleUpgradeOrder(payload: {
  source_bundle_subscription_id: number
  target_plan_id: number
  payment_type: string
  use_balance: boolean
  return_url?: string
}): Promise<CreateOrderResult> {
  const { data } = await apiClient.post<CreateOrderResult>('/bundles/upgrade', payload)
  return data
}

export default {
  getPlans,
  getPlanDetail,
  getMyBundle,
  getMyUsage,
  checkout,
  previewBundleUpgrade,
  createBundleUpgradeOrder,
}
