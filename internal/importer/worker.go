package importer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"sendmail/internal/domaincheck"
	"sendmail/internal/recipients"
	"sendmail/internal/store"
)

// Defaults; tests make them smaller.
const (
	DefaultChunkSize = 1000             // rows saved per transaction
	DefaultLease     = 60 * time.Second // a task whose worker is silent this long can be taken over
)

// Store is what the import worker needs from the database.
type Store interface {
	ClaimImportJob(ctx context.Context, workerID string, lease time.Duration) (*store.ImportJob, error)
	RenewImportLease(ctx context.Context, jobID int64, workerID string) error
	SaveImportChunk(ctx context.Context, job *store.ImportJob, workerID string, c store.ImportChunk) error
	FailImport(ctx context.Context, job *store.ImportJob, reason string) error
	ReleaseImport(ctx context.Context, job *store.ImportJob, workerID, reason string, refund bool) error
}

// DomainChecker reports whether an email domain can receive mail.
// *domaincheck.Checker satisfies it; tests pass a fake.
type DomainChecker interface {
	Check(ctx context.Context, domain string) domaincheck.Result
}

// Worker imports one file at a time in the background (one is enough: the
// database, not the CPU, is usually the limit, and it keeps memory bounded).
type Worker struct {
	store        Store
	files        Files
	domains      DomainChecker // nil = no domain check available
	domainsOn    atomic.Bool   // the Settings switch
	pollInterval time.Duration

	ChunkSize int // optional test override; production uses SetChunkSize
	chunkSize atomic.Int64
	Lease     time.Duration

	id     string
	wake   chan struct{}
	wg     sync.WaitGroup
	cancel context.CancelFunc

	// afterChunk, when set (tests), runs after every saved chunk.
	afterChunk func(lastRow int)
}

// NewWorker builds an import worker. Call Start to run it.
func NewWorker(st Store, files Files, domains DomainChecker, pollInterval time.Duration) *Worker {
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	host, _ := os.Hostname()
	w := &Worker{
		store: st, files: files, domains: domains, pollInterval: pollInterval,
		Lease: DefaultLease,
		id:    fmt.Sprintf("%s:%d:import-%s", host, os.Getpid(), randomID()),
		wake:  make(chan struct{}, 1),
	}
	w.chunkSize.Store(DefaultChunkSize)
	w.domainsOn.Store(domains != nil)
	return w
}

// SetChunkSize sets how many rows each import transaction saves. Imports
// already in progress keep the chunk size they started with. Values below 1
// are ignored.
func (w *Worker) SetChunkSize(n int) {
	if n > 0 {
		w.chunkSize.Store(int64(n))
	}
}

func (w *Worker) effectiveChunkSize() int {
	if w.ChunkSize > 0 {
		return w.ChunkSize
	}
	if n := int(w.chunkSize.Load()); n > 0 {
		return n
	}
	return DefaultChunkSize
}

// SetDomainCheckEnabled switches the domain check on or off; it applies to
// imports that start after the change. No effect without a checker.
func (w *Worker) SetDomainCheckEnabled(on bool) { w.domainsOn.Store(on) }

// Wake makes an idle worker look for work now instead of at the next poll.
func (w *Worker) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Start runs the worker until Stop or until ctx ends.
func (w *Worker) Start(ctx context.Context) {
	ctx, w.cancel = context.WithCancel(ctx)
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		slog.Info("import worker started", "worker", w.id)
		for ctx.Err() == nil {
			worked, err := w.RunOnce(ctx)
			if err != nil && ctx.Err() == nil {
				slog.Error("import worker", "err", err)
			}
			if worked {
				continue
			}
			select {
			case <-ctx.Done():
			case <-w.wake:
			case <-time.After(w.pollInterval):
			}
		}
	}()
}

// Stop asks the worker to stop and waits. An import in progress is handed
// back (with its checkpoint) and continues on the next start.
func (w *Worker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
	slog.Info("import worker stopped")
}

// RunOnce claims and imports one file, if one is waiting. It reports whether
// it found work. Exported so tests can drive the worker step by step.
func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	job, err := w.store.ClaimImportJob(ctx, w.id, w.Lease)
	if errors.Is(err, store.ErrNoJobs) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	w.process(ctx, job)
	return true, nil
}

// fileError marks a problem with the file itself; retrying will not help.
type fileError struct{ msg string }

func (e fileError) Error() string { return e.msg }

