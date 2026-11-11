package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ImportJob is a claimed import task: which batch to import and where to
// resume.
type ImportJob struct {
	ID               int64
	BatchID          int64
	BatchType        string
	LastProcessedRow int // checkpoint: rows up to here are already saved
	Attempts         int // including this claim
	MaxAttempts      int
	LastError        string // why the previous attempt stopped, if it did
}

// ImportRow is one checked spreadsheet row, ready to be saved.
type ImportRow struct {
	Row    int
	Name   string
	Email  string
	Status string // RecipientValid, RecipientUnverified or RecipientInvalid
	Reason string // invalid rows only: one of InvalidReasons
}

// ClaimImportJob atomically takes the next import task that is waiting, or
// whose worker stopped renewing its lease (crashed) for longer than lease.
// The claim is a single UPDATE on a row picked with FOR UPDATE SKIP LOCKED,
// so two workers can never claim the same task. Returns ErrNoJobs when there
// is nothing to do.
func (s *Store) ClaimImportJob(ctx context.Context, workerID string, lease time.Duration) (*ImportJob, error) {
	var j ImportJob
	err := s.db.QueryRowContext(ctx, `
		WITH next AS (
			SELECT id FROM email_import_jobs
			WHERE status = 'pending'
			   OR (status = 'processing' AND locked_at < NOW() - $2 * INTERVAL '1 millisecond')
			ORDER BY id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE email_import_jobs i
		SET status = 'processing', attempts = attempts + 1,
		    locked_at = NOW(), locked_by = $1, updated_at = NOW()
		FROM next
		WHERE i.id = next.id
		RETURNING i.id, i.batch_id, i.last_processed_row, i.attempts, i.max_attempts,
		          COALESCE(i.last_error, ''), (SELECT type FROM email_batches WHERE id = i.batch_id)`,
		workerID, ms(lease)).
		Scan(&j.ID, &j.BatchID, &j.LastProcessedRow, &j.Attempts, &j.MaxAttempts, &j.LastError, &j.BatchType)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoJobs
	}
	if err != nil {
		return nil, fmt.Errorf("claim import job: %w", err)
	}
	return &j, nil
}

