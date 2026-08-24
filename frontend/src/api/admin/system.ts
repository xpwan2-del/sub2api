/**
 * System API endpoints for admin operations
 */

import { apiClient } from '../client'

export interface ReleaseInfo {
  name: string
  body: string
  published_at: string
  html_url: string
}

export interface VersionInfo {
  current_version: string
  /** 上游模式 = 最新 GitHub release；managed 模式（UPDATE_REGISTRY_* 已配置）= 最新 CalVer Build */
  latest_version: string
  has_update: boolean
  release_info?: ReleaseInfo
  cached: boolean
  warning?: string
  build_type: string // "source" for manual builds, "release" for CI builds
  /** true = 升级由外部部署工具管理（fork 止血），前端展示 guide 指引而非在线更新按钮 */
  managed_externally?: boolean
  /** 部署体系注入的升级指引（UPGRADE_GUIDE_* 环境变量 → 后端透传） */
  guide?: OpsGuide | null
}

/**
 * Get current version
 */
export async function getVersion(): Promise<{ version: string }> {
  const { data } = await apiClient.get<{ version: string }>('/admin/system/version')
  return data
}

/**
 * Check for updates
 * @param force - Force refresh from GitHub API
 */
export async function checkUpdates(force = false): Promise<VersionInfo> {
  const { data } = await apiClient.get<VersionInfo>('/admin/system/check-updates', {
    params: force ? { force: 'true' } : undefined
  })
  return data
}

export interface UpdateResult {
  message: string
  need_restart: boolean
}

export interface RollbackVersionInfo {
  version: string
  published_at: string
  html_url: string
}

/** 部署体系注入的运维指引（{UPGRADE,ROLLBACK}_GUIDE_* 环境变量 → 后端透传） */
export interface OpsGuide {
  title?: string
  note?: string
  commands?: string[]
}

/**
 * Rollback-versions endpoint payload.
 * managed_externally=true 表示在线二进制回退不可用（fork 止血 / Docker 部署），
 * 回退由外部部署工具管理：versions 为空，前端应渲染 guide 指引。
 */
export interface RollbackVersionsResult {
  versions: RollbackVersionInfo[]
  managed_externally: boolean
  guide?: OpsGuide | null
}

/**
 * Get versions available for rollback (up to 3 versions older than current),
 * or the deployment-managed rollback guide when online rollback is disabled.
 */
export async function getRollbackVersions(): Promise<RollbackVersionsResult> {
  const { data } = await apiClient.get<RollbackVersionsResult>('/admin/system/rollback-versions')
  return data
}

/**
 * In-place update/rollback downloads a full release binary from GitHub, which
 * can take several minutes on slow links. The global 30s axios timeout would
 * abort the request mid-download (#4504), so these calls wait as long as the
 * backend allows (15 minutes server-side).
 */
const UPDATE_REQUEST_TIMEOUT_MS = 15 * 60 * 1000

/**
 * Perform system update
 * Downloads and applies the latest version
 */
export async function performUpdate(): Promise<UpdateResult> {
  const { data } = await apiClient.post<UpdateResult>('/admin/system/update', undefined, {
    timeout: UPDATE_REQUEST_TIMEOUT_MS
  })
  return data
}

/**
 * Rollback to a previous version
 * @param version - Target version (e.g. "0.1.146"); omit to restore the local backup binary
 */
export async function rollback(version?: string): Promise<UpdateResult> {
  const { data } = await apiClient.post<UpdateResult>(
    '/admin/system/rollback',
    version ? { version } : undefined,
    { timeout: UPDATE_REQUEST_TIMEOUT_MS }
  )
  return data
}

/**
 * Restart the service
 */
export async function restartService(): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>('/admin/system/restart')
  return data
}

export const systemAPI = {
  getVersion,
  checkUpdates,
  performUpdate,
  getRollbackVersions,
  rollback,
  restartService
}

export default systemAPI
