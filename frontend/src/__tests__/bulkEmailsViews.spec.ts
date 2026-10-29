import { flushPromises, mount } from '@vue/test-utils'
import { VueQueryPlugin } from '@tanstack/vue-query'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import type { BatchListParams, JobListParams } from '@/api/batches'
import { ApiError } from '@/api/errors'
import { queryClient } from '@/queries/client'
import { routes } from '@/router/routes'
import type { BatchProgress, BatchSummary, Job, Paged, RetryResult } from '@/types/api'
import BulkEmailDetailView from '@/views/BulkEmailDetailView.vue'
import BulkEmailsView from '@/views/BulkEmailsView.vue'

const api = vi.hoisted(() => ({
  listBatches: vi.fn<(p: BatchListParams) => Promise<Paged<BatchSummary>>>(),
  getBatch: vi.fn<(id: number) => Promise<BatchProgress>>(),
  listJobs: vi.fn<(id: number, p: JobListParams) => Promise<Paged<Job>>>(),
  retryBatch: vi.fn<(id: number) => Promise<RetryResult>>(),
  retryJob: vi.fn<(id: number) => Promise<RetryResult>>(),
}))
vi.mock('@/api/batches', () => api)
vi.mock('@/api/health', () => ({ getHealth: () => Promise.resolve({ status: 'ok' }) }))

const batch = (over: Partial<BatchSummary> = {}): BatchSummary => ({
  id: 42,
  filename: 'refunds_oct27.xlsx',
  status: 'completed',
  total: 312,
  sent: 300,
  failed: 12,
  pending: 0,
  processing: 0,
  invalid: 0,
  created_at: new Date().toISOString(),
  ...over,
})

const job = (over: Partial<Job> = {}): Job => ({
  id: 1043,
  batch_id: 42,
  recipient_name: 'Rahim',
  recipient_email: 'rahim@shop.example',
  subject: 'Refund',
  body: 'Hello <b>Rahim</b>',
  status: 'failed',
  attempts: 3,
  max_attempts: 3,
  last_error: 'Mailbox full',
  run_at: '',
  created_at: '',
  updated_at: new Date().toISOString(),
  ...over,
})

async function mountAt(
  path: string,
  component: typeof BulkEmailsView | typeof BulkEmailDetailView,
) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  router.push(path)
  await router.isReady()
  const wrapper = mount(component, {
    attachTo: document.body,
    global: { plugins: [router, [VueQueryPlugin, { queryClient }], PrimeVue, ToastService] },
  })
  await flushPromises()
  return { wrapper, router }
}

beforeEach(() => {
  queryClient.clear()
  Object.values(api).forEach((fn) => fn.mockReset())
  api.listBatches.mockImplementation(async (p) => ({
    items:
      p.limit === 1
        ? []
        : [batch(), batch({ id: 43, filename: 'payouts.xlsx', status: 'processing', failed: 0 })],
    total: p.status === 'completed' ? 22 : 24,
    limit: p.limit,
    offset: 0,
  }))
})

describe('Bulk Emails list', () => {
  it('shows batches with the filename as the title and a status badge', async () => {
    const { wrapper } = await mountAt('/bulk-emails', BulkEmailsView)
    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.text()).toContain('refunds_oct27.xlsx')
    expect(rows[0]!.text()).toContain('Completed with failures')
    expect(rows[1]!.text()).toContain('Sending')
  })

  it('shows a count on each status tab', async () => {
    const { wrapper } = await mountAt('/bulk-emails', BulkEmailsView)
    const tabs = wrapper.findAll('[aria-label="Filter by status"] button').map((b) => b.text())
    expect(tabs).toEqual(['All 24', 'Pending 24', 'Processing 24', 'Completed 22'])
  })

  it('puts the chosen tab and search in the address, going back to page 1', async () => {
    const { wrapper, router } = await mountAt('/bulk-emails?page=3', BulkEmailsView)
    await wrapper.findAll('[aria-label="Filter by status"] button')[3]!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ status: 'completed' })
  })

  it('asks the API for the page in the address', async () => {
    await mountAt('/bulk-emails?status=completed&q=oct&page=2', BulkEmailsView)
    expect(api.listBatches).toHaveBeenCalledWith({
      status: 'completed',
      q: 'oct',
      limit: 20,
      offset: 20,
    })
  })

  it('explains an empty result differently when filters are on', async () => {
    api.listBatches.mockResolvedValue({ items: [], total: 0, limit: 20, offset: 0 })
    expect((await mountAt('/bulk-emails', BulkEmailsView)).wrapper.text()).toContain(
      'No Bulk Emails yet',
    )
    queryClient.clear()
    expect((await mountAt('/bulk-emails?q=zzz', BulkEmailsView)).wrapper.text()).toContain(
      'No Bulk Emails match',
    )
  })

  it('shows the server message and a way to try again when loading fails', async () => {
    api.listBatches.mockRejectedValue(new ApiError('database is down', 500, 'http'))
    const { wrapper } = await mountAt('/bulk-emails', BulkEmailsView)
    await vi.waitFor(() => expect(wrapper.find('[role="alert"]').exists()).toBe(true), {
      timeout: 4000,
    })
    expect(wrapper.find('[role="alert"]').text()).toContain('database is down')
  })
})

