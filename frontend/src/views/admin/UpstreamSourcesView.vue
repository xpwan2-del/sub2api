<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <!-- Left: Search -->
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <div class="relative w-full sm:w-72">
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
              />
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('admin.upstreamSources.searchPlaceholder', 'Search sources...')"
                class="input pl-10"
              />
            </div>
          </div>

          <!-- Right: Actions -->
          <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
            <button
              @click="loadSources"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh', 'Refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button @click="openCreateDialog" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.upstreamSources.createSource', 'Create Source') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="filteredSources"
          :loading="loading"
          row-key="id"
          :default-sort-key="'name'"
          :default-sort-order="'asc'"
        >
          <template #cell-name="{ row }">
            <div class="flex items-center gap-2">
              <span class="font-medium text-gray-900 dark:text-white">{{ row.name }}</span>
              <span
                v-if="!row.enabled"
                class="inline-flex items-center rounded bg-gray-100 px-1.5 py-0.5 text-xs font-medium text-gray-500 dark:bg-dark-700 dark:text-gray-400"
              >
                {{ t('admin.upstreamSources.disabled', 'Disabled') }}
              </span>
            </div>
          </template>

          <template #cell-base_url="{ value }">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{ value }}</span>
          </template>

          <template #cell-target_channel_id="{ row }">
            <span class="text-sm text-gray-700 dark:text-gray-300">
              {{ channelName(row.target_channel_id) }}
            </span>
          </template>

          <template #cell-pricing_source="{ value }">
            <span
              class="inline-flex items-center rounded bg-indigo-50 px-2 py-0.5 text-xs font-medium text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300"
            >
              {{ value }}
            </span>
          </template>

          <template #cell-last_sync_at="{ value, row }">
            <div class="flex flex-col">
              <span class="text-sm text-gray-600 dark:text-gray-400">
                {{ value ? formatDateTime(value) : t('admin.upstreamSources.neverSynced', 'Never') }}
              </span>
              <span
                v-if="row.last_error"
                class="mt-0.5 max-w-[16rem] truncate text-xs text-red-600 dark:text-red-400"
                :title="row.last_error"
              >
                {{ row.last_error }}
              </span>
            </div>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button
                @click="handleSync(row)"
                :disabled="syncingId === row.id"
                class="btn-icon text-indigo-600 hover:text-indigo-700 dark:text-indigo-400 dark:hover:text-indigo-300"
                :title="t('admin.upstreamSources.syncNow', 'Sync Now')"
              >
                <Icon name="sync" size="md" :class="syncingId === row.id ? 'animate-spin' : ''" />
              </button>
              <button
                @click="openEditDialog(row)"
                class="btn-icon text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white"
                :title="t('common.edit', 'Edit')"
              >
                <Icon name="edit" size="md" />
              </button>
              <button
                @click="handleDelete(row)"
                class="btn-icon text-red-600 hover:text-red-700 dark:text-red-400 dark:hover:text-red-300"
                :title="t('common.delete', 'Delete')"
              >
                <Icon name="trash" size="md" />
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              icon=""
              :title="t('admin.upstreamSources.noSourcesTitle', 'No Upstream Sources')"
              :description="t('admin.upstreamSources.noSourcesDesc', 'Create an upstream new-api source to sync model pricing.')"
              :action-text="t('admin.upstreamSources.createSource', 'Create Source')"
              @action="openCreateDialog"
            />
          </template>
        </DataTable>
      </template>
    </TablePageLayout>

    <!-- Create/Edit Dialog -->
    <BaseDialog
      :show="showDialog"
      :title="editingSource ? t('admin.upstreamSources.editSource', 'Edit Source') : t('admin.upstreamSources.createSource', 'Create Source')"
      width="wide"
      @close="closeDialog"
    >
      <form :id="'upstream-source-form'" @submit.prevent="handleSubmit" class="space-y-4">
        <!-- Name -->
        <div>
          <label class="input-label">{{ t('admin.upstreamSources.fields.name', 'Name') }} <span class="text-red-500">*</span></label>
          <input v-model="form.name" type="text" required class="input" :placeholder="t('admin.upstreamSources.fields.namePlaceholder', 'e.g. Production new-api')" />
        </div>

        <!-- Base URL -->
        <div>
          <label class="input-label">{{ t('admin.upstreamSources.fields.baseUrl', 'Base URL') }} <span class="text-red-500">*</span></label>
          <input v-model="form.base_url" type="url" required class="input" placeholder="https://new-api.example.com" />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.upstreamSources.fields.baseUrlHint', 'Must be in the allowed new-api hosts whitelist.') }}
          </p>
        </div>

        <!-- API Key + Dashboard Token -->
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.upstreamSources.fields.apiKey', 'API Key') }} <span class="text-red-500">*</span></label>
            <input v-model="form.api_key" type="password" required class="input" autocomplete="off" :placeholder="t('admin.upstreamSources.fields.apiKeyPlaceholder', 'sk-...')" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.upstreamSources.fields.dashboardToken', 'Dashboard Token') }}</label>
            <input v-model="form.dashboard_token" type="password" class="input" autocomplete="off" :placeholder="t('admin.upstreamSources.fields.optional', 'Optional')" />
          </div>
        </div>

        <!-- Target Channel + Pricing Source -->
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.upstreamSources.fields.targetChannel', 'Target Channel') }} <span class="text-red-500">*</span></label>
            <Select
              v-model="form.target_channel_id"
              :options="channelOptions"
              :placeholder="t('admin.upstreamSources.fields.selectChannel', 'Select channel')"
              searchable
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.upstreamSources.fields.pricingSource', 'Pricing Source') }}</label>
            <Select v-model="form.pricing_source" :options="pricingSourceOptions" />
          </div>
        </div>

        <!-- Base Price Per 1k -->
        <div>
          <label class="input-label">{{ t('admin.upstreamSources.fields.basePricePer1k', 'Base Price per 1k (USD)') }} <span class="text-red-500">*</span></label>
          <input v-model.number="form.base_price_per_1k" type="number" step="any" min="0" required class="input" placeholder="0.002" />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.upstreamSources.fields.basePriceHint', 'USD price per 1k input tokens used to reverse-convert upstream ratios.') }}
          </p>
        </div>

        <!-- Toggles -->
        <div class="flex flex-wrap items-center gap-6 pt-2">
          <label class="flex cursor-pointer items-center gap-2">
            <Toggle :modelValue="form.enabled" @update:modelValue="form.enabled = $event" />
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.upstreamSources.fields.enabled', 'Enabled') }}</span>
          </label>
          <label class="flex cursor-pointer items-center gap-2">
            <Toggle :modelValue="form.sync_model_price" @update:modelValue="form.sync_model_price = $event" />
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.upstreamSources.fields.syncModelPrice', 'Sync Model Price') }}</span>
          </label>
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end space-x-3">
          <button @click="closeDialog" type="button" class="btn btn-secondary">
            {{ t('common.cancel', 'Cancel') }}
          </button>
          <button type="submit" form="upstream-source-form" :disabled="submitting" class="btn btn-primary">
            {{ submitting ? t('common.submitting', 'Submitting...') : (editingSource ? t('common.update', 'Update') : t('common.create', 'Create')) }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Delete Confirmation -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.upstreamSources.deleteSource', 'Delete Source')"
      :message="t('admin.upstreamSources.deleteConfirm', 'Are you sure you want to delete this upstream source? This action cannot be undone.')"
      :confirm-text="t('common.delete', 'Delete')"
      :cancel-text="t('common.cancel', 'Cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type { UpstreamSourceConfig } from '@/api/admin/upstreamPriceSync'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()
const router = useRouter()

// ── Table columns ──
const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.upstreamSources.columns.name', 'Name'), sortable: true },
  { key: 'base_url', label: t('admin.upstreamSources.columns.baseUrl', 'Base URL'), sortable: true },
  { key: 'target_channel_id', label: t('admin.upstreamSources.columns.targetChannel', 'Target Channel'), sortable: false },
  { key: 'pricing_source', label: t('admin.upstreamSources.columns.pricingSource', 'Pricing Source'), sortable: false },
  { key: 'last_sync_at', label: t('admin.upstreamSources.columns.lastSync', 'Last Sync'), sortable: true },
  { key: 'actions', label: t('admin.upstreamSources.columns.actions', 'Actions'), sortable: false },
])

