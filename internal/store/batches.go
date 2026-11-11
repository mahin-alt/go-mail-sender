package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Batch types: which page created the batch. A verification batch becomes a
// bulk_send batch when it is sent, so the Bulk Emails list shows everything
// that was (or is about to be) sent.
const (
	TypeVerification = "verification"
	TypeBulkSend     = "bulk_send"
)

// Batch statuses (email_batches.status). See the lifecycle in
// docs/features/four-table-overhaul.md.
const (
	StatusImporting           = "importing"
	StatusImportFailed        = "import_failed"
	StatusDraft               = "draft" // bulk_send: imported, waiting for Send
	StatusGeneratingJobs      = "generating_jobs"
	StatusQueued              = "queued" // every email job exists, none claimed yet
	StatusSending             = "sending"
	StatusCompleted           = "completed" // verification: imported; bulk_send: every email sent
	StatusCompletedWithErrors = "completed_with_errors"
	StatusFailed              = "failed" // the batch cannot continue (job generation gave up)
	StatusCancelled           = "cancelled"
)

// InvalidReasons are the codes of invalid rows, in the order the UI shows them.
var InvalidReasons = []string{"empty_email", "empty_name", "invalid_email", "bad_domain"}

// Batch is one uploaded file: its status, counters and templates.
type Batch struct {
	ID              int64  `json:"id"`
	Type            string `json:"type"`
	Filename        string `json:"filename"`
	Status          string `json:"status"`
	SubjectTemplate string `json:"subject_template"`
	BodyTemplate    string `json:"body_template,omitempty"` // left out of list responses

	TotalRows       int            `json:"total_rows"`
	ProcessedRows   int            `json:"processed_rows"`
	ValidCount      int            `json:"valid_count"`
	UnverifiedCount int            `json:"unverified_count"`
	InvalidCount    int            `json:"invalid_count"`
	InvalidCounts   map[string]int `json:"invalid_counts"`
	ImportError     *string        `json:"import_error"`

	PendingCount    int `json:"pending_count"`
	ProcessingCount int `json:"processing_count"`
	SentCount       int `json:"sent_count"`
	FailedCount     int `json:"failed_count"`
	CancelledCount  int `json:"cancelled_count"`

	JobGenerationStatus *string `json:"job_generation_status"`
	JobGenerationError  *string `json:"job_generation_error"`

	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	SendRequestedAt *time.Time `json:"send_requested_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	CancelledAt     *time.Time `json:"cancelled_at"`
}

// SendableCount is how many rows will get an email: valid + unverified.
func (b *Batch) SendableCount() int { return b.ValidCount + b.UnverifiedCount }

// batchColumns is the SELECT list scanBatch expects. bodyExpr lets the list
// leave the (possibly long) body out.
func batchColumns(bodyExpr string) string {
	return `id, type, original_filename, status, COALESCE(subject_template, ''), ` + bodyExpr + `,
		total_rows, processed_rows, valid_count, unverified_count, invalid_count, invalid_counts, import_error,
		pending_count, processing_count, sent_count, failed_count, cancelled_count,
		job_generation_status, job_generation_error,
		created_at, updated_at, send_requested_at, completed_at, cancelled_at`
}

type rowScanner interface{ Scan(dest ...any) error }

func scanBatch(r rowScanner) (*Batch, error) {
	var b Batch
	var counts []byte
	err := r.Scan(&b.ID, &b.Type, &b.Filename, &b.Status, &b.SubjectTemplate, &b.BodyTemplate,
		&b.TotalRows, &b.ProcessedRows, &b.ValidCount, &b.UnverifiedCount, &b.InvalidCount, &counts, &b.ImportError,
		&b.PendingCount, &b.ProcessingCount, &b.SentCount, &b.FailedCount, &b.CancelledCount,
		&b.JobGenerationStatus, &b.JobGenerationError,
		&b.CreatedAt, &b.UpdatedAt, &b.SendRequestedAt, &b.CompletedAt, &b.CancelledAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(counts, &b.InvalidCounts); err != nil {
		return nil, fmt.Errorf("decode invalid_counts: %w", err)
	}
	return &b, nil
}

// CreateBatch creates a batch in 'importing' state together with its import
// task, in one transaction. beforeCommit runs after both rows exist (with the
// new ID) and before the commit. The API uses it to move the uploaded
// file to its final name, so a committed batch always has its file, and a
// failed rename leaves no batch behind.
func (s *Store) CreateBatch(ctx context.Context, batchType, filename string, beforeCommit func(batchID int64) error) (int64, error) {
	if batchType != TypeVerification && batchType != TypeBulkSend {
		return 0, fmt.Errorf("unknown batch type %q", batchType)
	}
	var id int64
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO email_batches (type, original_filename) VALUES ($1, $2) RETURNING id`,
			batchType, filename).Scan(&id); err != nil {
			return fmt.Errorf("insert batch: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO email_import_jobs (batch_id) VALUES ($1)`, id); err != nil {
			return fmt.Errorf("insert import job: %w", err)
		}
		if beforeCommit != nil {
			return beforeCommit(id)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

// GetBatch returns one batch with its counters and templates.
func (s *Store) GetBatch(ctx context.Context, id int64) (*Batch, error) {
	b, err := scanBatch(s.db.QueryRowContext(ctx,
		`SELECT `+batchColumns(`COALESCE(body_template, '')`)+` FROM email_batches WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBatchNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get batch: %w", err)
	}
	return b, nil
}

// BatchExists reports whether a batch with that ID exists (orphan file cleanup).
func (s *Store) BatchExists(ctx context.Context, id int64) (bool, error) {
	var ok bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM email_batches WHERE id = $1)`, id).Scan(&ok)
	return ok, err
}

// BatchListFilter narrows and pages the batch list.
type BatchListFilter struct {
	Type   string // exact type, "" = any
	Status string // exact status, "" = any
	Search string // case-insensitive substring of the filename, "" = any
	Limit  int
	Offset int
}

// ListBatches returns one page of batches (newest first) plus how many match
// the filter. The list is small (one row per upload), so page numbers
// (offset) are fine here; the counters come straight from the batch rows.
func (s *Store) ListBatches(ctx context.Context, f BatchListFilter) ([]Batch, int, error) {
	const where = `
		WHERE ($1 = '' OR type = $1)
		  AND ($2 = '' OR status = $2)
		  AND ($3 = '' OR original_filename ILIKE '%' || $3 || '%')`

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM email_batches`+where,
		f.Type, f.Status, f.Search).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count batches: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `SELECT `+batchColumns(`''`)+` FROM email_batches`+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $4 OFFSET $5`, f.Type, f.Status, f.Search, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query batches: %w", err)
	}
	defer rows.Close()

	out := []Batch{} // non-nil so it encodes as [] not null
	for rows.Next() {
		b, err := scanBatch(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan batch: %w", err)
		}
		out = append(out, *b)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("batch rows: %w", err)
	}
	return out, total, nil
}

// peekBatchStatus reads a batch's status without locking it.
func peekBatchStatus(ctx context.Context, tx *sql.Tx, id int64) (string, error) {
	var status string
	err := tx.QueryRowContext(ctx, `SELECT status FROM email_batches WHERE id = $1`, id).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrBatchNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read batch status: %w", err)
	}
	return status, nil
}

// lockBatch reads a batch's status and type with a row lock, so concurrent
// send / cancel / retry requests on the same batch run one after another.
func lockBatch(ctx context.Context, tx *sql.Tx, id int64) (status, batchType string, sendable int, err error) {
	err = tx.QueryRowContext(ctx,
		`SELECT status, type, valid_count + unverified_count FROM email_batches WHERE id = $1 FOR UPDATE`, id).
		Scan(&status, &batchType, &sendable)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", 0, ErrBatchNotFound
	}
	if err != nil {
		return "", "", 0, fmt.Errorf("lock batch: %w", err)
	}
	return status, batchType, sendable, nil
}

