import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { adminNotificationsAPI } from '@/api/admin/adminNotifications'
import type { AdminNotification } from '@/types/adminNotification'

const POLL_INTERVAL_MS = 60 * 1000 // 1 minute

/**
 * 管理员站内通知 Store
 *
 * - 未读列表 + 未读数量
 * - 启动 / 停止轮询（仅在管理员布局挂载时运行）
 * - 标记单条 / 全部已读
 */
export const useAdminNotificationStore = defineStore('adminNotifications', () => {
  const notifications = ref<AdminNotification[]>([])
  const unreadCount = ref(0)
  const loading = ref(false)
  const lastFetchAt = ref(0)
  let pollTimer: ReturnType<typeof setInterval> | null = null

  const hasUnread = computed(() => unreadCount.value > 0)

  async function refreshCount() {
    try {
      const res = await adminNotificationsAPI.countUnread()
      unreadCount.value = res?.count ?? 0
    } catch (err) {
      // 静默失败：401/网络错误不应阻塞后台轮询
      console.error('adminNotifications: countUnread failed', err)
    }
  }

  async function fetchUnread(force = false) {
    const now = Date.now()
    if (!force && lastFetchAt.value > 0 && now - lastFetchAt.value < POLL_INTERVAL_MS) {
      await refreshCount()
      return
    }
    lastFetchAt.value = now

    loading.value = true
    try {
      const [list, countRes] = await Promise.all([
        adminNotificationsAPI.listUnread(20),
        adminNotificationsAPI.countUnread()
      ])
      notifications.value = list ?? []
      unreadCount.value = countRes?.count ?? 0
    } catch (err) {
      lastFetchAt.value = 0
      console.error('adminNotifications: fetchUnread failed', err)
    } finally {
      loading.value = false
    }
  }

  /** 启动后台轮询（幂等：重复调用安全） */
  function startPolling() {
    // 拉一次，再起定时器
    fetchUnread(true)
    if (pollTimer) return
    pollTimer = setInterval(() => {
      fetchUnread(false)
    }, POLL_INTERVAL_MS)
  }

  /** 停止轮询（离开 admin 布局或登出时调用） */
  function stopPolling() {
    if (pollTimer) {
      clearInterval(pollTimer)
      pollTimer = null
    }
  }

  async function markRead(id: number) {
    try {
      await adminNotificationsAPI.markRead(id)
      notifications.value = notifications.value.filter((n) => n.id !== id)
      if (unreadCount.value > 0) unreadCount.value -= 1
    } catch (err) {
      console.error('adminNotifications: markRead failed', err)
      throw err
    }
  }

  async function markAllRead() {
    try {
      await adminNotificationsAPI.markAllRead()
      notifications.value = []
      unreadCount.value = 0
    } catch (err) {
      console.error('adminNotifications: markAllRead failed', err)
      throw err
    }
  }

  function reset() {
    notifications.value = []
    unreadCount.value = 0
    lastFetchAt.value = 0
    stopPolling()
  }

  return {
    // state
    notifications,
    unreadCount,
    loading,
    // getters
    hasUnread,
    // actions
    fetchUnread,
    refreshCount,
    startPolling,
    stopPolling,
    markRead,
    markAllRead,
    reset
  }
})
