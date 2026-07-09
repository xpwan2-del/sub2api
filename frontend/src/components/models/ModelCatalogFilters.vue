<template>
  <section class="model-catalog-filters" :class="{ 'is-dark': isDark }">
    <label class="model-search-field">
      <Icon name="search" size="sm" />
      <input
        :value="filters.search"
        type="search"
        :placeholder="t('modelCatalog.searchPlaceholder')"
        @input="update('search', ($event.target as HTMLInputElement).value)"
      />
    </label>

    <select :value="filters.platform" @change="update('platform', ($event.target as HTMLSelectElement).value)">
      <option value="">{{ t('modelCatalog.filters.allPlatforms') }}</option>
      <option v-for="platform in facets.platforms" :key="platform" :value="platform">
        {{ platform }}
      </option>
    </select>

    <select :value="filters.capability" @change="update('capability', ($event.target as HTMLSelectElement).value)">
      <option value="">{{ t('modelCatalog.filters.allCapabilities') }}</option>
      <option v-for="capability in facets.capabilities" :key="capability" :value="capability">
        {{ t(`modelCatalog.capabilities.${capability}`) }}
      </option>
    </select>

    <select :value="filters.billingMode" @change="update('billingMode', ($event.target as HTMLSelectElement).value)">
      <option value="">{{ t('modelCatalog.filters.allBilling') }}</option>
      <option v-for="mode in facets.billingModes" :key="mode" :value="mode">
        {{ billingModeLabel(mode) }}
      </option>
    </select>

    <select :value="filters.sortBy" @change="update('sortBy', ($event.target as HTMLSelectElement).value)">
      <option v-for="option in facets.sortOptions" :key="option" :value="option">
        {{ t(`modelCatalog.sort.${option}`) }}
      </option>
    </select>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { ModelCatalogFacets, ModelCatalogFilters, ModelCatalogSort } from '@/utils/modelCatalog'

const props = defineProps<{
  filters: ModelCatalogFilters
  facets: ModelCatalogFacets
  isDark?: boolean
}>()

const emit = defineEmits<{
  'update:filters': [filters: ModelCatalogFilters]
}>()

const { t } = useI18n()
const knownBillingModes = new Set(['token', 'image', 'per_request', 'unknown'])

function update(key: keyof ModelCatalogFilters, value: string) {
  emit('update:filters', {
    ...props.filters,
    [key]: key === 'sortBy' ? value as ModelCatalogSort : value
  })
}

function billingModeLabel(mode: string) {
  return knownBillingModes.has(mode) ? t(`modelCatalog.billingModes.${mode}`) : mode
}
</script>

<style scoped>
.model-catalog-filters {
  display: grid;
  grid-template-columns: minmax(220px, 1.5fr) repeat(4, minmax(140px, 1fr));
  gap: 12px;
  border: 1px solid rgba(37, 99, 235, 0.16);
  background: rgba(255, 255, 255, 0.56);
  padding: 14px;
  backdrop-filter: blur(14px);
}

.model-search-field,
select {
  width: 100%;
  min-height: 42px;
  border: 1px solid rgba(37, 99, 235, 0.16);
  background: rgba(255, 255, 255, 0.62);
  color: #0f172a;
}

.model-search-field {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 0 12px;
}

.model-search-field input {
  min-width: 0;
  flex: 1;
  border: 0;
  background: transparent;
  color: inherit;
  outline: none;
}

select {
  padding: 0 12px;
}

.model-catalog-filters.is-dark {
  border-color: rgba(94, 234, 212, 0.18);
  background: rgba(2, 6, 23, 0.62);
}

.model-catalog-filters.is-dark .model-search-field,
.model-catalog-filters.is-dark select {
  border-color: rgba(125, 211, 252, 0.18);
  background: rgba(15, 23, 42, 0.78);
  color: #e0fffb;
}

@media (max-width: 1040px) {
  .model-catalog-filters {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .model-search-field {
    grid-column: 1 / -1;
  }
}

@media (max-width: 620px) {
  .model-catalog-filters {
    grid-template-columns: 1fr;
  }
}
</style>
