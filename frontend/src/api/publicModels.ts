import { apiClient } from './client'

export interface PublicModelPricingInterval {
  min_tokens: number
  max_tokens: number | null
  tier_label?: string
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  per_request_price: number | null
}

export interface PublicModelPricing {
  billing_mode: string
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  image_output_price: number | null
  per_request_price: number | null
  intervals: PublicModelPricingInterval[]
}

export interface PublicModelHealthHistoryPoint {
  status: string
  request_count: number
  success_rate?: number | null
}

export interface PublicModelHealth {
  status: string
  request_count: number
  success_rate?: number | null
  history: PublicModelHealthHistoryPoint[]
}

export interface PublicModelCatalogItem {
  name: string
  provider: string
  platform: string
  status: string
  description?: string
  capabilities?: string[]
  pricing: PublicModelPricing | null
  health?: PublicModelHealth | null
}

export const publicModelsAPI = {
  async getCatalog(options?: { signal?: AbortSignal }): Promise<PublicModelCatalogItem[]> {
    const response = await apiClient.get<PublicModelCatalogItem[]>('/public/models/catalog', {
      signal: options?.signal
    })
    return response.data
  }
}
