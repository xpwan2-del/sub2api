import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}))

vi.mock('../client', () => ({
  apiClient: {
    get,
    post,
  },
}))

import { getRollbackVersions, rollback, type RollbackVersionInfo } from '@/api/admin/system'

describe('admin system rollback API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
  })

  it('getRollbackVersions fetches the rollback version list', async () => {
    const versions: RollbackVersionInfo[] = [
      {
        version: '0.1.146',
        published_at: '2026-07-07T00:00:00Z',
        html_url: 'https://github.com/Wei-Shaw/sub2api/releases/tag/v0.1.146'
      }
    ]
    get.mockResolvedValue({ data: { versions, managed_externally: false } })

    const result = await getRollbackVersions()

    expect(get).toHaveBeenCalledWith('/admin/system/rollback-versions')
    expect(result.versions).toEqual(versions)
    expect(result.managed_externally).toBe(false)
  })

  it('getRollbackVersions returns the deployment-managed guide', async () => {
    get.mockResolvedValue({
      data: {
        versions: [],
        managed_externally: true,
        guide: {
          title: '版本回退由部署仓库管理',
          note: '降级只回退镜像, 不回滚数据库',
          commands: ['./ops rollback sub2api', './ops rollback sub2api --confirm']
        }
      }
    })

    const result = await getRollbackVersions()

    expect(get).toHaveBeenCalledWith('/admin/system/rollback-versions')
    expect(result.managed_externally).toBe(true)
    expect(result.versions).toEqual([])
    expect(result.guide?.commands).toEqual(['./ops rollback sub2api', './ops rollback sub2api --confirm'])
  })

  it('rollback posts the target version in the request body', async () => {
    post.mockResolvedValue({ data: { message: 'ok', need_restart: true } })

    const result = await rollback('0.1.146')

    expect(post).toHaveBeenCalledWith(
      '/admin/system/rollback',
      { version: '0.1.146' },
      { timeout: 15 * 60 * 1000 }
    )
    expect(result.need_restart).toBe(true)
  })

  it('rollback without a version posts no body (legacy backup rollback)', async () => {
    post.mockResolvedValue({ data: { message: 'ok', need_restart: true } })

    await rollback()

    expect(post).toHaveBeenCalledWith(
      '/admin/system/rollback',
      undefined,
      { timeout: 15 * 60 * 1000 }
    )
  })
})
