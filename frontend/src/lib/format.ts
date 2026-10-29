const relative = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })
const shortDate = new Intl.DateTimeFormat('en', { day: 'numeric', month: 'short', year: 'numeric' })
const fullDate = new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' })

/** "just now", "12 minutes ago", "yesterday", or a date once it is older than a week. */
export function timeAgo(iso: string | undefined, now: number = Date.now()): string {
  if (!iso) return '-'
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return '-'
  const seconds = Math.round((then - now) / 1000)
  const abs = Math.abs(seconds)
  if (abs < 45) return 'just now'
  if (abs < 3600) return relative.format(Math.round(seconds / 60), 'minute')
  if (abs < 86_400) return relative.format(Math.round(seconds / 3600), 'hour')
  if (abs < 7 * 86_400) return relative.format(Math.round(seconds / 86_400), 'day')
  return shortDate.format(then)
}

/** Exact local time, for tooltips. */
export function exactTime(iso: string | undefined): string {
  if (!iso) return ''
  const t = new Date(iso)
  return Number.isNaN(t.getTime()) ? '' : fullDate.format(t)
}

export function formatNumber(n: number): string {
  return n.toLocaleString('en')
}

/** Rounded percentage of `part` in `total`, 0 when there is no total. */
export function percent(part: number, total: number): number {
  return total > 0 ? Math.round((part / total) * 100) : 0
}
