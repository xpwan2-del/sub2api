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
              @change="onFilterChange"
            />
            <Select
              v-model="filters.source_config_id"
              :options="sourceFilterOptions"
              :placeholder="t('admin.priceChangeRequests.allSources', 'All Sources')"
              class="w-48"
              @change="onFilterChange"
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
                  v-for="[key, count] in sortedSummaryEntries(row.summary)"
                  :key="key"
                  :class="['inline-flex items-center rounded px-1.5 py-0.5 text-xs', statusBadgeClass(String(key))]"
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
                  @click="requestBatch('apply')"
                  :disabled="batchRunning || applyableSelectedItems(row.id).length === 0"
                  class="btn btn-primary btn-sm"
                >
                  <Icon name="check" size="md" class="mr-1" />
                  {{ t('admin.priceChangeRequests.batchApply', 'Batch Apply') }}
                </button>
                <button
                  @click="requestBatch('reject')"
                  :disabled="batchRunning || selectedItemsForRequest(row.id).length === 0"
                  class="btn btn-sm border border-red-300 bg-white text-red-600 hover:bg-red-50 dark:border-red-700 dark:bg-dark-800 dark:text-red-400 dark:hover:bg-red-900/20"
                >
                  <Icon name="x" size="md" class="mr-1" />
                  {{ t('admin.priceChangeRequests.batchReject', 'Batch Reject') }}
                </button>
                <button
                  @click="requestBatch('ignore')"
                  :disabled="batchRunning || selectedItemsForRequest(row.id).length === 0"
                  class="btn btn-secondary btn-sm"
                >
                  <Icon name="ban" size="md" class="mr-1" />
                  {{ t('admin.priceChangeRequests.batchIgnore', 'Batch Ignore') }}
                </button>
                <button
                  v-if="row.status === 'open' || row.status === 'partially_applied'"
                  @click="requestClose(row)"
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
              <div v-else class="space-y-5 overflow-x-auto">
                <section v-if="groupExpandedItems.length" data-test="group-ratio-section">
                  <h3 class="mb-2 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.priceChangeRequests.groupSection', 'Group rate changes') }}</h3>
                  <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
                    <thead><tr class="text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                      <th class="w-8 px-2 py-2"></th>
                      <th class="px-2 py-2">{{ t('admin.priceChangeRequests.groupItems.group', 'Local group') }}</th>
                      <th class="px-2 py-2">{{ t('admin.priceChangeRequests.groupItems.upstreamRatio', 'Upstream ratio') }}</th>
                      <th class="px-2 py-2">{{ t('admin.priceChangeRequests.groupItems.currentRate', 'Current rate') }}</th>
                      <th class="px-2 py-2">{{ t('admin.priceChangeRequests.groupItems.suggestedRate', 'Suggested rate') }}</th>
                      <th class="px-2 py-2">{{ t('admin.priceChangeRequests.groupItems.applyRate', 'Apply rate') }}</th>
                      <th class="px-2 py-2 text-right">{{ t('admin.priceChangeRequests.items.actions', 'Actions') }}</th>
                    </tr></thead>
                    <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                      <tr v-for="item in groupExpandedItems" :key="item.id" class="text-sm" data-test="group-ratio-item">
                        <td class="px-2 py-2"><input v-if="!isItemDone(item)" type="checkbox" :checked="isSelected(item.id)" @change="toggleSelect(item.id, ($event.target as HTMLInputElement).checked)" class="h-4 w-4 rounded border-gray-300 text-primary-600" /></td>
                        <td class="px-2 py-2 font-medium text-gray-900 dark:text-white">{{ item.group_rate_change.local_group_name }}</td>
                        <td class="px-2 py-2 font-mono text-xs">{{ formatRate(item.group_rate_change.upstream_old_ratio) }} → {{ formatRate(item.group_rate_change.upstream_new_ratio) }}</td>
                        <td class="px-2 py-2 font-mono text-xs">{{ formatRate(item.group_rate_change.local_current_rate) }}</td>
                        <td class="px-2 py-2 font-mono text-xs text-indigo-600 dark:text-indigo-400">{{ formatRate(item.group_rate_change.suggested_rate) }}</td>
                        <td class="px-2 py-2"><input v-model.number="groupDrafts[item.id]" type="number" step="0.0001" min="0.0001" :disabled="isItemDone(item)" class="input py-1 text-xs" style="width: 7rem" :aria-label="t('admin.priceChangeRequests.groupItems.applyRate', 'Apply rate')" /></td>
                        <td class="px-2 py-2 text-right"><div class="flex items-center justify-end gap-1">
                          <span v-if="item.status && item.status !== 'pending'" :class="['mr-1 inline-flex items-center rounded px-1.5 py-0.5 text-xs', statusBadgeClass(item.status)]">{{ statusLabel(item.status) }}</span>
                          <button @click="reviewSingle(item, 'apply')" :disabled="isItemDone(item) || reviewingId === item.id" class="btn-icon text-green-600 disabled:opacity-30" :title="t('admin.priceChangeRequests.actions.apply', 'Apply')"><Icon name="check" size="md" /></button>
                          <button @click="reviewSingle(item, 'reject')" :disabled="isItemDone(item) || reviewingId === item.id" class="btn-icon text-red-600 disabled:opacity-30" :title="t('admin.priceChangeRequests.actions.reject', 'Reject')"><Icon name="x" size="md" /></button>
                          <button @click="reviewSingle(item, 'ignore')" :disabled="isItemDone(item) || reviewingId === item.id" class="btn-icon text-gray-500 disabled:opacity-30" :title="t('admin.priceChangeRequests.actions.ignore', 'Ignore')"><Icon name="ban" size="md" /></button>
                        </div></td>
                      </tr>
                    </tbody>
                  </table>
                </section>
                <section v-if="sortedModelItems.length" data-test="model-price-section">
                  <h3 v-if="groupExpandedItems.length" class="mb-2 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.priceChangeRequests.modelSection', 'Model price changes') }}</h3>
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
                      v-for="item in sortedModelItems"
                      :key="item.id"
                      class="text-sm"
                    >
                      <td class="px-2 py-2 align-top">
                        <input
                          v-if="!isItemDone(item)"
                          type="checkbox"
                          :checked="isSelected(item.id)"
                          @change="toggleSelect(item.id, ($event.target as HTMLInputElement).checked)"
                          class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                        />
                      </td>
                      <td class="px-2 py-2 align-top font-medium text-gray-900 dark:text-white">
                        {{ modelField(item, 'name') }}
                      </td>
                      <td class="px-2 py-2 align-top text-gray-600 dark:text-gray-400">
                        <select
                          v-if="item.platform === '' && !isItemDone(item)"
                          v-model="platformDrafts[item.id]"
                          class="input py-1 text-xs"
                          :aria-label="t('admin.priceChangeRequests.items.platform', 'Platform')"
                        >
                          <option value="">{{ t('admin.priceChangeRequests.items.selectPlatform', 'Select platform') }}</option>
                          <option v-for="p in platformOptions" :key="p" :value="p">{{ p }}</option>
                        </select>
                        <span v-else>{{ item.platform || '-' }}</span>
                      </td>
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
                        <span v-if="isRemoved(item) || isUnchanged(item)" class="text-xs text-gray-400">—</span>
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
                            :disabled="isItemDone(item) || reviewingId === item.id"
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
                </section>
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
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <ConfirmDialog
      :show="showBatchConfirm"
      :title="batchConfirmTitle"
      :message="batchConfirmMessage"
      :confirm-text="t('common.confirm', 'Confirm')"
      :cancel-text="t('common.cancel', 'Cancel')"
      :danger="pendingBatchAction === 'reject'"
      @confirm="confirmBatch"
      @cancel="showBatchConfirm = false"
    />

    <ConfirmDialog
      :show="showCloseConfirm"
      :title="t('admin.priceChangeRequests.closeRequest', 'Close Request')"
      :message="closeConfirmMessage"
      :confirm-text="t('common.confirm', 'Confirm')"
      :cancel-text="t('common.cancel', 'Cancel')"
      :danger="true"
      @confirm="confirmCloseRequest"
      @cancel="showCloseConfirm = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type {
  ConvertedPrice,
  GroupRatioChangeItem,
  ModelPriceChangeItem,
  PriceChangeRequest,
  PriceChangeItem,
  ReviewItemBody,
} from '@/api/admin/upstreamPriceSync'
import type { Column } from '@/components/common/types'
import { perTokenToMTok, mTokToPerToken } from '@/components/admin/channel/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

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
  // 缺失字段默认显示 0(而非省略):上游还原值未返回 cache 等字段时,审批单展示更直观。
  const mTok0 = (v: number | null | undefined) => fmtNum(perTokenToMTok(v ?? 0))
  if (c.mode === 'per_request') {
    lines.push(`per_req=${fmtNum(c.perRequest ?? 0)}/req`)
  } else {
    lines.push(`in=${mTok0(c.input)}`)
    lines.push(`out=${mTok0(c.output)}`)
    lines.push(`cache_r=${mTok0(c.cacheRead)}`)
    lines.push(`cache_w=${mTok0(c.cacheWrite)}`)
  }
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

