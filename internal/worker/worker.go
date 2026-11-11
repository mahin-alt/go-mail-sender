// Package worker runs the background goroutines that turn a confirmed batch
// into delivered email:
//
//   - Generator (generator.go): after the user clicks Send, creates one
//     email_jobs row per sendable recipient, in chunks with a checkpoint.
//   - Pool (this file): a fixed number of sending workers that claim email
//     jobs from PostgreSQL, build each email from the batch's templates, and
//     deliver it through the SMTP connection pool.
//
// Concept: Worker Pool pattern. Instead of spawning an unbounded number of
// goroutines (which could overwhelm the database and SMTP server), we spawn a
// fixed number of workers.
//
// Concurrency concepts used here:
//   - sync.WaitGroup: tracks active worker goroutines so the server can wait
//     for in-flight email sends to complete before shutting down.
//   - context.Context: signals cancellation to all workers when the server stops.
//   - non-blocking wake channel: notifies workers immediately when there is
//     new work, avoiding any delay from the polling interval.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/textproto"
	"os"
	"strings"
	"sync"
	"time"

	"sendmail/internal/logging"
	"sendmail/internal/mailer"
	"sendmail/internal/store"
	"sendmail/internal/template"
)

// Timing of crash recovery for emails.
const (
	// StuckAfter: an email still 'processing' this long after it was claimed
	// belongs to a worker that died; it is put back in the queue. It must be
	// longer than any real send (the SMTP pool times out well before this).
	StuckAfter = 5 * time.Minute
	// reapEvery is how often the pool looks for such emails.
	reapEvery = 30 * time.Second
)

// Pool manages a group of concurrent background sending workers.
type Pool struct {
	workerCount  int
	pollInterval time.Duration
	store        *store.Store
	mailer       mailer.Mailer
	idPrefix     string

	// wakeChan allows API handlers to wake up idle workers immediately.
	wakeChan chan struct{}

	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

// NewPool constructs a worker pool. Call Start() to run the background workers.
func NewPool(workerCount int, pollInterval time.Duration, st *store.Store, m mailer.Mailer) *Pool {
	if workerCount <= 0 {
		workerCount = 1
	}
	if pollInterval <= 0 {
		pollInterval = 500 * time.Millisecond
	}
	host, _ := os.Hostname()
	return &Pool{
		workerCount:  workerCount,
		pollInterval: pollInterval,
		store:        st,
		mailer:       m,
		idPrefix:     fmt.Sprintf("%s:%d:send", host, os.Getpid()),
		wakeChan:     make(chan struct{}, workerCount),
	}
}

// Start launches the worker goroutines (and the stuck-email reaper).
func (p *Pool) Start(parentCtx context.Context) {
	p.ctx, p.cancel = context.WithCancel(parentCtx)

	slog.Info("starting email workers", "count", p.workerCount, "poll_interval", p.pollInterval)
	for i := 1; i <= p.workerCount; i++ {
		p.wg.Add(1)
		go p.runWorker(i)
	}
	p.wg.Add(1)
	go p.runReaper()
}

// Wake sends non-blocking signals to wake up ALL idle workers immediately.
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
	workerID := fmt.Sprintf("%s-%d", p.idPrefix, id)

	for {
		// 1. Check if the application is shutting down.
		if p.ctx.Err() != nil {
			return
		}

		// 2. Try to claim the next email using FOR UPDATE SKIP LOCKED.
		job, err := p.store.ClaimNextJob(p.ctx, workerID)
		if err != nil {
			wait := p.pollInterval
			if !errors.Is(err, store.ErrNoJobs) {
				// Transient database error: log and back off a little longer.
				if p.ctx.Err() == nil {
					slog.Error("claim job failed", "worker", id, "err", err)
				}
				wait = time.Second
			}
			select {
			case <-time.After(wait):
			case <-p.wakeChan:
			case <-p.ctx.Done():
				return
			}
			continue
		}

		// 3. Send it. The send itself is not cut short by shutdown: an email
		// in flight finishes and its result is recorded.
		p.processJob(workerID, job)
	}
}

// runReaper puts emails whose worker died mid-send back in the queue.
func (p *Pool) runReaper() {
	defer p.wg.Done()
	t := time.NewTicker(reapEvery)
	defer t.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-t.C:
		}
		n, err := p.store.RequeueStuckJobs(p.ctx, StuckAfter)
		if err != nil && p.ctx.Err() == nil {
			slog.Error("requeue stuck emails failed", "err", err)
		}
		if n > 0 {
			slog.Warn("emails stuck in processing were requeued (their worker stopped mid-send)", "count", n)
			p.Wake()
		}
	}
}

