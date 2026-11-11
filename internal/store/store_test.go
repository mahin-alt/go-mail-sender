package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// newImportedBatch creates a batch and runs its import through the real
// store calls with the given rows, in chunks of chunkSize.
func newImportedBatch(t *testing.T, st *Store, batchType string, rows []ImportRow, chunkSize int) int64 {
	t.Helper()
	ctx := context.Background()
	id, err := st.CreateBatch(ctx, batchType, "people.xlsx", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.db.ExecContext(ctx, `DELETE FROM email_batches WHERE id = $1`, id) })

	job := claimImportFor(t, st, id, "w1")
	for i := 0; i < len(rows) || i == 0; i += chunkSize {
		end := min(i+chunkSize, len(rows))
		c := ImportChunk{Rows: rows[i:end], Done: end == len(rows)}
		if end > 0 {
			c.LastRow = rows[end-1].Row
		}
		if err := st.SaveImportChunk(ctx, job, "w1", c); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

// claimImportFor claims import tasks until it gets the one for batchID
// (other tests' leftovers are released again).
func claimImportFor(t *testing.T, st *Store, batchID int64, worker string) *ImportJob {
	t.Helper()
	ctx := context.Background()
	if _, err := st.db.ExecContext(ctx, `UPDATE email_import_jobs SET status = 'failed' WHERE batch_id <> $1 AND status IN ('pending', 'processing')`, batchID); err != nil {
		t.Fatal(err)
	}
	job, err := st.ClaimImportJob(ctx, worker, time.Minute)
	if err != nil {
		t.Fatalf("claim import: %v", err)
	}
	if job.BatchID != batchID {
		t.Fatalf("claimed batch %d, want %d", job.BatchID, batchID)
	}
	return job
}

func people(n int) []ImportRow {
	out := make([]ImportRow, n)
	for i := range out {
		out[i] = ImportRow{Row: i + 2, Name: fmt.Sprintf("P%d", i), Email: fmt.Sprintf("p%d@example.com", i), Status: RecipientValid}
	}
	return out
}

func TestImport_ChunksCountersAndFinish(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	rows := people(7)
	rows[1] = ImportRow{Row: 3, Name: "", Email: "x@example.com", Status: RecipientInvalid, Reason: "empty_name"}
	rows[3] = ImportRow{Row: 5, Name: "U", Email: "u@slow.example", Status: RecipientUnverified}
	rows[5] = ImportRow{Row: 7, Name: "B", Email: "b@dead.example", Status: RecipientInvalid, Reason: "bad_domain"}

	id := newImportedBatch(t, st, TypeBulkSend, rows, 3)
	b, err := st.GetBatch(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != StatusDraft || b.TotalRows != 7 || b.ValidCount != 4 || b.UnverifiedCount != 1 || b.InvalidCount != 2 {
		t.Fatalf("batch = %s status %s", counters(b), b.Status)
	}
	if b.InvalidCounts["empty_name"] != 1 || b.InvalidCounts["bad_domain"] != 1 || b.InvalidCounts["empty_email"] != 0 {
		t.Fatalf("invalid_counts = %v", b.InvalidCounts)
	}
	assertCountersMatch(t, st, id)

	var status string
	var last int
	st.db.QueryRow(`SELECT status, last_processed_row FROM email_import_jobs WHERE batch_id = $1`, id).Scan(&status, &last)
	if status != "completed" || last != 8 {
		t.Fatalf("import job = %s at row %d", status, last)
	}

	// Issues: invalid rows in order, one reason, unverified, search, cursor.
	inv, _ := st.ListRecipients(ctx, id, RecipientFilter{Status: RecipientInvalid, Limit: 10})
	if len(inv) != 2 || inv[0].Row != 3 || inv[1].Row != 7 || *inv[1].ValidationReason != "bad_domain" {
		t.Fatalf("invalid rows = %+v", inv)
	}
	bad, _ := st.ListRecipients(ctx, id, RecipientFilter{Status: RecipientInvalid, Reasons: []string{"bad_domain"}, Limit: 10})
	if len(bad) != 1 || bad[0].Row != 7 {
		t.Fatalf("bad_domain rows = %+v", bad)
	}
	two, _ := st.ListRecipients(ctx, id, RecipientFilter{Status: RecipientInvalid, Reasons: []string{"bad_domain", "empty_name"}, Limit: 10})
	if len(two) != 2 {
		t.Fatalf("two reasons = %+v", two)
	}
	unv, _ := st.ListRecipients(ctx, id, RecipientFilter{Status: RecipientUnverified, Limit: 10})
	if len(unv) != 1 || unv[0].Email != "u@slow.example" {
		t.Fatalf("unverified rows = %+v", unv)
	}
	page2, _ := st.ListRecipients(ctx, id, RecipientFilter{Status: RecipientInvalid, After: inv[0].ID, Limit: 10})
	if len(page2) != 1 || page2[0].Row != 7 {
		t.Fatalf("after cursor = %+v", page2)
	}
	found, _ := st.ListRecipients(ctx, id, RecipientFilter{Status: Sendable, Search: "P4", Limit: 10})
	if len(found) != 1 || found[0].Name != "P4" {
		t.Fatalf("search = %+v", found)
	}
	sample, _ := st.ListRecipients(ctx, id, RecipientFilter{Status: Sendable, Limit: 3})
	if len(sample) != 3 || sample[0].Row != 2 || sample[1].Row != 4 || sample[2].Row != 5 {
		t.Fatalf("sample = %+v", sample)
	}
	if _, err := st.ListRecipients(ctx, id, RecipientFilter{Status: "nope", Limit: 1}); err == nil {
		t.Fatal("unknown status must be an error")
	}
}

func TestImport_VerificationEndsCompleted(t *testing.T) {
	st := openTestStore(t)
	id := newImportedBatch(t, st, TypeVerification, people(2), 10)
	b, _ := st.GetBatch(context.Background(), id)
	if b.Status != StatusCompleted || b.CompletedAt == nil {
		t.Fatalf("verification batch = %s, completed_at %v", b.Status, b.CompletedAt)
	}
}

func TestImport_EmptyFileFails(t *testing.T) {
	st := openTestStore(t)
	id := newImportedBatch(t, st, TypeBulkSend, nil, 10)
	b, _ := st.GetBatch(context.Background(), id)
	if b.Status != StatusImportFailed || b.ImportError == nil || !strings.Contains(*b.ImportError, "no recipient rows") {
		t.Fatalf("empty import = %s %v", b.Status, b.ImportError)
	}
}

func TestImport_SameRowTwiceIsRejected(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	id, _ := st.CreateBatch(ctx, TypeBulkSend, "f.xlsx", nil)
	t.Cleanup(func() { st.db.Exec(`DELETE FROM email_batches WHERE id = $1`, id) })
	job := claimImportFor(t, st, id, "w1")
	if err := st.SaveImportChunk(ctx, job, "w1", ImportChunk{Rows: people(2), LastRow: 3}); err != nil {
		t.Fatal(err)
	}
	// The unique (batch_id, row_number) index is the second line of defence.
	if err := st.SaveImportChunk(ctx, job, "w1", ImportChunk{Rows: people(1), LastRow: 3}); err == nil {
		t.Fatal("saving row 2 again must fail")
	}
	// The failed chunk changed nothing, not even the checkpoint.
	b, _ := st.GetBatch(ctx, id)
	if b.TotalRows != 2 {
		t.Fatalf("total rows = %d, want 2", b.TotalRows)
	}
	assertCountersMatch(t, st, id)
}

// A worker that crashes mid-import: its lease expires, another worker takes
// over from the checkpoint, and the first worker can no longer write.
func TestImport_LeaseExpiryAndResume(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	id, _ := st.CreateBatch(ctx, TypeBulkSend, "f.xlsx", nil)
	t.Cleanup(func() { st.db.Exec(`DELETE FROM email_batches WHERE id = $1`, id) })

	rows := people(6)
	job1 := claimImportFor(t, st, id, "w1")
	if err := st.SaveImportChunk(ctx, job1, "w1", ImportChunk{Rows: rows[:3], LastRow: 4}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ClaimImportJob(ctx, "w2", time.Minute); !errors.Is(err, ErrNoJobs) {
		t.Fatalf("a live lease must not be claimable: %v", err)
	}

	// w1 "crashes": its lease goes stale.
	st.db.Exec(`UPDATE email_import_jobs SET locked_at = NOW() - INTERVAL '10 minutes' WHERE id = $1`, job1.ID)
	job2, err := st.ClaimImportJob(ctx, "w2", time.Minute)
	if err != nil || job2.ID != job1.ID || job2.LastProcessedRow != 4 || job2.Attempts != 2 {
		t.Fatalf("takeover = %+v, %v", job2, err)
	}
	if err := st.SaveImportChunk(ctx, job1, "w1", ImportChunk{Rows: rows[3:], LastRow: 7}); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("old worker must lose its lease: %v", err)
	}
	if err := st.RenewImportLease(ctx, job1.ID, "w1"); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("old worker renew: %v", err)
	}
	if err := st.RenewImportLease(ctx, job2.ID, "w2"); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveImportChunk(ctx, job2, "w2", ImportChunk{Rows: rows[3:], LastRow: 7, Done: true}); err != nil {
		t.Fatal(err)
	}
	b, _ := st.GetBatch(ctx, id)
	if b.Status != StatusDraft || b.TotalRows != 6 {
		t.Fatalf("after resume: %s %s", b.Status, counters(b))
	}
	assertCountersMatch(t, st, id)
}

func TestImport_FailAndRelease(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	id, _ := st.CreateBatch(ctx, TypeVerification, "f.xlsx", nil)
	t.Cleanup(func() { st.db.Exec(`DELETE FROM email_batches WHERE id = $1`, id) })

	job := claimImportFor(t, st, id, "w1")
	if err := st.ReleaseImport(ctx, job, "w1", "db hiccup", false); err != nil {
		t.Fatal(err)
	}
	again := claimImportFor(t, st, id, "w2")
	if again.Attempts != 2 || again.LastError != "db hiccup" {
		t.Fatalf("attempts = %d (%q), want 2 after a counted release", again.Attempts, again.LastError)
	}
	if err := st.ReleaseImport(ctx, again, "w2", "shutting down", true); err != nil {
		t.Fatal(err)
	}
	again = claimImportFor(t, st, id, "w2")
	if again.Attempts != 2 {
		t.Fatalf("attempts = %d, want 2: a shutdown release is refunded", again.Attempts)
	}
	if err := st.FailImport(ctx, again, "sheet must have name and email columns"); err != nil {
		t.Fatal(err)
	}
	b, _ := st.GetBatch(ctx, id)
	if b.Status != StatusImportFailed || *b.ImportError != "sheet must have name and email columns" {
		t.Fatalf("failed import = %s %v", b.Status, b.ImportError)
	}
	if err := st.RequestSend(ctx, id, "s", "b"); !errors.Is(err, ErrNotSendable) {
		t.Fatalf("a failed import cannot be sent: %v", err)
	}
}

func TestRequestSend_Rules(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	draft := newImportedBatch(t, st, TypeBulkSend, people(3), 10)
	if err := st.RequestSend(ctx, draft, "Hi {{.Name}}", "Body"); err != nil {
		t.Fatal(err)
	}
	b, _ := st.GetBatch(ctx, draft)
	if b.Status != StatusGeneratingJobs || *b.JobGenerationStatus != "pending" || b.SubjectTemplate != "Hi {{.Name}}" || b.SendRequestedAt == nil {
		t.Fatalf("after send: %+v", b)
	}
	// Double click: the second request is refused, nothing starts twice.
	if err := st.RequestSend(ctx, draft, "x", "y"); !errors.Is(err, ErrNotSendable) {
		t.Fatalf("second send: %v", err)
	}

	// Verify first, send later: a completed verification batch can be sent
	// and becomes a bulk_send batch.
	ver := newImportedBatch(t, st, TypeVerification, people(2), 10)
	if err := st.RequestSend(ctx, ver, "s", "b"); err != nil {
		t.Fatal(err)
	}
	if b, _ := st.GetBatch(ctx, ver); b.Type != TypeBulkSend || b.CompletedAt != nil {
		t.Fatalf("sent verification batch = %s completed_at %v", b.Type, b.CompletedAt)
	}

	allBad := newImportedBatch(t, st, TypeBulkSend, []ImportRow{{Row: 2, Email: "x", Status: RecipientInvalid, Reason: "invalid_email"}}, 10)
	if err := st.RequestSend(ctx, allBad, "s", "b"); !errors.Is(err, ErrNothingToSend) {
		t.Fatalf("nothing to send: %v", err)
	}

	importing, _ := st.CreateBatch(ctx, TypeBulkSend, "f.xlsx", nil)
	t.Cleanup(func() { st.db.Exec(`DELETE FROM email_batches WHERE id = $1`, importing) })
	if err := st.RequestSend(ctx, importing, "s", "b"); !errors.Is(err, ErrNotSendable) {
		t.Fatalf("still importing: %v", err)
	}
	if err := st.RequestSend(ctx, 999999999, "s", "b"); !errors.Is(err, ErrBatchNotFound) {
		t.Fatalf("unknown batch: %v", err)
	}
}

// claimGenerationFor claims generation tasks until it gets batchID's.
func claimGenerationFor(t *testing.T, st *Store, batchID int64, worker string) *GenerationTask {
	t.Helper()
	st.db.Exec(`UPDATE email_batches SET status = 'failed' WHERE id <> $1 AND status = 'generating_jobs'`, batchID)
	task, err := st.ClaimGeneration(context.Background(), worker, time.Minute)
	if err != nil {
		t.Fatalf("claim generation: %v", err)
	}
	if task.BatchID != batchID {
		t.Fatalf("claimed %d, want %d", task.BatchID, batchID)
	}
	return task
}

// sendReadyBatch imports n valid rows (plus one invalid), requests sending
// and generates every job.
func sendReadyBatch(t *testing.T, st *Store, n int) int64 {
	t.Helper()
	ctx := context.Background()
	// Keep other tests' leftovers out of the claim queue.
	st.db.Exec(`UPDATE email_batches SET status = 'completed' WHERE status IN ('queued', 'sending')`)
	rows := append(people(n), ImportRow{Row: n + 2, Email: "bad", Status: RecipientInvalid, Reason: "invalid_email"})
	id := newImportedBatch(t, st, TypeBulkSend, rows, 100)
	if err := st.RequestSend(ctx, id, "Hi {{.Name}}", "Hello {{.Email}}"); err != nil {
		t.Fatal(err)
	}
	task := claimGenerationFor(t, st, id, "g1")
	for {
		_, done, err := st.GenerateChunk(ctx, task, "g1", 4)
		if err != nil {
			t.Fatal(err)
		}
		if done {
			return id
		}
	}
}

func TestGeneration_ChunksResumeAndQueue(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	rows := people(10)
	rows[4] = ImportRow{Row: 6, Email: "bad", Status: RecipientInvalid, Reason: "invalid_email"}
	rows[7].Status = RecipientUnverified
	id := newImportedBatch(t, st, TypeBulkSend, rows, 100)
	if err := st.RequestSend(ctx, id, "s", "b"); err != nil {
		t.Fatal(err)
	}

	task := claimGenerationFor(t, st, id, "g1")
	created, done, err := st.GenerateChunk(ctx, task, "g1", 4)
	if err != nil || created != 4 || done {
		t.Fatalf("first chunk: %d %v %v", created, done, err)
	}
	// Workers must not touch a batch that is still generating.
	if _, err := st.ClaimNextJob(ctx, "s1"); !errors.Is(err, ErrNoJobs) {
		t.Fatalf("claim during generation: %v", err)
	}

	// g1 crashes; g2 takes over after the lease and resumes at the checkpoint.
	st.db.Exec(`UPDATE email_batches SET job_generation_locked_at = NOW() - INTERVAL '10 minutes' WHERE id = $1`, id)
	task2 := claimGenerationFor(t, st, id, "g2")
	if task2.LastRecipientID != task.LastRecipientID || task2.Attempts != 2 {
		t.Fatalf("takeover = %+v (was %+v)", task2, task)
	}
	if _, _, err := st.GenerateChunk(ctx, task, "g1", 4); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("old generator must lose its lease: %v", err)
	}
	// Pretend the checkpoint was lost, so the same rows are offered again:
	// the UNIQUE recipient_id makes the retry harmless.
	st.db.Exec(`UPDATE email_batches SET last_generated_recipient_id = 0 WHERE id = $1`, id)
	total := 0
	for {
		n, done, err := st.GenerateChunk(ctx, task2, "g2", 4)
		if err != nil {
			t.Fatal(err)
		}
		total += n
		if done {
			break
		}
	}
	if total != 5 {
		t.Fatalf("second generator created %d, want 5 (4 already existed)", total)
	}
	var jobs int
	st.db.QueryRow(`SELECT COUNT(*) FROM email_jobs WHERE batch_id = $1`, id).Scan(&jobs)
	b, _ := st.GetBatch(ctx, id)
	if jobs != 9 || b.Status != StatusQueued || *b.JobGenerationStatus != "completed" || b.PendingCount != 9 {
		t.Fatalf("jobs = %d, batch %s %s", jobs, b.Status, counters(b))
	}
	assertCountersMatch(t, st, id)
}

