import { flushPromises, mount } from '@vue/test-utils'
import { VueQueryPlugin } from '@tanstack/vue-query'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import { ApiError } from '@/api/errors'
import type { SettingsChange, TestEmailRequest } from '@/api/settings'
import { queryClient } from '@/queries/client'
import { routes } from '@/router/routes'
import type { SettingItem } from '@/types/api'
import SettingsView from '@/views/SettingsView.vue'

const api = vi.hoisted(() => ({
  getSettings: vi.fn<() => Promise<SettingItem[]>>(),
  updateSettings: vi.fn<(c: SettingsChange) => Promise<SettingItem[]>>(),
  sendTestEmail: vi.fn<(r: TestEmailRequest) => Promise<string>>(),
}))
vi.mock('@/api/settings', () => api)

const base = {
  description: 'd',
  has_value: true,
  editable: true,
  secret: false,
  apply: 'instant' as const,
  restart_required: false,
  source: 'env' as const,
}
const make = (): SettingItem[] => [
  { ...base, key: 'MAIL_FROM', kind: 'string', value: 'noreply@shop.example' },
  { ...base, key: 'SMTP_HOST', kind: 'string', value: 'smtp.example.com', source: 'override' },
  { ...base, key: 'SMTP_PORT', kind: 'int', value: '587' },
  {
    ...base,
    key: 'SMTP_TLS_MODE',
    kind: 'enum',
    value: 'starttls',
    options: ['auto', 'none', 'starttls', 'tls'],
  },
  { ...base, key: 'SMTP_USERNAME', kind: 'string', value: 'apikey' },
  { ...base, key: 'SMTP_PASSWORD', kind: 'string', value: '', secret: true },
  { ...base, key: 'DOMAIN_CHECK_ENABLED', kind: 'bool', value: 'true' },
  { ...base, key: 'MAX_JOB_RETRIES', kind: 'int', value: '3' },
  {
    ...base,
    key: 'WORKER_COUNT',
    kind: 'int',
    value: '6',
    apply: 'restart',
    restart_required: true,
  },
  { ...base, key: 'SMTP_POOL_SIZE', kind: 'int', value: '3', apply: 'restart' },
  { ...base, key: 'JOB_POLL_INTERVAL', kind: 'duration', value: '500ms', apply: 'restart' },
  { ...base, key: 'HTTP_ADDR', kind: 'string', value: ':8080', editable: false, apply: 'readonly' },
  {
    ...base,
    key: 'HTTP_READ_TIMEOUT',
    kind: 'duration',
    value: '30s',
    editable: false,
    apply: 'readonly',
  },
  {
    ...base,
    key: 'HTTP_WRITE_TIMEOUT',
    kind: 'duration',
    value: '30s',
    editable: false,
    apply: 'readonly',
  },
]

async function mountPage() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  router.push('/settings')
  await router.isReady()
  const wrapper = mount(SettingsView, {
    attachTo: document.body,
    global: { plugins: [router, [VueQueryPlugin, { queryClient }], PrimeVue, ToastService] },
  })
  await flushPromises()
  return wrapper
}
type W = Awaited<ReturnType<typeof mountPage>>
const tab = (w: W, name: string) =>
  w.findAll('[role="tab"]').find((t) => t.text().startsWith(name))!
const button = (w: W, text: string) => w.findAll('button').find((b) => b.text().startsWith(text))!

beforeEach(() => {
  queryClient.clear()
  Object.values(api).forEach((fn) => fn.mockReset())
  api.getSettings.mockResolvedValue(make())
  document.body.innerHTML = ''
})

