<template>
  <AppLayout>
    <div class="space-y-6">
    <!-- 页头 -->
    <div>
      <h1 class="text-xl font-bold text-gray-900 dark:text-white">
        {{ t('admin.catalogManage.title') }}
      </h1>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.catalogManage.description') }}
      </p>
    </div>

    <!-- 加载中 -->
    <section v-if="loading" class="card flex items-center justify-center gap-2 py-10 text-sm text-gray-500 dark:text-gray-400">
      <span class="inline-block h-4 w-4 animate-spin rounded-full border-2 border-primary-500 border-t-transparent" />
      {{ t('admin.catalogManage.loading') }}
    </section>

    <!-- 加载失败 -->
    <section v-else-if="loadError" class="card flex flex-col items-center justify-center gap-3 py-10 text-sm">
      <span class="text-red-500 dark:text-red-400">{{ loadError }}</span>
      <button type="button" class="btn btn-secondary btn-sm" @click="load">
        {{ t('admin.catalogManage.retry') }}
      </button>
    </section>

    <template v-else>
      <!-- 统一可拖拽列表（含置顶） -->
      <section class="card">
        <div class="flex flex-col gap-3 border-b border-gray-100 px-4 py-3 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('admin.catalogManage.allModelsSection') }}
            </h2>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.catalogManage.sortHint') }}
            </p>
          </div>
          <div class="flex items-center gap-2">
            <input
              v-model="search"
              type="text"
              :placeholder="t('admin.catalogManage.searchPlaceholder')"
              class="w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm sm:w-64 dark:border-dark-600 dark:bg-dark-800 dark:text-white"
            />
            <button
              type="button"
              class="btn btn-primary btn-sm inline-flex shrink-0 items-center gap-1.5"
              :class="{ 'opacity-60': !dirty }"
              :disabled="!dirty || saving"
              @click="saveAll"
            >
              <span v-if="saving" class="inline-block h-3.5 w-3.5 animate-spin rounded-full border-2 border-current border-t-transparent" />
              {{ saving ? t('admin.catalogManage.saving') : t('admin.catalogManage.saveAll') }}
            </button>
          </div>
        </div>

        <div v-if="visibleList.length === 0" class="py-10 text-center text-sm text-gray-400 dark:text-gray-500">
          {{ t('admin.catalogManage.empty') }}
        </div>

        <!-- 非搜索态：全量可拖拽 -->
        <VueDraggable
          v-else-if="!hasSearch"
          v-model="localItems"
          :animation="200"
          handle=".catalog-drag-handle"
          class="divide-y divide-gray-100 dark:divide-dark-700"
          @end="onDragEnd"
        >
          <CatalogModelRow
            v-for="item in localItems"
            :key="keyOf(item)"
            :item="item"
            draggable
            @toggle-pin="togglePin(item)"
            @toggle-flag="toggleFlag(item, $event)"
            @toggle-tag="toggleTag(item, $event)"
            @toggle-hidden="toggleHidden(item)"
            @change-featured-until="setFeaturedUntilValue(item, $event)"
          />
        </VueDraggable>

        <!-- 搜索态：只读过滤列表（不可拖拽，避免子集乱序） -->
        <ul v-else class="divide-y divide-gray-100 dark:divide-dark-700">
          <li v-for="item in filteredList" :key="keyOf(item)">
            <CatalogModelRow
              :item="item"
              @toggle-pin="togglePin(item)"
              @toggle-flag="toggleFlag(item, $event)"
              @toggle-tag="toggleTag(item, $event)"
              @toggle-hidden="toggleHidden(item)"
              @change-featured-until="setFeaturedUntilValue(item, $event)"
            />
          </li>
        </ul>
      </section>
    </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { VueDraggable } from 'vue-draggable-plus'
import CatalogModelRow from '@/components/admin/CatalogModelRow.vue'
import { getCatalogConfig, saveCatalogConfig, type CatalogConfigItem } from '@/api/adminCatalog'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const appStore = useAppStore()

// 运营配置列表（单一数据源：拖拽副本与保存均基于此）。
const items = ref<CatalogConfigItem[]>([])
const loading = ref(true)
const loadError = ref('')
const saving = ref(false)
const dirty = ref(false)
const search = ref('')

const hasSearch = computed(() => search.value.trim().length > 0)

// 列表（按模型名/平台搜索）
const filteredList = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return items.value
  return items.value.filter(
    (i) => i.model_name.toLowerCase().includes(q) || i.platform.toLowerCase().includes(q),
  )
})

// 本地拖拽副本：全量、按 (pinned desc, sort_weight desc) 初始排序，与公开页顺序一致。
// 元素与 items.value 共享引用，故开关改动同步；拖拽仅重排此数组，@end 时把顺序写回 items.sort_weight。
const localItems = ref<CatalogConfigItem[]>([])
watch(
  items,
  (val) => {
    localItems.value = [...val].sort((a, b) => {
      if (a.pinned !== b.pinned) return a.pinned ? -1 : 1
      return b.sort_weight - a.sort_weight
    })
  },
  { immediate: true },
)

const visibleList = computed(() => (hasSearch.value ? filteredList.value : localItems.value))

function keyOf(item: CatalogConfigItem): string {
  return `${item.platform}__${item.model_name}`
}

function markDirty(): void {
  dirty.value = true
}

function togglePin(item: CatalogConfigItem): void {
  // 排序统一由拖拽管理：置顶仅翻转标记（公开页置顶浮顶）。
  item.pinned = !item.pinned
  markDirty()
}

function toggleFlag(item: CatalogConfigItem, key: 'is_new' | 'featured'): void {
  item[key] = !item[key]
  markDirty()
}

function toggleTag(item: CatalogConfigItem, tag: string): void {
  const idx = item.custom_tags.indexOf(tag)
  if (idx >= 0) item.custom_tags.splice(idx, 1)
  else item.custom_tags.push(tag)
  markDirty()
}

function toggleHidden(item: CatalogConfigItem): void {
  item.hidden = !item.hidden
  markDirty()
}

// featured_until 为后端 *time.Time（RFC3339）；date 控件用 yyyy-mm-dd，转回当天 23:59:59Z。
function setFeaturedUntilValue(item: CatalogConfigItem, value: string): void {
  item.featured_until = value ? `${value}T23:59:59Z` : null
  markDirty()
}

// 拖拽结束：按 localItems 新顺序重写 sort_weight（高位在前），写回 items.value 对应项。
function onDragEnd(): void {
  const total = localItems.value.length
  localItems.value.forEach((local, idx) => {
    const target = items.value.find((i) => keyOf(i) === keyOf(local))
    if (target) target.sort_weight = total - idx
  })
  markDirty()
}

async function saveAll(): Promise<void> {
  if (saving.value) return
  saving.value = true
  try {
    await saveCatalogConfig(items.value)
    dirty.value = false
    appStore.showSuccess(t('admin.catalogManage.saved'))
  } catch (err: unknown) {
    const message = (err as { message?: string })?.message
    appStore.showError(message || t('admin.catalogManage.saveFailed'))
  } finally {
    saving.value = false
  }
}

async function load(): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    items.value = await getCatalogConfig()
    dirty.value = false
  } catch (err: unknown) {
    const message = (err as { message?: string })?.message
    loadError.value = message || t('admin.catalogManage.loadFailed')
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>
