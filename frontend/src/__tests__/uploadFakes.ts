import { vi } from 'vitest'

import type * as BatchesApi from '@/api/batches'
import type { IssueParams } from '@/api/batches'
import type { Batch, BatchType, CursorPage, InvalidReason, Recipient } from '@/types/api'

/** A spreadsheet row as the server stores it (id = row number, which keeps tests readable). */
export interface FakeRow {
  row: number
  name: string
  email: string
  status: 'valid' | 'unverified' | 'invalid'
  reason?: InvalidReason
}

const REASONS: InvalidReason[] = ['empty_email', 'empty_name', 'invalid_email', 'bad_domain']

const toRecipient = (r: FakeRow): Recipient => ({
  id: r.row,
  row: r.row,
  name: r.name,
  email: r.email,
  validation_status: r.status,
  validation_reason: r.reason ?? null,
})

/** A batch as the server would hold it after importing these rows. */
export function fakeBatch(
  rows: FakeRow[],
  { id = 7, filename = 'orders.xlsx', type = 'bulk_send' as BatchType } = {},
): Batch {
  const counts = Object.fromEntries(REASONS.map((k) => [k, 0])) as Record<InvalidReason, number>
  for (const r of rows) if (r.reason) counts[r.reason]++
  return {
    id,
    type,
    filename,
    status: type === 'verification' ? 'completed' : 'draft',
    subject_template: '',
    total_rows: rows.length,
    processed_rows: rows.length + 1,
    valid_count: rows.filter((r) => r.status === 'valid').length,
    unverified_count: rows.filter((r) => r.status === 'unverified').length,
    invalid_count: rows.filter((r) => r.status === 'invalid').length,
    invalid_counts: counts,
    import_error: null,
    pending_count: 0,
    processing_count: 0,
    sent_count: 0,
    failed_count: 0,
    cancelled_count: 0,
    job_generation_status: null,
    job_generation_error: null,
    created_at: '2026-11-08T10:00:00Z',
    updated_at: '2026-11-08T10:00:00Z',
    send_requested_at: null,
    completed_at: null,
    cancelled_at: null,
  }
}

/** Behaves like GET /batches/{id}/issues: filter by status and reasons, then page with after/limit. */
export function fakeIssues(rows: FakeRow[]) {
  return (_id: number, params: IssueParams = {}): Promise<CursorPage<Recipient>> => {
    const status = params.status ?? 'invalid'
    const after = params.after ?? 0
    const limit = params.limit ?? 100
    const matching = rows.filter(
      (r) =>
        r.status === status &&
        (!params.reasons?.length || (r.reason && params.reasons.includes(r.reason))) &&
        r.row > after,
    )
    const items = matching.slice(0, limit).map(toRecipient)
    const more = matching.length > limit
    return Promise.resolve({ items, next_after: more ? items[items.length - 1]!.id : null })
  }
}

/** Behaves like GET /batches/{id}/sample. */
export function fakeSample(rows: FakeRow[]) {
  return (_id: number, limit = 5): Promise<Recipient[]> =>
    Promise.resolve(rows.filter((r) => r.status !== 'invalid').slice(0, limit).map(toRecipient))
}

type Api = typeof BatchesApi

/** The mocked '@/api/batches' module; tests pass this to vi.mock. */
export function batchesApiMock() {
  return {
    uploadSpreadsheet: vi.fn<Api['uploadSpreadsheet']>(),
    getBatch: vi.fn<Api['getBatch']>(),
    listIssues: vi.fn<Api['listIssues']>(),
    getSample: vi.fn<Api['getSample']>(),
    sendBatch: vi.fn<Api['sendBatch']>(),
    listBatches: vi.fn<Api['listBatches']>(),
    listEmails: vi.fn<Api['listEmails']>(),
    retryBatch: vi.fn<Api['retryBatch']>(),
    retryJob: vi.fn<Api['retryJob']>(),
    cancelBatch: vi.fn<Api['cancelBatch']>(),
  }
}

/**
 * Make the server behave as if every uploaded file held these rows: the upload is accepted
 * (importing), and the batch reads as imported.
 */
export function serverHas(
  api: ReturnType<typeof batchesApiMock>,
  rows: FakeRow[],
  opts: { type?: BatchType; filename?: string } = {},
) {
  const batch = fakeBatch(rows, opts)
  api.uploadSpreadsheet.mockImplementation((type, file) =>
    Promise.resolve({ batch_id: batch.id, type, filename: file.name, status: 'importing' as const }),
  )
  api.getBatch.mockImplementation(() => Promise.resolve({ ...batch, type: opts.type ?? batch.type }))
  api.listIssues.mockImplementation(fakeIssues(rows))
  api.getSample.mockImplementation(fakeSample(rows))
  return batch
}

/** The usual test file: two valid rows, a repeated address, two invalid rows and one unverified row. */
export const typicalRows: FakeRow[] = [
  { row: 2, name: 'Aisha', email: 'aisha@shop.example', status: 'valid' },
  { row: 3, name: 'রহিম', email: 'rahim@shop.example', status: 'valid' },
  { row: 4, name: 'Ben', email: 'ben@@x', status: 'invalid', reason: 'invalid_email' },
  { row: 5, name: 'Aisha', email: 'aisha@shop.example', status: 'valid' }, // duplicates are sent
  { row: 6, name: 'Kabir', email: 'k@nope.example', status: 'invalid', reason: 'bad_domain' },
  { row: 7, name: 'Lina', email: 'lina@slow.example', status: 'unverified' },
]