describe('Settings page', () => {
  it('has four tabs and starts on Sender and SMTP, without the long no-login warning', async () => {
    const w = await mountPage()
    expect(w.findAll('[role="tab"]').map((t) => t.text())).toEqual([
      'Sender and SMTP',
      'Sending behaviour',
      'Server limits',
      'Read-only',
    ])
    expect(tab(w, 'Sender').attributes('aria-selected')).toBe('true')
    expect(w.text()).not.toContain('There is no login yet')
    expect(w.text()).toContain('Where email is sent from') // each tab keeps its one-line explanation
  })

  it('shows the setting names without descriptions or tags, and keeps Reset to default', async () => {
    const w = await mountPage()
    expect(w.text()).not.toContain('Applies instantly')
    expect(w.text()).not.toContain('Changed here')
    expect(w.text()).not.toContain('From environment')
    expect(w.find('label[for="setting-MAIL_FROM"]').attributes('title')).toBe('d') // description is only a tooltip
    expect(w.text()).toContain('Reset to default')
  })

  it('never shows the password, only whether one is set', async () => {
    const w = await mountPage()
    expect(w.find('input[type="password"]').exists()).toBe(false)
    expect(w.text()).toContain('Password is set')
    await button(w, 'Change password').trigger('click')
    expect((w.find('input[type="password"]').element as HTMLInputElement).value).toBe('')
  })

  it('starts with Save and Discard off, and turns them on after an edit with a dot on the tab', async () => {
    const w = await mountPage()
    expect(button(w, 'Save').attributes('disabled')).toBeDefined()
    await w.find('#setting-SMTP_PORT').setValue('2525')
    expect(button(w, 'Save').attributes('disabled')).toBeUndefined()
    expect(tab(w, 'Sender').text()).toContain('unsaved changes')
    await button(w, 'Discard').trigger('click')
    expect((w.find('#setting-SMTP_PORT').element as HTMLInputElement).value).toBe('587')
  })

  it('calls the save button just "Save" on every tab, even when the SMTP settings are being changed', async () => {
    const w = await mountPage()
    await w.find('#setting-SMTP_PORT').setValue('2525')
    const save = w.findAll('button').filter((b) => b.text().startsWith('Save'))
    expect(save.map((b) => b.text())).toEqual(['Save'])
    expect(w.text()).not.toContain('Test connection')
    await tab(w, 'Sending').trigger('click')
    expect(
      w
        .findAll('button')
        .filter((b) => b.text().startsWith('Save'))
        .map((b) => b.text()),
    ).toEqual(['Save'])
  })

  it('keeps edits when you switch tabs', async () => {
    const w = await mountPage()
    await w.find('#setting-SMTP_PORT').setValue('2525')
    await tab(w, 'Sending').trigger('click')
    await tab(w, 'Sender').trigger('click')
    expect((w.find('#setting-SMTP_PORT').element as HTMLInputElement).value).toBe('2525')
  })

  it('saves only what changed', async () => {
    api.updateSettings.mockResolvedValue(make())
    const w = await mountPage()
    await w.find('#setting-SMTP_PORT').setValue('2525')
    await button(w, 'Save').trigger('click')
    await flushPromises()
    expect(api.updateSettings).toHaveBeenCalledWith({ values: { SMTP_PORT: '2525' } })
  })

  it('sends a new password only when one was typed', async () => {
    api.updateSettings.mockResolvedValue(make())
    const w = await mountPage()
    await button(w, 'Change password').trigger('click')
    await w.find('input[type="password"]').setValue('s3cret')
    await button(w, 'Save').trigger('click')
    await flushPromises()
    expect(api.updateSettings).toHaveBeenCalledWith({ values: { SMTP_PASSWORD: 's3cret' } })
  })

  it('removes the password by saving an empty one', async () => {
    api.updateSettings.mockResolvedValue(make())
    const w = await mountPage()
    await button(w, 'Remove').trigger('click')
    expect(w.text()).toContain('The password will be removed')
    await button(w, 'Save').trigger('click')
    await flushPromises()
    expect(api.updateSettings).toHaveBeenCalledWith({ values: { SMTP_PASSWORD: '' } })
  })

  it('saves a switch as the text true or false', async () => {
    api.updateSettings.mockResolvedValue(make())
    const w = await mountPage()
    await tab(w, 'Sending').trigger('click')
    await w.find('#setting-DOMAIN_CHECK_ENABLED').trigger('click')
    await button(w, 'Save').trigger('click')
    await flushPromises()
    expect(api.updateSettings).toHaveBeenCalledWith({ values: { DOMAIN_CHECK_ENABLED: 'false' } })
  })

  it('puts the server message under the field it names and keeps the edit', async () => {
    api.updateSettings.mockRejectedValue(
      new ApiError('SMTP_PORT must be a whole number between 1 and 65535', 400, 'http'),
    )
    const w = await mountPage()
    await w.find('#setting-SMTP_PORT').setValue('99999')
    await button(w, 'Save').trigger('click')
    await flushPromises()
    expect(w.find('#setting-SMTP_PORT-error').text()).toContain('between 1 and 65535')
    expect((w.find('#setting-SMTP_PORT').element as HTMLInputElement).value).toBe('99999')
  })

  it('shows a refused connection above the buttons, and saves nothing', async () => {
    api.updateSettings.mockRejectedValue(
      new ApiError('not saved: the new SMTP settings do not work: connection refused', 400, 'http'),
    )
    const w = await mountPage()
    await w.find('#setting-SMTP_HOST').setValue('nope.example')
    await button(w, 'Save').trigger('click')
    await flushPromises()
    expect(w.find('[role="tabpanel"] [role="alert"]').text()).toContain('connection refused')
    expect(tab(w, 'Sender').text()).toContain('unsaved changes')
  })

  it('resets one setting to its default right away', async () => {
    api.updateSettings.mockResolvedValue(make())
    const w = await mountPage()
    await button(w, 'Reset to default').trigger('click')
    await flushPromises()
    expect(api.updateSettings).toHaveBeenCalledWith({ reset: ['SMTP_HOST'] })
  })

  it('tests the email with the values on screen, not the saved ones', async () => {
    api.sendTestEmail.mockResolvedValue('test email sent to me@shop.example')
    const w = await mountPage()
    await w.find('#setting-SMTP_HOST').setValue('smtp.other.example')
    await w.find('input[aria-label="Send the test to"]').setValue('me@shop.example')
    await button(w, 'Send test email').trigger('click')
    await flushPromises()
    expect(api.sendTestEmail).toHaveBeenCalledWith({
      to: 'me@shop.example',
      from: 'noreply@shop.example',
      host: 'smtp.other.example',
      port: '587',
      tls_mode: 'starttls',
      username: 'apikey',
    })
    expect(w.text()).toContain('test email sent to me@shop.example')
  })

  it('shows why a test email failed', async () => {
    api.sendTestEmail.mockRejectedValue(
      new ApiError('test email failed: dial tcp: connection refused', 502, 'http'),
    )
    const w = await mountPage()
    await button(w, 'Send test email').trigger('click')
    await flushPromises()
    expect(w.text()).toContain('connection refused')
  })

  it('tells you a restart is needed on the Server limits tab', async () => {
    const w = await mountPage()
    await tab(w, 'Server').trigger('click')
    expect(w.text()).toContain('Restart the server to apply')
    expect(w.text()).not.toContain('Needs a restart')
  })

  it('has no Save buttons on the Read-only tab and the boxes cannot be edited', async () => {
    const w = await mountPage()
    await tab(w, 'Read-only').trigger('click')
    expect(w.findAll('button').some((b) => b.text().startsWith('Save'))).toBe(false)
    expect(w.find('#setting-HTTP_ADDR').attributes('readonly')).toBeDefined()
  })

  it('moves between tabs with the arrow keys', async () => {
    const w = await mountPage()
    await tab(w, 'Sender').trigger('keydown', { key: 'ArrowRight' })
    expect(tab(w, 'Sending').attributes('aria-selected')).toBe('true')
  })

  it('shows the server message and a way to try again if the settings cannot be loaded', async () => {
    api.getSettings.mockRejectedValue(new ApiError('settings are not configured', 500, 'http'))
    const w = await mountPage()
    await vi.waitFor(() => expect(w.find('[role="alert"]').exists()).toBe(true), { timeout: 4000 })
    expect(w.find('[role="alert"]').text()).toContain('settings are not configured')
  })
})