func (w *Worker) process(ctx context.Context, job *store.ImportJob) {
	log := slog.With("batch_id", job.BatchID, "import_job", job.ID, "attempt", job.Attempts)
	bg := context.Background() // for the final bookkeeping, even during shutdown

	if job.Attempts > job.MaxAttempts {
		reason := fmt.Sprintf("the import stopped %d times and was given up", job.MaxAttempts)
		if job.LastError != "" {
			reason += ": " + job.LastError
		}
		log.Error("import given up", "reason", reason)
		if err := w.store.FailImport(bg, job, reason); err != nil {
			log.Error("record import failure failed", "err", err)
		}
		return
	}

	// Keep the lease fresh while we work. If another worker took the task
	// over (we were too slow), stop at once.
	workCtx, stopWork := context.WithCancel(ctx)
	defer stopWork()
	var leaseLost atomic.Bool
	go func() {
		t := time.NewTicker(w.Lease / 4)
		defer t.Stop()
		for {
			select {
			case <-workCtx.Done():
				return
			case <-t.C:
				if err := w.store.RenewImportLease(workCtx, job.ID, w.id); errors.Is(err, store.ErrLeaseLost) {
					leaseLost.Store(true)
					stopWork()
					return
				}
			}
		}
	}()

	start := time.Now()
	stats, err := w.importFile(workCtx, job)
	switch {
	case err == nil && stats.rows == 0 && job.LastProcessedRow == 0:
		log.Warn("import failed: the file has no data rows")
	case err == nil:
		log.Info("import finished", "rows", stats.rows, "chunk_size", stats.chunkSize, "resumed_after_row", job.LastProcessedRow,
			"domain_lookups", stats.lookups, "took", time.Since(start).Round(time.Millisecond),
			"rate", fmt.Sprintf("%.0f rows/s", float64(stats.rows)/max(time.Since(start).Seconds(), 0.001)))
	case leaseLost.Load() || errors.Is(err, store.ErrLeaseLost):
		log.Warn("import taken over by another worker; stopping")
	case ctx.Err() != nil: // shutting down: hand the task back, attempt refunded
		log.Info("import paused for shutdown; it continues on the next start", "rows_this_run", stats.rows)
		if err := w.store.ReleaseImport(bg, job, w.id, "server stopped during import", true); err != nil {
			log.Error("release import failed", "err", err)
		}
	default:
		var fe fileError
		if errors.As(err, &fe) {
			log.Warn("import failed: bad file", "reason", fe.msg)
			if err := w.store.FailImport(bg, job, fe.msg); err != nil {
				log.Error("record import failure failed", "err", err)
			}
			return
		}
		log.Error("import interrupted; it will be retried", "err", err)
		if err := w.store.ReleaseImport(bg, job, w.id, err.Error(), false); err != nil {
			log.Error("release import failed", "err", err)
		}
	}
}

type importStats struct{ rows, lookups, chunkSize int }

// importFile reads the batch's file from the checkpoint on and saves it in
// chunks. Rows up to job.LastProcessedRow are read but skipped: they were
// saved by an earlier attempt.
func (w *Worker) importFile(ctx context.Context, job *store.ImportJob) (importStats, error) {
	chunkSize := w.effectiveChunkSize()
	var st importStats
	st.chunkSize = chunkSize
	f, err := os.Open(w.files.Path(job.BatchID))
	if errors.Is(err, os.ErrNotExist) {
		return st, fileError{"the uploaded file is missing on the server"}
	}
	if err != nil {
		return st, fmt.Errorf("open upload: %w", err)
	}
	defer f.Close()

	// Read the setting once, so a change mid-file cannot treat half the
	// rows one way and half the other.
	checkDomains := w.domains != nil && w.domainsOn.Load()
	domainResults := map[string]domaincheck.Result{} // each domain looked up once per import

	chunk := make([]store.ImportRow, 0, chunkSize)
	lastRow := job.LastProcessedRow
	var saveErr error
	flush := func(done bool) error {
		err := w.store.SaveImportChunk(ctx, job, w.id, store.ImportChunk{Rows: chunk, LastRow: lastRow, Done: done})
		if err != nil {
			saveErr = err
			return err
		}
		st.rows += len(chunk)
		chunk = chunk[:0]
		if w.afterChunk != nil {
			w.afterChunk(lastRow)
		}
		return nil
	}

	err = recipients.Scan(f, func(r recipients.Row) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if r.Num <= job.LastProcessedRow {
			return nil // saved before a restart
		}
		if checkDomains && r.Kind == recipients.KindOK {
			domain := r.Email[strings.LastIndex(r.Email, "@")+1:]
			res, seen := domainResults[domain]
			if !seen {
				res = w.domains.Check(ctx, domain)
				domainResults[domain] = res
				st.lookups++
			}
			switch res.Status {
			case domaincheck.Invalid:
				r.Kind = recipients.KindBadDomain
			case domaincheck.Unknown:
				r.Kind = recipients.KindUnverified
			}
		}
		chunk = append(chunk, toImportRow(r))
		lastRow = r.Num
		if len(chunk) >= chunkSize {
			return flush(false)
		}
		return nil
	})
	switch {
	case saveErr != nil:
		return st, saveErr
	case ctx.Err() != nil:
		return st, ctx.Err()
	case errors.Is(err, recipients.ErrMissingColumns):
		return st, fileError{err.Error()}
	case err != nil:
		return st, fileError{"could not read the spreadsheet: " + err.Error()}
	}
	return st, flush(true)
}

func toImportRow(r recipients.Row) store.ImportRow {
	out := store.ImportRow{Row: r.Num, Name: r.Name, Email: r.Email}
	switch r.Kind {
	case recipients.KindOK:
		out.Status = store.RecipientValid
	case recipients.KindUnverified:
		out.Status = store.RecipientUnverified
	default:
		out.Status, out.Reason = store.RecipientInvalid, string(r.Kind)
	}
	return out
}
