<template>
  <AppLayout>
    <TablePageLayout>
      <!-- Actions -->
      <template #actions>
        <div class="flex items-center justify-end gap-2">
          <button @click="loadPlans" :disabled="plansLoading" class="btn btn-secondary" :title="t('common.refresh')">
            <Icon name="refresh" size="md" :class="plansLoading ? 'animate-spin' : ''" />
          </button>
          <button @click="openPlanEdit(null)" class="btn btn-primary">{{ t('bundles.admin.createPlan') }}</button>
        </div>
      </template>

      <!-- Plans Table -->
      <template #table>
        <DataTable :columns="planColumns" :data="plans" :loading="plansLoading">
          <template #cell-tier="{ value }">
            <span :class="tierBadgeClass(value)">{{ tierLabel(value) }}</span>
          </template>
          <template #cell-price="{ value, row }">
            <div class="text-sm">
              <span class="font-medium text-gray-900 dark:text-white">{{ row.currency === 'CNY' ? '¥' : '$' }}{{ (value ?? 0).toFixed(2) }}</span>
              <span v-if="row.original_price" class="ml-1 text-xs text-gray-400 line-through">{{ row.currency === 'CNY' ? '¥' : '$' }}{{ row.original_price.toFixed(2) }}</span>
            </div>
          </template>
          <template #cell-validity_days="{ value }">
            <span class="text-sm">{{ value }} {{ t('bundles.admin.days') }}</span>
          </template>
          <template #cell-status="{ value }">
            <span :class="statusBadgeClass(value)">{{ statusLabel(value) }}</span>
          </template>
          <template #cell-group_quotas="{ value }">
            <span class="text-sm">{{ value?.length ?? 0 }}</span>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex items-center gap-2">
              <button @click="openPlanEdit(row)" class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400">
                <Icon name="edit" size="sm" />
                <span class="text-xs">{{ t('common.edit') }}</span>
              </button>
              <button
                v-if="row.status === 'active'"
                @click="handleDisablePlan(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-yellow-50 hover:text-yellow-600 dark:hover:bg-yellow-900/20 dark:hover:text-yellow-400"
              >
                <Icon name="ban" size="sm" />
                <span class="text-xs">{{ t('bundles.admin.disable') }}</span>
              </button>
              <button
                v-else
                @click="handleEnablePlan(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-green-50 hover:text-green-600 dark:hover:bg-green-900/20 dark:hover:text-green-400"
              >
                <Icon name="check" size="sm" />
                <span class="text-xs">{{ t('bundles.admin.enable') }}</span>
              </button>
            </div>
          </template>
        </DataTable>
      </template>
    </TablePageLayout>

    <!-- Plan Edit Dialog -->
    <BaseDialog :show="showPlanDialog" :title="editingPlan ? t('bundles.admin.editPlan') : t('bundles.admin.createPlan')" width="extra-wide" @close="showPlanDialog = false">
      <form id="bundle-plan-form" @submit.prevent="handleSavePlan" class="space-y-4">
        <!-- Basic Info -->
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('bundles.admin.planName') }} <span class="text-red-500">*</span></label>
            <input v-model="planForm.name" type="text" class="input" required />
          </div>
          <div>
            <label class="input-label">{{ t('bundles.admin.tier') }} <span class="text-red-500">*</span></label>
            <Select v-model="planForm.tier" :options="tierOptions" :placeholder="t('bundles.admin.selectTier')" class="w-full" />
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('bundles.admin.planDescription') }}</label>
          <textarea v-model="planForm.description" rows="2" class="input"></textarea>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('bundles.admin.price') }} <span class="text-red-500">*</span></label>
            <input v-model.number="planForm.price" type="number" step="0.01" min="0.01" class="input" required />
          </div>
          <div>
            <label class="input-label">{{ t('bundles.admin.originalPrice') }}</label>
            <input v-model.number="planForm.original_price" type="number" step="0.01" min="0" class="input" />
          </div>
        </div>
        <div class="grid grid-cols-4 gap-4">
          <div>
            <label class="input-label">{{ t('bundles.admin.currency') }}</label>
            <Select v-model="planForm.currency" :options="currencyOptions" class="w-full" />
          </div>
          <div>
            <label class="input-label">{{ t('bundles.admin.validityDays') }} <span class="text-red-500">*</span></label>
            <input v-model.number="planForm.validity_days" type="number" min="1" class="input" required />
          </div>
          <div>
            <label class="input-label">{{ t('bundles.admin.sortOrder') }}</label>
            <input v-model.number="planForm.sort_order" type="number" min="0" class="input" />
          </div>
          <div>
            <label class="input-label">{{ t('bundles.admin.forSale') }}</label>
            <div class="mt-2 flex items-center gap-2">
              <button
                type="button"
                @click="planForm.for_sale = !planForm.for_sale"
                :class="[
                  'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                  planForm.for_sale ? 'bg-blue-600' : 'bg-gray-300 dark:bg-gray-600'
                ]"
              >
                <span
                  :class="[
                    'pointer-events-none inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                    planForm.for_sale ? 'translate-x-4' : 'translate-x-0'
                  ]"
                />
              </button>
              <span class="text-sm" :class="planForm.for_sale ? 'text-green-600 dark:text-green-400' : 'text-gray-400'">
                {{ planForm.for_sale ? t('bundles.admin.onSale') : t('bundles.admin.offSale') }}
              </span>
            </div>
          </div>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('bundles.admin.concurrencyLimit') }}</label>
            <input v-model.number="planForm.concurrency_limit" type="number" min="0" class="input" />
          </div>
          <div>
            <label class="input-label">{{ t('bundles.admin.rpmLimit') }}</label>
            <input v-model.number="planForm.rpm_limit" type="number" min="0" class="input" />
          </div>
        </div>

        <!-- Group Quotas Section -->
        <div>
          <div class="mb-2 flex items-center justify-between">
            <label class="input-label mb-0">{{ t('bundles.admin.groupQuotas') }}</label>
            <button type="button" @click="addGroupQuota" class="btn btn-secondary btn-sm">
              <Icon name="plus" size="sm" class="mr-1" />
              {{ t('bundles.admin.addGroup') }}
            </button>
          </div>

          <!-- Add Group Selector Row (when adding) -->
          <div v-if="showGroupSelector" class="mb-3 rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-800">
            <div class="flex items-center gap-3">
              <Select
                v-model="selectedNewGroupId"
                :options="availableGroupOptions"
                :placeholder="t('bundles.admin.selectGroup')"
                class="flex-1"
              />
              <button type="button" @click="confirmAddGroupQuota" :disabled="!selectedNewGroupId" class="btn btn-primary btn-sm">{{ t('common.confirm') }}</button>
              <button type="button" @click="showGroupSelector = false; selectedNewGroupId = null" class="btn btn-secondary btn-sm">{{ t('common.cancel') }}</button>
            </div>
          </div>

          <!-- Group Quota Rows -->
          <div v-if="planForm.group_quotas.length === 0" class="rounded-lg border border-dashed border-gray-300 p-6 text-center text-sm text-gray-400 dark:border-dark-600">
            {{ t('bundles.admin.noGroupQuotas') }}
          </div>
          <div v-for="(quota, idx) in planForm.group_quotas" :key="idx" class="mb-2 rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-600 dark:bg-dark-800">
            <div class="grid grid-cols-12 gap-3">
              <!-- Group Name -->
              <div class="col-span-3">
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.admin.group') }}</label>
                <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">
                  {{ getGroupName(quota.group_id) }}
                </div>
              </div>
              <!-- Quota Scope -->
              <div class="col-span-2">
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.admin.quotaScope') }}</label>
                <Select v-model="quota.quota_scope" :options="quotaScopeOptions" class="mt-1 w-full" @change="onQuotaScopeChange(quota)" />
              </div>
              <!-- Model Pattern (only for model scope) -->
              <div class="col-span-2">
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.admin.modelPattern') }}</label>
                <input
                  v-model="quota.model_pattern"
                  type="text"
                  class="input mt-1"
                  :disabled="quota.quota_scope !== 'model'"
                  :placeholder="quota.quota_scope === 'model' ? 'gpt-4*' : '-'"
                />
              </div>
              <!-- Daily / Weekly / Monthly USD Limits -->
              <div class="col-span-1">
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.admin.daily') }} ($)</label>
                <input v-model.number="quota.daily_limit_usd" type="number" step="0.01" min="0" class="input mt-1" />
              </div>
              <div class="col-span-1">
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.admin.weekly') }} ($)</label>
                <input v-model.number="quota.weekly_limit_usd" type="number" step="0.01" min="0" class="input mt-1" />
              </div>
              <div class="col-span-1">
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.admin.monthly') }} ($)</label>
                <input v-model.number="quota.monthly_limit_usd" type="number" step="0.01" min="0" class="input mt-1" />
              </div>
              <!-- Delete -->
              <div class="col-span-2 flex items-end justify-end">
                <button type="button" @click="removeGroupQuota(idx)" class="rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400">
                  <Icon name="trash" size="sm" />
                </button>
              </div>
            </div>
            <!-- Daily / Weekly / Monthly Count Limits (次) -->
            <div class="mt-2 grid grid-cols-12 gap-3 border-t border-gray-100 pt-2 dark:border-dark-700">
              <div class="col-span-3 text-xs text-gray-500 dark:text-gray-400">
                {{ t('bundles.admin.countLimitHint') }}
              </div>
              <div class="col-span-4"></div>
              <div class="col-span-1">
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.admin.daily') }} ({{ t('bundles.admin.countUnit') }})</label>
                <input v-model.number="quota.daily_limit_count" type="number" step="1" min="0" class="input mt-1" :placeholder="t('bundles.admin.countPlaceholder')" />
              </div>
              <div class="col-span-1">
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.admin.weekly') }} ({{ t('bundles.admin.countUnit') }})</label>
                <input v-model.number="quota.weekly_limit_count" type="number" step="1" min="0" class="input mt-1" :placeholder="t('bundles.admin.countPlaceholder')" />
              </div>
              <div class="col-span-1">
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.admin.monthly') }} ({{ t('bundles.admin.countUnit') }})</label>
                <input v-model.number="quota.monthly_limit_count" type="number" step="1" min="0" class="input mt-1" :placeholder="t('bundles.admin.countPlaceholder')" />
              </div>
              <div class="col-span-2"></div>
            </div>
          </div>
        </div>

        <!-- Features -->
        <div>
          <label class="input-label">{{ t('bundles.admin.features') }}</label>
          <textarea v-model="featuresText" rows="3" class="input" :placeholder="t('bundles.admin.featuresPlaceholder')"></textarea>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('bundles.admin.featuresHint') }}</p>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" @click="showPlanDialog = false" class="btn btn-secondary">{{ t('common.cancel') }}</button>
          <button type="submit" form="bundle-plan-form" :disabled="saving" class="btn btn-primary">{{ saving ? t('common.saving') : t('common.save') }}</button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { bundlesAPI } from '@/api/admin/bundles'
