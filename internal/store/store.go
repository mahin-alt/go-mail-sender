// Package store manages all database operations for batches and email background jobs.
//
// Concept: repository pattern. The API and background workers don't write
// raw SQL; they talk to this Store. This keeps all SQL statements, transactions,
// and table schemas organized in one place.
//
// We use Go's built-in "database/sql" package with the pgx driver.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	// Register pgx as the database/sql driver. The blank identifier "_" imports
	// the package solely for its side-effects (driver registration with database/sql).
	_ "github.com/jackc/pgx/v5/stdlib"
)

// ErrNoJobs is returned when there are no pending jobs available to process.
var ErrNoJobs = errors.New("no jobs available in queue")

// ErrBatchNotFound is returned when a requested batch ID does not exist.
var ErrBatchNotFound = errors.New("batch not found")

// ErrJobNotRetryable is returned when a job does not exist or is not in the 'failed' state.
var ErrJobNotRetryable = errors.New("job not found or not failed")

// Batch represents one bulk email upload campaign.
type Batch struct {
	ID              int64      `json:"id"`
	Filename        string     `json:"filename"`
	SubjectTemplate string     `json:"subject_template"`
	BodyTemplate    string     `json:"body_template"`
	TotalJobs       int        `json:"total_jobs"`
	Status          string     `json:"status"` // 'pending', 'processing', 'completed', 'failed'
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
}

