/**
 * Admin Model Catalog API
 * 运营端模型广场配置接口：置顶/排序权重/运营标签/精选有效期/隐藏
 *
 * 对接后端 Task A6：
 *   GET  /admin/catalog/config  → []AdminCatalogConfig（含 hidden，供管理页展示）
 *   PUT  /admin/catalog/config  → 批量保存 pinned/sort_weight/custom_tags/featured_until/hidden
 *
 * 后端 List 前会 EnsureFirstSeen 登记当前可见模型的首见时间（best-effort）。
 * 字段为 snake_case json tag，与后端 service.AdminCatalogConfig 对齐。
 */

import { apiClient } from './client'

/** 单条模型广场运营配置（与后端 AdminCatalogConfig 同形） */
export interface CatalogConfigItem {
  /** 平台标识（如 openai / anthropic / gemini） */
  platform: string
  /** 模型名（原始 model_name） */
  model_name: string
  /** 是否置顶（置顶区按 sort_weight 降序展示） */
  pinned: boolean
  /** 排序权重（置顶区拖拽后按序赋 100/99/98…） */
  sort_weight: number
  /** 运营标签（featured 特色 / recommended 推荐，由管理员勾选） */
  custom_tags: string[]
  /** 精选有效期（ISO 日期字符串，null 表示不限时） */
  featured_until: string | null
  /** 是否对用户隐藏 */
  hidden: boolean
  /** 首见时间（ISO，只读——后端 EnsureFirstSeen 登记，BatchSave 不写回） */
  first_seen_at: string
  /** 自动标签（new/multimodal/reasoning 等，后端聚合，只读展示） */
  tags?: string[]
  /** 手动 NEW 开关（持久化，默认 false） */
  is_new: boolean
  /** 手动精选开关（持久化；可选 featured_until 到期自动隐藏） */
  featured: boolean
}

/** 读取全部运营配置（后端已 EnsureFirstSeen） */
export const getCatalogConfig = (): Promise<CatalogConfigItem[]> =>
  apiClient.get<CatalogConfigItem[]>('/admin/catalog/config').then((r) => r.data)

/** 批量保存运营配置（置顶/权重/标签/精选/隐藏） */
export const saveCatalogConfig = (cfgs: CatalogConfigItem[]): Promise<unknown> =>
  apiClient.put('/admin/catalog/config', cfgs)
