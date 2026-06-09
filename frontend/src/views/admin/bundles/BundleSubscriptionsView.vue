<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-start justify-between gap-4">
          <!-- Left: Filters -->
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <!-- User ID Search -->
            <div class="w-full sm:w-48">
              <input
                v-model.number="filters.user_id"
                type="number"
                :placeholder="t('bundles.admin.searchUserId')"
                class="input"
                @input="debounceSearch"
              />
            </div>

            <!-- Status Filter -->
            <div class="w-full sm:w-40">
              <Select
                v-model="filters.status"
                :options="statusOptions"
                :placeholder="t('bundles.admin.allStatus')"
                @change="loadSubscriptions"
              />
            </div>
          </div>

          <!-- Right: Actions -->
          <div class="flex items-center gap-2">
            <button @click="loadSubscriptions" :disabled="loading" class="btn btn-secondary" :title="t('common.refresh')">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </template>

      <!-- Subscriptions Table -->
      <template #table>
        <DataTable :columns="columns" :data="subscriptions" :loading="loading">
          <!-- Clickable row toggle -->
          <template #cell-id="{ row }">
            <div class="flex items-center gap-2">
              <button
                @click.stop="toggleRow(row.id)"
                class="rounded p-0.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700 dark:hover:text-gray-300"
                :title="expandedRowId === row.id ? t('common.collapse') : t('common.expand')"
              >
                <svg
                  class="h-4 w-4 transition-transform"
                  :class="{ 'rotate-90': expandedRowId === row.id }"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="2"
                >
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
                </svg>
              </button>
              <span class="text-sm font-medium text-gray-900 dark:text-white">{{ row.id }}</span>
            </div>
          </template>

          <template #cell-user_id="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-300">#{{ value }}</span>
          </template>

          <template #cell-plan_id="{ row }">
            <span class="text-sm font-medium text-gray-900 dark:text-white">
              {{ row.plan?.name || `#${row.plan_id}` }}
            </span>
          </template>

          <template #cell-status="{ value }">
            <span :class="statusBadgeClass(value)">{{ statusLabel(value) }}</span>
          </template>

          <template #cell-starts_at="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ formatDate(value) }}</span>
          </template>

          <template #cell-expires_at="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ formatDate(value) }}</span>
          </template>

          <template #cell-source="{ value }">
            <span :class="sourceBadgeClass(value)">{{ sourceLabel(value) }}</span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button
                v-if="row.status === 'active'"
                @click="openExtendDialog(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
              >
                <Icon name="calendar" size="sm" />
                <span class="text-xs">{{ t('bundles.admin.extend') }}</span>
              </button>
              <button
                v-if="row.status === 'active'"
                @click="handleRevoke(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              >
                <Icon name="ban" size="sm" />
                <span class="text-xs">{{ t('bundles.admin.revoke') }}</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState :title="t('bundles.admin.noSubscriptions')" />
          </template>
        </DataTable>

        <!-- Expanded Row Detail -->
        <div
          v-if="expandedRowId !== null"
          class="border-t border-gray-100 bg-gray-50 px-5 py-4 dark:border-dark-800 dark:bg-dark-800/50"
        >
          <template v-if="expandedSubscription?.group_usages?.length">
            <h4 class="mb-3 text-sm font-semibold text-gray-700 dark:text-gray-300">
              {{ t('bundles.admin.groupUsageDetails') }}
            </h4>
            <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
              <div
                v-for="usage in expandedSubscription.group_usages"
                :key="usage.id"
                class="rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-600 dark:bg-dark-800"
              >
                <div class="mb-2 flex items-center justify-between">
                  <span class="text-sm font-medium text-gray-900 dark:text-white">
                    {{ usage.group?.name || `Group #${usage.group_id}` }}
                  </span>
                  <span class="text-xs text-gray-500 dark:text-gray-400">
                    {{ usage.group?.platform || '' }}
                  </span>
                </div>
                <div v-if="usage.model_pattern" class="mb-2 text-xs text-gray-500 dark:text-gray-400">
                  Model: {{ usage.model_pattern }}
                </div>
                <div class="space-y-1.5">
                  <!-- Daily -->
                  <div class="flex items-center gap-2 text-xs">
                    <span class="w-12 text-gray-500 dark:text-gray-400">Daily</span>
                    <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                      <div
                        class="h-1.5 rounded-full transition-all"
                        :class="getProgressClass(usage.daily_usage_usd, usage.daily_limit_usd)"
                        :style="{ width: getProgressWidth(usage.daily_usage_usd, usage.daily_limit_usd) }"
                      ></div>
                    </div>
                    <span class="w-28 text-right text-gray-600 dark:text-gray-300">
                      ${{ usage.daily_usage_usd.toFixed(2) }} / ${{ usage.daily_limit_usd.toFixed(2) }}
                    </span>
                  </div>
                  <!-- Weekly -->
                  <div class="flex items-center gap-2 text-xs">
                    <span class="w-12 text-gray-500 dark:text-gray-400">Weekly</span>
                    <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                      <div
                        class="h-1.5 rounded-full transition-all"
                        :class="getProgressClass(usage.weekly_usage_usd, usage.weekly_limit_usd)"
                        :style="{ width: getProgressWidth(usage.weekly_usage_usd, usage.weekly_limit_usd) }"
                      ></div>
                    </div>
                    <span class="w-28 text-right text-gray-600 dark:text-gray-300">
                      ${{ usage.weekly_usage_usd.toFixed(2) }} / ${{ usage.weekly_limit_usd.toFixed(2) }}
                    </span>
                  </div>
                  <!-- Monthly -->
                  <div class="flex items-center gap-2 text-xs">
                    <span class="w-12 text-gray-500 dark:text-gray-400">Monthly</span>
                    <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                      <div
                        class="h-1.5 rounded-full transition-all"
                        :class="getProgressClass(usage.monthly_usage_usd, usage.monthly_limit_usd)"
                        :style="{ width: getProgressWidth(usage.monthly_usage_usd, usage.monthly_limit_usd) }"
                      ></div>
                    </div>
                    <span class="w-28 text-right text-gray-600 dark:text-gray-300">
                      ${{ usage.monthly_usage_usd.toFixed(2) }} / ${{ usage.monthly_limit_usd.toFixed(2) }}
                    </span>
                  </div>
                </div>
              </div>
            </div>
          </template>
          <div v-else class="text-sm text-gray-400 dark:text-gray-500">
            {{ t('bundles.admin.noGroupUsage') }}
          </div>
        </div>
      </template>

      <!-- Pagination -->
      <template #pagination>
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

    <!-- Extend Dialog -->
    <BaseDialog
      :show="showExtendDialog"
      :title="t('bundles.admin.extendSubscription')"
      width="narrow"
      @close="showExtendDialog = false"
    >
      <form id="extend-form" @submit.prevent="handleExtend" class="space-y-4">
        <p class="text-sm text-gray-600 dark:text-gray-400">
          {{ t('bundles.admin.extendHint', { id: extendingSubscription?.id }) }}
        </p>
        <div>
          <label class="input-label">{{ t('bundles.admin.extendDays') }} <span class="text-red-500">*</span></label>
          <input
            v-model.number="extendDays"
            type="number"
            min="1"
            class="input"
            required
          />
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" @click="showExtendDialog = false" class="btn btn-secondary">{{ t('common.cancel') }}</button>
          <button type="submit" form="extend-form" :disabled="extending" class="btn btn-primary">
            {{ extending ? t('common.saving') : t('common.confirm') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Revoke Confirm Dialog -->
    <BaseDialog
      :show="showRevokeDialog"
      :title="t('bundles.admin.revokeSubscription')"
      width="narrow"
      @close="showRevokeDialog = false"
    >
      <p class="text-sm text-gray-600 dark:text-gray-400">
        {{ t('bundles.admin.revokeConfirm', { id: revokingSubscription?.id }) }}
      </p>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" @click="showRevokeDialog = false" class="btn btn-secondary">{{ t('common.cancel') }}</button>
          <button type="button" @click="confirmRevoke" :disabled="revoking" class="btn btn-danger">
            {{ revoking ? t('common.saving') : t('bundles.admin.revoke') }}
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
import { bundlesAPI } from '@/api/admin/bundles'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { BundleSubscription } from '@/types/bundle'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import EmptyState from '@/components/common/EmptyState.vue'

const { t } = useI18n()
const appStore = useAppStore()

// ==================== BundleSubscriptionsView：管理后台套餐订阅管理页 ====================
// 提供套餐订阅的管理界面，包括：
// - 订阅列表表格（支持按用户ID搜索、状态过滤）
// - 展开行显示各渠道组的日/周/月用量进度条
// - 延期（延长天数）和撤销操作

// ==================== 状态 ====================
// ==================== State ====================

const loading = ref(false)
const subscriptions = ref<BundleSubscription[]>([])
const expandedRowId = ref<number | null>(null)

const filters = ref<{
  user_id: number | undefined
  status: '' | 'active' | 'expired' | 'revoked'
}>({
  user_id: undefined,
  status: '',
})

const pagination = ref({
  page: 1,
  page_size: 20,
  total: 0,
})

// Extend dialog
const showExtendDialog = ref(false)
const extendingSubscription = ref<BundleSubscription | null>(null)
const extendDays = ref(30)
const extending = ref(false)

// Revoke dialog
const showRevokeDialog = ref(false)
const revokingSubscription = ref<BundleSubscription | null>(null)
const revoking = ref(false)

// Debounce timer
let searchTimer: ReturnType<typeof setTimeout> | null = null

// ==================== 计算属性 ====================
// ==================== Computed ====================

const columns = computed((): Column[] => [
  { key: 'id', label: 'ID' },
  { key: 'user_id', label: t('bundles.admin.userId') },
  { key: 'plan_id', label: t('bundles.admin.planName') },
  { key: 'status', label: t('bundles.admin.status') },
  { key: 'starts_at', label: t('bundles.admin.startsAt') },
  { key: 'expires_at', label: t('bundles.admin.expiresAt') },
  { key: 'source', label: t('bundles.admin.source') },
  { key: 'actions', label: t('common.actions') },
])

const statusOptions = computed(() => [
  { value: '', label: t('bundles.admin.allStatus') },
  { value: 'active', label: t('bundles.admin.statusActive') },
  { value: 'expired', label: t('bundles.admin.statusExpired') },
  { value: 'revoked', label: t('bundles.admin.statusRevoked') },
])

const expandedSubscription = computed(() =>
  subscriptions.value.find(s => s.id === expandedRowId.value) ?? null
)

// ==================== 工具函数 ====================
// ==================== Helpers ====================

function formatDate(value: string): string {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

function statusBadgeClass(status: string): string {
  const base = 'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium'
  switch (status) {
    case 'active': return `${base} bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300`
    case 'expired': return `${base} bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400`
    case 'revoked': return `${base} bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300`
    default: return `${base} bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300`
  }
}

function statusLabel(status: string): string {
  switch (status) {
    case 'active': return t('bundles.admin.statusActive')
    case 'expired': return t('bundles.admin.statusExpired')
    case 'revoked': return t('bundles.admin.statusRevoked')
    default: return status
  }
}

function sourceBadgeClass(source: string): string {
  const base = 'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium'
  switch (source) {
    case 'purchase': return `${base} bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300`
    case 'redeem': return `${base} bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300`
    case 'admin_assign': return `${base} bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300`
    default: return `${base} bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300`
  }
}

function sourceLabel(source: string): string {
  switch (source) {
    case 'purchase': return t('bundles.admin.sourcePurchase')
    case 'redeem': return t('bundles.admin.sourceRedeem')
    case 'admin_assign': return t('bundles.admin.sourceAdminAssign')
    default: return source
  }
}

function toggleRow(id: number) {
  expandedRowId.value = expandedRowId.value === id ? null : id
}

function getProgressWidth(used: number, limit: number): string {
  if (!limit || limit <= 0) return '0%'
  const pct = Math.min((used / limit) * 100, 100)
  return `${pct.toFixed(1)}%`
}

function getProgressClass(used: number, limit: number): string {
  if (!limit || limit <= 0) return 'bg-gray-300 dark:bg-dark-500'
  const pct = (used / limit) * 100
  if (pct >= 100) return 'bg-red-500'
  if (pct >= 80) return 'bg-orange-500'
  return 'bg-green-500'
}

// ==================== 数据加载 ====================
// ==================== Data Loading ====================

function debounceSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    pagination.value.page = 1
    loadSubscriptions()
  }, 400)
}

