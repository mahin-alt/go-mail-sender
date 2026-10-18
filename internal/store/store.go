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
	"time"

	// Register pgx as the database/sql driver. The blank identifier "_" imports
	// the package solely for its side-effects (driver registration with database/sql).
	_ "github.com/jackc/pgx/v5/stdlib"
)

// ErrNoJobs is returned when there are no pending jobs available to process.
var ErrNoJobs = errors.New("no jobs available in queue")

// ErrBatchNotFound is returned when a requested batch ID does not exist.
var ErrBatchNotFound = errors.New("batch not found")

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

// Store wraps the database connection.
type Store struct {
	db *sql.DB
}

// New creates a new Store backed by db.
func New(db *sql.DB) *Store {
	return &Store{db: db}
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

	CREATE INDEX IF NOT EXISTS idx_email_jobs_fetchable
	ON email_jobs (run_at, id)
	WHERE status = 'pending';

	CREATE INDEX IF NOT EXISTS idx_email_jobs_batch_id
	ON email_jobs (batch_id, status);
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

		if err := insertJobsChunk(ctx, tx, batchID, chunk); err != nil {
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
func insertJobsChunk(ctx context.Context, tx *sql.Tx, batchID int64, jobs []Job) error {
	var valueStrings []string
	var valueArgs []any

	// 8 columns per row
	for i, j := range jobs {
		offset := i * 8
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			offset+1, offset+2, offset+3, offset+4, offset+5, offset+6, offset+7, offset+8))

		maxAttempts := j.MaxAttempts
		if maxAttempts <= 0 {
			maxAttempts = 3
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
