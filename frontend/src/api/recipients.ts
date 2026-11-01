import { http } from './client'
import type { CreateBatchResult, ParseResult } from '@/types/api'

/** Checking a big file looks up every recipient domain, which can take a while. */
const UPLOAD_TIMEOUT_MS = 120_000

/** Check a spreadsheet without saving or sending anything. */
export async function parseRecipients(file: File): Promise<ParseResult> {
  const form = new FormData()
  form.append('file', file)
  const { data } = await http.post<ParseResult>('/api/v1/recipients/parse', form, {
    timeout: UPLOAD_TIMEOUT_MS,
  })
  return data
}

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
