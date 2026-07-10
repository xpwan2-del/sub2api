import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { h } from 'vue'

import DataTable from '../DataTable.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

const stubDesktopMatchMedia = () => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
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
}

describe('DataTable', () => {
  beforeEach(() => {
    stubDesktopMatchMedia()
    localStorage.clear()
  })

  it('renders paired sort arrows and highlights the active direction', async () => {
    const wrapper = mount(DataTable, {
      props: {
        columns: [
          { key: 'name', label: 'Name', sortable: true },
          { key: 'created_at', label: 'Created', sortable: true }
        ],
        data: [
          { id: 1, name: 'Beta', created_at: '2026-01-02T00:00:00Z' },
          { id: 2, name: 'Alpha', created_at: '2026-01-01T00:00:00Z' }
        ],
        defaultSortKey: 'name',
        defaultSortOrder: 'asc'
      }
    })

    await wrapper.vm.$nextTick()

    const nameHeader = wrapper.findAll('th')[0]
    expect(nameHeader.attributes('aria-sort')).toBe('ascending')
    expect(nameHeader.findAll('svg')).toHaveLength(2)
    expect(nameHeader.findAll('svg')[0].classes()).toContain('text-primary-600')
    expect(nameHeader.findAll('svg')[1].classes()).toContain('text-gray-300')

    await nameHeader.trigger('click')
    await wrapper.vm.$nextTick()

    expect(nameHeader.attributes('aria-sort')).toBe('descending')
    expect(nameHeader.findAll('svg')[0].classes()).toContain('text-gray-300')
    expect(nameHeader.findAll('svg')[1].classes()).toContain('text-primary-600')
  })
})

describe('DataTable inline row expansion (virtual=false)', () => {
  beforeEach(() => {
    stubDesktopMatchMedia()
    localStorage.clear()
  })

  it('renders row-expansion slot under the matching row', async () => {
    const wrapper = mount(DataTable, {
      props: {
        virtual: false,
        rowKey: 'id',
        expandedRowKey: 2,
        columns: [{ key: 'name', label: 'Name' }],
        data: [
          { id: 1, name: 'Alpha' },
          { id: 2, name: 'Beta' }
        ]
      },
      slots: {
        'row-expansion': ({ row }: { row: { id: number } }) =>
          h('div', { class: 'expansion' }, `usage-${row.id}`)
      }
    })

    await wrapper.vm.$nextTick()

    const expansions = wrapper.findAll('.expansion')
    expect(expansions).toHaveLength(1)
    expect(expansions[0].text()).toBe('usage-2')
    expect(wrapper.find('tr.expanded-row td').attributes('colspan')).toBe('1')
  })

  it('does not render row-expansion when expandedRowKey is null', async () => {
    const wrapper = mount(DataTable, {
      props: {
        virtual: false,
        rowKey: 'id',
        expandedRowKey: null,
        columns: [{ key: 'name', label: 'Name' }],
        data: [{ id: 1, name: 'Alpha' }]
      },
      slots: {
        'row-expansion': () => h('div', { class: 'expansion' }, 'x')
      }
    })

    await wrapper.vm.$nextTick()
    expect(wrapper.findAll('.expansion')).toHaveLength(0)
  })

  it('does not render row-expansion in virtual mode (default)', async () => {
    const wrapper = mount(DataTable, {
      props: {
        rowKey: 'id',
        expandedRowKey: 1,
        columns: [{ key: 'name', label: 'Name' }],
        data: [{ id: 1, name: 'Alpha' }]
      },
      slots: {
        'row-expansion': () => h('div', { class: 'expansion' }, 'x')
      }
    })

    await wrapper.vm.$nextTick()
    expect(wrapper.findAll('.expansion')).toHaveLength(0)
  })
})
