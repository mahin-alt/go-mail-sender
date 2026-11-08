package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

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

// seedBatch creates a batch whose jobs have the given statuses and returns
// its ID and job IDs (in insertion order). The batch is deleted on cleanup
// (jobs cascade).
func seedBatch(t *testing.T, st *Store, statuses ...string) (int64, []int64) {
	t.Helper()
	ctx := context.Background()
	jobs := make([]Job, len(statuses))
	for i, status := range statuses {
		jobs[i] = Job{
			RecipientName:  fmt.Sprintf("Person %d", i),
			RecipientEmail: fmt.Sprintf("person%d@example.com", i),
			Subject:        "s", Body: "b", Status: status,
		}
	}
	batchID, err := st.CreateBatchWithJobs(ctx, "seed.xlsx", "s", "b", jobs)
	if err != nil {
		t.Fatalf("seed batch: %v", err)
	}
	t.Cleanup(func() { st.db.ExecContext(ctx, `DELETE FROM batches WHERE id = $1`, batchID) })

	rows, err := st.db.QueryContext(ctx, `SELECT id FROM email_jobs WHERE batch_id = $1 ORDER BY id`, batchID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return batchID, ids
}

func jobStatus(t *testing.T, st *Store, id int64) string {
	t.Helper()
	var status string
	if err := st.db.QueryRow(`SELECT status FROM email_jobs WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func batchStatus(t *testing.T, st *Store, id int64) string {
	t.Helper()
	var status string
	if err := st.db.QueryRow(`SELECT status FROM batches WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestListBatches(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	items, total, err := st.ListBatches(ctx, BatchListFilter{Limit: 5})
	if err != nil {
		t.Fatalf("ListBatches: %v", err)
	}
	if items == nil {
		t.Fatal("items must be non-nil so it encodes as []")
	}
	if len(items) > 5 || len(items) > total {
		t.Fatalf("got %d items for limit 5, total %d", len(items), total)
	}
	for i := 1; i < len(items); i++ {
		if items[i-1].ID <= items[i].ID {
			t.Fatalf("not newest-first: %d then %d", items[i-1].ID, items[i].ID)
		}
	}

	// A status nothing has must yield an empty page, not an error.
	none, n, err := st.ListBatches(ctx, BatchListFilter{Status: "no-such-status", Limit: 5})
	if err != nil || len(none) != 0 || n != 0 {
		t.Fatalf("unknown status: items=%d total=%d err=%v", len(none), n, err)
	}
}

func TestListBatchJobs_PagingAndSearch(t *testing.T) {
	st := openTestStore(t)
	batchID, _ := seedBatch(t, st, "sent", "failed", "sent", "invalid")
	ctx := context.Background()

	page, total, err := st.ListBatchJobs(ctx, batchID, JobListFilter{Limit: 2})
	if err != nil || total != 4 || len(page) != 2 {
		t.Fatalf("page 1: n=%d total=%d err=%v", len(page), total, err)
	}
	page2, _, _ := st.ListBatchJobs(ctx, batchID, JobListFilter{Limit: 2, Offset: 2})
	if len(page2) != 2 || page2[0].ID <= page[1].ID {
		t.Fatalf("page 2 should continue after page 1")
	}

	failed, total, _ := st.ListBatchJobs(ctx, batchID, JobListFilter{Status: "failed", Limit: 10})
	if total != 1 || len(failed) != 1 {
		t.Fatalf("status filter: n=%d total=%d", len(failed), total)
	}

	found, total, _ := st.ListBatchJobs(ctx, batchID, JobListFilter{Search: "PERSON3@", Limit: 10})
	if total != 1 || len(found) != 1 || found[0].RecipientEmail != "person3@example.com" {
		t.Fatalf("search: n=%d total=%d", len(found), total)
	}
}

func TestRetryFailedJobs_Batch(t *testing.T) {
	st := openTestStore(t)
	batchID, ids := seedBatch(t, st, "sent", "failed", "failed", "invalid")
	ctx := context.Background()

	if got := batchStatus(t, st, batchID); got != "pending" {
		// jobs were seeded without any 'pending' row, so the batch starts completed
		if got != "completed" {
			t.Fatalf("unexpected seed batch status %q", got)
		}
	}

	n, err := st.RetryFailedJobs(ctx, batchID, nil)
	if err != nil || n != 2 {
		t.Fatalf("retried %d, err %v; want 2", n, err)
	}
	if jobStatus(t, st, ids[1]) != "pending" || jobStatus(t, st, ids[2]) != "pending" {
		t.Fatal("failed jobs should be pending again")
	}
	if jobStatus(t, st, ids[0]) != "sent" || jobStatus(t, st, ids[3]) != "invalid" {
		t.Fatal("sent and invalid jobs must be left alone")
	}
	if got := batchStatus(t, st, batchID); got != "pending" {
		t.Fatalf("batch should be reopened, status = %q", got)
	}

	// Nothing failed any more: succeeds with zero, not an error.
	if n, err := st.RetryFailedJobs(ctx, batchID, nil); err != nil || n != 0 {
		t.Fatalf("second retry: n=%d err=%v", n, err)
	}
	if _, err := st.RetryFailedJobs(ctx, 999999999, nil); !errors.Is(err, ErrBatchNotFound) {
		t.Fatalf("unknown batch: err = %v, want ErrBatchNotFound", err)
	}
}

func TestRetryFailedJob_Single(t *testing.T) {
	st := openTestStore(t)
	_, ids := seedBatch(t, st, "failed", "failed", "sent")
	ctx := context.Background()

	if n, err := st.RetryFailedJob(ctx, ids[0]); err != nil || n != 1 {
		t.Fatalf("retry one: n=%d err=%v", n, err)
	}
	if jobStatus(t, st, ids[0]) != "pending" || jobStatus(t, st, ids[1]) != "failed" {
		t.Fatal("only the chosen job should be requeued")
	}
	if _, err := st.RetryFailedJob(ctx, ids[2]); !errors.Is(err, ErrJobNotRetryable) {
		t.Fatalf("sent job: err = %v, want ErrJobNotRetryable", err)
	}
}

func TestMarkJob_ReportsBatchCompletionOnce(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	batchID, ids := seedBatch(t, st, "pending", "pending", "invalid")

	if done, err := st.MarkJobSent(ctx, ids[0]); err != nil || done {
		t.Fatalf("first of two: done=%v err=%v, want false", done, err)
	}
	if done, err := st.MarkJobFailed(ctx, ids[1], "boom", 0); err != nil || done {
		t.Fatalf("failure with attempts left (job back to pending): done=%v err=%v, want false", done, err)
	}
	// Use up the remaining attempts so the next failure is final.
	if _, err := st.db.ExecContext(ctx, `UPDATE email_jobs SET attempts = max_attempts WHERE id = $1`, ids[1]); err != nil {
		t.Fatal(err)
	}
	if done, err := st.MarkJobFailed(ctx, ids[1], "boom", 0); err != nil || !done {
		t.Fatalf("last job finished: done=%v err=%v, want true", done, err)
	}
	if got := batchStatus(t, st, batchID); got != "completed" {
		t.Fatalf("batch status = %q, want completed", got)
	}
	bp, err := st.GetBatchProgress(ctx, batchID)
	if err != nil || bp.CreatedAt.IsZero() || bp.CompletedAt == nil || bp.CompletedAt.Before(bp.CreatedAt) {
		t.Fatalf("progress times: %+v err=%v, want created_at and a later completed_at", bp, err)
	}
	if done, err := st.checkBatchCompletion(ctx, batchID); err != nil || done {
		t.Fatalf("already completed: done=%v err=%v, want false (report only once)", done, err)
	}
}

func TestClaimNextJob_ReportsQueueTime(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	_, ids := seedBatch(t, st, "pending")
	if _, err := st.db.ExecContext(ctx, `UPDATE email_jobs SET run_at = NOW() - INTERVAL '2 seconds' WHERE id = $1`, ids[0]); err != nil {
		t.Fatal(err)
	}

	job, err := st.ClaimNextJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if job.ID != ids[0] {
		t.Fatalf("claimed job %d, want %d (leftover pending jobs in the test schema?)", job.ID, ids[0])
	}
	if job.QueuedFor < 2*time.Second || job.QueuedFor > time.Minute {
		t.Fatalf("QueuedFor = %v, want about 2s", job.QueuedFor)
	}
}

func TestCancelBatch(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	batchID, ids := seedBatch(t, st, "pending", "pending", "sent", "invalid", "processing", "failed")

	n, err := st.CancelBatch(ctx, batchID)
	if err != nil || n != 2 {
		t.Fatalf("CancelBatch = %d, %v; want 2 pending jobs cancelled", n, err)
	}
	want := []string{"cancelled", "cancelled", "sent", "invalid", "processing", "failed"}
	for i, id := range ids {
		if got := jobStatus(t, st, id); got != want[i] {
			t.Errorf("job %d status = %q, want %q", i, got, want[i])
		}
	}
	bp, err := st.GetBatchProgress(ctx, batchID)
	if err != nil || bp.Status != "cancelled" || bp.CancelledAt == nil || bp.Cancelled != 2 || bp.Processing != 1 {
		t.Fatalf("progress = %+v err=%v, want cancelled with cancelled_at, 2 cancelled, 1 processing", bp, err)
	}

	// The email that was mid-send finishes; the batch must stay cancelled.
	if done, err := st.MarkJobSent(ctx, ids[4]); err != nil || done {
		t.Fatalf("in-flight job finishing: done=%v err=%v, want false", done, err)
	}
	if got := batchStatus(t, st, batchID); got != "cancelled" {
		t.Fatalf("batch status after in-flight send = %q, want cancelled", got)
	}

	if _, err := st.CancelBatch(ctx, batchID); !errors.Is(err, ErrBatchNotActive) {
		t.Fatalf("second cancel: err = %v, want ErrBatchNotActive", err)
	}
	if _, err := st.RetryFailedJobs(ctx, batchID, nil); !errors.Is(err, ErrBatchNotActive) {
		t.Fatalf("retry on cancelled batch: err = %v, want ErrBatchNotActive", err)
	}
	if _, err := st.RetryFailedJob(ctx, ids[5]); !errors.Is(err, ErrBatchNotActive) {
		t.Fatalf("retry one job of a cancelled batch: err = %v, want ErrBatchNotActive", err)
	}
	if _, err := st.CancelBatch(ctx, 999999999); !errors.Is(err, ErrBatchNotFound) {
		t.Fatalf("unknown batch: err = %v, want ErrBatchNotFound", err)
	}
}

func TestCancelBatch_FailureDuringCancelIsNotRetried(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	batchID, ids := seedBatch(t, st, "processing", "pending")
	if _, err := st.CancelBatch(ctx, batchID); err != nil {
		t.Fatal(err)
	}
	// The in-flight email fails with attempts left: normally it would go back to pending.
	if _, err := st.MarkJobFailed(ctx, ids[0], "connection reset", 0); err != nil {
		t.Fatal(err)
	}
	if got := jobStatus(t, st, ids[0]); got != "cancelled" {
		t.Fatalf("job status = %q, want cancelled (never requeued after a cancel)", got)
	}
}

func TestCancelBatch_FinishedBatchIsNotActive(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	batchID, _ := seedBatch(t, st, "invalid") // nothing to send: created as completed
	if _, err := st.CancelBatch(ctx, batchID); !errors.Is(err, ErrBatchNotActive) {
		t.Fatalf("err = %v, want ErrBatchNotActive", err)
	}
}

func TestStatusCheckConstraints(t *testing.T) {
	st := openTestStore(t)
	_, ids := seedBatch(t, st, "pending")
	if _, err := st.db.Exec(`UPDATE email_jobs SET status = 'canceled' WHERE id = $1`, ids[0]); err == nil {
		t.Fatal("a misspelled status should be rejected by the database")
	}
}
