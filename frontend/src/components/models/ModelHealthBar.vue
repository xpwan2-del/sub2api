<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PublicModelHealth, PublicModelHealthHistoryPoint } from '@/api/publicModels'

const props = defineProps<{
  health: PublicModelHealth | null
}>()

const { t, te } = useI18n()

const points = computed(() => props.health?.history ?? [])
const statusLabel = computed(() => statusText(props.health?.status || 'unknown'))
const requestCount = computed(() => props.health?.request_count ?? 0)
const successRate = computed(() => formatPercent(props.health?.success_rate))

function statusText(status: string): string {
  const key = `modelCatalog.health.status.${status}`
  return te(key) ? t(key) : t('modelCatalog.health.status.unknown')
}

function statusClass(status: string): string {
  switch (status) {
    case 'operational':
      return 'is-operational'
    case 'degraded':
    case 'rate_limited':
      return 'is-degraded'
    case 'failed':
      return 'is-failed'
    default:
      return 'is-idle'
  }
}

function formatPercent(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return `${value.toFixed(1)}%`
}

function pointTitle(point: PublicModelHealthHistoryPoint, index: number): string {
  return [
    t('modelCatalog.health.bucket', { index: index + 1 }),
    `${t('modelCatalog.health.statusLabel')}: ${statusText(point.status)}`,
    `${t('modelCatalog.health.requests')}: ${point.request_count}`,
    `${t('modelCatalog.health.successRate')}: ${formatPercent(point.success_rate)}`
  ].join('\n')
}
</script>

<template>
  <div class="model-health">
    <div class="model-health-head">
      <span>{{ t('modelCatalog.health.title') }}</span>
      <strong>{{ statusLabel }}</strong>
    </div>

    <div class="model-health-bar" aria-hidden="true">
      <span
        v-for="(point, index) in points"
        :key="index"
        :class="statusClass(point.status)"
        :title="pointTitle(point, index)"
      />
    </div>

    <div class="model-health-meta">
      <span>{{ t('modelCatalog.health.window') }}</span>
      <span>{{ t('modelCatalog.health.requests') }} {{ requestCount }}</span>
      <span>{{ t('modelCatalog.health.successRate') }} {{ successRate }}</span>
    </div>
  </div>
</template>

<style scoped>
.model-health {
  position: relative;
  display: grid;
  gap: 8px;
  border: 1px solid rgba(94, 234, 212, 0.14);
  background: rgba(15, 23, 42, 0.38);
  padding: 10px;
}

.model-health-head,
.model-health-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.model-health-head span,
.model-health-meta {
  color: rgba(203, 213, 225, 0.64);
  font-size: 11px;
  font-weight: 700;
}

.model-health-head strong {
  color: #dffefa;
  font-size: 12px;
  font-weight: 850;
}

.model-health-bar {
  display: grid;
  grid-template-columns: repeat(48, minmax(0, 1fr));
  gap: 3px;
}

.model-health-bar span {
  min-width: 0;
  height: 11px;
  border-radius: 2px;
}

.model-health-bar .is-operational {
  background: #10b981;
}

.model-health-bar .is-degraded {
  background: #f59e0b;
}

.model-health-bar .is-failed {
  background: #ef4444;
}

.model-health-bar .is-idle {
  background: rgba(148, 163, 184, 0.28);
}

.model-health-meta {
  flex-wrap: wrap;
  justify-content: flex-start;
  row-gap: 4px;
}
</style>
