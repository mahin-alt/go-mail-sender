/**
 * Hand-written mirrors of the Go API's JSON (there is no OpenAPI spec). Source of truth:
 * the README API table and the `json` tags in internal/store, internal/api and internal/settings.
 * Only endpoints that exist are typed here.
 */

export type JobStatus = 'pending' | 'processing' | 'sent' | 'failed' | 'invalid'
export type BatchStatus = 'pending' | 'processing' | 'completed' | 'failed'

/** Standard error body: `{"error": "message"}`. */
export interface ApiErrorBody {
  error: string
}

export interface Paged<T> {
  items: T[]
  total: number
  limit: number
  offset: number
}

/** GET /healthz */
export interface Health {
  status: string
}

/** Row of GET /batches. */
export interface BatchSummary {
  id: number
  filename: string
  status: BatchStatus
  total: number
  sent: number
  failed: number
  pending: number
  processing: number
  invalid: number
  created_at: string
  completed_at?: string
}

/** GET /batches/{id} */
export interface BatchProgress {
  batch_id: number
  filename: string
  status: BatchStatus
  total: number
  sent: number
  failed: number
  pending: number
  processing: number
  invalid: number
}

/** Row of GET /batches/{id}/jobs (paged form). */
export interface Job {
  id: number
  batch_id: number
  recipient_name: string
  recipient_email: string
  subject: string
  body: string
  status: JobStatus
  attempts: number
  max_attempts: number
  last_error?: string
  locked_at?: string
  run_at: string
  created_at: string
  updated_at: string
  sent_at?: string
}

/** POST /batches/{id}/retry and POST /jobs/{id}/retry */
export interface RetryResult {
  retried: number
}

/** One spreadsheet row that was rejected, from POST /recipients/parse. */
export interface RowProblem {
  row: number
  name?: string
  email?: string
  reason: string
}

export interface Recipient {
  name: string
  email: string
  row: number
}

/** POST /recipients/parse */
export interface ParseResult {
  valid: Recipient[]
  problems: RowProblem[]
  unverified_count: number
  unverified?: RowProblem[]
}

/** POST /batches (202) */
export interface CreateBatchResult {
  batch_id: number
  filename: string
  total_rows: number
  valid_count: number
  invalid_count: number
  status: string
  problems?: RowProblem[]
  unverified_count: number
  unverified?: RowProblem[]
  message: string
}

/** GET /stats */
export interface Stats {
  generated_at: string
  timezone: string
  today: { sent: number; failed: number }
  queue: { pending: number; retrying: number; processing: number }
  failed_total: number
  active_batches: number
  daily: { date: string; sent: number; failed: number }[]
  needs_attention: AttentionBatch[]
}

export interface AttentionBatch {
  batch_id: number
  filename: string
  status: BatchStatus
  total: number
  failed: number
  invalid: number
  created_at: string
  reasons: ('failed_jobs' | 'high_invalid_rate')[]
}

/** Row of GET /settings. Secret values are never returned (`value` is empty, see `has_value`). */
export interface SettingItem {
  key: string
  description: string
  kind: string
  value: string
  has_value: boolean
  source: 'env' | 'override'
  editable: boolean
  secret: boolean
  apply: 'instant' | 'restart' | 'readonly'
  options?: string[]
  restart_required: boolean
  updated_at?: string
}
