import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'

import { describeStatus } from '@/components/status/statusMeta'
import StatusBadge from '@/components/status/StatusBadge.vue'

describe('describeStatus', () => {
  it('uses different colours for failed (red) and invalid (amber)', () => {
    expect(describeStatus('failed').severity).toBe('danger')
    expect(describeStatus('invalid').severity).toBe('warn')
  })

  it('shows a pending job that was already tried as retrying', () => {
    expect(describeStatus('pending').label).toBe('Pending')
    expect(describeStatus('pending', 2, 4).label).toBe('Retrying 2 of 4')
    expect(describeStatus('pending', 1).label).toBe('Retrying')
  })

  it('spins only while sending', () => {
    expect(describeStatus('processing').spin).toBe(true)
    expect(describeStatus('sent').spin).toBe(false)
  })
})

describe('StatusBadge', () => {
  it('always renders a text label and a hidden icon, never colour alone', () => {
    const wrapper = mount(StatusBadge, {
      props: { status: 'invalid' },
      global: { plugins: [PrimeVue] },
    })
    expect(wrapper.text()).toBe('Skipped (invalid)')
    expect(wrapper.get('svg').attributes('aria-hidden')).toBe('true')
  })
})
