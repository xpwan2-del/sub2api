/**
 * 管理员站内通知类型
 * 对应后端 admin_notification_handler.go 的 adminNotificationResponse
 */

export type AdminNotificationType =
  | 'upstream_price_change'
  | 'system'
  | 'alert'
  | string

export type AdminNotificationSeverity = 'info' | 'warning' | 'critical' | string

/** 未读通知项 */
export interface AdminNotification {
  id: number
  type: AdminNotificationType
  title: string
  content: string
  severity: AdminNotificationSeverity
  target_link?: string
  related_ids: number[]
  created_at: string
}

/** CountUnread 返回 */
export interface AdminNotificationUnreadCount {
  count: number
}
