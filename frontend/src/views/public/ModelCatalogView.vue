<template>
  <div class="model-catalog-page" :class="{ 'is-dark': isDark }">
    <div class="model-catalog-bg" aria-hidden="true"></div>

    <PublicTopBar :is-dark="isDark" @toggle-theme="toggleTheme" />

    <main class="model-catalog-main">
      <ModelCatalogHeader
        :model-count="catalog.items.length"
        :platform-count="catalog.facets.platforms.length"
        :is-dark="isDark"
      />

      <ModelCatalogFilters
        v-model:filters="filters"
        :facets="catalog.facets"
        :is-dark="isDark"
      />

      <section v-if="loading" class="model-catalog-state">
        <Icon name="sync" size="lg" />
        <span>{{ t('modelCatalog.loading') }}</span>
      </section>

      <section v-else-if="errorMessage" class="model-catalog-state is-error">
        <Icon name="exclamationTriangle" size="lg" />
        <span>{{ errorMessage }}</span>
        <button type="button" @click="loadCatalog">{{ t('modelCatalog.retry') }}</button>
      </section>

      <section v-else-if="filteredModels.length === 0" class="model-catalog-state">
        <Icon name="inbox" size="lg" />
        <span>{{ t('modelCatalog.empty') }}</span>
      </section>

      <section v-else class="model-catalog-grid">
        <ModelCard
          v-for="model in filteredModels"
          :key="model.id"
          :model="model"
          @copy="copyModelName"
        />
      </section>
    </main>

    <footer class="model-catalog-footer">
      © {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ModelCard from '@/components/models/ModelCard.vue'
import ModelCatalogFilters from '@/components/models/ModelCatalogFilters.vue'
import ModelCatalogHeader from '@/components/models/ModelCatalogHeader.vue'
import PublicTopBar from '@/components/public/PublicTopBar.vue'
import { publicModelsAPI, type PublicModelCatalogItem } from '@/api/publicModels'
import { buildModelCatalog, filterModelCatalog, type ModelCatalogFilters as CatalogFilters } from '@/utils/modelCatalog'
import { useClipboard } from '@/composables/useClipboard'
import { usePublicBranding } from '@/composables/usePublicBranding'
import { useThemeMode } from '@/composables/useThemeMode'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const { isDark, toggleTheme } = useThemeMode()
const { siteName } = usePublicBranding()

const rows = ref<PublicModelCatalogItem[]>([])
const loading = ref(true)
const errorMessage = ref('')
let abortController: AbortController | null = null

const filters = reactive<CatalogFilters>({
  search: '',
  platform: '',
  capability: '',
  billingMode: '',
  sortBy: 'price'
})

const currentYear = new Date().getFullYear()
const catalog = computed(() => buildModelCatalog(rows.value))
const filteredModels = computed(() => filterModelCatalog(catalog.value.items, filters))

watch(
  () => catalog.value.facets.sortOptions,
  (options) => {
    if (!options.includes(filters.sortBy)) {
      filters.sortBy = options[0] || 'name'
    }
  },
  { immediate: true }
)

async function loadCatalog() {
  abortController?.abort()
  abortController = new AbortController()

  loading.value = true
  errorMessage.value = ''

  try {
    rows.value = await publicModelsAPI.getCatalog({ signal: abortController.signal })
  } catch (error: any) {
    if (error?.code === 'ERR_CANCELED') return
    errorMessage.value = error?.message || t('modelCatalog.loadFailed')
  } finally {
    loading.value = false
  }
}

function copyModelName(name: string) {
  copyToClipboard(name, t('modelCatalog.copied'))
}

onMounted(() => {
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
  loadCatalog()
})

onUnmounted(() => {
  abortController?.abort()
})
</script>

<style scoped>
.model-catalog-page {
  position: relative;
  min-height: 100vh;
  overflow: hidden;
  background: #f7f7f5;
  color: #0f172a;
}

.model-catalog-page.is-dark {
  background: #030712;
  color: #f8fafc;
}

.model-catalog-bg {
  position: fixed;
  inset: 0;
  pointer-events: none;
  background:
    radial-gradient(circle at 50% 28%, rgba(20, 184, 166, 0.2), transparent 27%),
    radial-gradient(circle at 78% 18%, rgba(59, 130, 246, 0.13), transparent 24%),
    linear-gradient(90deg, rgba(247, 247, 245, 0.96), rgba(247, 247, 245, 0.08) 30%, rgba(247, 247, 245, 0.08) 70%, rgba(247, 247, 245, 0.96)),
    linear-gradient(rgba(20, 184, 166, 0.06) 1px, transparent 1px),
    linear-gradient(90deg, rgba(20, 184, 166, 0.06) 1px, transparent 1px),
    #f7f7f5;
  background-size: auto, auto, auto, 64px 64px, 64px 64px, auto;
}

.model-catalog-page.is-dark .model-catalog-bg {
  background:
    radial-gradient(circle at 50% 28%, rgba(20, 184, 166, 0.28), transparent 26%),
    radial-gradient(circle at 78% 18%, rgba(59, 130, 246, 0.18), transparent 24%),
    linear-gradient(90deg, rgba(2, 6, 23, 0.92), rgba(8, 24, 44, 0.22) 30%, rgba(8, 24, 44, 0.22) 70%, rgba(2, 6, 23, 0.92)),
    linear-gradient(rgba(94, 255, 238, 0.08) 1px, transparent 1px),
    linear-gradient(90deg, rgba(94, 255, 238, 0.08) 1px, transparent 1px),
    #030712;
  background-size: auto, auto, auto, 64px 64px, 64px 64px, auto;
}

.model-catalog-main,
.model-catalog-footer {
  position: relative;
  z-index: 1;
}

.model-catalog-main {
  width: min(1220px, calc(100% - 36px));
  margin: 0 auto;
  padding: 44px 0 54px;
}

.model-catalog-main > * + * {
  margin-top: 26px;
}

.model-catalog-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 18px;
}

.model-catalog-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  min-height: 220px;
  border: 1px solid rgba(94, 234, 212, 0.18);
  background: rgba(2, 6, 23, 0.62);
  color: rgba(226, 232, 240, 0.78);
}

.model-catalog-state.is-error {
  color: #fecaca;
}

.model-catalog-state button {
  border: 1px solid rgba(248, 113, 113, 0.28);
  background: rgba(127, 29, 29, 0.25);
  color: #ffffff;
  font-weight: 800;
  padding: 9px 12px;
}

.model-catalog-footer {
  color: rgba(203, 213, 225, 0.56);
  font-size: 13px;
  padding: 0 clamp(18px, 4vw, 44px) 28px;
}

@media (max-width: 1080px) {
  .model-catalog-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 720px) {
  .model-catalog-grid {
    grid-template-columns: 1fr;
  }
}
</style>
