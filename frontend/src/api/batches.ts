import { http } from './client'
import type {
  Batch,
  BatchStatus,
  BatchType,
  CancelResult,
  CursorPage,
  Email,
  InvalidReason,
  JobStatus,
  Paged,
  Recipient,
  RetryResult,
  UploadResult,
} from '@/types/api'

/**
 * Uploading only streams the file to the server (the import runs in the background afterwards),
 * but a large file on a slow connection still takes a while.
 */
const UPLOAD_TIMEOUT_MS = 300_000

/** Empty filters are left out of the query string, as the API expects. */
function clean<T extends object>(params: T): Partial<T> {
  return Object.fromEntries(
    Object.entries(params).filter(([, v]) => v !== '' && v !== undefined),
  ) as Partial<T>
}

/**
 * Upload a spreadsheet. `verification` (Verify Emails) or `bulk_send` (New Bulk Email). The server
 * answers at once; poll the batch for the import progress.
 */
export async function uploadSpreadsheet(
  type: BatchType,
  file: File,
  onProgress?: (fraction: number) => void,
): Promise<UploadResult> {
  const form = new FormData()
  form.append('file', file)
  const url = type === 'verification' ? '/api/v1/verify' : '/api/v1/batches'
  const { data } = await http.post<UploadResult>(url, form, {
    timeout: UPLOAD_TIMEOUT_MS,
    onUploadProgress: (e) => {
      if (onProgress && e.total) onProgress(e.loaded / e.total)
    },
  })
  return data
}

export interface BatchListParams {
  type: BatchType | ''
  status: BatchStatus | ''
  q: string
  limit: number
  offset: number
}

export async function listBatches(params: BatchListParams): Promise<Paged<Batch>> {
  const { data } = await http.get<Paged<Batch>>('/api/v1/batches', {
    params: clean(params),
  })
  return data
}

export async function getBatch(id: number): Promise<Batch> {
  const { data } = await http.get<Batch>(`/api/v1/batches/${id}`)
  return data
}

export interface IssueParams {
  /** `invalid` (default): rows that will not be sent; `unverified`: sent, domain not checked. */
  status?: 'invalid' | 'unverified'
  /** Only invalid rows with one of these reasons. */
  reasons?: InvalidReason[]
  q?: string
  after?: number
  limit?: number
}

/** One page of a batch's problem rows, in spreadsheet order. */
export async function listIssues(id: number, params: IssueParams = {}): Promise<CursorPage<Recipient>> {
  const { status, reasons, q, after, limit } = params
  const { data } = await http.get<CursorPage<Recipient>>(`/api/v1/batches/${id}/issues`, {
    params: {
      status: status === 'unverified' ? status : undefined,
      reason: reasons?.length ? reasons.join(',') : undefined,
      q: q || undefined,
      after: after || undefined,
      limit,
    },
  })
  return data
}

/** The first few rows that will be sent, for the email preview. */
export async function getSample(id: number, limit = 5): Promise<Recipient[]> {
  const { data } = await http.get<{ items: Recipient[] }>(`/api/v1/batches/${id}/sample`, {
    params: { limit },
  })
  return data.items
}

/** The explicit "Send": saves the message and starts creating the emails. */
export async function sendBatch(id: number, message: { subject: string; body: string }): Promise<Batch> {
  const { data } = await http.post<Batch>(`/api/v1/batches/${id}/send`, message)
  return data
}

export interface EmailParams {
  status: JobStatus | ''
  q: string
  after?: number
  limit: number
}

/** One page of a batch's emails, oldest first. */
export async function listEmails(id: number, params: EmailParams): Promise<CursorPage<Email>> {
  const { data } = await http.get<CursorPage<Email>>(`/api/v1/batches/${id}/emails`, {
    params: clean({ ...params, after: params.after || undefined }),
  })
  return data
}

export async function retryBatch(id: number): Promise<RetryResult> {
  const { data } = await http.post<RetryResult>(`/api/v1/batches/${id}/retry`)
  return data
}

/** Stop a batch that is still sending: its unsent emails are cancelled for good. */
export async function cancelBatch(id: number): Promise<CancelResult> {
  const { data } = await http.post<CancelResult>(`/api/v1/batches/${id}/cancel`)
  return data
}

export async function retryJob(id: number): Promise<RetryResult> {
  const { data } = await http.post<RetryResult>(`/api/v1/jobs/${id}/retry`)
  return data
}
