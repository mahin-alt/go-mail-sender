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
import { queryClient } from '@/queries/client'
import { routes } from '@/router/routes'
import type { Email } from '@/types/api'
import { fakeBatch, typicalRows } from './uploadFakes'

vi.mock('@/api/health', () => ({ getHealth: () => Promise.resolve({ status: 'ok' }) }))

vi.mock('@/api/batches', async () => {
  const { fakeIssues, fakeSample, typicalRows } = await import('./uploadFakes')
  return { listIssues: fakeIssues(typicalRows), getSample: fakeSample(typicalRows) }
})

async function mountReview() {
  queryClient.clear()
  const w = mount(ReviewReport, {
    props: { result: fakeBatch(typicalRows) },
    global: { plugins: [[VueQueryPlugin, { queryClient }]] },
  })
  await flushPromises()
  return w
}

describe('review table reasons', () => {
  it('shows the short reason in each group, without the email address', async () => {
    const w = await mountReview()
    const reasons = () => w.findAll('tbody tr').map((r) => r.findAll('td')[3]!.text())
    expect(reasons()).toEqual(['invalid email', 'domain does not accept mail'])
    await w
      .findAll('[aria-label="Show problems"] button')
      .find((b) => b.text().startsWith('Unverified'))!
      .trigger('click')
    await flushPromises()
    expect(reasons()).toEqual(['could not verify domain'])
  })

  it('has no "rows will be skipped" notice, only the tiles and the table', async () => {
    const w = await mountReview()
    expect(w.text()).not.toContain('will be skipped.')
    expect(w.text()).not.toContain('You can send')
    expect(w.text()).toContain('Will be skipped')
  })
})

describe('jobs table', () => {
  it("shows a failed email's error as the server gave it", () => {
    const job: Email = {
      id: 1,
      batch_id: 1,
      recipient_id: 1,
      row: 2,
      name: 'Ben',
      email: 'ben@shop.example',
      subject: '',
      body: '',
      status: 'failed',
      attempts: 3,
      max_attempts: 3,
      last_error: 'rcpt to: 550 no such user',
      available_at: '',
      sent_at: null,
      created_at: '',
      updated_at: '',
    }
    const w = mount(JobsTable, { props: { jobs: [job] } })
    expect(w.findAll('td')[3]!.text()).toBe('rcpt to: 550 no such user')
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
