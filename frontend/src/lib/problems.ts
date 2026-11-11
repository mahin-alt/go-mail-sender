import type { Batch, InvalidReason } from '@/types/api'

/** The tabs of the review table. */
export type ProblemTab = 'all' | 'invalid' | 'domain' | 'unverified'

/** What each tab asks the issues endpoint for. `all` = every invalid row. */
export const TAB_FILTER: Record<
  ProblemTab,
  { status: 'invalid' | 'unverified'; reasons: InvalidReason[] }
> = {
  all: { status: 'invalid', reasons: [] },
  invalid: { status: 'invalid', reasons: ['empty_email', 'empty_name', 'invalid_email'] },
  domain: { status: 'invalid', reasons: ['bad_domain'] },
  unverified: { status: 'unverified', reasons: [] },
}

/** How many rows a tab holds, from the counters on the batch. */
export function tabCount(tab: ProblemTab, batch: Batch): number {
  if (tab === 'all') return batch.invalid_count
  if (tab === 'unverified') return batch.unverified_count
  return TAB_FILTER[tab].reasons.reduce((n, reason) => n + (batch.invalid_counts[reason] ?? 0), 0)
}

const REASON_TEXT: Record<InvalidReason, string> = {
  empty_email: 'email is empty',
  empty_name: 'name is empty',
  invalid_email: 'invalid email',
  bad_domain: 'domain does not accept mail',
}

/** The short reason shown for a problem row. Unverified rows have no reason code. */
export function reasonText(row: { validation_reason: InvalidReason | null }): string {
  return row.validation_reason ? REASON_TEXT[row.validation_reason] : 'could not verify domain'
}
