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
