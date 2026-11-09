-- +goose Up
-- Checked spreadsheets ("uploads") are kept apart from sent emails.
--
-- Choosing a file on Verify Emails or New Bulk Email parses it once and saves
-- every row here with its result, so the page can show counts and page through
-- the problems without the server sending every row back. email_jobs keeps
-- holding only emails that were actually sent (decision in
-- docs/features/large-file-upload.md). Uploads are temporary: the server
-- deletes them after expires_at, and their rows go with them (CASCADE).

-- uploads: one checked file, with its totals worked out once while parsing.
CREATE TABLE IF NOT EXISTS uploads (
    id               BIGSERIAL PRIMARY KEY,
    filename         TEXT NOT NULL DEFAULT '',
    file_sha256      TEXT NOT NULL DEFAULT '',  -- fingerprint of the file, to spot the same file again
    status           VARCHAR(16) NOT NULL DEFAULT 'importing'
                     CHECK (status IN ('importing', 'ready', 'failed')),
    total_rows       INT NOT NULL DEFAULT 0,    -- non-empty data rows
    valid_count      INT NOT NULL DEFAULT 0,    -- rows that will be sent (ok + unverified)
    problem_count    INT NOT NULL DEFAULT 0,    -- rows that will be skipped
    unverified_count INT NOT NULL DEFAULT 0,    -- sent, but the domain could not be checked
    problem_counts   JSONB NOT NULL DEFAULT '{}', -- skipped rows per kind, e.g. {"duplicate": 12}
    error            TEXT,                      -- why it failed, when status = 'failed'
    batch_id         BIGINT REFERENCES batches(id) ON DELETE SET NULL, -- the bulk email sent from it
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at       TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours'
);

-- The cleanup loop looks for expired uploads.
CREATE INDEX IF NOT EXISTS idx_uploads_expires_at ON uploads (expires_at);

-- upload_rows: every non-empty data row of a checked file, with its result.
-- kind: 'ok' and 'unverified' will be sent; the others are skipped.
CREATE TABLE IF NOT EXISTS upload_rows (
    upload_id BIGINT NOT NULL REFERENCES uploads(id) ON DELETE CASCADE,
    row_num   INT NOT NULL,                     -- spreadsheet row (header is row 1)
    name      TEXT NOT NULL DEFAULT '',
    email     TEXT NOT NULL DEFAULT '',
    kind      VARCHAR(16) NOT NULL CHECK (kind IN (
                  'ok', 'unverified',
                  'empty_email', 'empty_name', 'invalid_email', 'duplicate', 'bad_domain')),
    reason    TEXT,                             -- human readable; NULL for 'ok'
    PRIMARY KEY (upload_id, row_num)
);

-- "All problems": skipped rows in spreadsheet order, read a page at a time
-- ("rows after N"). Partial, so the valid rows do not make it bigger.
CREATE INDEX IF NOT EXISTS idx_upload_rows_skipped
ON upload_rows (upload_id, row_num)
WHERE kind NOT IN ('ok', 'unverified');

-- One kind at a time (the Duplicate, Invalid, Bad domain and Unverified tabs).
CREATE INDEX IF NOT EXISTS idx_upload_rows_kind
ON upload_rows (upload_id, kind, row_num)
WHERE kind <> 'ok';

-- +goose Down
DROP TABLE IF EXISTS upload_rows;
DROP TABLE IF EXISTS uploads;
