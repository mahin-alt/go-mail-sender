package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	"sendmail/internal/migrations"
)

// openTestStore connects to TEST_DATABASE_URL, or skips the test when unset.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Some tests wipe tables, so refuse to touch the real "public" schema of a
	// database that is not clearly a test database.
	var schema, dbname string
	if err := db.QueryRow(`SELECT current_schema(), current_database()`).Scan(&schema, &dbname); err != nil {
		t.Fatalf("inspect db: %v", err)
	}
	if schema == "public" && !strings.Contains(dbname, "test") {
		t.Skipf("refusing to run: TEST_DATABASE_URL points at the public schema of %q; use a scratch schema or a *test* database", dbname)
	}

	st := New(db)
	if _, err := migrations.Up(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return st
}

// seedBatch builds a bulk batch directly in the database: one recipient per
// status, and an email job for every status except "invalid" (an invalid
// recipient gets no email). Counters are recounted, and the batch status is
// 'sending' while any email is pending or processing, otherwise finished.
// Returns the batch ID and the job IDs (in order, invalid rows skipped).
func seedBatch(t *testing.T, st *Store, statuses ...string) (int64, []int64) {
	t.Helper()
	ctx := context.Background()
	var batchID int64
	if err := st.db.QueryRowContext(ctx, `
		INSERT INTO email_batches (type, original_filename, status, subject_template, body_template,
		                           job_generation_status, send_requested_at)
		VALUES ('bulk_send', 'seed.xlsx', 'sending', 'Hi {{.Name}}', 'Hello {{.Name}}', 'completed', NOW())
		RETURNING id`).Scan(&batchID); err != nil {
		t.Fatalf("seed batch: %v", err)
	}
	t.Cleanup(func() { st.db.ExecContext(ctx, `DELETE FROM email_batches WHERE id = $1`, batchID) })

	var jobIDs []int64
	active := false
	for i, status := range statuses {
		vstatus, reason := "valid", sql.NullString{}
		if status == "invalid" {
			vstatus, reason = "invalid", sql.NullString{String: "invalid_email", Valid: true}
		}
		var rid int64
		if err := st.db.QueryRowContext(ctx, `
			INSERT INTO email_recipients (batch_id, row_number, name, email, validation_status, validation_reason)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			batchID, i+2, fmt.Sprintf("Person %d", i), fmt.Sprintf("person%d@example.com", i), vstatus, reason).Scan(&rid); err != nil {
			t.Fatalf("seed recipient: %v", err)
		}
		if status == "invalid" {
			continue
		}
		if status == "pending" || status == "processing" {
			active = true
		}
		var jid int64
		if err := st.db.QueryRowContext(ctx, `
			INSERT INTO email_jobs (batch_id, recipient_id, status, locked_at, locked_by)
			VALUES ($1, $2, $3, CASE WHEN $3 = 'processing' THEN NOW() END, CASE WHEN $3 = 'processing' THEN 'seed' END)
			RETURNING id`, batchID, rid, status).Scan(&jid); err != nil {
			t.Fatalf("seed job: %v", err)
		}
		jobIDs = append(jobIDs, jid)
	}
	if _, err := st.ReconcileBatchCounters(ctx, batchID); err != nil {
		t.Fatal(err)
	}
	if !active {
		if _, err := st.db.ExecContext(ctx, `
			UPDATE email_batches
			SET status = CASE WHEN failed_count > 0 THEN 'completed_with_errors' ELSE 'completed' END,
			    completed_at = NOW()
			WHERE id = $1`, batchID); err != nil {
			t.Fatal(err)
		}
	}
	return batchID, jobIDs
}

// assertCountersMatch fails when a batch's stored counters differ from a
// fresh count of its rows (they must always be kept in step).
func assertCountersMatch(t *testing.T, st *Store, batchID int64) {
	t.Helper()
	ctx := context.Background()
	before, err := st.GetBatch(ctx, batchID)
	if err != nil {
		t.Fatal(err)
	}
	drift, err := st.ReconcileBatchCounters(ctx, batchID)
	if err != nil {
		t.Fatal(err)
	}
	if drift {
		after, _ := st.GetBatch(ctx, batchID)
		t.Fatalf("batch %d counters drifted:\n stored  %+v\n counted %+v", batchID, counters(before), counters(after))
	}
}

func counters(b *Batch) string {
	return fmt.Sprintf("rows=%d valid=%d unverified=%d invalid=%d %v pending=%d processing=%d sent=%d failed=%d cancelled=%d",
		b.TotalRows, b.ValidCount, b.UnverifiedCount, b.InvalidCount, b.InvalidCounts,
		b.PendingCount, b.ProcessingCount, b.SentCount, b.FailedCount, b.CancelledCount)
}