describe('Bulk Email detail', () => {
  const progress = (over: Partial<BatchProgress> = {}): BatchProgress => ({
    batch_id: 42,
    filename: 'refunds_oct27.xlsx',
    status: 'completed',
    total: 312,
    sent: 300,
    failed: 12,
    pending: 0,
    processing: 0,
    invalid: 0,
    ...over,
  })

  beforeEach(() => {
    api.getBatch.mockResolvedValue(progress())
    api.listJobs.mockResolvedValue({
      items: [
        job(),
        job({ id: 1046, status: 'sent', last_error: 'old error', recipient_name: 'Sadia' }),
      ],
      total: 312,
      limit: 20,
      offset: 0,
    })
  })

  it('shows the counters and a Retry button named with the failed count', async () => {
    const { wrapper } = await mountAt('/bulk-emails/42', BulkEmailDetailView)
    expect(wrapper.find('h1').text()).toBe('refunds_oct27.xlsx')
    expect(wrapper.find('button[aria-label="Retry 12 failed emails"]').text()).toBe('Retry')
  })

  it('has no Retry button when nothing failed', async () => {
    api.getBatch.mockResolvedValue(progress({ failed: 0 }))
    const { wrapper } = await mountAt('/bulk-emails/42', BulkEmailDetailView)
    expect(wrapper.find('button[aria-label*="failed email"]').exists()).toBe(false)
  })

  it('does not show the old error on a sent email', async () => {
    const { wrapper } = await mountAt('/bulk-emails/42', BulkEmailDetailView)
    expect(wrapper.text()).toContain('Mailbox full')
    expect(wrapper.text()).not.toContain('old error')
  })

  it('asks for confirmation before retrying, then retries the whole batch', async () => {
    api.retryBatch.mockResolvedValue({ retried: 12 })
    const { wrapper } = await mountAt('/bulk-emails/42', BulkEmailDetailView)
    await wrapper.find('button[aria-label="Retry 12 failed emails"]').trigger('click')
    await flushPromises()
    expect(api.retryBatch).not.toHaveBeenCalled()
    expect(document.body.textContent).toContain('Retry 12 failed emails?')
    const confirm = [...document.body.querySelectorAll('button')].find(
      (b) => b.textContent?.trim() === 'Retry 12 emails',
    )!
    confirm.click()
    await flushPromises()
    expect(api.retryBatch).toHaveBeenCalledWith(42)
  })

  it('retries one email from its row without opening the panel', async () => {
    api.retryJob.mockResolvedValue({ retried: 1 })
    const { wrapper } = await mountAt('/bulk-emails/42', BulkEmailDetailView)
    await wrapper.find('button[aria-label="Retry email to rahim@shop.example"]').trigger('click')
    await flushPromises()
    expect(api.retryJob).toHaveBeenCalledWith(1043)
    expect(document.body.querySelector('.p-drawer')).toBeNull()
  })

  it('opens the side panel with the message as plain text', async () => {
    const { wrapper } = await mountAt('/bulk-emails/42', BulkEmailDetailView)
    await wrapper.find('tbody tr button.text-left').trigger('click')
    await flushPromises()
    const panel = document.body.querySelector('.p-drawer')!
    expect(panel.textContent).toContain('Hello <b>Rahim</b>')
    expect(panel.querySelector('pre b')).toBeNull()
  })

  it('says so when the Bulk Email does not exist', async () => {
    api.getBatch.mockRejectedValue(new ApiError('batch not found', 404, 'http'))
    const { wrapper } = await mountAt('/bulk-emails/9999', BulkEmailDetailView)
    await vi.waitFor(() => expect(wrapper.find('[role="alert"]').exists()).toBe(true))
    expect(wrapper.find('[role="alert"]').text()).toContain('does not exist')
  })
})
