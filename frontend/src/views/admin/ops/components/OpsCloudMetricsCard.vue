<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpsGoogleCloudMetricsResult, OpsModelGatewaySummary } from '@/api/admin/ops'
import OpsHealthHistoryBar from './OpsHealthHistoryBar.vue'

interface Props {
  metrics: OpsGoogleCloudMetricsResult | null
  gateway: OpsModelGatewaySummary | null
}

const props = defineProps<Props>()
const { t } = useI18n()

const statusClass = computed(() => {
  switch (props.metrics?.status) {
    case 'ok':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'partial':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    case 'error':
      return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    default:
      return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  }
})

function fmtPercent(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return `${value.toFixed(1)}%`
}

function fmtRate(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  if (value >= 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MB/s`
  if (value >= 1024) return `${(value / 1024).toFixed(1)} KB/s`
  return `${value.toFixed(0)} B/s`
}

function fmtNumber(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '0.0'
  return value.toFixed(1)
}

function fmtInteger(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return String(Math.round(value))
}

function fmtMs(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return value >= 1000 ? `${(value / 1000).toFixed(1)}s` : `${Math.round(value)}ms`
}

function fmtHealth(value?: boolean | null): string {
  if (typeof value !== 'boolean') return '-'
  return value ? t('admin.ops.modelStatus.online') : t('admin.ops.modelStatus.offline')
}

function healthClass(value?: boolean | null): string {
  if (typeof value !== 'boolean') return 'text-gray-500 dark:text-gray-400'
  return value ? 'text-emerald-600 dark:text-emerald-300' : 'text-red-600 dark:text-red-300'
}

function routeStatusClass(status?: string): string {
  switch (status) {
    case 'operational':
      return 'bg-emerald-500'
    case 'degraded':
      return 'bg-amber-500'
    case 'failed':
      return 'bg-red-500'
    default:
      return 'bg-gray-400'
  }
}
</script>

<template>
  <section class="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
    <article class="card p-4 md:p-5">
      <div class="mb-4 flex items-center justify-between gap-3">
        <h3 class="text-sm font-bold text-gray-900 dark:text-white">
          {{ t('admin.ops.modelStatus.cloudTitle') }}
        </h3>
        <span class="rounded-full px-2 py-0.5 text-xs font-semibold" :class="statusClass">
          {{ t(`admin.ops.modelStatus.cloudStatus.${metrics?.status || 'disabled'}`) }}
        </span>
      </div>

      <div class="grid grid-cols-2 gap-3">
        <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.modelStatus.cpu') }}</div>
          <div class="mt-1 text-xl font-bold text-gray-900 dark:text-white">{{ fmtPercent(metrics?.metrics.cpu_percent) }}</div>
        </div>
        <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.modelStatus.memory') }}</div>
          <div class="mt-1 text-xl font-bold text-gray-900 dark:text-white">{{ fmtPercent(metrics?.metrics.memory_percent) }}</div>
        </div>
        <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
          <div class="text-xs text-gray-500 dark:text-gray-400">
            {{ metrics?.source === 'local_system_metrics' ? t('admin.ops.modelStatus.db') : t('admin.ops.modelStatus.disk') }}
          </div>
          <div v-if="metrics?.source === 'local_system_metrics'" class="mt-1 text-xl font-bold" :class="healthClass(metrics?.metrics.db_ok)">
            {{ fmtHealth(metrics?.metrics.db_ok) }}
          </div>
          <div v-else class="mt-1 text-xl font-bold text-gray-900 dark:text-white">{{ fmtPercent(metrics?.metrics.disk_percent) }}</div>
          <div v-if="metrics?.source === 'local_system_metrics'" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.modelStatus.db') }} {{ fmtInteger(metrics?.metrics.db_conn_active) }} / {{ fmtInteger(metrics?.metrics.db_conn_idle) }}
          </div>
        </div>
        <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
          <div class="text-xs text-gray-500 dark:text-gray-400">
            {{ metrics?.source === 'local_system_metrics' ? 'Redis' : t('admin.ops.modelStatus.network') }}
          </div>
          <div v-if="metrics?.source === 'local_system_metrics'" class="mt-1 text-xl font-bold" :class="healthClass(metrics?.metrics.redis_ok)">
            {{ fmtHealth(metrics?.metrics.redis_ok) }}
          </div>
          <div v-else class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">
            {{ fmtRate(metrics?.metrics.network_rx_bytes_sec) }} / {{ fmtRate(metrics?.metrics.network_tx_bytes_sec) }}
          </div>
          <div v-if="metrics?.source === 'local_system_metrics'" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            Redis {{ fmtInteger(metrics?.metrics.redis_conn_total) }} / {{ fmtInteger(metrics?.metrics.redis_conn_idle) }}
          </div>
        </div>
      </div>

      <div v-if="metrics?.source === 'local_system_metrics'" class="mt-3 flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
        <span>{{ t('admin.ops.modelStatus.localMetrics') }}</span>
        <span>Goroutines {{ fmtInteger(metrics?.metrics.goroutine_count) }}</span>
      </div>

      <p v-if="metrics?.error" class="mt-3 text-xs text-amber-600 dark:text-amber-300">
        {{ metrics.error }}
      </p>
    </article>

    <article class="card p-4 md:p-5">
      <div class="mb-4 flex items-center justify-between gap-3">
        <h3 class="text-sm font-bold text-gray-900 dark:text-white">
          {{ t('admin.ops.modelStatus.gatewayTitle') }}
        </h3>
        <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.modelStatus.healthHistory') }} · 48h</span>
      </div>

      <OpsHealthHistoryBar :points="gateway?.history ?? []" />

      <div class="mt-4 grid grid-cols-2 gap-3 md:grid-cols-4">
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.modelStatus.requests') }}</div>
          <div class="text-lg font-bold text-gray-900 dark:text-white">{{ gateway?.request_count_total ?? 0 }}</div>
        </div>
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.modelStatus.sla') }}</div>
          <div class="text-lg font-bold text-gray-900 dark:text-white">{{ (gateway?.sla ?? 0).toFixed(2) }}%</div>
        </div>
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">QPS</div>
          <div class="text-lg font-bold text-gray-900 dark:text-white">{{ fmtNumber(gateway?.qps?.current) }}</div>
        </div>
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">TPS</div>
          <div class="text-lg font-bold text-gray-900 dark:text-white">{{ fmtNumber(gateway?.tps?.current) }}</div>
        </div>
      </div>

      <div class="mt-5 space-y-2">
        <div class="flex items-center justify-between text-xs">
          <span class="font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.modelStatus.routes') }}</span>
          <span class="text-gray-500 dark:text-gray-400">{{ gateway?.routes?.length ?? 0 }}</span>
        </div>
        <div class="max-h-64 space-y-2 overflow-auto pr-1">
          <div
            v-for="route in gateway?.routes ?? []"
            :key="route.endpoint"
            class="flex items-center justify-between gap-3 rounded-lg bg-gray-50 p-2 text-xs dark:bg-dark-800"
          >
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <span class="h-2 w-2 rounded-full" :class="routeStatusClass(route.status)" />
                <span class="truncate font-semibold text-gray-900 dark:text-white" :title="route.endpoint">{{ route.endpoint }}</span>
              </div>
              <div class="mt-1 text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.modelStatus.requests') }} {{ route.request_count }}
                <span v-if="route.error_count"> · {{ t('admin.ops.modelStatus.errors') }} {{ route.error_count }}</span>
              </div>
            </div>
            <div class="shrink-0 text-right text-gray-700 dark:text-gray-200">
              <div class="font-bold">{{ fmtPercent(route.success_rate) }}</div>
              <div class="text-gray-500 dark:text-gray-400">P95 {{ fmtMs(route.p95_latency_ms) }}</div>
            </div>
          </div>
          <div v-if="!gateway?.routes?.length" class="rounded-lg bg-gray-50 p-3 text-xs text-gray-500 dark:bg-dark-800 dark:text-gray-400">
            {{ t('admin.ops.modelStatus.emptyRoutes') }}
          </div>
        </div>
      </div>
    </article>
  </section>
</template>
