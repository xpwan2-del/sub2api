export interface BundlePlan {
  id: number
  name: string
  description: string
  tier: 'basic' | 'flagship' | 'enterprise'
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

export interface BundlePlanGroupQuota {
  id: number
  plan_id: number
  group_id: number
  group?: {
    id: number
    name: string
    platform: string
  }
  quota_scope: 'platform' | 'model'
  model_pattern: string
  daily_limit_usd: number
  weekly_limit_usd: number
  monthly_limit_usd: number
}

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
  group_usages: BundleSubscriptionUsage[]
}

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
}

export interface BundleUsageProgress {
  group_id: number
  group_name: string
  platform: string
  model_pattern: string
  daily_used: number
  daily_limit: number
  weekly_used: number
  weekly_limit: number
  monthly_used: number
  monthly_limit: number
}

export interface CreateBundlePlanRequest {
  name: string
  description?: string
  tier: 'basic' | 'flagship' | 'enterprise'
  price: number
  original_price?: number
  currency: 'USD' | 'CNY'
  validity_days: number
  concurrency_limit?: number
  rpm_limit?: number
  features?: string[]
  group_quotas: CreateGroupQuotaRequest[]
}

export interface CreateGroupQuotaRequest {
  group_id: number
  quota_scope: 'platform' | 'model'
  model_pattern?: string
  daily_limit_usd?: number
  weekly_limit_usd?: number
  monthly_limit_usd?: number
}
