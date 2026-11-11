# Feature: Four-table overhaul (batches, recipients, import jobs, email jobs)

**Status:** done (not committed yet)
**Branch:** `refactor/database-schema-changes-with-system-overhaul` (based on: `main` at `c03b967`)
**Owner / session notes:** source plan: [excel_email_system_complete_plan_chatgpt.md](../excel_email_system_complete_plan_chatgpt.md). Decisions below override it where they differ. Project-wide summary: [decision 0013](../decisions/0013-four-table-data-model.md).

## Where we left off
> Update this at the end of every chunk of work, in the same commit. A new session reads this first.
- **Last done:** (2026-11-09) All five phases built and verified: backend (migration 00006, store, importer, generator, sending pool, `/verify` + `/batches` API), frontend on the new API (Verify Emails, New Bulk Email, Bulk Emails list/detail, Dashboard), docs (README, CLAUDE.md, PROJECT, FEATURES, decisions 0007/0013, Postman, `.env.example`, bench script), end-to-end in the browser, crash tests with `kill -9` during import and job generation, 1M benchmark.
- **Next:** the user reviews and commits (nothing committed yet), then merges into `main`. Starting the real dev server applies 00006, which drops the old tables (backup taken). Later items are on the ROADMAP (delete batch, temporary upload files, separate worker, faster search, periodic counter reconcile).
- **Blocked on / waiting for the user:** review and commit.
- **Updated:** 2026-11-09

## Why
Today a checked file lives in `uploads` / `upload_rows`, and sending copies every row again into `email_jobs` with its own rendered subject and body (about 1.9M jobs make the dev database 826 MB). The upload request also does all the parsing while the browser waits. The new model keeps **one place for every spreadsheet row** (`email_recipients`), which both Verify Emails and New Bulk Email read, does the import in the background with crash recovery, creates delivery jobs only after the user confirms, and builds each email at send time.

## Scope
**In:**
- New migration with the 4 tables; the old `batches`, `email_jobs`, `uploads`, `upload_rows` are dropped (`app_settings` is kept, so SMTP settings survive).
- Background import worker (streaming `.xlsx`, chunks, checkpoint, lease, domain check), job-generation worker, sending workers rewritten for the new `email_jobs`.
- API under `/verify` and `/batches` (table below); single email, cancel, retry batch, retry one job kept.
- Frontend moved to the new API: Verify Emails, New Bulk Email, Bulk Emails list and detail, Dashboard (stats shape unchanged).
- Docs, README API table, FEATURES.md, CLAUDE.md conventions, Postman collection.

**Out (not now, noted in [ROADMAP](../ROADMAP.md)):**
- Delete a batch (uploads are permanent, so the database grows).
- A separate worker process (`cmd/worker`); everything stays in one Go process for now.
- Deleting the saved `.xlsx` after import (kept permanently for now; may become temporary later).
- Flagging duplicate emails, content-hash duplicate warning (`file_sha256` dropped).
- Login, send-operations / sending one batch twice, editing rows.

## Design and decisions