// 上游源下拉:全部 + 各上游源(id→name),按名排序。复用 sourcesIndex(页面已加载,无新增请求)。
const sourceFilterOptions = computed(() => [
  { value: '', label: t('admin.priceChangeRequests.allSources', 'All Sources') },
  ...Object.entries(sourcesIndex.value)
    .map(([id, name]) => ({ value: Number(id), label: name }))
    .sort((a, b) => a.label.localeCompare(b.label)),
])

// ── State ──
const requests = ref<PriceChangeRequest[]>([])
const loading = ref(false)
const filters = reactive({ status: '', source_config_id: '' as number | '' })
const pagination = reactive({ page: 1, page_size: 20, total: 0, pages: 0 })
const sourcesIndex = ref<Record<number, string>>({})
const sourceChannelIndex = ref<Record<number, number>>({})
const channelsIndex = ref<Record<number, string>>({})

const expandedRequestId = ref<number | null>(null)
const expandedItems = ref<PriceChangeItem[]>([])
const loadingItems = ref(false)
const reviewingId = ref<number | null>(null)
const batchRunning = ref(false)
const showBatchConfirm = ref(false)
const pendingBatchAction = ref<'apply' | 'reject' | 'ignore' | null>(null)
const showCloseConfirm = ref(false)
const pendingCloseRequest = ref<PriceChangeRequest | null>(null)

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
const groupDrafts = reactive<Record<number, number>>({})
const platformDrafts = reactive<Record<number, string>>({})