// processJob builds and sends the email and records the result.
//
// Logging levels: a successful send is DEBUG (a 1M-row batch would otherwise
// write a million lines), a retry is WARN, a final failure is ERROR, and a
// finished batch gets one INFO summary. Addresses are masked.
func (p *Pool) processJob(workerID string, job *store.Job) {
	to := logging.MaskEmail(job.Email)
	ctx := context.Background()
	log := slog.With("job_id", job.ID, "batch_id", job.BatchID, "worker", workerID, "to", to, "attempt", job.Attempts)

	subject, body, err := render(job)
	if err != nil {
		// The templates were checked when Send was clicked; if one still fails
		// for this recipient, retrying will not help.
		log.Error("could not build the email", "err", err)
		p.recordFailure(ctx, log, job, workerID, "could not build the email: "+err.Error(), false)
		return
	}

	start := time.Now()
	err = p.mailer.Send(job.Email, subject, body)
	sendTime := time.Since(start).Round(time.Millisecond)
	if err != nil {
		retry := !permanent(err) && job.Attempts < job.MaxAttempts
		if retry {
			log.Warn("send failed, will retry", "max_attempts", job.MaxAttempts,
				"retry_in", backoff(job.Attempts), "send", sendTime, "err", err)
		} else {
			log.Error("send failed permanently", "max_attempts", job.MaxAttempts, "send", sendTime, "err", err)
		}
		p.recordFailure(ctx, log, job, workerID, err.Error(), retry)
		return
	}

	done, err := p.store.MarkJobSent(ctx, job.ID, workerID)
	if err != nil {
		// The email went out but stays 'processing'; the reaper requeues it
		// later, so the recipient may get it twice (at-least-once delivery).
		log.Error("mark job sent failed", "err", err)
		return
	}
	log.Debug("email sent", "send", sendTime, "queued", job.QueuedFor)
	if done {
		p.logBatchCompleted(ctx, job.BatchID)
	}
}

func (p *Pool) recordFailure(ctx context.Context, log *slog.Logger, job *store.Job, workerID, reason string, retry bool) {
	done, err := p.store.MarkJobFailed(ctx, job.ID, workerID, reason, retry, backoff(job.Attempts))
	if err != nil {
		log.Error("record job failure failed", "err", err)
		return
	}
	if done {
		p.logBatchCompleted(ctx, job.BatchID)
	}
}

// render builds one email from the batch templates and the recipient row.
// Line breaks in the subject (a name could contain one) become spaces: a
// subject is a single header line, and a line break there could add headers.
func render(job *store.Job) (subject, body string, err error) {
	data := template.RecipientData{Name: job.Name, Email: job.Email}
	if subject, err = template.Render(job.SubjectTemplate, data); err != nil {
		return "", "", err
	}
	if body, err = template.Render(job.BodyTemplate, data); err != nil {
		return "", "", err
	}
	subject = strings.Join(strings.Fields(subject), " ")
	return subject, body, nil
}

// permanent reports whether an SMTP error means "never going to work" (a 5xx
// reply such as 550 "no such user"), so the email is not retried. Network
// problems and 4xx replies ("try again later") are temporary.
func permanent(err error) bool {
	var te *textproto.Error
	return errors.As(err, &te) && te.Code >= 500 && te.Code < 600
}

// backoff is the wait before attempt n+1: 5s, 10s, 20s, 40s... capped at 10
// minutes, plus up to 20% random jitter so failed emails do not all come back
// at the same instant.
func backoff(attempt int) time.Duration {
	d := 5 * time.Second << min(max(attempt-1, 0), 7)
	d = min(d, 10*time.Minute)
	return d + time.Duration(rand.Int64N(int64(d)/5+1))
}

// logBatchCompleted writes the one-line summary of a finished batch: counts,
// how long sending took (from the Send click to the last email), and the rate.
func (p *Pool) logBatchCompleted(ctx context.Context, batchID int64) {
	b, err := p.store.GetBatch(ctx, batchID)
	if err != nil {
		slog.Info("batch completed", "batch_id", batchID)
		return
	}
	attrs := []any{"batch_id", batchID, "filename", b.Filename, "status", b.Status,
		"sent", b.SentCount, "failed", b.FailedCount, "invalid", b.InvalidCount}
	if b.CompletedAt != nil && b.SendRequestedAt != nil {
		took := b.CompletedAt.Sub(*b.SendRequestedAt)
		attrs = append(attrs, "duration", took.Round(time.Millisecond), "rate", sendRate(b.SentCount+b.FailedCount, took))
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
