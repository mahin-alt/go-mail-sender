import { flushPromises, mount } from '@vue/test-utils'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import App from '../App.vue'
import { queryClient } from '@/queries/client'
import { routes } from '@/router/routes'

vi.mock('@/api/health', () => ({ getHealth: () => Promise.resolve({ status: 'ok' }) }))

async function mountAt(path: string) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  router.push(path)
  await router.isReady()
  const wrapper = mount(App, {
    global: { plugins: [router, [VueQueryPlugin, { queryClient }]] },
  })
  await flushPromises()
  return { wrapper, router }
}

describe('App shell', () => {
  beforeEach(() => {
    localStorage.clear()
    queryClient.clear()
  })

  it('redirects / to the Dashboard', async () => {
    const { router } = await mountAt('/')
    expect(router.currentRoute.value.name).toBe('dashboard')
  })

  it('lists the six menu items in order, ungrouped', async () => {
    const { wrapper } = await mountAt('/dashboard')
    const labels = wrapper
      .findAll('nav[aria-label="Main"] a')
      .map((a) => a.attributes('aria-label'))
    expect(labels).toEqual([
      'Dashboard',
      'Bulk Emails',
      'Single Email',
      'Verify Emails',
      'Failures',
      'Settings',
    ])
  })

  it('marks the current page', async () => {
    const { wrapper } = await mountAt('/bulk-emails')
    const current = wrapper.findAll('nav[aria-label="Main"] a[aria-current="page"]')
    expect(current).toHaveLength(1)
    expect(current[0]?.attributes('aria-label')).toBe('Bulk Emails')
  })

  it('shows the page title as the heading', async () => {
    const { wrapper } = await mountAt('/settings')
    expect(wrapper.find('h1').text()).toBe('Settings')
  })

  it('collapses and re-opens from the sidebar button', async () => {
    const { wrapper } = await mountAt('/dashboard')
    const button = wrapper.find('button[aria-expanded]')
    expect(button.attributes('aria-expanded')).toBe('true')
    await button.trigger('click')
    expect(button.attributes('aria-expanded')).toBe('false')
    expect(button.attributes('aria-label')).toBe('Keep sidebar open')
    await button.trigger('click')
    expect(button.attributes('aria-expanded')).toBe('true')
  })

  it('shows the API as online when /healthz answers', async () => {
    const { wrapper } = await mountAt('/dashboard')
    expect(wrapper.find('[role="status"]').text()).toBe('API online')
  })

  it('shows a not-found page for unknown addresses', async () => {
    const { wrapper } = await mountAt('/nope')
    expect(wrapper.find('h1').text()).toBe('Page not found')
  })
})
