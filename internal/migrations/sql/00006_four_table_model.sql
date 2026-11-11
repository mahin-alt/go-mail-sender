-- +goose Up
-- Four-table model (docs/features/four-table-overhaul.md, decision 0013).
--
-- One uploaded file = one email_batches row. Every spreadsheet row is saved
-- once in email_recipients, which both Verify Emails and New Bulk Email read.
-- email_import_jobs is the background import task (checkpoint + lease), and
-- email_jobs holds one row per email to deliver, created only after the user
-- clicks Send. Emails are built at send time from the batch's templates, so
-- email_jobs keeps no copy of names, addresses, subjects or bodies.
--
-- The old tables are dropped without copying their data (the user backed the
-- database up first). app_settings is kept, so saved SMTP settings survive.

DROP TABLE IF EXISTS upload_rows;
DROP TABLE IF EXISTS uploads;
DROP TABLE IF EXISTS email_jobs;
DROP TABLE IF EXISTS batches;

-- email_batches: one row per uploaded file. Counters let the dashboard read a
-- small summary instead of counting millions of rows on every poll; they are
-- updated in the same transaction as the rows they count.
CREATE TABLE email_batches (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    type              TEXT NOT NULL CHECK (type IN ('verification', 'bulk_send')),
    original_filename TEXT NOT NULL DEFAULT '',  -- shown in the UI only, never used as a path
    status            TEXT NOT NULL DEFAULT 'importing' CHECK (status IN (
                          'importing', 'import_failed', 'draft', 'generating_jobs', 'queued',
                          'sending', 'completed', 'completed_with_errors', 'failed', 'cancelled')),
    subject_template  TEXT,                      -- set when Send is clicked
    body_template     TEXT,

    -- import (spreadsheet rows)
    total_rows        INT NOT NULL DEFAULT 0,    -- non-empty data rows saved
    processed_rows    INT NOT NULL DEFAULT 0,    -- spreadsheet rows read so far (blank rows included)
    valid_count       INT NOT NULL DEFAULT 0,
    unverified_count  INT NOT NULL DEFAULT 0,    -- domain could not be checked; still sent
    invalid_count     INT NOT NULL DEFAULT 0,
    invalid_counts    JSONB NOT NULL DEFAULT
                      '{"empty_email": 0, "empty_name": 0, "invalid_email": 0, "bad_domain": 0}',
    import_error      TEXT,

    -- delivery (email_jobs)
    pending_count     INT NOT NULL DEFAULT 0,
    processing_count  INT NOT NULL DEFAULT 0,
    sent_count        INT NOT NULL DEFAULT 0,
    failed_count      INT NOT NULL DEFAULT 0,
    cancelled_count   INT NOT NULL DEFAULT 0,

    -- job generation (one per batch, so it lives here instead of in its own table)
    job_generation_status       TEXT CHECK (job_generation_status IN ('pending', 'processing', 'completed', 'failed')),
    last_generated_recipient_id BIGINT NOT NULL DEFAULT 0,  -- checkpoint
    job_generation_attempts     INT NOT NULL DEFAULT 0,
    job_generation_locked_at    TIMESTAMPTZ,                -- lease
    job_generation_locked_by    TEXT,
    job_generation_error        TEXT,

    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    send_requested_at TIMESTAMPTZ,               -- when Send was clicked
    completed_at      TIMESTAMPTZ,
    cancelled_at      TIMESTAMPTZ
);

-- The Bulk Emails list, newest first.
CREATE INDEX idx_email_batches_created ON email_batches (created_at DESC, id DESC);

-- email_recipients: one row per non-empty spreadsheet row, valid or not.
CREATE TABLE email_recipients (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    batch_id          BIGINT NOT NULL REFERENCES email_batches(id) ON DELETE CASCADE,
    row_number        INT NOT NULL,              -- Excel row (header is row 1)
    name              TEXT NOT NULL DEFAULT '',  -- TEXT on purpose: an over-long cell is saved as invalid, not a failed chunk
    email             TEXT NOT NULL DEFAULT '',
    validation_status TEXT NOT NULL CHECK (validation_status IN ('valid', 'unverified', 'invalid')),
    validation_reason TEXT CHECK (validation_reason IN ('empty_email', 'empty_name', 'invalid_email', 'bad_domain')),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- A reason exactly when the row is invalid.
    CHECK ((validation_status = 'invalid') = (validation_reason IS NOT NULL))
);

-- A retried import chunk can never save the same spreadsheet row twice.
CREATE UNIQUE INDEX idx_email_recipients_row ON email_recipients (batch_id, row_number);
-- Issues tabs: invalid or unverified rows of a batch, in order.
CREATE INDEX idx_email_recipients_status ON email_recipients (batch_id, validation_status, id);
-- Per-reason tabs; only invalid rows, so it stays small.
CREATE INDEX idx_email_recipients_reason ON email_recipients (batch_id, validation_reason, id)
WHERE validation_reason IS NOT NULL;
-- Rows that will be sent (valid + unverified), in order: job generation and the preview sample.
CREATE INDEX idx_email_recipients_sendable ON email_recipients (batch_id, id)
WHERE validation_status <> 'invalid';

