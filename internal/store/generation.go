package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// GenerationTask is a claimed job-generation task: create the email jobs of
// one batch, resuming after LastRecipientID.
type GenerationTask struct {
	BatchID         int64
	LastRecipientID int64
	Attempts        int
}

// ClaimGeneration atomically takes the next batch waiting for its email jobs
// (or one whose generator stopped renewing its lease), like ClaimImportJob.
func (s *Store) ClaimGeneration(ctx context.Context, workerID string, lease time.Duration) (*GenerationTask, error) {
	var t GenerationTask
	err := s.db.QueryRowContext(ctx, `
		WITH next AS (
			SELECT id FROM email_batches
			WHERE status = 'generating_jobs'
			  AND (job_generation_status = 'pending'
			       OR (job_generation_status = 'processing'
			           AND job_generation_locked_at < NOW() - $2 * INTERVAL '1 millisecond'))
			ORDER BY id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE email_batches b
		SET job_generation_status = 'processing', job_generation_attempts = job_generation_attempts + 1,
		    job_generation_locked_at = NOW(), job_generation_locked_by = $1, updated_at = NOW()
		FROM next
		WHERE b.id = next.id
		RETURNING b.id, b.last_generated_recipient_id, b.job_generation_attempts`,
		workerID, ms(lease)).Scan(&t.BatchID, &t.LastRecipientID, &t.Attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoJobs
	}
	if err != nil {
		return nil, fmt.Errorf("claim generation: %w", err)
	}
	return &t, nil
}

// GenerateChunk creates email jobs for the next chunkSize sendable recipients
// after the checkpoint, and moves the checkpoint and pending counter in the
// SAME transaction. When no recipients are left it finishes generation and
// the batch becomes 'queued', which is when sending workers may start.
//
// Retrying a chunk is safe: email_jobs.recipient_id is UNIQUE and the insert
// skips rows that already have a job (ON CONFLICT DO NOTHING).
//
// Returns how many jobs were created and whether generation is finished.
// ErrLeaseLost if this worker no longer owns the task, or the batch was
// cancelled meanwhile.
func (s *Store) GenerateChunk(ctx context.Context, t *GenerationTask, workerID string, chunkSize int) (created int, done bool, err error) {
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		// Lock the batch and check ownership; a cancel waits for this or vice versa.
		var last int64
		err := tx.QueryRowContext(ctx, `
			SELECT last_generated_recipient_id FROM email_batches
			WHERE id = $1 AND status = 'generating_jobs' AND job_generation_locked_by = $2
			FOR UPDATE`, t.BatchID, workerID).Scan(&last)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrLeaseLost
		}
		if err != nil {
			return fmt.Errorf("lock batch for generation: %w", err)
		}

		var maxID sql.NullInt64
		var considered int
		err = tx.QueryRowContext(ctx, `
			WITH chunk AS (
				SELECT id FROM email_recipients
				WHERE batch_id = $1 AND validation_status <> 'invalid' AND id > $2
				ORDER BY id
				LIMIT $3
			), ins AS (
				INSERT INTO email_jobs (batch_id, recipient_id, max_attempts)
				SELECT $1, id, $4 FROM chunk
				ON CONFLICT (recipient_id) DO NOTHING
				RETURNING 1
			)
			SELECT (SELECT MAX(id) FROM chunk), (SELECT COUNT(*) FROM chunk), (SELECT COUNT(*) FROM ins)`,
			t.BatchID, last, chunkSize, s.maxAttempts.Load()).Scan(&maxID, &considered, &created)
		if err != nil {
			return fmt.Errorf("generate jobs: %w", err)
		}

		if considered == 0 {
			done = true
			_, err = tx.ExecContext(ctx, `
				UPDATE email_batches
				SET job_generation_status = 'completed', job_generation_locked_by = NULL,
				    status = 'queued', updated_at = NOW()
				WHERE id = $1`, t.BatchID)
			if err != nil {
				return fmt.Errorf("finish generation: %w", err)
			}
			return nil
		}
		// The checkpoint is the last recipient considered, not the last job
		// inserted, so a retried chunk does not look at the same rows again.
		_, err = tx.ExecContext(ctx, `
			UPDATE email_batches
			SET last_generated_recipient_id = $2, pending_count = pending_count + $3,
			    job_generation_locked_at = NOW(), updated_at = NOW()
			WHERE id = $1`, t.BatchID, maxID.Int64, created)
		if err != nil {
			return fmt.Errorf("move generation checkpoint: %w", err)
		}
		t.LastRecipientID = maxID.Int64
		return nil
	})
	return created, done, err
}

// FailGeneration gives up on generating a batch's jobs (too many attempts):
// the batch becomes 'failed'. Jobs already created are cancelled so they are
// never sent from a half-built batch.
func (s *Store) FailGeneration(ctx context.Context, batchID int64, reason string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		status, _, _, err := lockBatch(ctx, tx, batchID)
		if err != nil {
			return err
		}
		if status != StatusGeneratingJobs {
			return nil // cancelled meanwhile: nothing to fail
		}
		res, err := tx.ExecContext(ctx, `
			UPDATE email_jobs SET status = 'cancelled', last_error = 'job generation failed', updated_at = NOW()
			WHERE batch_id = $1 AND status = 'pending'`, batchID)
		if err != nil {
			return fmt.Errorf("cancel partial jobs: %w", err)
		}
		n, _ := res.RowsAffected()
		_, err = tx.ExecContext(ctx, `
			UPDATE email_batches
			SET status = 'failed', job_generation_status = 'failed', job_generation_error = $2,
			    job_generation_locked_by = NULL,
			    pending_count = pending_count - $3, cancelled_count = cancelled_count + $3,
			    completed_at = NOW(), updated_at = NOW()
			WHERE id = $1`, batchID, reason, n)
		if err != nil {
			return fmt.Errorf("fail generation: %w", err)
		}
		return nil
	})
}

// ReleaseGeneration hands a generation task back, so it is claimed again and
// resumes from its checkpoint. refund: a clean shutdown, which does not count
// as a failed attempt.
func (s *Store) ReleaseGeneration(ctx context.Context, batchID int64, workerID, reason string, refund bool) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE email_batches
		SET job_generation_status = 'pending', job_generation_error = $3,
		    job_generation_locked_by = NULL, updated_at = NOW(),
		    job_generation_attempts = job_generation_attempts - CASE WHEN $4 THEN 1 ELSE 0 END
		WHERE id = $1 AND job_generation_locked_by = $2 AND status = 'generating_jobs'`,
		batchID, workerID, reason, refund)
	if err != nil {
		return fmt.Errorf("release generation: %w", err)
	}
	return nil
}
