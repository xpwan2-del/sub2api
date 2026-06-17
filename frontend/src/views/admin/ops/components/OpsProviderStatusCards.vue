<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpsModelStatus, OpsProviderStatusItem } from '@/api/admin/ops'
import OpsHealthHistoryBar from './OpsHealthHistoryBar.vue'

interface Props {
  providers: OpsProviderStatusItem[]
}

const props = defineProps<Props>()
const { t } = useI18n()

const providerCards = computed(() =>
  props.providers.map((provider) => ({ ...provider }))
)

function statusClass(status: OpsModelStatus): string {
  switch (status) {
    case 'operational':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'degraded':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    case 'failed':
      return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    case 'rate_limited':
      return 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300'
    default:
      return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  }
}

function statusDotClass(status: OpsModelStatus): string {
  switch (status) {
    case 'operational':
      return 'bg-emerald-500'
    case 'degraded':
      return 'bg-amber-500'
    case 'failed':
      return 'bg-red-500'
    case 'rate_limited':
      return 'bg-orange-500'
    default:
      return 'bg-gray-400'
  }
}

function formatRate(value: number): string {
  if (!Number.isFinite(value)) return '0.0%'
  return `${value.toFixed(1)}%`
}

function formatMs(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return value >= 1000 ? `${(value / 1000).toFixed(1)}s` : `${Math.round(value)}ms`
}
</script>

<template>
  <section class="space-y-3">
    <div class="flex items-center justify-between">
      <h3 class="text-sm font-bold text-gray-900 dark:text-white">
        {{ t('admin.ops.modelStatus.providerCards') }}
      </h3>
      <span class="text-xs text-gray-500 dark:text-gray-400">{{ providers.length }}</span>
    </div>

    <div class="grid grid-cols-1 gap-4 xl:grid-cols-2">
      <article
        v-for="provider in providerCards"
        :key="provider.platform"
        class="card p-4 md:p-5"
      >
        <div class="flex items-start justify-between gap-4">
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <span class="h-2.5 w-2.5 rounded-full" :class="statusDotClass(provider.status)" />
              <h4 class="truncate text-base font-bold text-gray-900 dark:text-white">
                {{ provider.platform }}
              </h4>
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.modelStatus.requests') }} {{ provider.request_count }}
              <span class="mx-1">/</span>
              {{ t('admin.ops.modelStatus.successRate') }} {{ formatRate(provider.success_rate) }}
            </p>
          </div>
          <span class="shrink-0 rounded-full px-2 py-0.5 text-xs font-semibold" :class="statusClass(provider.status)">
            {{ t(`admin.ops.modelStatus.status.${provider.status}`) }}
          </span>
        </div>

        <div class="mt-5">
          <div class="mb-2 flex items-center justify-between text-xs">
            <span class="font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.modelStatus.healthHistory') }}</span>
            <span class="text-gray-500 dark:text-gray-400">48h</span>
          </div>
          <OpsHealthHistoryBar :points="provider.history ?? []" />
        </div>

        <div class="mt-5 grid grid-cols-2 gap-3 text-xs md:grid-cols-4">
          <div class="rounded-lg bg-gray-50 p-2 dark:bg-dark-800">
            <div class="text-gray-500 dark:text-gray-400">{{ t('admin.ops.modelStatus.requests') }}</div>
            <div class="mt-0.5 font-bold text-gray-900 dark:text-white">{{ provider.request_count }}</div>
          </div>
          <div class="rounded-lg bg-gray-50 p-2 dark:bg-dark-800">
            <div class="text-gray-500 dark:text-gray-400">{{ t('admin.ops.modelStatus.successRate') }}</div>
            <div class="mt-0.5 font-bold text-gray-900 dark:text-white">{{ provider.request_count ? formatRate(provider.success_rate) : '-' }}</div>
          </div>
          <div class="rounded-lg bg-gray-50 p-2 dark:bg-dark-800">
            <div class="text-gray-500 dark:text-gray-400">P95</div>
            <div class="mt-0.5 font-bold text-gray-900 dark:text-white">{{ formatMs(provider.p95_latency_ms) }}</div>
          </div>
          <div class="rounded-lg bg-gray-50 p-2 dark:bg-dark-800">
            <div class="text-gray-500 dark:text-gray-400">{{ t('admin.ops.modelStatus.accountAvailability') }}</div>
            <div class="mt-0.5 font-bold text-gray-900 dark:text-white">{{ provider.available_accounts }} / {{ provider.total_accounts }}</div>
          </div>
        </div>
      </article>

      <div v-if="providerCards.length === 0" class="card p-6 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.ops.modelStatus.emptyProviders') }}
      </div>
    </div>
  </section>
</template>
