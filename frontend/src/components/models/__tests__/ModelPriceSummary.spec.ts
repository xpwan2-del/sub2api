import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelPriceSummary from '@/components/models/ModelPriceSummary.vue'
import type { PublicModelPricing } from '@/api/publicModels'

// Mock vue-i18n: return the i18n key verbatim so missing translations don't break the test.
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

function makePricing(overrides: Partial<PublicModelPricing> = {}): PublicModelPricing {
  return {
    billing_mode: 'token',
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

function mountSummary(pricing: PublicModelPricing | null) {
  return mount(ModelPriceSummary, { props: { pricing } })
}

describe('ModelPriceSummary — token price scaling', () => {
  it('scales image_output_price per 1M tokens, identical to input/output', () => {
    // image_output_price is a per-token USD value (backend: ImageOutputPricePerToken),
    // so it must be displayed per 1M tokens just like input_price/output_price.
    const wrapper = mountSummary(
      makePricing({ image_output_price: 0.000003 }) // $3 per 1M tokens
    )
    const text = wrapper.text()

    // 0.000003 * 1_000_000 === 3 → "$3"
    expect(text).toContain('$3')
    // The raw per-token value must NOT leak through (that is the bug).
    expect(text).not.toContain('$0.000003')
  })

  it('scales input/output/image consistently to the same per-1M unit', () => {
    const wrapper = mountSummary(
      makePricing({
        input_price: 0.000003,        // $3 / 1M
        output_price: 0.000015,       // $15 / 1M
        image_output_price: 0.000007, // $7 / 1M (unique — not masked by input/output)
      })
    )
    const text = wrapper.text()

    expect(text).toContain('$3')   // input
    expect(text).toContain('$15')  // output
    expect(text).toContain('$7')   // image — same per-1M scale as the others
    expect(text).not.toContain('$0.000007')
  })
})