func TestGeneration_Fail(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	id := newImportedBatch(t, st, TypeBulkSend, people(6), 100)
	st.RequestSend(ctx, id, "s", "b")
	task := claimGenerationFor(t, st, id, "g1")
	st.GenerateChunk(ctx, task, "g1", 4)
	if err := st.FailGeneration(ctx, id, "gave up"); err != nil {
		t.Fatal(err)
	}
	b, _ := st.GetBatch(ctx, id)
	if b.Status != StatusFailed || b.CancelledCount != 4 || b.PendingCount != 0 {
		t.Fatalf("failed generation: %s %s", b.Status, counters(b))
	}
	assertCountersMatch(t, st, id)
}

func TestClaimAndMark_FullLifecycle(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	id := sendReadyBatch(t, st, 3)

	j, err := st.ClaimNextJob(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if j.BatchID != id || j.Name != "P0" || j.Email != "p0@example.com" || j.SubjectTemplate != "Hi {{.Name}}" || j.Attempts != 1 {
		t.Fatalf("claimed = %+v", j)
	}
	b, _ := st.GetBatch(ctx, id)
	if b.Status != StatusSending || b.ProcessingCount != 1 || b.PendingCount != 2 {
		t.Fatalf("after claim: %s %s", b.Status, counters(b))
	}
	if _, err := st.MarkJobSent(ctx, j.ID, "someone-else"); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("only the owner may record the result: %v", err)
	}
	if done, err := st.MarkJobSent(ctx, j.ID, "s1"); err != nil || done {
		t.Fatalf("sent: %v %v", done, err)
	}

	// Second email fails with a retry left: back to pending, due later.
	j2, _ := st.ClaimNextJob(ctx, "s1")
	if done, err := st.MarkJobFailed(ctx, j2.ID, "s1", "421 try later", true, time.Hour); err != nil || done {
		t.Fatalf("retry later: %v %v", done, err)
	}
	// Third email fails permanently (retry false = permanent error).
	j3, _ := st.ClaimNextJob(ctx, "s1")
	if j3 == nil || j3.ID == j2.ID {
		t.Fatalf("the delayed retry must not be claimed yet: %+v", j3)
	}
	if done, err := st.MarkJobFailed(ctx, j3.ID, "s1", "550 no such user", false, 0); err != nil || done {
		t.Fatalf("permanent failure: %v %v", done, err)
	}
	if _, err := st.ClaimNextJob(ctx, "s1"); !errors.Is(err, ErrNoJobs) {
		t.Fatalf("nothing due now: %v", err)
	}

	// Make the retry due, send it: that finishes the batch, with errors.
	st.db.Exec(`UPDATE email_jobs SET available_at = NOW() WHERE id = $1`, j2.ID)
	j2b, err := st.ClaimNextJob(ctx, "s2")
	if err != nil || j2b.ID != j2.ID || j2b.Attempts != 2 {
		t.Fatalf("retry claim = %+v %v", j2b, err)
	}
	done, err := st.MarkJobSent(ctx, j2b.ID, "s2")
	if err != nil || !done {
		t.Fatalf("last email must finish the batch: %v %v", done, err)
	}
	b, _ = st.GetBatch(ctx, id)
	if b.Status != StatusCompletedWithErrors || b.SentCount != 2 || b.FailedCount != 1 || b.CompletedAt == nil {
		t.Fatalf("finished: %s %s", b.Status, counters(b))
	}
	assertCountersMatch(t, st, id)

	// Delivery list: cursor paging, status filter, search.
	all, _ := st.ListBatchJobs(ctx, id, JobListFilter{Limit: 2})
	if len(all) != 2 || all[0].Row != 2 || all[0].Email != "p0@example.com" {
		t.Fatalf("page 1 = %+v", all)
	}
	rest, _ := st.ListBatchJobs(ctx, id, JobListFilter{After: all[1].ID, Limit: 2})
	if len(rest) != 1 {
		t.Fatalf("page 2 = %+v", rest)
	}
	failed, _ := st.ListBatchJobs(ctx, id, JobListFilter{Status: JobFailed, Limit: 10})
	if len(failed) != 1 || failed[0].LastError == nil || *failed[0].LastError != "550 no such user" {
		t.Fatalf("failed = %+v", failed)
	}
	hit, _ := st.ListBatchJobs(ctx, id, JobListFilter{Search: "P1", Limit: 10})
	if len(hit) != 1 || hit[0].Name != "P1" {
		t.Fatalf("search = %+v", hit)
	}

	// Retry the failed one, send it: completed without errors now.
	n, err := st.RetryFailedJob(ctx, j3.ID)
	if err != nil || n != 1 {
		t.Fatalf("retry job: %d %v", n, err)
	}
	if b, _ := st.GetBatch(ctx, id); b.Status != StatusSending || b.CompletedAt != nil {
		t.Fatalf("retry must reopen the batch: %s", b.Status)
	}
	if _, err := st.RetryFailedJob(ctx, j3.ID); !errors.Is(err, ErrJobNotRetryable) {
		t.Fatalf("retrying a pending job: %v", err)
	}
	j3b, _ := st.ClaimNextJob(ctx, "s1")
	if done, _ := st.MarkJobSent(ctx, j3b.ID, "s1"); !done {
		t.Fatal("the retried email must finish the batch")
	}
	if b, _ := st.GetBatch(ctx, id); b.Status != StatusCompleted {
		t.Fatalf("status = %s, want completed", b.Status)
	}
	assertCountersMatch(t, st, id)
}

