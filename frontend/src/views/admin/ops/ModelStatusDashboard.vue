<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import {
  opsAPI,
  type OpsModelStatus,
  type OpsModelStatusSnapshotResponse,
  type OpsModelStatusSnapshotParams
} from '@/api/admin/ops'
import OpsCloudMetricsCard from './components/OpsCloudMetricsCard.vue'
import OpsModelStatusSummary from './components/OpsModelStatusSummary.vue'
import OpsProviderStatusCards from './components/OpsProviderStatusCards.vue'
import OpsModelStatusCards from './components/OpsModelStatusCards.vue'
import OpsModelStatusTable from './components/OpsModelStatusTable.vue'

type TimeRange = NonNullable<OpsModelStatusSnapshotParams['time_range']>
type ViewMode = 'cards' | 'table'

const { t } = useI18n()

const loading = ref(false)
const refreshing = ref(false)
const errorMessage = ref('')
const snapshot = ref<OpsModelStatusSnapshotResponse | null>(null)
const timeRange = ref<TimeRange>('1h')
const provider = ref('')
const status = ref<OpsModelStatus | ''>('')
const query = ref('')
const page = ref(1)
const pageSize = ref(50)
const viewMode = ref<ViewMode>('cards')
const lastUpdated = ref<Date | null>(null)

let abortController: AbortController | null = null
let refreshTimer: number | undefined

const providerOptions = computed(() => {
  const providers = new Set(snapshot.value?.providers?.map((item) => item.platform) ?? [])
  return Array.from(providers).sort()
})

const pagination = computed(() => snapshot.value?.pagination ?? { page: page.value, page_size: pageSize.value, total: 0 })

function buildParams(): OpsModelStatusSnapshotParams {
  return {
    time_range: timeRange.value,
    provider: provider.value || undefined,
    status: status.value || undefined,
    q: query.value || undefined,
    page: page.value,
    page_size: pageSize.value
  }
}

async function loadSnapshot(options: { silent?: boolean } = {}) {
  if (options.silent && loading.value) return
  abortController?.abort()
  abortController = new AbortController()
  if (options.silent) {
    refreshing.value = true
  } else {
    loading.value = true
    errorMessage.value = ''
  }
  try {
    snapshot.value = await opsAPI.getModelStatusSnapshot(buildParams(), { signal: abortController.signal })
    lastUpdated.value = new Date()
  } catch (err: any) {
    if (err?.name === 'AbortError' || err?.code === 'ERR_CANCELED') return
    console.error('[ModelStatusDashboard] Failed to load snapshot', err)
    if (!options.silent) {
      errorMessage.value = err?.message || t('admin.ops.modelStatus.failedToLoad')
    }
  } finally {
    if (options.silent) {
      refreshing.value = false
    } else {
      loading.value = false
    }
  }
}

watch([timeRange, provider, status, query, pageSize], () => {
  if (page.value !== 1) {
    page.value = 1
    return
  }
  void loadSnapshot()
})

watch(page, () => {
  void loadSnapshot()
})

onMounted(() => {
  void loadSnapshot()
  refreshTimer = window.setInterval(() => {
    void loadSnapshot({ silent: true })
  }, 15000)
})

onUnmounted(() => {
  abortController?.abort()
  if (refreshTimer) window.clearInterval(refreshTimer)
})
</script>