// RenewImportLease tells others the worker is still alive. ErrLeaseLost if
// another worker has taken the task over.
func (s *Store) RenewImportLease(ctx context.Context, jobID int64, workerID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE email_import_jobs SET locked_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND locked_by = $2 AND status = 'processing'`, jobID, workerID)
	if err != nil {
		return fmt.Errorf("renew import lease: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrLeaseLost
	}
	return nil
}

// ImportChunk is one chunk of rows plus the checkpoint it reaches.
type ImportChunk struct {
	Rows    []ImportRow
	LastRow int // last spreadsheet row read (blank rows included)
	Done    bool
}

// SaveImportChunk saves a chunk of rows, moves the checkpoint and the batch
// counters forward, all in ONE transaction: either everything is saved or
// nothing is, so after a crash the checkpoint says exactly where to resume.
//
// It first checks (and locks) that this worker still owns the task, so a
// worker whose lease expired cannot write over the one that took over.
//
// With Done set, the same transaction also finishes the import: the task
// becomes 'completed' and the batch 'draft' (bulk_send) or 'completed'
// (verification). A file with no data rows at all fails the import instead.
func (s *Store) SaveImportChunk(ctx context.Context, job *ImportJob, workerID string, c ImportChunk) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE email_import_jobs
			SET last_processed_row = $3, locked_at = NOW(), updated_at = NOW()
			WHERE id = $1 AND locked_by = $2 AND status = 'processing'`,
			job.ID, workerID, c.LastRow)
		if err != nil {
			return fmt.Errorf("move import checkpoint: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrLeaseLost
		}

		var valid, unverified, invalid int
		reasons := map[string]int{}
		if len(c.Rows) > 0 {
			nums := make([]int32, len(c.Rows))
			names := make([]string, len(c.Rows))
			emails := make([]string, len(c.Rows))
			statuses := make([]string, len(c.Rows))
			reasonCol := make([]*string, len(c.Rows))
			for i, r := range c.Rows {
				nums[i], names[i], emails[i], statuses[i] = int32(r.Row), r.Name, r.Email, r.Status
				switch r.Status {
				case RecipientValid:
					valid++
				case RecipientUnverified:
					unverified++
				default:
					invalid++
					reasons[r.Reason]++
					reason := r.Reason
					reasonCol[i] = &reason
				}
			}
			// One INSERT for the whole chunk: unnest turns the arrays into rows.
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO email_recipients (batch_id, row_number, name, email, validation_status, validation_reason)
				SELECT $1, * FROM unnest($2::int[], $3::text[], $4::text[], $5::text[], $6::text[])`,
				job.BatchID, nums, names, emails, statuses, reasonCol); err != nil {
				return fmt.Errorf("insert recipients: %w", err)
			}
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE email_batches
			SET total_rows = total_rows + $2, processed_rows = $3,
			    valid_count = valid_count + $4, unverified_count = unverified_count + $5,
			    invalid_count = invalid_count + $6,
			    invalid_counts = jsonb_build_object(
			        'empty_email',   (invalid_counts->>'empty_email')::int + $7,
			        'empty_name',    (invalid_counts->>'empty_name')::int + $8,
			        'invalid_email', (invalid_counts->>'invalid_email')::int + $9,
			        'bad_domain',    (invalid_counts->>'bad_domain')::int + $10),
			    updated_at = NOW()
			WHERE id = $1`,
			job.BatchID, len(c.Rows), c.LastRow, valid, unverified, invalid,
			reasons["empty_email"], reasons["empty_name"], reasons["invalid_email"], reasons["bad_domain"]); err != nil {
			return fmt.Errorf("update batch counters: %w", err)
		}

		if !c.Done {
			return nil
		}
		var total int
		if err := tx.QueryRowContext(ctx, `SELECT total_rows FROM email_batches WHERE id = $1`, job.BatchID).Scan(&total); err != nil {
			return fmt.Errorf("read total rows: %w", err)
		}
		if total == 0 {
			return failImport(ctx, tx, job, "no recipient rows found in the file")
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE email_import_jobs SET status = 'completed', locked_by = NULL, updated_at = NOW()
			WHERE id = $1`, job.ID); err != nil {
			return fmt.Errorf("finish import job: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE email_batches
			SET status = CASE WHEN type = 'verification' THEN 'completed' ELSE 'draft' END,
			    completed_at = CASE WHEN type = 'verification' THEN NOW() ELSE NULL END,
			    updated_at = NOW()
			WHERE id = $1 AND status = 'importing'`, job.BatchID); err != nil {
			return fmt.Errorf("finish batch import: %w", err)
		}
		return nil
	})
}

// FailImport gives up on an import for good (corrupt file, missing columns,
// too many attempts): the task becomes 'failed' and the batch 'import_failed'
// with the reason shown to the user. Rows saved so far stay, but the batch
// can never be sent.
func (s *Store) FailImport(ctx context.Context, job *ImportJob, reason string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error { return failImport(ctx, tx, job, reason) })
}

func failImport(ctx context.Context, tx *sql.Tx, job *ImportJob, reason string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE email_import_jobs SET status = 'failed', last_error = $2, locked_by = NULL, updated_at = NOW()
		WHERE id = $1`, job.ID, reason); err != nil {
		return fmt.Errorf("fail import job: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE email_batches SET status = 'import_failed', import_error = $2, updated_at = NOW()
		WHERE id = $1`, job.BatchID, reason); err != nil {
		return fmt.Errorf("fail batch import: %w", err)
	}
	return nil
}

// ReleaseImport hands a task back: it becomes 'pending' again and is picked
// up from its checkpoint. After a temporary problem (the database went away
// for a moment) the attempt counts; on a clean shutdown (refund) it does not,
// so restarting the server never uses up a file's attempts.
func (s *Store) ReleaseImport(ctx context.Context, job *ImportJob, workerID, reason string, refund bool) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE email_import_jobs
		SET status = 'pending', last_error = $3, locked_by = NULL, updated_at = NOW(),
		    attempts = attempts - CASE WHEN $4 THEN 1 ELSE 0 END
		WHERE id = $1 AND locked_by = $2 AND status = 'processing'`, job.ID, workerID, reason, refund)
	if err != nil {
		return fmt.Errorf("release import job: %w", err)
	}
	return nil
}
