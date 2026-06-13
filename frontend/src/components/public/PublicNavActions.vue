<template>
  <div class="public-nav-actions" :class="{ 'is-dark': isDark }">
    <LocaleSwitcher />

    <a
      v-if="isExternalDocUrl"
      :href="docLink"
      target="_blank"
      rel="noopener noreferrer"
      class="public-nav-button"
      :title="t('home.viewDocs')"
    >
      <Icon name="book" size="md" />
      <span>{{ t('home.viewDocs') }}</span>
    </a>
    <router-link
      v-else
      :to="docLink"
      class="public-nav-button"
      :class="{ 'is-active': route.path === docLink }"
      :title="t('home.viewDocs')"
    >
      <Icon name="book" size="md" />
      <span>{{ t('home.viewDocs') }}</span>
    </router-link>

    <router-link
      to="/models"
      class="public-nav-button"
      :class="{ 'is-active': route.path === '/models' }"
      :title="t('modelCatalog.title')"
    >
      <Icon name="grid" size="md" />
      <span>{{ t('home.models') }}</span>
    </router-link>

    <button
      type="button"
      class="public-nav-button"
      :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
      @click="$emit('toggle-theme')"
    >
      <Icon v-if="isDark" name="sun" size="md" />
      <Icon v-else name="moon" size="md" />
    </button>

    <router-link
      v-if="isAuthenticated"
      :to="dashboardPath"
      class="public-nav-auth"
    >
      <span class="public-nav-user-mark">{{ userInitial }}</span>
      <span class="hidden sm:inline">{{ t('home.dashboard') }}</span>
      <Icon name="arrowRight" size="xs" />
    </router-link>
    <router-link
      v-else
      to="/login"
      class="public-nav-auth"
      :class="{ 'is-active': route.path === '/login' }"
    >
      {{ t('home.login') }}
    </router-link>
  </div>
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

const route = useRoute()
const { t } = useI18n()
const authStore = useAuthStore()
const { appStore, docLink, isExternalDocUrl } = usePublicBranding()

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
.public-nav-actions {
  position: relative;
  z-index: 40;
  display: flex;
  align-items: center;
  gap: 8px;
  overflow: visible;
  border: 1px solid rgba(59, 130, 246, 0.28);
  border-radius: 14px;
  background:
    linear-gradient(135deg, rgba(240, 253, 250, 0.9), rgba(219, 234, 254, 0.72)),
    rgba(255, 255, 255, 0.64);
  box-shadow: 0 0 30px rgba(45, 212, 191, 0.12), inset 0 0 24px rgba(59, 130, 246, 0.08);
  padding: 8px;
  backdrop-filter: blur(16px);
}

.public-nav-actions.is-dark {
  border-color: rgba(92, 255, 236, 0.28);
  background:
    linear-gradient(135deg, rgba(8, 35, 58, 0.86), rgba(23, 37, 84, 0.7)),
    rgba(4, 14, 28, 0.58);
  box-shadow: 0 0 30px rgba(45, 255, 229, 0.1), inset 0 0 24px rgba(45, 255, 229, 0.04);
}

.public-nav-actions :deep(> .relative > button),
.public-nav-button,
.public-nav-auth {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-width: 36px;
  height: 36px;
  border: 1px solid rgba(37, 99, 235, 0.16);
  border-radius: 9px;
  background: rgba(255, 255, 255, 0.42);
  color: rgba(15, 23, 42, 0.78);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.02em;
  padding: 0 10px;
  text-decoration: none;
  text-transform: uppercase;
  transition: background 0.16s ease, color 0.16s ease, border-color 0.16s ease, box-shadow 0.16s ease;
}

.public-nav-actions.is-dark :deep(> .relative > button),
.public-nav-actions.is-dark .public-nav-button,
.public-nav-actions.is-dark .public-nav-auth {
  border-color: rgba(106, 255, 240, 0.16);
  background: rgba(255, 255, 255, 0.06);
  color: rgba(234, 255, 252, 0.78);
}

.public-nav-actions :deep(> .relative > button:hover),
.public-nav-button:hover,
.public-nav-auth:hover,
.public-nav-button.is-active,
.public-nav-auth.is-active {
  border-color: rgba(37, 99, 235, 0.34);
  background: rgba(255, 255, 255, 0.72);
  color: #0f172a;
}

.public-nav-actions.is-dark :deep(> .relative > button:hover),
.public-nav-actions.is-dark .public-nav-button:hover,
.public-nav-actions.is-dark .public-nav-auth:hover,
.public-nav-actions.is-dark .public-nav-button.is-active,
.public-nav-actions.is-dark .public-nav-auth.is-active {
  border-color: rgba(106, 255, 240, 0.36);
  background: rgba(255, 255, 255, 0.12);
  color: #ffffff;
}

.public-nav-actions :deep(> .relative > div) {
  z-index: 80;
  border-color: rgba(45, 212, 191, 0.24);
  background: rgba(255, 255, 255, 0.96);
  box-shadow: 0 18px 50px rgba(15, 23, 42, 0.18);
}

.public-nav-actions.is-dark :deep(> .relative > div) {
  border-color: rgba(92, 255, 236, 0.24);
  background: rgba(8, 20, 38, 0.96);
  box-shadow: 0 18px 50px rgba(0, 0, 0, 0.36);
}

.public-nav-auth {
  border-color: rgba(37, 99, 235, 0.28);
  background: linear-gradient(135deg, rgba(45, 212, 191, 0.18), rgba(59, 130, 246, 0.18));
  padding: 0 14px;
}

.public-nav-actions.is-dark .public-nav-auth {
  border-color: rgba(106, 255, 240, 0.34);
  background: rgba(92, 255, 236, 0.12);
  color: #eafffc;
}

.public-nav-user-mark {
  display: grid;
  width: 22px;
  height: 22px;
  place-items: center;
  border: 1px solid rgba(37, 99, 235, 0.26);
  border-radius: 6px;
  background: rgba(59, 130, 246, 0.16);
  color: #1d4ed8;
  font-size: 10px;
  font-weight: 800;
}

.public-nav-actions.is-dark .public-nav-user-mark {
  border-color: rgba(106, 255, 240, 0.36);
  background: rgba(92, 255, 236, 0.16);
  color: #ffffff;
}

@media (max-width: 920px) {
  .public-nav-actions {
    gap: 5px;
    padding: 6px;
  }
}

@media (max-width: 640px) {
  .public-nav-actions {
    max-width: 56vw;
    overflow: visible;
  }

  .public-nav-button {
    padding: 0;
  }

  .public-nav-button span,
  .public-nav-actions :deep(> .relative > button span:not(:first-child)) {
    display: none;
  }

  .public-nav-auth {
    padding: 0 10px;
  }
}
</style>
