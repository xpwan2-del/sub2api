<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <!-- Left: Status filter -->
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <Select
              v-model="filters.status"
              :options="statusFilterOptions"
              :placeholder="t('admin.priceChangeRequests.allStatuses', 'All Statuses')"
              class="w-44"
              @change="loadRequests"
            />
          </div>

          <!-- Right: Actions -->
          <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
            <button
              @click="loadRequests"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh', 'Refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="requests"
          :loading="loading"
          row-key="id"
          :virtual="false"
          :clickable-rows="true"
          :expanded-row-key="expandedRequestId"
          :default-sort-key="'created_at'"
          :default-sort-order="'desc'"
          @rowClick="toggleExpand"
        >
          <template #cell-id="{ value }">
            <span class="font-mono text-sm text-gray-700 dark:text-gray-300">#{{ value }}</span>
          </template>

          <template #cell-source_config_id="{ value }">
            <span class="text-sm text-gray-600 dark:text-gray-400">
              {{ sourceLabel(value) }}
            </span>
          </template>

          <template #cell-channel="{ row }">
            <span class="text-sm text-gray-600 dark:text-gray-400">
              {{ channelName(sourceChannelFor(row)) }}
            </span>
          </template>

          <template #cell-status="{ value }">
            <span :class="['inline-flex items-center rounded px-2 py-0.5 text-xs font-medium', statusBadgeClass(value)]">
              {{ statusLabel(value) }}
            </span>
          </template>

          <template #cell-summary="{ row }">
            <div class="flex flex-wrap gap-1">
              <template v-if="row.summary && Object.keys(row.summary).length">
                <span
                  v-for="(count, key) in row.summary"
                  :key="key"
                  class="inline-flex items-center rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-300"
                >
                  {{ statusLabel(String(key)) }}: <span class="ml-1 font-medium">{{ count }}</span>
                </span>
              </template>
              <span v-else class="text-xs text-gray-400">-</span>
            </div>
          </template>

          <template #cell-created_at="{ value }">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{ formatDateTime(value) }}</span>
          </template>

          <template #cell-actions="{ row }">
            <button
              @click.stop="toggleExpand(row)"
              class="btn-icon text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white"
              :title="expandedRequestId === row.id ? t('common.collapse', 'Collapse') : t('common.expand', 'Expand')"
            >
              <Icon :name="expandedRequestId === row.id ? 'chevronUp' : 'chevronDown'" size="md" />
            </button>
          </template>

          <!-- Expanded: items review -->
          <template #row-expansion="{ row }">
            <div class="bg-gray-50 px-4 py-4 dark:bg-dark-800/60">
              <!-- Batch bar -->
              <div
                v-if="expandedRequestId === row.id && expandedItems.length > 0"
                class="mb-3 flex flex-wrap items-center gap-3"
              >
                <label class="flex cursor-pointer items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                  <input
                    type="checkbox"
                    :checked="allVisibleSelected(row.id)"
                    :indeterminate.prop="someVisibleSelected(row.id)"
                    @change="toggleSelectAllVisible(row.id, ($event.target as HTMLInputElement).checked)"
                    class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                  />
                  {{ t('admin.priceChangeRequests.selectAllPending', 'Select all pending') }}
                </label>
                <span class="text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.priceChangeRequests.selectedCount', { count: selectedCount(row.id) }) }}
                </span>
                <button
                  @click="batchApply(row.id)"
                  :disabled="batchRunning || selectedItemsForRequest(row.id).length === 0"
                  class="btn btn-primary btn-sm"
                >
                  <Icon name="check" size="md" class="mr-1" />
                  {{ t('admin.priceChangeRequests.batchApply', 'Batch Apply') }}
                </button>
                <button
                  v-if="row.status === 'open' || row.status === 'partially_applied'"
                  @click="closeRequest(row)"
                  :disabled="batchRunning"
                  class="btn btn-secondary btn-sm"
                >
                  {{ t('admin.priceChangeRequests.closeRequest', 'Close Request') }}
                </button>
                <span v-if="loadingItems" class="text-sm text-gray-500 dark:text-gray-400">
                  {{ t('common.loading', 'Loading...') }}
                </span>
              </div>

              <!-- Items table -->
              <div v-if="loadingItems && expandedItems.length === 0" class="py-6 text-center text-sm text-gray-500 dark:text-gray-400">
                {{ t('common.loading', 'Loading...') }}
              </div>
              <div v-else-if="expandedItems.length === 0" class="py-6 text-center text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.priceChangeRequests.noItems', 'No items') }}
              </div>
              <div v-else class="overflow-x-auto">
                <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
                  <thead>
                    <tr class="text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                      <th class="px-2 py-2 w-8"></th>
                      <th class="px-2 py-2">{{ t('admin.priceChangeRequests.items.model', 'Model') }}</th>
                      <th class="px-2 py-2">{{ t('admin.priceChangeRequests.items.platform', 'Platform') }}</th>
                      <th class="px-2 py-2">{{ t('admin.priceChangeRequests.items.channel', 'Channel') }}</th>
                      <th class="px-2 py-2">{{ t('admin.priceChangeRequests.items.kind', 'Kind') }}</th>
                      <th class="px-2 py-2">{{ t('admin.priceChangeRequests.items.upstream', 'Upstream') }} <span class="font-normal normal-case text-gray-400">$/MTok</span></th>
                      <th class="px-2 py-2">{{ t('admin.priceChangeRequests.items.local', 'Local Current') }} <span class="font-normal normal-case text-gray-400">$/MTok</span></th>
                      <th class="px-2 py-2">{{ t('admin.priceChangeRequests.items.applyValue', 'Apply Value') }} <span class="font-normal normal-case text-gray-400">$/MTok</span></th>
                      <th class="px-2 py-2 text-right">{{ t('admin.priceChangeRequests.items.actions', 'Actions') }}</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                    <tr
                      v-for="item in expandedItems"
                      :key="item.id"
                      :class="['text-sm', isRemoved(item) ? 'opacity-50' : '']"
                    >
                      <td class="px-2 py-2 align-top">
                        <input
                          v-if="!isRemoved(item) && !isItemDone(item)"
                          type="checkbox"
                          :checked="isSelected(item.id)"
                          @change="toggleSelect(item.id, ($event.target as HTMLInputElement).checked)"
                          class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                        />
                      </td>
                      <td class="px-2 py-2 align-top font-medium text-gray-900 dark:text-white">
                        {{ modelField(item, 'name') }}
                      </td>
                      <td class="px-2 py-2 align-top text-gray-600 dark:text-gray-400">{{ item.platform || '-' }}</td>
                      <td class="px-2 py-2 align-top text-gray-600 dark:text-gray-400">
                        {{ channelName((item as any).target_channel_id ?? (item as any).TargetChannelID) }}
                      </td>
                      <td class="px-2 py-2 align-top">
                        <span :class="['inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium', kindBadgeClass(item.kind)]">
                          {{ kindLabel(item.kind) }}
                        </span>
                      </td>
                      <td class="px-2 py-2 align-top font-mono text-xs text-gray-700 dark:text-gray-300">
                        <div class="flex flex-col leading-tight">
                          <span v-for="(line, idx) in priceDetailStrings(item.upstream_converted)" :key="'up-' + item.id + '-' + idx" class="block">{{ line }}</span>
                        </div>
                      </td>
                      <td class="px-2 py-2 align-top font-mono text-xs text-gray-700 dark:text-gray-300">
                        <div class="flex flex-col leading-tight">
                          <span v-for="(line, idx) in priceDetailStrings(item.local_current)" :key="'lc-' + item.id + '-' + idx" class="block">{{ line }}</span>
                        </div>
                      </td>
                      <td class="px-2 py-2 align-top">
                        <span v-if="isRemoved(item)" class="text-xs text-gray-400">—</span>
                        <!-- per_request 计费:单一按次价格 -->
                        <div v-else-if="draftMode(drafts[item.id]) === 'per_request'" class="flex flex-col gap-1">
                          <label class="flex items-center gap-1 text-xs">
                            <span class="inline-block w-14 shrink-0 text-right text-gray-500 dark:text-gray-400">per_req</span>
                            <input
                              :value="drafts[item.id]?.perRequest"
                              type="number"
                              step="any"
                              min="0"
                              :disabled="isItemDone(item)"
                              :placeholder="upstreamFieldHint(item, 'perRequest')"
                              class="input py-1 text-xs"
                              style="width: 6rem"
                              @input="setDraftField(drafts[item.id], 'perRequest', ($event.target as HTMLInputElement).value)"
                            />
                          </label>
                        </div>
                        <!-- token 计费:逐字段编辑 in / out / cache_r / cache_w(默认 = 上游还原值) -->
                        <div v-else class="flex flex-col gap-1">
                          <label v-for="f in tokenApplyFields" :key="f.key" class="flex items-center gap-1 text-xs">
                            <span class="inline-block w-14 shrink-0 text-right text-gray-500 dark:text-gray-400">{{ f.label }}</span>
                            <input
                              :value="drafts[item.id]?.[f.key]"
                              type="number"
                              step="any"
                              min="0"
                              :disabled="isItemDone(item)"
                              :placeholder="upstreamFieldHint(item, f.key)"
                              class="input py-1 text-xs"
                              style="width: 6rem"
                              @input="setDraftField(drafts[item.id], f.key, ($event.target as HTMLInputElement).value)"
                            />
                          </label>
                        </div>
                      </td>
                      <td class="px-2 py-2 align-top text-right">
                        <div class="flex items-center justify-end gap-1">
                          <span
                            v-if="item.status && item.status !== 'pending'"
                            :class="['mr-1 inline-flex items-center rounded px-1.5 py-0.5 text-xs', statusBadgeClass(item.status)]"
                          >
                            {{ statusLabel(item.status) }}
                          </span>
                          <button
                            @click="reviewSingle(item, 'apply')"
                            :disabled="isRemoved(item) || isItemDone(item) || reviewingId === item.id"
                            class="btn-icon text-green-600 hover:text-green-700 disabled:opacity-30 dark:text-green-400"
                            :title="t('admin.priceChangeRequests.actions.apply', 'Apply')"
                          >
                            <Icon name="check" size="md" />
                          </button>
                          <button
                            @click="reviewSingle(item, 'reject')"
                            :disabled="isItemDone(item) || reviewingId === item.id"
                            class="btn-icon text-red-600 hover:text-red-700 disabled:opacity-30 dark:text-red-400"
                            :title="t('admin.priceChangeRequests.actions.reject', 'Reject')"
                          >
                            <Icon name="x" size="md" />
                          </button>
                          <button
                            @click="reviewSingle(item, 'ignore')"
                            :disabled="isItemDone(item) || reviewingId === item.id"
                            class="btn-icon text-gray-500 hover:text-gray-700 disabled:opacity-30 dark:text-gray-400"
                            :title="t('admin.priceChangeRequests.actions.ignore', 'Ignore')"
                          >
                            <Icon name="ban" size="md" />
                          </button>
                        </div>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </template>

          <template #empty>
            <EmptyState
              icon=""
              :title="t('admin.priceChangeRequests.noRequestsTitle', 'No Price Change Requests')"
              :description="t('admin.priceChangeRequests.noRequestsDesc', 'Trigger a sync from an upstream source to generate review items.')"
            />
          </template>
        </DataTable>
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type { PriceChangeRequest, PriceChangeItem } from '@/api/admin/upstreamPriceSync'
import type { Column } from '@/components/common/types'
import { perTokenToMTok, mTokToPerToken } from '@/components/admin/channel/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

