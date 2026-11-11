import { flushPromises, mount } from '@vue/test-utils'
import { VueQueryPlugin } from '@tanstack/vue-query'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import type { BatchListParams, EmailParams } from '@/api/batches'
import { ApiError } from '@/api/errors'
import { queryClient } from '@/queries/client'
import { routes } from '@/router/routes'
import type {
  Batch,
  CancelResult,
  CursorPage,
  Email,
  Paged,
  RetryResult,
} from '@/types/api'
import BulkEmailDetailView from '@/views/BulkEmailDetailView.vue'
import BulkEmailsView from '@/views/BulkEmailsView.vue'
import { fakeBatch } from './uploadFakes'

const api = vi.hoisted(() => ({
  listBatches: vi.fn<(p: BatchListParams) => Promise<Paged<Batch>>>(),
  getBatch: vi.fn<(id: number) => Promise<Batch>>(),
  listEmails: vi.fn<(id: number, p: EmailParams) => Promise<CursorPage<Email>>>(),
  retryBatch: vi.fn<(id: number) => Promise<RetryResult>>(),
  retryJob: vi.fn<(id: number) => Promise<RetryResult>>(),
  cancelBatch: vi.fn<(id: number) => Promise<CancelResult>>(),
}))
vi.mock('@/api/batches', () => api)
vi.mock('@/api/health', () => ({ getHealth: () => Promise.resolve({ status: 'ok' }) }))

/** A sent bulk batch: 312 rows, 300 sent, 12 failed. */
const batch = (over: Partial<Batch> = {}): Batch => ({
  ...fakeBatch([]),
  id: 42,
  filename: 'refunds_oct27.xlsx',
  status: 'completed_with_errors',
  total_rows: 312,
  valid_count: 312,
  sent_count: 300,
  failed_count: 12,
  created_at: new Date().toISOString(),
  send_requested_at: new Date().toISOString(),
  ...over,
})

