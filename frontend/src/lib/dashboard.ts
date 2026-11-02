import { formatNumber, percent } from './format'
import type { AttentionBatch, Stats } from '@/types/api'

type Queue = Stats['queue']

export const RANGES = [7, 14, 30] as const
export type Range = (typeof RANGES)[number]

/** The browser's time zone, so "today" on the Dashboard is the user's today. Falls back to UTC. */
export function timeZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  } catch {
    return 'UTC'
  }
}

/** "Monday, 2 November" in the given time zone. */
export function todayLabel(now: Date, tz: string): string {
  return new Intl.DateTimeFormat('en-GB', {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    timeZone: tz,
  }).format(now)
}

/** "2026-11-02" read as a calendar day (not shifted by the viewer's time zone). */
function dayParts(isoDate: string): Date {
  const [y, m, d] = isoDate.split('-').map(Number)
  return new Date(Date.UTC(y ?? 1970, (m ?? 1) - 1, d ?? 1))
}

/** "2 Nov" for tooltips and the table. */
export function dayLabel(isoDate: string): string {
  return new Intl.DateTimeFormat('en-GB', {
    day: 'numeric',
    month: 'short',
    timeZone: 'UTC',
  }).format(dayParts(isoDate))
}

/** The axis label: just the day of the month. */
export function dayOfMonth(isoDate: string): string {
  return String(dayParts(isoDate).getUTCDate())
}

/** "5 sending now · 3 retrying", or a calm line when nothing is waiting. */
export function queueCaption(queue: Queue): string {
  const waiting = queue.pending + queue.processing
  if (waiting === 0) return 'nothing waiting'
  const parts = [`${formatNumber(queue.processing)} sending now`]
  if (queue.retrying > 0) parts.push(`${formatNumber(queue.retrying)} retrying`)
  return parts.join(' · ')
}

export const inQueue = (queue: Queue) => queue.pending + queue.processing

/** Why a Bulk Email is on the "Needs attention" list, in words. */
export function attentionReasons(item: AttentionBatch): string[] {
  const reasons: string[] = []
  if (item.reasons.includes('failed_jobs')) {
    reasons.push(`${formatNumber(item.failed)} failed ${item.failed === 1 ? 'email' : 'emails'}`)
  }
  if (item.reasons.includes('high_invalid_rate')) {
    reasons.push(`${percent(item.invalid, item.total)}% of rows were invalid`)
  }
  return reasons
}
