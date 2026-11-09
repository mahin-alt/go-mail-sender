import { flushPromises, mount } from '@vue/test-utils'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import { ApiError } from '@/api/errors'
import type { NewBulkEmail } from '@/api/recipients'
import { queryClient } from '@/queries/client'
import { routes } from '@/router/routes'
import type { ProblemParams } from '@/api/uploads'
import type { CreateBatchResult, ProblemPage, SettingItem, UploadRow, UploadSummary } from '@/types/api'
import NewBulkEmailView from '@/views/NewBulkEmailView.vue'
import { fakeUpload, typicalRows } from './uploadFakes'

const api = vi.hoisted(() => ({
  createBatch: vi.fn<(input: NewBulkEmail) => Promise<CreateBatchResult>>(),
}))
vi.mock('@/api/recipients', () => api)

const uploads = vi.hoisted(() => ({
  createUpload: vi.fn<(file: File) => Promise<UploadSummary>>(),
  listUploadProblems: vi.fn<(id: number, params?: ProblemParams) => Promise<ProblemPage>>(),
  getUploadSample: vi.fn<(id: number, limit?: number) => Promise<UploadRow[]>>(),
}))
vi.mock('@/api/uploads', () => uploads)

/** Make the server answer as if the chosen file held these rows. */
function serverHas(rows: UploadRow[]) {
  const fake = fakeUpload(rows)
  uploads.createUpload.mockResolvedValue(fake.summary)
  uploads.listUploadProblems.mockImplementation(fake.problems)
  uploads.getUploadSample.mockImplementation(fake.sample)
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

const xlsx = (name = 'orders.xlsx', size = 2048) => new File([new Uint8Array(size)], name)

async function mountPage() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  router.push('/bulk-emails/new')
  await router.isReady()
  const wrapper = mount(NewBulkEmailView, {
    attachTo: document.body,
    global: {
      plugins: [router, createPinia(), [VueQueryPlugin, { queryClient }], PrimeVue, ToastService],
    },
  })
  await flushPromises()
  return { wrapper, router }
}

async function choose(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper'], file: File) {
  const input = wrapper.find('input[type="file"]')
  Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
  await input.trigger('change')
  await flushPromises()
}

const sendButton = (w: Awaited<ReturnType<typeof mountPage>>['wrapper']) =>
  w.findAll('button').find((b) => b.text() === 'Send')!

beforeEach(() => {
  queryClient.clear()
  Object.values(api).forEach((fn) => fn.mockReset())
  Object.values(uploads).forEach((fn) => fn.mockReset())
  serverHas(typicalRows)
  settingsApi.getSettings.mockReset()
  settingsApi.getSettings.mockResolvedValue([uploadLimit(10)])
  document.body.innerHTML = ''
})

