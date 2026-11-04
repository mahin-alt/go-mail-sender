// Package worker runs background Go goroutines that pull email jobs
// from PostgreSQL and deliver them through the SMTP connection pool.
//
// Concept: Worker Pool pattern.
// Instead of spawning an unbounded number of goroutines (which could overwhelm
// the database and SMTP server), we spawn a fixed number of workers (e.g. 3).
//
// Concurrency concepts used here:
//   - sync.WaitGroup: tracks active worker goroutines so the server can wait
//     for in-flight email sends to complete before shutting down.
//   - context.Context: signals cancellation to all workers when the server stops.
//   - non-blocking wake channel: notifies workers immediately when a new batch
//     is uploaded, avoiding any delay from the polling interval.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"sendmail/internal/logging"
	"sendmail/internal/mailer"
	"sendmail/internal/store"
)

// Pool manages a group of concurrent background worker goroutines.
type Pool struct {
	workerCount  int
	pollInterval time.Duration
	store        *store.Store
	mailer       mailer.Mailer

	// wakeChan allows API handlers to wake up idle workers immediately
	// whenever a new batch is created.
	wakeChan chan struct{}

	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

// NewPool constructs a worker pool. Call Start() to run the background workers.
func NewPool(
	workerCount int,
	pollInterval time.Duration,
	st *store.Store,
	m mailer.Mailer,
) *Pool {
	if workerCount <= 0 {
		workerCount = 1
	}
	if pollInterval <= 0 {
		pollInterval = 500 * time.Millisecond
	}

	return &Pool{
		workerCount:  workerCount,
		pollInterval: pollInterval,
		store:        st,
		mailer:       m,
		wakeChan:     make(chan struct{}, workerCount),
	}
}

// Start launches the worker goroutines in the background.
func (p *Pool) Start(parentCtx context.Context) {
	p.ctx, p.cancel = context.WithCancel(parentCtx)

	slog.Info("starting email workers", "count", p.workerCount, "poll_interval", p.pollInterval)
	for i := 1; i <= p.workerCount; i++ {
		p.wg.Add(1)
		go p.runWorker(i)
	}
}

// Wake sends non-blocking signals to wake up ALL idle workers immediately
// whenever a new batch is created.
func (p *Pool) Wake() {
	for i := 0; i < p.workerCount; i++ {
		select {
		case p.wakeChan <- struct{}{}:
		default:
		}
	}
}

// Stop gracefully shuts down all workers: it stops claiming new jobs and waits
// for all current in-flight emails to finish sending.
func (p *Pool) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	slog.Info("email workers stopped")
}

// runWorker is the main loop for a single worker goroutine.
func (p *Pool) runWorker(id int) {
	defer p.wg.Done()

	for {
		// 1. Check if the application is shutting down.
		select {
		case <-p.ctx.Done():
			return
		default:
		}

		// 2. Try to claim the next job from PostgreSQL using FOR UPDATE SKIP LOCKED.
		job, err := p.store.ClaimNextJob(p.ctx)
		if err != nil {
			if errors.Is(err, store.ErrNoJobs) {
				// Queue is empty: wait for poll interval or immediate wake signal.
				select {
				case <-time.After(p.pollInterval):
					continue
				case <-p.wakeChan:
					continue
				case <-p.ctx.Done():
					return
				}
			}

			// Transient database error: log and back off slightly before retrying.
			if !errors.Is(p.ctx.Err(), context.Canceled) {
				slog.Error("claim job failed", "worker", id, "err", err)
			}
			select {
			case <-time.After(1 * time.Second):
				continue
			case <-p.ctx.Done():
				return
			}
		}

		// 3. Process the claimed job.
		p.processJob(id, job)
	}
}

// processJob sends the email and updates the database with the result.
//
// Logging levels: a successful send is DEBUG (a 1M-row batch would otherwise
// write a million lines), a retry is WARN, a final failure is ERROR, and a
// finished batch gets one INFO summary. Addresses are masked.
func (p *Pool) processJob(workerID int, job *store.Job) {
	to := logging.MaskEmail(job.RecipientEmail)
	ctx := context.Background()

	start := time.Now()
	err := p.mailer.Send(job.RecipientEmail, job.Subject, job.Body)
	sendTime := time.Since(start).Round(time.Millisecond)
	if err != nil {
		// Linear backoff: 5s, 10s, 15s...
		retryDelay := time.Duration(job.Attempts*5) * time.Second
		if job.Attempts < job.MaxAttempts {
			slog.Warn("send failed, will retry", "job_id", job.ID, "batch_id", job.BatchID, "to", to,
				"attempt", job.Attempts, "max_attempts", job.MaxAttempts, "retry_in", retryDelay,
				"send", sendTime, "err", err)
		} else {
			slog.Error("send failed permanently", "job_id", job.ID, "batch_id", job.BatchID, "to", to,
				"attempts", job.Attempts, "send", sendTime, "err", err)
		}

		done, markErr := p.store.MarkJobFailed(ctx, job.ID, err.Error(), retryDelay)
		if markErr != nil {
			slog.Error("record job failure failed", "job_id", job.ID, "err", markErr)
			return
		}
		if done {
			p.logBatchCompleted(ctx, job.BatchID)
		}
		return
	}

	done, markErr := p.store.MarkJobSent(ctx, job.ID)
	if markErr != nil {
		// The email went out but the job stays 'processing'.
		slog.Error("mark job sent failed", "job_id", job.ID, "err", markErr)
		return
	}
	slog.Debug("email sent", "job_id", job.ID, "batch_id", job.BatchID, "to", to, "worker", workerID,
		"send", sendTime, "queued", job.QueuedFor)
	if done {
		p.logBatchCompleted(ctx, job.BatchID)
	}
}

// logBatchCompleted writes the one-line summary of a finished batch: counts,
// how long it took from upload to the last email, and the sending rate.
func (p *Pool) logBatchCompleted(ctx context.Context, batchID int64) {
	bp, err := p.store.GetBatchProgress(ctx, batchID)
	if err != nil {
		slog.Info("batch completed", "batch_id", batchID)
		return
	}
	attrs := []any{"batch_id", batchID, "filename", bp.Filename,
		"sent", bp.Sent, "failed", bp.Failed, "invalid", bp.Invalid}
	if bp.CompletedAt != nil {
		took := bp.CompletedAt.Sub(bp.CreatedAt)
		attrs = append(attrs, "duration", took.Round(time.Millisecond), "rate", sendRate(bp.Sent+bp.Failed, took))
	}
	slog.Info("batch completed", attrs...)
}

// sendRate formats emails handled per second, e.g. "12.5/s".
func sendRate(n int, d time.Duration) string {
	if d <= 0 {
		return fmt.Sprintf("%d in <1ms", n)
	}
	return fmt.Sprintf("%.1f/s", float64(n)/d.Seconds())
}
