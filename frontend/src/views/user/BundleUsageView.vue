<template>
  <AppLayout>
    <div class="max-w-4xl mx-auto space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-20">
        <div class="w-8 h-8 border-4 rounded-full animate-spin border-primary-500 border-t-transparent"></div>
      </div>

      <template v-else>
        <!-- No Active Bundle -->
        <div v-if="!bundle" class="p-12 text-center card">
          <div class="flex items-center justify-center w-16 h-16 mx-auto mb-4 bg-gray-100 rounded-full dark:bg-dark-700">
            <Icon name="cube" size="xl" class="text-gray-400" />
          </div>
          <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">{{ t('bundles.noActiveBundle') }}</h3>
          <p class="mb-4 text-gray-500 dark:text-gray-400">{{ t('bundles.noActiveBundleDesc') }}</p>
          <button
            class="px-5 py-2 text-sm font-semibold text-white transition-colors rounded-xl bg-primary-500 hover:bg-primary-600"
            @click="router.push('/bundles')"
          >
            {{ t('bundles.browsePlans') }}
          </button>
        </div>

        <template v-else>
          <!-- Bundle Info Header — 参考 BundlesView 活跃套餐卡片样式 -->
          <div class="overflow-hidden border rounded-2xl border-primary-500/20 bg-gradient-to-r from-primary-50 to-white dark:from-primary-900/20 dark:to-dark-800">
            <!-- 标题栏 -->
            <div class="flex items-center gap-3 p-4 border-b border-primary-100 dark:border-dark-700">
              <div class="flex items-center justify-center w-10 h-10 rounded-xl bg-primary-100 dark:bg-primary-900/40">
                <Icon name="cube" size="lg" class="text-primary-600 dark:text-primary-400" />
              </div>
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2">
                  <h2 class="text-lg font-bold text-gray-900 truncate dark:text-white">
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

            <!-- 信息网格：到期时间 + 并发数 + RPM -->
            <div class="grid gap-4 p-4 sm:grid-cols-3">
              <!-- Expiration -->
              <div class="p-3 rounded-xl bg-white/60 dark:bg-dark-700/40">
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.expiresAt') }}</p>
                <p :class="expirationClass">
                  {{ formatExpirationDate(bundle.expires_at) }}
                  <span class="ml-1 align-middle text-sm font-normal text-gray-400 dark:text-gray-500">{{ formatExpirationTime(bundle.expires_at) }}</span>
                </p>
                <p :class="['mt-0.5 text-xs', remainingDaysColorClass]">
                  {{ remainingDaysText(bundle.expires_at) }}
                </p>
              </div>
              <!-- Concurrency -->
              <div class="p-3 rounded-xl bg-white/60 dark:bg-dark-700/40">
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.concurrency') }}</p>
                <p class="text-lg font-bold text-gray-900 dark:text-white">{{ bundle.concurrency_limit || '-' }}</p>
              </div>
              <!-- RPM -->
              <div class="p-3 rounded-xl bg-white/60 dark:bg-dark-700/40">
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.rpm') }}</p>
                <p class="text-lg font-bold text-gray-900 dark:text-white">{{ bundle.rpm_limit || '-' }}</p>
              </div>
            </div>

            <div class="p-4 border-t border-primary-100 dark:border-dark-700">
              <button
                class="inline-flex items-center gap-1.5 rounded-xl bg-primary-500 px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-primary-600 active:bg-primary-700"
                @click="router.push('/bundles')"
              >
                <Icon name="arrowLeft" size="sm" />
                {{ t('bundles.backToBundles') }}
              </button>
            </div>
          </div>

          <!-- Usage Cards by Group -->
          <div>
            <div class="flex items-center justify-between mb-4">
              <div class="flex items-center gap-2">
                <h3 class="text-base font-bold text-gray-900 dark:text-white">{{ t('bundles.usageByGroup') }}</h3>
                <button
                  type="button"
                  class="btn btn-secondary btn-icon"
                  :title="t('common.refresh')"
                  :disabled="refreshing"
                  @click="refreshUsage"
                >
                  <Icon name="refresh" size="sm" :class="refreshing ? 'animate-spin' : ''" />
                </button>
              </div>
              <button
                class="inline-flex items-center gap-1 text-sm font-medium transition-colors text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                @click="router.push('/usage')"
              >
                <Icon name="document" size="sm" />
                {{ t('bundles.viewUsageRecords') }}
                <Icon name="chevronRight" size="sm" />
              </button>
            </div>

            <div v-if="usages.length === 0" class="py-12 text-center card">
              <Icon name="chart" size="xl" class="mx-auto mb-3 text-gray-300 dark:text-dark-600" />
              <p class="text-gray-500 dark:text-gray-400">{{ t('bundles.noUsageData') }}</p>
            </div>

            <div v-else class="grid gap-4 sm:grid-cols-2">
              <div
                v-for="usage in usages"
                :key="usage.group_id"
                class="flex flex-col overflow-hidden bg-white border rounded-2xl dark:bg-dark-800"
                :class="platformBorderClass(usage.platform)"
              >
                <!-- Group Header -->
                <div class="flex items-center gap-2 p-3 border-b border-gray-100 dark:border-dark-700">
                  <div :class="['h-2 w-2 rounded-full', platformDotClass(usage.platform)]" />
                  <span class="text-sm font-semibold text-gray-900 dark:text-white">
                    {{ usage.group_name || t('bundles.groupFallback', { id: usage.group_id }) }}
                  </span>
                  <span :class="['rounded-md border px-2 py-0.5 text-[11px] font-medium', platformBadgeLightClass(usage.platform)]">
                    {{ platformLabel(usage.platform) }}
                  </span>
                  <div v-if="usage.model_pattern" class="flex flex-wrap items-center justify-end gap-1 ml-auto">
                    <span
                      v-for="(p, i) in splitModelPatterns(usage.model_pattern)"
                      :key="i"
                      class="inline-block rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-200"
                    >{{ p }}</span>
                  </div>
                </div>

                <!-- Progress Bars（有限额时展示进度条） -->
                <div v-if="hasAnyUsageLimit(usage)" class="p-3 space-y-3">
                  <!-- 限额（日/周/月 USD 用量） -->
                  <template v-if="hasUsdLimit(usage)">
                    <!-- <div class="pt-2 border-t border-gray-100 dark:border-dark-700"></div> -->
                    <div class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ t('bundles.limits') }}</div>
                    <!-- Daily Usage -->
                    <div v-if="usage.daily_limit_usd > 0" class="space-y-1">
                      <div class="flex items-center justify-between text-[11px]">
                        <span class="text-gray-500 dark:text-gray-400">{{ t('bundles.daily') }}</span>
                        <span class="text-gray-500 dark:text-gray-400">${{ usage.daily_usage_usd.toFixed(2) }} / ${{ usage.daily_limit_usd.toFixed(2) }}</span>
                      </div>
                      <div class="relative h-1.5 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                        <div
                          class="absolute inset-y-0 left-0 transition-all duration-300 rounded-full"
                          :class="progressBarClass(usage.daily_usage_usd, usage.daily_limit_usd)"
                          :style="{ width: progressWidth(usage.daily_usage_usd, usage.daily_limit_usd) }"
                        ></div>
                      </div>
                    </div>

                    <!-- Weekly Usage -->
                    <div v-if="usage.weekly_limit_usd > 0" class="space-y-1">
                      <div class="flex items-center justify-between text-[11px]">
                        <span class="text-gray-500 dark:text-gray-400">{{ t('bundles.weekly') }}</span>
                        <span class="text-gray-500 dark:text-gray-400">${{ usage.weekly_usage_usd.toFixed(2) }} / ${{ usage.weekly_limit_usd.toFixed(2) }}</span>
                      </div>
                      <div class="relative h-1.5 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                        <div
                          class="absolute inset-y-0 left-0 transition-all duration-300 rounded-full"
                          :class="progressBarClass(usage.weekly_usage_usd, usage.weekly_limit_usd)"
                          :style="{ width: progressWidth(usage.weekly_usage_usd, usage.weekly_limit_usd) }"
                        ></div>
                      </div>
                    </div>

                    <!-- Monthly Usage -->
                    <div v-if="usage.monthly_limit_usd > 0" class="space-y-1">
                      <div class="flex items-center justify-between text-[11px]">
                        <span class="text-gray-500 dark:text-gray-400">{{ t('bundles.monthly') }}</span>
                        <span class="text-gray-500 dark:text-gray-400">${{ usage.monthly_usage_usd.toFixed(2) }} / ${{ usage.monthly_limit_usd.toFixed(2) }}</span>
                      </div>
                      <div class="relative h-1.5 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                        <div
                          class="absolute inset-y-0 left-0 transition-all duration-300 rounded-full"
                          :class="progressBarClass(usage.monthly_usage_usd, usage.monthly_limit_usd)"
                          :style="{ width: progressWidth(usage.monthly_usage_usd, usage.monthly_limit_usd) }"
                        ></div>
                      </div>
                    </div>
                  </template>

                  <!-- Image count usage -->
                  <template v-if="hasImageLimit(usage)">
                    <!-- <div class="pt-2 border-t border-gray-100 dark:border-dark-700"></div> -->
                    <div class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ t('bundles.image') }}</div>
                    <template v-for="m in countMetrics(usage, 'image')" :key="m.key">
                      <div v-if="m.limit > 0" class="space-y-1">
                        <div class="flex items-center justify-between text-[11px]">
                          <span class="text-gray-500 dark:text-gray-400">{{ m.label }}</span>
                          <span class="text-gray-500 dark:text-gray-400">{{ m.used }} / {{ m.limit }}</span>
                        </div>
                        <div class="relative h-1.5 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                          <div
                            class="absolute inset-y-0 left-0 transition-all duration-300 rounded-full"
                            :class="progressBarClass(m.used, m.limit)"
                            :style="{ width: progressWidth(m.used, m.limit) }"
                          ></div>
                        </div>
                      </div>
                    </template>
                  </template>

                  <!-- Video count usage -->
                  <template v-if="hasVideoLimit(usage)">
                    <!-- <div class="pt-2 border-t border-gray-100 dark:border-dark-700"></div> -->
                    <div class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ t('bundles.video') }}</div>
                    <template v-for="m in countMetrics(usage, 'video')" :key="'video-' + m.key">
                      <div v-if="m.limit > 0" class="space-y-1">
                        <div class="flex items-center justify-between text-[11px]">
                          <span class="text-gray-500 dark:text-gray-400">{{ m.label }}</span>
                          <span class="text-gray-500 dark:text-gray-400">{{ m.used }} / {{ m.limit }}</span>
                        </div>
                        <div class="relative h-1.5 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                          <div
                            class="absolute inset-y-0 left-0 transition-all duration-300 rounded-full"
                            :class="progressBarClass(m.used, m.limit)"
                            :style="{ width: progressWidth(m.used, m.limit) }"
                          ></div>
                        </div>
                      </div>
                    </template>
                  </template>

                </div>

                <!-- No limits：不限额度——浅绿色块撑满卡片剩余空间，文案居中 -->
                <div
                  v-else
                  class="flex flex-1 items-center justify-center min-h-[120px] bg-emerald-50 dark:bg-emerald-900/20"
                >
                  <span class="text-sm text-emerald-600 dark:text-emerald-400">∞ {{ t('bundles.unlimited') }}</span>
                </div>
              </div>
            </div>
          </div>

        </template>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { getPlans, getMyBundle, getMyUsage } from '@/api/bundles'