// 渠道支持的平台全集(与后端 domain/model 常量一致),用于给推断不出平台的模型补充平台。
const platformOptions = ['anthropic', 'openai', 'gemini', 'antigravity', 'grok'] as const

// Selected item ids (per request, but stored globally keyed by item id; current request implied)
const selected = reactive<Set<number>>(new Set())

// 渲染排序:价格变更 → 模型移除 → 新增模型 → 无变化;同类内按 id 升序。
// id 升序对齐渠道管理顺序:后端 DiffPricing 按 channel_model_pricing.id 升序
// (渠道添加顺序)落库本地已有模型,新增模型按模型名稳定排序后追加;
// ListItems ORDER BY id ASC → 同类内即按渠道顺序(新增类按模型名序)展示。
const kindRank: Record<ModelPriceChangeItem['kind'], number> = {
  model_price: 0,
  model_removed: 1,
  model_added: 2,
  model_unchanged: 3,
}

function isGroupRatioItem(item: PriceChangeItem): item is GroupRatioChangeItem {
  return item.kind === 'group_ratio'
}

const groupExpandedItems = computed<GroupRatioChangeItem[]>(() =>
  expandedItems.value
    .filter(isGroupRatioItem)
    .sort((a, b) => {
      const order = a.group_rate_change.local_group_sort_order - b.group_rate_change.local_group_sort_order
      return order !== 0 ? order : a.target_group_id - b.target_group_id
    }),
)

