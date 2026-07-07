import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelPatternTagInput from '../common/ModelPatternTagInput.vue'

describe('ModelPatternTagInput', () => {
  it('renders chips from comma-separated modelValue', () => {
    const wrapper = mount(ModelPatternTagInput, {
      props: { modelValue: 'gpt-4o,claude-3-opus' },
    })
    const chips = wrapper.findAll('.chip')
    expect(chips).toHaveLength(2)
    expect(chips[0].text()).toContain('gpt-4o')
    expect(chips[1].text()).toContain('claude-3-opus')
  })

  it('emits update when a chip is removed', async () => {
    const wrapper = mount(ModelPatternTagInput, {
      props: { modelValue: 'gpt-4o,claude-3-opus' },
    })
    await wrapper.findAll('.chip-remove')[0].trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['claude-3-opus'])
  })

  it('adds chip on Enter', async () => {
    const wrapper = mount(ModelPatternTagInput, { props: { modelValue: '' } })
    const input = wrapper.find('.tag-input')
    await input.setValue('gemini-pro')
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['gemini-pro'])
  })

  it('commits chips on comma key', async () => {
    const wrapper = mount(ModelPatternTagInput, { props: { modelValue: '' } })
    const input = wrapper.find('.tag-input')
    await input.setValue('gpt-4o,claude-3-opus')
    await input.trigger('keydown', { key: ',' })
    // Component v-model emits the comma-joined string per its v-model:string contract.
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['gpt-4o,claude-3-opus'])
  })

  it('paste with commas splits into multiple chips', async () => {
    const wrapper = mount(ModelPatternTagInput, { props: { modelValue: '' } })
    const input = wrapper.find('.tag-input')
    // Attach clipboardData to the event (handler reads e.clipboardData, not the element).
    await input.trigger('paste', {
      clipboardData: { getData: () => 'gpt-4o,claude-3-opus' },
    })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['gpt-4o,claude-3-opus'])
  })
})
