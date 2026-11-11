import { beforeEach, describe, expect, it, vi } from 'vitest'

const get = vi.fn<(url: string, config?: unknown) => Promise<unknown>>()
const post = vi.fn<(url: string, body?: unknown) => Promise<unknown>>()
vi.mock('@/api/client', () => ({
  http: {
    get: (url: string, config?: unknown) => get(url, config),
    post: (url: string, body?: unknown) => (body === undefined ? post(url) : post(url, body)),
  },
}))

import { listBatches, listEmails, listIssues, retryBatch, retryJob, sendBatch } from '@/api/batches'

describe('batches API', () => {
  beforeEach(() => {
    get.mockReset().mockResolvedValue({ data: { items: [], total: 0, limit: 20, offset: 0 } })
    post.mockReset().mockResolvedValue({ data: { retried: 3 } })
  })

  it('leaves empty filters out of the query string', async () => {
    await listBatches({ type: '', status: '', q: '', limit: 20, offset: 0 })
    expect(get).toHaveBeenCalledWith('/api/v1/batches', { params: { limit: 20, offset: 0 } })
  })

  it('sends the filters that are set', async () => {
    await listBatches({ type: 'bulk_send', status: 'completed', q: 'oct', limit: 20, offset: 40 })
    expect(get).toHaveBeenCalledWith('/api/v1/batches', {
      params: { type: 'bulk_send', status: 'completed', q: 'oct', limit: 20, offset: 40 },
    })
  })

  it('pages emails with a cursor and leaves the first page cursor out', async () => {
    await listEmails(7, { status: 'failed', q: '', limit: 20, after: 0 })
    expect(get).toHaveBeenCalledWith('/api/v1/batches/7/emails', {
      params: { status: 'failed', limit: 20 },
    })
    await listEmails(7, { status: '', q: 'ais', limit: 20, after: 51 })
    expect(get).toHaveBeenLastCalledWith('/api/v1/batches/7/emails', {
      params: { q: 'ais', limit: 20, after: 51 },
    })
  })

  it('asks for problem rows by status and reasons', async () => {
    await listIssues(7, { status: 'invalid', reasons: ['empty_email', 'empty_name'], limit: 100 })
    expect(get).toHaveBeenCalledWith('/api/v1/batches/7/issues', {
      params: { status: undefined, reason: 'empty_email,empty_name', q: undefined, after: undefined, limit: 100 },
    })
    await listIssues(7, { status: 'unverified', after: 9 })
    expect(get).toHaveBeenLastCalledWith('/api/v1/batches/7/issues', {
      params: { status: 'unverified', reason: undefined, q: undefined, after: 9, limit: undefined },
    })
  })

  it('sends the message with the explicit Send', async () => {
    post.mockResolvedValueOnce({ data: { id: 7, status: 'generating_jobs' } })
    await sendBatch(7, { subject: 'Hi', body: 'Hello' })
    expect(post).toHaveBeenCalledWith('/api/v1/batches/7/send', { subject: 'Hi', body: 'Hello' })
  })

  it('posts retries to the right endpoints', async () => {
    expect(await retryBatch(7)).toEqual({ retried: 3 })
    expect(post).toHaveBeenCalledWith('/api/v1/batches/7/retry')
    await retryJob(9)
    expect(post).toHaveBeenCalledWith('/api/v1/jobs/9/retry')
  })
})