// Several workers claiming at once never get the same email.
func TestClaimNextJob_ConcurrentWorkersNeverShareAJob(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	st.db.Exec(`UPDATE email_batches SET status = 'completed' WHERE status IN ('queued', 'sending')`)
	id := sendReadyBatch(t, st, 120)

	var mu sync.Mutex
	seen := map[int64]string{}
	var wg sync.WaitGroup
	for w := range 6 {
		wg.Add(1)
		go func(worker string) {
			defer wg.Done()
			for {
				j, err := st.ClaimNextJob(ctx, worker)
				if errors.Is(err, ErrNoJobs) {
					return
				}
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				if other, dup := seen[j.ID]; dup {
					t.Errorf("job %d claimed by %s and %s", j.ID, other, worker)
				}
				seen[j.ID] = worker
				mu.Unlock()
				if _, err := st.MarkJobSent(ctx, j.ID, worker); err != nil {
					t.Error(err)
					return
				}
			}
		}(fmt.Sprintf("w%d", w))
	}
	wg.Wait()
	if len(seen) != 120 {
		t.Fatalf("sent %d distinct emails, want 120", len(seen))
	}
	b, _ := st.GetBatch(ctx, id)
	if b.Status != StatusCompleted || b.SentCount != 120 {
		t.Fatalf("batch = %s %s", b.Status, counters(b))
	}
	assertCountersMatch(t, st, id)
}

