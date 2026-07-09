<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpsModelStatus, OpsModelStatusItem } from '@/api/admin/ops'

interface Props {
  models: OpsModelStatusItem[]
  loading?: boolean
  page: number
  pageSize: number
  total: number
}

const props = defineProps<Props>()
const emit = defineEmits<{
  'update:page': [value: number]
}>()

const { t } = useI18n()

const totalPages = computed(() => Math.max(1, Math.ceil((props.total || 0) / props.pageSize)))

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

function formatPercent(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return `${value.toFixed(1)}%`
}

function formatMs(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return value >= 1000 ? `${(value / 1000).toFixed(1)}s` : `${Math.round(value)}ms`
}

function formatDate(value?: string | null): string {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '-'
  return d.toLocaleString()
}

function prevPage() {
  if (props.page > 1) emit('update:page', props.page - 1)
}

function nextPage() {
  if (props.page < totalPages.value) emit('update:page', props.page + 1)
}
</script>

<template>
  <section class="card overflow-hidden">
    <div class="flex items-center justify-between gap-3 border-b border-gray-100 p-4 dark:border-dark-700">
      <h3 class="text-sm font-bold text-gray-900 dark:text-white">
        {{ t('admin.ops.modelStatus.models') }}
      </h3>
      <span class="text-xs text-gray-500 dark:text-gray-400">{{ total }}</span>
    </div>

    <div class="overflow-x-auto">
      <table class="min-w-full divide-y divide-gray-100 dark:divide-dark-700">
        <thead class="bg-gray-50 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:bg-dark-800 dark:text-gray-400">
          <tr>
            <th class="px-4 py-3">{{ t('admin.ops.modelStatus.model') }}</th>
            <th class="px-4 py-3">{{ t('admin.ops.modelStatus.provider') }}</th>
            <th class="px-4 py-3">{{ t('admin.ops.modelStatus.statusLabel') }}</th>
            <th class="px-4 py-3">{{ t('admin.ops.modelStatus.successRate') }}</th>
            <th class="px-4 py-3">{{ t('admin.ops.modelStatus.requests') }}</th>
            <th class="px-4 py-3">{{ t('admin.ops.modelStatus.latency') }}</th>
            <th class="px-4 py-3">{{ t('admin.ops.modelStatus.accounts') }}</th>
            <th class="px-4 py-3">{{ t('admin.ops.modelStatus.lastSeen') }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 bg-white text-sm dark:divide-dark-700 dark:bg-dark-900">
          <tr v-for="model in models" :key="`${model.platform}:${model.model}`" class="hover:bg-gray-50 dark:hover:bg-dark-800">
            <td class="px-4 py-3">
              <div class="max-w-[260px] truncate font-semibold text-gray-900 dark:text-white" :title="model.model">
                {{ model.model }}
              </div>
              <div class="mt-1 flex flex-wrap gap-1">
                <span v-for="flag in model.source_flags" :key="flag" class="rounded bg-gray-100 px-1.5 py-0.5 text-[11px] text-gray-500 dark:bg-dark-700 dark:text-gray-300">
                  {{ t(`admin.ops.modelStatus.source.${flag}`) }}
                </span>
              </div>
            </td>
            <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ model.platform }}</td>
            <td class="px-4 py-3">
              <span class="rounded-full px-2 py-0.5 text-xs font-semibold" :class="statusClass(model.status)">
                {{ t(`admin.ops.modelStatus.status.${model.status}`) }}
              </span>
            </td>
            <td class="px-4 py-3 text-gray-700 dark:text-gray-200">{{ formatPercent(model.success_rate) }}</td>
            <td class="px-4 py-3 text-gray-700 dark:text-gray-200">
              {{ model.request_count }}
              <span v-if="model.error_count" class="text-red-500">/ {{ model.error_count }}</span>
            </td>
            <td class="px-4 py-3 text-gray-700 dark:text-gray-200">{{ formatMs(model.avg_latency_ms) }} / {{ formatMs(model.p95_latency_ms) }}</td>
            <td class="px-4 py-3 text-gray-700 dark:text-gray-200">{{ model.available_accounts }} / {{ model.total_accounts }}</td>
            <td class="px-4 py-3 text-gray-500 dark:text-gray-400">{{ formatDate(model.last_seen_at || model.last_error_at) }}</td>
          </tr>
          <tr v-if="!loading && models.length === 0">
            <td colspan="8" class="px-4 py-10 text-center text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.modelStatus.emptyModels') }}
            </td>
          </tr>
          <tr v-if="loading">
            <td colspan="8" class="px-4 py-10 text-center text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.loadingText') }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="flex items-center justify-between gap-3 border-t border-gray-100 p-4 dark:border-dark-700">
      <button class="btn btn-secondary btn-sm" :disabled="loading || page <= 1" @click="prevPage">
        {{ t('admin.ops.modelStatus.previous') }}
      </button>
      <span class="text-xs text-gray-500 dark:text-gray-400">{{ page }} / {{ totalPages }}</span>
      <button class="btn btn-secondary btn-sm" :disabled="loading || page >= totalPages" @click="nextPage">
        {{ t('admin.ops.modelStatus.next') }}
      </button>
    </div>
  </section>
</template>
