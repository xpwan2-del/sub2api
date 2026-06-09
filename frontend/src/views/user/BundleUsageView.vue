<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-20">
        <div class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></div>
      </div>

      <template v-else>
        <!-- No Active Bundle -->
        <div v-if="!bundle" class="card p-12 text-center">
          <div class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700">
            <Icon name="cube" size="xl" class="text-gray-400" />
          </div>
          <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">{{ t('bundles.noActiveBundle') }}</h3>
          <p class="mb-4 text-gray-500 dark:text-gray-400">{{ t('bundles.noActiveBundleDesc') }}</p>
          <button
            class="rounded-xl bg-primary-500 px-5 py-2 text-sm font-semibold text-white transition-colors hover:bg-primary-600"
            @click="router.push('/bundles')"
          >
            {{ t('bundles.browsePlans') }}
          </button>
        </div>

        <template v-else>
          <!-- Bundle Info Header -->
          <div class="overflow-hidden rounded-2xl border border-primary-500/20 bg-white dark:bg-dark-800">
            <div class="flex items-center gap-3 border-b border-gray-100 p-4 dark:border-dark-700">
              <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-primary-100 dark:bg-primary-900/40">
                <Icon name="cube" size="lg" class="text-primary-600 dark:text-primary-400" />
              </div>
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2">
                  <h2 class="truncate text-lg font-bold text-gray-900 dark:text-white">
                    {{ bundle.plan?.name || t('bundles.currentBundle') }}
                  </h2>
                  <span :class="tierBadgeClass(bundle.plan?.tier)">
                    {{ tierLabel(bundle.plan?.tier) }}
                  </span>
                </div>
                <div class="mt-0.5 flex flex-wrap gap-x-4 text-xs text-gray-500 dark:text-gray-400">
                  <span>{{ formatExpirationDate(bundle.expires_at) }}</span>
                  <span :class="remainingDaysClass(bundle.expires_at)">
                    {{ remainingDaysText(bundle.expires_at) }}
                  </span>
                </div>
              </div>
            </div>

            <!-- Concurrency / RPM -->
            <div class="flex gap-4 border-b border-gray-100 p-4 dark:border-dark-700">
              <div class="flex items-center gap-2 rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-700/50">
                <Icon name="bolt" size="sm" class="text-amber-500" />
                <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.concurrency') }}</span>
                <span class="text-sm font-bold text-gray-900 dark:text-white">{{ bundle.concurrency_limit || '-' }}</span>
              </div>
              <div class="flex items-center gap-2 rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-700/50">
                <Icon name="clock" size="sm" class="text-blue-500" />
                <span class="text-xs text-gray-500 dark:text-gray-400">RPM</span>
                <span class="text-sm font-bold text-gray-900 dark:text-white">{{ bundle.rpm_limit || '-' }}</span>
              </div>
            </div>
          </div>

          <!-- Usage Cards by Group -->
          <div>
            <h3 class="mb-4 text-base font-bold text-gray-900 dark:text-white">{{ t('bundles.usageByGroup') }}</h3>

            <div v-if="usages.length === 0" class="card py-12 text-center">
              <Icon name="chart" size="xl" class="mx-auto mb-3 text-gray-300 dark:text-dark-600" />
              <p class="text-gray-500 dark:text-gray-400">{{ t('bundles.noUsageData') }}</p>
            </div>

            <div v-else class="grid gap-4 sm:grid-cols-2">
              <div
                v-for="usage in usages"
                :key="usage.group_id"
                class="overflow-hidden rounded-2xl border bg-white dark:bg-dark-800"
                :class="platformBorderClass(usage.platform)"
              >
                <!-- Group Header -->
                <div class="flex items-center gap-2 border-b border-gray-100 p-3 dark:border-dark-700">
                  <div :class="['h-1.5 w-1.5 rounded-full', platformDotClass(usage.platform)]" />
                  <span class="text-sm font-semibold text-gray-900 dark:text-white">{{ usage.group_name }}</span>
                  <span :class="['rounded-md border px-2 py-0.5 text-[11px] font-medium', platformBadgeLightClass(usage.platform)]">
                    {{ platformLabel(usage.platform) }}
                  </span>
                  <span v-if="usage.model_pattern" class="ml-auto rounded bg-gray-100 px-1.5 py-0.5 text-[10px] text-gray-500 dark:bg-dark-700 dark:text-gray-400">
                    {{ usage.model_pattern }}
                  </span>
                </div>

                <!-- Progress Bars -->
                <div class="space-y-3 p-3">
                  <!-- Daily Usage -->
                  <div v-if="usage.daily_limit > 0" class="space-y-1.5">
                    <div class="flex items-center justify-between text-xs">
                      <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('bundles.daily') }}</span>
                      <span class="text-gray-500 dark:text-gray-400">${{ usage.daily_used.toFixed(2) }} / ${{ usage.daily_limit.toFixed(2) }}</span>
                    </div>
                    <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                      <div
                        class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                        :class="progressBarClass(usage.daily_used, usage.daily_limit)"
                        :style="{ width: progressWidth(usage.daily_used, usage.daily_limit) }"
                      ></div>
                    </div>
                  </div>

                  <!-- Weekly Usage -->
                  <div v-if="usage.weekly_limit > 0" class="space-y-1.5">
                    <div class="flex items-center justify-between text-xs">
                      <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('bundles.weekly') }}</span>
                      <span class="text-gray-500 dark:text-gray-400">${{ usage.weekly_used.toFixed(2) }} / ${{ usage.weekly_limit.toFixed(2) }}</span>
                    </div>
                    <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                      <div
                        class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                        :class="progressBarClass(usage.weekly_used, usage.weekly_limit)"
                        :style="{ width: progressWidth(usage.weekly_used, usage.weekly_limit) }"
                      ></div>
                    </div>
                  </div>

                  <!-- Monthly Usage -->
                  <div v-if="usage.monthly_limit > 0" class="space-y-1.5">
                    <div class="flex items-center justify-between text-xs">
                      <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('bundles.monthly') }}</span>
                      <span class="text-gray-500 dark:text-gray-400">${{ usage.monthly_used.toFixed(2) }} / ${{ usage.monthly_limit.toFixed(2) }}</span>
                    </div>
                    <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                      <div
                        class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                        :class="progressBarClass(usage.monthly_used, usage.monthly_limit)"
                        :style="{ width: progressWidth(usage.monthly_used, usage.monthly_limit) }"
                      ></div>
                    </div>
                  </div>

                  <!-- No limits -->
                  <div
                    v-if="usage.daily_limit === 0 && usage.weekly_limit === 0 && usage.monthly_limit === 0"
                    class="flex items-center justify-center rounded-lg bg-emerald-50 py-3 dark:bg-emerald-900/20"
                  >
                    <span class="text-sm text-emerald-600 dark:text-emerald-400">∞ {{ t('bundles.unlimited') }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Back to Bundles -->
          <div class="flex justify-center">
            <button
              class="text-sm text-gray-500 transition-colors hover:text-primary-600 dark:text-gray-400 dark:hover:text-primary-400"
              @click="router.push('/bundles')"
            >
              {{ t('bundles.backToBundles') }}
            </button>
          </div>
        </template>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { getMyBundle, getMyUsage } from '@/api/bundles'
import type { BundleSubscription, BundleUsageProgress } from '@/types/bundle'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { platformBadgeLightClass, platformBorderClass, platformLabel } from '@/utils/platformColors'
import { getTierTheme, getTierI18nKey } from '@/constants/bundleTiers'
import { formatDateOnly } from '@/utils/format'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()

const loading = ref(true)
const bundle = ref<BundleSubscription | null>(null)
const usages = ref<BundleUsageProgress[]>([])

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

function progressWidth(used: number, limit: number): string {
  if (!limit || limit === 0) return '0%'
  return `${Math.min((used / limit) * 100, 100)}%`
}

function progressBarClass(used: number, limit: number): string {
  if (!limit || limit === 0) return 'bg-gray-400'
  const pct = (used / limit) * 100
  if (pct >= 100) return 'bg-red-500'
  if (pct >= 80) return 'bg-orange-500'
  return 'bg-green-500'
}

function formatExpirationDate(expiresAt: string): string {
  return formatDateOnly(expiresAt)
}

function remainingDaysText(expiresAt: string): string {
  const diff = new Date(expiresAt).getTime() - Date.now()
  const days = Math.max(0, Math.ceil(diff / (1000 * 60 * 60 * 24)))
  return t('bundles.daysRemaining', { days })
}

function remainingDaysClass(expiresAt: string): string {
  const diff = new Date(expiresAt).getTime() - Date.now()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))
  if (days <= 0) return 'font-medium text-red-600 dark:text-red-400'
  if (days <= 3) return 'font-medium text-red-600 dark:text-red-400'
  if (days <= 7) return 'text-orange-600 dark:text-orange-400'
  return ''
}

async function loadData() {
  try {
    loading.value = true
    const [bundleData, usageData] = await Promise.allSettled([
      getMyBundle(),
      getMyUsage()
    ])
    if (bundleData.status === 'fulfilled') {
      bundle.value = bundleData.value
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

onMounted(() => {
  loadData()
})
</script>
