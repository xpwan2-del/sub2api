<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="homeContent" class="min-h-screen">
    <!-- iframe mode -->
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <!-- HTML mode - SECURITY: homeContent is admin-only setting, XSS risk is acceptable -->
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Default Home Page -->
  <div v-else class="home-jarvis-page" :class="{ 'is-dark': isDark }">
    <PublicTopBar class="home-public-topbar" :is-dark="isDark" @toggle-theme="toggleTheme" />
    <JarvisGatewayScene
      class="home-jarvis-scene"
      :copy-kicker="t('home.jarvis.kicker')"
      :copy-title="siteName"
      :copy-description="siteSubtitle"
      :show-topbar="false"
    >
    </JarvisGatewayScene>

    <footer class="home-jarvis-footer">
      © {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import JarvisGatewayScene from '@/components/home/JarvisGatewayScene.vue'
import PublicTopBar from '@/components/public/PublicTopBar.vue'
import { usePublicBranding } from '@/composables/usePublicBranding'
import { useThemeMode } from '@/composables/useThemeMode'

const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()
const { isDark, toggleTheme } = useThemeMode()
const { siteName, siteSubtitle: configuredSiteSubtitle, homeContent } = usePublicBranding()

// Site settings - directly from appStore (already initialized from injected config)
const siteSubtitle = computed(() => configuredSiteSubtitle.value || t('home.jarvis.description'))
const currentYear = new Date().getFullYear()

// Check if homeContent is a URL (for iframe display)
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

onMounted(() => {
  // Check auth state
  authStore.checkAuth()

  // Ensure public settings are loaded (will use cache if already loaded from injected config)
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
.home-jarvis-page {
  position: relative;
  min-height: 100vh;
  overflow: hidden;
  background: #f7f7f5;
}

.home-jarvis-scene {
  min-height: 100vh;
}

.home-public-topbar {
  position: fixed;
  left: 0;
  right: 0;
}

.home-jarvis-footer {
  position: absolute;
  right: 28px;
  bottom: 70px;
  z-index: 12;
  color: rgba(30, 64, 175, 0.68);
  font-size: 12px;
  letter-spacing: 0;
  pointer-events: none;
  text-shadow: 0 1px 16px rgba(255, 255, 255, 0.54);
}

.home-jarvis-page.is-dark .home-jarvis-footer {
  color: rgba(221, 255, 251, 0.64);
  text-shadow: 0 1px 16px rgba(0, 0, 0, 0.35);
}

@media (max-width: 640px) {
  .home-jarvis-footer {
    right: 14px;
    bottom: 66px;
    max-width: calc(100vw - 28px);
    font-size: 11px;
  }
}
</style>