func TestCancelBatch(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	st.db.Exec(`UPDATE email_batches SET status = 'completed' WHERE status IN ('queued', 'sending')`)
	id := sendReadyBatch(t, st, 4)

	inFlight, _ := st.ClaimNextJob(ctx, "s1")
	n, err := st.CancelBatch(ctx, id)
	if err != nil || n != 3 {
		t.Fatalf("cancel = %d, %v; want 3", n, err)
	}
	if _, err := st.ClaimNextJob(ctx, "s1"); !errors.Is(err, ErrNoJobs) {
		t.Fatalf("a cancelled batch must not be sent: %v", err)
	}
	// The email being sent fails: it is cancelled, not retried.
	if _, err := st.MarkJobFailed(ctx, inFlight.ID, "s1", "timeout", true, 0); err != nil {
		t.Fatal(err)
	}
	b, _ := st.GetBatch(ctx, id)
	if b.Status != StatusCancelled || b.CancelledCount != 4 || b.CancelledAt == nil {
		t.Fatalf("cancelled: %s %s", b.Status, counters(b))
	}
	assertCountersMatch(t, st, id)

	if _, err := st.CancelBatch(ctx, id); !errors.Is(err, ErrBatchNotActive) {
		t.Fatalf("second cancel: %v", err)
	}
	if _, err := st.RetryFailedJobs(ctx, id, nil); !errors.Is(err, ErrBatchNotActive) {
		t.Fatalf("retry after cancel: %v", err)
	}
	draft := newImportedBatch(t, st, TypeBulkSend, people(1), 10)
	if _, err := st.CancelBatch(ctx, draft); !errors.Is(err, ErrBatchNotActive) {
		t.Fatalf("cancel a draft: %v", err)
	}
	if _, err := st.CancelBatch(ctx, 999999999); !errors.Is(err, ErrBatchNotFound) {
		t.Fatalf("cancel unknown: %v", err)
	}
}

