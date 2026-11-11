package importer

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"sendmail/internal/domaincheck"
	"sendmail/internal/migrations"
	"sendmail/internal/store"
)

func sample(name string) string { return filepath.Join("..", "..", "samples", name) }

func TestReceive(t *testing.T) {
	files := Files{Dir: t.TempDir()}
	real, err := os.ReadFile(sample("1_valid.xlsx"))
	if err != nil {
		t.Fatal(err)
	}

	path, err := files.Receive(bytes.NewReader(real), int64(len(real)))
	if err != nil {
		t.Fatalf("a file exactly at the limit must be accepted: %v", err)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, real) {
		t.Fatal("the saved file differs from the upload")
	}
	if err := files.Promote(path, 42); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(files.Path(42)); err != nil {
		t.Fatalf("promoted file missing: %v", err)
	}

	if _, err := files.Receive(bytes.NewReader(real), int64(len(real))-1); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("one byte over: %v", err)
	}
	if _, err := files.Receive(strings.NewReader("name,email\nA,a@b.co\n"), 1<<20); !errors.Is(err, ErrNotXLSX) {
		t.Fatalf("a CSV renamed to .xlsx: %v", err)
	}
	if _, err := files.Receive(strings.NewReader("PK"), 1<<20); !errors.Is(err, ErrNotXLSX) {
		t.Fatalf("a tiny file: %v", err)
	}
	// Failed uploads leave nothing behind.
	entries, _ := os.ReadDir(files.Dir)
	if len(entries) != 1 {
		t.Fatalf("folder holds %d files, want only batch-42.xlsx", len(entries))
	}
}

type fakeBatches map[int64]bool

func (f fakeBatches) BatchExists(_ context.Context, id int64) (bool, error) { return f[id], nil }

func TestCleanupOrphans(t *testing.T) {
	files := Files{Dir: t.TempDir()}
	old := time.Now().Add(-2 * time.Hour)
	touch := func(name string, at time.Time) {
		p := filepath.Join(files.Dir, name)
		os.WriteFile(p, []byte("x"), 0o600)
		os.Chtimes(p, at, at)
	}
	touch("batch-1.xlsx", old)              // batch exists: keep
	touch("batch-2.xlsx", old)              // batch gone: remove
	touch(".incoming-abc.xlsx", old)        // abandoned upload: remove
	touch(".incoming-new.xlsx", time.Now()) // upload in progress: keep
	touch("batch-3.xlsx", time.Now())       // too new to judge: keep
	touch("notes.txt", old)                 // not ours: keep

	n, err := files.CleanupOrphans(context.Background(), fakeBatches{1: true}, time.Hour)
	if err != nil || n != 2 {
		t.Fatalf("removed %d, %v; want 2", n, err)
	}
	for _, keep := range []string{"batch-1.xlsx", ".incoming-new.xlsx", "batch-3.xlsx", "notes.txt"} {
		if _, err := os.Stat(filepath.Join(files.Dir, keep)); err != nil {
			t.Errorf("%s was removed", keep)
		}
	}
	if n, err := (Files{Dir: filepath.Join(files.Dir, "missing")}).CleanupOrphans(context.Background(), fakeBatches{}, 0); n != 0 || err != nil {
		t.Fatalf("missing folder: %d %v", n, err)
	}
}

// --- database tests (skipped without TEST_DATABASE_URL) ---

func openStore(t *testing.T) *store.Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var schema, dbname string
	db.QueryRow(`SELECT current_schema(), current_database()`).Scan(&schema, &dbname)
	if schema == "public" && !strings.Contains(dbname, "test") {
		t.Skipf("refusing to run on the public schema of %q", dbname)
	}
	if _, err := migrations.Up(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	// Other tests' unfinished imports must not be claimed here.
	db.Exec(`UPDATE email_import_jobs SET status = 'failed' WHERE status IN ('pending', 'processing')`)
	return store.New(db)
}

