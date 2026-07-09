<template>
  <div class="auth-page-shell relative flex min-h-screen items-center justify-center overflow-hidden p-4" :class="{ 'is-dark': isDark }">
    <!-- Background -->
    <div class="auth-page-bg absolute inset-0"></div>

    <nav class="auth-public-topbar">
      <PublicBrand :is-dark="isDark" />
      <PublicNavActions :is-dark="isDark" @toggle-theme="toggleTheme" />
    </nav>

    <!-- Decorative Elements -->
    <div class="pointer-events-none absolute inset-0 overflow-hidden">
      <!-- Grid Pattern -->
      <div
        class="absolute inset-0 bg-[linear-gradient(rgba(20,184,166,0.06)_1px,transparent_1px),linear-gradient(90deg,rgba(20,184,166,0.06)_1px,transparent_1px)] bg-[size:64px_64px] dark:bg-[linear-gradient(rgba(94,255,238,0.08)_1px,transparent_1px),linear-gradient(90deg,rgba(94,255,238,0.08)_1px,transparent_1px)]"
      ></div>
    </div>

    <!-- Content Container -->
    <div class="relative z-10 mt-24 w-full max-w-md sm:mt-16">
      <!-- Logo/Brand -->
      <div class="mb-8 text-center">
        <!-- Custom Logo or Default Logo -->
        <template v-if="settingsLoaded">
          <div
            class="mb-4 inline-flex h-16 w-16 items-center justify-center overflow-hidden rounded-2xl border border-primary-300/30 bg-white/50 shadow-lg shadow-primary-500/20 dark:border-primary-300/20 dark:bg-dark-900/50"
          >
            <img :src="brandLogo" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <h1 class="text-gradient mb-2 text-3xl font-bold">
            {{ siteName }}
          </h1>
          <p class="text-sm text-gray-500 dark:text-dark-400">
            {{ siteSubtitle }}
          </p>
        </template>
      </div>

      <!-- Card Container -->
      <div class="card-glass rounded-2xl p-8 shadow-glass">
        <slot />
      </div>

      <!-- Footer Links -->
      <div class="mt-6 text-center text-sm">
        <slot name="footer" />
      </div>

      <!-- Copyright -->
      <div class="mt-8 text-center text-xs text-gray-400 dark:text-dark-500">
        &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import PublicBrand from '@/components/public/PublicBrand.vue'
import PublicNavActions from '@/components/public/PublicNavActions.vue'
import { usePublicBranding } from '@/composables/usePublicBranding'
import { useThemeMode } from '@/composables/useThemeMode'

const appStore = useAppStore()
const { t } = useI18n()
const { isDark, toggleTheme } = useThemeMode()
const { siteName, siteSubtitle: configuredSiteSubtitle, brandLogo } = usePublicBranding()

const siteSubtitle = computed(() => configuredSiteSubtitle.value || '')
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
.text-gradient {
  @apply bg-gradient-to-r from-primary-600 to-primary-500 bg-clip-text text-transparent;
}

.auth-page-bg {
  background:
    radial-gradient(circle at 50% 28%, rgba(20, 184, 166, 0.2), transparent 27%),
    radial-gradient(circle at 78% 18%, rgba(59, 130, 246, 0.13), transparent 24%),
    linear-gradient(90deg, rgba(247, 247, 245, 0.96), rgba(247, 247, 245, 0.08) 30%, rgba(247, 247, 245, 0.08) 70%, rgba(247, 247, 245, 0.96)),
    #f7f7f5;
}

.auth-page-shell.is-dark .auth-page-bg {
  background:
    radial-gradient(circle at 50% 28%, rgba(20, 184, 166, 0.28), transparent 26%),
    radial-gradient(circle at 78% 18%, rgba(59, 130, 246, 0.18), transparent 24%),
    linear-gradient(90deg, rgba(2, 6, 23, 0.92), rgba(8, 24, 44, 0.22) 30%, rgba(8, 24, 44, 0.22) 70%, rgba(2, 6, 23, 0.92)),
    #030712;
}

.auth-public-topbar {
  position: absolute;
  left: clamp(18px, 4vw, 44px);
  right: clamp(18px, 4vw, 44px);
  top: 24px;
  z-index: 20;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

@media (max-width: 720px) {
  .auth-public-topbar {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
