<template>
  <div class="space-y-6 px-4 py-6 md:px-6">
    <!-- 页头 -->
    <div>
      <h1 class="text-xl font-bold text-gray-900 dark:text-white">
        {{ t('admin.catalogManage.title') }}
      </h1>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.catalogManage.description') }}
      </p>
    </div>

    <!-- 顶部设置条：新模型窗口 new_model_days -->
    <section class="card">
      <div class="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
        <div class="min-w-0">
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.catalogManage.newModelDays') }}
          </label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.catalogManage.newModelDaysHint') }}
          </p>
        </div>
        <div class="flex shrink-0 items-center gap-2">
          <input
            v-model.number="newModelDays"
            type="number"
            min="0"
            step="1"
            :disabled="settingsLoading"
            class="w-28 rounded-md border border-gray-300 px-2 py-1.5 text-sm dark:border-dark-600 dark:bg-dark-800 dark:text-white"
          />
          <button
            type="button"
            class="btn btn-primary btn-sm inline-flex items-center gap-1.5"
            :disabled="savingSettings || settingsLoading"
            @click="saveNewModelDays"
          >
            <span v-if="savingSettings" class="inline-block h-3.5 w-3.5 animate-spin rounded-full border-2 border-current border-t-transparent" />
            {{ t('admin.catalogManage.saveSettings') }}
          </button>
        </div>
      </div>
    </section>

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
      <!-- 置顶区（可拖拽排序） -->
      <section class="card">
        <div class="border-b border-gray-100 px-4 py-3 dark:border-dark-700">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ t('admin.catalogManage.pinnedSection') }}
          </h2>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.catalogManage.pinnedSectionHint') }}
          </p>
        </div>
        <div class="p-4">
          <VueDraggable
            v-if="pinnedItems.length"
            v-model="pinnedLocal"
            :animation="200"
            handle=".catalog-drag-handle"
            class="space-y-2"
            @end="onPinnedDragEnd"
          >
            <div
              v-for="item in pinnedLocal"
              :key="keyOf(item)"
              class="flex items-center gap-3 rounded-lg border border-gray-100 bg-white p-3 dark:border-dark-700 dark:bg-dark-800/40"
            >
              <span
                class="catalog-drag-handle flex cursor-grab items-center text-gray-300 hover:text-gray-500 active:cursor-grabbing dark:text-dark-600 dark:hover:text-dark-400"
                :title="t('admin.catalogManage.pinnedSectionHint')"
              >
                <svg class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
                  <path d="M7 2a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM13 2a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM7 8a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM13 8a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM7 14a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM13 14a2 2 0 1 0 0 4 2 2 0 0 0 0-4z" />
                </svg>
              </span>
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-1.5">
                  <span class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ item.model_name }}</span>
                  <span v-if="item.is_new" class="rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-bold uppercase text-amber-700 dark:bg-amber-900/40 dark:text-amber-300">NEW</span>
                </div>
                <div class="truncate text-xs text-gray-400">{{ item.platform }}</div>
              </div>
              <span v-if="hasTag(item, 'featured')" class="rounded bg-emerald-100 px-1.5 py-0.5 text-[10px] font-bold uppercase text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300">
                {{ t('admin.catalogManage.tagFeatured') }}
              </span>
              <span v-if="hasTag(item, 'recommended')" class="rounded bg-indigo-100 px-1.5 py-0.5 text-[10px] font-bold uppercase text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300">
                {{ t('admin.catalogManage.tagRecommended') }}
              </span>
              <button type="button" class="btn btn-secondary btn-sm" @click="togglePin(item)">
                {{ t('admin.catalogManage.unpin') }}
              </button>
            </div>
          </VueDraggable>
          <div v-else class="py-6 text-center text-sm text-gray-400 dark:text-gray-500">
            {{ t('admin.catalogManage.pinnedEmpty') }}
          </div>
        </div>
      </section>

      <!-- 全部模型列表 -->
      <section class="card">
        <div class="flex flex-col gap-3 border-b border-gray-100 px-4 py-3 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ t('admin.catalogManage.allModelsSection') }}
          </h2>
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

        <div v-if="filteredList.length === 0" class="py-10 text-center text-sm text-gray-400 dark:text-gray-500">
          {{ t('admin.catalogManage.empty') }}
        </div>
        <ul v-else class="divide-y divide-gray-100 dark:divide-dark-700">
          <li
            v-for="item in filteredList"
            :key="keyOf(item)"
            class="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3"
          >
            <!-- 模型 + 平台 -->
            <div class="min-w-[160px] flex-1">
              <div class="flex items-center gap-1.5">
                <span class="text-sm font-medium text-gray-900 dark:text-white">{{ item.model_name }}</span>
                <span v-if="item.is_new" class="rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-bold uppercase text-amber-700 dark:bg-amber-900/40 dark:text-amber-300">NEW</span>
                <span v-if="item.pinned" class="rounded bg-teal-100 px-1.5 py-0.5 text-[10px] font-bold uppercase text-teal-700 dark:bg-teal-900/40 dark:text-teal-300">{{ t('modelCatalog.pinned') }}</span>
              </div>
              <div class="text-xs text-gray-400">{{ item.platform }}</div>
            </div>

            <!-- 置顶切换 -->
            <label class="inline-flex cursor-pointer items-center gap-1.5 text-xs text-gray-600 dark:text-gray-300">
              <input type="checkbox" class="catalog-checkbox" :checked="item.pinned" @change="togglePin(item)" />
              {{ item.pinned ? t('admin.catalogManage.unpin') : t('admin.catalogManage.pin') }}
            </label>

            <!-- 运营标签 -->
            <div class="flex items-center gap-1.5">
              <label
                class="inline-flex cursor-pointer items-center gap-1 rounded-md border px-1.5 py-0.5 text-[11px] font-medium transition-colors"
                :class="hasTag(item, 'featured')
                  ? 'border-emerald-300 bg-emerald-50 text-emerald-700 dark:border-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
                  : 'border-gray-200 text-gray-500 dark:border-dark-600 dark:text-gray-400'"
              >
                <input type="checkbox" class="catalog-checkbox" :checked="hasTag(item, 'featured')" @change="toggleTag(item, 'featured')" />
                {{ t('admin.catalogManage.tagFeatured') }}
              </label>
              <label
                class="inline-flex cursor-pointer items-center gap-1 rounded-md border px-1.5 py-0.5 text-[11px] font-medium transition-colors"
                :class="hasTag(item, 'recommended')
                  ? 'border-indigo-300 bg-indigo-50 text-indigo-700 dark:border-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300'
                  : 'border-gray-200 text-gray-500 dark:border-dark-600 dark:text-gray-400'"
              >
                <input type="checkbox" class="catalog-checkbox" :checked="hasTag(item, 'recommended')" @change="toggleTag(item, 'recommended')" />
                {{ t('admin.catalogManage.tagRecommended') }}
              </label>
            </div>

            <!-- 精选有效期 -->
            <label class="inline-flex cursor-pointer items-center gap-1 text-xs text-gray-600 dark:text-gray-300">
              {{ t('admin.catalogManage.colFeaturedUntil') }}
              <input
                type="date"
                :value="dateOf(item.featured_until)"
                class="rounded-md border border-gray-300 px-1.5 py-1 text-xs dark:border-dark-600 dark:bg-dark-800 dark:text-white"
                @change="setFeaturedUntil(item, $event)"
              />
            </label>

            <!-- 隐藏切换 -->
            <label class="inline-flex cursor-pointer items-center gap-1.5 text-xs text-gray-600 dark:text-gray-300">
              <input type="checkbox" class="catalog-checkbox" :checked="item.hidden" @change="toggleHidden(item)" />
              {{ item.hidden ? t('admin.catalogManage.hidden') : t('admin.catalogManage.visible') }}
            </label>

            <!-- 自动标签（只读） -->
            <div v-if="item.tags && item.tags.length" class="flex min-w-[120px] flex-1 flex-wrap justify-end gap-1">
              <span
                v-for="tag in item.tags"
                :key="tag"
                class="rounded bg-gray-100 px-1.5 py-0.5 font-mono text-[10px] text-gray-500 dark:bg-dark-700 dark:text-gray-400"
              >{{ tag }}</span>
            </div>
            <span v-else class="min-w-[120px] flex-1 text-right text-xs text-gray-300 dark:text-dark-600">{{ t('admin.catalogManage.noAutoTags') }}</span>
          </li>
        </ul>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { VueDraggable } from 'vue-draggable-plus'
