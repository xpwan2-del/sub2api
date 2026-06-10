/**
 * Bundle Store
 * Global state management for user bundle subscriptions with caching and deduplication
 */

import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import bundlesAPI from '@/api/bundles'
import type { BundlePlan, BundleSubscription, BundleUsageProgress } from '@/types/bundle'

// Cache TTL: 60 seconds
const CACHE_TTL_MS = 60_000

// Request generation counter to invalidate stale in-flight responses
let bundleRequestGeneration = 0
let usageRequestGeneration = 0

export const useBundleStore = defineStore('bundles', () => {
  // State
  const activeBundle = ref<BundleSubscription | null>(null)
  const activePlan = ref<BundlePlan | null>(null)
  const usageProgress = ref<BundleUsageProgress[]>([])
  const bundleLoading = ref(false)
  const usageLoading = ref(false)
  const bundleLoaded = ref(false)
  const usageLoaded = ref(false)
  const bundleLastFetchedAt = ref<number | null>(null)
  const usageLastFetchedAt = ref<number | null>(null)

  // In-flight request deduplication
  let activeBundlePromise: Promise<BundleSubscription | null> | null = null
  let usagePromise: Promise<BundleUsageProgress[]> | null = null

  // Computed
  const hasActiveBundle = computed(
    () => activeBundle.value?.status === 'active'
  )

  /**
   * Fetch active bundle subscription with caching and deduplication
   * @param force - Force refresh even if cache is valid
   */
  async function fetchActiveBundle(force = false): Promise<BundleSubscription | null> {
    const now = Date.now()

    // Return cached data if valid
    if (
      !force &&
      bundleLoaded.value &&
      bundleLastFetchedAt.value &&
      now - bundleLastFetchedAt.value < CACHE_TTL_MS
    ) {
      return activeBundle.value
    }

    // Return in-flight request if exists (deduplication)
    if (activeBundlePromise && !force) {
      return activeBundlePromise
    }

    const currentGeneration = ++bundleRequestGeneration

    // Start new request
    bundleLoading.value = true
    const requestPromise = bundlesAPI
      .getMyBundle()
      .then(async (data) => {
        if (currentGeneration === bundleRequestGeneration) {
          // Backend returns an array — extract first active subscription
          const bundle = Array.isArray(data)
            ? data.find(b => b.status === 'active') ?? null
            : data ?? null
          activeBundle.value = bundle

          // Load plan details for name, description, tier, features etc.
          if (bundle?.plan_id) {
            try {
              activePlan.value = await bundlesAPI.getPlanDetail(bundle.plan_id)
            } catch {
              activePlan.value = null
            }
          } else {
            activePlan.value = null
          }

          bundleLoaded.value = true
          bundleLastFetchedAt.value = Date.now()
        }
        return activeBundle.value
      })
      .catch((error) => {
        console.error('Failed to fetch active bundle:', error)
        throw error
      })
      .finally(() => {
        if (activeBundlePromise === requestPromise) {
          bundleLoading.value = false
          activeBundlePromise = null
        }
      })

    activeBundlePromise = requestPromise

    return activeBundlePromise
  }

  /**
   * Fetch bundle usage progress with caching and deduplication
   * @param force - Force refresh even if cache is valid
   */
  async function fetchUsageProgress(force = false): Promise<BundleUsageProgress[]> {
    const now = Date.now()

    // Return cached data if valid
    if (
      !force &&
      usageLoaded.value &&
      usageLastFetchedAt.value &&
      now - usageLastFetchedAt.value < CACHE_TTL_MS
    ) {
      return usageProgress.value
    }

    // Return in-flight request if exists (deduplication)
    if (usagePromise && !force) {
      return usagePromise
    }

    const currentGeneration = ++usageRequestGeneration

    // Start new request
    usageLoading.value = true
    const requestPromise = bundlesAPI
      .getMyUsage()
      .then((data) => {
        if (currentGeneration === usageRequestGeneration) {
          usageProgress.value = data || []
          usageLoaded.value = true
          usageLastFetchedAt.value = Date.now()
        }
        return data
      })
      .catch((error) => {
        console.error('Failed to fetch bundle usage:', error)
        throw error
      })
      .finally(() => {
        if (usagePromise === requestPromise) {
          usageLoading.value = false
          usagePromise = null
        }
      })

    usagePromise = requestPromise

    return usagePromise
  }

  /**
   * Clear all bundle data
   */
  function clear() {
    bundleRequestGeneration++
    usageRequestGeneration++
    activeBundlePromise = null
    usagePromise = null
    activeBundle.value = null
    activePlan.value = null
    usageProgress.value = []
    bundleLoaded.value = false
    usageLoaded.value = false
    bundleLastFetchedAt.value = null
    usageLastFetchedAt.value = null
  }

  /**
   * Invalidate cache (force next fetch to reload)
   */
  function invalidateCache() {
    bundleLastFetchedAt.value = null
    usageLastFetchedAt.value = null
  }

  return {
    // State
    activeBundle,
    activePlan,
    usageProgress,
    bundleLoading,
    usageLoading,
    hasActiveBundle,

    // Actions
    fetchActiveBundle,
    fetchUsageProgress,
    clear,
    invalidateCache
  }
})
