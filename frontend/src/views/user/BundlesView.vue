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
                  {{ activePlan?.name || t('bundles.currentBundle') }}
                </h2>
                <span :class="tierBadgeClass(activePlan?.tier)">
                  {{ tierLabel(activePlan?.tier) }}
                </span>
                <span class="rounded-full bg-emerald-100 px-2 py-0.5 text-[11px] font-medium text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300">
                  {{ t('bundles.active') }}
                </span>
              </div>
              <p v-if="activePlan?.description"
                class="mt-0.5 line-clamp-2 text-xs text-gray-500 dark:text-gray-400"
                :title="activePlan.description">
                {{ activePlan.description }}
              </p>
            </div>
          </div>

          <div class="grid gap-4 p-4 sm:grid-cols-3">
            <!-- Expiration -->
            <div class="rounded-xl bg-white/60 p-3 dark:bg-dark-700/40">
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.expiresAt') }}</p>
              <p :class="expirationClass">
                {{ formatExpiration(activeBundle.expires_at) }}
                <span class="ml-1 align-middle text-sm font-normal text-gray-400 dark:text-gray-500">{{ formatExpirationTime(activeBundle.expires_at) }}</span>
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
          <div v-if="activePlan?.group_quotas?.length" class="border-t border-primary-100 p-4 dark:border-dark-700">
            <p class="mb-2 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('bundles.includedGroups') }}</p>
            <div class="flex flex-wrap gap-2">
              <div
                v-for="gq in activePlan.group_quotas"
                :key="gq.id"
                class="group/Chip relative flex items-center gap-1.5 rounded-lg border border-gray-100 bg-white px-2.5 py-1.5 dark:border-dark-700 dark:bg-dark-800"
              >
                <div :class="['h-1.5 w-1.5 rounded-full', platformDotClass(gq.group_platform || '')]" />
                <span class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ gq.group_name || `Group #${gq.group_id}` }}</span>
                <span :class="['rounded px-1.5 py-0.5 text-[10px] font-medium', platformBadgeLightClass(gq.group_platform || '')]">
                  {{ platformLabel(gq.group_platform || '') }}
                </span>
                <!-- Hover tooltip with quota details -->
                <div v-if="hasAnyQuotaLimit(gq)"
                  class="pointer-events-none absolute left-1/2 top-full z-10 mt-1 -translate-x-1/2 rounded-lg border border-gray-200 bg-white px-3 py-2 opacity-0 shadow-lg transition-opacity group-hover/Chip:opacity-100 dark:border-dark-600 dark:bg-dark-800">
                  <div class="flex gap-3 whitespace-nowrap text-[11px]">
                    <template v-for="seg in quotaSegments(gq)" :key="seg.kind">
                      <span v-if="seg.daily"><span class="text-gray-400 dark:text-dark-500">{{ t('bundles.daily') }} </span><span class="font-medium text-gray-700 dark:text-gray-300">{{ formatSegmentValue(seg.kind, seg.daily) }}</span></span>
                      <span v-if="seg.weekly"><span class="text-gray-400 dark:text-dark-500">{{ t('bundles.weekly') }} </span><span class="font-medium text-gray-700 dark:text-gray-300">{{ formatSegmentValue(seg.kind, seg.weekly) }}</span></span>
                      <span v-if="seg.monthly"><span class="text-gray-400 dark:text-dark-500">{{ t('bundles.monthly') }} </span><span class="font-medium text-gray-700 dark:text-gray-300">{{ formatSegmentValue(seg.kind, seg.monthly) }}</span></span>
                    </template>
                  </div>
                </div>
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
              v-for="plan in plans"
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
                    <p v-if="plan.description"
                      class="mt-0.5 line-clamp-2 text-xs leading-relaxed text-gray-500 dark:text-dark-400"
                      :title="plan.description">
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

                <!-- Features (hero section) -->
                <div v-if="plan.features?.length" class="mb-3 rounded-lg bg-gray-50 px-3 py-2.5 dark:bg-dark-700/50">
                  <div class="space-y-1.5">
                    <div v-for="feature in plan.features" :key="feature" class="flex items-start gap-2">
                      <svg :class="['mt-0.5 h-4 w-4 flex-shrink-0', tierIconClass(plan.tier)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                      </svg>
                      <span class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ feature }}</span>
                    </div>
                  </div>
                </div>

                <!-- Group Quotas (summary + compact chips) -->
                <div v-if="plan.group_quotas?.length" class="mb-3">
                  <p class="mb-1.5 text-xs font-medium text-gray-500 dark:text-gray-400">
                    {{ t('bundles.includesGroupCount', { count: plan.group_quotas.length }) }}
                  </p>
                  <div class="flex flex-wrap gap-1.5">
                    <div
                      v-for="gq in plan.group_quotas"
                      :key="gq.id"
                      class="group/Chip relative flex items-center gap-1.5 rounded-lg border border-gray-100 bg-white px-2.5 py-1.5 dark:border-dark-600 dark:bg-dark-800"
                    >
                      <span :class="['h-1.5 w-1.5 rounded-full', platformDotClass(gq.group_platform || '')]" />
                      <span class="text-[11px] font-medium text-gray-700 dark:text-gray-300">{{ gq.group_name || `Group #${gq.group_id}` }}</span>
                      <span :class="['rounded px-1 py-0.5 text-[10px] font-medium', platformBadgeLightClass(gq.group_platform || '')]">
                        {{ platformLabel(gq.group_platform || '') }}
                      </span>
                      <!-- Hover tooltip with quota details -->
                      <div v-if="hasAnyQuotaLimit(gq)"
                        class="pointer-events-none absolute left-1/2 top-full z-10 mt-1 -translate-x-1/2 rounded-lg border border-gray-200 bg-white px-3 py-2 opacity-0 shadow-lg transition-opacity group-hover/Chip:opacity-100 dark:border-dark-600 dark:bg-dark-800">
                        <div class="flex gap-3 whitespace-nowrap text-[11px]">
                          <template v-for="seg in quotaSegments(gq)" :key="seg.kind">
                            <span v-if="seg.daily"><span class="text-gray-400 dark:text-dark-500">{{ t('bundles.daily') }} </span><span class="font-medium text-gray-700 dark:text-gray-300">{{ formatSegmentValue(seg.kind, seg.daily) }}</span></span>
                            <span v-if="seg.weekly"><span class="text-gray-400 dark:text-dark-500">{{ t('bundles.weekly') }} </span><span class="font-medium text-gray-700 dark:text-gray-300">{{ formatSegmentValue(seg.kind, seg.weekly) }}</span></span>
                            <span v-if="seg.monthly"><span class="text-gray-400 dark:text-dark-500">{{ t('bundles.monthly') }} </span><span class="font-medium text-gray-700 dark:text-gray-300">{{ formatSegmentValue(seg.kind, seg.monthly) }}</span></span>
                          </template>
                        </div>
                      </div>
                    </div>
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

                <!-- Purchase / Upgrade Button -->
                <!-- 1) 当前已订阅套餐 → 使用中（置灰） -->
                <button
                  v-if="isCurrentPlan(plan)"
                  :class="['w-full rounded-xl py-2.5 text-sm font-semibold', tierDisabledBtnClass(plan.tier)]"
                  disabled
                >
                  {{ t('bundles.currentPlan') }}
                </button>
                <!-- 2) 已有套餐 & 目标价值更高 → 升级 -->
                <button
                  v-else-if="activeBundle && isUpgradeTarget(plan)"
                  :class="['w-full rounded-xl py-2.5 text-sm font-semibold transition-all active:scale-[0.98] flex items-center justify-center gap-1.5', tierBtnClass(plan.tier)]"
                  :disabled="upgradeLoading && upgradeTargetPlan?.id === plan.id"
                  @click="handleUpgradeClick(plan)"
                >
                  <span
                    v-if="upgradeLoading && upgradeTargetPlan?.id === plan.id"
                    class="h-3.5 w-3.5 animate-spin rounded-full border-2 border-white border-t-transparent"
                  />
                  {{ t('bundles.upgrade') }}
                </button>
                <!-- 3) 已有套餐 & 目标价值不高 → 到期后可购买（置灰） -->
                <button
                  v-else-if="activeBundle"
                  :class="['w-full rounded-xl py-2.5 text-sm font-semibold', tierDisabledBtnClass(plan.tier)]"
                  disabled
                >
                  {{ t('bundles.notUpgradeable') }}
                </button>
                <!-- 4) 无活跃套餐 → 立即购买 -->
                <button
                  v-else
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

    <!-- Upgrade Confirm Modal -->
    <Teleport to="body">
      <Transition name="modal">
        <div
          v-if="showUpgradeModal && upgradePreview && upgradeTargetPlan"
          class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm"
          @click.self="closeUpgradeModal"
        >
          <div class="relative w-full max-w-md rounded-2xl border border-gray-200 bg-white p-6 shadow-2xl dark:border-dark-700 dark:bg-dark-900">
            <!-- Close -->
            <button
              class="absolute right-4 top-4 rounded-lg p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700 dark:hover:text-gray-200"
              @click="closeUpgradeModal"
            >
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>

            <h3 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('bundles.upgradeConfirmTitle') }}
            </h3>

            <!-- Plan transition: old → new -->
            <div class="mb-4 flex items-center gap-2 rounded-xl bg-gray-50 px-3 py-2.5 dark:bg-dark-700/50">
              <div class="min-w-0 flex-1">
                <p class="text-[11px] text-gray-400 dark:text-gray-500">{{ t('bundles.upgradeFromLabel') }}</p>
                <p class="truncate text-sm font-medium text-gray-700 dark:text-gray-300">{{ upgradePreview.old_plan_name }}</p>
              </div>
              <svg class="h-4 w-4 shrink-0 text-gray-400 dark:text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M13 7l5 5m0 0l-5 5m5-5H6" />
              </svg>
              <div class="min-w-0 flex-1 text-right">
                <p class="text-[11px] text-gray-400 dark:text-gray-500">{{ t('bundles.upgradeToLabel') }}</p>
                <p class="truncate text-sm font-bold text-primary-600 dark:text-primary-400">{{ upgradePreview.new_plan_name }}</p>
              </div>
            </div>

            <!-- Credit / Due / Validity breakdown -->
            <div class="space-y-2 text-sm">
              <div class="flex items-start gap-2 rounded-lg bg-emerald-50 px-3 py-2 dark:bg-emerald-900/20">
                <svg class="mt-0.5 h-4 w-4 shrink-0 text-emerald-600 dark:text-emerald-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                </svg>
                <span class="text-emerald-700 dark:text-emerald-300">
                  {{ t('bundles.upgradeCreditHint', { credit: upgradePreview.credit.toFixed(2) }) }}
                </span>
              </div>
              <div class="flex items-start gap-2 rounded-lg bg-amber-50 px-3 py-2 dark:bg-amber-900/20">
                <svg class="mt-0.5 h-4 w-4 shrink-0 text-amber-600 dark:text-amber-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z" />
                </svg>
                <span class="text-amber-700 dark:text-amber-300">
                  {{ t('bundles.upgradeDueHint', { due: upgradePreview.due_amount.toFixed(2), days: upgradePreview.validity_days }) }}
                </span>
              </div>
            </div>

            <!-- Actions -->
            <div class="mt-5 flex gap-2">
              <button class="btn btn-secondary flex-1" @click="closeUpgradeModal">
                {{ t('common.cancel') }}
              </button>
              <button
                class="flex flex-1 items-center justify-center gap-1.5 rounded-xl bg-primary-500 px-4 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-600 active:bg-primary-700"
                @click="goToUpgradePayment"
              >
                {{ t('bundles.goToPayUpgrade') }}
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { getPlans, getMyBundle, previewBundleUpgrade } from '@/api/bundles'
import type { UpgradePreview } from '@/api/bundles'
import type { BundlePlan, BundleSubscription } from '@/types/bundle'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { platformBadgeLightClass, platformLabel } from '@/utils/platformColors'
import { getTierTheme, getTierI18nKey } from '@/constants/bundleTiers'
import { formatDateOnly, formatTimeOnly } from '@/utils/format'
import { formatSegmentValue, hasAnyQuotaLimit, quotaSegments } from '@/utils/bundleQuota'

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

