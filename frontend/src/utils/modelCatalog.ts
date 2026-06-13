import type { PublicModelCatalogItem, PublicModelPricing } from '@/api/publicModels'

export type ModelCatalogSort = 'price' | 'name' | 'provider'

export interface ModelCatalogCard {
  id: string
  name: string
  provider: string
  platforms: string[]
  status: string
  pricing: PublicModelPricing | null
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
    const capabilities: string[] = []
    const current = grouped.get(key)

    if (!current) {
      grouped.set(key, {
        id: key.replace(/[^a-z0-9._-]+/g, '-'),
        name,
        provider: row.provider || row.platform,
        platforms: row.platform ? [row.platform] : [],
        status: row.status,
        pricing: row.pricing,
        capabilities
      })
      continue
    }

    current.provider = current.provider || row.provider || row.platform
    if (row.platform && !current.platforms.includes(row.platform)) {
      current.platforms.push(row.platform)
    }
    if (preferPricing(row.pricing, current.pricing)) {
      current.pricing = row.pricing
    }
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

function mergeSortedCapabilities(a: string[], b: string[]): string[] {
  return Array.from(new Set([...a, ...b])).sort(compareCapabilities)
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
