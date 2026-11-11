/**
 * Hand-written mirrors of the Go API's JSON (there is no OpenAPI spec). Source of truth:
 * the README API table and the `json` tags in internal/store, internal/api and internal/settings.
 * Only endpoints that exist are typed here.
 */

/** email_jobs.status: one email to deliver. */
export type JobStatus = 'pending' | 'processing' | 'sent' | 'failed' | 'cancelled'

/** email_batches.status. See docs/features/four-table-overhaul.md for the lifecycle. */
export type BatchStatus =
  | 'importing'
  | 'import_failed'
  | 'draft'
  | 'generating_jobs'
  | 'queued'
  | 'sending'
  | 'completed'
  | 'completed_with_errors'
  | 'failed'
  | 'cancelled'

/** Which page created the batch. A verification batch becomes `bulk_send` when it is sent. */
export type BatchType = 'verification' | 'bulk_send'

/** Why a row will not be sent (email_recipients.validation_reason). */
export type InvalidReason = 'empty_email' | 'empty_name' | 'invalid_email' | 'bad_domain'

/** Standard error body: `{"error": "message"}`. */
export interface ApiErrorBody {
  error: string
}

/** Page-numbered list (only GET /batches). */
export interface Paged<T> {
  items: T[]
  total: number
  limit: number
  offset: number
}

/** Cursor-paged list: send `next_after` as `after` for the next page; `null` = last page. */
export interface CursorPage<T> {
  items: T[]
  next_after: number | null
}

/** GET /healthz */
export interface Health {
  status: string
}

/**
 * GET /verify/{id}, GET /batches/{id} and the rows of GET /batches: one uploaded file with its
 * counters. List rows leave `body_template` out.
 */
export interface Batch {
  id: number
  type: BatchType
  filename: string
  status: BatchStatus
  subject_template: string
  body_template?: string

  /** Non-empty data rows saved so far (the final count once the import is done). */
  total_rows: number
  /** Spreadsheet rows read so far, blank rows included. */
  processed_rows: number
  valid_count: number
  /** Sent too, but the domain could not be checked. */
  unverified_count: number
  invalid_count: number
  invalid_counts: Record<InvalidReason, number>
  import_error: string | null

  pending_count: number
  processing_count: number
  sent_count: number
  failed_count: number
  cancelled_count: number

  job_generation_status: 'pending' | 'processing' | 'completed' | 'failed' | null
  job_generation_error: string | null

  created_at: string
  updated_at: string
  send_requested_at: string | null
  completed_at: string | null
  cancelled_at: string | null
}

/** POST /verify and POST /batches (202): the import continues in the background. */
export interface UploadResult {
  batch_id: number
  type: BatchType
  filename: string
  status: BatchStatus
}

/** One saved spreadsheet row, from GET .../issues and GET /batches/{id}/sample. */
export interface Recipient {
  id: number
  row: number
  name: string
  email: string
  validation_status: 'valid' | 'unverified' | 'invalid'
  validation_reason: InvalidReason | null
}

/** One email of GET /batches/{id}/emails, with subject and body built from the templates. */
export interface Email {
  id: number
  batch_id: number
  recipient_id: number
  row: number
  name: string
  email: string
  status: JobStatus
  attempts: number
  max_attempts: number
  last_error: string | null
  available_at: string
  sent_at: string | null
  created_at: string
  updated_at: string
  subject: string
  body: string
}

/** POST /batches/{id}/cancel */
export interface CancelResult {
  cancelled: number
}

/** POST /batches/{id}/retry and POST /jobs/{id}/retry */
export interface RetryResult {
  retried: number
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