<template>
  <AppLayout>
    <div class="space-y-6 pb-12">
      <div class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
        <div>
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('admin.ops.modelStatus.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.modelStatus.description') }}
          </p>
        </div>
        <div class="flex items-center gap-2">
          <span v-if="refreshing" class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.modelStatus.refreshing') }}
          </span>
          <span v-else-if="lastUpdated" class="text-xs text-gray-500 dark:text-gray-400">
            {{ lastUpdated.toLocaleTimeString() }}
          </span>
          <button class="btn btn-primary btn-sm" :disabled="loading || refreshing" @click="loadSnapshot()">
            {{ t('admin.ops.modelStatus.refresh') }}
          </button>
        </div>
      </div>

      <div class="card p-4">
        <div class="grid grid-cols-1 gap-3 md:grid-cols-5">
          <label class="block">
            <span class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.ops.modelStatus.timeRange') }}</span>
            <select v-model="timeRange" class="input">
              <option value="5m">{{ t('admin.ops.timeRange.5m') }}</option>
              <option value="30m">{{ t('admin.ops.timeRange.30m') }}</option>
              <option value="1h">{{ t('admin.ops.timeRange.1h') }}</option>
              <option value="6h">{{ t('admin.ops.timeRange.6h') }}</option>
              <option value="24h">{{ t('admin.ops.timeRange.24h') }}</option>
            </select>
          </label>
          <label class="block">
            <span class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.ops.modelStatus.provider') }}</span>
            <select v-model="provider" class="input">
              <option value="">{{ t('common.all') }}</option>
              <option v-for="item in providerOptions" :key="item" :value="item">{{ item }}</option>
            </select>
          </label>
          <label class="block">
            <span class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.ops.modelStatus.statusLabel') }}</span>
            <select v-model="status" class="input">
              <option value="">{{ t('common.all') }}</option>
              <option value="operational">{{ t('admin.ops.modelStatus.status.operational') }}</option>
              <option value="degraded">{{ t('admin.ops.modelStatus.status.degraded') }}</option>
              <option value="rate_limited">{{ t('admin.ops.modelStatus.status.rate_limited') }}</option>
              <option value="failed">{{ t('admin.ops.modelStatus.status.failed') }}</option>
              <option value="no_recent_traffic">{{ t('admin.ops.modelStatus.status.no_recent_traffic') }}</option>
              <option value="unknown">{{ t('admin.ops.modelStatus.status.unknown') }}</option>
            </select>
          </label>
          <label class="block md:col-span-2">
            <span class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.ops.modelStatus.search') }}</span>
            <input v-model.trim="query" class="input" type="search" :placeholder="t('admin.ops.modelStatus.searchPlaceholder')" />
          </label>
        </div>
      </div>

      <div v-if="errorMessage" class="rounded-lg bg-red-50 p-4 text-sm text-red-600 dark:bg-red-900/20 dark:text-red-300">
        {{ errorMessage }}
      </div>

      <OpsCloudMetricsCard :metrics="snapshot?.cloud_metrics ?? null" :gateway="snapshot?.gateway_summary ?? null" />
      <OpsModelStatusSummary
        :summary="snapshot?.model_summary ?? null"
        :providers="snapshot?.providers ?? []"
        :availability="snapshot?.account_availability ?? null"
      />

      <OpsProviderStatusCards :providers="snapshot?.providers ?? []" />

      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 class="text-base font-bold text-gray-900 dark:text-white">
            {{ t('admin.ops.modelStatus.models') }}
          </h2>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.modelStatus.viewModeHint') }}
          </p>
        </div>
        <div class="inline-flex rounded-lg border border-gray-200 bg-white p-1 dark:border-dark-700 dark:bg-dark-900">
          <button
            class="rounded-md px-3 py-1.5 text-sm font-medium transition"
            :class="viewMode === 'cards'
              ? 'bg-primary-600 text-white shadow-sm'
              : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-800'"
            @click="viewMode = 'cards'"
          >
            {{ t('admin.ops.modelStatus.cardView') }}
          </button>
          <button
            class="rounded-md px-3 py-1.5 text-sm font-medium transition"
            :class="viewMode === 'table'
              ? 'bg-primary-600 text-white shadow-sm'
              : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-800'"
            @click="viewMode = 'table'"
          >
            {{ t('admin.ops.modelStatus.tableView') }}
          </button>
        </div>
      </div>

      <OpsModelStatusCards
        v-if="viewMode === 'cards'"
        v-model:page="page"
        :models="snapshot?.models ?? []"
        :loading="loading"
        :page-size="pagination.page_size"
        :total="pagination.total"
      />
      <OpsModelStatusTable
        v-else
        v-model:page="page"
        :models="snapshot?.models ?? []"
        :loading="loading"
        :page-size="pagination.page_size"
        :total="pagination.total"
      />
    </div>
  </AppLayout>
</template>