async function loadSubscriptions() {
  loading.value = true
  try {
    const params: Record<string, unknown> = {
      page: pagination.value.page,
      page_size: pagination.value.page_size,
    }
    if (filters.value.user_id) params.user_id = filters.value.user_id
    if (filters.value.status) params.status = filters.value.status

    const res = await bundlesAPI.listSubscriptions(params as Parameters<typeof bundlesAPI.listSubscriptions>[0])
    subscriptions.value = res.items || []
    pagination.value.total = res.total || 0
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

function handlePageChange(page: number) {
  pagination.value.page = page
  loadSubscriptions()
}

function handlePageSizeChange(pageSize: number) {
  pagination.value.page_size = pageSize
  pagination.value.page = 1
  loadSubscriptions()
}

// ==================== 延期操作 ====================
// ==================== Extend ====================

function openExtendDialog(sub: BundleSubscription) {
  extendingSubscription.value = sub
  extendDays.value = 30
  showExtendDialog.value = true
}

async function handleExtend() {
  if (!extendingSubscription.value || extendDays.value < 1) return
  extending.value = true
  try {
    await bundlesAPI.extendSubscription(extendingSubscription.value.id, extendDays.value)
    appStore.showSuccess(t('common.saved'))
    showExtendDialog.value = false
    loadSubscriptions()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    extending.value = false
  }
}

// ==================== 撤销操作 ====================
// ==================== Revoke ====================

function handleRevoke(sub: BundleSubscription) {
  revokingSubscription.value = sub
  showRevokeDialog.value = true
}

async function confirmRevoke() {
  if (!revokingSubscription.value) return
  revoking.value = true
  try {
    await bundlesAPI.revokeSubscription(revokingSubscription.value.id)
    appStore.showSuccess(t('common.saved'))
    showRevokeDialog.value = false
    loadSubscriptions()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    revoking.value = false
  }
}

// ==================== 生命周期 ====================
// ==================== Lifecycle ====================

onMounted(() => {
  loadSubscriptions()
})
</script>
