import { http } from './client'
import type { CreateBatchResult } from '@/types/api'

/** Sending a big file re-reads and saves every row, which can take a while. */
const UPLOAD_TIMEOUT_MS = 120_000

export interface NewBulkEmail {
  file: File
  subject: string
  body: string
}

/** Save the batch and queue every valid row for sending. */
export async function createBatch(input: NewBulkEmail): Promise<CreateBatchResult> {
  const form = new FormData()
  form.append('file', input.file)
  form.append('subject', input.subject)
  form.append('body', input.body)
  const { data } = await http.post<CreateBatchResult>('/api/v1/batches', form, {
    timeout: UPLOAD_TIMEOUT_MS,
  })
  return data
}
