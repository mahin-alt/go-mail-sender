import { describe, expect, it } from 'vitest'

import { formatNumber, percent, timeAgo } from '@/lib/format'

const NOW = new Date('2026-10-29T12:00:00Z').getTime()
const ago = (ms: number) => new Date(NOW - ms).toISOString()

describe('timeAgo', () => {
  it('says just now for the last minute', () => {
    expect(timeAgo(ago(10_000), NOW)).toBe('just now')
  })
  it('uses minutes, hours and days', () => {
    expect(timeAgo(ago(12 * 60_000), NOW)).toBe('12 minutes ago')
    expect(timeAgo(ago(3 * 3_600_000), NOW)).toBe('3 hours ago')
    expect(timeAgo(ago(26 * 3_600_000), NOW)).toBe('yesterday')
  })
  it('switches to a date after a week', () => {
    expect(timeAgo(ago(10 * 86_400_000), NOW)).toBe('Oct 19, 2026')
  })
  it('copes with missing or broken values', () => {
    expect(timeAgo(undefined, NOW)).toBe('-')
    expect(timeAgo('not a date', NOW)).toBe('-')
  })
})

describe('numbers', () => {
  it('groups thousands', () => expect(formatNumber(1000)).toBe('1,000'))
  it('rounds percentages and survives a zero total', () => {
    expect(percent(300, 312)).toBe(96)
    expect(percent(0, 0)).toBe(0)
  })
})
