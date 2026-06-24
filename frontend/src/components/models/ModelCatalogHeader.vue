<template>
  <header class="model-catalog-header" :class="{ 'is-dark': isDark }">
    <div>
      <p class="model-catalog-kicker">{{ t('modelCatalog.kicker') }}</p>
      <h1>{{ t('modelCatalog.title') }}</h1>
      <p v-if="showSubtitle" class="model-catalog-subtitle">{{ t('modelCatalog.subtitle') }}</p>
    </div>

    <div class="model-catalog-stats">
      <div>
        <span>{{ t('modelCatalog.stats.models') }}</span>
        <strong>{{ modelCount }}</strong>
      </div>
      <div>
        <span>{{ t('modelCatalog.stats.platforms') }}</span>
        <strong>{{ platformCount }}</strong>
      </div>
      <div>
        <span>{{ t('modelCatalog.stats.currency') }}</span>
        <strong>USD</strong>
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

defineProps<{
  modelCount: number
  platformCount: number
  isDark?: boolean
  showSubtitle?: boolean
}>()

const { t } = useI18n()
</script>

<style scoped>
.model-catalog-header {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 28px;
  align-items: end;
}

.model-catalog-kicker {
  color: #0f766e;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 13px;
  font-weight: 800;
  margin: 0 0 14px;
  text-transform: uppercase;
}

.model-catalog-header h1 {
  color: #0f172a;
  font-size: clamp(42px, 7vw, 92px);
  font-weight: 950;
  line-height: 0.95;
  margin: 0;
  max-width: 820px;
}

.model-catalog-subtitle {
  color: rgba(30, 41, 59, 0.72);
  font-size: 18px;
  line-height: 1.7;
  margin: 22px 0 0;
  max-width: 680px;
}

.model-catalog-stats {
  display: grid;
  gap: 10px;
  min-width: 230px;
}

.model-catalog-stats div {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border: 1px solid rgba(37, 99, 235, 0.18);
  background: rgba(255, 255, 255, 0.58);
  padding: 13px 15px;
  backdrop-filter: blur(14px);
}

.model-catalog-stats span {
  color: rgba(30, 41, 59, 0.64);
  font-size: 12px;
  text-transform: uppercase;
}

.model-catalog-stats strong {
  color: #0f766e;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.model-catalog-header.is-dark .model-catalog-kicker {
  color: #22d3ee;
}

.model-catalog-header.is-dark h1 {
  color: #f8fafc;
}

.model-catalog-header.is-dark .model-catalog-subtitle {
  color: rgba(226, 232, 240, 0.74);
}

.model-catalog-header.is-dark .model-catalog-stats div {
  border-color: rgba(94, 234, 212, 0.2);
  background: rgba(15, 23, 42, 0.56);
}

.model-catalog-header.is-dark .model-catalog-stats span {
  color: rgba(226, 232, 240, 0.64);
}

.model-catalog-header.is-dark .model-catalog-stats strong {
  color: #67e8f9;
}

@media (max-width: 860px) {
  .model-catalog-header {
    grid-template-columns: 1fr;
  }

  .model-catalog-stats {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (max-width: 560px) {
  .model-catalog-stats {
    grid-template-columns: 1fr;
  }
}
</style>
