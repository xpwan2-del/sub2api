<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpsModelAccountAvailability, OpsModelSummary, OpsProviderStatusItem } from '@/api/admin/ops'

interface Props {
  summary: OpsModelSummary | null
  providers: OpsProviderStatusItem[]
  availability: OpsModelAccountAvailability | null
}

const props = defineProps<Props>()
const { t } = useI18n()

const rows = computed(() => [
  { key: 'total', label: t('admin.ops.modelStatus.totalModels'), value: props.summary?.total_models ?? 0, class: 'text-gray-900 dark:text-white' },
  { key: 'operational', label: t('admin.ops.modelStatus.status.operational'), value: props.summary?.operational ?? 0, class: 'text-emerald-600 dark:text-emerald-300' },
  { key: 'degraded', label: t('admin.ops.modelStatus.status.degraded'), value: props.summary?.degraded ?? 0, class: 'text-amber-600 dark:text-amber-300' },
  { key: 'failed', label: t('admin.ops.modelStatus.status.failed'), value: props.summary?.failed ?? 0, class: 'text-red-600 dark:text-red-300' },
  { key: 'no_recent_traffic', label: t('admin.ops.modelStatus.status.no_recent_traffic'), value: props.summary?.no_recent_traffic ?? 0, class: 'text-gray-600 dark:text-gray-300' }
])

function rate(provider: OpsProviderStatusItem): string {
  if (!provider.request_count) return '-'
  return `${provider.success_rate.toFixed(1)}%`
}
</script>

<template>
  <section class="space-y-4">
    <div class="grid grid-cols-2 gap-3 md:grid-cols-5">
      <div v-for="row in rows" :key="row.key" class="card p-4">
        <div class="text-xs text-gray-500 dark:text-gray-400">{{ row.label }}</div>
        <div class="mt-1 text-2xl font-bold" :class="row.class">{{ row.value }}</div>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
      <div class="card p-4">
        <h3 class="mb-3 text-sm font-bold text-gray-900 dark:text-white">
          {{ t('admin.ops.modelStatus.accountAvailability') }}
        </h3>
        <div class="grid grid-cols-2 gap-3 text-sm">
          <div>
            <div class="text-gray-500 dark:text-gray-400">{{ t('admin.ops.modelStatus.availableAccounts') }}</div>
            <div class="font-bold text-emerald-600 dark:text-emerald-300">{{ availability?.available_accounts ?? 0 }}</div>
          </div>
          <div>
            <div class="text-gray-500 dark:text-gray-400">{{ t('admin.ops.modelStatus.totalAccounts') }}</div>
            <div class="font-bold text-gray-900 dark:text-white">{{ availability?.total_accounts ?? 0 }}</div>
          </div>
          <div>
            <div class="text-gray-500 dark:text-gray-400">{{ t('admin.ops.modelStatus.rateLimitedAccounts') }}</div>
            <div class="font-bold text-amber-600 dark:text-amber-300">{{ availability?.rate_limited_accounts ?? 0 }}</div>
          </div>
          <div>
            <div class="text-gray-500 dark:text-gray-400">{{ t('admin.ops.modelStatus.errorAccounts') }}</div>
            <div class="font-bold text-red-600 dark:text-red-300">{{ availability?.error_accounts ?? 0 }}</div>
          </div>
        </div>
      </div>

      <div class="card p-4 lg:col-span-2">
        <h3 class="mb-3 text-sm font-bold text-gray-900 dark:text-white">
          {{ t('admin.ops.modelStatus.providers') }}
        </h3>
        <div class="grid grid-cols-1 gap-2 md:grid-cols-2 xl:grid-cols-3">
          <div v-for="provider in providers" :key="provider.platform" class="rounded-lg border border-gray-100 p-3 dark:border-dark-700">
            <div class="flex items-center justify-between gap-2">
              <span class="font-semibold text-gray-900 dark:text-white">{{ provider.platform }}</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ rate(provider) }}</span>
            </div>
            <div class="mt-2 flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
              <span>{{ provider.operational_models }}/{{ provider.total_models }}</span>
              <span>{{ provider.available_accounts }}/{{ provider.total_accounts }}</span>
            </div>
          </div>
          <div v-if="providers.length === 0" class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.modelStatus.emptyProviders') }}
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
