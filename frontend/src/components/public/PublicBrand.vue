<template>
  <router-link to="/home" class="public-brand" :class="{ 'is-dark': isDark }">
    <img :src="brandLogo" :alt="siteName" />
    <span>{{ siteName }}</span>
  </router-link>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { usePublicBranding } from '@/composables/usePublicBranding'

defineProps<{
  isDark?: boolean
}>()

const { appStore, siteName, brandLogo } = usePublicBranding()

onMounted(() => {
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
.public-brand {
  display: inline-flex;
  align-items: center;
  justify-content: flex-start;
  gap: 10px;
  width: auto;
  min-height: 54px;
  overflow: hidden;
  border: 1px solid rgba(59, 130, 246, 0.28);
  border-radius: 16px;
  background:
    linear-gradient(135deg, rgba(240, 253, 250, 0.9), rgba(219, 234, 254, 0.72)),
    rgba(255, 255, 255, 0.64);
  box-shadow: 0 0 30px rgba(45, 212, 191, 0.12), inset 0 0 24px rgba(59, 130, 246, 0.08);
  color: #0f172a;
  padding: 8px 16px 8px 10px;
  text-decoration: none;
  backdrop-filter: blur(16px);
}

.public-brand.is-dark {
  border-color: rgba(92, 255, 236, 0.28);
  background:
    linear-gradient(135deg, rgba(8, 35, 58, 0.86), rgba(23, 37, 84, 0.7)),
    rgba(4, 14, 28, 0.58);
  box-shadow: 0 0 30px rgba(45, 255, 229, 0.1), inset 0 0 24px rgba(45, 255, 229, 0.04);
  color: #e9fffb;
}

.public-brand img {
  width: 38px;
  height: 38px;
  flex: 0 0 auto;
  object-fit: contain;
  object-position: center;
  filter: drop-shadow(0 0 12px rgba(92, 180, 255, 0.38));
}

.public-brand span {
  background: linear-gradient(90deg, #0f766e, #2563eb 62%, #172554);
  background-clip: text;
  color: transparent;
  font-size: 18px;
  font-weight: 900;
  letter-spacing: 0.08em;
  line-height: 1;
  white-space: nowrap;
}

.public-brand.is-dark span {
  background-image: linear-gradient(90deg, #ecfeff, #60a5fa 58%, #c7d2fe);
}

@media (max-width: 920px) {
  .public-brand {
    min-height: 52px;
    padding: 7px 13px 7px 9px;
  }

  .public-brand img {
    width: 36px;
    height: 36px;
  }

  .public-brand span {
    font-size: 16px;
  }
}

@media (max-width: 640px) {
  .public-brand {
    min-height: 44px;
    gap: 7px;
    padding: 6px 10px 6px 7px;
  }

  .public-brand img {
    width: 28px;
    height: 28px;
  }

  .public-brand span {
    font-size: 13px;
  }
}
</style>
