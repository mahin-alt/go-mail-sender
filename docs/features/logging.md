# Feature: Structured logging

**Status:** done (essentials)
**Branch:** `feat/structured-logging` (based on: `main` at `335e292`)
**Owner / session notes:** the user asked for the essentials only, with the log file at `logs/server.log` and the settings in `.env`.

## Where we left off
> Update this at the end of every chunk of work, in the same commit. A new session reads this first.
- **Last done:** `log/slog` setup with `LOG_LEVEL` / `LOG_FORMAT` / `LOG_FILE`, request log with `X-Request-ID`, every existing `log.Printf` moved to a level, quieter worker logs with masked addresses and a batch-completed summary. Then (same day, after the user found the polling lines too noisy): successful GET requests moved to DEBUG, stage timings on uploads, `send` / `queued` per email, `duration` / `rate` per batch. Tests pass (including DB tests in a scratch schema) and real runs showed the lines as expected.
- **Next:** nothing planned for logging. Proposed next branches: stop dashboard polling when nothing is active (decision 0010 option B), then the large-file work (smaller parse response, upload once; see [large-file-upload.md](large-file-upload.md)).
- **Blocked on / waiting for the user:** nothing; committed and merged into `main`.
- **Found while testing:** uploading the 1M-row file through the UI crashed the PC (browser out of memory: 78 MB response and a 169,547-row table drawn three times). Not a logging issue; the fix is the parse-response work in [large-file-upload.md](large-file-upload.md). A 1M batch sent via Postman also showed there is no way to cancel a batch (proposed as its own feature).
- **Updated:** 2026-11-04

## Why
Plain `log.Printf` lines had no levels, no request log, and a 1M-row batch would write about 2 million lines with customers' addresses in them.

## Scope
**In:** levels, text/json, optional log file, request log with request ID, worker log levels, email masking.
**Out (not now):** rotation, Settings-page log level, request ID inside handler error lines, central log shipping, frontend logs. See [decision 0012](../decisions/0012-structured-logging-slog.md).

## Design and decisions
See [decision 0012](../decisions/0012-structured-logging-slog.md). Smaller points:
- **Decision:** `MarkJobSent` / `MarkJobFailed` now also return `batchDone`, and `checkBatchCompletion` only reports (and only updates) the first time a batch completes. **Because:** the worker needs to know when to write the one "batch completed" line; before, the UPDATE ran again on already completed batches.
- **Decision:** the worker decides "retry" vs "final" from the claimed job's `attempts` and `max_attempts`, the same rule the store uses. **Because:** no extra query.
- **Decision:** the request log wraps CORS, so preflights are logged (DEBUG); the X-Request-ID header is exposed through CORS so the dashboard can read it later.
- Corrected the worker comment that called the retry delay "exponential": it is linear (5 s, 10 s, 15 s).

## Tasks
- [x] Config: `LOG_LEVEL`, `LOG_FORMAT`, `LOG_FILE` with validation
- [x] `internal/logging`: Setup (terminal + file), New, ParseLevel, MaskEmail
- [x] Request log middleware + `X-Request-ID` (exposed via CORS)
- [x] All `log.Printf` calls moved to slog with levels
- [x] Worker: DEBUG sends, WARN retries, ERROR final failures, INFO batch summary, masked addresses
- [x] `.env`, `.env.example`, `.gitignore` (`logs/`)
- [x] Tests: logging package, request log, config, CORS header, store completion reporting
- [x] README section, decision 0012, ROADMAP row
- [x] Successful GET/HEAD requests at DEBUG (polling noise)
- [x] Timings: `recipients parsed` / `batch created` stage times, per-email `send` and `queued`, batch `duration` and `rate`
- [x] `GET /batches/{id}` also returns `created_at` / `completed_at` (needed for the batch duration; additive)

## Verification
- `go build ./... && go vet ./... && go test ./...`, and the DB tests in a scratch schema with `-race`.
- Real run (`api-scratch` launch config, schema `scratch_ui_test`): `/healthz` not logged; a 413 upload logged as WARN with the same `req_id` as the `X-Request-ID` header; settings change, batch created and one batch-completed summary at INFO; no per-email lines at `info`; shutdown lines at the end; a migration failure at startup came out as an ERROR line in the file.

## Notes / history
- 2026-11-04: first startup line used the key `level`, which clashed with slog's own `level`; renamed to `log_level`.
- 2026-11-04: real 1M-row parse logged `parse=6.191s domain_check=467ms total=6.707s` while the request took 6.965s, so about 0.26 s went into writing the 78 MB response.
- 2026-11-04: the `api-scratch` launch config now logs at debug to `logs/scratch.log`, so local checks never mix into `logs/server.log`.