// ── Inline PriceDetail render function (formats a ConvertedPrice-like object) ──
// Reads both PascalCase (backend raw) and snake_case keys defensively.
function pickPrice(p: any): {
  mode: string
  input: number | null
  output: number | null
  cacheRead: number | null
  cacheWrite: number | null
  perRequest: number | null
} {
  const g = (keys: string[]): any => {
    for (const k of keys) {
      const v = p?.[k]
      if (v !== undefined && v !== null && v !== '') return v
    }
    return null
  }
  return {
    mode: g(['BillingMode', 'billing_mode']) ?? '',
    input: g(['InputPrice', 'input_price']),
    output: g(['OutputPrice', 'output_price']),
    cacheRead: g(['CacheReadPrice', 'cache_read_price']),
    cacheWrite: g(['CacheWritePrice', 'cache_write_price']),
    perRequest: g(['PerRequestPrice', 'per_request_price']),
  }
}

function fmtNum(v: number | null | undefined): string {
  if (v === null || v === undefined || Number.isNaN(v)) return '-'
  // 价格以 $/MTok(token 字段)或 $/次(per_request)展示,均为常规量级;保留 6 位小数去尾零。
  return String(Number(v.toFixed(6)))
}

function priceDetailStrings(p: any): string[] {
  if (!p || typeof p !== 'object') return ['-']
  const c = pickPrice(p)
  const lines: string[] = []
  if (c.mode) lines.push(`[${c.mode}]`)
  // per_request 为 $/次,直接展示;token 字段(in/out/cache_*)为 per-token 存储,×1e6 显示成 $/MTok。
  if (c.perRequest !== null) lines.push(`per_req=${fmtNum(c.perRequest)}/req`)
  if (c.input !== null) lines.push(`in=${fmtNum(perTokenToMTok(c.input))}`)
  if (c.output !== null) lines.push(`out=${fmtNum(perTokenToMTok(c.output))}`)
  if (c.cacheRead !== null) lines.push(`cache_r=${fmtNum(perTokenToMTok(c.cacheRead))}`)
  if (c.cacheWrite !== null) lines.push(`cache_w=${fmtNum(perTokenToMTok(c.cacheWrite))}`)
  return lines.length ? lines : ['-']
}

