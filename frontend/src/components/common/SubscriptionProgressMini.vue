<template>
  <div v-if="hasActiveBundle" class="relative" ref="containerRef">
    <!-- Mini Indicator -->
    <button
      @click="toggleTooltip"
      class="flex cursor-pointer items-center gap-2 rounded-xl px-3 py-1.5 transition-colors bg-primary-50 hover:bg-primary-100 dark:bg-primary-900/20 dark:hover:bg-primary-900/30"
      :title="t('subscriptionProgress.viewDetails')"
    >
      <Icon name="sparkles" size="sm" class="text-primary-600 dark:text-primary-400" />
      <span class="text-xs font-medium text-primary-700 dark:text-primary-300">
        {{ t('subscriptionProgress.currentSubscription') }}
      </span>
    </button>

    <!-- Hover/Click Tooltip -->
    <transition name="dropdown">
      <div
        v-if="tooltipOpen"
        class="absolute right-0 z-50 mt-2 w-[320px] overflow-hidden rounded-xl border border-gray-200 bg-white shadow-xl dark:border-dark-700 dark:bg-dark-800"
      >
        <!-- Bundle Info Card -->
        <div class="p-4">
          <!-- Tier accent bar -->
          <div class="mb-3 h-1 w-full rounded-full" :class="tierTheme.accentClass"></div>

          <!-- Plan name + tier badge -->
          <div class="flex items-center gap-2">
            <h3 class="truncate text-base font-bold text-gray-900 dark:text-white">
              {{ bundleStore.activePlan?.name || t('subscriptionProgress.bundleActive') }}
            </h3>
            <span :class="tierTheme.badgeClass">
              {{ tierLabel }}
            </span>
          </div>

          <!-- Description -->
          <p v-if="bundleStore.activePlan?.description" class="mt-1 text-xs leading-relaxed text-gray-500 dark:text-gray-400">
            {{ bundleStore.activePlan.description }}
          </p>

          <!-- Meta info grid -->
          <div class="mt-3 grid grid-cols-2 gap-2">
            <!-- Expiration -->
            <div v-if="bundleStore.activeBundle?.expires_at" class="rounded-lg bg-gray-50 p-2 dark:bg-dark-700/50">
              <p class="text-[10px] text-gray-400 dark:text-gray-500">{{ t('subscriptionProgress.expires') }}</p>
              <p class="text-xs font-medium" :class="getDaysRemainingClass(bundleStore.activeBundle.expires_at)">
                {{ formatDaysRemaining(bundleStore.activeBundle.expires_at) }}
              </p>
            </div>
            <!-- Concurrency -->
            <div v-if="bundleStore.activeBundle?.concurrency_limit" class="rounded-lg bg-gray-50 p-2 dark:bg-dark-700/50">
              <p class="text-[10px] text-gray-400 dark:text-gray-500">{{ t('subscriptionProgress.concurrency') }}</p>
              <p class="text-xs font-medium text-gray-700 dark:text-gray-300">
                {{ bundleStore.activeBundle.concurrency_limit }}
              </p>
            </div>
            <!-- RPM -->
            <div v-if="bundleStore.activeBundle?.rpm_limit" class="rounded-lg bg-gray-50 p-2 dark:bg-dark-700/50">
              <p class="text-[10px] text-gray-400 dark:text-gray-500">{{ t('subscriptionProgress.rpm') }}</p>
              <p class="text-xs font-medium text-gray-700 dark:text-gray-300">
                {{ bundleStore.activeBundle.rpm_limit }}
              </p>
            </div>
            <!-- Source -->
            <div v-if="bundleStore.activeBundle?.source" class="rounded-lg bg-gray-50 p-2 dark:bg-dark-700/50">
              <p class="text-[10px] text-gray-400 dark:text-gray-500">{{ t('subscriptionProgress.source') }}</p>
              <p class="text-xs font-medium text-gray-700 dark:text-gray-300">
                {{ sourceLabel(bundleStore.activeBundle.source) }}
              </p>
            </div>
          </div>

          <!-- Features list -->
          <div v-if="bundleStore.activePlan?.features?.length" class="mt-3 space-y-1">
            <div
              v-for="(feature, idx) in bundleStore.activePlan.features.slice(0, 4)"
              :key="idx"
              class="flex items-center gap-1.5"
            >
              <Icon name="check" size="xs" :class="tierTheme.iconClass" />
              <span class="text-xs text-gray-600 dark:text-gray-400">{{ feature }}</span>
            </div>
            <p v-if="bundleStore.activePlan.features.length > 4" class="pl-4 text-[10px] text-gray-400">
              +{{ bundleStore.activePlan.features.length - 4 }} {{ t('subscriptionProgress.moreFeatures') }}
            </p>
          </div>
        </div>

        <!-- View All Link -->
        <div class="border-t border-gray-100 p-2 dark:border-dark-700">
          <router-link
            to="/bundles"
            @click="closeTooltip"
            class="block w-full py-1 text-center text-xs text-primary-600 hover:underline dark:text-primary-400"
          >
            {{ t('subscriptionProgress.viewCurrent') }}
          </router-link>
        </div>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useBundleStore } from '@/stores'
import { getTierTheme, getTierI18nKey } from '@/constants/bundleTiers'

const { t } = useI18n()

const bundleStore = useBundleStore()

const containerRef = ref<HTMLElement | null>(null)
const tooltipOpen = ref(false)

// Tier display helpers (used by dropdown tooltip)
const tier = computed(() => bundleStore.activePlan?.tier)
const tierTheme = computed(() => getTierTheme(tier.value))
const tierLabel = computed(() => tier.value ? t(getTierI18nKey(tier.value, 'user')) : '')

// Use store data
const hasActiveBundle = computed(() => bundleStore.hasActiveBundle)

function sourceLabel(source: string): string {
  switch (source) {
    case 'purchase': return t('subscriptionProgress.sourcePurchase')
    case 'redeem': return t('subscriptionProgress.sourceRedeem')
    case 'admin_assign': return t('subscriptionProgress.sourceAdmin')
    default: return source
  }
}

function formatDaysRemaining(expiresAt: string): string {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  if (diff < 0) return t('subscriptionProgress.expired')
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))
  if (days === 0) return t('subscriptionProgress.expiresToday')
  if (days === 1) return t('subscriptionProgress.expiresTomorrow')
  return t('subscriptionProgress.daysRemaining', { days })
}

function getDaysRemainingClass(expiresAt: string): string {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))
  if (days <= 3) return 'text-red-600 dark:text-red-400'
  if (days <= 7) return 'text-orange-600 dark:text-orange-400'
  return 'text-gray-700 dark:text-gray-300'
}

function toggleTooltip() {
  tooltipOpen.value = !tooltipOpen.value
}

function closeTooltip() {
  tooltipOpen.value = false
}

function handleClickOutside(event: MouseEvent) {
  if (containerRef.value && !containerRef.value.contains(event.target as Node)) {
    closeTooltip()
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  bundleStore.fetchActiveBundle().catch((error) => {
    console.error('Failed to load bundle data in SubscriptionProgressMini:', error)
  })
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.2s ease;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: scale(0.95) translateY(-4px);
}
</style>
