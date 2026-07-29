import { describe, expect, it } from 'vitest'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_PER_SECOND,
  BILLING_MODE_TOKEN,
  BILLING_MODE_VIDEO,
  getModelCatalogBillingModeLabel
} from '../billingMode'

// identity mock：返回 key 本身，便于断言命中了哪个 i18n key
const t = (key: string) => key

describe('getModelCatalogBillingModeLabel', () => {
  it('把每个规范计费模式映射到对应的 modelCatalog i18n key', () => {
    expect(getModelCatalogBillingModeLabel(BILLING_MODE_TOKEN, t)).toBe('modelCatalog.billingModes.token')
    expect(getModelCatalogBillingModeLabel(BILLING_MODE_PER_REQUEST, t)).toBe('modelCatalog.billingModes.per_request')
    expect(getModelCatalogBillingModeLabel(BILLING_MODE_IMAGE, t)).toBe('modelCatalog.billingModes.image')
    expect(getModelCatalogBillingModeLabel(BILLING_MODE_VIDEO, t)).toBe('modelCatalog.billingModes.video')
    expect(getModelCatalogBillingModeLabel(BILLING_MODE_PER_SECOND, t)).toBe('modelCatalog.billingModes.per_second')
  })

  it('mode 为空/null/undefined 时回退到 unknown 文案键', () => {
    expect(getModelCatalogBillingModeLabel('', t)).toBe('modelCatalog.billingModes.unknown')
    expect(getModelCatalogBillingModeLabel(null, t)).toBe('modelCatalog.billingModes.unknown')
    expect(getModelCatalogBillingModeLabel(undefined, t)).toBe('modelCatalog.billingModes.unknown')
  })

  it('未知但非空的 mode 原样返回（便于运营发现脏数据/新模式）', () => {
    expect(getModelCatalogBillingModeLabel('future_mode', t)).toBe('future_mode')
  })
})
