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
    <JarvisGatewayScene
      class="home-jarvis-scene"
      :copy-kicker="t('home.jarvis.kicker')"
      :copy-title="siteName"
      :copy-description="siteSubtitle"
    >
      <template #brand>
        <router-link to="/home" class="home-jarvis-brand-card">
          <img :src="brandLogo" :alt="siteName" class="home-jarvis-wide-logo" />
          <span class="home-jarvis-brand-word">{{ siteName }}</span>
        </router-link>
      </template>

      <template #topbar-end>
        <div class="home-jarvis-actions">
          <LocaleSwitcher />

          <a
            v-if="docUrl"
            :href="docUrl"
            :target="isExternalDocUrl ? '_blank' : undefined"
            :rel="isExternalDocUrl ? 'noopener noreferrer' : undefined"
            class="home-jarvis-action-button"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
            <span class="home-jarvis-doc-label">{{ t('home.viewDocs') }}</span>
          </a>

          <button
            @click="toggleTheme"
            class="home-jarvis-action-button"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>

          <router-link
            v-if="isAuthenticated"
            :to="dashboardPath"
            class="home-jarvis-auth-link"
          >
            <span class="home-jarvis-user-mark">
              {{ userInitial }}
            </span>
            <span class="hidden sm:inline">{{ t('home.dashboard') }}</span>
            <Icon name="arrowRight" size="xs" />
          </router-link>
          <router-link
            v-else
            to="/login"
            class="home-jarvis-auth-link"
          >
            {{ t('home.login') }}
          </router-link>
        </div>
      </template>
    </JarvisGatewayScene>

    <footer class="home-jarvis-footer">
      © {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import JarvisGatewayScene from '@/components/home/JarvisGatewayScene.vue'

const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

// Site settings - directly from appStore (already initialized from injected config)
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'TOP-AI')
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || t('home.jarvis.description'))
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const brandLogo = computed(() => siteLogo.value || '/top-ai-logo.png')
const isExternalDocUrl = computed(() => /^https?:\/\//i.test(docUrl.value))
const currentYear = new Date().getFullYear()

// Check if homeContent is a URL (for iframe display)
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

// Theme
const isDark = ref(document.documentElement.classList.contains('dark'))

// Auth state
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const userInitial = computed(() => {
  const user = authStore.user
  if (!user || !user.email) return ''
  return user.email.charAt(0).toUpperCase()
})

// Toggle theme
function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

// Initialize theme
function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

onMounted(() => {
  initTheme()

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

.home-jarvis-brand-card,
.home-jarvis-actions {
  border: 1px solid rgba(59, 130, 246, 0.28);
  background:
    linear-gradient(135deg, rgba(240, 253, 250, 0.9), rgba(219, 234, 254, 0.72)),
    rgba(255, 255, 255, 0.64);
  box-shadow: 0 0 30px rgba(45, 212, 191, 0.12), inset 0 0 24px rgba(59, 130, 246, 0.08);
  backdrop-filter: blur(16px);
}

.home-jarvis-page.is-dark .home-jarvis-brand-card,
.home-jarvis-page.is-dark .home-jarvis-actions {
  border-color: rgba(92, 255, 236, 0.28);
  background:
    linear-gradient(135deg, rgba(8, 35, 58, 0.86), rgba(23, 37, 84, 0.7)),
    rgba(4, 14, 28, 0.58);
  box-shadow: 0 0 30px rgba(45, 255, 229, 0.1), inset 0 0 24px rgba(45, 255, 229, 0.04);
}

.home-jarvis-brand-card {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 10px;
  width: auto;
  height: 58px;
  padding: 8px 16px 8px 10px;
  color: #0f172a;
  border-radius: 16px;
  overflow: hidden;
  text-decoration: none;
}

.home-jarvis-page.is-dark .home-jarvis-brand-card {
  color: #e9fffb;
}

.home-jarvis-wide-logo {
  width: 43px;
  height: 43px;
  flex: 0 0 auto;
  object-fit: contain;
  object-position: center;
  filter: drop-shadow(0 0 12px rgba(92, 180, 255, 0.38));
}

.home-jarvis-brand-word {
  background: linear-gradient(90deg, #0f766e, #2563eb 62%, #172554);
  background-clip: text;
  color: transparent;
  font-size: 18px;
  font-weight: 900;
  letter-spacing: 0.08em;
  line-height: 1;
  white-space: nowrap;
}

.home-jarvis-page.is-dark .home-jarvis-brand-word {
  background-image: linear-gradient(90deg, #ecfeff, #60a5fa 58%, #c7d2fe);
}

.home-jarvis-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px;
  border-radius: 14px;
  overflow: visible;
  position: relative;
  z-index: 40;
}

.home-jarvis-actions :deep(> .relative > button),
.home-jarvis-action-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  height: 36px;
  min-width: 36px;
  padding: 0 10px;
  border: 1px solid rgba(37, 99, 235, 0.16);
  border-radius: 9px;
  background: rgba(255, 255, 255, 0.42);
  color: rgba(15, 23, 42, 0.78);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  font-weight: 700;
  text-decoration: none;
  text-transform: uppercase;
  transition: background 0.16s ease, color 0.16s ease, border-color 0.16s ease;
}

.home-jarvis-page.is-dark .home-jarvis-actions :deep(> .relative > button),
.home-jarvis-page.is-dark .home-jarvis-action-button {
  border-color: rgba(106, 255, 240, 0.16);
  background: rgba(255, 255, 255, 0.06);
  color: rgba(234, 255, 252, 0.78);
}

.home-jarvis-actions :deep(> .relative > button:hover),
.home-jarvis-action-button:hover {
  border-color: rgba(37, 99, 235, 0.34);
  background: rgba(255, 255, 255, 0.72);
  color: #0f172a;
}

.home-jarvis-page.is-dark .home-jarvis-actions :deep(> .relative > button:hover),
.home-jarvis-page.is-dark .home-jarvis-action-button:hover {
  border-color: rgba(106, 255, 240, 0.36);
  background: rgba(255, 255, 255, 0.12);
  color: #ffffff;
}

.home-jarvis-actions :deep(> .relative > div) {
  z-index: 80;
  border-color: rgba(45, 212, 191, 0.24);
  background: rgba(255, 255, 255, 0.96);
  box-shadow: 0 18px 50px rgba(15, 23, 42, 0.18);
}

.home-jarvis-page.is-dark .home-jarvis-actions :deep(> .relative > div) {
  border-color: rgba(92, 255, 236, 0.24);
  background: rgba(8, 20, 38, 0.96);
  box-shadow: 0 18px 50px rgba(0, 0, 0, 0.36);
}

.home-jarvis-auth-link {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 36px;
  gap: 6px;
  border: 1px solid rgba(37, 99, 235, 0.28);
  border-radius: 9px;
  background: linear-gradient(135deg, rgba(45, 212, 191, 0.18), rgba(59, 130, 246, 0.18));
  padding: 0 14px;
  color: #0f172a;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.02em;
  text-decoration: none;
  text-transform: uppercase;
  transition: background 0.16s ease, border-color 0.16s ease, box-shadow 0.16s ease;
}

.home-jarvis-page.is-dark .home-jarvis-auth-link {
  border-color: rgba(106, 255, 240, 0.34);
  background: rgba(92, 255, 236, 0.12);
  color: #eafffc;
}

.home-jarvis-auth-link:hover {
  border-color: rgba(37, 99, 235, 0.48);
  background: linear-gradient(135deg, rgba(45, 212, 191, 0.28), rgba(59, 130, 246, 0.26));
  box-shadow: 0 0 20px rgba(59, 130, 246, 0.14);
}

.home-jarvis-page.is-dark .home-jarvis-auth-link:hover {
  border-color: rgba(106, 255, 240, 0.58);
  background: rgba(92, 255, 236, 0.2);
  box-shadow: 0 0 20px rgba(45, 255, 229, 0.14);
}

.home-jarvis-user-mark {
  display: grid;
  place-items: center;
  width: 22px;
  height: 22px;
  border: 1px solid rgba(37, 99, 235, 0.26);
  border-radius: 6px;
  background: rgba(59, 130, 246, 0.16);
  color: #1d4ed8;
  font-size: 10px;
  font-weight: 800;
}

.home-jarvis-page.is-dark .home-jarvis-user-mark {
  border-color: rgba(106, 255, 240, 0.36);
  background: rgba(92, 255, 236, 0.16);
  color: #ffffff;
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

@media (max-width: 920px) {
  .home-jarvis-brand-card {
    height: 52px;
    padding: 7px 13px 7px 9px;
  }

  .home-jarvis-wide-logo {
    width: 36px;
    height: 36px;
  }

  .home-jarvis-brand-word {
    font-size: 16px;
  }

  .home-jarvis-actions {
    gap: 5px;
    padding: 6px;
  }
}

@media (max-width: 640px) {
  .home-jarvis-brand-card {
    height: 44px;
    gap: 7px;
    padding: 6px 10px 6px 7px;
  }

  .home-jarvis-wide-logo {
    width: 28px;
    height: 28px;
  }

  .home-jarvis-brand-word {
    font-size: 13px;
  }

  .home-jarvis-actions :deep(> .relative > button span:not(:first-child)) {
    display: none;
  }

  .home-jarvis-actions {
    max-width: 52vw;
    overflow: visible;
  }

  .home-jarvis-action-button {
    padding: 0;
  }

  .home-jarvis-doc-label {
    display: none;
  }

  .home-jarvis-auth-link {
    padding: 0 10px;
  }

  .home-jarvis-footer {
    right: 14px;
    bottom: 66px;
    max-width: calc(100vw - 28px);
    font-size: 11px;
  }
}
</style>
