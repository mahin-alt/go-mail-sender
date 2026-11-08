import { flushPromises, mount } from '@vue/test-utils'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import { ApiError } from '@/api/errors'
import type { BatchListParams } from '@/api/batches'
import type { StatsParams } from '@/api/stats'
import { attentionReasons, dayLabel, dayOfMonth, queueCaption } from '@/lib/dashboard'
import { queryClient } from '@/queries/client'
import { routes } from '@/router/routes'
import type { AttentionBatch, BatchSummary, Paged, Stats } from '@/types/api'
import DashboardView from '@/views/DashboardView.vue'

const api = vi.hoisted(() => ({
  getStats: vi.fn<(p: StatsParams) => Promise<Stats>>(),
  listBatches: vi.fn<(p: BatchListParams) => Promise<Paged<BatchSummary>>>(),
}))
vi.mock('@/api/stats', () => ({ getStats: api.getStats }))
vi.mock('@/api/batches', () => ({ listBatches: api.listBatches }))

/** jsdom has no canvas, so the real chart cannot draw. This stand-in just records what it was given. */
const chartProps = vi.hoisted(() => ({ last: null as null | { data: unknown; options: unknown } }))
vi.mock('vue-chartjs', async () => {
  const { defineComponent: define, h: hh } = await import('vue')
  return {
    Bar: define({
      props: ['data', 'options'],
      setup(props: { data: unknown; options: unknown }) {
        return () => {
          chartProps.last = { data: props.data, options: props.options }
          return hh('div', { 'data-chart': 'bar' })
        }
      },
    }),
  }
})

const daily = (n: number): Stats['daily'] =>
  Array.from({ length: n }, (_, i) => ({
    date: `2026-10-${String(30 - n + i + 1).padStart(2, '0')}`,
    sent: 100 + i,
    failed: i === n - 1 ? 79 : 0,
  }))

const stats = (over: Partial<Stats> = {}, days = 14): Stats => ({
  generated_at: '2026-11-02T12:00:00Z',
  timezone: 'UTC',
  today: { sent: 1243, failed: 79 },
  queue: { pending: 86, retrying: 3, processing: 5 },
  failed_total: 89,
  active_batches: 1,
  daily: daily(days),
  needs_attention: [
    {
      batch_id: 42,
      filename: 'refunds_oct27.xlsx',
      status: 'completed',
      total: 312,
      failed: 12,
      invalid: 0,
      created_at: '',
      reasons: ['failed_jobs'],
    },
    {
      batch_id: 36,
      filename: 'stock_alerts.xlsx',
      status: 'completed',
      total: 98,
      failed: 0,
      invalid: 41,
      created_at: '',
      reasons: ['high_invalid_rate'],
    },
  ],
  ...over,
})

const batch = (over: Partial<BatchSummary> = {}): BatchSummary => ({
  id: 45,
  filename: 'orders_oct28.xlsx',
  status: 'completed',
  total: 1000,
  sent: 1000,
  failed: 0,
  pending: 0,
  processing: 0,
  invalid: 0,
  cancelled: 0,
  created_at: new Date().toISOString(),
  ...over,
})

async function mountPage() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  router.push('/dashboard')
  await router.isReady()
  const wrapper = mount(DashboardView, {
    attachTo: document.body,
    global: {
      plugins: [router, createPinia(), [VueQueryPlugin, { queryClient }], PrimeVue, ToastService],
    },
  })
  await flushPromises()
  await flushPromises()
  return wrapper
}
type W = Awaited<ReturnType<typeof mountPage>>
const tileValue = (w: W, label: string) => {
  const tile = w.findAll('span').find((s) => s.text() === label)!.element.parentElement!
  return tile.querySelector('strong')!.textContent
}

beforeEach(() => {
  queryClient.clear()
  chartProps.last = null
  api.getStats.mockReset().mockImplementation(async (p) => stats({}, p.days))
  api.listBatches.mockReset().mockResolvedValue({
    items: [batch(), batch({ id: 44, filename: 'payouts_nov.xlsx' })],
    total: 2,
    limit: 5,
    offset: 0,
  })
  document.body.innerHTML = ''
})

describe('dashboard helpers', () => {
  it('describes why a Bulk Email needs attention', () => {
    const base: AttentionBatch = {
      batch_id: 1,
      filename: 'a.xlsx',
      status: 'completed',
      total: 100,
      failed: 12,
      invalid: 25,
      created_at: '',
      reasons: ['failed_jobs', 'high_invalid_rate'],
    }
    expect(attentionReasons(base)).toEqual(['12 failed emails', '25% of rows were invalid'])
    expect(attentionReasons({ ...base, failed: 1, reasons: ['failed_jobs'] })).toEqual([
      '1 failed email',
    ])
  })
  it('summarises the queue', () => {
    expect(queueCaption({ pending: 0, retrying: 0, processing: 0 })).toBe('nothing waiting')
    expect(queueCaption({ pending: 86, retrying: 3, processing: 5 })).toBe(
      '5 sending now · 3 retrying',
    )
    expect(queueCaption({ pending: 9, retrying: 0, processing: 2 })).toBe('2 sending now')
  })
  it('reads a calendar day the same in every time zone', () => {
    expect(dayLabel('2026-11-02')).toBe('2 Nov')
    expect(dayOfMonth('2026-11-02')).toBe('2')
  })
})

