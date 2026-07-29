export const BILLING_MODE_TOKEN = 'token'
export const BILLING_MODE_PER_REQUEST = 'per_request'
export const BILLING_MODE_IMAGE = 'image'
export const BILLING_MODE_VIDEO = 'video'
export const BILLING_MODE_PER_SECOND = 'per_second'

export function getBillingModeLabel(mode: string | null | undefined, t: (key: string) => string): string {
  switch (mode) {
    case BILLING_MODE_PER_REQUEST: return t('admin.usage.billingModePerRequest')
    case BILLING_MODE_IMAGE: return t('admin.usage.billingModeImage')
    case BILLING_MODE_VIDEO: return t('admin.usage.billingModeVideo')
    case BILLING_MODE_PER_SECOND: return t('admin.usage.billingModePerSecond')
    default: return t('admin.usage.billingModeToken')
  }
}

export function getBillingModeBadgeClass(mode: string | null | undefined): string {
  switch (mode) {
    case BILLING_MODE_PER_REQUEST: return 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300'
    case BILLING_MODE_IMAGE: return 'bg-pink-100 text-pink-700 dark:bg-pink-900/30 dark:text-pink-300'
    case BILLING_MODE_VIDEO: return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    case BILLING_MODE_PER_SECOND: return 'bg-teal-100 text-teal-700 dark:bg-teal-900/30 dark:text-teal-300'
    default: return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
  }
}

/** 模型广场已知的计费模式集合（与后端 service.BillingMode* 一致）。 */
const CATALOG_BILLING_MODES = new Set<string>([
  BILLING_MODE_TOKEN,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_IMAGE,
  BILLING_MODE_VIDEO,
  BILLING_MODE_PER_SECOND,
])

/**
 * 模型广场计费模式标签（单一真相源）。
 * - 已知模式：走 modelCatalog.billingModes.<mode> 文案
 * - 空/null/undefined：回退 modelCatalog.billingModes.unknown（"价格待配置"）
 * - 未知非空：原样返回，便于发现新脏数据/未补文案的新模式
 */
export function getModelCatalogBillingModeLabel(
  mode: string | null | undefined,
  t: (key: string) => string,
): string {
  if (mode && CATALOG_BILLING_MODES.has(mode)) {
    return t(`modelCatalog.billingModes.${mode}`)
  }
  return mode || t('modelCatalog.billingModes.unknown')
}

interface ImageBillingRow {
  image_count: number
  billing_mode?: string | null
  total_cost: number
}

export function isImageUsage(row: Pick<ImageBillingRow, 'image_count' | 'billing_mode'> | null | undefined): boolean {
  return (row?.image_count ?? 0) > 0 && row?.billing_mode !== BILLING_MODE_TOKEN && row?.billing_mode !== BILLING_MODE_VIDEO && row?.billing_mode !== BILLING_MODE_PER_SECOND
}

export function getDisplayBillingMode(row: Pick<ImageBillingRow, 'billing_mode' | 'image_count'> | null | undefined): string | null | undefined {
  if ((row?.image_count ?? 0) > 0 && !row?.billing_mode) {
    return BILLING_MODE_IMAGE
  }
  return row?.billing_mode
}

export function imageUnitPrice(row: Pick<ImageBillingRow, 'image_count' | 'total_cost'> | null): number {
  if (!row || row.image_count <= 0) return 0
  const total = row.total_cost ?? 0
  const price = total / row.image_count
  return Number.isFinite(price) ? price : 0
}