// Job represents a single email to be delivered by a background worker.
type Job struct {
	ID             int64      `json:"id"`
	BatchID        int64      `json:"batch_id"`
	RecipientName  string     `json:"recipient_name"`
	RecipientEmail string     `json:"recipient_email"`
	Subject        string     `json:"subject"`
	Body           string     `json:"body"`
	Status         string     `json:"status"` // 'pending', 'processing', 'sent', 'failed'
	Attempts       int        `json:"attempts"`
	MaxAttempts    int        `json:"max_attempts"`
	LastError      *string    `json:"last_error,omitempty"`
	LockedAt       *time.Time `json:"locked_at,omitempty"`
	RunAt          time.Time  `json:"run_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	SentAt         *time.Time `json:"sent_at,omitempty"`
}

// BatchProgress summarizes the current state and counts of a batch.
type BatchProgress struct {
	BatchID    int64  `json:"batch_id"`
	Filename   string `json:"filename"`
	Status     string `json:"status"`
	Total      int    `json:"total"`
	Sent       int    `json:"sent"`
	Failed     int    `json:"failed"`
	Pending    int    `json:"pending"`
	Processing int    `json:"processing"`
	Invalid    int    `json:"invalid"`
}

// BatchSummary is one row of the batch list: the batch plus its job counts.
type BatchSummary struct {
	ID          int64      `json:"id"`
	Filename    string     `json:"filename"`
	Status      string     `json:"status"`
	Total       int        `json:"total"`
	Sent        int        `json:"sent"`
	Failed      int        `json:"failed"`
	Pending     int        `json:"pending"`
	Processing  int        `json:"processing"`
	Invalid     int        `json:"invalid"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// BatchListFilter narrows and pages the batch list.
type BatchListFilter struct {
	Status string // exact batch status, "" = any
	Search string // case-insensitive substring of the filename, "" = any
	Limit  int
	Offset int
}

// defaultMaxAttempts is used when no value is configured via SetMaxAttempts.
const defaultMaxAttempts = 3

// Store wraps the database connection.
type Store struct {
	db          *sql.DB
	maxAttempts atomic.Int64 // read by uploads while Settings may change it
}

// New creates a new Store backed by db.
func New(db *sql.DB) *Store {
	s := &Store{db: db}
	s.maxAttempts.Store(defaultMaxAttempts)
	return s
}

// SetMaxAttempts sets how many delivery attempts newly queued jobs get.
// Values below 1 are ignored.
func (s *Store) SetMaxAttempts(n int) {
	if n > 0 {
		s.maxAttempts.Store(int64(n))
	}
}

// InitSchema ensures the required tables and indexes exist.
// This allows the app to start up and run automatically without manual SQL migration steps.
func (s *Store) InitSchema(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS batches (
		id BIGSERIAL PRIMARY KEY,
		filename TEXT NOT NULL DEFAULT '',
		subject_template TEXT NOT NULL,
		body_template TEXT NOT NULL,
		total_jobs INT NOT NULL DEFAULT 0,
		status VARCHAR(32) NOT NULL DEFAULT 'pending',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		completed_at TIMESTAMPTZ
	);

	CREATE TABLE IF NOT EXISTS email_jobs (
		id BIGSERIAL PRIMARY KEY,
		batch_id BIGINT NOT NULL REFERENCES batches(id) ON DELETE CASCADE,
		recipient_name TEXT NOT NULL,
		recipient_email TEXT NOT NULL,
		subject TEXT NOT NULL,
		body TEXT NOT NULL,
		status VARCHAR(32) NOT NULL DEFAULT 'pending',
		attempts INT NOT NULL DEFAULT 0,
		max_attempts INT NOT NULL DEFAULT 3,
		last_error TEXT,
		locked_at TIMESTAMPTZ,
		run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		sent_at TIMESTAMPTZ
	);

	CREATE TABLE IF NOT EXISTS app_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		secret BOOLEAN NOT NULL DEFAULT FALSE,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_email_jobs_fetchable
	ON email_jobs (run_at, id)
	WHERE status = 'pending';

	CREATE INDEX IF NOT EXISTS idx_email_jobs_batch_id
	ON email_jobs (batch_id, status);

	-- Small partial indexes that keep the dashboard's stats queries fast as the
	-- table grows: each covers only the rows one stats query looks at.
	CREATE INDEX IF NOT EXISTS idx_email_jobs_sent_at
	ON email_jobs (sent_at) WHERE status = 'sent';

	CREATE INDEX IF NOT EXISTS idx_email_jobs_failed
	ON email_jobs (updated_at, batch_id) WHERE status = 'failed';

	CREATE INDEX IF NOT EXISTS idx_email_jobs_processing
	ON email_jobs (id) WHERE status = 'processing';
	`
	_, err := s.db.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("init schema: %w", err)
	}
	return nil
}

// CreateBatchWithJobs saves a batch and all its individual email jobs
// inside a single atomic database transaction.
//
// Concept: why a transaction?
// If the app crashed halfway through inserting 500 emails, a transaction
// rolls back completely so you never end up with a half-uploaded batch.
func (s *Store) CreateBatchWithJobs(
	ctx context.Context,
	filename, subjectTmpl, bodyTmpl string,
	jobs []Job,
) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() // Safe: no-op if tx.Commit() succeeds below.

	// Determine initial batch status based on whether there are pending jobs to process
	hasPending := false
	for _, j := range jobs {
		if j.Status == "pending" || j.Status == "" {
			hasPending = true
			break
		}
	}
	initialStatus := "pending"
	if !hasPending && len(jobs) > 0 {
		initialStatus = "completed"
	}

	// 1. Insert the parent batch record
	var batchID int64
	insertBatchSQL := `
		INSERT INTO batches (filename, subject_template, body_template, total_jobs, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`
	err = tx.QueryRowContext(ctx, insertBatchSQL, filename, subjectTmpl, bodyTmpl, len(jobs), initialStatus).Scan(&batchID)
	if err != nil {
		return 0, fmt.Errorf("insert batch: %w", err)
	}

	if len(jobs) == 0 {
		if err := tx.Commit(); err != nil {
			return 0, fmt.Errorf("commit empty batch: %w", err)
		}
		return batchID, nil
	}

	// 2. Bulk-insert the email_jobs in chunks of 500 to keep parameter counts well within limits.
	const chunkSize = 500
	for i := 0; i < len(jobs); i += chunkSize {
		end := i + chunkSize
		if end > len(jobs) {
			end = len(jobs)
		}
		chunk := jobs[i:end]

		if err := insertJobsChunk(ctx, tx, batchID, chunk, int(s.maxAttempts.Load())); err != nil {
			return 0, fmt.Errorf("insert jobs chunk: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit batch transaction: %w", err)
	}

	return batchID, nil
}

// insertJobsChunk builds a multi-row INSERT statement:
// INSERT INTO email_jobs (...) VALUES ($1, $2...), ($9, $10...)
func insertJobsChunk(ctx context.Context, tx *sql.Tx, batchID int64, jobs []Job, defaultAttempts int) error {
	var valueStrings []string
	var valueArgs []any

	// 8 columns per row
	for i, j := range jobs {
		offset := i * 8
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			offset+1, offset+2, offset+3, offset+4, offset+5, offset+6, offset+7, offset+8))

		maxAttempts := j.MaxAttempts
		if maxAttempts <= 0 {
			maxAttempts = defaultAttempts
		}

		status := j.Status
		if status == "" {
			status = "pending"
		}

		valueArgs = append(valueArgs,
			batchID,
			j.RecipientName,
			j.RecipientEmail,
			j.Subject,
			j.Body,
			status,
			j.LastError,
			maxAttempts,
		)
	}

	query := fmt.Sprintf(`
		INSERT INTO email_jobs (batch_id, recipient_name, recipient_email, subject, body, status, last_error, max_attempts)
		VALUES %s
	`, strings.Join(valueStrings, ", "))

	_, err := tx.ExecContext(ctx, query, valueArgs...)
	return err
}

// ClaimNextJob uses PostgreSQL's row-level locking "FOR UPDATE SKIP LOCKED"
// to safely claim the next available job for a worker.
//
// How this query works:
//  1. "next_job" CTE selects the oldest 'pending' job ready to run (run_at <= NOW()).
//  2. "FOR UPDATE" places an exclusive row lock on that job.
//  3. "SKIP LOCKED" skips any row already locked by another concurrent worker!
//  4. The outer UPDATE immediately sets status = 'processing' and increments attempts.
//  5. Everything happens ATOMICALLY in a single database round-trip.
func (s *Store) ClaimNextJob(ctx context.Context) (*Job, error) {
	query := `
	WITH next_job AS (
		SELECT id
		FROM email_jobs
		WHERE status = 'pending'
		  AND run_at <= NOW()
		ORDER BY id ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	)
	UPDATE email_jobs
	SET status = 'processing',
	    attempts = attempts + 1,
	    locked_at = NOW(),
	    updated_at = NOW()
	FROM next_job
	WHERE email_jobs.id = next_job.id
	RETURNING email_jobs.id, email_jobs.batch_id, email_jobs.recipient_name,
	          email_jobs.recipient_email, email_jobs.subject, email_jobs.body,
	          email_jobs.status, email_jobs.attempts, email_jobs.max_attempts
	`

	row := s.db.QueryRowContext(ctx, query)
	var j Job
	err := row.Scan(
		&j.ID,
		&j.BatchID,
		&j.RecipientName,
		&j.RecipientEmail,
		&j.Subject,
		&j.Body,
		&j.Status,
		&j.Attempts,
		&j.MaxAttempts,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoJobs
		}
		return nil, fmt.Errorf("claim job: %w", err)
	}

	// Update the batch status to 'processing' if it is still 'pending'
	_ = s.setBatchProcessing(ctx, j.BatchID)

	return &j, nil
}

// MarkJobSent records that an email was successfully sent by a worker.
func (s *Store) MarkJobSent(ctx context.Context, jobID int64) error {
	var batchID int64
	query := `
		UPDATE email_jobs
		SET status = 'sent',
		    sent_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
		RETURNING batch_id
	`
	err := s.db.QueryRowContext(ctx, query, jobID).Scan(&batchID)
	if err != nil {
		return fmt.Errorf("mark job sent: %w", err)
	}

	// Check if this was the last remaining job in the batch
	return s.checkBatchCompletion(ctx, batchID)
}

// MarkJobFailed records a failure. If the job has remaining retry attempts,
// it schedules a retry in the future; otherwise it permanently marks it 'failed'.
func (s *Store) MarkJobFailed(ctx context.Context, jobID int64, reason string, retryIn time.Duration) error {
	var (
		attempts    int
		maxAttempts int
		batchID     int64
	)

	checkSQL := `
		SELECT batch_id, attempts, max_attempts
		FROM email_jobs
		WHERE id = $1
	`
	err := s.db.QueryRowContext(ctx, checkSQL, jobID).Scan(&batchID, &attempts, &maxAttempts)
	if err != nil {
		return fmt.Errorf("inspect job for failure: %w", err)
	}

	if attempts < maxAttempts {
		// Retry later: set status back to 'pending' with a future run_at timestamp.
		retrySQL := `
			UPDATE email_jobs
			SET status = 'pending',
			    run_at = NOW() + ($1 * INTERVAL '1 millisecond'),
			    last_error = $2,
			    updated_at = NOW()
			WHERE id = $3
		`
		_, err = s.db.ExecContext(ctx, retrySQL, retryIn.Milliseconds(), reason, jobID)
	} else {
		// Permanent failure
		failSQL := `
			UPDATE email_jobs
			SET status = 'failed',
			    last_error = $1,
			    updated_at = NOW()
			WHERE id = $2
		`
		_, err = s.db.ExecContext(ctx, failSQL, reason, jobID)
	}
	if err != nil {
		return fmt.Errorf("update failed job: %w", err)
	}

	return s.checkBatchCompletion(ctx, batchID)
}

// setBatchProcessing marks a batch as 'processing' when its first job starts.
func (s *Store) setBatchProcessing(ctx context.Context, batchID int64) error {
	query := `
		UPDATE batches
		SET status = 'processing',
		    updated_at = NOW()
		WHERE id = $1 AND status = 'pending'
	`
	_, err := s.db.ExecContext(ctx, query, batchID)
	return err
}

// checkBatchCompletion checks if all jobs for a batch are finished (either 'sent' or 'failed').
// If so, it updates the batch status to 'completed'.
func (s *Store) checkBatchCompletion(ctx context.Context, batchID int64) error {
	query := `
		UPDATE batches
		SET status = 'completed',
		    completed_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
		  AND NOT EXISTS (
			  SELECT 1 FROM email_jobs
			  WHERE batch_id = $1 AND status IN ('pending', 'processing')
		  )
	`
	_, err := s.db.ExecContext(ctx, query, batchID)
	return err
}

// GetBatchProgress returns current statistics for a batch.
func (s *Store) GetBatchProgress(ctx context.Context, batchID int64) (*BatchProgress, error) {
	query := `
		SELECT b.id,
		       b.filename,
		       b.status,
		       b.total_jobs,
		       COUNT(*) FILTER (WHERE j.status = 'sent') AS sent_count,
		       COUNT(*) FILTER (WHERE j.status = 'failed') AS failed_count,
		       COUNT(*) FILTER (WHERE j.status = 'pending') AS pending_count,
		       COUNT(*) FILTER (WHERE j.status = 'processing') AS processing_count,
		       COUNT(*) FILTER (WHERE j.status = 'invalid') AS invalid_count
		FROM batches b
		LEFT JOIN email_jobs j ON j.batch_id = b.id
		WHERE b.id = $1
		GROUP BY b.id, b.filename, b.status, b.total_jobs
	`

	var bp BatchProgress
	err := s.db.QueryRowContext(ctx, query, batchID).Scan(
		&bp.BatchID,
		&bp.Filename,
		&bp.Status,
		&bp.Total,
		&bp.Sent,
		&bp.Failed,
		&bp.Pending,
		&bp.Processing,
		&bp.Invalid,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrBatchNotFound
		}
		return nil, fmt.Errorf("get batch progress: %w", err)
	}

	return &bp, nil
}

// GetBatchJobs retrieves jobs for a given batch, optionally filtered by status (e.g. "invalid").
func (s *Store) GetBatchJobs(ctx context.Context, batchID int64, statusFilter string) ([]Job, error) {
	query := `
		SELECT id, batch_id, recipient_name, recipient_email, subject, body,
		       status, attempts, max_attempts, last_error, locked_at, run_at,
		       created_at, updated_at, sent_at
		FROM email_jobs
		WHERE batch_id = $1
		  AND ($2 = '' OR status = $2)
		ORDER BY id ASC
	`
	rows, err := s.db.QueryContext(ctx, query, batchID, statusFilter)
	if err != nil {
		return nil, fmt.Errorf("query batch jobs: %w", err)
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(
			&j.ID,
			&j.BatchID,
			&j.RecipientName,
			&j.RecipientEmail,
			&j.Subject,
			&j.Body,
			&j.Status,
			&j.Attempts,
			&j.MaxAttempts,
			&j.LastError,
			&j.LockedAt,
			&j.RunAt,
			&j.CreatedAt,
			&j.UpdatedAt,
			&j.SentAt,
		); err != nil {
			return nil, fmt.Errorf("scan job row: %w", err)
		}
		jobs = append(jobs, j)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return jobs, nil
}

// ListBatches returns one page of batches (newest first) with their job counts,
// plus the total number of batches matching the filter (for pagination).
func (s *Store) ListBatches(ctx context.Context, f BatchListFilter) ([]BatchSummary, int, error) {
	const where = `
		WHERE ($1 = '' OR b.status = $1)
		  AND ($2 = '' OR b.filename ILIKE '%' || $2 || '%')
	`

	var total int
	countSQL := `SELECT COUNT(*) FROM batches b ` + where
	if err := s.db.QueryRowContext(ctx, countSQL, f.Status, f.Search).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count batches: %w", err)
	}

	listSQL := `
		SELECT b.id, b.filename, b.status, b.total_jobs,
		       COUNT(*) FILTER (WHERE j.status = 'sent'),
		       COUNT(*) FILTER (WHERE j.status = 'failed'),
		       COUNT(*) FILTER (WHERE j.status = 'pending'),
		       COUNT(*) FILTER (WHERE j.status = 'processing'),
		       COUNT(*) FILTER (WHERE j.status = 'invalid'),
		       b.created_at, b.completed_at
		FROM batches b
		LEFT JOIN email_jobs j ON j.batch_id = b.id
	` + where + `
		GROUP BY b.id
		ORDER BY b.id DESC
		LIMIT $3 OFFSET $4
	`
	rows, err := s.db.QueryContext(ctx, listSQL, f.Status, f.Search, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query batches: %w", err)
	}
	defer rows.Close()

	batches := []BatchSummary{} // non-nil so it encodes as [] not null
	for rows.Next() {
		var b BatchSummary
		if err := rows.Scan(
			&b.ID, &b.Filename, &b.Status, &b.Total,
			&b.Sent, &b.Failed, &b.Pending, &b.Processing, &b.Invalid,
			&b.CreatedAt, &b.CompletedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan batch row: %w", err)
		}
		batches = append(batches, b)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows iteration error: %w", err)
	}
	return batches, total, nil
}

// JobListFilter narrows and pages a batch's job list.
type JobListFilter struct {
	Status string // exact job status, "" = any
	Search string // case-insensitive substring of the recipient email or name, "" = any
	Limit  int
	Offset int
}

// ListBatchJobs returns one page of a batch's jobs (oldest first) plus the
// total number of jobs matching the filter (for pagination).
func (s *Store) ListBatchJobs(ctx context.Context, batchID int64, f JobListFilter) ([]Job, int, error) {
	const where = `
		WHERE batch_id = $1
		  AND ($2 = '' OR status = $2)
		  AND ($3 = '' OR recipient_email ILIKE '%' || $3 || '%' OR recipient_name ILIKE '%' || $3 || '%')
	`

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM email_jobs `+where,
		batchID, f.Status, f.Search).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count batch jobs: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, batch_id, recipient_name, recipient_email, subject, body,
		       status, attempts, max_attempts, last_error, locked_at, run_at,
		       created_at, updated_at, sent_at
		FROM email_jobs `+where+`
		ORDER BY id ASC
		LIMIT $4 OFFSET $5`,
		batchID, f.Status, f.Search, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query batch jobs: %w", err)
	}
	defer rows.Close()

	jobs := []Job{} // non-nil so it encodes as [] not null
	for rows.Next() {
		var j Job
		if err := rows.Scan(
			&j.ID, &j.BatchID, &j.RecipientName, &j.RecipientEmail, &j.Subject, &j.Body,
			&j.Status, &j.Attempts, &j.MaxAttempts, &j.LastError, &j.LockedAt, &j.RunAt,
			&j.CreatedAt, &j.UpdatedAt, &j.SentAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan job row: %w", err)
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows iteration error: %w", err)
	}
	return jobs, total, nil
}

// RetryFailedJobs puts 'failed' jobs back in the queue with a fresh set of
// attempts and returns how many were requeued. Only 'failed' jobs are touched:
// 'invalid' rows were never rendered into an email, so there is nothing to send.
//
// With jobID == nil every failed job in the batch is requeued; otherwise only
// that one job (which must belong to the batch). If any job was requeued and
// the batch had already finished, the batch is reopened so progress and
// completion tracking work again. Returns ErrBatchNotFound for an unknown batch.
func (s *Store) RetryFailedJobs(ctx context.Context, batchID int64, jobID *int64) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE email_jobs
		SET status = 'pending',
		    attempts = 0,
		    run_at = NOW(),
		    locked_at = NULL,
		    updated_at = NOW()
		WHERE batch_id = $1
		  AND status = 'failed'
		  AND ($2::BIGINT IS NULL OR id = $2)
	`, batchID, jobID)
	if err != nil {
		return 0, fmt.Errorf("requeue failed jobs: %w", err)
	}
	n64, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("requeue failed jobs: %w", err)
	}

	if n64 > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE batches
			SET status = 'pending', completed_at = NULL, updated_at = NOW()
			WHERE id = $1 AND status = 'completed'
		`, batchID); err != nil {
			return 0, fmt.Errorf("reopen batch: %w", err)
		}
	} else {
		var exists bool
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM batches WHERE id = $1)`, batchID).Scan(&exists); err != nil {
			return 0, fmt.Errorf("check batch exists: %w", err)
		}
		if !exists {
			return 0, ErrBatchNotFound
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit retry: %w", err)
	}
	return int(n64), nil
}