import { groupsAPI } from '@/api/admin/groups'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { BundlePlan, CreateGroupQuotaRequest } from '@/types/bundle'
import { getTierTheme, getTierI18nKey, getTierSelectOptions, type BundleTier } from '@/constants/bundleTiers'
import type { AdminGroup } from '@/types'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

// ==================== BundlePlansView：管理后台套餐计划管理页 ====================
// 提供套餐计划的 CRUD 界面，包括：
// - 计划列表表格（ID、名称、层级、价格、状态、渠道组数量）
// - 创建/编辑对话框（含渠道组额度的增删编辑）
// - 启用/停用操作

// ==================== 渠道组数据 ====================
// ==================== Groups ====================

const groups = ref<AdminGroup[]>([])

async function loadGroups() {
  try {
    groups.value = await groupsAPI.getAll()
  } catch { /* ignore */ }
}

function getGroupName(groupId: number): string {
  const group = groups.value.find(g => g.id === groupId)
  return group ? `${group.name} (${group.platform})` : `#${groupId}`
}

const subscriptionGroupOptions = computed(() =>
  groups.value
    .filter(g => g.subscription_type === 'subscription')
    .map(g => ({
      value: g.id,
      label: `${g.name} — ${g.platform}`,
    }))
)

// Groups not yet added to the form
const availableGroupOptions = computed(() => {
  const usedIds = new Set(planForm.group_quotas.map(q => q.group_id))
  return subscriptionGroupOptions.value.filter(opt => !usedIds.has(opt.value as number))
})

