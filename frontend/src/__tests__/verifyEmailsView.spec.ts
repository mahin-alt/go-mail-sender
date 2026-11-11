import { flushPromises, mount } from '@vue/test-utils'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import { ApiError } from '@/api/errors'
import { queryClient } from '@/queries/client'
import { routes } from '@/router/routes'
import type { SettingItem } from '@/types/api'
import NewBulkEmailView from '@/views/NewBulkEmailView.vue'
import VerifyEmailsView from '@/views/VerifyEmailsView.vue'
import { batchesApiMock, serverHas as fakeServer, typicalRows, type FakeRow } from './uploadFakes'

const api = vi.hoisted(() => ({}) as ReturnType<typeof batchesApiMock>)
vi.mock('@/api/batches', async () => {
  const { batchesApiMock } = await import('./uploadFakes')
  return Object.assign(api, batchesApiMock())
})

/** Make the server answer as if the chosen file held these rows (a verification batch). */
function serverHas(rows: FakeRow[]) {
  return fakeServer(api, rows, { type: 'verification' })
}

const settingsApi = vi.hoisted(() => ({
  getSettings: vi.fn<() => Promise<SettingItem[]>>(),
}))
vi.mock('@/api/settings', () => settingsApi)
const uploadLimit = (mb: number): SettingItem => ({
  key: 'MAX_UPLOAD_MB',
  description: '',
  kind: 'int',
  value: String(mb),
  has_value: true,
  source: 'override',
  editable: true,
  secret: false,
  apply: 'instant',
  restart_required: false,
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
  serverHas(typicalRows)
  settingsApi.getSettings.mockReset()
  settingsApi.getSettings.mockResolvedValue([uploadLimit(10)])
  document.body.innerHTML = ''
})

describe('Verify Emails page', () => {
  it('starts with only the upload card and no explanatory text', async () => {
    const { wrapper } = await mountAt('/verify-emails', VerifyEmailsView)
    expect(wrapper.find('h1').text()).toBe('Verify Emails')
    expect(wrapper.text()).not.toContain('Nothing is sent or saved')
    expect(wrapper.text()).not.toContain('We check every row')
    expect(wrapper.text()).toContain('First sheet, with a header row') // the drop-zone hint stays
    expect(wrapper.text()).not.toContain('Result')
  })

  it('checks the file and shows the result without any Send button or long notices', async () => {
    const { wrapper } = await mountAt('/verify-emails', VerifyEmailsView)
    const file = xlsx()
    await choose(wrapper, file)
    expect(api.uploadSpreadsheet).toHaveBeenCalledWith('verification', file, expect.any(Function))
    expect(wrapper.text()).toContain('Ready to send')
    expect(wrapper.text()).toContain('Will be skipped')
    expect(wrapper.text()).not.toContain('rows checked')
    expect(wrapper.text()).not.toContain('would be skipped if you used this file')
    expect(wrapper.text()).not.toContain('will be sent')
    expect(wrapper.findAll('button').some((b) => b.text().startsWith('Send to'))).toBe(false)
    expect(api.sendBatch).not.toHaveBeenCalled()
  })

  it('says every row is valid when nothing is wrong', async () => {
    serverHas(typicalRows.filter((r) => r.status === 'valid'))
    const { wrapper } = await mountAt('/verify-emails', VerifyEmailsView)
    await choose(wrapper, xlsx())
    expect(wrapper.text()).toContain('Every row is valid')
  })

  it('cannot continue when no row is valid', async () => {
    serverHas(typicalRows.filter((r) => r.status === 'invalid'))
    const { wrapper } = await mountAt('/verify-emails', VerifyEmailsView)
    await choose(wrapper, xlsx())
    expect(wrapper.text()).toContain('No valid emails in this file')
    expect(button(wrapper, 'Continue to New Bulk Email').attributes('disabled')).toBeDefined()
  })

  it('refuses a file that is not .xlsx and shows the server message for a bad file', async () => {
    const { wrapper } = await mountAt('/verify-emails', VerifyEmailsView)
    await choose(wrapper, xlsx('notes.csv'))
    expect(api.uploadSpreadsheet).not.toHaveBeenCalled()
    expect(wrapper.find('[role="alert"]').text()).toBe('Only .xlsx files are supported.')
    api.uploadSpreadsheet.mockRejectedValue(
      new ApiError('the file is not a valid .xlsx spreadsheet', 400, 'http'),
    )
    await choose(wrapper, xlsx())
    expect(wrapper.find('[role="alert"]').text()).toBe('the file is not a valid .xlsx spreadsheet')
  })

  it('shows the reason when the server could not read the file in the background', async () => {
    const batch = serverHas(typicalRows)
    api.getBatch.mockResolvedValue({
      ...batch,
      status: 'import_failed',
      import_error: 'sheet must have "name" and "email" columns in its header row',
    })
    const { wrapper } = await mountAt('/verify-emails', VerifyEmailsView)
    await choose(wrapper, xlsx())
    expect(wrapper.find('[role="alert"]').text()).toContain('"name" and "email" columns')
    expect(wrapper.text()).not.toContain('Result')
  })

  it('shows the import progress while the server reads the file', async () => {
    const batch = serverHas(typicalRows)
    api.getBatch.mockResolvedValue({ ...batch, status: 'importing', processed_rows: 120000 })
    const { wrapper } = await mountAt('/verify-emails', VerifyEmailsView)
    await choose(wrapper, xlsx())
    expect(wrapper.find('[role="status"]').text()).toContain('120,000 read so far')
    expect(wrapper.text()).not.toContain('Result')
  })

  it('"Check another file" goes back to an empty upload', async () => {
    const { wrapper } = await mountAt('/verify-emails', VerifyEmailsView)
    await choose(wrapper, xlsx())
    await button(wrapper, 'Check another file').trigger('click')
    expect(wrapper.text()).toContain('Drop your .xlsx file here')
    expect(wrapper.text()).not.toContain('Result')
  })

  it('hands the checked batch to New Bulk Email, which starts from it without uploading again', async () => {
    const { wrapper, router, pinia } = await mountAt('/verify-emails', VerifyEmailsView)
    await choose(wrapper, xlsx('customers.xlsx'))
    expect(api.uploadSpreadsheet).toHaveBeenCalledTimes(1)
    await button(wrapper, 'Continue to New Bulk Email').trigger('click')
    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('new-bulk-email'))
    expect(router.currentRoute.value.query.batch).toBe('7')

    // The next page opens the same batch by its ID.
    setActivePinia(pinia)
    const next = mount(NewBulkEmailView, {
      attachTo: document.body,
      global: {
        plugins: [router, pinia, [VueQueryPlugin, { queryClient }], PrimeVue, ToastService],
      },
    })
    await flushPromises()
    expect(next.text()).toContain('orders.xlsx') // the batch's filename, from the server
    expect(next.text()).toContain('Ready to send')
    const send = next.findAll('button').find((b) => b.text() === 'Send')!
    expect(send.attributes('disabled')).toBeUndefined() // the checked file is ready to send
    expect(api.uploadSpreadsheet).toHaveBeenCalledTimes(1) // not uploaded a second time
  })

  it('New Bulk Email opened directly still starts empty', async () => {
    const { wrapper } = await mountAt('/bulk-emails/new', NewBulkEmailView)
    expect(wrapper.text()).toContain('Drop your .xlsx file here')
  })
})
