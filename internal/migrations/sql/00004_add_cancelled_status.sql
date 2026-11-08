-- +goose Up
-- Cancelling a bulk email: a person stops it before every email is sent.
--
-- email_jobs.status gains 'cancelled' (stopped on purpose, never sent, never
-- retried) and batches.status gains 'cancelled'. The status columns were free
-- text until now; the CHECK constraints list the allowed values so a typo
-- such as 'canceled' is rejected by the database itself.
ALTER TABLE email_jobs
    ADD CONSTRAINT email_jobs_status_check
    CHECK (status IN ('pending', 'processing', 'sent', 'failed', 'invalid', 'cancelled'));

ALTER TABLE batches
    ADD CONSTRAINT batches_status_check
    CHECK (status IN ('pending', 'processing', 'completed', 'cancelled'));

-- When the batch was cancelled (NULL for batches that were not).
ALTER TABLE batches ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;

-- +goose Down
-- Turn cancelled rows into the closest old meaning. Jobs become 'invalid', not
-- 'failed': both are never sent, but 'failed' would let Retry send them again.
UPDATE email_jobs SET status = 'invalid' WHERE status = 'cancelled';
UPDATE batches SET status = 'completed', completed_at = COALESCE(completed_at, cancelled_at)
WHERE status = 'cancelled';
ALTER TABLE batches DROP COLUMN IF EXISTS cancelled_at;
ALTER TABLE batches DROP CONSTRAINT IF EXISTS batches_status_check;
ALTER TABLE email_jobs DROP CONSTRAINT IF EXISTS email_jobs_status_check;
