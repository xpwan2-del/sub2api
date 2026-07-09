<script setup lang="ts">
import type { OpsHealthHistoryPoint } from '@/api/admin/ops'

interface Props {
  points: OpsHealthHistoryPoint[]
  rows?: number
}

withDefaults(defineProps<Props>(), {
  rows: 1
})

function statusClass(status: string): string {
  switch (status) {
    case 'operational':
      return 'bg-emerald-500'
    case 'degraded':
      return 'bg-amber-500'
    case 'failed':
      return 'bg-red-500'
    default:
      return 'bg-gray-300 dark:bg-dark-600'
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

function formatTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString()
}

function tooltip(point: OpsHealthHistoryPoint): string {
  return [
    `${formatTime(point.bucket_start)} - ${formatTime(point.bucket_end)}`,
    `status: ${point.status}`,
    `requests: ${point.request_count}`,
    `success: ${point.success_count}`,
    `failed: ${point.error_count}`,
    `success rate: ${formatPercent(point.success_rate)}`,
    `p95: ${formatMs(point.p95_latency_ms)}`
  ].join('\n')
}
</script>

<template>
  <div
    class="grid gap-1"
    :style="{ gridTemplateColumns: `repeat(${rows > 1 ? 24 : 48}, minmax(0, 1fr))` }"
  >
    <span
      v-for="(point, index) in points"
      :key="`${point.bucket_start}-${index}`"
      class="h-3 min-w-0 rounded-sm"
      :class="statusClass(point.status)"
      :title="tooltip(point)"
    />
  </div>
</template>
