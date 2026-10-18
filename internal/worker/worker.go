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
	"log"
	"sync"
	"time"

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
		wakeChan:     make(chan struct{}, 1),
	}
}

// Start launches the worker goroutines in the background.
func (p *Pool) Start(parentCtx context.Context) {
	p.ctx, p.cancel = context.WithCancel(parentCtx)

	log.Printf("starting %d background email worker(s) (poll interval: %s)...", p.workerCount, p.pollInterval)
	for i := 1; i <= p.workerCount; i++ {
		p.wg.Add(1)
		go p.runWorker(i)
	}
}

// Wake sends a non-blocking signal to wake up idle workers immediately.
// If a signal is already queued, it does nothing.
func (p *Pool) Wake() {
	select {
	case p.wakeChan <- struct{}{}:
	default:
	}
}

// Stop gracefully shuts down all workers: it stops claiming new jobs and waits
// for all current in-flight emails to finish sending.
func (p *Pool) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	log.Printf("all email workers stopped cleanly")
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
				log.Printf("worker %d: claim job error: %v", id, err)
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
func (p *Pool) processJob(workerID int, job *store.Job) {
	log.Printf("worker %d: sending email to %s (job %d)...", workerID, job.RecipientEmail, job.ID)

	// Send email using the pooled mailer
	err := p.mailer.Send(job.RecipientEmail, job.Subject, job.Body)
	if err != nil {
		log.Printf("worker %d: send email failed for job %d: %v", workerID, job.ID, err)

		// Exponential backoff retry: 5s, 10s, 20s...
		retryDelay := time.Duration(job.Attempts*5) * time.Second
		if markErr := p.store.MarkJobFailed(context.Background(), job.ID, err.Error(), retryDelay); markErr != nil {
			log.Printf("worker %d: failed to update job failure status for job %d: %v", workerID, job.ID, markErr)
		}
		return
	}

	// Success! Mark job as sent.
	if markErr := p.store.MarkJobSent(context.Background(), job.ID); markErr != nil {
		log.Printf("worker %d: failed to mark job %d as sent: %v", workerID, job.ID, markErr)
		return
	}

	log.Printf("worker %d: successfully sent job %d to %s", workerID, job.ID, job.RecipientEmail)
}