// ==================== 套餐计划 ====================
// ==================== Plans ====================

const plansLoading = ref(false)
const plans = ref<BundlePlan[]>([])
const showPlanDialog = ref(false)
const editingPlan = ref<BundlePlan | null>(null)
const saving = ref(false)

// Group quota editor state
const showGroupSelector = ref(false)
const selectedNewGroupId = ref<number | null>(null)

interface PlanFormData {
  name: string
  description: string
  tier: string | null
  price: number
  original_price: number
  currency: string | null
  validity_days: number
  concurrency_limit: number
  rpm_limit: number
  sort_order: number
  for_sale: boolean
  features: string[]
  group_quotas: CreateGroupQuotaRequest[]
}

const planForm = reactive<PlanFormData>({
  name: '',
  description: '',
  tier: null,
  price: 0,
  original_price: 0,
  currency: 'USD',
  validity_days: 30,
  concurrency_limit: 0,
  rpm_limit: 0,
  sort_order: 0,
  for_sale: true,
  features: [],
  group_quotas: [],
})

const featuresText = ref('')

const planColumns = computed((): Column[] => [
  { key: 'id', label: 'ID' },
  { key: 'name', label: t('bundles.admin.planName') },
  { key: 'tier', label: t('bundles.admin.tier') },
  { key: 'price', label: t('bundles.admin.price') },
  { key: 'validity_days', label: t('bundles.admin.validityDays') },
  { key: 'status', label: t('bundles.admin.status') },
  { key: 'group_quotas', label: t('bundles.admin.groupCount') },
  { key: 'actions', label: t('common.actions') },
])