// ── Columns ──
const columns = computed<Column[]>(() => [
  { key: 'id', label: t('admin.priceChangeRequests.columns.id', 'ID'), sortable: true },
  { key: 'source_config_id', label: t('admin.priceChangeRequests.columns.upstreamSource', 'Upstream Source'), sortable: false },
  { key: 'channel', label: t('admin.priceChangeRequests.columns.channel', 'Channel'), sortable: false },
  { key: 'status', label: t('admin.priceChangeRequests.columns.status', 'Status'), sortable: true },
  { key: 'summary', label: t('admin.priceChangeRequests.columns.summary', 'Summary'), sortable: false },
  { key: 'created_at', label: t('admin.priceChangeRequests.columns.created', 'Created'), sortable: true },
  { key: 'actions', label: '', sortable: false },
])

const statusFilterOptions = computed(() => [
  { value: '', label: t('admin.priceChangeRequests.allStatuses', 'All Statuses') },
  { value: 'open', label: t('admin.priceChangeRequests.statusOpen', 'Open') },
  { value: 'partially_applied', label: t('admin.priceChangeRequests.statusPartially', 'Partially Applied') },
  { value: 'closed', label: t('admin.priceChangeRequests.statusClosed', 'Closed') },
  { value: 'expired', label: t('admin.priceChangeRequests.statusExpired', 'Expired') },
])