import { getCatalogConfig, saveCatalogConfig, type CatalogConfigItem } from '@/api/adminCatalog'
import { getSettings, updateSettings, type SystemSettings } from '@/api/admin/settings'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const appStore = useAppStore()

// 运营配置列表（单一数据源：置顶区与列表共享）
const items = ref<CatalogConfigItem[]>([])
const loading = ref(true)
const loadError = ref('')
const saving = ref(false)
const dirty = ref(false)
const search = ref('')

// 顶部设置：新模型判定窗口（天）。后端默认 30；后端未暴露该字段时回退默认值。
const newModelDays = ref(30)
const settingsLoading = ref(true)
const savingSettings = ref(false)
// 完整系统设置快照：保存 new_model_days 时基于它构造 full payload。
// 后端 PUT /admin/settings 按字段全量持久化（多数字段无 partial 保护），
// 传 partial 会把未提供的字段零值覆盖，故必须传完整对象。
const cachedSettings = ref<SystemSettings | null>(null)

// 置顶项（按 sort_weight 降序）
const pinnedItems = computed(() =>
  items.value
    .filter((i) => i.pinned)
    .sort((a, b) => b.sort_weight - a.sort_weight),
)

// 列表（支持按模型名/平台搜索）
const filteredList = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return items.value
  return items.value.filter(
    (i) => i.model_name.toLowerCase().includes(q) || i.platform.toLowerCase().includes(q),
  )
})