// RequestSend is the user's explicit "Send": it saves the templates and hands
// the batch to the job generator. Allowed for a 'draft' bulk batch and for a
// 'completed' verification batch (verify first, send later), which becomes a
// bulk_send batch. A second request (double click) gets ErrNotSendable, so a
// batch can never start sending twice.
func (s *Store) RequestSend(ctx context.Context, id int64, subject, body string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		status, batchType, sendable, err := lockBatch(ctx, tx, id)
		if err != nil {
			return err
		}
		ok := status == StatusDraft || (batchType == TypeVerification && status == StatusCompleted)
		if !ok {
			return ErrNotSendable
		}
		if sendable == 0 {
			return ErrNothingToSend
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE email_batches
			SET type = 'bulk_send', status = 'generating_jobs',
			    subject_template = $2, body_template = $3,
			    job_generation_status = 'pending', last_generated_recipient_id = 0,
			    job_generation_attempts = 0, job_generation_error = NULL,
			    send_requested_at = NOW(), completed_at = NULL, updated_at = NOW()
			WHERE id = $1`, id, subject, body)
		if err != nil {
			return fmt.Errorf("request send: %w", err)
		}
		return nil
	})
}

// CancelBatch stops a batch that is generating or sending: job generation
// stops, every 'pending' email becomes 'cancelled' (never sent, never
// retried) and the batch becomes 'cancelled'. An email being sent right now
// finishes; if it fails it is cancelled instead of retried. Returns how many
// emails were cancelled, ErrBatchNotFound, or ErrBatchNotActive when there is
// nothing to stop.
func (s *Store) CancelBatch(ctx context.Context, id int64) (int, error) {
	var n int64
	active := func(status string) bool {
		return status == StatusGeneratingJobs || status == StatusQueued || status == StatusSending
	}
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		// Lock order matters: workers lock an email row, then its batch row. We
		// do the same (emails first, batch second), otherwise a worker and a
		// cancel could each wait for the other's lock (a deadlock).
		status, err := peekBatchStatus(ctx, tx, id)
		if err != nil {
			return err
		}
		if !active(status) {
			return ErrBatchNotActive
		}
		// A job a worker claims at this moment is row-locked by its claim; this
		// UPDATE waits for it, re-checks the status and skips it ('processing').
		res, err := tx.ExecContext(ctx, `
			UPDATE email_jobs
			SET status = 'cancelled', last_error = 'cancelled', locked_by = NULL, updated_at = NOW()
			WHERE batch_id = $1 AND status = 'pending'`, id)
		if err != nil {
			return fmt.Errorf("cancel jobs: %w", err)
		}
		if n, err = res.RowsAffected(); err != nil {
			return err
		}
		if status, _, _, err = lockBatch(ctx, tx, id); err != nil {
			return err
		}
		if !active(status) { // finished meanwhile: undo everything
			return ErrBatchNotActive
		}
		if status == StatusGeneratingJobs {
			// The generator may have committed a chunk of new emails while we
			// waited for the batch lock; cancel those too. No worker can be
			// holding them: emails of a generating batch are never claimed.
			res, err := tx.ExecContext(ctx, `
				UPDATE email_jobs
				SET status = 'cancelled', last_error = 'cancelled', updated_at = NOW()
				WHERE batch_id = $1 AND status = 'pending'`, id)
			if err != nil {
				return fmt.Errorf("cancel new jobs: %w", err)
			}
			more, _ := res.RowsAffected()
			n += more
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE email_batches
			SET status = 'cancelled', cancelled_at = NOW(), updated_at = NOW(),
			    pending_count = pending_count - $2, cancelled_count = cancelled_count + $2,
			    job_generation_locked_by = NULL,
			    job_generation_status = CASE WHEN job_generation_status IN ('pending', 'processing')
			                                 THEN 'failed' ELSE job_generation_status END,
			    job_generation_error = CASE WHEN job_generation_status IN ('pending', 'processing')
			                                THEN 'cancelled' ELSE job_generation_error END
			WHERE id = $1`, id, n)
		if err != nil {
			return fmt.Errorf("cancel batch: %w", err)
		}
		return nil
	})
	return int(n), err
}

