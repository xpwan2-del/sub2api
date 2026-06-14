<template>
  <AppLayout>
    <TablePageLayout>
      <!-- Filters + Actions -->
      <template #actions>
        <div class="flex w-full flex-wrap items-center justify-between gap-2">
          <div class="flex flex-wrap items-center gap-2">
            <Select
              v-model="filterSourceId"
              :options="sourceFilterOptions"
              :placeholder="t('upstreamPricing.changes.allSources')"
              class="w-48"
              @change="loadChanges"
            />
            <Select
              v-model="filterStatus"
              :options="statusFilterOptions"
              class="w-36"
              @change="loadChanges"
            />
          </div>
          <button @click="loadChanges" :disabled="loading" class="btn btn-secondary" :title="t('common.refresh')">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </template>

      <!-- Changes Table -->
      <template #table>
        <DataTable :columns="columns" :data="changes" :loading="loading">
          <template #cell-source_id="{ value }">
            <span class="text-sm">{{ sourceLabel(value) }}</span>
          </template>
          <template #cell-prices="{ row }">
            <div class="text-xs leading-relaxed">
              <div>
                <span class="text-gray-400">in:</span>
                <span class="font-medium text-gray-900 dark:text-white">{{ formatPrice(row.prev_input_price) }}</span>
                <span class="mx-1 text-gray-400">→</span>
                <span class="font-medium text-gray-900 dark:text-white">{{ formatPrice(row.curr_input_price) }}</span>
                <span :class="deltaClass(row.input_delta_pct)" class="ml-1 font-medium">{{ formatDelta(row.input_delta_pct) }}</span>
              </div>
              <div>
                <span class="text-gray-400">out:</span>
                <span class="font-medium text-gray-900 dark:text-white">{{ formatPrice(row.prev_output_price) }}</span>
                <span class="mx-1 text-gray-400">→</span>
                <span class="font-medium text-gray-900 dark:text-white">{{ formatPrice(row.curr_output_price) }}</span>
                <span :class="deltaClass(row.output_delta_pct)" class="ml-1 font-medium">{{ formatDelta(row.output_delta_pct) }}</span>
              </div>
            </div>
          </template>
          <template #cell-suggested_input_price="{ value, row }">
            <div class="text-sm">
              <div class="font-medium text-gray-900 dark:text-white">{{ formatPrice(value) }}</div>
              <div v-if="row.suggested_multiplier" class="text-[11px] text-gray-400 dark:text-gray-500">
                × {{ row.suggested_multiplier.toFixed(2) }}
              </div>
            </div>
          </template>
          <template #cell-status="{ value }">
            <span :class="statusBadgeClass(value)">{{ statusLabel(value) }}</span>
          </template>
          <template #cell-detected_at="{ value }">
            <span class="text-xs text-gray-500 dark:text-gray-400">{{ formatRelativeTime(value) }}</span>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1.5">
              <button
                v-if="row.status === 'pending'"
                @click="openApply(row, 'follow_cost')"
                :disabled="busyId === row.id"
                class="rounded-md bg-emerald-50 px-2 py-1 text-xs font-medium text-emerald-700 transition-colors hover:bg-emerald-100 disabled:opacity-50 dark:bg-emerald-900/20 dark:text-emerald-300 dark:hover:bg-emerald-900/40"
                :title="t('upstreamPricing.changes.followCostHint')"
              >
                {{ t('upstreamPricing.changes.followCost') }}
              </button>
              <button
                v-if="row.status === 'pending'"
                @click="openApply(row, 'lock_price')"
                :disabled="busyId === row.id"
                class="rounded-md bg-blue-50 px-2 py-1 text-xs font-medium text-blue-700 transition-colors hover:bg-blue-100 disabled:opacity-50 dark:bg-blue-900/20 dark:text-blue-300 dark:hover:bg-blue-900/40"
                :title="t('upstreamPricing.changes.lockPriceHint')"
              >
                {{ t('upstreamPricing.changes.lockPrice') }}
              </button>
              <button
                v-if="row.status === 'pending'"
                @click="handleDismiss(row)"
                :disabled="busyId === row.id"
                class="rounded-md bg-gray-50 px-2 py-1 text-xs font-medium text-gray-600 transition-colors hover:bg-gray-100 disabled:opacity-50 dark:bg-gray-700/40 dark:text-gray-300 dark:hover:bg-gray-700"
              >
                {{ t('upstreamPricing.changes.dismiss') }}
              </button>
              <span v-else class="text-xs text-gray-400">—</span>
            </div>
          </template>
        </DataTable>
      </template>
    </TablePageLayout>

    <!-- Apply Dialog -->
    <BaseDialog
      :show="showApplyDialog"
      :title="applyTitle"
      width="narrow"
      @close="showApplyDialog = false"
    >
      <div v-if="applyTarget" class="space-y-4">
        <!-- Change summary -->
        <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm dark:border-dark-600 dark:bg-dark-800">
          <div class="flex items-center justify-between">
            <span class="text-gray-500 dark:text-gray-400">{{ t('upstreamPricing.changes.model') }}</span>
            <span class="font-medium text-gray-900 dark:text-white">{{ applyTarget.model_name }}</span>
          </div>
          <div class="mt-1 flex items-center justify-between">
            <span class="text-gray-500 dark:text-gray-400">{{ t('upstreamPricing.changes.suggestedPrice') }}</span>
            <span class="font-medium text-gray-900 dark:text-white">{{ formatPrice(applyTarget.suggested_input_price) }}</span>
          </div>
          <div class="mt-1 flex items-center justify-between">
            <span class="text-gray-500 dark:text-gray-400">{{ t('upstreamPricing.changes.mode') }}</span>
            <span class="font-medium" :class="applyMode === 'lock_price' ? 'text-blue-600 dark:text-blue-400' : 'text-emerald-600 dark:text-emerald-400'">
              {{ applyMode === 'lock_price' ? t('upstreamPricing.changes.lockPrice') : t('upstreamPricing.changes.followCost') }}
            </span>
          </div>
        </div>

        <div>
          <label class="input-label">
            {{ applyMode === 'lock_price' ? t('upstreamPricing.changes.targetGroup') : t('upstreamPricing.changes.targetChannel') }}
          </label>
          <Select
            v-model="targetIdInput"
            :options="targetOptions"
            :placeholder="targetPlaceholder"
            :empty-text="t('upstreamPricing.changes.noTargets')"
            searchable
            class="w-full"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ applyMode === 'lock_price' ? t('upstreamPricing.changes.targetGroupHint') : t('upstreamPricing.changes.targetChannelHint') }}
          </p>
        </div>
      </div>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" @click="showApplyDialog = false" class="btn btn-secondary">{{ t('common.cancel') }}</button>
          <button type="button" @click="confirmApply" :disabled="applying" class="btn btn-primary">
            {{ applying ? t('common.saving') : t('common.confirm') }}
          </button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { upstreamPricingAPI } from '@/api/admin/upstreamPricing'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatRelativeTime } from '@/utils/format'