describe('New Bulk Email page', () => {
  it('starts locked: review and write are unavailable until a file is checked', async () => {
    const { wrapper } = await mountPage()
    const sections = wrapper.findAll('section')
    expect(sections[1]!.attributes('inert')).toBeDefined()
    expect(sections[2]!.attributes('inert')).toBeDefined()
    expect(sendButton(wrapper).attributes('disabled')).toBeDefined()
  })

  it('checks the file on the server and shows what will happen', async () => {
    const { wrapper } = await mountPage()
    const file = xlsx()
    await choose(wrapper, file)
    expect(uploads.createUpload).toHaveBeenCalledWith(file)
    expect(wrapper.text()).toContain('Ready to send')
    expect(wrapper.text()).toContain('Will be skipped')
    expect(wrapper.text()).not.toContain('rows will be skipped')
    expect(wrapper.text()).not.toContain('will be sent ·')
    expect(sendButton(wrapper).attributes('disabled')).toBeUndefined()
    expect(wrapper.findAll('section')[1]!.attributes('inert')).toBeUndefined()
  })

  it('groups the problems and lists the unverified rows separately', async () => {
    const { wrapper } = await mountPage()
    await choose(wrapper, xlsx())
    const tab = (label: string) =>
      wrapper
        .findAll('[aria-label="Show problems"] button')
        .find((b) => b.text().startsWith(label))!
    expect(tab('All problems').text()).toContain('3')
    await tab('Duplicate').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('tbody tr')).toHaveLength(1)
    expect(wrapper.find('tbody').text()).toContain('duplicate email')
    await tab('Unverified').trigger('click')
    await flushPromises()
    expect(wrapper.find('tbody').text()).toContain('lina@slow.example')
  })

  it('pages through problems with Load more instead of loading them all', async () => {
    const many: UploadRow[] = Array.from({ length: 150 }, (_, i) => ({
      row: i + 2,
      name: `P${i}`,
      email: `p${i}@`,
      kind: 'invalid_email',
      reason: `invalid email "p${i}@"`,
    }))
    serverHas([{ row: 1000, name: 'Ok', email: 'ok@shop.example', kind: 'ok' }, ...many])
    const { wrapper } = await mountPage()
    await choose(wrapper, xlsx())
    expect(wrapper.findAll('tbody tr')).toHaveLength(100)
    expect(wrapper.text()).toContain('100 of 150')
    expect(uploads.listUploadProblems).toHaveBeenLastCalledWith(7, {
      kinds: [],
      q: '',
      after: 0,
      limit: 100,
    })

    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Load more')!
      .trigger('click')
    await flushPromises()
    expect(wrapper.findAll('tbody tr')).toHaveLength(150)
    expect(uploads.listUploadProblems).toHaveBeenLastCalledWith(7, {
      kinds: [],
      q: '',
      after: 101, // row of the 100th problem
      limit: 100,
    })
    expect(wrapper.findAll('button').some((b) => b.text() === 'Load more')).toBe(false)
  })

  it('asks the server for one tab at a time', async () => {
    const { wrapper } = await mountPage()
    await choose(wrapper, xlsx())
    await wrapper
      .findAll('[aria-label="Show problems"] button')
      .find((b) => b.text().startsWith('Invalid'))!
      .trigger('click')
    await flushPromises()
    expect(uploads.listUploadProblems).toHaveBeenLastCalledWith(7, {
      kinds: ['empty_email', 'empty_name', 'invalid_email'],
      q: '',
      after: 0,
      limit: 100,
    })
    expect(wrapper.find('tbody').text()).toContain('ben@@x')
  })

  it('prefills the message and previews it for the first recipient', async () => {
    const { wrapper } = await mountPage()
    await choose(wrapper, xlsx())
    expect((wrapper.find('#subject').element as HTMLInputElement).value).toBe(
      'Update on your order, {{.Name}}',
    )
    expect(wrapper.text()).toContain('Update on your order, Aisha')
    expect(wrapper.text()).toContain('Preview for recipient 1 of 3') // ok + unverified rows
    await wrapper.find('button[aria-label="Next recipient"]').trigger('click')
    expect(wrapper.text()).toContain('Update on your order, রহিম')
  })

  it('warns about unknown fields and blocks sending', async () => {
    const { wrapper } = await mountPage()
    await choose(wrapper, xlsx())
    await wrapper.find('#subject').setValue('Hi {{.Phone}}')
    expect(wrapper.find('[role="alert"]').text()).toContain('Unknown field')
    expect(sendButton(wrapper).attributes('disabled')).toBeDefined()
  })

  it('blocks sending an empty subject', async () => {
    const { wrapper } = await mountPage()
    await choose(wrapper, xlsx())
    await wrapper.find('#subject').setValue('   ')
    expect(wrapper.text()).toContain('Write a subject')
    expect(sendButton(wrapper).attributes('disabled')).toBeDefined()
  })

  it('refuses a file that is not .xlsx without calling the server', async () => {
    const { wrapper } = await mountPage()
    await choose(wrapper, xlsx('notes.csv'))
    expect(uploads.createUpload).not.toHaveBeenCalled()
    expect(wrapper.find('[role="alert"]').text()).toBe('Only .xlsx files are supported.')
  })

  it('refuses a file over 10 MB', async () => {
    const { wrapper } = await mountPage()
    await choose(wrapper, xlsx('big.xlsx', 10 * 1024 * 1024 + 1))
    expect(wrapper.find('[role="alert"]').text()).toContain('10 MB')
  })

  it('uses the upload limit from Settings', async () => {
    settingsApi.getSettings.mockResolvedValue([uploadLimit(40)])
    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('Up to 40 MB.')

    const big = xlsx('big.xlsx', 18 * 1024 * 1024)
    await choose(wrapper, big)
    expect(uploads.createUpload).toHaveBeenCalledWith(big)

    await choose(wrapper, xlsx('huge.xlsx', 40 * 1024 * 1024 + 1))
    expect(wrapper.find('[role="alert"]').text()).toBe('The file is too large. The limit is 40 MB.')
  })

  it("shows the server's message when the file cannot be read", async () => {
    uploads.createUpload.mockRejectedValue(
      new ApiError('missing required columns: email', 400, 'http'),
    )
    const { wrapper } = await mountPage()
    await choose(wrapper, xlsx())
    expect(wrapper.find('[role="alert"]').text()).toBe('missing required columns: email')
    expect(wrapper.findAll('section')[1]!.attributes('inert')).toBeDefined()
  })

  it('says so when no row is valid, and cannot send', async () => {
    serverHas(typicalRows.filter((r) => r.kind !== 'ok' && r.kind !== 'unverified'))
    const { wrapper } = await mountPage()
    await choose(wrapper, xlsx())
    expect(wrapper.text()).toContain('No valid emails in this file')
    expect(sendButton(wrapper).attributes('disabled')).toBeDefined()
  })

  it('removes the file and starts again', async () => {
    const { wrapper } = await mountPage()
    await choose(wrapper, xlsx())
    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Remove')!
      .trigger('click')
    expect(wrapper.text()).toContain('Drop your .xlsx file here')
    expect(wrapper.findAll('section')[1]!.attributes('inert')).toBeDefined()
  })

  it('asks for confirmation, then queues the batch and opens its page', async () => {
    api.createBatch.mockResolvedValue({
      batch_id: 45,
      filename: 'orders.xlsx',
      total_rows: 5,
      valid_count: 2,
      invalid_count: 3,
      status: 'pending',
      unverified_count: 1,
      message: 'ok',
    })
    const { wrapper, router } = await mountPage()
    const file = xlsx()
    await choose(wrapper, file)
    await sendButton(wrapper).trigger('click')
    await flushPromises()
    expect(api.createBatch).not.toHaveBeenCalled()
    const dialog = document.body.querySelector('.p-dialog')!
    expect(dialog.querySelector('.p-dialog-title')!.textContent).toBe('Confirm')
    expect(dialog.textContent).toContain(
      '3 valid recipients will receive emails. 3 invalid entries will be excluded from sending.',
    )
    expect(dialog.textContent).not.toContain('This cannot be undone')
    const confirm = [...dialog.querySelectorAll('button')].find(
      (b) => b.textContent?.trim() === 'Send',
    )!
    confirm.click()
    await flushPromises()
    expect(api.createBatch).toHaveBeenCalledWith({
      file,
      subject: 'Update on your order, {{.Name}}',
      body: expect.stringContaining('Hello {{.Name}}'),
    })
    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('bulk-email-detail'))
    expect(router.currentRoute.value.params.id).toBe('45')
  })

  it('centres the title, leaves room before the buttons, and has just Cancel and Send', async () => {
    const { wrapper } = await mountPage()
    await choose(wrapper, xlsx())
    await sendButton(wrapper).trigger('click')
    await flushPromises()
    const dialog = document.body.querySelector('.p-dialog')!
    expect(dialog.querySelector('.p-dialog-header')!.classList.contains('justify-center')).toBe(
      true,
    )
    expect(dialog.querySelector('.p-dialog-title')!.classList.contains('text-center')).toBe(true)
    expect(dialog.querySelector('.p-dialog-content')!.classList.contains('pb-7')).toBe(true)
    expect(dialog.querySelector('.p-dialog-content p')!.classList.contains('text-center')).toBe(
      true,
    )
    expect([...dialog.querySelectorAll('button')].map((b) => b.textContent?.trim())).toEqual([
      'Cancel',
      'Send',
    ])
  })

  it('leaves out the invalid-entries sentence when nothing is skipped, and uses the singular for one', async () => {
    serverHas(typicalRows.slice(0, 1))
    const { wrapper } = await mountPage()
    await choose(wrapper, xlsx())
    await sendButton(wrapper).trigger('click')
    await flushPromises()
    const text = document.body.querySelector('.p-dialog-content')!.textContent
    expect(text).toBe('1 valid recipient will receive emails.')
  })

  it('sends nothing when the confirmation is cancelled', async () => {
    const { wrapper } = await mountPage()
    await choose(wrapper, xlsx())
    await sendButton(wrapper).trigger('click')
    await flushPromises()
    ;[...document.body.querySelectorAll<HTMLButtonElement>('.p-dialog button')]
      .find((b) => b.textContent?.trim() === 'Cancel')!
      .click()
    await flushPromises()
    expect(api.createBatch).not.toHaveBeenCalled()
  })

  it('stays on the page and keeps everything when sending fails', async () => {
    api.createBatch.mockRejectedValue(
      new ApiError('invalid body template: unexpected }', 400, 'http'),
    )
    const { wrapper, router } = await mountPage()
    await choose(wrapper, xlsx())
    await sendButton(wrapper).trigger('click')
    await flushPromises()
    ;[...document.body.querySelectorAll<HTMLButtonElement>('.p-dialog button')]
      .find((b) => b.textContent?.trim() === 'Send')!
      .click()
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('new-bulk-email')
    expect(wrapper.text()).toContain('orders.xlsx')
  })
})
