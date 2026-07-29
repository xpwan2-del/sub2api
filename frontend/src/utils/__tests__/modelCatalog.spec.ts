import { describe, expect, it } from 'vitest'
import {
  TAG_WEIGHTS,
  buildModelCatalog,
  filterModelCatalog,
  tagScore
} from '../modelCatalog'
import type { ModelCatalogCard } from '../modelCatalog'
import type { PublicModelCatalogItem, PublicModelPricing } from '@/api/publicModels'

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

function pricing(inputPrice: number): PublicModelPricing {
  return {
    billing_mode: 'token',
    input_price: inputPrice,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: []
  }
}

function makeCard(overrides: Partial<ModelCatalogCard> & Pick<ModelCatalogCard, 'name'>): ModelCatalogCard {
  return {
    id: overrides.name.toLowerCase(),
    provider: 'any',
    description: '',
    platforms: [],
    status: 'operational',
    pricing: null,
    health: null,
    capabilities: [],
    pinned: false,
    sort_weight: 0,
    tags: [],
    is_new: false,
    featured: false,
    ...overrides
  }
}

function sortByRecommended(items: ModelCatalogCard[]): ModelCatalogCard[] {
  return filterModelCatalog(items, {
    search: '',
    platform: '',
    capability: '',
    billingMode: '',
    sortBy: 'recommended'
  })
}

describe('TAG_WEIGHTS + tagScore', () => {
  it('sums tag weights, with new+multimodal (140) outranking featured (80)', () => {
    expect(tagScore(['new', 'multimodal'])).toBe(TAG_WEIGHTS.new + TAG_WEIGHTS.multimodal)
    expect(tagScore(['new', 'multimodal'])).toBe(140)
    expect(tagScore(['featured'])).toBe(80)
    expect(tagScore(['new', 'multimodal'])).toBeGreaterThan(tagScore(['featured']))
  })

  it('is resilient to empty/undefined/unknown tags', () => {
    expect(tagScore([])).toBe(0)
    expect(tagScore(undefined)).toBe(0)
    expect(tagScore(['totally-unknown-tag'])).toBe(0)
  })
})

describe('recommended sort', () => {
  it('puts pinned cards first, and within the pinned block orders by sort_weight desc', () => {
    const items = [
      makeCard({ name: 'pinned-low', pinned: true, sort_weight: 10 }),
      makeCard({ name: 'floating', tags: ['new', 'multimodal'] }),
      makeCard({ name: 'pinned-high', pinned: true, sort_weight: 20 })
    ]

    expect(sortByRecommended(items).map((item) => item.name)).toEqual([
      'pinned-high',
      'pinned-low',
      'floating'
    ])
  })

  it('ranks non-pinned cards by sort_weight when set (sort_weight beats tagScore)', () => {
    const items = [
      makeCard({ name: 'high-tag-zero-weight', tags: ['new', 'multimodal'], sort_weight: 0 }),
      makeCard({ name: 'low-tag-high-weight', tags: ['fast'], sort_weight: 50 })
    ]

    expect(sortByRecommended(items).map((item) => item.name)).toEqual([
      'low-tag-high-weight',
      'high-tag-zero-weight'
    ])
  })

  it('ranks non-pinned cards by accumulated tag weight (new+multimodal > featured)', () => {
    const items = [
      makeCard({ name: 'featured-only', tags: ['featured'] }),
      makeCard({ name: 'new-and-multimodal', tags: ['new', 'multimodal'] })
    ]

    expect(sortByRecommended(items).map((item) => item.name)).toEqual([
      'new-and-multimodal',
      'featured-only'
    ])
  })

  it('falls back to ascending price when tag weights tie', () => {
    const items = [
      makeCard({ name: 'pricier', tags: ['fast'], pricing: pricing(2) }),
      makeCard({ name: 'cheaper', tags: ['fast'], pricing: pricing(1) })
    ]

    expect(sortByRecommended(items).map((item) => item.name)).toEqual(['cheaper', 'pricier'])
  })

  it('falls back to name when tag weight and price both tie', () => {
    const items = [makeCard({ name: 'bravo' }), makeCard({ name: 'alpha' })]

    expect(sortByRecommended(items).map((item) => item.name)).toEqual(['alpha', 'bravo'])
  })
})
