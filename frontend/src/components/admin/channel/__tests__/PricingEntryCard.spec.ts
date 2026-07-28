import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PricingEntryCard from '@/components/admin/channel/PricingEntryCard.vue'
import type { PricingFormEntry } from '@/components/admin/channel/types'

// Mock vue-i18n: return the i18n key verbatim so missing translations (Task 8) don't break the test.
// Preserve other exports (createI18n etc.) because transitive imports (i18n/index.ts via api/client.ts) need them.
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

// Stub Select to capture the `options` prop without rendering its full dropdown.
const SelectStub = {
  name: 'Select',
  props: ['modelValue', 'options'],
  template: '<div class="select-stub"></div>',
}

function makeEntry(overrides: Partial<PricingFormEntry> = {}): PricingFormEntry {
  return {
    models: [],
    billing_mode: 'per_second',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: [],
    ...overrides,
  }
}

function mountCard(entry: PricingFormEntry) {
  return mount(PricingEntryCard, {
    props: { entry, platform: 'openai' },
    global: {
      stubs: {
        Select: SelectStub,
        Icon: true,
        ModelTagInput: true,
        IntervalRow: true,
      },
    },
  })
}

describe('PricingEntryCard — per_second billing mode', () => {
  it('exposes the per_second option in billingModeOptions', () => {
    const wrapper = mountCard(makeEntry({ billing_mode: 'per_second' }))
    const select = wrapper.findComponent({ name: 'Select' })
    const options = (select.props('options') || []) as { value: string; label: string }[]
    expect(options.map((o) => o.value)).toContain('per_second')
  })

  it('renders the per-second price input area when billing_mode is per_second', () => {
    const wrapper = mountCard(makeEntry({ billing_mode: 'per_second' }))
    const text = wrapper.text()
    // Default per-second price label (mirrors video block's defaultVideoPrice)
    expect(text).toContain('admin.channels.form.defaultPerSecondPrice')
    // Unit label is USD/s (not $)
    expect(text).toContain('USD/s')
    // Per-second tier block label (mirrors videoTiers)
    expect(text).toContain('admin.channels.form.perSecondTiers')
  })
})

describe('IntervalRow — per_second unit copy', () => {
  it('shows per-second price (USD/s) label when mode is per_second', async () => {
    const { default: IntervalRow } = await import('../IntervalRow.vue')
    const wrapper = mount(IntervalRow, {
      props: {
        interval: {
          min_tokens: 0,
          max_tokens: null,
          tier_label: '480P',
          input_price: null,
          output_price: null,
          cache_write_price: null,
          cache_read_price: null,
          per_request_price: null,
          sort_order: 0,
        },
        mode: 'per_second',
      },
      global: {
        stubs: {
          Select: SelectStub,
          Icon: true,
        },
      },
    })
    const text = wrapper.text()
    expect(text).toContain('admin.channels.form.perSecondPrice')
    expect(text).toContain('USD/s')
    // Should NOT show the per-request copy
    expect(text).not.toContain('admin.channels.form.perRequestPrice')
  })
})
