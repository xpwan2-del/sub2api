import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put, del } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  del: vi.fn(),
}))

vi.mock('../client', () => ({
  apiClient: { get, post, put, delete: del },
}))

import {
  createSource,
  getRequest,
  listRequests,
  reviewItem,
  type UpstreamSourceConfig,
} from '@/api/admin/upstreamPriceSync'

describe('upstream price sync API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
    del.mockReset()
  })

  it('creates a source with group ratio exclusions', async () => {
    const source: UpstreamSourceConfig = {
      name: 'source',
      base_url: 'https://upstream.example',
      api_key: '',
      target_channel_id: 7,
      enabled: true,
      base_price_per_1k: 0.002,
      pricing_source: 'auto',
      sync_model_price: true,
      sync_group_ratio: true,
      target_upstream_group: 'default',
      excluded_group_ids: [3],
    }
    post.mockResolvedValue({ data: { ...source, id: 1 } })

    await createSource(source)

    expect(post).toHaveBeenCalledWith('/admin/channels/upstream-sources', source)
  })

  it('passes pagination and source filters to request list', async () => {
    get.mockResolvedValue({ data: { items: [], total: 0, page: 2, page_size: 10, pages: 0 } })

    await listRequests({ status: 'open', page: 2, page_size: 10 })

    expect(get).toHaveBeenCalledWith('/admin/channels/price-change-requests', {
      params: { status: 'open', page: 2, page_size: 10 },
    })
  })

  it('uses the flat request detail contract', async () => {
    const detail = { id: 4, source_config_id: 1, status: 'open', summary: {}, created_at: '', items: [] }
    get.mockResolvedValue({ data: detail })

    await expect(getRequest(4)).resolves.toEqual(detail)
    expect(get).toHaveBeenCalledWith('/admin/channels/price-change-requests/4')
  })

  it('sends edited apply_rate for a group ratio item', async () => {
    post.mockResolvedValue({ data: { reviewed: 8 } })

    await reviewItem(4, 8, { action: 'apply', apply_rate: 0.8357 })

    expect(post).toHaveBeenCalledWith(
      '/admin/channels/price-change-requests/4/items/8/review',
      { action: 'apply', apply_rate: 0.8357 },
    )
  })
})
