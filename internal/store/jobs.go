package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Email job statuses (email_jobs.status).
const (
	JobPending    = "pending"
	JobProcessing = "processing"
	JobSent       = "sent"
	JobFailed     = "failed"
	JobCancelled  = "cancelled"
)

// Job is a claimed email: everything a worker needs to build and send it.
// The subject and body are rendered by the worker from the batch templates.
type Job struct {
	ID              int64
	BatchID         int64
	RecipientID     int64
	Name            string
	Email           string
	SubjectTemplate string
	BodyTemplate    string
	Attempts        int // including this claim
	MaxAttempts     int

	// QueuedFor is how long the job waited from becoming due (available_at)
	// until a worker claimed it. For logging.
	QueuedFor time.Duration
}

// ClaimNextJob atomically claims the next due email of a batch that is
// 'queued' or 'sending' (never one still generating its jobs), marks it
// 'processing' under this worker's lease, moves the batch counters, and
// returns it with the recipient and templates. Batches are served oldest
// first. ErrNoJobs when nothing is due.
//
// Concept: FOR UPDATE SKIP LOCKED. The SELECT locks the job row it picks; a
// second worker running the same query at the same moment skips that locked
// row and takes the next one, so no email is claimed twice and nobody waits.
func (s *Store) ClaimNextJob(ctx context.Context, workerID string) (*Job, error) {
	for range 5 { // a picked batch may stop sending meanwhile; then try again
		j, err := s.claimOnce(ctx, workerID)
		if !errors.Is(err, errBatchStopped) {
			return j, err
		}
	}
	return nil, ErrNoJobs
}

// errBatchStopped: the picked email's batch stopped sending (cancelled)
// between picking and locking it.
var errBatchStopped = errors.New("batch stopped sending")

// claimOnce is one claim attempt. Locks are taken email first, batch second,
// the same order as every other writer, so no two transactions can each hold
// the lock the other one waits for.
func (s *Store) claimOnce(ctx context.Context, workerID string) (*Job, error) {
	var j Job
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
			SELECT j.id, j.batch_id
			FROM email_batches b
			CROSS JOIN LATERAL (
				SELECT id, batch_id FROM email_jobs
				WHERE batch_id = b.id AND status = 'pending' AND available_at <= NOW()
				ORDER BY available_at, id
				LIMIT 1
				FOR UPDATE SKIP LOCKED
			) j
			WHERE b.status IN ('queued', 'sending')
			ORDER BY b.id
			LIMIT 1`).Scan(&j.ID, &j.BatchID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNoJobs
		}
		if err != nil {
			return fmt.Errorf("pick job: %w", err)
		}

		var status string
		if err := tx.QueryRowContext(ctx, `
			SELECT status, COALESCE(subject_template, ''), COALESCE(body_template, '')
			FROM email_batches WHERE id = $1 FOR UPDATE`, j.BatchID).
			Scan(&status, &j.SubjectTemplate, &j.BodyTemplate); err != nil {
			return fmt.Errorf("lock batch: %w", err)
		}
		if status != StatusQueued && status != StatusSending {
			return errBatchStopped
		}

		var queuedMS int64
		if err := tx.QueryRowContext(ctx, `
			UPDATE email_jobs e
			SET status = 'processing', attempts = attempts + 1,
			    locked_at = NOW(), locked_by = $2, updated_at = NOW()
			FROM email_recipients r
			WHERE e.id = $1 AND r.id = e.recipient_id
			RETURNING e.recipient_id, r.name, r.email, e.attempts, e.max_attempts,
			          (EXTRACT(EPOCH FROM (NOW() - e.available_at)) * 1000)::BIGINT`, j.ID, workerID).
			Scan(&j.RecipientID, &j.Name, &j.Email, &j.Attempts, &j.MaxAttempts, &queuedMS); err != nil {
			return fmt.Errorf("claim job: %w", err)
		}
		j.QueuedFor = time.Duration(queuedMS) * time.Millisecond

		if _, err := tx.ExecContext(ctx, `
			UPDATE email_batches
			SET pending_count = pending_count - 1, processing_count = processing_count + 1,
			    status = 'sending', updated_at = NOW()
			WHERE id = $1`, j.BatchID); err != nil {
			return fmt.Errorf("count claimed job: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// finishBatchIfDone marks a sending batch finished once no email is waiting
// or being sent: 'completed', or 'completed_with_errors' when any failed.
// Reports whether this call finished it (so the summary is logged once).
func finishBatchIfDone(ctx context.Context, tx *sql.Tx, batchID int64) (bool, error) {
	res, err := tx.ExecContext(ctx, `
		UPDATE email_batches
		SET status = CASE WHEN failed_count > 0 THEN 'completed_with_errors' ELSE 'completed' END,
		    completed_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status IN ('queued', 'sending')
		  AND pending_count = 0 AND processing_count = 0`, batchID)
	if err != nil {
		return false, fmt.Errorf("check batch completion: %w", err)
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// MarkJobSent records a successful send and reports whether it finished the
// batch. Only the worker holding the job's lease can record it.
func (s *Store) MarkJobSent(ctx context.Context, jobID int64, workerID string) (batchDone bool, err error) {
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		var batchID int64
		err := tx.QueryRowContext(ctx, `
			UPDATE email_jobs
			SET status = 'sent', sent_at = NOW(), locked_by = NULL, last_error = NULL, updated_at = NOW()
			WHERE id = $1 AND status = 'processing' AND locked_by = $2
			RETURNING batch_id`, jobID, workerID).Scan(&batchID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrLeaseLost
		}
		if err != nil {
			return fmt.Errorf("mark job sent: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE email_batches
			SET processing_count = processing_count - 1, sent_count = sent_count + 1, updated_at = NOW()
			WHERE id = $1`, batchID); err != nil {
			return fmt.Errorf("count sent job: %w", err)
		}
		batchDone, err = finishBatchIfDone(ctx, tx, batchID)
		return err
	})
	return batchDone, err
}