// newUpload copies a sample into place as a new batch, like the API does.
func newUpload(t *testing.T, st *store.Store, files Files, batchType, name string) int64 {
	t.Helper()
	f, err := os.Open(sample(name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	tmp, err := files.Receive(f, 50<<20)
	if err != nil {
		t.Fatal(err)
	}
	id, err := st.CreateBatch(context.Background(), batchType, name, func(id int64) error { return files.Promote(tmp, id) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.DB().Exec(`DELETE FROM email_batches WHERE id = $1`, id) })
	return id
}

type fakeDomains map[string]domaincheck.Result

func (f fakeDomains) Check(_ context.Context, domain string) domaincheck.Result {
	if r, ok := f[domain]; ok {
		return r
	}
	return domaincheck.Result{Status: domaincheck.Valid}
}

func TestImport_SmallFileWithDomainCheck(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	files := Files{Dir: t.TempDir()}
	id := newUpload(t, st, files, store.TypeVerification, "9_mixed_everything.xlsx")

	w := NewWorker(st, files, fakeDomains{"example.com": {Status: domaincheck.Unknown, Reason: "timeout"}}, 0)
	w.ChunkSize = 3
	if worked, err := w.RunOnce(ctx); !worked || err != nil {
		t.Fatalf("RunOnce = %v, %v", worked, err)
	}
	b, err := st.GetBatch(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	// 9_mixed_everything: 8 rows; 4 well-formed, 3 of them @example.com
	// (unverified here, the lookup "timed out"), and 4 invalid.
	if b.Status != store.StatusCompleted || b.TotalRows != 8 || b.ValidCount != 1 || b.UnverifiedCount != 3 || b.InvalidCount != 4 {
		t.Fatalf("batch = %+v", b)
	}

	// Switched off, the same file has no unverified rows.
	w.SetDomainCheckEnabled(false)
	id2 := newUpload(t, st, files, store.TypeVerification, "9_mixed_everything.xlsx")
	w.RunOnce(ctx)
	if b, _ := st.GetBatch(ctx, id2); b.ValidCount != 4 || b.UnverifiedCount != 0 {
		t.Fatalf("domain check off: %+v", b)
	}
}

func TestImport_BadFilesFailWithAReason(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	files := Files{Dir: t.TempDir()}
	w := NewWorker(st, files, nil, 0)

	headers := newUpload(t, st, files, store.TypeBulkSend, "7_wrong_headers.xlsx")
	w.RunOnce(ctx)
	if b, _ := st.GetBatch(ctx, headers); b.Status != store.StatusImportFailed || !strings.Contains(*b.ImportError, `"name" and "email"`) {
		t.Fatalf("wrong headers: %s %v", b.Status, b.ImportError)
	}

	empty := newUpload(t, st, files, store.TypeBulkSend, "8_headers_only.xlsx")
	w.RunOnce(ctx)
	if b, _ := st.GetBatch(ctx, empty); b.Status != store.StatusImportFailed || !strings.Contains(*b.ImportError, "no recipient rows") {
		t.Fatalf("headers only: %s %v", b.Status, b.ImportError)
	}

	gone := newUpload(t, st, files, store.TypeBulkSend, "1_valid.xlsx")
	os.Remove(files.Path(gone))
	w.RunOnce(ctx)
	if b, _ := st.GetBatch(ctx, gone); b.Status != store.StatusImportFailed || !strings.Contains(*b.ImportError, "missing") {
		t.Fatalf("missing file: %s %v", b.Status, b.ImportError)
	}
}

// The worker dies right after saving its second chunk (no clean-up, no
// release). After the lease runs out a second worker takes over, continues
// from the checkpoint, and the end result is exactly a clean import: every
// row once, none missing.
func TestImport_CrashMidFileResumesWithoutGapsOrDuplicates(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	files := Files{Dir: t.TempDir()}
	id := newUpload(t, st, files, store.TypeBulkSend, "10_large_mixed_1000.xlsx")

	crashing := NewWorker(st, files, nil, 0)
	crashing.ChunkSize = 100
	chunks := 0
	crashing.afterChunk = func(int) {
		if chunks++; chunks == 2 {
			runtime.Goexit() // the goroutine stops dead, like a killed process
		}
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); crashing.RunOnce(ctx) }()
	wg.Wait()

	b, _ := st.GetBatch(ctx, id)
	if b.Status != store.StatusImporting || b.TotalRows != 200 {
		t.Fatalf("after the crash: %s with %d rows, want importing with 200", b.Status, b.TotalRows)
	}

	takeover := NewWorker(st, files, nil, 0)
	takeover.ChunkSize = 100
	if worked, _ := takeover.RunOnce(ctx); worked {
		t.Fatal("a live lease must not be taken over")
	}
	st.DB().Exec(`UPDATE email_import_jobs SET locked_at = NOW() - INTERVAL '10 minutes' WHERE batch_id = $1`, id)
	if worked, err := takeover.RunOnce(ctx); !worked || err != nil {
		t.Fatalf("takeover = %v, %v", worked, err)
	}

	// Compare with a clean import of the same file.
	clean := newUpload(t, st, files, store.TypeBulkSend, "10_large_mixed_1000.xlsx")
	takeover.RunOnce(ctx)
	got, _ := st.GetBatch(ctx, id)
	want, _ := st.GetBatch(ctx, clean)
	if got.Status != store.StatusDraft || got.TotalRows != want.TotalRows || got.ValidCount != want.ValidCount ||
		got.InvalidCount != want.InvalidCount || got.TotalRows < 900 {
		t.Fatalf("resumed = %s rows %d valid %d invalid %d; clean = rows %d valid %d invalid %d",
			got.Status, got.TotalRows, got.ValidCount, got.InvalidCount, want.TotalRows, want.ValidCount, want.InvalidCount)
	}
	var distinct, rows int
	st.DB().QueryRow(`SELECT COUNT(DISTINCT row_number), COUNT(*) FROM email_recipients WHERE batch_id = $1`, id).Scan(&distinct, &rows)
	if distinct != rows || rows != want.TotalRows {
		t.Fatalf("%d rows, %d distinct row numbers, want %d", rows, distinct, want.TotalRows)
	}
	var attempts int
	st.DB().QueryRow(`SELECT attempts FROM email_import_jobs WHERE batch_id = $1`, id).Scan(&attempts)
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

// A clean shutdown hands the import back; the next start finishes it and the
// stop does not count as a failed attempt.
func TestImport_ShutdownPausesAndResumes(t *testing.T) {
	st := openStore(t)
	files := Files{Dir: t.TempDir()}
	id := newUpload(t, st, files, store.TypeBulkSend, "10_large_mixed_1000.xlsx")

	ctx, cancel := context.WithCancel(context.Background())
	w := NewWorker(st, files, nil, 0)
	w.ChunkSize = 100
	w.afterChunk = func(int) { cancel() } // "Ctrl+C" after the first chunk
	w.RunOnce(ctx)

	var status string
	var attempts, last int
	st.DB().QueryRow(`SELECT status, attempts, last_processed_row FROM email_import_jobs WHERE batch_id = $1`, id).Scan(&status, &attempts, &last)
	if status != "pending" || attempts != 0 || last == 0 {
		t.Fatalf("after shutdown: %s attempts %d checkpoint %d", status, attempts, last)
	}

	w2 := NewWorker(st, files, nil, 0)
	if worked, err := w2.RunOnce(context.Background()); !worked || err != nil {
		t.Fatalf("restart = %v %v", worked, err)
	}
	if b, _ := st.GetBatch(context.Background(), id); b.Status != store.StatusDraft {
		t.Fatalf("after restart: %s", b.Status)
	}
}

func TestImport_GivesUpAfterMaxAttempts(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	files := Files{Dir: t.TempDir()}
	id := newUpload(t, st, files, store.TypeBulkSend, "1_valid.xlsx")
	st.DB().Exec(`UPDATE email_import_jobs SET attempts = 3, last_error = 'out of memory' WHERE batch_id = $1`, id)

	NewWorker(st, files, nil, 0).RunOnce(ctx)
	b, _ := st.GetBatch(ctx, id)
	if b.Status != store.StatusImportFailed || !strings.Contains(*b.ImportError, "out of memory") {
		t.Fatalf("batch = %s %v", b.Status, b.ImportError)
	}
}
