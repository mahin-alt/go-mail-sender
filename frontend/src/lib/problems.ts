import type { RowProblem } from '@/types/api'

export type ProblemKind = 'invalid' | 'duplicate' | 'domain'

/**
 * The API gives each skipped row a plain-text reason, not a code. These start with fixed words
 * (see internal/recipients and internal/domaincheck), so the kind is read from the start.
 */
export function problemKind(problem: Pick<RowProblem, 'reason'>): ProblemKind {
  const reason = problem.reason.toLowerCase()
  if (reason.startsWith('duplicate')) return 'duplicate'
  if (reason.startsWith('domain ')) return 'domain'
  return 'invalid'
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