func TestCancelBatch_DuringGeneration(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	id := newImportedBatch(t, st, TypeBulkSend, people(10), 100)
	st.RequestSend(ctx, id, "s", "b")
	task := claimGenerationFor(t, st, id, "g1")
	st.GenerateChunk(ctx, task, "g1", 4)

	n, err := st.CancelBatch(ctx, id)
	if err != nil || n != 4 {
		t.Fatalf("cancel during generation = %d, %v", n, err)
	}
	if _, _, err := st.GenerateChunk(ctx, task, "g1", 4); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("generation must stop after cancel: %v", err)
	}
	b, _ := st.GetBatch(ctx, id)
	if b.Status != StatusCancelled || *b.JobGenerationStatus != "failed" {
		t.Fatalf("batch = %s gen %v", b.Status, *b.JobGenerationStatus)
	}
	assertCountersMatch(t, st, id)
}

func TestRetryFailedJobs_Batch(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	id, jobs := seedBatch(t, st, "sent", "failed", "failed", "invalid")
	st.db.Exec(`UPDATE email_jobs SET attempts = 3, last_error = 'x' WHERE id = ANY($1)`, jobs[1:])

	n, err := st.RetryFailedJobs(ctx, id, nil)
	if err != nil || n != 2 {
		t.Fatalf("retry = %d %v", n, err)
	}
	b, _ := st.GetBatch(ctx, id)
	if b.Status != StatusSending || b.PendingCount != 2 || b.FailedCount != 0 {
		t.Fatalf("after retry: %s %s", b.Status, counters(b))
	}
	var attempts int
	st.db.QueryRow(`SELECT attempts FROM email_jobs WHERE id = $1`, jobs[1]).Scan(&attempts)
	if attempts != 0 {
		t.Fatalf("attempts = %d, want a fresh start", attempts)
	}
	assertCountersMatch(t, st, id)
	if n, _ := st.RetryFailedJobs(ctx, id, nil); n != 0 {
		t.Fatalf("nothing left to retry, got %d", n)
	}
}

