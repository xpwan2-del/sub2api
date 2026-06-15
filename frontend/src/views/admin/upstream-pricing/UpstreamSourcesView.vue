<template>
  <AppLayout>
    <TablePageLayout>
      <!-- Actions -->
      <template #actions>
        <div class="flex items-center justify-end gap-2">
          <button @click="loadSources" :disabled="loading" class="btn btn-secondary" :title="t('common.refresh')">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
          <button @click="openEdit(null)" class="btn btn-primary">
            <Icon name="plus" size="sm" class="mr-1" />
            {{ t('upstreamPricing.sources.create') }}
          </button>
        </div>
      </template>

      <!-- Sources Table -->
      <template #table>
        <DataTable :columns="columns" :data="sources" :loading="loading">
          <template #cell-enabled="{ value }">
            <span :class="enabledBadgeClass(value)">{{ value ? t('common.enabled') : t('common.disabled') }}</span>
          </template>
          <template #cell-sync_interval_minutes="{ value }">
            <span class="text-sm">{{ value }} {{ t('upstreamPricing.sources.minutes') }}</span>
          </template>
          <template #cell-last_sync_status="{ value, row }">
            <div class="flex flex-col">
              <span :class="syncBadgeClass(value)">{{ syncStatusLabel(value) }}</span>
              <span v-if="row.last_sync_at" class="mt-0.5 text-[11px] text-gray-400 dark:text-gray-500">
                {{ formatRelativeTime(row.last_sync_at) }}
              </span>
            </div>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1.5">
              <button
                @click="handleTest(row)"
                :disabled="busyId === row.id"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-emerald-50 hover:text-emerald-600 dark:hover:bg-emerald-900/20 dark:hover:text-emerald-400 disabled:opacity-50"
                :title="t('upstreamPricing.sources.testConnection')"
              >
                <Icon name="bolt" size="sm" />
              </button>
              <button
                @click="handleSync(row)"
                :disabled="busyId === row.id"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400 disabled:opacity-50"
                :title="t('upstreamPricing.sources.syncNow')"
              >
                <Icon name="sync" size="sm" />
              </button>
              <button
                @click="openEdit(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
                :title="t('common.edit')"
              >
                <Icon name="edit" size="sm" />
              </button>
              <button
                @click="handleDelete(row)"
                :disabled="busyId === row.id"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400 disabled:opacity-50"
                :title="t('common.delete')"
              >
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </template>
        </DataTable>
      </template>
    </TablePageLayout>

    <!-- Edit Dialog -->
    <BaseDialog
      :show="showDialog"
      :title="editing ? t('upstreamPricing.sources.edit') : t('upstreamPricing.sources.create')"
      width="wide"
      @close="showDialog = false"
    >
      <form id="upstream-source-form" @submit.prevent="handleSave" class="space-y-4">
        <div>
          <label class="input-label">{{ t('upstreamPricing.sources.name') }} <span class="text-red-500">*</span></label>
          <input v-model="form.name" type="text" class="input" required maxlength="100" />
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('upstreamPricing.sources.baseUrl') }} <span class="text-red-500">*</span></label>
            <input v-model="form.base_url" type="url" class="input" required placeholder="https://api.openai.com" />
          </div>
          <div>
            <label class="input-label">{{ t('upstreamPricing.sources.pricingEndpoint') }} <span class="text-red-500">*</span></label>
            <input v-model="form.pricing_endpoint" type="text" class="input" required placeholder="/v1/pricing" />
          </div>
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('upstreamPricing.sources.parserType') }} <span class="text-red-500">*</span></label>
            <Select v-model="form.parser_type" :options="parserOptions" :placeholder="t('common.selectOption')" class="w-full" />
          </div>
          <div>
            <label class="input-label">{{ t('upstreamPricing.sources.apiKey') }}</label>
            <input v-model="form.api_key" type="password" class="input" autocomplete="new-password" />
          </div>
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('upstreamPricing.sources.syncIntervalMinutes') }}</label>
            <input v-model.number="form.sync_interval_minutes" type="number" min="0" class="input" />
          </div>
          <div>
            <label class="input-label">{{ t('upstreamPricing.sources.enabled') }}</label>
            <div class="mt-2 flex items-center gap-2">
              <button
                type="button"
                @click="form.enabled = !form.enabled"
                :class="[
                  'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                  form.enabled ? 'bg-blue-600' : 'bg-gray-300 dark:bg-gray-600'
                ]"
              >
                <span
                  :class="[
                    'pointer-events-none inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                    form.enabled ? 'translate-x-4' : 'translate-x-0'
                  ]"
                />
              </button>
              <span class="text-sm" :class="form.enabled ? 'text-green-600 dark:text-green-400' : 'text-gray-400'">
                {{ form.enabled ? t('common.enabled') : t('common.disabled') }}
              </span>
            </div>
          </div>
        </div>

        <div>
          <label class="input-label">{{ t('upstreamPricing.sources.modelAliasMap') }}</label>
          <textarea
            v-model="aliasMapText"
            rows="4"
            class="input font-mono text-xs"
            :placeholder='`{\n  "gpt-4o": "gpt-4o-2024-08-06",\n  "claude-3-5-sonnet": "claude-3-5-sonnet-20241022"\n}`'
          ></textarea>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('upstreamPricing.sources.modelAliasMapHint') }}
          </p>
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" @click="showDialog = false" class="btn btn-secondary">{{ t('common.cancel') }}</button>
          <button type="submit" form="upstream-source-form" :disabled="saving" class="btn btn-primary">
            {{ saving ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { upstreamPricingAPI } from '@/api/admin/upstreamPricing'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatRelativeTime } from '@/utils/format'
import type {
  UpstreamPriceSource,
  CreateUpstreamPriceSourceRequest
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

// ==================== Sources ====================

const loading = ref(false)
const saving = ref(false)
const busyId = ref<number | null>(null)
const sources = ref<UpstreamPriceSource[]>([])

const showDialog = ref(false)
const editing = ref<UpstreamPriceSource | null>(null)

interface SourceForm {
  name: string
  base_url: string
  pricing_endpoint: string
  api_key: string
  parser_type: string
  sync_interval_minutes: number
  enabled: boolean
}

const form = reactive<SourceForm>({
  name: '',
  base_url: '',
  pricing_endpoint: '',
  api_key: '',
  parser_type: 'openai',
  sync_interval_minutes: 360,
  enabled: true
})

const aliasMapText = ref('{}')

const parserOptions = computed(() => [
  { value: 'openai', label: 'OpenAI' },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'custom', label: t('upstreamPricing.sources.parserCustom') }
])

const columns = computed((): Column[] => [
  { key: 'id', label: 'ID' },
  { key: 'name', label: t('upstreamPricing.sources.name') },
  { key: 'parser_type', label: t('upstreamPricing.sources.parserType') },
  { key: 'enabled', label: t('upstreamPricing.sources.enabled') },
  { key: 'sync_interval_minutes', label: t('upstreamPricing.sources.syncInterval') },
  { key: 'last_sync_status', label: t('upstreamPricing.sources.lastSync') },
  { key: 'actions', label: t('common.actions') }
])

function enabledBadgeClass(v: boolean): string {
  const base = 'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium'
  return v
    ? `${base} bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300`
    : `${base} bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400`
}

function syncBadgeClass(status?: string): string {
  const base = 'inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium'
  switch (status) {
    case 'success':
      return `${base} bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300`
    case 'failed':
      return `${base} bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300`
    case 'running':
      return `${base} bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300`
    default:
      return `${base} bg-gray-100 text-gray-500 dark:bg-gray-700 dark:text-gray-400`
  }
}

function syncStatusLabel(status?: string): string {
  switch (status) {
    case 'success': return t('upstreamPricing.sources.syncSuccess')
    case 'failed': return t('upstreamPricing.sources.syncFailed')
    case 'running': return t('upstreamPricing.sources.syncRunning')
    default: return t('upstreamPricing.sources.syncNever')
  }
}

async function loadSources() {
  loading.value = true
  try {
    sources.value = await upstreamPricingAPI.listSources()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

function openEdit(src: UpstreamPriceSource | null) {
  editing.value = src
  if (src) {
    Object.assign(form, {
      name: src.name,
      base_url: src.base_url,
      pricing_endpoint: src.pricing_endpoint,
      api_key: '',
      parser_type: src.parser_type,
      sync_interval_minutes: src.sync_interval_minutes || 0,
      enabled: src.enabled
    })
    try {
      aliasMapText.value = JSON.stringify(src.model_alias_map || {}, null, 2)
    } catch {
      aliasMapText.value = '{}'
    }
  } else {
    Object.assign(form, {
      name: '',
      base_url: '',
      pricing_endpoint: '',
      api_key: '',
      parser_type: 'openai',
      sync_interval_minutes: 360,
      enabled: true
    })
    aliasMapText.value = '{}'
  }
  showDialog.value = true
}

function parseAliasMap(): Record<string, string> | undefined {
  const raw = aliasMapText.value.trim()
  if (!raw || raw === '{}' || raw === 'null') return undefined
  try {
    const parsed = JSON.parse(raw)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      const out: Record<string, string> = {}
      for (const [k, v] of Object.entries(parsed)) {
        out[k] = String(v)
      }
      return out
    }
  } catch {
    /* fall through */
  }
  throw new Error(t('upstreamPricing.sources.modelAliasMapInvalid'))
}

async function handleSave() {
  if (!form.name.trim()) {
    appStore.showError(t('upstreamPricing.sources.nameRequired'))
    return
  }
  if (!form.base_url.trim() || !form.pricing_endpoint.trim()) {
    appStore.showError(t('upstreamPricing.sources.urlRequired'))
    return
  }

  let aliasMap: Record<string, string> | undefined
  try {
    aliasMap = parseAliasMap()
  } catch (err: any) {
    appStore.showError(err?.message || t('common.error'))
    return
  }

  saving.value = true
  try {
    if (editing.value) {
      const payload: Partial<CreateUpstreamPriceSourceRequest> = {
        name: form.name,
        base_url: form.base_url,
        pricing_endpoint: form.pricing_endpoint,
        parser_type: form.parser_type,
        sync_interval_minutes: form.sync_interval_minutes,
        enabled: form.enabled,
        model_alias_map: aliasMap
      }
      // 仅在填入时才更新 api_key（避免清空）
      if (form.api_key.trim()) payload.api_key = form.api_key.trim()
      await upstreamPricingAPI.updateSource(editing.value.id, payload)
    } else {
      const payload: CreateUpstreamPriceSourceRequest = {
        name: form.name,
        base_url: form.base_url,
        pricing_endpoint: form.pricing_endpoint,
        api_key: form.api_key.trim() || undefined,
        parser_type: form.parser_type,
        sync_interval_minutes: form.sync_interval_minutes,
        enabled: form.enabled,
        model_alias_map: aliasMap
      }
      await upstreamPricingAPI.createSource(payload)
    }
    appStore.showSuccess(t('common.saved'))
    showDialog.value = false
    loadSources()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    saving.value = false
  }
}

async function handleTest(src: UpstreamPriceSource) {
  busyId.value = src.id
  try {
    const res = await upstreamPricingAPI.testSource(src.id)
    if (res.reachable) {
      appStore.showSuccess(
        t('upstreamPricing.sources.testOk', { count: res.model_count })
      )
    } else {
      appStore.showError(
        res.error
          ? `${t('upstreamPricing.sources.testFailed')}: ${res.error}`
          : t('upstreamPricing.sources.testFailed')
      )
    }
    loadSources()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    busyId.value = null
  }
}

async function handleSync(src: UpstreamPriceSource) {
  busyId.value = src.id
  try {
    await upstreamPricingAPI.syncSource(src.id)
    appStore.showSuccess(t('upstreamPricing.sources.syncStarted'))
    loadSources()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    busyId.value = null
  }
}

async function handleDelete(src: UpstreamPriceSource) {
  if (!window.confirm(t('upstreamPricing.sources.deleteConfirm', { name: src.name }))) return
  busyId.value = src.id
  try {
    await upstreamPricingAPI.deleteSource(src.id)
    appStore.showSuccess(t('common.deleted'))
    loadSources()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    busyId.value = null
  }
}

onMounted(() => {
  loadSources()
})
</script>
