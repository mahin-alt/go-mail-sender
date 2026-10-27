package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
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
	st := New(db)
	if err := st.InitSchema(context.Background()); err != nil {
		t.Fatalf("init schema: %v", err)
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