const pricingSourceOptions = computed(() => [
  { value: 'auto', label: t('admin.upstreamSources.pricingSourceAuto', 'auto') },
  { value: 'ratio_config', label: t('admin.upstreamSources.pricingSourceRatioConfig', 'ratio_config') },
  { value: 'pricing', label: t('admin.upstreamSources.pricingSourcePricing', 'pricing') },
])

// ── State ──
const sources = ref<UpstreamSourceConfig[]>([])
const channels = ref<{ id: number; name: string }[]>([])
const loading = ref(false)
const submitting = ref(false)
const searchQuery = ref('')
const syncingId = ref<number | null>(null)

const showDialog = ref(false)
const editingSource = ref<UpstreamSourceConfig | null>(null)
const showDeleteDialog = ref(false)
const deletingSource = ref<UpstreamSourceConfig | null>(null)

interface SourceForm {
  name: string
  base_url: string
  api_key: string
  dashboard_token: string
  target_channel_id: number | null
  base_price_per_1k: number
  pricing_source: 'auto' | 'ratio_config' | 'pricing'
  enabled: boolean
  sync_model_price: boolean
}

const form = reactive<SourceForm>({
  name: '',
  base_url: '',
  api_key: '',
  dashboard_token: '',
  target_channel_id: null,
  base_price_per_1k: 0,
  pricing_source: 'auto',
  enabled: true,
  sync_model_price: true,
})

