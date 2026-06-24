/**
 * 套餐捆绑销售模块 TypeScript 类型定义
 *
 * 定义套餐计划（BundlePlan）、渠道组额度（BundlePlanGroupQuota）、
 * 套餐订阅（BundleSubscription）、用量跟踪和进度等核心类型，
 * 以及创建套餐的请求 DTO。
 */

import type { BundleTier } from '@/constants/bundleTiers'

/** 套餐计划 — 定义可供购买的套餐方案 */
export interface BundlePlan {
  id: number
  name: string
  description: string
  tier: BundleTier
  price: number
  original_price: number
  currency: string
  validity_days: number
  concurrency_limit: number
  rpm_limit: number
  features: string[]
  for_sale: boolean
  sort_order: number
  status: 'active' | 'disabled'
  group_quotas: BundlePlanGroupQuota[]
}

/** 套餐计划渠道组额度 — 套餐中单个渠道组的额度配置 */
export interface BundlePlanGroupQuota {
  id: number
  plan_id: number
  group_id: number
  group_name?: string
  group_platform?: string
  quota_scope: 'platform' | 'model'
  model_pattern: string
  daily_limit_usd: number
  weekly_limit_usd: number
  monthly_limit_usd: number
  daily_limit_count: number
  weekly_limit_count: number
  monthly_limit_count: number
}

/** 套餐订阅 — 用户购买的套餐实例 */
export interface BundleSubscription {
  id: number
  user_id: number
  plan_id: number
  plan?: BundlePlan
  status: 'active' | 'expired' | 'revoked'
  starts_at: string
  expires_at: string
  concurrency_limit: number
  rpm_limit: number
  source: 'purchase' | 'redeem' | 'admin_assign'
  /** 管理端列表 enrich 的用户邮箱 */
  user_email?: string
}

/** 套餐订阅用量 — 按渠道组统计的日/周/月用量 */
export interface BundleSubscriptionUsage {
  id: number
  bundle_subscription_id: number
  group_id: number
  group?: {
    id: number
    name: string
    platform: string
  }
  model_pattern: string
  daily_usage_usd: number
  weekly_usage_usd: number
  monthly_usage_usd: number
  daily_limit_usd: number
  weekly_limit_usd: number
  monthly_limit_usd: number
  daily_usage_count: number
  weekly_usage_count: number
  monthly_usage_count: number
  daily_limit_count: number
  weekly_limit_count: number
  monthly_limit_count: number
}

/** 套餐用量进度 — 单个渠道组的用量与限额对比 */
export interface BundleUsageProgress {
  group_id: number
  group_name: string
  platform: string
  model_pattern: string
  daily_usage_usd: number
  daily_limit_usd: number
  weekly_usage_usd: number
  weekly_limit_usd: number
  monthly_usage_usd: number
  monthly_limit_usd: number
  daily_usage_count: number
  daily_limit_count: number
  weekly_usage_count: number
  weekly_limit_count: number
  monthly_usage_count: number
  monthly_limit_count: number
}

/** 创建套餐计划请求 DTO */
export interface CreateBundlePlanRequest {
  name: string
  description?: string
  tier: BundleTier
  price: number
  original_price?: number
  currency: 'USD' | 'CNY'
  validity_days: number
  concurrency_limit?: number
  rpm_limit?: number
  features?: string[]
  for_sale?: boolean
  group_quotas: CreateGroupQuotaRequest[]
}

/** 创建渠道组额度请求 DTO */
export interface CreateGroupQuotaRequest {
  group_id: number
  quota_scope: 'platform' | 'model'
  model_pattern?: string
  daily_limit_usd?: number
  weekly_limit_usd?: number
  monthly_limit_usd?: number
  daily_limit_count?: number
  weekly_limit_count?: number
  monthly_limit_count?: number
}
