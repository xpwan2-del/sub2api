<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpsModelStatus, OpsModelStatusItem } from '@/api/admin/ops'
import OpsHealthHistoryBar from './OpsHealthHistoryBar.vue'

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
  <section class="space-y-3">
    <div class="flex items-center justify-between">
      <div>
        <h3 class="text-sm font-bold text-gray-900 dark:text-white">
          {{ t('admin.ops.modelStatus.modelCards') }}
        </h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.modelStatus.modelCardsHint') }}
        </p>
      </div>
      <span class="text-xs text-gray-500 dark:text-gray-400">{{ total }}</span>
    </div>

    <div v-if="loading" class="card p-10 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('admin.ops.loadingText') }}
    </div>

    <div v-else-if="models.length === 0" class="card p-10 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('admin.ops.modelStatus.emptyModels') }}
    </div>

    <div v-else class="grid grid-cols-1 gap-4 md:grid-cols-2 2xl:grid-cols-3">
      <article
        v-for="model in models"
        :key="`${model.platform}:${model.model}`"
        class="card p-4"
      >
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <h4 class="truncate text-sm font-bold text-gray-900 dark:text-white" :title="model.model">
              {{ model.model }}
            </h4>
            <div class="mt-1 flex flex-wrap items-center gap-1.5">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ model.platform }}</span>
              <span
                v-for="flag in model.source_flags"
                :key="flag"
                class="rounded bg-gray-100 px-1.5 py-0.5 text-[11px] text-gray-500 dark:bg-dark-700 dark:text-gray-300"
              >
                {{ t(`admin.ops.modelStatus.source.${flag}`) }}
              </span>
            </div>
          </div>
          <span class="shrink-0 rounded-full px-2 py-0.5 text-xs font-semibold" :class="statusClass(model.status)">
            {{ t(`admin.ops.modelStatus.status.${model.status}`) }}
          </span>
        </div>

        <div class="mt-4">
          <div class="mb-2 flex items-center justify-between text-xs">
            <span class="font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.modelStatus.healthHistory') }}</span>
            <span class="text-gray-500 dark:text-gray-400">48h</span>
          </div>
          <OpsHealthHistoryBar :points="model.history ?? []" />
        </div>

        <div class="mt-4 grid grid-cols-2 gap-3 text-xs">
          <div class="rounded-lg bg-gray-50 p-2 dark:bg-dark-800">
            <div class="text-gray-500 dark:text-gray-400">{{ t('admin.ops.modelStatus.requests') }}</div>
            <div class="mt-0.5 font-bold text-gray-900 dark:text-white">
              {{ model.request_count }}
              <span v-if="model.error_count" class="text-red-500">/ {{ model.error_count }}</span>
            </div>
          </div>
          <div class="rounded-lg bg-gray-50 p-2 dark:bg-dark-800">
            <div class="text-gray-500 dark:text-gray-400">{{ t('admin.ops.modelStatus.successRate') }}</div>
            <div class="mt-0.5 font-bold text-gray-900 dark:text-white">
              {{ model.request_count ? formatPercent(model.success_rate) : '-' }}
            </div>
          </div>
          <div class="rounded-lg bg-gray-50 p-2 dark:bg-dark-800">
            <div class="text-gray-500 dark:text-gray-400">P95</div>
            <div class="mt-0.5 font-bold text-gray-900 dark:text-white">
              {{ formatMs(model.p95_latency_ms) }}
            </div>
          </div>
          <div class="rounded-lg bg-gray-50 p-2 dark:bg-dark-800">
            <div class="text-gray-500 dark:text-gray-400">{{ t('admin.ops.modelStatus.accounts') }}</div>
            <div class="mt-0.5 font-bold text-gray-900 dark:text-white">
              {{ model.available_accounts }} / {{ model.total_accounts }}
            </div>
          </div>
          <div class="col-span-2 rounded-lg bg-gray-50 p-2 dark:bg-dark-800">
            <div class="text-gray-500 dark:text-gray-400">{{ t('admin.ops.modelStatus.lastSeen') }}</div>
            <div class="mt-0.5 font-bold text-gray-900 dark:text-white">
              {{ formatDate(model.last_seen_at || model.last_error_at) }}
            </div>
          </div>
        </div>
      </article>
    </div>

    <div class="flex items-center justify-between gap-3">
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
