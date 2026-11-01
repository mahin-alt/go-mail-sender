import { flushPromises, mount } from '@vue/test-utils'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import { ApiError } from '@/api/errors'
import type { NewBulkEmail } from '@/api/recipients'
import { queryClient } from '@/queries/client'
import { routes } from '@/router/routes'
import { usePendingCheckStore } from '@/stores/pendingCheck'
import type { CreateBatchResult, ParseResult } from '@/types/api'
import NewBulkEmailView from '@/views/NewBulkEmailView.vue'
import VerifyEmailsView from '@/views/VerifyEmailsView.vue'

const api = vi.hoisted(() => ({
  parseRecipients: vi.fn<(file: File) => Promise<ParseResult>>(),
  createBatch: vi.fn<(input: NewBulkEmail) => Promise<CreateBatchResult>>(),
}))
vi.mock('@/api/recipients', () => api)

const parsed = (over: Partial<ParseResult> = {}): ParseResult => ({
  valid: [
    { name: 'Aisha', email: 'aisha@shop.example', row: 2 },
    { name: 'Rahim', email: 'rahim@shop.example', row: 3 },
  ],
  problems: [{ row: 4, name: 'Ben', email: 'ben@@x', reason: 'invalid email "ben@@x"' }],
  unverified_count: 0,
  ...over,
})
const xlsx = (name = 'orders.xlsx') => new File([new Uint8Array(2048)], name)

async function mountAt(path: string, component: typeof VerifyEmailsView | typeof NewBulkEmailView) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({ history: createMemoryHistory(), routes })
  router.push(path)
  await router.isReady()
  const wrapper = mount(component, {
    attachTo: document.body,
    global: { plugins: [router, pinia, [VueQueryPlugin, { queryClient }], PrimeVue, ToastService] },
  })
  await flushPromises()
  return { wrapper, router, pinia }
}
type W = Awaited<ReturnType<typeof mountAt>>['wrapper']

async function choose(wrapper: W, file: File) {
  const input = wrapper.find('input[type="file"]')
  Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
  await input.trigger('change')
  await flushPromises()
}
const button = (w: W, text: string) => w.findAll('button').find((b) => b.text().startsWith(text))!

beforeEach(() => {
  queryClient.clear()
  Object.values(api).forEach((fn) => fn.mockReset())
  api.parseRecipients.mockResolvedValue(parsed())
  document.body.innerHTML = ''
})

describe('Verify Emails page', () => {
  it('starts with only the upload card, and says nothing is sent', async () => {
    const { wrapper } = await mountAt('/verify-emails', VerifyEmailsView)
    expect(wrapper.find('h1').text()).toBe('Verify Emails')
    expect(wrapper.text()).toContain('Nothing is sent or saved')
    expect(wrapper.text()).not.toContain('Result')
  })

  it('checks the file and shows the result in "verify" wording, with no Send button', async () => {
    const { wrapper } = await mountAt('/verify-emails', VerifyEmailsView)
    const file = xlsx()
    await choose(wrapper, file)
    expect(api.parseRecipients).toHaveBeenCalledWith(file)
    expect(wrapper.text()).toContain('3 rows checked. 2 are ready to send, 1 will be skipped.')
    expect(wrapper.text()).toContain('1 row has a problem')
    expect(wrapper.text()).toContain('It would be skipped if you used this file')
    expect(wrapper.text()).not.toContain('will be sent')
    expect(wrapper.findAll('button').some((b) => b.text().startsWith('Send to'))).toBe(false)
    expect(api.createBatch).not.toHaveBeenCalled()
  })

  it('says every row is valid when nothing is wrong', async () => {
    api.parseRecipients.mockResolvedValue(parsed({ problems: [] }))
    const { wrapper } = await mountAt('/verify-emails', VerifyEmailsView)
    await choose(wrapper, xlsx())
    expect(wrapper.text()).toContain('Every row is valid')
  })

  it('cannot continue when no row is valid', async () => {
    api.parseRecipients.mockResolvedValue(parsed({ valid: [] }))
    const { wrapper } = await mountAt('/verify-emails', VerifyEmailsView)
    await choose(wrapper, xlsx())
    expect(wrapper.text()).toContain('check it again')
    expect(button(wrapper, 'Continue to New Bulk Email').attributes('disabled')).toBeDefined()
  })

  it('refuses a file that is not .xlsx and shows the server message for a bad file', async () => {
    const { wrapper } = await mountAt('/verify-emails', VerifyEmailsView)
    await choose(wrapper, xlsx('notes.csv'))
    expect(api.parseRecipients).not.toHaveBeenCalled()
    expect(wrapper.find('[role="alert"]').text()).toBe('Only .xlsx files are supported.')
    api.parseRecipients.mockRejectedValue(
      new ApiError('missing required columns: email', 400, 'http'),
    )
    await choose(wrapper, xlsx())
    expect(wrapper.find('[role="alert"]').text()).toBe('missing required columns: email')
  })

  it('"Check another file" goes back to an empty upload', async () => {
    const { wrapper } = await mountAt('/verify-emails', VerifyEmailsView)
    await choose(wrapper, xlsx())
    await button(wrapper, 'Check another file').trigger('click')
    expect(wrapper.text()).toContain('Drop your .xlsx file here')
    expect(wrapper.text()).not.toContain('Result')
  })

  it('hands the checked file to New Bulk Email, which starts from it without checking again', async () => {
    const { wrapper, router, pinia } = await mountAt('/verify-emails', VerifyEmailsView)
    const file = xlsx('customers.xlsx')
    await choose(wrapper, file)
    expect(api.parseRecipients).toHaveBeenCalledTimes(1)
    await button(wrapper, 'Continue to New Bulk Email').trigger('click')
    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('new-bulk-email'))

    // The next page takes over the same File object and the same result.
    setActivePinia(pinia)
    const next = mount(NewBulkEmailView, {
      attachTo: document.body,
      global: {
        plugins: [router, pinia, [VueQueryPlugin, { queryClient }], PrimeVue, ToastService],
      },
    })
    await flushPromises()
    expect(next.text()).toContain('customers.xlsx')
    expect(next.text()).toContain('3 rows checked. 2 are ready to send, 1 will be skipped.')
    expect(next.text()).toContain('2 will be sent · 1 skipped')
    expect(api.parseRecipients).toHaveBeenCalledTimes(1) // not uploaded and checked a second time
    expect(usePendingCheckStore().take()).toBeNull() // and it is only handed over once
  })

  it('New Bulk Email opened directly still starts empty', async () => {
    const { wrapper } = await mountAt('/bulk-emails/new', NewBulkEmailView)
    expect(wrapper.text()).toContain('Drop your .xlsx file here')
  })
})
