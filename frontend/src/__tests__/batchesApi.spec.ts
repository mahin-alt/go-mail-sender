import { beforeEach, describe, expect, it, vi } from 'vitest'

const get = vi.fn<(url: string, config?: unknown) => Promise<unknown>>()
const post = vi.fn<(url: string) => Promise<unknown>>()
vi.mock('@/api/client', () => ({
  http: {
    get: (url: string, config?: unknown) => get(url, config),
    post: (url: string) => post(url),
  },
}))

import { listBatches, listJobs, retryBatch, retryJob } from '@/api/batches'

describe('batches API', () => {
  beforeEach(() => {
    get.mockReset().mockResolvedValue({ data: { items: [], total: 0, limit: 20, offset: 0 } })
    post.mockReset().mockResolvedValue({ data: { retried: 3 } })
  })

  it('leaves empty filters out of the query string', async () => {
    await listBatches({ status: '', q: '', limit: 20, offset: 0 })
    expect(get).toHaveBeenCalledWith('/api/v1/batches', { params: { limit: 20, offset: 0 } })
  })

  it('sends the filters that are set', async () => {
    await listBatches({ status: 'completed', q: 'oct', limit: 20, offset: 40 })
    expect(get).toHaveBeenCalledWith('/api/v1/batches', {
      params: { status: 'completed', q: 'oct', limit: 20, offset: 40 },
    })
  })

  it('always asks for the paged jobs form (limit is sent)', async () => {
    await listJobs(7, { status: 'failed', q: '', limit: 20, offset: 0 })
    expect(get).toHaveBeenCalledWith('/api/v1/batches/7/jobs', {
      params: { status: 'failed', limit: 20, offset: 0 },
    })
  })

  it('posts retries to the right endpoints', async () => {
    expect(await retryBatch(7)).toEqual({ retried: 3 })
    expect(post).toHaveBeenCalledWith('/api/v1/batches/7/retry')
    await retryJob(9)
    expect(post).toHaveBeenCalledWith('/api/v1/jobs/9/retry')
  })
})