// MarkJobFailed records a failed send. With attempts left (and retry true)
// the email goes back to 'pending', due after retryIn; otherwise it is
// 'failed' for good. If the batch was cancelled while this email was being
// sent, the email becomes 'cancelled' instead of being retried. Reports
// whether this finished the batch.
func (s *Store) MarkJobFailed(ctx context.Context, jobID int64, workerID, reason string, retry bool, retryIn time.Duration) (batchDone bool, err error) {
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		var batchID int64
		var newStatus string
		err := tx.QueryRowContext(ctx, `
			UPDATE email_jobs e
			SET status = CASE
			        WHEN (SELECT status FROM email_batches WHERE id = e.batch_id) = 'cancelled' THEN 'cancelled'
			        WHEN $3 AND e.attempts < e.max_attempts THEN 'pending'
			        ELSE 'failed'
			    END,
			    available_at = NOW() + $4 * INTERVAL '1 millisecond',
			    last_error = $5, locked_by = NULL, updated_at = NOW()
			WHERE id = $1 AND status = 'processing' AND locked_by = $2
			RETURNING batch_id, status`, jobID, workerID, retry, ms(retryIn), reason).Scan(&batchID, &newStatus)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrLeaseLost
		}
		if err != nil {
			return fmt.Errorf("mark job failed: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE email_batches
			SET processing_count = processing_count - 1,
			    pending_count   = pending_count   + CASE WHEN $2 = 'pending'   THEN 1 ELSE 0 END,
			    failed_count    = failed_count    + CASE WHEN $2 = 'failed'    THEN 1 ELSE 0 END,
			    cancelled_count = cancelled_count + CASE WHEN $2 = 'cancelled' THEN 1 ELSE 0 END,
			    updated_at = NOW()
			WHERE id = $1`, batchID, newStatus); err != nil {
			return fmt.Errorf("count failed job: %w", err)
		}
		batchDone, err = finishBatchIfDone(ctx, tx, batchID)
		return err
	})
	return batchDone, err
}

// RequeueStuckJobs finds emails left in 'processing' for longer than lease
// (the worker crashed or was killed mid-send) and puts them back in the
// queue, or fails them if they used up their attempts. Counters move with
// them. Returns how many were recovered.
//
// Note: if the crash happened after the SMTP server accepted the email, the
// recipient gets it twice. SMTP cannot promise exactly-once delivery.
func (s *Store) RequeueStuckJobs(ctx context.Context, lease time.Duration) (int, error) {
	var total int
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			WITH stuck AS (
				UPDATE email_jobs e
				SET status = CASE
				        WHEN (SELECT status FROM email_batches WHERE id = e.batch_id) = 'cancelled' THEN 'cancelled'
				        WHEN attempts < max_attempts THEN 'pending'
				        ELSE 'failed'
				    END,
				    available_at = NOW(), locked_by = NULL,
				    last_error = 'worker stopped while sending; requeued', updated_at = NOW()
				WHERE status = 'processing' AND locked_at < NOW() - $1 * INTERVAL '1 millisecond'
				RETURNING batch_id, status
			)
			SELECT batch_id,
			       COUNT(*) FILTER (WHERE status = 'pending'),
			       COUNT(*) FILTER (WHERE status = 'failed'),
			       COUNT(*) FILTER (WHERE status = 'cancelled')
			FROM stuck GROUP BY batch_id`, ms(lease))
		if err != nil {
			return fmt.Errorf("requeue stuck jobs: %w", err)
		}
		type moved struct {
			batchID                    int64
			pending, failed, cancelled int
		}
		var all []moved
		for rows.Next() {
			var m moved
			if err := rows.Scan(&m.batchID, &m.pending, &m.failed, &m.cancelled); err != nil {
				rows.Close()
				return err
			}
			all = append(all, m)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, m := range all {
			n := m.pending + m.failed + m.cancelled
			total += n
			if _, err := tx.ExecContext(ctx, `
				UPDATE email_batches
				SET processing_count = processing_count - $2, pending_count = pending_count + $3,
				    failed_count = failed_count + $4, cancelled_count = cancelled_count + $5, updated_at = NOW()
				WHERE id = $1`, m.batchID, n, m.pending, m.failed, m.cancelled); err != nil {
				return fmt.Errorf("move stuck counters: %w", err)
			}
			if _, err := finishBatchIfDone(ctx, tx, m.batchID); err != nil {
				return err
			}
		}
		return nil
	})
	return total, err
}

