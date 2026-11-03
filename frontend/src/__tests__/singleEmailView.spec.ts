import { flushPromises, mount } from '@vue/test-utils'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import { ApiError } from '@/api/errors'
import type { SingleEmail } from '@/api/emails'
import { emailErrors } from '@/composables/useSingleEmail'
import { queryClient } from '@/queries/client'
import { routes } from '@/router/routes'
import type { SettingItem } from '@/types/api'
import SingleEmailView from '@/views/SingleEmailView.vue'

const api = vi.hoisted(() => ({
  sendEmail: vi.fn<(input: SingleEmail) => Promise<string>>(),
  getSettings: vi.fn<() => Promise<SettingItem[]>>(),
}))
vi.mock('@/api/emails', () => ({ sendEmail: api.sendEmail }))
vi.mock('@/api/settings', () => ({ getSettings: api.getSettings }))

const mailFrom: SettingItem = {
  key: 'MAIL_FROM',
  description: '',
  kind: 'string',
  value: 'noreply@shop.example',
  has_value: true,
  source: 'env',
  editable: true,
  secret: false,
  apply: 'instant',
  restart_required: false,
}

async function mountPage() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  router.push('/single-email')
  await router.isReady()
  const wrapper = mount(SingleEmailView, {
    attachTo: document.body,
    global: {
      plugins: [router, createPinia(), [VueQueryPlugin, { queryClient }], PrimeVue, ToastService],
    },
  })
  await flushPromises()
  return wrapper
}
type W = Awaited<ReturnType<typeof mountPage>>

async function fill(
  w: W,
  to = 'sadia@shop.example',
  subject = 'Order update',
  body = 'Your order is on its way.',
) {
  await w.find('#single-to').setValue(to)
  await w.find('#single-subject').setValue(subject)
  await w.find('#single-body').setValue(body)
}
const dialogButton = (text: string) =>
  [...document.body.querySelectorAll<HTMLButtonElement>('.p-dialog button')].find(
    (b) => b.textContent?.trim() === text,
  )!

beforeEach(() => {
  queryClient.clear()
  api.sendEmail.mockReset()
  api.getSettings.mockReset().mockResolvedValue([mailFrom])
  document.body.innerHTML = ''
})

describe('emailErrors', () => {
  it('names what is wrong with each field', () => {
    expect(emailErrors({ to: '', subject: '', body: '' })).toEqual({
      to: 'Enter the recipient’s email address.',
      subject: 'Write a subject.',
      body: 'Write a message.',
    })
    expect(emailErrors({ to: 'not-an-email', subject: 's', body: 'b' }).to).toBe(
      'That does not look like an email address.',
    )
    expect(emailErrors({ to: ' ok@shop.example ', subject: 's', body: 'b' })).toEqual({
      to: '',
      subject: '',
      body: '',
    })
  })
})

