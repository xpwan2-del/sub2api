<template>
  <DataTable :columns="columns" :data="orders" :loading="loading">
    <template #cell-id="{ value }">
      <span class="font-mono text-sm">#{{ value }}</span>
    </template>
    <template #cell-out_trade_no="{ value }">
      <span class="text-sm text-gray-900 dark:text-white">{{ value }}</span>
    </template>
    <template v-if="showUser" #cell-user_email="{ value, row }">
      <div class="text-sm">
        <span class="text-gray-900 dark:text-white">{{ value || row.user_name || '#' + row.user_id }}</span>
        <span v-if="row.user_notes" class="ml-1 text-xs text-gray-400">({{ row.user_notes }})</span>
      </div>
    </template>
    <template #cell-pay_amount="{ value, row }">
      <div class="text-sm">
        <span class="font-medium text-gray-900 dark:text-white">{{ paymentAmountSymbol(row) }}{{ value.toFixed(2) }}</span>
        <span v-if="row.fee_rate > 0" class="ml-1 text-xs text-gray-400" :title="t('payment.orders.fee') + ': ' + row.fee_rate + '%'">
          ({{ t('payment.orders.fee') }} {{ row.fee_rate }}%)
        </span>
        <div v-if="(row.balance_deduct_amount ?? 0) > 0" class="text-xs text-gray-500">
          {{ t('payment.orders.balanceDeductAmount') }}: {{ creditedAmountSymbol }}{{ row.balance_deduct_amount.toFixed(2) }}
        </div>
        <div v-else-if="row.amount !== row.pay_amount" class="text-xs text-gray-500">
          {{ t('payment.orders.amount') }}: {{ creditedAmountSymbol }}{{ row.amount.toFixed(2) }}
        </div>
      </div>
    </template>
    <template #cell-order_type="{ value }">
      <span :class="orderTypeClass(value)">
        {{ orderTypeLabel(value) }}
      </span>
    </template>
    <template #cell-payment_type="{ value }">
      <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.methods.' + value, value) }}</span>
    </template>
    <template #cell-status="{ value }">
      <OrderStatusBadge :status="value" />
    </template>
    <template #cell-created_at="{ value }">
      <span class="text-xs text-gray-500 dark:text-gray-400">{{ formatDate(value) }}</span>
    </template>
    <template #cell-actions="{ row }">
      <slot name="actions" :row="row" />
    </template>
  </DataTable>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PaymentOrder } from '@/types/payment'
import type { Column } from '@/components/common/types'
import DataTable from '@/components/common/DataTable.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'
import { currencySymbol } from '@/components/payment/currency'

const { t } = useI18n()

const props = defineProps<{
  orders: PaymentOrder[]
  loading: boolean
  showUser?: boolean
}>()

function formatDate(dateStr: string) { return new Date(dateStr).toLocaleString() }

const creditedAmountSymbol = currencySymbol('USD')

function paymentAmountSymbol(order: PaymentOrder): string {
  return currencySymbol(order.currency)
}

function orderTypeLabel(type: string): string {
  switch (type) {
    case 'balance': return t('payment.admin.balanceOrder')
    case 'subscription': return t('payment.admin.subscriptionOrder')
    case 'bundle': return t('payment.admin.bundleOrder')
    case 'bundle_upgrade': return t('payment.admin.bundleUpgradeOrder')
    default: return type
  }
}

function orderTypeClass(type: string): string {
  const base = 'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium'
  switch (type) {
    case 'balance': return `${base} bg-sky-100 text-sky-800 dark:bg-sky-900/30 dark:text-sky-300`
    case 'subscription': return `${base} bg-indigo-100 text-indigo-800 dark:bg-indigo-900/30 dark:text-indigo-300`
    case 'bundle': return `${base} bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-300`
    case 'bundle_upgrade': return `${base} bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300`
    default: return `${base} bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300`
  }
}

const columns = computed((): Column[] => {
  const cols: Column[] = [
    { key: 'id', label: t('payment.orders.orderId') },
    { key: 'out_trade_no', label: t('payment.orders.orderNo') },
  ]
  if (props.showUser) {
    cols.push({ key: 'user_email', label: t('payment.admin.colUser') })
  }
  cols.push(
    { key: 'pay_amount', label: t('payment.orders.payAmount') },
    { key: 'order_type', label: t('payment.orders.orderType') },
    { key: 'payment_type', label: t('payment.orders.paymentMethod') },
    { key: 'status', label: t('payment.orders.status') },
    { key: 'created_at', label: t('payment.orders.createdAt') },
    { key: 'actions', label: t('common.actions') },
  )
  return cols
})
</script>