func TestRequeueStuckJobs(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	id, jobs := seedBatch(t, st, "processing", "processing", "processing")
	st.db.Exec(`UPDATE email_jobs SET attempts = 1, max_attempts = 3, locked_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, jobs[0])
	st.db.Exec(`UPDATE email_jobs SET attempts = 3, max_attempts = 3, locked_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, jobs[1])
	// jobs[2] is fresh: its worker is still busy with it.

	n, err := st.RequeueStuckJobs(ctx, 5*time.Minute)
	if err != nil || n != 2 {
		t.Fatalf("requeued %d, %v; want 2", n, err)
	}
	b, _ := st.GetBatch(ctx, id)
	if b.PendingCount != 1 || b.FailedCount != 1 || b.ProcessingCount != 1 {
		t.Fatalf("after requeue: %s", counters(b))
	}
	assertCountersMatch(t, st, id)
}

func TestListBatches(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	a := newImportedBatch(t, st, TypeBulkSend, people(2), 10)
	v := newImportedBatch(t, st, TypeVerification, people(2), 10)
	st.db.Exec(`UPDATE email_batches SET original_filename = 'orders-november.xlsx' WHERE id = $1`, a)

	bulk, total, err := st.ListBatches(ctx, BatchListFilter{Type: TypeBulkSend, Search: "orders-nov", Limit: 10})
	if err != nil || total != 1 || len(bulk) != 1 || bulk[0].ID != a || bulk[0].BodyTemplate != "" {
		t.Fatalf("bulk list = %+v (%d) %v", bulk, total, err)
	}
	ver, total, _ := st.ListBatches(ctx, BatchListFilter{Type: TypeVerification, Status: StatusCompleted, Limit: 100})
	found := false
	for _, b := range ver {
		found = found || b.ID == v
	}
	if !found || total < 1 {
		t.Fatalf("verification list misses %d: %+v", v, ver)
	}
	page, total, _ := st.ListBatches(ctx, BatchListFilter{Limit: 1, Offset: 0})
	if len(page) != 1 || total < 2 {
		t.Fatalf("paged = %d of %d", len(page), total)
	}
}

func TestStatusCheckConstraints(t *testing.T) {
	st := openTestStore(t)
	id, jobs := seedBatch(t, st, "pending")
	for _, q := range []struct{ sql string }{
		{`UPDATE email_batches SET status = 'canceled' WHERE id = $1`},
		{`UPDATE email_batches SET type = 'other' WHERE id = $1`},
	} {
		if _, err := st.db.Exec(q.sql, id); err == nil {
			t.Errorf("%s: want a CHECK violation", q.sql)
		}
	}
	if _, err := st.db.Exec(`UPDATE email_jobs SET status = 'invalid' WHERE id = $1`, jobs[0]); err == nil {
		t.Error("email_jobs.status 'invalid' must be rejected")
	}
	if _, err := st.db.Exec(`UPDATE email_recipients SET validation_reason = NULL, validation_status = 'invalid' WHERE batch_id = $1`, id); err == nil {
		t.Error("an invalid row must have a reason")
	}
	if _, err := st.db.Exec(`UPDATE email_recipients SET validation_reason = 'duplicate', validation_status = 'invalid' WHERE batch_id = $1`, id); err == nil {
		t.Error("'duplicate' is not a reason any more")
	}
}