// RetryFailedJobs puts failed emails back in the queue with a fresh set of
// attempts and returns how many were requeued. With jobID == nil every failed
// email of the batch is retried, otherwise only that one (which must belong
// to the batch). The same job rows are reused, so "one email per recipient"
// still holds. A finished batch goes back to 'sending'.
func (s *Store) RetryFailedJobs(ctx context.Context, batchID int64, jobID *int64) (int, error) {
	var n int64
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		// Emails first, batch second: the same lock order as the workers.
		status, err := peekBatchStatus(ctx, tx, batchID)
		if err != nil {
			return err
		}
		if status == StatusCancelled {
			return ErrBatchNotActive // a cancelled batch stays stopped
		}
		res, err := tx.ExecContext(ctx, `
			UPDATE email_jobs
			SET status = 'pending', attempts = 0, available_at = NOW(),
			    locked_at = NULL, locked_by = NULL, updated_at = NOW()
			WHERE batch_id = $1 AND status = 'failed'
			  AND ($2::BIGINT IS NULL OR id = $2)`, batchID, jobID)
		if err != nil {
			return fmt.Errorf("requeue failed jobs: %w", err)
		}
		if n, err = res.RowsAffected(); err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		if status, _, _, err = lockBatch(ctx, tx, batchID); err != nil {
			return err
		}
		if status == StatusCancelled { // cancelled meanwhile: undo
			return ErrBatchNotActive
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE email_batches
			SET failed_count = failed_count - $2, pending_count = pending_count + $2,
			    status = CASE WHEN status IN ('completed', 'completed_with_errors') THEN 'sending' ELSE status END,
			    completed_at = CASE WHEN status IN ('completed', 'completed_with_errors') THEN NULL ELSE completed_at END,
			    updated_at = NOW()
			WHERE id = $1`, batchID, n)
		if err != nil {
			return fmt.Errorf("reopen batch: %w", err)
		}
		return nil
	})
	return int(n), err
}

// RetryFailedJob retries one failed email. ErrJobNotRetryable if it does not
// exist or is not 'failed'.
func (s *Store) RetryFailedJob(ctx context.Context, jobID int64) (int, error) {
	var batchID int64
	err := s.db.QueryRowContext(ctx,
		`SELECT batch_id FROM email_jobs WHERE id = $1 AND status = 'failed'`, jobID).Scan(&batchID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrJobNotRetryable
	}
	if err != nil {
		return 0, fmt.Errorf("find failed job: %w", err)
	}
	n, err := s.RetryFailedJobs(ctx, batchID, &jobID)
	if err == nil && n == 0 {
		return 0, ErrJobNotRetryable // someone else retried it first
	}
	return n, err
}

// ReconcileBatchCounters recounts a batch's counters from its recipient and
// email rows and saves them, reporting whether anything had drifted. The
// counters are kept in step transactionally, so this is a safety net (and a
// helper for tests and maintenance), not part of the normal flow.
func (s *Store) ReconcileBatchCounters(ctx context.Context, id int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		WITH r AS (
			SELECT COUNT(*) AS total,
			       COUNT(*) FILTER (WHERE validation_status = 'valid') AS valid,
			       COUNT(*) FILTER (WHERE validation_status = 'unverified') AS unverified,
			       COUNT(*) FILTER (WHERE validation_status = 'invalid') AS invalid,
			       jsonb_build_object(
			           'empty_email',   COUNT(*) FILTER (WHERE validation_reason = 'empty_email'),
			           'empty_name',    COUNT(*) FILTER (WHERE validation_reason = 'empty_name'),
			           'invalid_email', COUNT(*) FILTER (WHERE validation_reason = 'invalid_email'),
			           'bad_domain',    COUNT(*) FILTER (WHERE validation_reason = 'bad_domain')) AS reasons
			FROM email_recipients WHERE batch_id = $1
		), j AS (
			SELECT COUNT(*) FILTER (WHERE status = 'pending') AS pending,
			       COUNT(*) FILTER (WHERE status = 'processing') AS processing,
			       COUNT(*) FILTER (WHERE status = 'sent') AS sent,
			       COUNT(*) FILTER (WHERE status = 'failed') AS failed,
			       COUNT(*) FILTER (WHERE status = 'cancelled') AS cancelled
			FROM email_jobs WHERE batch_id = $1
		)
		UPDATE email_batches b
		SET total_rows = r.total, valid_count = r.valid, unverified_count = r.unverified,
		    invalid_count = r.invalid, invalid_counts = r.reasons,
		    pending_count = j.pending, processing_count = j.processing, sent_count = j.sent,
		    failed_count = j.failed, cancelled_count = j.cancelled, updated_at = NOW()
		FROM r, j
		WHERE b.id = $1
		  AND (b.total_rows, b.valid_count, b.unverified_count, b.invalid_count, b.invalid_counts,
		       b.pending_count, b.processing_count, b.sent_count, b.failed_count, b.cancelled_count)
		      IS DISTINCT FROM
		      (r.total::int, r.valid::int, r.unverified::int, r.invalid::int, r.reasons,
		       j.pending::int, j.processing::int, j.sent::int, j.failed::int, j.cancelled::int)`, id)
	if err != nil {
		return false, fmt.Errorf("reconcile batch counters: %w", err)
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
