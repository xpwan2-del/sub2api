import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

describe('bundle subscription status locale keys', () => {
  it('contains zh labels for all subscription statuses', () => {
    expect(zh.bundles.admin.statusActive).toBe('启用')
    expect(zh.bundles.admin.statusExpired).toBe('已过期')
    expect(zh.bundles.admin.statusRevoked).toBe('已撤销')
    expect(zh.bundles.admin.statusUpgraded).toBe('已升级')
  })

  it('contains en labels for all subscription statuses', () => {
    expect(en.bundles.admin.statusActive).toBe('Active')
    expect(en.bundles.admin.statusExpired).toBe('Expired')
    expect(en.bundles.admin.statusRevoked).toBe('Revoked')
    expect(en.bundles.admin.statusUpgraded).toBe('Upgraded')
  })
})
