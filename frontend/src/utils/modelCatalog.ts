import type {
  PublicModelCatalogItem,
  PublicModelHealth,
  PublicModelHealthHistoryPoint,
  PublicModelPricing
} from '@/api/publicModels'

export type ModelCatalogSort = 'price' | 'name' | 'provider'

export interface ModelCatalogCard {
  id: string
  name: string
  provider: string
  description: string
  platforms: string[]
  status: string
  pricing: PublicModelPricing | null
  health: PublicModelHealth | null
  capabilities: string[]
}

export interface ModelCatalogFilters {
  search: string
  platform: string
  capability: string
  billingMode: string
  sortBy: ModelCatalogSort
}

export interface ModelCatalogFacets {
  platforms: string[]
  capabilities: string[]
  billingModes: string[]
  sortOptions: ModelCatalogSort[]
}

export interface ModelCatalogBuildResult {
  items: ModelCatalogCard[]
  facets: ModelCatalogFacets
}

const capabilityOrder = ['reasoning', 'coding', 'longContext', 'lowCost', 'multimodal', 'fast']

export function buildModelCatalog(rows: PublicModelCatalogItem[]): ModelCatalogBuildResult {
  const grouped = new Map<string, ModelCatalogCard>()

  for (const row of rows) {
    const name = row.name?.trim()
    if (!name) continue

    const key = name.toLowerCase()
    const capabilities = normalizeCapabilities(row.capabilities?.length ? row.capabilities : inferModelCapabilities(row))
    const current = grouped.get(key)

    if (!current) {
      grouped.set(key, {
        id: key.replace(/[^a-z0-9._-]+/g, '-'),
        name,
        provider: row.provider || row.platform,
        description: row.description?.trim() || modelDescriptionFromCapabilities(capabilities),
        platforms: row.platform ? [row.platform] : [],
        status: row.status,
        pricing: row.pricing,
        health: cloneModelHealth(row.health),
        capabilities
      })
      continue
    }

    current.provider = current.provider || row.provider || row.platform
    if (!current.description && row.description?.trim()) {
      current.description = row.description.trim()
    }
    if (row.platform && !current.platforms.includes(row.platform)) {
      current.platforms.push(row.platform)
    }
    if (preferPricing(row.pricing, current.pricing)) {
      current.pricing = row.pricing
    }
    current.health = mergeModelHealth(current.health, row.health)
    current.capabilities = mergeSortedCapabilities(current.capabilities, capabilities)
  }

  const items = Array.from(grouped.values()).map((item) => ({
    ...item,
    platforms: item.platforms.sort((a, b) => a.localeCompare(b))
  }))

  const platformSet = new Set<string>()
  const capabilitySet = new Set<string>()
  const billingSet = new Set<string>()

  for (const item of items) {
    item.platforms.forEach((platform) => platformSet.add(platform))
    item.capabilities.forEach((capability) => capabilitySet.add(capability))
    if (item.pricing?.billing_mode) {
      billingSet.add(item.pricing.billing_mode)
    }
  }

  return {
    items,
    facets: {
      platforms: Array.from(platformSet).sort((a, b) => a.localeCompare(b)),
      capabilities: Array.from(capabilitySet).sort(compareCapabilities),
      billingModes: Array.from(billingSet).sort((a, b) => a.localeCompare(b)),
      sortOptions: billingSet.size > 0 ? ['price', 'name', 'provider'] : ['name', 'provider']
    }
  }
}

export function filterModelCatalog(
  items: ModelCatalogCard[],
  filters: ModelCatalogFilters
): ModelCatalogCard[] {
  const search = filters.search.trim().toLowerCase()

  const filtered = items.filter((item) => {
    if (search && !item.name.toLowerCase().includes(search) && !item.provider.toLowerCase().includes(search)) {
      return false
    }
    if (filters.platform && !item.platforms.includes(filters.platform)) {
      return false
    }
    if (filters.capability && !item.capabilities.includes(filters.capability)) {
      return false
    }
    if (filters.billingMode && item.pricing?.billing_mode !== filters.billingMode) {
      return false
    }
    return true
  })

  return filtered.sort((a, b) => compareCatalogCards(a, b, filters.sortBy))
}

export function modelPriceScore(pricing: PublicModelPricing | null): number {
  if (!pricing) return Number.POSITIVE_INFINITY

  const values = [
    pricing.input_price,
    pricing.output_price,
    pricing.cache_write_price,
    pricing.cache_read_price,
    pricing.image_output_price,
    pricing.per_request_price
  ].filter((value): value is number => typeof value === 'number')

  if (values.length === 0) return Number.POSITIVE_INFINITY
  return values.reduce((sum, value) => sum + value, 0)
}

function preferPricing(next: PublicModelPricing | null, current: PublicModelPricing | null): boolean {
  if (!current) return !!next
  if (!next) return false
  return modelPriceScore(next) < modelPriceScore(current)
}

function cloneModelHealth(health?: PublicModelHealth | null): PublicModelHealth | null {
  if (!health) return null
  return {
    ...health,
    history: health.history.map((point) => ({ ...point }))
  }
}