import type { BundlePlan, BundleSubscription, BundleUsageProgress } from '@/types/bundle'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { platformBadgeLightClass, platformBorderClass, platformLabel } from '@/utils/platformColors'
import { getTierTheme, getTierI18nKey } from '@/constants/bundleTiers'
import { formatDateOnly, formatTimeOnly } from '@/utils/format'
import { splitModelPatterns } from '@/utils/bundleQuota'

// ==================== BundleUsageView：用户套餐用量页 ====================
// 展示用户当前套餐的各渠道组用量进度（日/周/月），
// 用进度条可视化用量占比，颜色随用量变化（绿→橙→红）

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()

// 页面加载状态
const loading = ref(true)
// 在售套餐计划列表（用于获取完整 plan 信息：名称、描述、tier）
const plans = ref<BundlePlan[]>([])
// 当前套餐订阅
const bundle = ref<BundleSubscription | null>(null)
// 各渠道组的用量进度列表
const usages = ref<BundleUsageProgress[]>([])
// 手动刷新用量时的局部 loading（仅图标旋转，不触发整页骨架）
const refreshing = ref(false)

// 从 plans 中匹配当前订阅的 plan（和 BundlesView 一致的方式）
const activePlan = computed<BundlePlan | null>(() => {
  if (!bundle.value) return null
  return plans.value.find(p => p.id === bundle.value!.plan_id) ?? null
})