// ── Derived ──
const filteredSources = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return sources.value
  return sources.value.filter((s) =>
    [s.name, s.base_url, s.pricing_source].some((v) => String(v ?? '').toLowerCase().includes(q)),
  )
})

const channelOptions = computed(() =>
  channels.value.map((c) => ({ value: c.id, label: c.name })),
)

function channelName(id: number): string {
  return channels.value.find((c) => c.id === id)?.name ?? `#${id}`
}

// ── Helpers ──
function formatDateTime(value: string): string {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

// ── Load ──
async function loadSources() {
  loading.value = true
  try {
    sources.value = await adminAPI.upstreamPriceSync.listSources()
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreamSources.loadError', 'Failed to load sources')))
  } finally {
    loading.value = false
  }
}

async function loadChannels() {
  try {
    const res = await adminAPI.channels.list(1, 1000)
    channels.value = (res.items || []).map((c) => ({ id: c.id, name: c.name }))
  } catch (error) {
    console.error('Failed to load channels for dropdown:', error)
  }
}

// ── Dialog ──
function resetForm() {
  form.name = ''
  form.base_url = ''
  form.api_key = ''
  form.dashboard_token = ''
  form.target_channel_id = channels.value[0]?.id ?? null
  form.base_price_per_1k = 0
  form.pricing_source = 'auto'
  form.enabled = true
  form.sync_model_price = true
}

async function openCreateDialog() {
  editingSource.value = null
  if (channels.value.length === 0) await loadChannels()
  resetForm()
  showDialog.value = true
}

async function openEditDialog(source: UpstreamSourceConfig) {
  editingSource.value = source
  if (channels.value.length === 0) await loadChannels()
  form.name = source.name ?? ''
  form.base_url = source.base_url ?? ''
  form.api_key = source.api_key ?? ''
  form.dashboard_token = source.dashboard_token ?? ''
  form.target_channel_id = source.target_channel_id ?? null
  form.base_price_per_1k = source.base_price_per_1k ?? 0
  form.pricing_source = (source.pricing_source as SourceForm['pricing_source']) || 'auto'
  form.enabled = !!source.enabled
  form.sync_model_price = source.sync_model_price !== false
  showDialog.value = true
}

function closeDialog() {
  showDialog.value = false
  editingSource.value = null
}

async function handleSubmit() {
  if (form.target_channel_id == null) {
    appStore.showError(t('admin.upstreamSources.errors.selectChannel', 'Please select a target channel'))
    return
  }
  const payload: UpstreamSourceConfig = {
    name: form.name.trim(),
    base_url: form.base_url.trim(),
    api_key: form.api_key,
    dashboard_token: form.dashboard_token || '',
    target_channel_id: form.target_channel_id,
    base_price_per_1k: form.base_price_per_1k,
    pricing_source: form.pricing_source,
    enabled: form.enabled,
    sync_model_price: form.sync_model_price,
  }

  submitting.value = true
  try {
    if (editingSource.value && editingSource.value.id != null) {
      await adminAPI.upstreamPriceSync.updateSource(editingSource.value.id, payload)
      appStore.showSuccess(t('admin.upstreamSources.updateSuccess', 'Source updated'))
    } else {
      await adminAPI.upstreamPriceSync.createSource(payload)
      appStore.showSuccess(t('admin.upstreamSources.createSuccess', 'Source created'))
    }
    closeDialog()
    loadSources()
  } catch (error: unknown) {
    appStore.showError(
      extractApiErrorMessage(
        error,
        editingSource.value
          ? t('admin.upstreamSources.updateError', 'Failed to update source')
          : t('admin.upstreamSources.createError', 'Failed to create source'),
      ),
    )
  } finally {
    submitting.value = false
  }
}

// ── Sync ──
async function handleSync(source: UpstreamSourceConfig) {
  if (source.id == null) return
  syncingId.value = source.id
  try {
    const res: any = await adminAPI.upstreamPriceSync.syncNow(source.id)
    const reqId = res?.request_id
    if (reqId && reqId > 0) {
      appStore.showSuccess(t('admin.upstreamSources.syncCreated', 'Sync started — review the new change request'))
      router.push('/admin/price-change-requests')
    } else {
      appStore.showInfo(t('admin.upstreamSources.syncNoChanges', 'Sync completed — no pricing changes detected'))
      loadSources()
    }
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreamSources.syncError', 'Failed to sync upstream')))
  } finally {
    syncingId.value = null
  }
}

// ── Delete ──
function handleDelete(source: UpstreamSourceConfig) {
  deletingSource.value = source
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (!deletingSource.value || deletingSource.value.id == null) return
  try {
    await adminAPI.upstreamPriceSync.deleteSource(deletingSource.value.id)
    appStore.showSuccess(t('admin.upstreamSources.deleteSuccess', 'Source deleted'))
    showDeleteDialog.value = false
    deletingSource.value = null
    loadSources()
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreamSources.deleteError', 'Failed to delete source')))
  }
}

// ── Lifecycle ──
onMounted(() => {
  loadSources()
  loadChannels()
})
</script>
