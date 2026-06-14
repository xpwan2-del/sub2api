<template>
  <div class="relative" ref="rootRef">
    <!-- 铃铛按钮 -->
    <button
      @click="toggleOpen"
      class="relative flex h-9 w-9 items-center justify-center rounded-lg text-gray-600 transition-all hover:bg-gray-100 hover:scale-105 dark:text-gray-400 dark:hover:bg-dark-800"
      :class="{ 'text-amber-600 dark:text-amber-400': hasUnread }"
      :aria-label="t('adminNotifications.title')"
    >
      <Icon name="bell" size="md" />
      <!-- 未读红点 -->
      <span v-if="hasUnread" class="absolute right-1 top-1 flex h-2 w-2">
        <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-red-500 opacity-75"></span>
        <span class="relative inline-flex h-2 w-2 rounded-full bg-red-500"></span>
      </span>
    </button>

    <!-- 下拉面板 -->
    <Transition name="dropdown">
      <div
        v-if="open"
        class="absolute right-0 z-50 mt-2 w-80 origin-top-right overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-2xl ring-1 ring-black/5 dark:border-dark-700 dark:bg-dark-800 dark:ring-white/10"
        @click.stop
      >
        <!-- Header -->
        <div class="flex items-center justify-between border-b border-gray-100 px-4 py-3 dark:border-dark-700">
          <div class="flex items-center gap-2">
            <div class="flex h-7 w-7 items-center justify-center rounded-lg bg-gradient-to-br from-amber-500 to-orange-600 text-white shadow">
              <Icon name="bell" size="sm" />
            </div>
            <span class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('adminNotifications.title') }}
            </span>
            <span
              v-if="hasUnread"
              class="rounded-full bg-red-100 px-1.5 py-0.5 text-xs font-medium text-red-700 dark:bg-red-900/40 dark:text-red-300"
            >
              {{ unreadCount }}
            </span>
          </div>
          <button
            v-if="hasUnread"
            @click="onMarkAllRead"
            :disabled="loading"
            class="rounded-md px-2 py-1 text-xs font-medium text-blue-600 transition-colors hover:bg-blue-50 disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
          >
            {{ t('adminNotifications.markAllRead') }}
          </button>
        </div>

        <!-- Body -->
        <div class="max-h-[60vh] overflow-y-auto">
          <!-- Loading -->
          <div v-if="loading && notifications.length === 0" class="flex items-center justify-center py-10">
            <div class="h-7 w-7 animate-spin rounded-full border-2 border-gray-200 border-t-blue-600 dark:border-dark-600 dark:border-t-blue-400"></div>
          </div>

          <!-- List -->
          <div v-else-if="notifications.length > 0">
            <div
              v-for="item in notifications"
              :key="item.id"
              class="group flex cursor-pointer items-start gap-3 border-b border-gray-50 px-4 py-3 transition-colors last:border-b-0 hover:bg-gray-50 dark:border-dark-700/50 dark:hover:bg-dark-700/30"
              @click="onSelect(item)"
            >
              <!-- Severity dot -->
              <span
                class="mt-1.5 inline-block h-2 w-2 flex-shrink-0 rounded-full"
                :class="severityDotClass(item.severity)"
              ></span>
              <div class="min-w-0 flex-1">
                <p class="truncate text-sm font-medium text-gray-900 dark:text-white">
                  {{ item.title }}
                </p>
                <p v-if="item.content" class="mt-0.5 line-clamp-2 text-xs text-gray-500 dark:text-gray-400">
                  {{ item.content }}
                </p>
                <p class="mt-1 text-[11px] text-gray-400 dark:text-gray-500">
                  {{ formatRelativeTime(item.created_at) }}
                </p>
              </div>
              <Icon name="chevronRight" size="sm" class="mt-1 flex-shrink-0 text-gray-300 group-hover:translate-x-0.5 dark:text-dark-500" />
            </div>
          </div>

          <!-- Empty -->
          <div v-else class="flex flex-col items-center justify-center px-4 py-10">
            <div class="mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700">
              <Icon name="checkCircle" size="lg" class="text-gray-400 dark:text-gray-500" />
            </div>
            <p class="text-sm font-medium text-gray-900 dark:text-white">
              {{ t('adminNotifications.empty') }}
            </p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('adminNotifications.emptyDescription') }}
            </p>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { useAppStore } from '@/stores/app'
import { useAdminNotificationStore } from '@/stores/adminNotifications'
import { formatRelativeTime } from '@/utils/format'
import type { AdminNotification } from '@/types/adminNotification'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()
const store = useAdminNotificationStore()

const { notifications, unreadCount, loading } = storeToRefs(store)
const hasUnread = computed(() => unreadCount.value > 0)

const open = ref(false)
const rootRef = ref<HTMLElement | null>(null)

function toggleOpen() {
  open.value = !open.value
  if (open.value) {
    store.fetchUnread(true)
  }
}

function close() {
  open.value = false
}

async function onSelect(item: AdminNotification) {
  // 标记已读（fire-and-forget）
  try {
    await store.markRead(item.id)
  } catch (err: any) {
    appStore.showError(err?.message || t('common.unknownError'))
  }

  const link = (item.target_link || '').trim()
  close()
  if (link) {
    if (/^https?:\/\//i.test(link)) {
      window.open(link, '_blank', 'noopener')
    } else {
      router.push(link)
    }
  }
}

async function onMarkAllRead() {
  try {
    await store.markAllRead()
    appStore.showSuccess(t('adminNotifications.allMarkedAsRead'))
  } catch (err: any) {
    appStore.showError(err?.message || t('common.unknownError'))
  }
}

function severityDotClass(severity: string): string {
  switch (severity) {
    case 'critical':
      return 'bg-red-500'
    case 'warning':
      return 'bg-amber-500'
    default:
      return 'bg-blue-500'
  }
}

function handleClickOutside(e: MouseEvent) {
  if (rootRef.value && !rootRef.value.contains(e.target as Node)) {
    close()
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  store.startPolling()
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
  store.stopPolling()
})
</script>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.18s ease;
}
.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: scale(0.96) translateY(-4px);
}
</style>
