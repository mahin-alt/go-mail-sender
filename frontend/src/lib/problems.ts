import type { RowKind, UploadSummary } from '@/types/api'

/** The tabs of the review table. */
export type ProblemTab = 'all' | 'invalid' | 'duplicate' | 'domain' | 'unverified'

/** Which row kinds each tab lists. `all` is empty: the server then lists every skipped kind. */
export const TAB_KINDS: Record<ProblemTab, RowKind[]> = {
  all: [],
  invalid: ['empty_email', 'empty_name', 'invalid_email'],
  duplicate: ['duplicate'],
  domain: ['bad_domain'],
  unverified: ['unverified'],
}

/** How many rows a tab holds, from the totals the server worked out once. */
export function tabCount(tab: ProblemTab, summary: UploadSummary): number {
  if (tab === 'all') return summary.problem_count
  if (tab === 'unverified') return summary.unverified_count
  return TAB_KINDS[tab].reduce(
    (n, kind) => n + (summary.problem_counts[kind as keyof UploadSummary['problem_counts']] ?? 0),
    0,
  )
}

/**
 * The API's reasons name the email address (`invalid email "x@y"`), which is already in its own
 * column, and give DNS detail. Show only the part that explains the problem.
 */
export function tidyReason(reason: string): string {
  const text = reason.trim()
  const lower = text.toLowerCase()
  if (lower.startsWith('duplicate')) return 'duplicate email'
  if (lower.startsWith('invalid email')) return 'invalid email'
  if (lower.startsWith('could not verify domain')) return 'could not verify domain'
  if (lower.startsWith('domain ')) return 'domain does not accept mail'
  return text
}

/** The error text for an email: rows that were skipped as invalid get the short reason, delivery errors stay as the server gave them. */
export function jobReason(job: { status: string; last_error?: string }): string {
  const text = job.last_error ?? ''
  return job.status === 'invalid' ? tidyReason(text) : text
}