// EmailItem is one email in a batch's delivery list.
type EmailItem struct {
	ID          int64      `json:"id"`
	BatchID     int64      `json:"batch_id"`
	RecipientID int64      `json:"recipient_id"`
	Row         int        `json:"row"`
	Name        string     `json:"name"`
	Email       string     `json:"email"`
	Status      string     `json:"status"`
	Attempts    int        `json:"attempts"`
	MaxAttempts int        `json:"max_attempts"`
	LastError   *string    `json:"last_error"`
	AvailableAt time.Time  `json:"available_at"`
	SentAt      *time.Time `json:"sent_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// JobListFilter selects one page of a batch's emails (cursor paging by job ID).
type JobListFilter struct {
	Status string // exact job status, "" = any
	Search string // case-insensitive substring of the recipient's email or name, "" = any
	After  int64  // only emails with a larger ID (0 = from the start)
	Limit  int
}

// ListBatchJobs returns up to f.Limit emails of a batch, oldest first.
func (s *Store) ListBatchJobs(ctx context.Context, batchID int64, f JobListFilter) ([]EmailItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT j.id, j.batch_id, j.recipient_id, r.row_number, r.name, r.email,
		       j.status, j.attempts, j.max_attempts, j.last_error, j.available_at,
		       j.sent_at, j.created_at, j.updated_at
		FROM email_jobs j
		JOIN email_recipients r ON r.id = j.recipient_id
		WHERE j.batch_id = $1
		  AND ($2 = '' OR j.status = $2)
		  AND j.id > $3
		  AND ($4 = '' OR r.email ILIKE '%' || $4 || '%' OR r.name ILIKE '%' || $4 || '%')
		ORDER BY j.id
		LIMIT $5`, batchID, f.Status, f.After, f.Search, f.Limit)
	if err != nil {
		return nil, fmt.Errorf("list batch jobs: %w", err)
	}
	defer rows.Close()

	out := []EmailItem{} // non-nil so it encodes as [] not null
	for rows.Next() {
		var e EmailItem
		if err := rows.Scan(&e.ID, &e.BatchID, &e.RecipientID, &e.Row, &e.Name, &e.Email,
			&e.Status, &e.Attempts, &e.MaxAttempts, &e.LastError, &e.AvailableAt,
			&e.SentAt, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("job rows: %w", err)
	}
	return out, nil
}
