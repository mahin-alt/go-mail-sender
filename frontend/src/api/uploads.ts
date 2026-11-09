import { http } from './client'
import type { ProblemPage, RowKind, UploadRow, UploadSummary } from '@/types/api'

/** Checking a big file reads every row (and may look up every domain), which can take a while. */
const UPLOAD_TIMEOUT_MS = 120_000

/** Check a spreadsheet and keep the result on the server. Returns the totals only. */
export async function createUpload(file: File): Promise<UploadSummary> {
  const form = new FormData()
  form.append('file', file)
  const { data } = await http.post<UploadSummary>('/api/v1/uploads', form, {
    timeout: UPLOAD_TIMEOUT_MS,
  })
  return data
}

export interface ProblemParams {
  /** Only these kinds; empty means every skipped kind. */
  kinds?: RowKind[]
  q?: string
  /** Rows after this spreadsheet row (the previous page's `next_after`). */
  after?: number
  limit?: number
}

/** One page of a checked file's problem rows, in spreadsheet order. */
export async function listUploadProblems(id: number, params: ProblemParams = {}): Promise<ProblemPage> {
  const { kinds, q, after, limit } = params
  const { data } = await http.get<ProblemPage>(`/api/v1/uploads/${id}/problems`, {
    params: {
      kind: kinds?.length ? kinds.join(',') : undefined,
      q: q || undefined,
      after: after || undefined,
      limit,
    },
  })
  return data
}

/** The first few rows that will be sent, for the email preview. */
export async function getUploadSample(id: number, limit = 5): Promise<UploadRow[]> {
  const { data } = await http.get<{ items: UploadRow[] }>(`/api/v1/uploads/${id}/sample`, {
    params: { limit },
  })
  return data.items
}