- **Decision:** 4 tables `email_batches`, `email_recipients`, `email_import_jobs`, `email_jobs`; job generation state lives on `email_batches` (no fifth table). **Because:** one generation per batch; the plan's reasoning.
- **Decision:** one new migration drops the old tables and creates the new ones (no data copied). Its Down recreates the old tables empty. **Because:** the user starts fresh after a `pg_dump` backup; applied migrations are never edited.
- **Decision:** the uploaded `.xlsx` is saved to disk at `UPLOAD_DIR/batch-<id>.xlsx` (new setting `UPLOAD_DIR`, default `data/uploads`, gitignored) and **kept permanently**. No `storage_key` column: the path comes from the batch id. **Because:** a background import needs the file to resume after a crash; the user may make it temporary later (delete after import), which only changes the import worker.
- **Decision:** no `file_sha256`. **Because:** nothing used it; duplicate-upload warning is not wanted now.
- **Decision:** statuses follow the plan: bulk batches stop at `draft` after import, verification batches end at `completed`. A `completed` verification batch can still be sent (verify first, send later), so `POST /batches/{id}/send` accepts `draft` and `completed`-verification batches. **Because:** user's choice.
- **Decision:** duplicates are not flagged; a repeated email is a normal `valid` row and is sent. **Because:** user's choice.
- **Decision:** domain check (DNS/MX) kept, done by the import worker with a per-import cache of domain results; `unverified` rows are sent, `bad_domain` rows are not. The Settings switch is read when an import starts. **Because:** existing feature the user wants.
- **Decision:** `subject_template` and `body_template` live on `email_batches`, set by `POST /batches/{id}/send`, validated there, and rendered per email by the sending worker. `email_jobs` stores no name, email, subject or body. **Because:** removes the largest copy of data; same output since the inputs never change.
- **Decision:** `TEXT` + `CHECK` lists for statuses and codes, `TEXT` for names and emails (no `VARCHAR(320)`). **Because:** Postgres TEXT is as fast; an over-long garbage cell must be saved as an invalid row, not fail its whole chunk; Go enforces lengths.
- **Decision:** IDs `BIGINT GENERATED ALWAYS AS IDENTITY`; counts and row numbers `INT`. **Because:** IDs grow forever across batches; one sheet has at most 1,048,576 rows.
- **Decision:** `email_recipients` keeps `created_at` and `updated_at`. **Because:** user's choice.
- **Decision:** the issues endpoint takes several reasons at once (`?reason=empty_email,empty_name`), so the review table keeps its "Invalid" tab (three reasons) next to "Bad domain" and "Unverified". The "Duplicate" tab is gone. **Because:** duplicates are not flagged any more.
- **Decision:** Verify Emails hands a checked file to New Bulk Email as `?batch=<id>` (the Pinia store that carried the browser's File is gone); a draft in the Bulk Emails list opens the same way ("Write and send"). A verification batch becomes `bulk_send` when sent, so the Bulk Emails list (`type=bulk_send`) shows everything that was sent and every unsent bulk upload. **Because:** the server holds the checked file; nothing is uploaded twice.
- **Decision:** the frontend shows three stages: "Uploading N%" (browser), "Checking every row: N read so far" (server import, polled every second), then the result. **Because:** the plan asks that upload, import and delivery progress are told apart.
- **Decision:** paging. `/batches` list keeps page numbers (`limit`/`offset` + `total`; the table is small). `/issues` and `/emails` use cursors (`?after=<id>&limit=`, response `next_after`), totals come from the batch counters. **Because:** offset paging over a million rows gets slower page by page; counters avoid `COUNT(*)`.
- **Decision:** counters on `email_batches` are updated in the same transaction as the rows they count (import chunk, generation chunk, each job result, retry, cancel). **Because:** the summary must match the rows; per-email batch-row updates are fine at 5 workers, and the plan's "aggregate later" stays an option if it becomes a bottleneck.
- **Decision:** tasks are claimed atomically (`UPDATE ... WHERE id = (SELECT ... FOR UPDATE SKIP LOCKED) RETURNING`), with a lease (`locked_at`, `locked_by`) renewed each chunk; a task whose lease is older than the lease time (60 s) can be claimed by another worker. Progress is written only if the worker still owns the lease. **Because:** crash recovery without two workers on one task.
- **Decision:** sending workers only claim jobs of batches in `queued` / `sending`. **Because:** no sending while generation is partial.
- **Decision:** retry resets the same job row (`failed` -> `pending`, attempts 0) and moves the batch back to `sending`. **Because:** keeps "one job per recipient" (`UNIQUE recipient_id`), no send-operations table needed.
- **Decision:** cancel works in `generating_jobs`, `queued`, `sending`: generation stops, `pending` jobs become `cancelled`, the batch becomes `cancelled`; an email being sent at that moment finishes. **Because:** same meaning as today's cancel.
- **Decision:** one Go process runs the API, 1 import worker, 1 generation worker, the sending workers (`WORKER_COUNT`) and the orphan-file cleanup. The API wakes the right worker after an upload or send. **Because:** simplest to run; a separate worker process is on the ROADMAP.
- **Decision:** single email (`POST /emails`) is unchanged: sent straight away, not stored.
- **Decision:** `/stats` keeps its response shape; its queries move to the new tables ("high invalid rate" uses batch counters).

### Schema

`email_batches` (one row per uploaded file)

| Column | Type | Notes |
|---|---|---|
| `id` | BIGINT identity PK | |
| `type` | TEXT CHECK (`verification`, `bulk_send`) | the page that created it |
| `original_filename` | TEXT NOT NULL DEFAULT '' | display only, never a path |
| `status` | TEXT CHECK | `importing`, `import_failed`, `draft`, `generating_jobs`, `queued`, `sending`, `completed`, `completed_with_errors`, `failed`, `cancelled` |
| `subject_template`, `body_template` | TEXT NULL | set on send |
| `total_rows`, `processed_rows` | INT DEFAULT 0 | import progress |
| `valid_count`, `unverified_count`, `invalid_count` | INT DEFAULT 0 | sendable = valid + unverified |
| `invalid_counts` | JSONB DEFAULT '{}' | per reason, e.g. `{"bad_domain": 40}` |
| `pending_count`, `processing_count`, `sent_count`, `failed_count`, `cancelled_count` | INT DEFAULT 0 | delivery progress |
| `import_error` | TEXT | |
| `job_generation_status` | TEXT NULL CHECK (`pending`, `processing`, `completed`, `failed`) | NULL = send never requested |
| `last_generated_recipient_id` | BIGINT DEFAULT 0 | generation checkpoint |
| `job_generation_attempts` | INT DEFAULT 0 | |
| `job_generation_locked_at`, `job_generation_locked_by` | TIMESTAMPTZ, TEXT | lease |
| `job_generation_error` | TEXT | |
| `created_at`, `updated_at` | TIMESTAMPTZ DEFAULT now() | |
| `send_requested_at` | TIMESTAMPTZ | when Send was clicked; sending duration starts here |
| `completed_at`, `cancelled_at` | TIMESTAMPTZ | |

Index: `(created_at DESC, id DESC)`. `failed` is used only when a batch cannot continue (generation failed for good); all-jobs-failed is `completed_with_errors`.

`email_recipients` (one row per non-empty spreadsheet row)

| Column | Type | Notes |
|---|---|---|
| `id` | BIGINT identity PK | |
| `batch_id` | BIGINT NOT NULL FK ON DELETE CASCADE | |
| `row_number` | INT NOT NULL | Excel row (header is row 1) |
| `name`, `email` | TEXT NOT NULL DEFAULT '' | trimmed |
| `validation_status` | TEXT CHECK (`valid`, `unverified`, `invalid`) | |
| `validation_reason` | TEXT NULL CHECK (`empty_email`, `empty_name`, `invalid_email`, `bad_domain`) | NULL when not invalid |
| `created_at`, `updated_at` | TIMESTAMPTZ DEFAULT now() | |

Indexes: `UNIQUE (batch_id, row_number)`; `(batch_id, validation_status, id)`; `(batch_id, validation_reason, id) WHERE validation_reason IS NOT NULL`.

`email_import_jobs` (one per batch)

| Column | Type | Notes |
|---|---|---|
| `id` | BIGINT identity PK | |
| `batch_id` | BIGINT NOT NULL UNIQUE FK CASCADE | |
| `status` | TEXT CHECK (`pending`, `processing`, `completed`, `failed`) | |
| `last_processed_row` | INT DEFAULT 0 | checkpoint |
| `attempts`, `max_attempts` | INT DEFAULT 0 / 3 | cap so a file that crashes the importer is not retried forever |
| `locked_at`, `locked_by` | TIMESTAMPTZ, TEXT | lease |
| `last_error` | TEXT | |
| `created_at`, `updated_at` | TIMESTAMPTZ | |

`email_jobs` (one per email to deliver, created after Send)

| Column | Type | Notes |
|---|---|---|
| `id` | BIGINT identity PK | |
| `batch_id` | BIGINT NOT NULL FK CASCADE | copied from the recipient in the same `INSERT ... SELECT` |
| `recipient_id` | BIGINT NOT NULL UNIQUE FK CASCADE | one email per row |
| `status` | TEXT CHECK (`pending`, `processing`, `sent`, `failed`, `cancelled`) | |
| `attempts`, `max_attempts` | INT DEFAULT 0 / `MAX_JOB_RETRIES` | |
| `available_at` | TIMESTAMPTZ DEFAULT now() | next allowed try |
| `locked_at`, `locked_by` | TIMESTAMPTZ, TEXT | lease |
| `last_error` | TEXT | |
| `sent_at`, `created_at`, `updated_at` | TIMESTAMPTZ | |

Indexes: `(available_at, id) WHERE status = 'pending'`; `(batch_id, status, id)`; `(sent_at) WHERE status = 'sent'`; `(updated_at, batch_id) WHERE status = 'failed'`; `(locked_at) WHERE status = 'processing'`.

### API (`/api/v1`)

| Method | Path | Purpose |
|---|---|---|
| POST | `/verify` | upload `.xlsx`, creates a `verification` batch; `202 {"batch_id", "status"}` |
| GET | `/verify/{id}` | summary + import progress |
| GET | `/verify/{id}/issues` | invalid and unverified rows; `?reason=`, `?status=unverified`, `?after=`, `?limit=` |
| POST | `/batches` | upload `.xlsx`, creates a `bulk_send` batch; `202` |
| GET | `/batches` | list; `?type=`, `?status=`, `?q=` (filename), `?limit=`, `?offset=` |
| GET | `/batches/{id}` | summary + progress |
| GET | `/batches/{id}/issues` | same as `/verify/{id}/issues` |
| GET | `/batches/{id}/sample` | first few sendable rows (default 5, max 20) for the New Bulk Email preview |
| POST | `/batches/{id}/send` | `{"subject", "body"}`; validates templates, starts generation; `409` if not sendable or already sent |
| GET | `/batches/{id}/emails` | delivery results; `?status=`, `?q=` (name/email), `?after=`, `?limit=` |
| POST | `/batches/{id}/cancel` | stop sending |
| POST | `/batches/{id}/retry` | retry every failed email |
| POST | `/jobs/{id}/retry` | retry one failed email |
| POST | `/emails` | single email (unchanged) |
| unchanged | `/healthz`, `/stats`, `/settings`, `/settings/smtp/test` | |

Removed: `/uploads/*`, the old `POST /batches` that took the file and templates in one request.

### Lifecycle

```
upload -> importing --(import ok)--> draft (bulk_send) / completed (verification)
                     \-(import fails 3x)--> import_failed
draft or completed verification --POST send--> generating_jobs --(all jobs made)--> queued
queued --(first claim)--> sending --(all jobs sent/failed)--> completed | completed_with_errors
generating_jobs / queued / sending --cancel--> cancelled
completed_with_errors --retry--> sending
```

## Tasks

### Before starting (user)
- [x] Back up the dev database with `pg_dump --format=custom` (commands in chat on 2026-11-09; keep the file outside the repo) and check it restores.
- [x] Branch `refactor/database-schema-changes-with-system-overhaul` from `main`.

### Phase 1: schema and store
- [x] Migration `00006`: drop `upload_rows`, `uploads`, `email_jobs`, `batches`; create the 4 tables, CHECKs and indexes; Down recreates the old tables.
- [x] Store: create batch + import job, claim/renew/finish import job, save recipient chunk + checkpoint + counters in one transaction.
- [x] Store: request send (state check, idempotent), claim generation, generate chunk + checkpoint, finish generation -> `queued`.
- [x] Store: claim email job (only `queued`/`sending` batches, expired leases reclaimed), mark sent / failed / retry later with counters, batch completion.
- [x] Store: list batches, get batch, list issues, sample, list emails (cursor + search), cancel, retry batch, retry job.
- [x] Store: stats queries on the new tables.
- [x] DB tests (scratch schema): constraints, chunk + checkpoint atomic, re-claim after expired lease without duplicates, generation resume without duplicates, two workers never claim the same job, cancel/retry counters.

### Phase 2: upload and import
- [x] `UPLOAD_DIR` setting; stream the upload straight to `batch-<id>.xlsx` with `r.MultipartReader()` + `io.Copy` (no whole-file buffer, no extra temp copy, size limit still enforced); orphan file cleanup.
- [x] Recipients scanner: drop duplicate detection (its `seen` map was the last thing growing with every row), resume from a row number; measure peak memory on the 1M sample.
- [x] Import worker: claim, stream rows, domain check with per-import cache, chunks of 1,000, lease renewal, retry up to 3, final status.
- [x] `POST /verify`, `POST /batches` (202), `GET /verify/{id}`, `GET /batches/{id}`, issues, sample.
- [x] Tests: handler tests, import worker test incl. a forced stop mid-file and resume.

### Phase 3: send, cancel, retry
- [x] `POST /batches/{id}/send`, generation worker (chunks of 5,000).
- [x] Sending workers: claim, render from batch templates (cached per batch), send, record result.
- [x] Cancel, retry batch, retry job, `GET /batches/{id}/emails`, `GET /batches`.
- [x] Remove old upload/batch code, wire everything in `main.go`.
- [x] Tests: end-to-end with the fake SMTP server; double-click send gives one generation.

### Phase 4: frontend
- [x] Types, API client, queries for `/verify` and `/batches`.
- [x] Verify Emails: upload -> import progress -> counts + issues tabs; "Continue to New Bulk Email" passes the same batch.
- [x] New Bulk Email: upload (or batch from Verify) -> import progress -> review -> write + preview from `/sample` -> confirm -> send.
- [x] Bulk Emails list (type/status tabs) and detail (counters, emails with cursor paging + search, cancel, retry).
- [x] Dashboard still works with `/stats`.
- [x] Lint, type-check, Vitest, build.

### Phase 5: docs and checks
- [x] README API table and setup (`UPLOAD_DIR`), CLAUDE.md conventions (table names), FEATURES.md, PROJECT.md glossary, Postman collection, `.env.example`.
- [x] End-to-end in the browser on a scratch schema with Mailpit: small file, 1M sample, kill the server mid-import and mid-send and see it resume.
- [x] Benchmark with `scripts/bench.sh` (adapted) and record numbers.

## Open questions
- None blocking. Later: make the saved `.xlsx` temporary; delete batch; separate worker process; whether per-email counter updates need batching at higher worker counts.

## Verification
- `go build ./... && go vet ./... && go test ./...`; DB tests on a scratch schema with `-p 1 -race`: all packages pass (store, importer, worker, api flow tests included).
- `cd frontend && npm run lint && npm run type-check && npx vitest run && npm run build`: pass (166 tests).
- Real run, 2026-11-09, scratch schema `scratch_ui_test`, Mailpit, `api-scratch` + `frontend-scratch` from `.claude/launch.json`:
  - Verify Emails in the browser with `9_mixed_everything.xlsx`: result card, tabs and reasons; "Continue to New Bulk Email" opened `?batch=1`; Send -> generating -> sent; Mailpit received "Update on your order, Eloise Dupont".
  - 1M sample (`12_million_mixed.xlsx`, 18.8 MB) via `POST /batches`: `202` in 23 ms. The server was killed with `kill -9` at row ~310k; after restart a new worker took the task over when the 60 s lease ran out and finished from the checkpoint (row 310,490). Result identical to a clean import of the same file (950,040 rows, 780,493 valid, 169,547 invalid, no missing or duplicate row numbers).
  - Clean 1M import: 16.8 s (56k rows/s), whole server peak RSS 62 MB.
  - Send on the 1M batch: `202`, a second click `409`. Killed with `kill -9` at 305,000 generated emails; after restart generation resumed (attempt 2, 475,493 more), exactly 780,493 emails, one per sendable row. Sending started; Cancel stopped 779,712 unsent emails after 781 sent; counters add up.
  - Reads on the 1M batch: summary, lists, email pages (first, deep, by status), issues, sample: 0-1 ms; `/stats` 49 ms; email search with no match 560 ms (scans every row).
- Benchmark: [docs/benchmarks/2026-11-09-four-tables](../benchmarks/2026-11-09-four-tables/results.md): upload answers in 0.03 s, check done in ~16.5 s (Phase 1: 10.3 s in the request), peak 59-90 MB (Phase 1: ~140), emails created in 10.8 s, 315 emails/s, every read 1-5 ms except search without a match (~660 ms) and stats (56 ms). About 230 MB of `email_recipients` per million-row file. A second run the same day repeated the numbers (sending 346/s). Sending is slower than the original baseline (475/s): see [the comparison](../benchmarks/2026-11-09-comparison.md).

## Notes / history
- 2026-11-09: built in one session on `refactor/database-schema-changes-with-system-overhaul` (no commits yet, the user decides). Found while building: workers and Cancel/Retry must take row locks in the same order (email row first, batch row second) or they can deadlock; the claim is a short transaction that checks the batch status under its lock, and Cancel runs a second pass for emails the generator committed while it waited.
- Known gaps / later: the KPI tiles overflow with 6-digit numbers on narrow windows (the deferred responsive list); email search without a status scans the whole batch (a trigram index could fix it); counters are kept in step transactionally and `ReconcileBatchCounters` exists, but nothing runs it periodically yet.
- 2026-11-09: user decisions in chat: 4 tables, `/verify` + `/batches` names, templates on the batch rendered at send time, keep domain check, no duplicate flag, keep single email / cancel / retry, uploads permanent, file kept on disk permanently (maybe temporary later), no `file_sha256`, no `storage_key`, plan's `draft`/`completed` statuses, `created_at`/`updated_at` on recipients, keep `/sample`, one Go process.