-- email_import_jobs: the background import of one batch's file.
CREATE TABLE email_import_jobs (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    batch_id           BIGINT NOT NULL UNIQUE REFERENCES email_batches(id) ON DELETE CASCADE,
    status             TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    last_processed_row INT NOT NULL DEFAULT 0,  -- checkpoint: last spreadsheet row whose chunk is saved
    attempts           INT NOT NULL DEFAULT 0,
    max_attempts       INT NOT NULL DEFAULT 3,  -- a file that keeps crashing the importer is given up on
    locked_at          TIMESTAMPTZ,             -- lease
    locked_by          TEXT,
    last_error         TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_email_import_jobs_open ON email_import_jobs (id)
WHERE status IN ('pending', 'processing');

-- email_jobs: one email to deliver.
CREATE TABLE email_jobs (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    batch_id     BIGINT NOT NULL REFERENCES email_batches(id) ON DELETE CASCADE,
    recipient_id BIGINT NOT NULL UNIQUE REFERENCES email_recipients(id) ON DELETE CASCADE, -- one email per row
    status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'sent', 'failed', 'cancelled')),
    attempts     INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), -- next allowed try
    locked_at    TIMESTAMPTZ,                        -- lease
    locked_by    TEXT,
    last_error   TEXT,
    sent_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Workers claim the next due email of a sending batch.
CREATE INDEX idx_email_jobs_claim ON email_jobs (batch_id, available_at, id) WHERE status = 'pending';
-- A batch's emails in order, with or without a status filter.
CREATE INDEX idx_email_jobs_batch ON email_jobs (batch_id, id);
CREATE INDEX idx_email_jobs_batch_status ON email_jobs (batch_id, status, id);
-- Dashboard stats (same idea as migration 00003).
CREATE INDEX idx_email_jobs_sent_at ON email_jobs (sent_at) WHERE status = 'sent';
CREATE INDEX idx_email_jobs_failed ON email_jobs (updated_at, batch_id) WHERE status = 'failed';
-- Finding emails stuck in 'processing' after a crash.
CREATE INDEX idx_email_jobs_processing ON email_jobs (locked_at) WHERE status = 'processing';

-- +goose Down
-- Back to the schema of migration 00005, empty.
DROP TABLE IF EXISTS email_jobs;
DROP TABLE IF EXISTS email_import_jobs;
DROP TABLE IF EXISTS email_recipients;
DROP TABLE IF EXISTS email_batches;

CREATE TABLE batches (
    id BIGSERIAL PRIMARY KEY,
    filename TEXT NOT NULL DEFAULT '',
    subject_template TEXT NOT NULL,
    body_template TEXT NOT NULL,
    total_jobs INT NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    CONSTRAINT batches_status_check CHECK (status IN ('pending', 'processing', 'completed', 'cancelled'))
);

CREATE TABLE email_jobs (
    id BIGSERIAL PRIMARY KEY,
    batch_id BIGINT NOT NULL REFERENCES batches(id) ON DELETE CASCADE,
    recipient_name TEXT NOT NULL,
    recipient_email TEXT NOT NULL,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    last_error TEXT,
    locked_at TIMESTAMPTZ,
    run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ,
    CONSTRAINT email_jobs_status_check CHECK (status IN ('pending', 'processing', 'sent', 'failed', 'invalid', 'cancelled'))
);

CREATE INDEX idx_email_jobs_fetchable ON email_jobs (run_at, id) WHERE status = 'pending';
CREATE INDEX idx_email_jobs_batch_id ON email_jobs (batch_id, status);
CREATE INDEX idx_email_jobs_sent_at ON email_jobs (sent_at) WHERE status = 'sent';
CREATE INDEX idx_email_jobs_failed ON email_jobs (updated_at, batch_id) WHERE status = 'failed';
CREATE INDEX idx_email_jobs_processing ON email_jobs (id) WHERE status = 'processing';

CREATE TABLE uploads (
    id               BIGSERIAL PRIMARY KEY,
    filename         TEXT NOT NULL DEFAULT '',
    file_sha256      TEXT NOT NULL DEFAULT '',
    status           VARCHAR(16) NOT NULL DEFAULT 'importing'
                     CHECK (status IN ('importing', 'ready', 'failed')),
    total_rows       INT NOT NULL DEFAULT 0,
    valid_count      INT NOT NULL DEFAULT 0,
    problem_count    INT NOT NULL DEFAULT 0,
    unverified_count INT NOT NULL DEFAULT 0,
    problem_counts   JSONB NOT NULL DEFAULT '{}',
    error            TEXT,
    batch_id         BIGINT REFERENCES batches(id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at       TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours'
);
CREATE INDEX idx_uploads_expires_at ON uploads (expires_at);

CREATE TABLE upload_rows (
    upload_id BIGINT NOT NULL REFERENCES uploads(id) ON DELETE CASCADE,
    row_num   INT NOT NULL,
    name      TEXT NOT NULL DEFAULT '',
    email     TEXT NOT NULL DEFAULT '',
    kind      VARCHAR(16) NOT NULL CHECK (kind IN (
                  'ok', 'unverified',
                  'empty_email', 'empty_name', 'invalid_email', 'duplicate', 'bad_domain')),
    reason    TEXT,
    PRIMARY KEY (upload_id, row_num)
);
CREATE INDEX idx_upload_rows_skipped ON upload_rows (upload_id, row_num)
WHERE kind NOT IN ('ok', 'unverified');
CREATE INDEX idx_upload_rows_kind ON upload_rows (upload_id, kind, row_num)
WHERE kind <> 'ok';
