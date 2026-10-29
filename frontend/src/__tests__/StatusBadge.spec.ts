import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'

import StatusBadge from '@/components/status/StatusBadge.vue'

describe('StatusBadge', () => {
  it('shows the label with a decorative icon', () => {
    const wrapper = mount(StatusBadge, { props: { status: 'sent' } })
    expect(wrapper.text()).toBe('Sent')
    expect(wrapper.find('svg').attributes('aria-hidden')).toBe('true')
  })

  it('uses the dark ink colour for text, not the fill colour', () => {
    expect(mount(StatusBadge, { props: { status: 'failed' } }).classes()).toContain('text-bad-ink')
  })

  it('passes batch context through', () => {
    const wrapper = mount(StatusBadge, { props: { status: 'completed', failed: 3 } })
    expect(wrapper.text()).toBe('Completed with failures')
  })
})