// 套餐升级流程状态
// 升级目标套餐（点击「升级」后暂存，用于确认窗展示）
const upgradeTargetPlan = ref<BundlePlan | null>(null)
// 升级试算结果（差价 / 抵扣 / 新有效期）
const upgradePreview = ref<UpgradePreview | null>(null)
// 试算请求 loading（点击升级 → 拉取 preview 期间）
const upgradeLoading = ref(false)
// 确认窗开关
const showUpgradeModal = ref(false)

// 从已加载的 plans 列表中匹配当前订阅的套餐（后端未返回 plan 详情）
const activePlan = computed<BundlePlan | null>(() => {
  if (!activeBundle.value) return null
  return plans.value.find(p => p.id === activeBundle.value!.plan_id) ?? null
})

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

function tierDisabledBtnClass(tier?: string): string {
  return getTierTheme(tier).disabledBtnClass
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

// 格式化到期时间（时分秒，弱化展示）
function formatExpirationTime(expiresAt: string): string {
  return formatTimeOnly(expiresAt)
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

// 判断套餐是否为当前已订阅的套餐
function isCurrentPlan(plan: BundlePlan): boolean {
  return activeBundle.value?.plan_id === plan.id
}

// 判断套餐对当前用户是否为「可升级」目标（价值更高方可升级）
function isUpgradeTarget(plan: BundlePlan): boolean {
  if (!activeBundle.value || !activePlan.value) return false
  if (isCurrentPlan(plan)) return false
  // 升级方向：目标套餐价格严格高于当前套餐价格
  // 最终是否可升以 previewBundleUpgrade 返回的 upgradeable 为准，这里仅做按钮展示启发判断
  return plan.price > activePlan.value.price
}

// 处理购买点击 — 跳转到支付页完成购买
function handlePurchase(plan: BundlePlan) {
  // 已有生效中的套餐时拦截：套餐暂不支持重复购买/并存，提前提示避免走到支付页才被拒
  if (activeBundle.value) {
    appStore.showError(t('payment.errors.BUNDLE_CONFLICT'))
    return
  }
  router.push({ path: '/purchase', query: { bundle_plan_id: String(plan.id) } })
}

// 点击「升级」：拉取差价试算，可升级则弹确认窗
async function handleUpgradeClick(plan: BundlePlan) {
  if (!activeBundle.value || upgradeLoading.value) return
  upgradeLoading.value = true
  upgradeTargetPlan.value = plan
  upgradePreview.value = null
  try {
    const preview = await previewBundleUpgrade(activeBundle.value.id, plan.id)
    upgradePreview.value = preview
    if (!preview.upgradeable) {
      // 后端判定不可升级（理论上不应走到，价格启发已过滤），兜底提示
      appStore.showInfo(t('bundles.notUpgradeable'))
      return
    }
    showUpgradeModal.value = true
  } catch (error) {
    console.error('Failed to preview bundle upgrade:', error)
    appStore.showError(t('bundles.upgradeFailedToPreview'))
  } finally {
    upgradeLoading.value = false
  }
}

// 确认升级 —— 跳转支付页走升级支付流程（复用 PaymentView 的支付方式选择 / 跳转 / 二维码）
function goToUpgradePayment() {
  const target = upgradeTargetPlan.value
  const preview = upgradePreview.value
  const source = activeBundle.value
  if (!target || !preview || !source) return
  showUpgradeModal.value = false
  router.push({
    path: '/purchase',
    query: {
      bundle_plan_id: String(target.id),
      upgrade_from: String(source.id),
      due: String(preview.due_amount),
      credit: String(preview.credit),
    },
  })
}

function closeUpgradeModal() {
  showUpgradeModal.value = false
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
      const bundles = bundleData.value
      activeBundle.value = bundles[0] ?? null
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