// ── State ──
const requests = ref<PriceChangeRequest[]>([])
const loading = ref(false)
const filters = reactive({ status: '' })
const sourcesIndex = ref<Record<number, string>>({})
const sourceChannelIndex = ref<Record<number, number>>({})
const channelsIndex = ref<Record<number, string>>({})

const expandedRequestId = ref<number | null>(null)
const expandedItems = ref<PriceChangeItem[]>([])
const loadingItems = ref(false)
const reviewingId = ref<number | null>(null)
const batchRunning = ref(false)

// Per-item editable apply_value drafts: itemId → { mode, input, output, cacheRead, cacheWrite, perRequest }
interface Draft {
  mode: string
  input: string
  output: string
  cacheRead: string
  cacheWrite: string
  perRequest: string
}
const drafts = reactive<Record<number, Draft>>({})

// Selected item ids (per request, but stored globally keyed by item id; current request implied)
const selected = reactive<Set<number>>(new Set())

// ── Helpers ──
function formatDateTime(value: string): string {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

function sourceLabel(id: number): string {
  return sourcesIndex.value[id] ?? `source #${id}`
}

// 顶层 request 通过 source_config_id 反查其目标渠道(一个 source 一对一绑一个 target channel)。
function sourceChannelFor(row: PriceChangeRequest): number | undefined {
  const sid = (row as any).source_config_id ?? (row as any).SourceConfigID
  return sid != null ? sourceChannelIndex.value[sid] : undefined
}

function channelName(id?: number | null): string {
  if (id == null) return '-'
  return channelsIndex.value[id] ?? `#${id}`
}

async function loadChannelsIndex() {
  try {
    const res: any = await adminAPI.channels.list(1, 1000)
    const items = res?.items ?? []
    for (const c of items) channelsIndex.value[c.id] = c.name
  } catch {
    // best-effort: 列表为空时 channel 列回退显示 #id
  }
}

// 拉取上游源配置,建立 source_config_id → name(上游源名称)与 → target_channel_id(渠道)映射。
// 修复历史问题:此前 sourcesIndex 只塞 `source #N` 占位,"来源"列始终无真实名称。
async function loadSourcesIndex() {
  try {
    const list = await adminAPI.upstreamPriceSync.listSources()
    for (const s of list ?? []) {
      const sid = (s as any).id
      if (sid == null) continue
      sourcesIndex.value[sid] = s.name || `source #${sid}`
      const cid = (s as any).target_channel_id
      if (cid != null) sourceChannelIndex.value[sid] = cid
    }
  } catch {
    // best-effort: 失败时顶层"上游源名称/渠道"列回退占位
  }
}

function isRemoved(item: PriceChangeItem): boolean {
  return item.kind === 'model_removed'
}

// item 是否已处理(非 pending 终态:applied/rejected/ignored/failed)。
// 已处理条目禁止再编辑应用值、再审批、被批量选中(与后端 ReviewItem 的 pending 守卫对齐)。
function isItemDone(item: PriceChangeItem): boolean {
  return !!item.status && item.status !== 'pending'
}

// 状态/类型枚举 → 本地化文本(未知值回退原始字符串,防御后端未来新增枚举)。
function statusLabel(status: string): string {
  return t(`admin.priceChangeRequests.statuses.${status}`, status)
}
function kindLabel(kind: string): string {
  return t(`admin.priceChangeRequests.kinds.${kind}`, kind)
}

function modelField(item: PriceChangeItem, _key: string): string {
  // Defensive: item.model_name (snake) or ModelName (pascal)
  return (item as any).model_name ?? (item as any).ModelName ?? '-'
}

function statusBadgeClass(status: string): string {
  switch (status) {
    case 'open':
      return 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
    case 'partially_applied':
      return 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    case 'closed':
      return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
    case 'expired':
      return 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    case 'applied':
      return 'bg-green-50 text-green-700 dark:bg-green-900/30 dark:text-green-300'
    case 'pending':
      return 'bg-gray-50 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
    case 'rejected':
    case 'failed':
      return 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    case 'ignored':
      return 'bg-gray-50 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
    default:
      return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  }
}

function kindBadgeClass(kind: string): string {
  switch (kind) {
    case 'model_added':
      return 'bg-green-50 text-green-700 dark:bg-green-900/30 dark:text-green-300'
    case 'model_removed':
      return 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    default:
      return 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  }
}

// Draft helpers
function buildDraft(item: PriceChangeItem): Draft {
  // 已保存的 apply_value 优先,逐字段缺失则回退上游还原值(满足"同步后默认 = 上游还原值")。
  const a = pickPrice((item as any).apply_value)
  const u = pickPrice((item as any).upstream_converted)
  const mode = (a?.mode || u?.mode) || 'token'
  // draft 以 $/MTok(token 字段)/ $/次(per_request)展示与编辑;token 字段从 per-token ×1e6。
  const mTok = (av: number | null | undefined, uv: number | null | undefined): string => {
    const v = av !== null && av !== undefined ? av : uv
    return v === null || v === undefined ? '' : String(perTokenToMTok(v))
  }
  const num = (av: number | null | undefined, uv: number | null | undefined): string => {
    const v = av !== null && av !== undefined ? av : uv
    return v === null || v === undefined ? '' : String(v)
  }
  return {
    mode,
    input: mTok(a?.input, u?.input),
    output: mTok(a?.output, u?.output),
    cacheRead: mTok(a?.cacheRead, u?.cacheRead),
    cacheWrite: mTok(a?.cacheWrite, u?.cacheWrite),
    perRequest: num(a?.perRequest, u?.perRequest),
  }
}

// token 计费下逐字段编辑的输入框定义(label 与 priceDetailStrings 的 in/out/cache_r/cache_w 对齐)。
const tokenApplyFields: ReadonlyArray<{ key: 'input' | 'output' | 'cacheRead' | 'cacheWrite'; label: string }> = [
  { key: 'input', label: 'in' },
  { key: 'output', label: 'out' },
  { key: 'cacheRead', label: 'cache_r' },
  { key: 'cacheWrite', label: 'cache_w' },
]

type DraftFieldKey = 'input' | 'output' | 'cacheRead' | 'cacheWrite' | 'perRequest'

function draftMode(d: Draft | undefined): string {
  return d?.mode || 'token'
}

function setDraftField(d: Draft | undefined, key: DraftFieldKey, v: string) {
  if (!d) return
  d[key] = v
}

// placeholder:提示上游还原值对应的 $/MTok(token 字段)或 $/次(per_request),为空则不提示。
function upstreamFieldHint(item: PriceChangeItem, key: DraftFieldKey): string {
  const c = pickPrice((item as any).upstream_converted)
  if (!c) return ''
  if (key === 'perRequest') return c.perRequest === null ? '' : fmtNum(c.perRequest)
  const v = c[key]
  return v === null ? '' : fmtNum(perTokenToMTok(v))
}

// Build apply_value payload from a draft. Send BOTH PascalCase and snake_case
// keys so the value binds regardless of whether the backend service struct has
// json tags (defensive against the cross-task serialization contract).
function buildApplyPayload(itemId: number): Record<string, unknown> {
  const d = drafts[itemId]
  if (!d) return {}
  // token 字段:draft 为 $/MTok,提交前 ÷1e6 还原 per-token;per_request 直接透传。
  const mode = d.mode || 'token'
  const input = mTokToPerToken(d.input)
  const output = mTokToPerToken(d.output)
  const cacheRead = mTokToPerToken(d.cacheRead)
  const cacheWrite = mTokToPerToken(d.cacheWrite)
  const perRequest = d.perRequest === '' ? null : Number(d.perRequest)
  return {
    BillingMode: mode,
    billing_mode: mode,
    InputPrice: input,
    input_price: input,
    OutputPrice: output,
    output_price: output,
    CacheReadPrice: cacheRead,
    cache_read_price: cacheRead,
    CacheWritePrice: cacheWrite,
    cache_write_price: cacheWrite,
    PerRequestPrice: perRequest,
    per_request_price: perRequest,
  }
}

// Selection helpers (operate within the currently expanded request)
function currentRequestItemIds(): number[] {
  return expandedItems.value.filter((i) => !isRemoved(i) && !isItemDone(i)).map((i) => i.id)
}

function selectedItemsForRequest(_reqId: number): PriceChangeItem[] {
  return expandedItems.value.filter((i) => selected.has(i.id))
}

function selectedCount(_reqId: number): number {
  return selectedItemsForRequest(_reqId).length
}

function isSelected(itemId: number): boolean {
  return selected.has(itemId)
}

function toggleSelect(itemId: number, checked: boolean) {
  if (checked) selected.add(itemId)
  else selected.delete(itemId)
}

function allVisibleSelected(_reqId: number): boolean {
  const ids = currentRequestItemIds()
  return ids.length > 0 && ids.every((id) => selected.has(id))
}

function someVisibleSelected(_reqId: number): boolean {
  const ids = currentRequestItemIds()
  return ids.some((id) => selected.has(id)) && !ids.every((id) => selected.has(id))
}

function toggleSelectAllVisible(_reqId: number, checked: boolean) {
  for (const id of currentRequestItemIds()) {
    if (checked) selected.add(id)
    else selected.delete(id)
  }
}

// ── Load ──
async function loadRequests() {
  loading.value = true
  try {
    const res: any = await adminAPI.upstreamPriceSync.listRequests({
      status: filters.status || undefined,
    })
    // Defensive: backend may return a paginated {items, total} or a plain array.
    const list = Array.isArray(res) ? res : (res?.items ?? [])
    requests.value = list as PriceChangeRequest[]
    // Index source labels (best-effort)
    for (const r of requests.value) {
      const sid = (r as any).source_config_id ?? (r as any).SourceConfigID
      if (sid && !(sid in sourcesIndex.value)) sourcesIndex.value[sid] = `source #${sid}`
    }
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.priceChangeRequests.loadError', 'Failed to load requests')))
  } finally {
    loading.value = false
  }
}

async function toggleExpand(row: PriceChangeRequest) {
  const id = (row as any).id ?? (row as any).ID
  if (expandedRequestId.value === id) {
    expandedRequestId.value = null
    expandedItems.value = []
    return
  }
  expandedRequestId.value = id
  expandedItems.value = []
  selected.clear()
  loadingItems.value = true
  try {
    const res: any = await adminAPI.upstreamPriceSync.getRequest(id)
    // Defensive: backend may return {request, items} or a flattened {...request, items}.
    const items = (res?.items ?? []) as PriceChangeItem[]
    expandedItems.value = items
    // Initialize drafts for pending items
    for (const it of items) {
      if (!(it.id in drafts)) drafts[it.id] = buildDraft(it)
    }
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.priceChangeRequests.loadItemsError', 'Failed to load request details')))
  } finally {
    loadingItems.value = false
  }
}

// ── Review actions ──
async function reviewSingle(item: PriceChangeItem, action: 'apply' | 'reject' | 'ignore') {
  if (reviewingId.value !== null) return
  const reqId = expandedRequestId.value
  if (reqId == null) return
  reviewingId.value = item.id
  try {
    const body: { action: string; apply_value?: Record<string, unknown> } = { action }
    if (action === 'apply') body.apply_value = buildApplyPayload(item.id)
    await adminAPI.upstreamPriceSync.reviewItem(reqId, item.id, body)
    appStore.showSuccess(t('admin.priceChangeRequests.reviewDone', 'Item reviewed'))
    await refreshExpanded()
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.priceChangeRequests.reviewError', 'Failed to review item')))
  } finally {
    reviewingId.value = null
  }
}

async function batchApply(_reqId: number) {
  const items = selectedItemsForRequest(_reqId)
  if (items.length === 0) return
  batchRunning.value = true
  let ok = 0
  let fail = 0
  try {
    for (const item of items) {
      try {
        await adminAPI.upstreamPriceSync.reviewItem(expandedRequestId.value as number, item.id, {
          action: 'apply',
          apply_value: buildApplyPayload(item.id),
        })
        ok++
      } catch {
        fail++
      }
    }
    if (fail === 0) {
      appStore.showSuccess(t('admin.priceChangeRequests.batchDone', { count: ok }))
    } else {
      appStore.showWarning(t('admin.priceChangeRequests.batchPartial', { ok, fail }))
    }
    selected.clear()
    await refreshExpanded()
  } finally {
    batchRunning.value = false
  }
}

async function closeRequest(row: PriceChangeRequest) {
  const id = (row as any).id ?? (row as any).ID
  try {
    await adminAPI.upstreamPriceSync.closeRequest(id)
    appStore.showSuccess(t('admin.priceChangeRequests.closed', 'Request closed'))
    await loadRequests()
    expandedRequestId.value = null
    expandedItems.value = []
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.priceChangeRequests.closeError', 'Failed to close request')))
  }
}

async function refreshExpanded() {
  const reqId = expandedRequestId.value
  if (reqId == null) return
  try {
    const res: any = await adminAPI.upstreamPriceSync.getRequest(reqId)
    expandedItems.value = (res?.items ?? []) as PriceChangeItem[]
    // Refresh drafts for any new pending items
    for (const it of expandedItems.value) {
      if (!(it.id in drafts)) drafts[it.id] = buildDraft(it)
    }
  } catch {
    // non-fatal
  }
  // Also refresh the request list summary
  await loadRequests()
}

// ── Lifecycle ──
onMounted(() => {
  loadRequests()
  loadChannelsIndex()
  loadSourcesIndex()
})
</script>
