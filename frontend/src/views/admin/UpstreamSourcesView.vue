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
              :disabled="loading || batchRefreshingBalances"
              class="btn btn-secondary"
              :title="t('common.refresh', 'Refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button
              @click="handleBatchRefreshBalances"
              :disabled="loading || batchRefreshingBalances || sources.length === 0"
              class="btn btn-secondary"
            >
              <Icon name="refresh" size="md" class="mr-2" :class="batchRefreshingBalances ? 'animate-spin' : ''" />
              {{ batchRefreshingBalances
                ? t('admin.upstreamSources.refreshingBalances', 'Refreshing...')
                : t('admin.upstreamSources.refreshAllBalances', 'Refresh All Balances') }}
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
            <a
              v-if="value"
              :href="ensureProtocol(value)"
              target="_blank"
              rel="noopener noreferrer"
              class="inline-flex max-w-[16rem] items-center gap-1 text-sm font-medium text-indigo-600 hover:text-indigo-700 hover:underline dark:text-indigo-400 dark:hover:text-indigo-300"
            >
              <span class="truncate">{{ value }}</span>
              <Icon name="externalLink" size="xs" class="flex-shrink-0 opacity-70" />
            </a>
            <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
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

          <template #cell-proxy_id="{ row }">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{ proxyName(row.proxy_id) }}</span>
          </template>

          <template #cell-balance="{ row }">
            <div class="flex flex-col items-start gap-1">
              <div class="flex items-center gap-2">
                <span class="font-medium text-gray-900 dark:text-white">{{ formatBalance(row.last_balance_usd) }}</span>
                <span :class="balanceStatusClass(row.balance_status)" class="inline-flex rounded px-2 py-0.5 text-xs font-medium">
                  {{ balanceStatusLabel(row.balance_status) }}
                </span>
              </div>
              <span v-if="row.last_balance_checked_at" class="text-xs text-gray-500 dark:text-gray-400">
                {{ formatDateTime(row.last_balance_checked_at) }}
              </span>
              <span v-if="row.last_balance_error" class="max-w-[15rem] truncate text-xs text-red-600 dark:text-red-400" :title="row.last_balance_error">
                {{ row.last_balance_error }}
              </span>
            </div>
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
                class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-indigo-600 hover:bg-indigo-50 disabled:cursor-not-allowed disabled:opacity-40 dark:text-indigo-400 dark:hover:bg-indigo-900/20"
                :title="t('admin.upstreamSources.syncNow', 'Sync Now')"
              >
                <Icon name="sync" size="sm" :class="syncingId === row.id ? 'animate-spin' : ''" />
                {{ t('admin.upstreamSources.syncNow', 'Sync Now') }}
              </button>
              <button
                @click="handleRefreshBalance(row)"
                :disabled="refreshingBalanceId === row.id || !row.dashboard_token"
                class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-emerald-600 hover:bg-emerald-50 disabled:cursor-not-allowed disabled:opacity-40 dark:text-emerald-400 dark:hover:bg-emerald-900/20"
                :title="t('admin.upstreamSources.refreshBalance', 'Refresh Balance')"
              >
                <Icon name="refresh" size="sm" :class="refreshingBalanceId === row.id ? 'animate-spin' : ''" />
                {{ t('admin.upstreamSources.refreshBalance', 'Refresh Balance') }}
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

        <!-- Dashboard Auth Mode + User ID -->
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.upstreamSources.fields.dashboardAuthMode', 'Dashboard Auth Mode') }}</label>
            <Select v-model="form.dashboard_auth_mode" :options="dashboardAuthModeOptions" />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.upstreamSources.fields.dashboardAuthModeHint', 'How to authenticate when querying balance. Use auto for most cases.') }}
            </p>
          </div>
          <div>
            <label class="input-label">{{ t('admin.upstreamSources.fields.dashboardUserId', 'Dashboard User ID') }}</label>
            <input
              v-model.number="form.dashboard_user_id"
              type="number"
              min="1"
              class="input"
              :placeholder="t('admin.upstreamSources.fields.optional', 'Optional')"
              :disabled="!needsDashboardUserId"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.upstreamSources.fields.dashboardUserIdHint', 'Required only for raw_user/bearer_user modes (older new-api versions).') }}
            </p>
          </div>
        </div>

        <!-- Target Channel + Proxy -->
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
            <label class="input-label">{{ t('admin.upstreamSources.fields.proxy', 'Proxy') }}</label>
            <ProxySelector v-model="form.proxy_id" :proxies="proxies" />
          </div>
        </div>

        <!-- Upstream Group Filter -->
        <div>
          <label class="input-label">{{ t('admin.upstreamSources.fields.upstreamGroup', 'Upstream Group Filter') }}</label>
          <div class="flex gap-2">
            <Select
              v-model="form.target_upstream_group"
              :options="upstreamGroupOptions"
              :disabled="loadingGroups"
              searchable
              class="flex-1"
            />
            <button
              type="button"
              @click="loadGroupsFromForm"
              :disabled="!form.base_url || loadingGroups"
              class="btn btn-secondary whitespace-nowrap"
              :title="t('admin.upstreamSources.fields.loadGroups', 'Load Groups')"
            >
              <Icon name="refresh" size="sm" class="mr-1" :class="loadingGroups ? 'animate-spin' : ''" />
              {{ t('admin.upstreamSources.fields.loadGroups', 'Load Groups') }}
            </button>
          </div>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.upstreamSources.fields.upstreamGroupHint', 'Fill Base URL then click Load Groups. Only sync models enabled for the selected group; empty = sync all models.') }}
          </p>
        </div>

        <!-- Group Ratio Sync -->
        <div class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-700">
          <label class="flex cursor-pointer items-center gap-2">
            <Toggle :modelValue="form.sync_group_ratio" @update:modelValue="form.sync_group_ratio = $event" />
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.upstreamSources.fields.syncGroupRatio', 'Sync selected upstream group ratio') }}
            </span>
          </label>
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.upstreamSources.fields.syncGroupRatioHint', 'The first sync only records a baseline. Later changes proportionally scale each participating local group and create approval items.') }}
          </p>
          <div v-if="form.sync_group_ratio" class="space-y-2">
            <p class="text-xs font-medium text-gray-600 dark:text-gray-300">
              {{ t('admin.upstreamSources.fields.participatingGroups', 'Participating local groups') }}
            </p>
            <label
              v-for="group in availableChannelGroups"
              :key="group.id"
              class="flex cursor-pointer items-center justify-between rounded border border-gray-100 px-3 py-2 dark:border-dark-700"
            >
              <span class="text-sm text-gray-700 dark:text-gray-300">{{ group.name }}</span>
              <input
                type="checkbox"
                :checked="!form.excluded_group_ids.includes(group.id)"
                @change="setGroupParticipation(group.id, ($event.target as HTMLInputElement).checked)"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              />
            </label>
            <p v-if="availableChannelGroups.length === 0" class="text-xs text-amber-600 dark:text-amber-400">
              {{ t('admin.upstreamSources.fields.noChannelGroups', 'The selected channel has no local groups.') }}
            </p>
          </div>
        </div>

        <!-- Pricing Source + Balance Threshold -->
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.upstreamSources.fields.pricingSource', 'Pricing Source') }}</label>
            <Select v-model="form.pricing_source" :options="pricingSourceOptions" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.upstreamSources.fields.balanceThreshold', 'Balance Alert Threshold (USD)') }}</label>
            <input v-model.number="form.balance_threshold_usd" type="number" step="0.0001" min="0" class="input" :placeholder="t('admin.upstreamSources.fields.optional', 'Optional')" />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.upstreamSources.fields.balanceThresholdHint', 'Requires a Dashboard Token. Alerts use the configured admin quota emails.') }}
            </p>
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
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type { UpstreamSourceConfig } from '@/api/admin/upstreamPriceSync'
import type { Column } from '@/components/common/types'
import type { AdminGroup, Proxy } from '@/types'
import type { Channel } from '@/api/admin/channels'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
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
  { key: 'proxy_id', label: t('admin.upstreamSources.columns.proxy', 'Proxy'), sortable: false },
  { key: 'balance', label: t('admin.upstreamSources.columns.balance', 'Balance'), sortable: false },
  { key: 'last_sync_at', label: t('admin.upstreamSources.columns.lastSync', 'Last Sync'), sortable: true },
  { key: 'actions', label: t('admin.upstreamSources.columns.actions', 'Actions'), sortable: false },
])

