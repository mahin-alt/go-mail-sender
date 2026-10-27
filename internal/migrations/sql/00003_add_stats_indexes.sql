-- Small partial indexes that keep the dashboard's stats queries fast as
-- email_jobs grows: each covers only the rows one stats query looks at.
--
-- CREATE INDEX CONCURRENTLY builds the index without blocking inserts and
-- updates on a live table, but PostgreSQL refuses to run it inside a
-- transaction, hence NO TRANSACTION (goose normally wraps each migration in one).

-- +goose NO TRANSACTION

-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_jobs_sent_at
ON email_jobs (sent_at) WHERE status = 'sent';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_jobs_failed
ON email_jobs (updated_at, batch_id) WHERE status = 'failed';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_jobs_processing
ON email_jobs (id) WHERE status = 'processing';

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS idx_email_jobs_processing;
DROP INDEX CONCURRENTLY IF EXISTS idx_email_jobs_failed;
DROP INDEX CONCURRENTLY IF EXISTS idx_email_jobs_sent_at;