function mergeModelHealth(current: PublicModelHealth | null, next?: PublicModelHealth | null): PublicModelHealth | null {
  if (!current) return cloneModelHealth(next)
  if (!next) return current

  const requestCount = current.request_count + next.request_count
  const successCount = successCountFromHealth(current) + successCountFromHealth(next)
  const history = mergeModelHealthHistory(current.history, next.history)
  return {
    status: worstModelHealthStatus(current.status, next.status),
    request_count: requestCount,
    success_rate: requestCount > 0 ? (successCount * 100) / requestCount : null,
    history
  }
}

function mergeModelHealthHistory(
  current: PublicModelHealthHistoryPoint[],
  next: PublicModelHealthHistoryPoint[]
): PublicModelHealthHistoryPoint[] {
  const length = Math.max(current.length, next.length)
  const history: PublicModelHealthHistoryPoint[] = []

  for (let i = 0; i < length; i++) {
    const a = current[i]
    const b = next[i]
    if (!a) {
      history.push({ ...b })
      continue
    }
    if (!b) {
      history.push({ ...a })
      continue
    }

    const requestCount = a.request_count + b.request_count
    const successCount = successCountFromPoint(a) + successCountFromPoint(b)
    history.push({
      status: worstModelHealthStatus(a.status, b.status),
      request_count: requestCount,
      success_rate: requestCount > 0 ? (successCount * 100) / requestCount : null
    })
  }

  return history
}

function successCountFromHealth(health: PublicModelHealth): number {
  if (health.request_count <= 0 || typeof health.success_rate !== 'number') return 0
  return health.request_count * health.success_rate / 100
}

function successCountFromPoint(point: PublicModelHealthHistoryPoint): number {
  if (point.request_count <= 0 || typeof point.success_rate !== 'number') return 0
  return point.request_count * point.success_rate / 100
}

function worstModelHealthStatus(a: string, b: string): string {
  return modelHealthRank(a) <= modelHealthRank(b) ? a : b
}

function modelHealthRank(status: string): number {
  switch (status) {
    case 'failed':
      return 0
    case 'rate_limited':
      return 1
    case 'degraded':
      return 2
    case 'unknown':
    case 'orphaned_history':
      return 3
    case 'no_recent_traffic':
      return 4
    case 'idle':
      return 5
    case 'operational':
      return 6
    default:
      return 3
  }
}

function mergeSortedCapabilities(a: string[], b: string[]): string[] {
  return Array.from(new Set([...a, ...b])).sort(compareCapabilities)
}

function normalizeCapabilities(values?: string[]): string[] {
  return Array.from(new Set((values || []).map((value) => value.trim()).filter(Boolean))).sort(compareCapabilities)
}

function inferModelCapabilities(row: PublicModelCatalogItem): string[] {
  const text = `${row.name || ''} ${row.provider || ''} ${row.platform || ''}`.toLowerCase()
  const capabilities: string[] = []
  const add = (value: string) => {
    if (!capabilities.includes(value)) capabilities.push(value)
  }

  if (containsAny(text, ['o1', 'o3', 'o4', 'r1', 'reason', 'think', 'deepseek', 'sonnet', 'opus', 'grok', 'gemini-2.5', 'gemini-3', 'gpt-5'])) {
    add('reasoning')
  }
  if (containsAny(text, ['code', 'coder', 'claude', 'sonnet', 'gpt', 'deepseek', 'qwen', 'glm', 'kimi'])) {
    add('coding')
  }
  if (containsAny(text, ['long', 'context', '128k', '200k', '1m', 'gemini', 'claude', 'kimi', 'qwen'])) {
    add('longContext')
  }
  if (containsAny(text, ['4o', 'omni', 'vision', 'image', 'video', 'grok', 'gemini', 'claude', 'sora', 'veo', 'kling', 'wan', 'hailuo', 'seedream', 'seedance'])) {
    add('multimodal')
  }
  if (containsAny(text, ['flash', 'mini', 'haiku', 'turbo', 'fast', 'lite'])) {
    add('fast')
  }
  if (containsAny(text, ['mini', 'flash', 'haiku', 'lite', 'cheap']) || modelPriceScore(row.pricing) <= 0.000002) {
    add('lowCost')
  }

  return normalizeCapabilities(capabilities)
}

function modelDescriptionFromCapabilities(capabilities: string[]): string {
  if (capabilities.length === 0) return 'Available through the TOP-AI gateway.'
  const labels: Record<string, string> = {
    reasoning: 'reasoning',
    coding: 'coding',
    longContext: 'long context',
    lowCost: 'low-cost',
    multimodal: 'multimodal',
    fast: 'fast response'
  }
  return `Suited for ${capabilities.slice(0, 3).map((capability) => labels[capability] || capability).join(', ')} workloads.`
}

function containsAny(text: string, values: string[]): boolean {
  return values.some((value) => text.includes(value))
}

function compareCapabilities(a: string, b: string): number {
  const ai = capabilityOrder.indexOf(a)
  const bi = capabilityOrder.indexOf(b)
  if (ai !== -1 || bi !== -1) {
    return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi)
  }
  return a.localeCompare(b)
}

function compareCatalogCards(a: ModelCatalogCard, b: ModelCatalogCard, sortBy: ModelCatalogSort): number {
  if (sortBy === 'price') {
    const delta = modelPriceScore(a.pricing) - modelPriceScore(b.pricing)
    if (delta !== 0) return delta
  }
  if (sortBy === 'provider') {
    const providerDelta = a.provider.localeCompare(b.provider)
    if (providerDelta !== 0) return providerDelta
  }
  return a.name.localeCompare(b.name)
}
