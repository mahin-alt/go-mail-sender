import { describe, it, expect, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ApiStatus from '@/components/layout/ApiStatus.vue'
import { getHealth } from '@/api/health'
import { ApiError } from '@/api/errors'
import { appPlugins } from './helpers'

vi.mock('@/api/health', () => ({ getHealth: vi.fn<typeof getHealth>() }))

describe('ApiStatus', () => {
  it('says connected when /healthz answers', async () => {
    vi.mocked(getHealth).mockResolvedValue({ status: 'ok' })
    const { plugins } = await appPlugins()
    const wrapper = mount(ApiStatus, { global: { plugins } })
    await flushPromises()

    expect(wrapper.get('[role="status"]').text()).toBe('API connected')
  })

  it('says unreachable when the request fails', async () => {
    vi.mocked(getHealth).mockRejectedValue(new ApiError('Cannot reach the server.', 0, 'network'))
    const { plugins } = await appPlugins()
    const wrapper = mount(ApiStatus, { global: { plugins } })
    // The hook retries once (about 1s delay) before reporting an error, so allow for that.
    await vi.waitFor(() => expect(wrapper.get('[role="status"]').text()).toBe('API unreachable'), {
      timeout: 5000,
    })
  })
})
