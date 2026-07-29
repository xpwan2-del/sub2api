<script setup lang="ts">
/**
 * CatalogModelRow —— 模型广场管理页的单行模型运营配置。
 *
 * 仅展示与 emit 事件，不直接修改 prop（避免 vue/no-mutating-props）。
 * 父组件 CatalogManageView 持有 item 引用并响应事件完成修改 + markDirty。
 * 拖拽手柄 `.catalog-drag-handle` 由父组件的 VueDraggable handle 选择器捕获。
 */
import { useI18n } from 'vue-i18n'
import type { CatalogConfigItem } from '@/api/adminCatalog'

const props = defineProps<{
  item: CatalogConfigItem
  /** 是否显示拖拽手柄（搜索态只读列表不显示）。 */
  draggable?: boolean
}>()

const emit = defineEmits<{
  togglePin: []
  toggleFlag: [key: 'is_new' | 'featured']
  toggleTag: [tag: string]
  toggleHidden: []
  changeFeaturedUntil: [value: string]
}>()

const { t } = useI18n()

function hasTag(tag: string): boolean {
  return props.item.custom_tags.includes(tag)
}

function dateOf(v: string | null | undefined): string {
  return v ? v.slice(0, 10) : ''
}
</script>

<template>
  <div class="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3">
    <!-- 拖拽手柄 -->
    <span
      v-if="draggable"
      class="catalog-drag-handle flex cursor-grab items-center text-gray-300 hover:text-gray-500 active:cursor-grabbing dark:text-dark-600 dark:hover:text-dark-400"
      :title="t('admin.catalogManage.sortHint')"
    >
      <svg class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
        <path d="M7 2a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM13 2a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM7 8a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM13 8a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM7 14a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM13 14a2 2 0 1 0 0 4 2 2 0 0 0 0-4z" />
      </svg>
    </span>

    <!-- 模型名 + 平台 + badges -->
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
      <input type="checkbox" class="catalog-checkbox" :checked="item.pinned" @change="emit('togglePin')" />
      {{ item.pinned ? t('admin.catalogManage.unpin') : t('admin.catalogManage.pin') }}
    </label>

    <!-- NEW 手动开关 -->
    <label
      class="inline-flex cursor-pointer items-center gap-1 rounded-md border px-1.5 py-0.5 text-[11px] font-medium transition-colors"
      :class="item.is_new
        ? 'border-amber-300 bg-amber-50 text-amber-700 dark:border-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
        : 'border-gray-200 text-gray-500 dark:border-dark-600 dark:text-gray-400'"
    >
      <input type="checkbox" class="catalog-checkbox" :checked="item.is_new" @change="emit('toggleFlag', 'is_new')" />
      {{ t('admin.catalogManage.tagNew') }}
    </label>

    <!-- featured 手动开关 -->
    <label
      class="inline-flex cursor-pointer items-center gap-1 rounded-md border px-1.5 py-0.5 text-[11px] font-medium transition-colors"
      :class="item.featured
        ? 'border-emerald-300 bg-emerald-50 text-emerald-700 dark:border-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
        : 'border-gray-200 text-gray-500 dark:border-dark-600 dark:text-gray-400'"
    >
      <input type="checkbox" class="catalog-checkbox" :checked="item.featured" @change="emit('toggleFlag', 'featured')" />
      {{ t('admin.catalogManage.tagFeatured') }}
    </label>

    <!-- recommended（仍写 custom_tags） -->
    <label
      class="inline-flex cursor-pointer items-center gap-1 rounded-md border px-1.5 py-0.5 text-[11px] font-medium transition-colors"
      :class="hasTag('recommended')
        ? 'border-indigo-300 bg-indigo-50 text-indigo-700 dark:border-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300'
        : 'border-gray-200 text-gray-500 dark:border-dark-600 dark:text-gray-400'"
    >
      <input type="checkbox" class="catalog-checkbox" :checked="hasTag('recommended')" @change="emit('toggleTag', 'recommended')" />
      {{ t('admin.catalogManage.tagRecommended') }}
    </label>

    <!-- 可选精选到期 -->
    <label class="inline-flex cursor-pointer items-center gap-1 text-xs text-gray-600 dark:text-gray-300">
      {{ t('admin.catalogManage.colFeaturedUntil') }}
      <input
        type="date"
        :value="dateOf(item.featured_until)"
        class="rounded-md border border-gray-300 px-1.5 py-1 text-xs dark:border-dark-600 dark:bg-dark-800 dark:text-white"
        @change="emit('changeFeaturedUntil', ($event.target as HTMLInputElement).value)"
      />
    </label>

    <!-- 隐藏切换 -->
    <label class="inline-flex cursor-pointer items-center gap-1.5 text-xs text-gray-600 dark:text-gray-300">
      <input type="checkbox" class="catalog-checkbox" :checked="item.hidden" @change="emit('toggleHidden')" />
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
  </div>
</template>

<style scoped>
.catalog-checkbox {
  accent-color: currentColor;
}
</style>
