import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'

import BundleSubscriptionsView from '../BundleSubscriptionsView.vue'

const { showError, showSuccess } = vi.hoisted(() => ({
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: { value: 'en' }
    })
  }
})

vi.mock('@/api/admin/bundles', () => ({
  bundlesAPI: {
    listSubscriptions: vi.fn().mockResolvedValue({
      items: [
        {
          id: 7,
          user_id: 1,
          plan_id: 2,
          status: 'upgraded',
          starts_at: '',
          expires_at: '',
          concurrency_limit: 1,
          rpm_limit: 0,
          source: 'upgrade'
        }
      ],
      total: 1
    }),
    getSubscriptionUsageProgress: vi.fn().mockResolvedValue([
      {
        group_id: 1,
        group_name: 'Group A',
        platform: 'openai',
        model_pattern: '',
        daily_usage_usd: 1,
        daily_limit_usd: 10,
        weekly_usage_usd: 2,
        weekly_limit_usd: 20,
        monthly_usage_usd: 3,
        monthly_limit_usd: 30,
        daily_image_usage_count: 0,
        daily_image_limit_count: 0,
        weekly_image_usage_count: 0,
        weekly_image_limit_count: 0,
        monthly_image_usage_count: 0,
        monthly_image_limit_count: 0,
        daily_video_usage_count: 0,
        daily_video_limit_count: 0,
        weekly_video_usage_count: 0,
        weekly_video_limit_count: 0,
        monthly_video_usage_count: 0,
        monthly_video_limit_count: 0
      }
    ]),
    extendSubscription: vi.fn(),
    revokeSubscription: vi.fn()
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess })
}))

describe('BundleSubscriptionsView upgraded status', () => {
  beforeEach(() => {
    showError.mockReset()
    showSuccess.mockReset()
    localStorage.clear()
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: true,
        media: query,
        onchange: null,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        addListener: vi.fn(),
        removeListener: vi.fn(),
        dispatchEvent: vi.fn()
      }))
    })
    vi.stubGlobal(
      'requestAnimationFrame',
      (cb: FrameRequestCallback) => window.setTimeout(() => cb(0), 0)
    )
  })

  it('renders upgraded status with translated label and sky badge', async () => {
    const wrapper = mount(BundleSubscriptionsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template:
              '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          Pagination: true,
          BaseDialog: true,
          Icon: true,
          EmptyState: true,
          Select: true
        }
      }
    })

    await flushPromises()
    await nextTick()

    // statusLabel('upgraded') -> t('bundles.admin.statusUpgraded') -> 该 key（t 被 mock 为返回 key）
    expect(wrapper.text()).toContain('bundles.admin.statusUpgraded')
    // statusBadgeClass('upgraded') 蓝色徽章
    expect(wrapper.html()).toContain('bg-sky-100')
    expect(wrapper.html()).toContain('text-sky-800')

    wrapper.unmount()
  })

  it('expands a row inline and renders group usage detail on click', async () => {
    const wrapper = mount(BundleSubscriptionsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template:
              '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          Pagination: true,
          BaseDialog: true,
          Icon: true,
          EmptyState: true,
          Select: true
        }
      }
    })

    // let listSubscriptions resolve and rows render
    await flushPromises()
    await nextTick()

    // the expand toggle is the only row button (mock row is 'upgraded' → no action buttons);
    // its title is the "expand" i18n key since useI18n.t is mocked to return the key
    const expandBtn = wrapper.find('button[title="common.expand"]')
    expect(expandBtn.exists()).toBe(true)
    await expandBtn.trigger('click')
    // let toggleRow's getSubscriptionUsageProgress resolve
    await flushPromises()
    await nextTick()

    // DataTable non-virtual branch renders tr.expanded-row when expandedRowKey matches
    expect(wrapper.find('tr.expanded-row').exists()).toBe(true)
    // usage detail rendered with the mocked group name
    expect(wrapper.text()).toContain('Group A')

    wrapper.unmount()
  })
})