function tierLabel(tier?: string): string {
  return t(getTierI18nKey(tier, 'user'))
}

function tierBadgeClass(tier?: string): string {
  return getTierTheme(tier).badgeClass
}

function platformDotClass(p: string): string {
  switch (p) {
    case 'anthropic': return 'bg-orange-500'
    case 'openai': return 'bg-emerald-500'
    case 'antigravity': return 'bg-purple-500'
    case 'gemini': return 'bg-blue-500'
    default: return 'bg-gray-400'
  }
}

// 计算进度条宽度百分比
function progressWidth(used: number, limit: number): string {
  if (!limit || limit === 0) return '0%'
  return `${Math.min((used / limit) * 100, 100)}%`
}

// 进度条颜色：<80% 绿色 / 80-100% 橙色 / >=100% 红色
function progressBarClass(used: number, limit: number): string {
  if (!limit || limit === 0) return 'bg-gray-400'
  const pct = (used / limit) * 100
  if (pct >= 100) return 'bg-red-500'
  if (pct >= 80) return 'bg-orange-500'
  return 'bg-green-500'
}

// 是否配置了图片/视频次数限额
function hasImageLimit(u: BundleUsageProgress): boolean {
  return u.daily_image_limit_count > 0 || u.weekly_image_limit_count > 0 || u.monthly_image_limit_count > 0
}
function hasVideoLimit(u: BundleUsageProgress): boolean {
  return u.daily_video_limit_count > 0 || u.weekly_video_limit_count > 0 || u.monthly_video_limit_count > 0
}
// 是否配置了日/周/月 USD 金额限额
function hasUsdLimit(u: BundleUsageProgress): boolean {
  return u.daily_limit_usd > 0 || u.weekly_limit_usd > 0 || u.monthly_limit_usd > 0
}
// 是否有任何限额（USD 或 count）；全为 0 时显示"不限"
function hasAnyUsageLimit(u: BundleUsageProgress): boolean {
  return hasUsdLimit(u) || hasImageLimit(u) || hasVideoLimit(u)
}
// 拼装某类（image/video）的日/周/月 count 指标，供模板 v-for 渲染
function countMetrics(u: BundleUsageProgress, kind: 'image' | 'video') {
  if (kind === 'image') {
    return [
      { key: 'daily', label: t('bundles.daily'), used: u.daily_image_usage_count, limit: u.daily_image_limit_count },
      { key: 'weekly', label: t('bundles.weekly'), used: u.weekly_image_usage_count, limit: u.weekly_image_limit_count },
      { key: 'monthly', label: t('bundles.monthly'), used: u.monthly_image_usage_count, limit: u.monthly_image_limit_count },
    ]
  }
  return [
    { key: 'daily', label: t('bundles.daily'), used: u.daily_video_usage_count, limit: u.daily_video_limit_count },
    { key: 'weekly', label: t('bundles.weekly'), used: u.weekly_video_usage_count, limit: u.weekly_video_limit_count },
    { key: 'monthly', label: t('bundles.monthly'), used: u.monthly_video_usage_count, limit: u.monthly_video_limit_count },
  ]
}