import type {
  UpstreamPriceChange,
  UpstreamPriceSource,
  UpstreamPriceApplyMode,
  ApplyTargetsResponse
} from '@/types/upstreamPricing'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

// ==================== Data ====================

const loading = ref(false)
const applying = ref(false)
const busyId = ref<number | null>(null)
const changes = ref<UpstreamPriceChange[]>([])
const sources = ref<UpstreamPriceSource[]>([])

// Filters
const filterSourceId = ref<number | null>(null)
const filterStatus = ref<string>('pending')

const sourceFilterOptions = computed(() => [
  { value: null as unknown as string, label: t('upstreamPricing.changes.allSources') },
  ...sources.value.map((s) => ({ value: String(s.id), label: s.name }))
])

const statusFilterOptions = computed(() => [
  { value: 'pending', label: t('upstreamPricing.changes.statusPending') },
  { value: 'applied', label: t('upstreamPricing.changes.statusApplied') },
  { value: 'dismissed', label: t('upstreamPricing.changes.statusDismissed') },
  { value: 'failed', label: t('upstreamPricing.changes.statusFailed') }
])

const columns = computed((): Column[] => [
  { key: 'id', label: 'ID' },
  { key: 'source_id', label: t('upstreamPricing.changes.source') },
  { key: 'local_model_name', label: t('upstreamPricing.changes.model') },
  { key: 'prices', label: t('upstreamPricing.changes.priceChange') },
  { key: 'suggested_input_price', label: t('upstreamPricing.changes.suggestedPrice') },
  { key: 'status', label: t('upstreamPricing.changes.status') },
  { key: 'detected_at', label: t('upstreamPricing.changes.detectedAt') },
  { key: 'actions', label: t('common.actions') }
])

// ==================== Apply Dialog ====================

const showApplyDialog = ref(false)
const applyTarget = ref<UpstreamPriceChange | null>(null)
const applyMode = ref<UpstreamPriceApplyMode>('follow_cost')
const targetIdInput = ref<number | null>(null)
const applyTargets = ref<ApplyTargetsResponse>({ channels: [], groups: [] })
const loadingTargets = ref(false)

