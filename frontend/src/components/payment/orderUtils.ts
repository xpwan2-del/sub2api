/**
 * Shared utility functions for payment order display.
 * Used by AdminOrderDetail, AdminOrderTable, AdminRefundDialog, AdminOrdersView, etc.
 */

const STATUS_BADGE_MAP: Record<string, string> = {
  PENDING: 'badge-warning',
  PAID: 'badge-info',
  RECHARGING: 'badge-info',
  COMPLETED: 'badge-success',
  EXPIRED: 'badge-secondary',
  CANCELLED: 'badge-secondary',
  FAILED: 'badge-danger',
  REFUND_REQUESTED: 'badge-warning',
  REFUNDING: 'badge-warning',
  REFUND_PENDING: 'badge-warning',
  PARTIALLY_REFUNDED: 'badge-warning',
  REFUNDED: 'badge-info',
  REFUND_FAILED: 'badge-danger',
}

const REFUNDABLE_STATUSES = ['COMPLETED', 'PARTIALLY_REFUNDED', 'REFUND_REQUESTED', 'REFUND_FAILED']

export function statusBadgeClass(status: string): string {
  return STATUS_BADGE_MAP[status] || 'badge-secondary'
}

export function canRefund(status: string): boolean {
  return REFUNDABLE_STATUSES.includes(status)
}

export function formatOrderDateTime(dateStr: string): string {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleString()
}

// 订单类型 badge 样式 —— OrderTable 列、admin/user 详情弹窗共享同一份真值表。
const ORDER_TYPE_BADGE_BASE =
  'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium'
const ORDER_TYPE_BADGE_CLASS: Record<string, string> = {
  balance: 'bg-sky-100 text-sky-800 dark:bg-sky-900/30 dark:text-sky-300',
  subscription: 'bg-indigo-100 text-indigo-800 dark:bg-indigo-900/30 dark:text-indigo-300',
  bundle: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-300',
  bundle_upgrade: 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300',
}

/** 返回订单类型 badge 的 Tailwind class（含未知类型的灰色兜底）。 */
export function orderTypeBadgeClass(type: string): string {
  return `${ORDER_TYPE_BADGE_BASE} ${ORDER_TYPE_BADGE_CLASS[type] ?? 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300'}`
}

// 订单类型 → 本地化标签的 i18n key。调用方用 t(orderTypeLabelKey(type), type) 渲染，
// 第二参数作为未知类型的回退（与 payment_type 的 t('payment.methods.'+v, v) 模式一致）。
const ORDER_TYPE_LABEL_KEY: Record<string, string> = {
  balance: 'payment.admin.balanceOrder',
  subscription: 'payment.admin.subscriptionOrder',
  bundle: 'payment.admin.bundleOrder',
  bundle_upgrade: 'payment.admin.bundleUpgradeOrder',
}

/** 返回订单类型本地化标签的 i18n key；未知类型返回空串（交由 t 的回退参数处理）。 */
export function orderTypeLabelKey(type: string): string {
  return ORDER_TYPE_LABEL_KEY[type] ?? ''
}
