-- +goose Up
-- Baseline: the schema as it existed before migrations were introduced.
-- Every statement uses IF NOT EXISTS so a database created by the old
-- "InitSchema on startup" code adopts this migration without changes: goose
-- simply records version 1 and moves on.

-- batches: one uploaded file / bulk email.
CREATE TABLE IF NOT EXISTS batches (
    id BIGSERIAL PRIMARY KEY,
    filename TEXT NOT NULL DEFAULT '',
    subject_template TEXT NOT NULL,
    body_template TEXT NOT NULL,
    total_jobs INT NOT NULL DEFAULT 0,
    -- Status can be: 'pending', 'processing', 'completed'
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

-- email_jobs: one email to one recipient, processed by background workers.
CREATE TABLE IF NOT EXISTS email_jobs (
    id BIGSERIAL PRIMARY KEY,
    batch_id BIGINT NOT NULL REFERENCES batches(id) ON DELETE CASCADE,
    recipient_name TEXT NOT NULL,
    recipient_email TEXT NOT NULL,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    -- Status can be: 'pending', 'processing', 'sent', 'failed', 'invalid'
    -- ('invalid' rows were rejected at upload and are never sent)
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

-- Lets workers find the next pending job in sub-millisecond time
-- (FOR UPDATE SKIP LOCKED queue claiming).
CREATE INDEX IF NOT EXISTS idx_email_jobs_fetchable
ON email_jobs (run_at, id)
WHERE status = 'pending';

-- Counts jobs per batch quickly when reporting progress.
CREATE INDEX IF NOT EXISTS idx_email_jobs_batch_id
ON email_jobs (batch_id, status);

-- +goose Down
DROP TABLE IF EXISTS email_jobs;
DROP TABLE IF EXISTS batches;
