import { flushPromises, mount } from '@vue/test-utils'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import AppSidebar from '@/components/layout/AppSidebar.vue'
import JobsTable from '@/components/batches/JobsTable.vue'
import ReviewReport from '@/components/wizard/ReviewReport.vue'
import SendButton from '@/components/wizard/SendButton.vue'
import { jobReason, tidyReason } from '@/lib/problems'
import { queryClient } from '@/queries/client'
import { routes } from '@/router/routes'
import type { Job, ParseResult } from '@/types/api'

vi.mock('@/api/health', () => ({ getHealth: () => Promise.resolve({ status: 'ok' }) }))

describe('tidyReason', () => {
  it.each([
    ['invalid email "not-an-email"', 'invalid email'],
    ['duplicate email "aisha@shop.example"', 'duplicate email'],
    ['domain nope.example does not exist or has no mail server', 'domain does not accept mail'],
    ['domain example.com does not accept mail', 'domain does not accept mail'],
    [
      'could not verify domain slow.example: lookup slow.example on 127.0.0.53:53: server misbehaving',
      'could not verify domain',
    ],
    ['email is empty', 'email is empty'],
    ['name is empty', 'name is empty'],
    ['email has no domain', 'email has no domain'],
  ])('%s -> %s', (reason, shown) => {
    expect(tidyReason(reason)).toBe(shown)
  })

  it('only shortens skipped rows; delivery errors keep the full server text', () => {
    expect(jobReason({ status: 'invalid', last_error: 'invalid email "x@@y"' })).toBe(
      'invalid email',
    )
    expect(
      jobReason({ status: 'failed', last_error: 'acquire smtp connection: connection refused' }),
    ).toBe('acquire smtp connection: connection refused')
    expect(jobReason({ status: 'sent' })).toBe('')
  })
})

const parsed: ParseResult = {
  valid: [{ name: 'Aisha', email: 'aisha@shop.example', row: 2 }],
  problems: [
    { row: 3, name: 'Ben', email: 'ben@@x', reason: 'invalid email "ben@@x"' },
    {
      row: 4,
      name: 'Aisha',
      email: 'aisha@shop.example',
      reason: 'duplicate email "aisha@shop.example"',
    },
    {
      row: 5,
      name: 'Kabir',
      email: 'k@nope.example',
      reason: 'domain nope.example does not exist or has no mail server',
    },
  ],
  unverified_count: 1,
  unverified: [
    {
      row: 6,
      name: 'Lina',
      email: 'lina@slow.example',
      reason: 'could not verify domain slow.example: lookup timed out',
    },
  ],
}

describe('review table reasons', () => {
  it('shows the short reason in each group, without the email address', async () => {
    const w = mount(ReviewReport, { props: { result: parsed } })
    const reasons = () => w.findAll('tbody tr').map((r) => r.findAll('td')[3]!.text())
    expect(reasons()).toEqual(['invalid email', 'duplicate email', 'domain does not accept mail'])
    await w
      .findAll('[aria-label="Show problems"] button')
      .find((b) => b.text().startsWith('Unverified'))!
      .trigger('click')
    expect(reasons()).toEqual(['could not verify domain'])
  })

  it('has no "rows will be skipped" notice, only the tiles and the table', () => {
    const w = mount(ReviewReport, { props: { result: parsed } })
    expect(w.text()).not.toContain('will be skipped.')
    expect(w.text()).not.toContain('You can send')
    expect(w.text()).toContain('Will be skipped')
  })
})

describe('jobs table', () => {
  it('shows the short reason for a skipped row', () => {
    const job: Job = {
      id: 1,
      batch_id: 1,
      recipient_name: 'Ben',
      recipient_email: 'ben@@x',
      subject: '',
      body: '',
      status: 'invalid',
      attempts: 0,
      max_attempts: 3,
      last_error: 'invalid email "ben@@x"',
      run_at: '',
      created_at: '',
      updated_at: '',
    }
    const w = mount(JobsTable, { props: { jobs: [job] } })
    expect(w.findAll('td')[3]!.text()).toBe('invalid email')
  })
})

describe('send button', () => {
  it('is just a "Send" button in the same style as Save, not stuck to the screen, with the reason it is off for assistive tech', () => {
    const w = mount(SendButton, { props: { valid: 296, blocker: '' } })
    expect(w.text()).toBe('Send')
    expect(w.find('button').attributes('aria-label')).toBe('Send to 296 recipients')
    expect(w.find('button').classes()).toEqual(
      expect.arrayContaining(['bg-ink', 'text-white', 'rounded-full']),
    )
    expect(w.find('button').classes()).toEqual(
      expect.arrayContaining(['px-6', 'py-3', 'text-base']), // slightly bigger than Save
    )
    expect(w.html()).not.toContain('sticky')
    expect(w.html()).not.toContain('fixed')
    const off = mount(SendButton, { props: { valid: 0, blocker: 'Write a subject' } })
    expect(off.find('button').attributes('disabled')).toBeDefined()
    expect(off.find('[role="status"]').text()).toBe('Write a subject')
    expect(off.find('[role="status"]').classes()).toContain('sr-only')
  })
})

describe('sidebar collapse button', () => {
  async function mountSidebar(pinned: boolean, expanded: boolean) {
    const router = createRouter({ history: createMemoryHistory(), routes })
    router.push('/dashboard')
    await router.isReady()
    const w = mount(AppSidebar, {
      props: { pinned, expanded },
      global: {
        plugins: [router, createPinia(), [VueQueryPlugin, { queryClient }], PrimeVue, ToastService],
      },
    })
    await flushPromises()
    return w
  }
  beforeEach(() => queryClient.clear())

  it('is an icon next to the logo, with no visible text, and not at the bottom', async () => {
    const w = await mountSidebar(true, true)
    const toggle = w.find('button[aria-expanded]')
    expect(toggle.text()).toBe('')
    expect(toggle.attributes('aria-label')).toBe('Collapse sidebar')
    const header = toggle.element.parentElement!
    expect(header.textContent).toContain('MailFlow') // the logo is in the same row
    expect(w.findAll('button')).toHaveLength(1)
    const bottom = w.find('[role="status"]').element.parentElement!
    expect(bottom.querySelector('button')).toBeNull()
  })

  it('offers to keep the sidebar open when it is collapsed, and hides itself in the rail until it opens', async () => {
    const rail = await mountSidebar(false, false)
    expect(rail.find('button[aria-expanded]').attributes('aria-label')).toBe('Keep sidebar open')
    expect(rail.find('button[aria-expanded]').classes()).toContain('opacity-0')
    const opened = await mountSidebar(false, true)
    expect(opened.find('button[aria-expanded]').classes()).toContain('opacity-100')
  })

  it('emits toggle when clicked', async () => {
    const w = await mountSidebar(true, true)
    await w.find('button[aria-expanded]').trigger('click')
    expect(w.emitted('toggle')).toHaveLength(1)
  })
})