describe('Dashboard page', () => {
  it('shows the four tiles with the real numbers', async () => {
    const w = await mountPage()
    expect(w.find('h1').text()).toBe('Dashboard')
    expect(tileValue(w, 'Sent today')).toBe('1,243')
    expect(tileValue(w, 'Failed today')).toBe('79')
    expect(tileValue(w, 'In the queue')).toBe('91')
    expect(tileValue(w, 'Active Bulk Emails')).toBe('1')
    expect(w.text()).toContain('5 sending now · 3 retrying')
  })

  it('asks the API for the browser time zone and 14 days to start with', async () => {
    await mountPage()
    expect(api.getStats).toHaveBeenCalledWith({
      days: 14,
      tz: Intl.DateTimeFormat().resolvedOptions().timeZone,
    })
  })

  it('draws two bars per day, sent and failed, side by side', async () => {
    const w = await mountPage()
    await vi.waitFor(() => expect(w.find('[data-chart="bar"]').exists()).toBe(true)) // the chart loads on demand
    const data = chartProps.last!.data as {
      labels: string[]
      datasets: { label: string; data: number[] }[]
    }
    expect(data.labels).toHaveLength(14)
    expect(data.datasets.map((d) => d.label)).toEqual(['Sent', 'Failed'])
    expect(data.datasets[1]!.data[data.datasets[1]!.data.length - 1]).toBe(79)
    expect(w.find('[role="img"]').attributes('aria-label')).toContain('last 14 days')
  })

  it('switches the range and asks for that many days', async () => {
    const w = await mountPage()
    await vi.waitFor(() => expect(chartProps.last).not.toBeNull())
    await w
      .findAll('[aria-label="Time range"] button')
      .find((b) => b.text() === '7 days')!
      .trigger('click')
    await flushPromises()
    expect(api.getStats).toHaveBeenLastCalledWith(expect.objectContaining({ days: 7 }))
    expect(w.find('#activity-title').text()).toBe('Last 7 days')
    await vi.waitFor(() =>
      expect((chartProps.last!.data as { labels: string[] }).labels).toHaveLength(7),
    )
    await w
      .findAll('[aria-label="Time range"] button')
      .find((b) => b.text() === '30 days')!
      .trigger('click')
    await flushPromises()
    expect(api.getStats).toHaveBeenLastCalledWith(expect.objectContaining({ days: 30 }))
  })

  it('can show the same numbers as a table, newest day first', async () => {
    const w = await mountPage()
    await w
      .findAll('button')
      .find((b) => b.text() === 'Show as table')!
      .trigger('click')
    const rows = w.findAll('[aria-labelledby="activity-title"] tbody tr')
    expect(rows).toHaveLength(14)
    expect(rows[0]!.text()).toContain('79')
    expect(w.find('[data-chart="bar"]').exists()).toBe(false)
    await w
      .findAll('button')
      .find((b) => b.text() === 'Show chart')!
      .trigger('click')
    await vi.waitFor(() => expect(w.find('[data-chart="bar"]').exists()).toBe(true))
  })

  it('lists what needs attention with a Review link to each Bulk Email', async () => {
    const w = await mountPage()
    const section = w.find('[aria-labelledby="attention-title"]')
    expect(section.text()).toContain('refunds_oct27.xlsx')
    expect(section.text()).toContain('12 failed emails')
    expect(section.text()).toContain('stock_alerts.xlsx')
    expect(section.text()).toContain('42% of rows were invalid')
    expect(section.findAll('a')[0]!.attributes('href')).toBe('/bulk-emails/42')
  })

  it('shows a calm message when nothing needs attention', async () => {
    api.getStats.mockResolvedValue(stats({ needs_attention: [], today: { sent: 10, failed: 0 } }))
    const w = await mountPage()
    expect(w.find('[aria-labelledby="attention-title"]').text()).toContain(
      'Nothing needs attention',
    )
    expect(tileValue(w, 'Failed today')).toBe('0')
    expect(w.text()).toContain('all good')
  })

  it('shows the latest five Bulk Emails, asking for no more', async () => {
    const w = await mountPage()
    expect(api.listBatches).toHaveBeenCalledWith({ status: '', q: '', limit: 5, offset: 0 })
    expect(w.find('[aria-labelledby="recent-title"]').text()).toContain('orders_oct28.xlsx')
    expect(w.find('[aria-labelledby="recent-title"]').text()).toContain('payouts_nov.xlsx')
  })

  it('welcomes a brand new install: empty chart and empty list, each with a way to start', async () => {
    api.getStats.mockResolvedValue(
      stats({
        today: { sent: 0, failed: 0 },
        queue: { pending: 0, retrying: 0, processing: 0 },
        failed_total: 0,
        active_batches: 0,
        needs_attention: [],
        daily: daily(14).map((d) => ({ ...d, sent: 0, failed: 0 })),
      }),
    )
    api.listBatches.mockResolvedValue({ items: [], total: 0, limit: 5, offset: 0 })
    const w = await mountPage()
    expect(w.text()).toContain('No emails sent in the last 14 days')
    expect(w.text()).toContain('No Bulk Emails yet')
    expect(w.findAll('a').filter((a) => a.text().includes('New Bulk Email'))).toHaveLength(2)
    expect(w.text()).toContain('nothing waiting')
    expect(w.text()).toContain('nothing sending')
    expect(w.find('[data-chart="bar"]').exists()).toBe(false)
  })

  it("shows the server's message and a way to try again if the numbers cannot load", async () => {
    api.getStats.mockRejectedValue(new ApiError('database is down', 500, 'http'))
    const w = await mountPage()
    await vi.waitFor(() => expect(w.find('[role="alert"]').exists()).toBe(true), { timeout: 4000 })
    expect(w.find('[role="alert"]').text()).toContain('database is down')
    api.getStats.mockResolvedValue(stats())
    await w
      .findAll('button')
      .find((b) => b.text() === 'Try again')!
      .trigger('click')
    await flushPromises()
    expect(tileValue(w, 'Sent today')).toBe('1,243')
  })
})