const tierOptions = computed(() => getTierSelectOptions(t))

const currencyOptions = computed(() => [
  { value: 'USD', label: 'USD' },
  { value: 'CNY', label: 'CNY' },
])

const quotaScopeOptions = computed(() => [
  { value: 'platform', label: t('bundles.admin.scopePlatform') },
  { value: 'model', label: t('bundles.admin.scopeModel') },
])

function tierBadgeClass(tier: string): string {
  return getTierTheme(tier).adminBadgeClass
}

function tierLabel(tier: string): string {
  return t(getTierI18nKey(tier, 'admin'))
}

function statusBadgeClass(status: string): string {
  const base = 'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium'
  switch (status) {
    case 'active': return `${base} bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300`
    case 'disabled': return `${base} bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400`
    default: return `${base} bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300`
  }
}

function statusLabel(status: string): string {
  switch (status) {
    case 'active': return t('bundles.admin.statusActive')
    case 'disabled': return t('bundles.admin.statusDisabled')
    default: return status
  }
}

async function loadPlans() {
  plansLoading.value = true
  try {
    const res = await bundlesAPI.listPlans()
    plans.value = res.items || []
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    plansLoading.value = false
  }
}

function openPlanEdit(plan: BundlePlan | null) {
  editingPlan.value = plan
  showGroupSelector.value = false
  selectedNewGroupId.value = null

  if (plan) {
    Object.assign(planForm, {
      name: plan.name,
      description: plan.description || '',
      tier: plan.tier,
      price: plan.price,
      original_price: plan.original_price || 0,
      currency: plan.currency || 'USD',
      validity_days: plan.validity_days,
      concurrency_limit: plan.concurrency_limit || 0,
      rpm_limit: plan.rpm_limit || 0,
      sort_order: plan.sort_order || 0,
      for_sale: plan.for_sale ?? true,
      features: plan.features || [],
      group_quotas: (plan.group_quotas || []).map(q => ({
        group_id: q.group_id,
        quota_scope: q.quota_scope,
        model_pattern: q.model_pattern || '',
        daily_limit_usd: q.daily_limit_usd || 0,
        weekly_limit_usd: q.weekly_limit_usd || 0,
        monthly_limit_usd: q.monthly_limit_usd || 0,
        daily_limit_count: q.daily_limit_count || 0,
        weekly_limit_count: q.weekly_limit_count || 0,
        monthly_limit_count: q.monthly_limit_count || 0,
      })),
    })
    featuresText.value = (plan.features || []).join('\n')
  } else {
    Object.assign(planForm, {
      name: '',
      description: '',
      tier: null,
      price: 0,
      original_price: 0,
      currency: 'USD',
      validity_days: 30,
      concurrency_limit: 0,
      rpm_limit: 0,
      sort_order: 0,
      for_sale: true,
      features: [],
      group_quotas: [],
    })
    featuresText.value = ''
  }

  showPlanDialog.value = true
}