const applyTitle = computed(() =>
  applyMode.value === 'lock_price'
    ? t('upstreamPricing.changes.lockPrice')
    : t('upstreamPricing.changes.followCost')
)

// 下拉选项：follow_cost → channels；lock_price → groups
const targetOptions = computed(() => {
  if (applyMode.value === 'lock_price') {
    return applyTargets.value.groups.map((g) => ({
      value: g.id,
      label: `${g.name} (× ${Number(g.rate_multiplier).toFixed(2)})`
    }))
  }
  return applyTargets.value.channels.map((c) => ({
    value: c.id,
    label: c.name
  }))
})

const targetPlaceholder = computed(() => {
  const list = applyMode.value === 'lock_price' ? applyTargets.value.groups : applyTargets.value.channels
  if (loadingTargets.value) return t('common.loading')
  if (list.length === 0) return t('upstreamPricing.changes.noTargets')
  return applyMode.value === 'lock_price'
    ? t('upstreamPricing.changes.targetGroupPlaceholder')
    : t('upstreamPricing.changes.targetChannelPlaceholder')
})

// ==================== Helpers ====================

function sourceLabel(sourceId: number): string {
  const s = sources.value.find((x) => x.id === sourceId)
  return s ? s.name : `#${sourceId}`
}

function formatPrice(v: number | null | undefined): string {
  if (v === null || v === undefined) return '—'
  return `$${Number(v).toFixed(4)}`
}

function formatDelta(pct: number): string {
  if (!Number.isFinite(pct) || pct === 0) return '0%'
  const sign = pct > 0 ? '+' : ''
  return `${sign}${pct.toFixed(1)}%`
}

function deltaClass(pct: number): string {
  if (pct > 0) return 'text-red-600 dark:text-red-400'
  if (pct < 0) return 'text-emerald-600 dark:text-emerald-400'
  return 'text-gray-400'
}

function statusBadgeClass(status: string): string {
  const base = 'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium'
  switch (status) {
    case 'pending':
      return `${base} bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300`
    case 'applied':
      return `${base} bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300`
    case 'dismissed':
      return `${base} bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400`
    case 'failed':
      return `${base} bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300`
    default:
      return `${base} bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300`
  }
}

function statusLabel(status: string): string {
  switch (status) {
    case 'pending': return t('upstreamPricing.changes.statusPending')
    case 'applied': return t('upstreamPricing.changes.statusApplied')
    case 'dismissed': return t('upstreamPricing.changes.statusDismissed')
    case 'failed': return t('upstreamPricing.changes.statusFailed')
    default: return status
  }
}

// ==================== Loaders ====================

async function loadSources() {
  try {
    sources.value = await upstreamPricingAPI.listSources()
  } catch {
    /* ignore; filter just won't have labels */
  }
}

async function loadChanges() {
  loading.value = true
  try {
    changes.value = await upstreamPricingAPI.listChanges({
      source_id: filterSourceId.value ?? undefined,
      status: filterStatus.value || undefined
    })
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

// ==================== Actions ====================

function openApply(row: UpstreamPriceChange, mode: UpstreamPriceApplyMode) {
  applyTarget.value = row
  applyMode.value = mode
  targetIdInput.value = null
  applyTargets.value = { channels: [], groups: [] }
  showApplyDialog.value = true
  loadApplyTargets(row.id)
}

async function loadApplyTargets(changeId: number) {
  loadingTargets.value = true
  try {
    applyTargets.value = await upstreamPricingAPI.getApplyTargets(changeId)
  } catch {
    // 失败时保留下拉（空列表），不阻塞 apply 流程
    applyTargets.value = { channels: [], groups: [] }
  } finally {
    loadingTargets.value = false
  }
}

async function confirmApply() {
  if (!applyTarget.value) return
  applying.value = true
  busyId.value = applyTarget.value.id
  try {
    await upstreamPricingAPI.applyChange(applyTarget.value.id, {
      mode: applyMode.value,
      target_id: targetIdInput.value ?? undefined
    })
    appStore.showSuccess(t('upstreamPricing.changes.applied'))
    showApplyDialog.value = false
    loadChanges()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    applying.value = false
    busyId.value = null
  }
}

async function handleDismiss(row: UpstreamPriceChange) {
  if (!window.confirm(t('upstreamPricing.changes.dismissConfirm'))) return
  busyId.value = row.id
  try {
    await upstreamPricingAPI.dismissChange(row.id)
    appStore.showSuccess(t('upstreamPricing.changes.dismissed'))
    loadChanges()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    busyId.value = null
  }
}

onMounted(() => {
  loadSources()
  loadChanges()
})
</script>
