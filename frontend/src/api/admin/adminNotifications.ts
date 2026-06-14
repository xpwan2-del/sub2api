/**
 * Admin Notifications API
 * 管理员站内通知 API（价格同步告警等）
 *
 * 对应后端路由：
 *   GET  /admin/admin-notifications/unread?limit=
 *   GET  /admin/admin-notifications/unread/count
 *   POST /admin/admin-notifications/:id/read
 *   POST /admin/admin-notifications/read-all
 */

import { apiClient } from '../client'
import type {
  AdminNotification,
  AdminNotificationUnreadCount
} from '@/types/adminNotification'

const BASE = '/admin/admin-notifications'

/** 列出当前管理员未读通知（空时返回 []） */
export async function listUnread(limit?: number): Promise<AdminNotification[]> {
  const { data } = await apiClient.get<AdminNotification[]>(`${BASE}/unread`, {
    params: limit ? { limit } : undefined
  })
  return data ?? []
}

/** 未读通知条数 */
export async function countUnread(): Promise<AdminNotificationUnreadCount> {
  const { data } = await apiClient.get<AdminNotificationUnreadCount>(`${BASE}/unread/count`)
  return data
}

/** 标记单条为已读 */
export async function markRead(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>(`${BASE}/${id}/read`)
  return data
}

/** 全部标记为已读 */
export async function markAllRead(): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>(`${BASE}/read-all`)
  return data
}

export const adminNotificationsAPI = {
  listUnread,
  countUnread,
  markRead,
  markAllRead
}

export default adminNotificationsAPI