const pricingSourceOptions = computed(() => [
  { value: 'auto', label: t('admin.upstreamSources.pricingSourceAuto', 'auto') },
  { value: 'ratio_config', label: t('admin.upstreamSources.pricingSourceRatioConfig', 'ratio_config') },
  { value: 'pricing', label: t('admin.upstreamSources.pricingSourcePricing', 'pricing') },
])

const dashboardAuthModeOptions = computed(() => [
  { value: 'auto', label: t('admin.upstreamSources.dashboardAuthModeAuto', 'auto') },
  { value: 'bearer', label: t('admin.upstreamSources.dashboardAuthModeBearer', 'bearer') },
  { value: 'raw', label: t('admin.upstreamSources.dashboardAuthModeRaw', 'raw') },
  { value: 'raw_user', label: t('admin.upstreamSources.dashboardAuthModeRawUser', 'raw_user') },
  { value: 'bearer_user', label: t('admin.upstreamSources.dashboardAuthModeBearerUser', 'bearer_user') },
])

const needsDashboardUserId = computed(() => form.dashboard_auth_mode === 'raw_user' || form.dashboard_auth_mode === 'bearer_user')

// ── State ──
const sources = ref<UpstreamSourceConfig[]>([])
const channels = ref<Channel[]>([])
const groups = ref<AdminGroup[]>([])
const proxies = ref<Proxy[]>([])
const loading = ref(false)
const submitting = ref(false)
const searchQuery = ref('')
const syncingId = ref<number | null>(null)
const refreshingBalanceId = ref<number | null>(null)
const batchRefreshingBalances = ref(false)

