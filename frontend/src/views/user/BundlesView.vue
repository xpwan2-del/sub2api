<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-20">
        <div class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></div>
      </div>

      <template v-else>
        <!-- Active Bundle Card -->
        <div v-if="activeBundle" class="overflow-hidden rounded-2xl border border-primary-500/20 bg-gradient-to-r from-primary-50 to-white dark:from-primary-900/20 dark:to-dark-800">
          <div class="flex items-center gap-3 border-b border-primary-100 p-4 dark:border-dark-700">
            <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-primary-100 dark:bg-primary-900/40">
              <Icon name="cube" size="lg" class="text-primary-600 dark:text-primary-400" />
            </div>
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <h2 class="truncate text-lg font-bold text-gray-900 dark:text-white">
                  {{ activeBundle.plan?.name || t('bundles.currentBundle') }}
                </h2>
                <span :class="tierBadgeClass(activeBundle.plan?.tier)">
                  {{ tierLabel(activeBundle.plan?.tier) }}
                </span>
                <span class="rounded-full bg-emerald-100 px-2 py-0.5 text-[11px] font-medium text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300">
                  {{ t('bundles.active') }}
                </span>
              </div>
              <p v-if="activeBundle.plan?.description" class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                {{ activeBundle.plan.description }}
              </p>
            </div>
          </div>

          <div class="grid gap-4 p-4 sm:grid-cols-3">
            <!-- Expiration -->
            <div class="rounded-xl bg-white/60 p-3 dark:bg-dark-700/40">
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.expiresAt') }}</p>
              <p :class="expirationClass">
                {{ formatExpiration(activeBundle.expires_at) }}
              </p>
              <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">
                {{ remainingDaysText(activeBundle.expires_at) }}
              </p>
            </div>
            <!-- Concurrency -->
            <div class="rounded-xl bg-white/60 p-3 dark:bg-dark-700/40">
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.concurrency') }}</p>
              <p class="text-lg font-bold text-gray-900 dark:text-white">{{ activeBundle.concurrency_limit || '-' }}</p>
            </div>
            <!-- RPM -->
            <div class="rounded-xl bg-white/60 p-3 dark:bg-dark-700/40">
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.rpm') }}</p>
              <p class="text-lg font-bold text-gray-900 dark:text-white">{{ activeBundle.rpm_limit || '-' }}</p>
            </div>
          </div>

          <!-- Included Groups -->
          <div v-if="activeBundle.plan?.group_quotas?.length" class="border-t border-primary-100 p-4 dark:border-dark-700">
            <p class="mb-2 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('bundles.includedGroups') }}</p>
            <div class="flex flex-wrap gap-2">
              <div
                v-for="gq in activeBundle.plan.group_quotas"
                :key="gq.id"
                class="flex items-center gap-1.5 rounded-lg border border-gray-100 bg-white px-2.5 py-1.5 dark:border-dark-700 dark:bg-dark-800"
              >
                <div :class="['h-1.5 w-1.5 rounded-full', platformDotClass(gq.group?.platform || '')]" />
                <span class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ gq.group?.name || `Group #${gq.group_id}` }}</span>
                <span :class="['rounded px-1.5 py-0.5 text-[10px] font-medium', platformBadgeLightClass(gq.group?.platform || '')]">
                  {{ platformLabel(gq.group?.platform || '') }}
                </span>
              </div>
            </div>
          </div>

          <div class="border-t border-primary-100 p-4 dark:border-dark-700">
            <button
              class="inline-flex items-center gap-1.5 rounded-xl bg-primary-500 px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-primary-600 active:bg-primary-700"
              @click="router.push('/bundles/usage')"
            >
              <Icon name="chart" size="sm" />
              {{ t('bundles.viewUsage') }}
            </button>
          </div>
        </div>

        <!-- Empty State (no active bundle) -->
        <div v-if="!activeBundle" class="card p-12 text-center">
          <div class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700">
            <Icon name="cube" size="xl" class="text-gray-400" />
          </div>
          <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">{{ t('bundles.noActiveBundle') }}</h3>
          <p class="text-gray-500 dark:text-gray-400">{{ t('bundles.noActiveBundleDesc') }}</p>
        </div>

        <!-- Available Plans -->
        <div>
          <h2 class="mb-4 text-lg font-bold text-gray-900 dark:text-white">{{ t('bundles.availablePlans') }}</h2>

          <div v-if="plans.length === 0" class="card py-16 text-center">
            <Icon name="gift" size="xl" class="mx-auto mb-3 text-gray-300 dark:text-dark-600" />
            <p class="text-gray-500 dark:text-gray-400">{{ t('bundles.noPlans') }}</p>
          </div>

          <div v-else :class="planGridClass">
            <div
              v-for="plan in sortedPlans"
              :key="plan.id"
              class="group relative flex flex-col overflow-hidden rounded-2xl border transition-all hover:shadow-xl hover:-translate-y-0.5 bg-white dark:bg-dark-800"
              :class="tierBorderClass(plan.tier)"
            >
              <!-- Tier accent bar -->
              <div :class="['h-1.5', tierAccentClass(plan.tier)]" />

              <div class="flex flex-1 flex-col p-4">
                <!-- Header -->
                <div class="mb-3 flex items-start justify-between gap-2">
                  <div class="min-w-0 flex-1">
                    <div class="flex items-center gap-2">
                      <h3 class="truncate text-base font-bold text-gray-900 dark:text-white">{{ plan.name }}</h3>
                      <span :class="tierBadgeClass(plan.tier)">{{ tierLabel(plan.tier) }}</span>
                    </div>
                    <p v-if="plan.description" class="mt-0.5 line-clamp-2 text-xs leading-relaxed text-gray-500 dark:text-dark-400">
                      {{ plan.description }}
                    </p>
                  </div>
                  <div class="shrink-0 text-right">
                    <div class="flex items-baseline gap-1">
                      <span class="text-xs text-gray-400 dark:text-dark-500">$</span>
                      <span :class="['text-2xl font-extrabold tracking-tight', tierTextClass(plan.tier)]">{{ plan.price }}</span>
                    </div>
                    <span class="text-[11px] text-gray-400 dark:text-dark-500">/ {{ plan.validity_days }}{{ t('bundles.days') }}</span>
                    <div v-if="plan.original_price && plan.original_price > plan.price" class="mt-0.5 flex items-center justify-end gap-1.5">
                      <span class="text-xs text-gray-400 line-through dark:text-dark-500">${{ plan.original_price }}</span>
                      <span :class="['rounded px-1 py-0.5 text-[10px] font-semibold', tierDiscountClass(plan.tier)]">
                        -{{ Math.round((1 - plan.price / plan.original_price) * 100) }}%
                      </span>
                    </div>
                  </div>
                </div>

                <!-- Group Quotas -->
                <div v-if="plan.group_quotas?.length" class="mb-3 rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-700/50">
                  <p class="mb-1 text-[11px] font-medium text-gray-400 dark:text-dark-500">{{ t('bundles.includedGroups') }}</p>
                  <div class="flex flex-wrap gap-1.5">
                    <div
                      v-for="gq in plan.group_quotas"
                      :key="gq.id"
                      class="flex items-center gap-1 rounded bg-gray-200/80 px-1.5 py-0.5 text-[10px] font-medium text-gray-600 dark:bg-dark-600 dark:text-gray-300"
                    >
                      <span :class="['h-1 w-1 rounded-full', platformDotClass(gq.group?.platform || '')]" />
                      {{ gq.group?.name || `Group #${gq.group_id}` }}
                    </div>
                  </div>
                  <!-- Quota details (first group as example) -->
                  <div v-if="plan.group_quotas[0]" class="mt-1.5 grid grid-cols-3 gap-x-3 text-[10px]">
                    <div v-if="plan.group_quotas[0].daily_limit_usd" class="flex justify-between">
                      <span class="text-gray-400 dark:text-dark-500">{{ t('bundles.daily') }}</span>
                      <span class="font-medium text-gray-600 dark:text-gray-300">${{ plan.group_quotas[0].daily_limit_usd }}</span>
                    </div>
                    <div v-if="plan.group_quotas[0].weekly_limit_usd" class="flex justify-between">
                      <span class="text-gray-400 dark:text-dark-500">{{ t('bundles.weekly') }}</span>
                      <span class="font-medium text-gray-600 dark:text-gray-300">${{ plan.group_quotas[0].weekly_limit_usd }}</span>
                    </div>
                    <div v-if="plan.group_quotas[0].monthly_limit_usd" class="flex justify-between">
                      <span class="text-gray-400 dark:text-dark-500">{{ t('bundles.monthly') }}</span>
                      <span class="font-medium text-gray-600 dark:text-gray-300">${{ plan.group_quotas[0].monthly_limit_usd }}</span>
                    </div>
                  </div>
                </div>

                <!-- Features -->
                <div v-if="plan.features?.length" class="mb-3 space-y-1">
                  <div v-for="feature in plan.features" :key="feature" class="flex items-start gap-1.5">
                    <svg :class="['mt-0.5 h-3.5 w-3.5 flex-shrink-0', tierIconClass(plan.tier)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                    </svg>
                    <span class="text-xs text-gray-600 dark:text-gray-300">{{ feature }}</span>
                  </div>
                </div>

                <!-- Concurrency / RPM -->
                <div class="mb-3 flex gap-3 text-xs">
                  <div v-if="plan.concurrency_limit" class="flex items-center gap-1 text-gray-500 dark:text-gray-400">
                    <Icon name="bolt" size="xs" />
                    <span>{{ plan.concurrency_limit }} {{ t('bundles.concurrencyShort') }}</span>
                  </div>
                  <div v-if="plan.rpm_limit" class="flex items-center gap-1 text-gray-500 dark:text-gray-400">
                    <Icon name="clock" size="xs" />
                    <span>{{ plan.rpm_limit }} RPM</span>
                  </div>
                </div>

                <div class="flex-1" />

                <!-- Purchase Button -->
                <button
                  :class="['w-full rounded-xl py-2.5 text-sm font-semibold transition-all active:scale-[0.98]', tierBtnClass(plan.tier)]"
                  @click="handlePurchase(plan)"
                >
                  {{ t('bundles.purchaseNow') }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { getPlans, getMyBundle } from '@/api/bundles'
import type { BundlePlan, BundleSubscription } from '@/types/bundle'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { platformBadgeLightClass, platformLabel } from '@/utils/platformColors'
import { getTierTheme, getTierI18nKey } from '@/constants/bundleTiers'
import { formatDateOnly } from '@/utils/format'

// ==================== BundlesView：用户套餐浏览页 ====================
// 展示用户当前活跃套餐 + 可购买的套餐计划列表
// 支持按层级主题色区分（starter=蓝/pro=紫/enterprise=金）

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()

// 页面加载状态
const loading = ref(true)
// 在售套餐计划列表
const plans = ref<BundlePlan[]>([])
// 当前用户的活跃套餐订阅
const activeBundle = ref<BundleSubscription | null>(null)

// 按 sort_order 排序后的计划列表
const sortedPlans = computed(() =>
  [...plans.value].sort((a, b) => a.sort_order - b.sort_order)
)

// 根据计划数量动态调整网格列数
const planGridClass = computed(() => {
  const n = plans.value.length
  if (n <= 2) return 'grid grid-cols-1 gap-5 sm:grid-cols-2'
  return 'grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3'
})

// 层级主题工具函数（徽章、边框、强调色等）
function tierLabel(tier?: string): string {
  return t(getTierI18nKey(tier, 'user'))
}

function tierBadgeClass(tier?: string): string {
  return getTierTheme(tier).badgeClass
}

function tierBorderClass(tier?: string): string {
  return getTierTheme(tier).borderClass
}

function tierAccentClass(tier?: string): string {
  return getTierTheme(tier).accentClass
}

function tierTextClass(tier?: string): string {
  return getTierTheme(tier).textClass
}

function tierIconClass(tier?: string): string {
  return getTierTheme(tier).iconClass
}

function tierBtnClass(tier?: string): string {
  return getTierTheme(tier).btnClass
}

function tierDiscountClass(tier?: string): string {
  return getTierTheme(tier).discountClass
}

// 平台标识点颜色（openai=绿/anthropic=橙/gemini=蓝）
function platformDotClass(p: string): string {
  switch (p) {
    case 'anthropic': return 'bg-orange-500'
    case 'openai': return 'bg-emerald-500'
    case 'antigravity': return 'bg-purple-500'
    case 'gemini': return 'bg-blue-500'
    default: return 'bg-gray-400'
  }
}

// 格式化到期日期
function formatExpiration(expiresAt: string): string {
  return formatDateOnly(expiresAt)
}

// 计算剩余天数文本
function remainingDaysText(expiresAt: string): string {
  const diff = new Date(expiresAt).getTime() - Date.now()
  const days = Math.max(0, Math.ceil(diff / (1000 * 60 * 60 * 24)))
  return t('bundles.daysRemaining', { days })
}

// 到期日期颜色（<=3天红色/<=7天橙色）
const expirationClass = computed(() => {
  if (!activeBundle.value?.expires_at) return 'text-lg font-bold text-gray-900 dark:text-white'
  const diff = new Date(activeBundle.value.expires_at).getTime() - Date.now()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))
  if (days <= 0) return 'text-lg font-bold text-red-600 dark:text-red-400'
  if (days <= 3) return 'text-lg font-bold text-red-600 dark:text-red-400'
  if (days <= 7) return 'text-lg font-bold text-orange-600 dark:text-orange-400'
  return 'text-lg font-bold text-gray-900 dark:text-white'
})

// 处理购买点击（暂未实现）
function handlePurchase(_plan: BundlePlan) {
  appStore.showInfo(t('bundles.purchaseNotAvailable'))
}

// 并行加载套餐计划和当前订阅数据
async function loadData() {
  try {
    loading.value = true
    const [plansData, bundleData] = await Promise.allSettled([
      getPlans(),
      getMyBundle()
    ])
    if (plansData.status === 'fulfilled') {
      plans.value = plansData.value.filter(p => p.for_sale && p.status === 'active')
    }
    if (bundleData.status === 'fulfilled') {
      // 后端无订阅时返回空数组 []，JS 中 [] 为 truthy，需转为 null
      const bundle = bundleData.value
      activeBundle.value = (Array.isArray(bundle) ? bundle[0] ?? null : bundle) ?? null
    }
  } catch (error) {
    console.error('Failed to load bundle data:', error)
    appStore.showError(t('bundles.failedToLoad'))
  } finally {
    loading.value = false
  }
}

// 页面挂载时加载数据
onMounted(() => {
  loadData()
})
</script>
