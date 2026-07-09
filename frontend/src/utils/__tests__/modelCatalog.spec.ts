import { describe, expect, it } from 'vitest'
import { buildModelCatalog, filterModelCatalog } from '../modelCatalog'
import type { PublicModelCatalogItem } from '@/api/publicModels'

describe('modelCatalog utils', () => {
  const rows: PublicModelCatalogItem[] = [
    {
      name: 'gpt-4o-mini',
      provider: 'OpenAI',
      platform: 'openai',
      status: 'available',
      pricing: {
        billing_mode: 'token',
        input_price: 0.00000015,
        output_price: 0.0000006,
        cache_write_price: null,
        cache_read_price: null,
        image_output_price: null,
        per_request_price: null,
        intervals: []
      }
    },
    {
      name: 'gpt-4o-mini',
      provider: 'OpenAI',
      platform: 'openai',
      status: 'available',
      pricing: {
        billing_mode: 'token',
        input_price: 0.0000003,
        output_price: 0.0000008,
        cache_write_price: null,
        cache_read_price: null,
        image_output_price: null,
        per_request_price: null,
        intervals: []
      }
    },
    {
      name: 'claude-sonnet-4',
      provider: 'Claude',
      platform: 'anthropic',
      status: 'available',
      pricing: null
    }
  ]

  it('groups duplicated model rows and keeps the cheapest public price', () => {
    const catalog = buildModelCatalog(rows)

    expect(catalog.items).toHaveLength(2)
    expect(catalog.items.find((item) => item.name === 'gpt-4o-mini')?.pricing?.input_price).toBe(0.00000015)
  })

  it('builds filters only from returned models', () => {
    const catalog = buildModelCatalog(rows)

    expect(catalog.facets.platforms).toEqual(['anthropic', 'openai'])
    expect(catalog.facets.capabilities).toContain('coding')
    expect(catalog.facets.billingModes).toEqual(['token'])
  })

  it('filters by platform and search text', () => {
    const catalog = buildModelCatalog(rows)

    const filtered = filterModelCatalog(catalog.items, {
      search: 'claude',
      platform: 'anthropic',
      capability: '',
      billingMode: '',
      sortBy: 'name'
    })

    expect(filtered.map((item) => item.name)).toEqual(['claude-sonnet-4'])
  })

  it('filters by inferred capability without showing unavailable models', () => {
    const catalog = buildModelCatalog(rows)

    const filtered = filterModelCatalog(catalog.items, {
      search: '',
      platform: '',
      capability: 'lowCost',
      billingMode: '',
      sortBy: 'name'
    })

    expect(filtered.map((item) => item.name)).toEqual(['gpt-4o-mini'])
  })
})
