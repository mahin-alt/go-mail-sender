package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"sendmail/internal/store"
)

// Generator defaults; tests make them smaller.
const (
	DefaultGenerationChunk = 5000             // email jobs created per transaction
	DefaultGenerationLease = 60 * time.Second // silent this long = taken over
	maxGenerationAttempts  = 5
)

// Generator creates a batch's email jobs after the user clicks Send: one job
// per valid or unverified recipient, DefaultGenerationChunk at a time. Each
// chunk is saved with its checkpoint (the last recipient ID handled), so a
// crash resumes where it stopped, and the UNIQUE recipient_id means a
// repeated chunk cannot create duplicates. Only when every job exists does
// the batch become 'queued', which is when the sending workers start on it.
type Generator struct {
	store        *store.Store
	pollInterval time.Duration
	onQueued     func() // wakes the sending pool

	ChunkSize int
	Lease     time.Duration

	id     string
	wake   chan struct{}
	wg     sync.WaitGroup
	cancel context.CancelFunc
}

// NewGenerator builds a generator; onQueued is called when a batch is ready
// to send (nil is fine).
func NewGenerator(st *store.Store, pollInterval time.Duration, onQueued func()) *Generator {
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	host, _ := os.Hostname()
	return &Generator{
		store: st, pollInterval: pollInterval, onQueued: onQueued,
		ChunkSize: DefaultGenerationChunk, Lease: DefaultGenerationLease,
		id:   fmt.Sprintf("%s:%d:generate", host, os.Getpid()),
		wake: make(chan struct{}, 1),
	}
}

// Wake makes an idle generator look for work now.
func (g *Generator) Wake() {
	select {
	case g.wake <- struct{}{}:
	default:
	}
}

// Start runs the generator until Stop or until ctx ends.
func (g *Generator) Start(ctx context.Context) {
	ctx, g.cancel = context.WithCancel(ctx)
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		for ctx.Err() == nil {
			worked, err := g.RunOnce(ctx)
			if err != nil && ctx.Err() == nil {
				slog.Error("job generator", "err", err)
			}
			if worked {
				continue
			}
			select {
			case <-ctx.Done():
			case <-g.wake:
			case <-time.After(g.pollInterval):
			}
		}
	}()
}

// Stop asks the generator to stop and waits. A batch in progress keeps its
// checkpoint and continues on the next start.
func (g *Generator) Stop() {
	if g.cancel != nil {
		g.cancel()
	}
	g.wg.Wait()
	slog.Info("job generator stopped")
}

// RunOnce claims one batch waiting for its jobs and generates them all. It
// reports whether it found work. Exported so tests can drive it.
func (g *Generator) RunOnce(ctx context.Context) (bool, error) {
	task, err := g.store.ClaimGeneration(ctx, g.id, g.Lease)
	if errors.Is(err, store.ErrNoJobs) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	log := slog.With("batch_id", task.BatchID, "attempt", task.Attempts)
	bg := context.Background()

	if task.Attempts > maxGenerationAttempts {
		log.Error("job generation given up")
		if err := g.store.FailGeneration(bg, task.BatchID, fmt.Sprintf("creating the emails failed %d times", maxGenerationAttempts)); err != nil {
			log.Error("record generation failure failed", "err", err)
		}
		return true, nil
	}

	start := time.Now()
	created := 0
	for {
		if ctx.Err() != nil { // shutting down: the checkpoint is saved, resume later
			g.store.ReleaseGeneration(bg, task.BatchID, g.id, "server stopped during job generation", true)
			log.Info("job generation paused for shutdown", "created_this_run", created)
			return true, nil
		}
		n, done, err := g.store.GenerateChunk(ctx, task, g.id, g.ChunkSize)
		if errors.Is(err, store.ErrLeaseLost) {
			log.Warn("job generation stopped: batch cancelled or taken over")
			return true, nil
		}
		if err != nil {
			log.Error("job generation interrupted; it will be retried", "err", err)
			g.store.ReleaseGeneration(bg, task.BatchID, g.id, err.Error(), false)
			return true, nil
		}
		created += n
		if done {
			log.Info("emails created; batch queued for sending", "created", created,
				"took", time.Since(start).Round(time.Millisecond))
			if g.onQueued != nil {
				g.onQueued()
			}
			return true, nil
		}
	}
}
