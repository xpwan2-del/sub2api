<template>
  <AppLayout>
    <TablePageLayout>
      <!-- Controls -->
      <template #actions>
        <div class="flex w-full items-center justify-between gap-2">
          <div class="flex items-center gap-2">
            <Select
              v-model="selectedSourceId"
              :options="sourceOptions"
              :placeholder="t('upstreamPricing.compare.selectSource')"
              class="w-56"
              @change="loadCompare"
            />
          </div>
          <button @click="loadCompare" :disabled="loading" class="btn btn-secondary" :title="t('common.refresh')">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </template>

      <!-- Compare Table -->
      <template #table>
        <DataTable :columns="columns" :data="rows" :loading="loading">
          <template #cell-upstream_input_price="{ value, row }">
            <div class="text-xs leading-relaxed">
              <div>
                <span class="text-gray-400">in:</span>
                <span class="font-medium text-gray-900 dark:text-white">{{ formatPrice(value) }}</span>
              </div>
              <div>
                <span class="text-gray-400">out:</span>
                <span class="font-medium text-gray-900 dark:text-white">{{ formatPrice(row.upstream_output_price) }}</span>
              </div>
            </div>
          </template>
          <template #cell-local_input_price="{ value, row }">
            <div class="text-xs leading-relaxed">
              <div>
                <span class="text-gray-400">in:</span>
                <span class="font-medium text-gray-900 dark:text-white">{{ formatPrice(value) }}</span>
              </div>
              <div>
                <span class="text-gray-400">out:</span>
                <span class="font-medium text-gray-900 dark:text-white">{{ formatPrice(row.local_output_price) }}</span>
              </div>
            </div>
          </template>
          <template #cell-local_multiplier="{ value }">
            <span class="text-sm font-medium text-gray-900 dark:text-white">× {{ formatMult(value) }}</span>
          </template>
          <template #cell-suggested_price="{ value }">
            <span class="text-sm font-semibold text-blue-600 dark:text-blue-400">{{ formatPrice(value) }}</span>
          </template>
          <template #cell-diff_pct="{ value }">
            <span :class="diffClass(value)" class="text-sm font-medium">{{ formatDelta(value) }}</span>
          </template>
        </DataTable>
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { upstreamPricingAPI } from '@/api/admin/upstreamPricing'
import { extractApiErrorMessage } from '@/utils/apiError'
import type {
  UpstreamPriceSource,
  UpstreamPriceCompareRow
} from '@/types/upstreamPricing'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

// ==================== Data ====================

const loading = ref(false)
const rows = ref<UpstreamPriceCompareRow[]>([])
const sources = ref<UpstreamPriceSource[]>([])
const selectedSourceId = ref<number | null>(null)

const sourceOptions = computed(() =>
  sources.value.map((s) => ({ value: String(s.id), label: s.name }))
)

const columns = computed((): Column[] => [
  { key: 'model_name', label: t('upstreamPricing.compare.model') },
  { key: 'upstream_input_price', label: t('upstreamPricing.compare.upstreamRef') },
  { key: 'local_input_price', label: t('upstreamPricing.compare.localPrice') },
  { key: 'local_multiplier', label: t('upstreamPricing.compare.localMultiplier') },
  { key: 'suggested_price', label: t('upstreamPricing.compare.suggested') },
  { key: 'diff_pct', label: t('upstreamPricing.compare.diffPct') }
])

// ==================== Helpers ====================

function formatPrice(v: number | null | undefined): string {
  if (v === null || v === undefined) return '—'
  return `$${Number(v).toFixed(4)}`
}

function formatMult(v: number | null | undefined): string {
  if (v === null || v === undefined) return '—'
  return Number(v).toFixed(2)
}

function formatDelta(pct: number): string {
  if (!Number.isFinite(pct) || pct === 0) return '0%'
  const sign = pct > 0 ? '+' : ''
  return `${sign}${pct.toFixed(1)}%`
}

function diffClass(pct: number): string {
  const abs = Math.abs(pct)
  if (abs >= 20) return 'text-red-600 dark:text-red-400'
  if (abs >= 5) return 'text-amber-600 dark:text-amber-400'
  return 'text-emerald-600 dark:text-emerald-400'
}

// ==================== Loaders ====================

async function loadSources() {
  try {
    sources.value = await upstreamPricingAPI.listSources()
    if (selectedSourceId.value === null && sources.value.length > 0) {
      selectedSourceId.value = sources.value[0].id
    }
  } catch {
    /* ignore */
  }
}

async function loadCompare() {
  if (!selectedSourceId.value) {
    rows.value = []
    return
  }
  loading.value = true
  try {
    rows.value = await upstreamPricingAPI.comparePrices(selectedSourceId.value)
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await loadSources()
  await loadCompare()
})
</script>