// RetryFailedJob requeues one failed job. It returns ErrJobNotRetryable if the
// job does not exist or is not currently 'failed'.
func (s *Store) RetryFailedJob(ctx context.Context, jobID int64) (int, error) {
	var batchID int64
	err := s.db.QueryRowContext(ctx,
		`SELECT batch_id FROM email_jobs WHERE id = $1 AND status = 'failed'`, jobID).Scan(&batchID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrJobNotRetryable
		}
		return 0, fmt.Errorf("find failed job: %w", err)
	}
	return s.RetryFailedJobs(ctx, batchID, &jobID)
}

// SettingRow is one stored settings override. For secret settings Value holds
// ciphertext; the store never interprets it.
type SettingRow struct {
	Value     string
	Secret    bool
	UpdatedAt time.Time
}

// GetSettings returns every stored settings override, keyed by setting name.
func (s *Store) GetSettings(ctx context.Context) (map[string]SettingRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value, secret, updated_at FROM app_settings`)
	if err != nil {
		return nil, fmt.Errorf("query settings: %w", err)
	}
	defer rows.Close()

	out := map[string]SettingRow{}
	for rows.Next() {
		var key string
		var r SettingRow
		if err := rows.Scan(&key, &r.Value, &r.Secret, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan setting: %w", err)
		}
		out[key] = r
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}
	return out, nil
}

// SaveSettings upserts the given overrides and deletes the listed keys, all in
// one transaction so a settings change is applied completely or not at all.
func (s *Store) SaveSettings(ctx context.Context, set map[string]SettingRow, remove []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	for key, r := range set {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO app_settings (key, value, secret, updated_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (key) DO UPDATE
			SET value = EXCLUDED.value, secret = EXCLUDED.secret, updated_at = NOW()
		`, key, r.Value, r.Secret); err != nil {
			return fmt.Errorf("save setting %s: %w", key, err)
		}
	}
	for _, key := range remove {
		if _, err := tx.ExecContext(ctx, `DELETE FROM app_settings WHERE key = $1`, key); err != nil {
			return fmt.Errorf("delete setting %s: %w", key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit settings: %w", err)
	}
	return nil
}