const sortedModelItems = computed<ModelPriceChangeItem[]>(() =>
  expandedItems.value
    .filter((item): item is ModelPriceChangeItem => !isGroupRatioItem(item))
    .sort((a, b) => {
      const ra = kindRank[a.kind]
      const rb = kindRank[b.kind]
      if (ra !== rb) return ra - rb
      return a.id - b.id
    }),
)

// ── Helpers ──
function formatDateTime(value: string): string {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

function formatRate(value: number | null | undefined): string {
  if (value == null || Number.isNaN(value)) return '-'
  return String(Number(value.toFixed(4)))
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

// model_unchanged:与本地一致(无变化),落库即终态 no_change,只读展示、无需审批。
function isUnchanged(item: PriceChangeItem): boolean {
  return item.kind === 'model_unchanged'
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

function modelField(item: ModelPriceChangeItem, _key: string): string {
  return item.model_name ?? '-'
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
      // 与状态列 open(待处理)同色:汇总里的 item 级 pending 也用蓝色,保持"待处理"视觉一致。
      return 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
    case 'rejected':
    case 'failed':
      return 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    case 'no_change':
      // 与 kind 列的 model_unchanged 保持同色(teal),避免与 ignored(灰)混淆。
      return 'bg-teal-50 text-teal-700 dark:bg-teal-900/30 dark:text-teal-300'
    case 'ignored':
      return 'bg-gray-50 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
    default:
      return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  }
}

// 汇总列展示顺序:待处理 → 应用 → 拒绝 → 忽略 → 无变化。
const summaryOrder: Record<string, number> = { pending: 0, applied: 1, rejected: 2, ignored: 3, no_change: 4 }
function sortedSummaryEntries(summary: Record<string, number>): [string, number][] {
  return Object.entries(summary).sort((a, b) => (summaryOrder[a[0]] ?? 99) - (summaryOrder[b[0]] ?? 99))
}

function kindBadgeClass(kind: string): string {
  switch (kind) {
    case 'model_added':
      return 'bg-green-50 text-green-700 dark:bg-green-900/30 dark:text-green-300'
    case 'model_removed':
      return 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    case 'model_unchanged':
      // 用 teal(青)而非 gray,与 ignored(灰)状态徽章拉开视觉距离。
      return 'bg-teal-50 text-teal-700 dark:bg-teal-900/30 dark:text-teal-300'
    default:
      return 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  }
}

// Draft helpers
function buildDraft(item: ModelPriceChangeItem): Draft {
  // 已保存的 apply_value 优先,逐字段缺失则回退上游还原值(满足"同步后默认 = 上游还原值")。
  // 两者都缺失(字段不存在)时表单补 0,与后端 apply_value 补 0 的默认语义保持一致。
  const a = pickPrice((item as any).apply_value)
  const u = pickPrice((item as any).upstream_converted)
  const mode = (a?.mode || u?.mode) || 'token'
  // draft 以 $/MTok(token 字段)/ $/次(per_request)展示与编辑;token 字段从 per-token ×1e6。
  const mTok = (av: number | null | undefined, uv: number | null | undefined): string => {
    const v = av !== null && av !== undefined ? av : uv
    return v === null || v === undefined ? '0' : String(perTokenToMTok(v))
  }
  const num = (av: number | null | undefined, uv: number | null | undefined): string => {
    const v = av !== null && av !== undefined ? av : uv
    return v === null || v === undefined ? '0' : String(v)
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
function upstreamFieldHint(item: ModelPriceChangeItem, key: DraftFieldKey): string {
  const c = pickPrice((item as any).upstream_converted)
  if (!c) return ''
  if (key === 'perRequest') return c.perRequest === null ? '' : fmtNum(c.perRequest)
  const v = c[key]
  return v === null ? '' : fmtNum(perTokenToMTok(v))
}

// Build apply_value payload from a draft. Send BOTH PascalCase and snake_case
// keys so the value binds regardless of whether the backend service struct has
// json tags (defensive against the cross-task serialization contract).
function buildApplyPayload(itemId: number): ConvertedPrice {
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
  return expandedItems.value.filter((i) => !isUnchanged(i) && !isItemDone(i)).map((i) => i.id)
}

function selectedItemsForRequest(_reqId: number): PriceChangeItem[] {
  return expandedItems.value.filter((i) => selected.has(i.id))
}

// 可应用选中项:全部选中项均可 apply(model_removed 应用后会删除渠道内对应模型)。
function applyableSelectedItems(reqId: number): PriceChangeItem[] {
  return selectedItemsForRequest(reqId)
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
      source_config_id: filters.source_config_id || undefined,
      page: pagination.page,
      page_size: pagination.page_size,
    })
    // Defensive: backend may return a paginated {items, total} or a plain array.
    const list = Array.isArray(res) ? res : (res?.items ?? [])
    requests.value = list as PriceChangeRequest[]
    pagination.total = Array.isArray(res) ? list.length : (res?.total ?? list.length)
    pagination.pages = Array.isArray(res) ? 0 : (res?.pages ?? 0)
    // 翻页/筛选后,若展开的批次已不在当前页,收起展开(避免残留 stale 条目 / 翻回时自动展开旧内容)。
    if (expandedRequestId.value != null && !requests.value.some((r) => ((r as any).id ?? (r as any).ID) === expandedRequestId.value)) {
      expandedRequestId.value = null
      expandedItems.value = []
    }
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

// 筛选条件变化:回到第 1 页再加载(避免停在越界页码看到空列表)。
function onFilterChange() {
  pagination.page = 1
  loadRequests()
}

function handlePageChange(page: number) {
  pagination.page = page
  loadRequests()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  loadRequests()
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
      if (isGroupRatioItem(it)) {
        if (!(it.id in groupDrafts)) groupDrafts[it.id] = it.apply_rate ?? it.group_rate_change.suggested_rate
      } else {
        if (!(it.id in drafts)) drafts[it.id] = buildDraft(it)
        if (!(it.id in platformDrafts)) platformDrafts[it.id] = it.platform
      }
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
    let body: ReviewItemBody
    if (action === 'apply') {
      body = isGroupRatioItem(item)
        ? { action, apply_rate: groupDrafts[item.id] }
        : { action, apply_value: buildApplyPayload(item.id), platform: platformDrafts[item.id] || undefined }
    } else {
      body = { action }
    }
    await adminAPI.upstreamPriceSync.reviewItem(reqId, item.id, body)
    appStore.showSuccess(t('admin.priceChangeRequests.reviewDone', 'Item reviewed'))
    await refreshExpanded()
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.priceChangeRequests.reviewError', 'Failed to review item')))
  } finally {
    reviewingId.value = null
  }
}

// 批量操作确认框标题/文案(按 pendingBatchAction 动态切换)。
const batchConfirmTitle = computed(() => {
  switch (pendingBatchAction.value) {
    case 'reject':
      return t('admin.priceChangeRequests.batchReject', 'Batch Reject')
    case 'ignore':
      return t('admin.priceChangeRequests.batchIgnore', 'Batch Ignore')
    default:
      return t('admin.priceChangeRequests.batchApply', 'Batch Apply')
  }
})
const batchConfirmMessage = computed(() => {
  const reqId = expandedRequestId.value
  // apply/reject/ignore 的确认数均为当前选中项数量。
  const count = reqId != null ? selectedItemsForRequest(reqId).length : 0
  return t('admin.priceChangeRequests.batchConfirmMessage', { count })
})

// 批量操作:先弹框确认(action 存入 pendingBatchAction),确认后执行 runBatch。
function requestBatch(action: 'apply' | 'reject' | 'ignore') {
  if (batchRunning.value) return
  if (selectedItemsForRequest(expandedRequestId.value!).length === 0) return
  pendingBatchAction.value = action
  showBatchConfirm.value = true
}

async function confirmBatch() {
  showBatchConfirm.value = false
  const action = pendingBatchAction.value
  pendingBatchAction.value = null
  if (action) await runBatch(action)
}

// runBatch 顺序对选中项执行 apply/reject/ignore(纯前端循环,每次 review 后端触发状态机重算)。
// 仅 apply 携带 apply_value;reject/ignore 只改 item 状态。
async function runBatch(action: 'apply' | 'reject' | 'ignore') {
  const reqId = expandedRequestId.value
  if (reqId == null) return
  // apply/reject/ignore 均对全部选中项执行(model_removed 的 apply 会删除渠道内对应模型)。
  const items = selectedItemsForRequest(reqId)
  if (items.length === 0) {
    appStore.showWarning(t('admin.priceChangeRequests.batchNoApplyable', 'No applyable items selected'))
    return
  }
  batchRunning.value = true
  let ok = 0
  let fail = 0
  try {
    for (const item of items) {
      try {
        let body: ReviewItemBody
        if (action === 'apply') {
          body = isGroupRatioItem(item)
            ? { action, apply_rate: groupDrafts[item.id] }
            : { action, apply_value: buildApplyPayload(item.id), platform: platformDrafts[item.id] || undefined }
        } else {
          body = { action }
        }
        await adminAPI.upstreamPriceSync.reviewItem(reqId, item.id, body)
        ok++
      } catch {
        fail++
      }
    }
    const doneKey = action === 'reject' ? 'batchRejectDone' : action === 'ignore' ? 'batchIgnoreDone' : 'batchDone'
    if (fail === 0) {
      appStore.showSuccess(t(`admin.priceChangeRequests.${doneKey}`, { count: ok }))
    } else {
      appStore.showWarning(t('admin.priceChangeRequests.batchPartial', { ok, fail }))
    }
    selected.clear()
    await refreshExpanded()
  } finally {
    batchRunning.value = false
  }
}

// 待处理条目(状态为 pending,含 group_ratio / model_* 全部 kind)。
// 关闭审批单时这些条目统一按「忽略」处理(不应用、不拒绝),与批量忽略语义一致。
function pendingItems(): PriceChangeItem[] {
  return expandedItems.value.filter((i) => !isItemDone(i))
}

// 关闭审批单确认框文案:提示将把全部待处理条目按「忽略」处理并关闭审批单。
const closeConfirmMessage = computed(() => {
  const count = pendingItems().length
  return t('admin.priceChangeRequests.closeConfirmMessage', { count })
})

// 点击「关闭审批单」:先弹框确认,不直接关闭。
function requestClose(row: PriceChangeRequest) {
  if (batchRunning.value) return
  pendingCloseRequest.value = row
  showCloseConfirm.value = true
}

// 确认关闭:对全部待处理条目逐条执行 ignore。
// 后端 FinalizeItemCAS 每条都会在事务内重算审批单状态,最后一条 pending 被忽略后
// pending=0 → 审批单自动置 closed,无需再调用 close 接口(此时再 close 会因状态已 closed 报错)。
async function confirmCloseRequest() {
  showCloseConfirm.value = false
  const row = pendingCloseRequest.value
  pendingCloseRequest.value = null
  if (!row) return
  const reqId = (row as any).id ?? (row as any).ID
  const items = pendingItems()
  // 防御:open / partially_applied 理论上必有 pending 条目;若为空则退回原关闭接口。
  if (items.length === 0) {
    await closeRequest(row)
    return
  }
  batchRunning.value = true
  let ok = 0
  let fail = 0
  try {
    for (const item of items) {
      try {
        await adminAPI.upstreamPriceSync.reviewItem(reqId, item.id, { action: 'ignore' })
        ok++
      } catch {
        fail++
      }
    }
    if (fail === 0) {
      appStore.showSuccess(t('admin.priceChangeRequests.closed', 'Request closed'))
    } else {
      appStore.showWarning(t('admin.priceChangeRequests.batchPartial', { ok, fail }))
    }
    await loadRequests()
    expandedRequestId.value = null
    expandedItems.value = []
    selected.clear()
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
      if (isGroupRatioItem(it)) {
        if (!(it.id in groupDrafts)) groupDrafts[it.id] = it.apply_rate ?? it.group_rate_change.suggested_rate
      } else {
        if (!(it.id in drafts)) drafts[it.id] = buildDraft(it)
        if (!(it.id in platformDrafts)) platformDrafts[it.id] = it.platform
      }
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
