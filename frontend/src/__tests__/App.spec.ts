import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'

import App from '../App.vue'

describe('App', () => {
  it('mounts with a router', async () => {
    const router = createRouter({ history: createMemoryHistory(), routes: [] })
    const wrapper = mount(App, { global: { plugins: [router] } })
    expect(wrapper.exists()).toBe(true)
  })
})
