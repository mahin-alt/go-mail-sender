import { describe, it, expect, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import App from '@/App.vue'
import { appPlugins } from './helpers'

vi.mock('@/api/health', () => ({
  getHealth: vi.fn<() => Promise<{ status: string }>>().mockResolvedValue({ status: 'ok' }),
}))

describe('App shell', () => {
  it('renders the routed page inside the shell with a skip link', async () => {
    const { plugins } = await appPlugins('/')
    const wrapper = mount(App, { global: { plugins } })
    await flushPromises()

    expect(wrapper.get('main#main').text()).toContain('Overview page')
    expect(wrapper.get('a[href="#main"]').text()).toBe('Skip to content')
  })

  it('lists only pages that declare nav meta, and marks the current page', async () => {
    const { plugins } = await appPlugins('/settings')
    const wrapper = mount(App, { global: { plugins } })
    await flushPromises()

    const nav = wrapper.get('nav[aria-label="Main"]')
    const labels = nav.findAll('a').map((a) => a.text())
    expect(labels).toEqual(['Overview', 'Settings'])
    expect(nav.get('a[aria-current="page"]').text()).toBe('Settings')
    expect(nav.text()).not.toContain('Hidden')
  })
})