// ==================== 渠道组额度编辑 ====================
// ==================== Group Quota Editing ====================

function addGroupQuota() {
  selectedNewGroupId.value = null
  showGroupSelector.value = true
}

function confirmAddGroupQuota() {
  if (!selectedNewGroupId.value) return
  planForm.group_quotas.push({
    group_id: selectedNewGroupId.value,
    quota_scope: 'platform',
    model_pattern: '',
    daily_limit_usd: 0,
    weekly_limit_usd: 0,
    monthly_limit_usd: 0,
    daily_limit_count: 0,
    weekly_limit_count: 0,
    monthly_limit_count: 0,
  })
  showGroupSelector.value = false
  selectedNewGroupId.value = null
}

function removeGroupQuota(idx: number) {
  planForm.group_quotas.splice(idx, 1)
}

function onQuotaScopeChange(quota: CreateGroupQuotaRequest) {
  if (quota.quota_scope === 'platform') {
    quota.model_pattern = ''
  }
}

// ==================== 保存操作 ====================
// ==================== Save ====================

function buildPayload() {
  const features = featuresText.value.split('\n').map(f => f.trim()).filter(Boolean)
  return {
    name: planForm.name,
    description: planForm.description || undefined,
    tier: planForm.tier as BundleTier,
    price: planForm.price,
    original_price: planForm.original_price || undefined,
    currency: (planForm.currency || 'USD') as 'USD' | 'CNY',
    validity_days: planForm.validity_days,
    concurrency_limit: planForm.concurrency_limit || undefined,
    rpm_limit: planForm.rpm_limit || undefined,
    sort_order: planForm.sort_order,
    for_sale: planForm.for_sale,
    features,
    group_quotas: planForm.group_quotas,
  }
}

async function handleSavePlan() {
  if (!planForm.tier) {
    appStore.showError(t('bundles.admin.tierRequired'))
    return
  }
  if (!planForm.price || planForm.price <= 0) {
    appStore.showError(t('bundles.admin.priceRequired'))
    return
  }
  if (!planForm.validity_days || planForm.validity_days < 1) {
    appStore.showError(t('bundles.admin.validityDaysRequired'))
    return
  }

  saving.value = true
  try {
    const data = buildPayload()
    if (editingPlan.value) {
      await bundlesAPI.updatePlan(editingPlan.value.id, data)
    } else {
      await bundlesAPI.createPlan(data)
    }
    appStore.showSuccess(t('common.saved'))
    showPlanDialog.value = false
    loadPlans()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    saving.value = false
  }
}

// ==================== 启用 / 停用 ====================
// ==================== Enable / Disable ====================

async function handleDisablePlan(plan: BundlePlan) {
  try {
    await bundlesAPI.updatePlan(plan.id, { status: 'disabled' } as any)
    plan.status = 'disabled'
    appStore.showSuccess(t('common.saved'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  }
}

async function handleEnablePlan(plan: BundlePlan) {
  try {
    await bundlesAPI.updatePlan(plan.id, { status: 'active' } as any)
    plan.status = 'active'
    appStore.showSuccess(t('common.saved'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  }
}

// ==================== 生命周期 ====================
// ==================== Lifecycle ====================

onMounted(() => {
  loadGroups()
  loadPlans()
})
</script>