// 拖拽本地副本：VueDraggable 直接重排此数组，拖完再把顺序写回 items.sort_weight
const pinnedLocal = ref<CatalogConfigItem[]>([])
watch(
  pinnedItems,
  (val) => {
    pinnedLocal.value = [...val]
  },
  { immediate: true },
)

function keyOf(item: CatalogConfigItem): string {
  return `${item.platform}__${item.model_name}`
}

function hasTag(item: CatalogConfigItem, tag: string): boolean {
  return item.custom_tags.includes(tag)
}

function markDirty(): void {
  dirty.value = true
}

function togglePin(item: CatalogConfigItem): void {
  item.pinned = !item.pinned
  if (item.pinned) {
    // 新置顶项放到置顶区末尾：取当前最小权重 - 1（首次置顶用 100 起）
    const weights = pinnedItems.value.map((i) => i.sort_weight)
    const minW = weights.length ? Math.min(...weights) : 101
    item.sort_weight = minW - 1
  }
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

// featured_until 为后端 *time.Time（RFC3339）；date 控件用 yyyy-mm-dd，转回当天 23:59:59Z
function dateOf(v: string | null | undefined): string {
  return v ? v.slice(0, 10) : ''
}

function setFeaturedUntil(item: CatalogConfigItem, evt: Event): void {
  const val = (evt.target as HTMLInputElement).value
  item.featured_until = val ? `${val}T23:59:59Z` : null
  markDirty()
}

// 拖拽结束：按新顺序赋 sort_weight 100/99/98… 并立即保存（置顶区拖完即存）
function onPinnedDragEnd(): void {
  pinnedLocal.value.forEach((local, idx) => {
    const target = items.value.find((i) => keyOf(i) === keyOf(local))
    if (target) target.sort_weight = 100 - idx
  })
  void saveAll()
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

async function saveNewModelDays(): Promise<void> {
  if (savingSettings.value) return
  const v = Number(newModelDays.value)
  if (!Number.isFinite(v) || v < 0) {
    appStore.showError(t('admin.catalogManage.settingsSaveFailed'))
    return
  }
  savingSettings.value = true
  try {
    // 基于完整设置快照构造 full payload：后端按字段全量持久化（多数字段无 partial
    // 保护），传 partial 会把 registration/smtp/oauth 等未提供字段零值覆盖。
    // 完整快照既保证 model_catalog_new_model_days 端到端落库，也避免误伤其他设置。
    const base = cachedSettings.value ?? (await getSettings())
    const days = Math.floor(v)
    await updateSettings({ ...base, model_catalog_new_model_days: days })
    cachedSettings.value = { ...base, model_catalog_new_model_days: days }
    appStore.showSuccess(t('admin.catalogManage.settingsSaved'))
  } catch (err: unknown) {
    const message = (err as { message?: string })?.message
    appStore.showError(message || t('admin.catalogManage.settingsSaveFailed'))
  } finally {
    savingSettings.value = false
  }
}

async function load(): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    const [cfgs, settings] = await Promise.all([
      getCatalogConfig(),
      getSettings().catch(() => null),
    ])
    items.value = cfgs
    cachedSettings.value = settings
    newModelDays.value = settings?.model_catalog_new_model_days ?? 30
    dirty.value = false
  } catch (err: unknown) {
    const message = (err as { message?: string })?.message
    loadError.value = message || t('admin.catalogManage.loadFailed')
  } finally {
    loading.value = false
    settingsLoading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.catalog-checkbox {
  accent-color: currentColor;
}
</style>
