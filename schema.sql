-- schema.sql
-- Defines the database schema for background email processing, batches, and job queues.

-- 1. batches: Tracks a single bulk email campaign / uploaded file.
CREATE TABLE IF NOT EXISTS batches (
    id BIGSERIAL PRIMARY KEY,
    filename TEXT NOT NULL DEFAULT '',
    subject_template TEXT NOT NULL,
    body_template TEXT NOT NULL,
    total_jobs INT NOT NULL DEFAULT 0,
    -- Status can be: 'pending', 'processing', 'completed', 'failed'
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

-- 2. email_jobs: Represents a single email to be delivered to a recipient.
CREATE TABLE IF NOT EXISTS email_jobs (
    id BIGSERIAL PRIMARY KEY,
    batch_id BIGINT NOT NULL REFERENCES batches(id) ON DELETE CASCADE,
    recipient_name TEXT NOT NULL,
    recipient_email TEXT NOT NULL,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    -- Status can be: 'pending', 'processing', 'sent', 'failed'
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    last_error TEXT,
    locked_at TIMESTAMPTZ,
    run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ
);

-- Index for high-performance job queue claiming with FOR UPDATE SKIP LOCKED.
-- When workers query for 'pending' jobs ready to run, this partial index
-- allows PostgreSQL to find the next job in sub-millisecond time.
CREATE INDEX IF NOT EXISTS idx_email_jobs_fetchable
ON email_jobs (run_at, id)
WHERE status = 'pending';

-- Index for counting jobs per batch quickly when reporting progress.
CREATE INDEX IF NOT EXISTS idx_email_jobs_batch_id
ON email_jobs (batch_id, status);