const job = (over: Partial<Email> = {}): Email => ({
  id: 1043,
  batch_id: 42,
  recipient_id: 1,
  row: 2,
  name: 'Rahim',
  email: 'rahim@shop.example',
  subject: 'Refund',
  body: 'Hello <b>Rahim</b>',
  status: 'failed',
  attempts: 3,
  max_attempts: 3,
  last_error: 'Mailbox full',
  available_at: '',
  sent_at: null,
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
        : [batch(), batch({ id: 43, filename: 'payouts.xlsx', status: 'sending', failed_count: 0 })],
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
    expect(tabs).toEqual([
      'All 24',
      'Not sent 24',
      'Sending 24',
      'Completed 22',
      'With failures 24',
      'Cancelled 24',
    ])
    // The list shows bulk batches only (verified files appear once they are sent).
    expect(api.listBatches).toHaveBeenCalledWith(expect.objectContaining({ type: 'bulk_send', limit: 1 }))
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
      type: 'bulk_send',
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
  const progress = batch

  beforeEach(() => {
    api.getBatch.mockResolvedValue(progress())
    api.listEmails.mockResolvedValue({
      items: [job(), job({ id: 1046, status: 'sent', last_error: 'old error', name: 'Sadia' })],
      next_after: null,
    })
  })

  it('shows the counters and a Retry button named with the failed count', async () => {
    const { wrapper } = await mountAt('/bulk-emails/42', BulkEmailDetailView)
    expect(wrapper.find('h1').text()).toBe('refunds_oct27.xlsx')
    expect(wrapper.find('button[aria-label="Retry 12 failed emails"]').text()).toBe('Retry')
  })

  it('has no Retry button when nothing failed', async () => {
    api.getBatch.mockResolvedValue(progress({ status: 'completed', failed_count: 0 }))
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

  it('offers Cancel only while the batch is still sending', async () => {
    const { wrapper } = await mountAt('/bulk-emails/42', BulkEmailDetailView)
    expect(wrapper.find('button[aria-label^="Cancel this Bulk Email"]').exists()).toBe(false)

    api.getBatch.mockResolvedValue(progress({ status: 'sending', pending_count: 250, failed_count: 0 }))
    queryClient.clear()
    const again = await mountAt('/bulk-emails/42', BulkEmailDetailView)
    expect(
      again.wrapper.find('button[aria-label="Cancel this Bulk Email (250 unsent)"]').exists(),
    ).toBe(true)
  })

  it('asks before cancelling, then cancels and reports how many were stopped', async () => {
    api.getBatch.mockResolvedValue(progress({ status: 'sending', pending_count: 250, failed_count: 0 }))
    api.cancelBatch.mockResolvedValue({ cancelled: 250 })
    const { wrapper } = await mountAt('/bulk-emails/42', BulkEmailDetailView)
    await wrapper.find('button[aria-label="Cancel this Bulk Email (250 unsent)"]').trigger('click')
    await flushPromises()
    expect(api.cancelBatch).not.toHaveBeenCalled()
    expect(document.body.textContent).toContain('Stop this Bulk Email?')
    expect(document.body.textContent).toContain('250 unsent emails will not be sent.')
    const buttons = [...document.body.querySelectorAll('button')]
    expect(buttons.some((b) => b.textContent?.trim() === 'Keep sending')).toBe(true)
    buttons.find((b) => b.textContent?.trim() === 'Stop sending')!.click()
    await flushPromises()
    expect(api.cancelBatch).toHaveBeenCalledWith(42)
  })

  it('shows a cancelled batch with its Cancelled count and tab', async () => {
    api.getBatch.mockResolvedValue(
      progress({
        status: 'cancelled',
        failed_count: 0,
        cancelled_count: 630036,
        cancelled_at: new Date().toISOString(),
      }),
    )
    const { wrapper } = await mountAt('/bulk-emails/42', BulkEmailDetailView)
    expect(wrapper.text()).toContain('Cancelled')
    expect(wrapper.text()).toContain('630,036')
    expect(wrapper.text()).toContain('stopped before sending')
    expect(wrapper.find('button[aria-label^="Cancel this Bulk Email"]').exists()).toBe(false)
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

  it('pages emails with Load more, asking for the next page after the last ID', async () => {
    api.listEmails.mockImplementation(async (_id, p) =>
      p.after
        ? { items: [job({ id: 2000, name: 'Later' })], next_after: null }
        : { items: [job()], next_after: 1043 },
    )
    const { wrapper } = await mountAt('/bulk-emails/42?status=failed', BulkEmailDetailView)
    expect(api.listEmails).toHaveBeenCalledWith(42, { status: 'failed', q: '', after: 0, limit: 20 })
    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Load more')!
      .trigger('click')
    await flushPromises()
    expect(api.listEmails).toHaveBeenLastCalledWith(42, {
      status: 'failed',
      q: '',
      after: 1043,
      limit: 20,
    })
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
    expect(wrapper.findAll('button').some((b) => b.text() === 'Load more')).toBe(false)
  })

  it('shows a draft as not sent yet, with a way to write and send it', async () => {
    api.getBatch.mockResolvedValue(
      progress({ status: 'draft', sent_count: 0, failed_count: 0, invalid_count: 4, send_requested_at: null }),
    )
    const { wrapper } = await mountAt('/bulk-emails/42', BulkEmailDetailView)
    expect(wrapper.text()).toContain('Not sent yet')
    expect(wrapper.text()).toContain('312 valid · 4 invalid')
    const link = wrapper.findAll('a').find((a) => a.text().includes('Write and send'))!
    expect(link.attributes('href')).toBe('/bulk-emails/new?batch=42')
    expect(api.listEmails).not.toHaveBeenCalled()
  })

  it('shows the import progress of a file that is still being checked', async () => {
    api.getBatch.mockResolvedValue(
      progress({ status: 'importing', processed_rows: 52000, sent_count: 0, failed_count: 0 }),
    )
    const { wrapper } = await mountAt('/bulk-emails/42', BulkEmailDetailView)
    expect(wrapper.text()).toContain('Checking the file')
    expect(wrapper.text()).toContain('52,000 rows read')
  })
})