const showDialog = ref(false)
const editingSource = ref<UpstreamSourceConfig | null>(null)
const showDeleteDialog = ref(false)
const deletingSource = ref<UpstreamSourceConfig | null>(null)

interface SourceForm {
  name: string
  base_url: string
  api_key: string
  dashboard_token: string
  dashboard_auth_mode: 'auto' | 'bearer' | 'raw' | 'raw_user' | 'bearer_user'
  dashboard_user_id: number | null
  proxy_id: number | null
  target_channel_id: number | null
  target_upstream_group: string
  balance_threshold_usd: number | null
  base_price_per_1k: number
  pricing_source: 'auto' | 'ratio_config' | 'pricing'
  enabled: boolean
  sync_model_price: boolean
  sync_group_ratio: boolean
  excluded_group_ids: number[]
}

const form = reactive<SourceForm>({
  name: '',
  base_url: '',
  api_key: '',
  dashboard_token: '',
  dashboard_auth_mode: 'auto',
  dashboard_user_id: null,
  proxy_id: null,
  target_channel_id: null,
  target_upstream_group: '',
  balance_threshold_usd: null,
  base_price_per_1k: 0.002,
  pricing_source: 'auto',
  enabled: true,
  sync_model_price: true,
  sync_group_ratio: false,
  excluded_group_ids: [],
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

const availableChannelGroups = computed(() => {
  const channel = channels.value.find((item) => item.id === form.target_channel_id)
  const groupIDs = new Set(channel?.group_ids ?? [])
  return groups.value
    .filter((group) => groupIDs.has(group.id))
    .sort((a, b) => a.sort_order - b.sort_order || a.id - b.id)
})

function setGroupParticipation(groupID: number, participating: boolean) {
  const excluded = new Set(form.excluded_group_ids)
  if (participating) excluded.delete(groupID)
  else excluded.add(groupID)
  form.excluded_group_ids = [...excluded]
}

// 上游可用分组:source 已保存后从 /api/pricing 全局 usable_group 拉取,用于按 group 过滤模型。
const upstreamGroups = ref<Record<string, string>>({})
const loadingGroups = ref(false)
const upstreamGroupOptions = computed(() => {
  const opts: { value: string; label: string }[] = [
    { value: '', label: t('admin.upstreamSources.fields.noGroupFilter', 'No filter (all models)') },
  ]
  for (const [key, name] of Object.entries(upstreamGroups.value)) {
    opts.push({ value: key, label: name ? `${key} — ${name}` : key })
  }
  return opts
})

watch(() => form.target_channel_id, (next, previous) => {
  if (previous != null && next !== previous) form.excluded_group_ids = []
})

async function loadGroupsFromForm() {
  const url = form.base_url.trim()
  if (!url) {
    upstreamGroups.value = {}
    return
  }
  loadingGroups.value = true
  try {
    const res = await adminAPI.upstreamPriceSync.previewUpstreamGroups(url, form.proxy_id, form.dashboard_token, form.api_key, form.dashboard_auth_mode, form.dashboard_user_id)
    upstreamGroups.value = res?.groups ?? {}
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreamSources.loadGroupsError', 'Failed to load upstream groups')))
    upstreamGroups.value = {}
  } finally {
    loadingGroups.value = false
  }
}

function channelName(id: number): string {
  return channels.value.find((c) => c.id === id)?.name ?? `#${id}`
}

function proxyName(id?: number | null): string {
  if (!id) return t('admin.upstreamSources.noProxy', 'Direct')
  return proxies.value.find((proxy) => proxy.id === id)?.name ?? `#${id}`
}

function formatBalance(value?: number | null): string {
  return value == null ? '-' : `$${value.toFixed(2)}`
}

function balanceStatusLabel(status?: UpstreamSourceConfig['balance_status']): string {
  return t(`admin.upstreamSources.balanceStatus.${status || 'unknown'}`, status || 'Unknown')
}

function balanceStatusClass(status?: UpstreamSourceConfig['balance_status']): string {
  switch (status) {
    case 'healthy': return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'low': return 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300'
    case 'error': return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    default: return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  }
}

// ── Helpers ──
function formatDateTime(value: string): string {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

function ensureProtocol(url: string): string {
  if (!url) return '#'
  return /^https?:\/\//i.test(url) ? url : `https://${url}`
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
    const [channelRes, groupList] = await Promise.all([
      adminAPI.channels.list(1, 1000),
      adminAPI.groups.getAllIncludingInactive(),
    ])
    channels.value = channelRes.items || []
    groups.value = groupList
  } catch (error) {
    console.error('Failed to load channels or groups for dropdown:', error)
  }
}

async function loadProxies() {
  try {
    proxies.value = await adminAPI.proxies.getAll()
  } catch (error) {
    console.error('Failed to load proxies for dropdown:', error)
  }
}

// ── Dialog ──
function resetForm() {
  form.name = ''
  form.base_url = ''
  form.api_key = ''
  form.dashboard_token = ''
  form.dashboard_auth_mode = 'auto'
  form.dashboard_user_id = null
  form.proxy_id = null
  form.target_channel_id = channels.value[0]?.id ?? null
  form.target_upstream_group = ''
  form.balance_threshold_usd = null
  form.base_price_per_1k = 0.002
  form.pricing_source = 'auto'
  form.enabled = true
  form.sync_model_price = true
  form.sync_group_ratio = false
  form.excluded_group_ids = []
}

async function openCreateDialog() {
  editingSource.value = null
  upstreamGroups.value = {}
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
  form.dashboard_auth_mode = (source.dashboard_auth_mode as SourceForm['dashboard_auth_mode']) || 'auto'
  form.dashboard_user_id = source.dashboard_user_id ?? null
  form.proxy_id = source.proxy_id ?? null
  form.target_channel_id = source.target_channel_id ?? null
  form.target_upstream_group = source.target_upstream_group ?? ''
  form.balance_threshold_usd = source.balance_threshold_usd ?? null
  form.base_price_per_1k = source.base_price_per_1k ?? 0.002
  form.pricing_source = (source.pricing_source as SourceForm['pricing_source']) || 'auto'
  form.enabled = !!source.enabled
  form.sync_model_price = source.sync_model_price !== false
  form.sync_group_ratio = source.sync_group_ratio === true
  form.excluded_group_ids = [...(source.excluded_group_ids ?? [])]
  await loadGroupsFromForm()
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
  if (form.sync_group_ratio && !form.target_upstream_group.trim()) {
    appStore.showError(t('admin.upstreamSources.errors.selectUpstreamGroup', 'Please select an upstream group before enabling group ratio sync'))
    return
  }
  const payload: UpstreamSourceConfig = {
    name: form.name.trim(),
    base_url: form.base_url.trim(),
    api_key: form.api_key,
    dashboard_token: form.dashboard_token || '',
    dashboard_auth_mode: form.dashboard_auth_mode,
    dashboard_user_id: needsDashboardUserId.value ? form.dashboard_user_id : null,
    proxy_id: form.proxy_id,
    target_channel_id: form.target_channel_id,
    target_upstream_group: form.target_upstream_group,
    balance_threshold_usd: form.balance_threshold_usd,
    base_price_per_1k: form.base_price_per_1k,
    pricing_source: form.pricing_source,
    enabled: form.enabled,
    sync_model_price: form.sync_model_price,
    sync_group_ratio: form.sync_group_ratio,
    excluded_group_ids: form.excluded_group_ids,
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

async function handleBatchRefreshBalances() {
  if (batchRefreshingBalances.value || sources.value.length === 0) return
  batchRefreshingBalances.value = true
  try {
    const result = await adminAPI.upstreamPriceSync.refreshAllBalances()
    if (result.failed > 0 || result.skipped > 0) {
      appStore.showInfo(t('admin.upstreamSources.batchBalanceRefreshPartial', {
        success: result.success,
        failed: result.failed,
        skipped: result.skipped,
      }))
    } else {
      appStore.showSuccess(t('admin.upstreamSources.batchBalanceRefreshSuccess', { success: result.success }))
    }
    await loadSources()
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreamSources.batchBalanceRefreshError', 'Failed to refresh upstream balances')))
    await loadSources()
  } finally {
    batchRefreshingBalances.value = false
  }
}

async function handleRefreshBalance(source: UpstreamSourceConfig) {
  if (source.id == null) return
  refreshingBalanceId.value = source.id
  try {
    const updated = await adminAPI.upstreamPriceSync.refreshBalance(source.id)
    const index = sources.value.findIndex((item) => item.id === source.id)
    if (index >= 0) sources.value[index] = updated
    appStore.showSuccess(t('admin.upstreamSources.balanceRefreshSuccess', 'Balance refreshed'))
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreamSources.balanceRefreshError', 'Failed to refresh balance')))
    await loadSources()
  } finally {
    refreshingBalanceId.value = null
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
  loadProxies()
})
</script>
