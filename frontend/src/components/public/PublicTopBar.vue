<template>
  <header class="public-topbar" :class="{ 'is-dark': isDark }">
    <router-link to="/home" class="public-topbar-brand">
      <span class="public-topbar-logo-wrap">
        <img :src="brandLogo" :alt="siteName" class="public-topbar-logo" />
      </span>
      <span class="public-topbar-brand-copy">
        <span class="public-topbar-brand-name">{{ siteName }}</span>
        <span class="public-topbar-brand-subtitle">{{ displaySubtitle }}</span>
      </span>
    </router-link>

    <div class="public-topbar-actions">
      <a
        v-if="isExternalDocUrl"
        :href="docLink"
        target="_blank"
        rel="noopener noreferrer"
        class="public-topbar-nav-link"
      >
        <Icon name="book" size="sm" />
        <span>{{ t('home.viewDocs') }}</span>
      </a>
      <router-link
        v-else
        :to="docLink"
        class="public-topbar-nav-link"
        :class="{ 'is-active': route.path === docLink }"
      >
        <Icon name="book" size="sm" />
        <span>{{ t('home.viewDocs') }}</span>
      </router-link>
      <router-link
        to="/models"
        class="public-topbar-nav-link"
        :class="{ 'is-active': route.path === '/models' }"
      >
        <Icon name="grid" size="sm" />
        <span>{{ t('home.models') }}</span>
      </router-link>
      <a
        :href="canvasPath"
        class="public-topbar-nav-link"
      >
        <Icon name="brain" size="sm" />
        <span>{{ t('home.canvas') }}</span>
      </a>
      <LocaleSwitcher />
      <button
        class="public-topbar-icon-button"
        :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
        @click="$emit('toggle-theme')"
      >
        <Icon v-if="isDark" name="sun" size="md" />
        <Icon v-else name="moon" size="md" />
      </button>
      <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="public-topbar-primary-link">
        <span v-if="isAuthenticated" class="public-topbar-user-mark">{{ userInitial }}</span>
        <span>{{ isAuthenticated ? t('home.dashboard') : t('home.login') }}</span>
        <Icon name="arrowRight" size="xs" />
      </router-link>
    </div>
  </header>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAuthStore } from '@/stores'
import { usePublicBranding } from '@/composables/usePublicBranding'

defineProps<{
  isDark?: boolean
}>()

defineEmits<{
  'toggle-theme': []
}>()

const { t } = useI18n()
const route = useRoute()
const authStore = useAuthStore()
const { appStore, siteName, siteSubtitle, brandLogo, docLink, isExternalDocUrl } = usePublicBranding()

const canvasPath = '/apps/canvas'
const displaySubtitle = computed(() => siteSubtitle.value || 'Pioneers of AI')
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const userInitial = computed(() => {
  const user = authStore.user
  if (!user?.email) return ''
  return user.email.charAt(0).toUpperCase()
})

onMounted(() => {
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
.public-topbar {
  position: sticky;
  top: 0;
  z-index: 30;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  padding: 18px 28px;
  border-bottom: 1px solid rgba(37, 99, 235, 0.12);
  background: rgba(247, 247, 245, 0.76);
  color: #102035;
  backdrop-filter: blur(18px);
}

.public-topbar.is-dark {
  border-bottom-color: rgba(92, 255, 236, 0.14);
  background: rgba(7, 17, 31, 0.78);
  color: #eafffc;
}

.public-topbar-brand {
  display: inline-flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  color: inherit;
  text-decoration: none;
}

.public-topbar-logo-wrap {
  display: inline-flex;
  width: 44px;
  height: 44px;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(37, 99, 235, 0.24);
  border-radius: 12px;
  background: linear-gradient(135deg, rgba(240, 253, 250, 0.9), rgba(219, 234, 254, 0.72));
  box-shadow: 0 0 26px rgba(45, 212, 191, 0.14);
}

.public-topbar.is-dark .public-topbar-logo-wrap {
  border-color: rgba(92, 255, 236, 0.28);
  background: rgba(255, 255, 255, 0.06);
}

.public-topbar-logo {
  width: 32px;
  height: 32px;
  object-fit: contain;
}

.public-topbar-brand-copy {
  display: flex;
  min-width: 0;
  flex-direction: column;
  line-height: 1.15;
}

.public-topbar-brand-name {
  font-size: 16px;
  font-weight: 900;
  letter-spacing: 0.06em;
}

.public-topbar-brand-subtitle {
  max-width: 260px;
  overflow: hidden;
  color: rgba(71, 85, 105, 0.9);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.public-topbar.is-dark .public-topbar-brand-subtitle {
  color: rgba(204, 251, 241, 0.66);
}

.public-topbar-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.public-topbar-icon-button,
.public-topbar-nav-link,
.public-topbar-primary-link {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  border: 1px solid rgba(37, 99, 235, 0.24);
  border-radius: 9px;
  color: inherit;
  font-size: 12px;
  font-weight: 800;
  text-decoration: none;
  transition: border-color 0.16s ease, background 0.16s ease, transform 0.16s ease;
}

.public-topbar-icon-button {
  width: 36px;
  height: 36px;
  background: rgba(255, 255, 255, 0.54);
}

.public-topbar-nav-link {
  height: 36px;
  padding: 0 12px;
  background: rgba(255, 255, 255, 0.38);
}

.public-topbar-primary-link {
  height: 36px;
  padding: 0 13px;
  background: rgba(45, 212, 191, 0.14);
  text-transform: uppercase;
}

.public-topbar.is-dark .public-topbar-icon-button,
.public-topbar.is-dark .public-topbar-nav-link,
.public-topbar.is-dark .public-topbar-primary-link {
  border-color: rgba(92, 255, 236, 0.28);
  background: rgba(255, 255, 255, 0.07);
}

.public-topbar-icon-button:hover,
.public-topbar-nav-link:hover,
.public-topbar-nav-link.is-active,
.public-topbar-primary-link:hover {
  border-color: rgba(37, 99, 235, 0.46);
  transform: translateY(-1px);
}

.public-topbar.is-dark .public-topbar-icon-button:hover,
.public-topbar.is-dark .public-topbar-nav-link:hover,
.public-topbar.is-dark .public-topbar-nav-link.is-active,
.public-topbar.is-dark .public-topbar-primary-link:hover {
  border-color: rgba(92, 255, 236, 0.48);
  background: rgba(45, 212, 191, 0.12);
}

.public-topbar-user-mark {
  display: inline-flex;
  width: 20px;
  height: 20px;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  background: rgba(37, 99, 235, 0.16);
}

@media (max-width: 700px) {
  .public-topbar {
    align-items: flex-start;
    flex-direction: column;
    padding: 14px;
  }

  .public-topbar-actions {
    width: 100%;
    flex-wrap: wrap;
    justify-content: space-between;
  }

  .public-topbar-brand-subtitle {
    max-width: calc(100vw - 110px);
  }
}
</style>
