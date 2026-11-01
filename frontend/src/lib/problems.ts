import type { ParseResult, RowProblem } from '@/types/api'

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

/** "312 rows checked. 296 are ready to send, 16 will be skipped." */
export function summarizeCheck(result: ParseResult): string {
  const valid = result.valid.length
  const skipped = result.problems.length
  const rows = valid + skipped
  const ready =
    valid === 1 ? '1 is ready to send' : `${valid.toLocaleString('en')} are ready to send`
  const left =
    skipped === 1 ? '1 will be skipped' : `${skipped.toLocaleString('en')} will be skipped`
  return `${rows.toLocaleString('en')} ${rows === 1 ? 'row' : 'rows'} checked. ${ready}, ${left}.`
}