describe('Single Email page', () => {
  it('shows the mail window with the sender address from the settings', async () => {
    const w = await mountPage()
    expect(w.find('h1').text()).toBe('Single Email')
    expect(w.text()).toContain('New message')
    const from = w.find('#single-from')
    expect((from.element as HTMLInputElement).value).toBe('noreply@shop.example')
    expect(from.attributes('readonly')).toBeDefined()
    expect(w.text()).not.toContain('sent immediately')
    expect(w.text()).not.toContain('only work in Bulk Emails')
    const order = w.findAll('input').map((i) => i.attributes('id'))
    expect(order.indexOf('single-from')).toBeLessThan(order.indexOf('single-to')) // From sits above To
  })

  it('still works when the sender address cannot be loaded', async () => {
    api.getSettings.mockRejectedValue(new ApiError('settings are not configured', 500, 'http'))
    const w = await mountPage()
    expect(w.text()).toContain('New message')
    expect(w.find('#single-from').exists()).toBe(false)
  })

  it('has a plain "Send" button in the mail window', async () => {
    const w = await mountPage()
    const submit = w.find('button[type="submit"]')
    expect(submit.text()).toBe('Send')
    expect(w.findAll('button').some((b) => b.text() === 'Send email')).toBe(false)
  })

  it('has no "send to myself" shortcut', async () => {
    const w = await mountPage()
    expect(w.text()).not.toContain('myself')
  })

  it('shows errors only after the first try, and sends nothing', async () => {
    const w = await mountPage()
    expect(w.findAll('[role="alert"]')).toHaveLength(0)
    await w.find('form').trigger('submit')
    expect(w.findAll('[role="alert"]').map((a) => a.text())).toEqual([
      'Enter the recipient’s email address.',
      'Write a subject.',
      'Write a message.',
    ])
    expect(document.body.textContent).not.toContain('Send this email now?')
    expect(api.sendEmail).not.toHaveBeenCalled()
  })

  it('flags an address that is not an email', async () => {
    const w = await mountPage()
    await fill(w, 'sadia')
    await w.find('form').trigger('submit')
    expect(w.find('#single-to-error').text()).toBe('That does not look like an email address.')
    expect(w.find('#single-to').attributes('aria-invalid')).toBe('true')
  })

  it('asks for confirmation before sending, naming the recipient', async () => {
    const w = await mountPage()
    await fill(w)
    await w.find('form').trigger('submit')
    await flushPromises()
    expect(document.body.textContent).toContain('Send this email now?')
    expect(document.body.textContent).toContain('sadia@shop.example')
    expect(api.sendEmail).not.toHaveBeenCalled()
  })

  it('sends nothing when the confirmation is cancelled', async () => {
    const w = await mountPage()
    await fill(w)
    await w.find('form').trigger('submit')
    await flushPromises()
    dialogButton('Cancel').click()
    await flushPromises()
    expect(api.sendEmail).not.toHaveBeenCalled()
    expect((w.find('#single-subject').element as HTMLInputElement).value).toBe('Order update')
  })

  it('sends the trimmed email after confirming and shows a banner', async () => {
    api.sendEmail.mockResolvedValue('email sent')
    const w = await mountPage()
    await fill(w, '  sadia@shop.example  ', '  Order update  ')
    await w.find('form').trigger('submit')
    await flushPromises()
    dialogButton('Send email').click()
    await flushPromises()
    expect(api.sendEmail).toHaveBeenCalledWith({
      to: 'sadia@shop.example',
      subject: 'Order update',
      body: 'Your order is on its way.',
    })
    const banner = w.find('[role="status"]')
    expect(banner.text()).toContain('Email sent')
    expect(banner.text()).toContain('Sent to sadia@shop.example')
    expect((w.find('#single-subject').element as HTMLInputElement).value).toBe('  Order update  ') // the form stays filled
  })

  it('hides the banner as soon as the form is edited', async () => {
    api.sendEmail.mockResolvedValue('email sent')
    const w = await mountPage()
    await fill(w)
    await w.find('form').trigger('submit')
    await flushPromises()
    dialogButton('Send email').click()
    await flushPromises()
    expect(w.find('[role="status"]').exists()).toBe(true)
    await w.find('#single-subject').setValue('Another subject')
    expect(w.find('[role="status"]').exists()).toBe(false)
  })

  it('"Send another" empties the form', async () => {
    api.sendEmail.mockResolvedValue('email sent')
    const w = await mountPage()
    await fill(w)
    await w.find('form').trigger('submit')
    await flushPromises()
    dialogButton('Send email').click()
    await flushPromises()
    await w
      .findAll('button')
      .find((b) => b.text() === 'Send another')!
      .trigger('click')
    expect((w.find('#single-to').element as HTMLInputElement).value).toBe('')
    expect(w.find('[role="status"]').exists()).toBe(false)
  })

  it('shows the failure with a hint, keeps the text, and says nothing was sent', async () => {
    api.sendEmail.mockRejectedValue(new ApiError('failed to send email', 502, 'http'))
    const w = await mountPage()
    await fill(w)
    await w.find('form').trigger('submit')
    await flushPromises()
    dialogButton('Send email').click()
    await flushPromises()
    const alert = w.find('[role="alert"]').text()
    expect(alert).toContain('failed to send email')
    expect(alert).toContain('Check the SMTP settings')
    expect(alert).toContain('Nothing was sent')
    expect((w.find('#single-to').element as HTMLInputElement).value).toBe('sadia@shop.example')
    expect(w.find('[role="status"]').exists()).toBe(false)
  })

  it("shows the server's own message for a rejected request", async () => {
    api.sendEmail.mockRejectedValue(new ApiError('"to" is not a valid email address', 400, 'http'))
    const w = await mountPage()
    await fill(w)
    await w.find('form').trigger('submit')
    await flushPromises()
    dialogButton('Send email').click()
    await flushPromises()
    expect(w.find('[role="alert"]').text()).toContain('"to" is not a valid email address')
  })
})