function formatExpirationDate(expiresAt: string): string {
  return formatDateOnly(expiresAt)
}

// 格式化到期时间（时分秒，弱化展示）
function formatExpirationTime(expiresAt: string): string {
  return formatTimeOnly(expiresAt)
}

function remainingDaysText(expiresAt: string): string {
  const diff = new Date(expiresAt).getTime() - Date.now()
  const days = Math.max(0, Math.ceil(diff / (1000 * 60 * 60 * 24)))
  return t('bundles.daysRemaining', { days })
}

// 到期日期文字颜色（<=3天红色 / <=7天橙色 / 正常灰色）
const expirationClass = computed(() => {
  if (!bundle.value?.expires_at) return 'text-lg font-bold text-gray-900 dark:text-white'
  const diff = new Date(bundle.value.expires_at).getTime() - Date.now()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))
  if (days <= 0) return 'text-lg font-bold text-red-600 dark:text-red-400'
  if (days <= 3) return 'text-lg font-bold text-red-600 dark:text-red-400'
  if (days <= 7) return 'text-lg font-bold text-orange-600 dark:text-orange-400'
  return 'text-lg font-bold text-gray-900 dark:text-white'
})

// 剩余天数文字颜色（辅助 expiration 下方的小字）
const remainingDaysColorClass = computed(() => {
  if (!bundle.value?.expires_at) return 'text-gray-400 dark:text-gray-500'
  const diff = new Date(bundle.value.expires_at).getTime() - Date.now()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))
  if (days <= 3) return 'font-medium text-red-600 dark:text-red-400'
  if (days <= 7) return 'text-orange-600 dark:text-orange-400'
  return 'text-gray-400 dark:text-gray-500'
})

// 并行加载套餐计划、订阅和用量数据
async function loadData() {
  try {
    loading.value = true
    const [plansData, bundleData, usageData] = await Promise.allSettled([
      getPlans(),
      getMyBundle(),
      getMyUsage()
    ])
    if (plansData.status === 'fulfilled') {
      plans.value = plansData.value
    }
    if (bundleData.status === 'fulfilled') {
      // 后端返回 BundleSubscription[]，取第一个或 null
      const bundles = bundleData.value
      bundle.value = bundles[0] ?? null
    }
    if (usageData.status === 'fulfilled') {
      usages.value = usageData.value
    }
  } catch (error) {
    console.error('Failed to load bundle usage:', error)
    appStore.showError(t('bundles.failedToLoad'))
  } finally {
    loading.value = false
  }
}

// 手动刷新各渠道组用量（仅重新拉取 usage，不重载套餐/订阅）
async function refreshUsage() {
  try {
    refreshing.value = true
    usages.value = await getMyUsage()
  } catch (error) {
    console.error('Failed to refresh bundle usage:', error)
    appStore.showError(t('bundles.failedToLoad'))
  } finally {
    refreshing.value = false
  }
}

onMounted(() => {
  loadData()
})
</script>
