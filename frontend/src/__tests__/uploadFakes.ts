import type { ProblemParams } from '@/api/uploads'
import type { ProblemPage, RowKind, SkippedKind, UploadRow, UploadSummary } from '@/types/api'

const SKIPPED: SkippedKind[] = ['empty_email', 'empty_name', 'invalid_email', 'duplicate', 'bad_domain']

/** A checked file as the server would keep it: its rows, and the totals worked out from them. */
export function fakeUpload(rows: UploadRow[], id = 7, filename = 'orders.xlsx') {
  const counts = Object.fromEntries(SKIPPED.map((k) => [k, 0])) as Record<SkippedKind, number>
  for (const r of rows) if (SKIPPED.includes(r.kind as SkippedKind)) counts[r.kind as SkippedKind]++
  const summary: UploadSummary = {
    upload_id: id,
    filename,
    status: 'ready',
    total_rows: rows.length,
    valid_count: rows.filter((r) => r.kind === 'ok' || r.kind === 'unverified').length,
    problem_count: rows.filter((r) => SKIPPED.includes(r.kind as SkippedKind)).length,
    unverified_count: rows.filter((r) => r.kind === 'unverified').length,
    problem_counts: counts,
    created_at: '2026-11-08T10:00:00Z',
    expires_at: '2026-11-09T10:00:00Z',
  }

  /** Behaves like GET /uploads/{id}/problems: filter by kinds, then page with after/limit. */
  function problems(_id: number, params: ProblemParams = {}): Promise<ProblemPage> {
    const kinds: RowKind[] = params.kinds?.length ? params.kinds : SKIPPED
    const after = params.after ?? 0
    const limit = params.limit ?? 100
    const matching = rows.filter((r) => kinds.includes(r.kind) && r.row > after)
    const items = matching.slice(0, limit)
    const more = matching.length > limit
    return Promise.resolve({ items, next_after: more ? items[items.length - 1]!.row : null })
  }

  /** Behaves like GET /uploads/{id}/sample. */
  function sample(_id: number, limit = 5): Promise<UploadRow[]> {
    return Promise.resolve(
      rows.filter((r) => r.kind === 'ok' || r.kind === 'unverified').slice(0, limit),
    )
  }

  return { summary, problems, sample }
}

/** The usual test file: two valid rows, three skipped rows and one unverified row. */
export const typicalRows: UploadRow[] = [
  { row: 2, name: 'Aisha', email: 'aisha@shop.example', kind: 'ok' },
  { row: 3, name: 'রহিম', email: 'rahim@shop.example', kind: 'ok' },
  { row: 4, name: 'Ben', email: 'ben@@x', kind: 'invalid_email', reason: 'invalid email "ben@@x"' },
  {
    row: 5,
    name: 'Aisha',
    email: 'aisha@shop.example',
    kind: 'duplicate',
    reason: 'duplicate email "aisha@shop.example"',
  },
  {
    row: 6,
    name: 'Kabir',
    email: 'k@nope.example',
    kind: 'bad_domain',
    reason: 'domain nope.example does not exist or has no mail server',
  },
  {
    row: 7,
    name: 'Lina',
    email: 'lina@slow.example',
    kind: 'unverified',
    reason: 'could not verify domain slow.example: lookup timed out',
  },
]
