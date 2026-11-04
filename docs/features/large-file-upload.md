# Feature: Large spreadsheet uploads (1M+ rows)

**Status:** in progress
**Branch:** `feat/upload-size-limit` (based on: `main` at `0a7a573`)
**Owner / session notes:** first goal is only to *parse* 1M+ row files; sending batches that large comes later.

## Where we left off
> Update this at the end of every chunk of work, in the same commit. A new session reads this first.
- **Last done:** `MAX_UPLOAD_MB` setting (instant, default 10, 1-50) on both upload endpoints; frontend reads the limit from Settings. Go + frontend checks pass, and it was run against a scratch-schema API: 18 MB / 1M-row sample parses at 40 MB, 45 MB file is refused, lowering to 10 MB applies with no restart.
- **Next:** shrink the parse response (new branch from `main`). Today it returns every valid row: for the 1M sample that is **78 MB of JSON** (780,493 valid + 169,547 problems), while the UI only uses the count and the first 5 rows. Agreed shape: `valid_count` + `valid_sample` (5 rows, the New Bulk Email preview needs real names), `problems_count` + `problem_counts` per kind (the review tabs count kinds today from the full list, so a capped list would make them wrong) + the first 500 problems, same for `unverified`. Same change for `POST /batches`. Then stream the parser (`Rows()` instead of `GetRows`).
- **Blocked on / waiting for the user:** nothing. The `MAX_UPLOAD_MB` work is committed and merged into `main`; logging comes first, then the response change.
- **Updated:** 2026-11-04

## Why
Staff want to check (and later send) spreadsheets of a million rows or more. The upload limit was a hard-coded 10 MB, and the 1M-row sample (`samples/12_million_mixed.xlsx`) is 18 MB.

## Scope
**In:**
- Configurable upload size limit, changeable at runtime.
- Making the parse/preview of a 1M-row file usable (response size, parser memory).

**Out (not now):**
- Sending 1M-row batches safely: background import, stuck-job reaper, idempotent uploads, delivery throughput (see the audit notes below).

## Design and decisions
- **Decision:** the limit is a Settings entry `MAX_UPLOAD_MB` that applies **instantly** (option A), not on restart (option B). **Because:** it is read on every request, so nothing needs a restart; the user wants to change it back and forth (10 -> 50 -> 40) without downtime. Same pattern as `DOMAIN_CHECK_ENABLED` (an atomic value in the handler, updated by the settings Applier).
- **Decision:** range 1-50 MB; `.env` values above 50 stop the server at startup with a clear error. **Because:** parsing holds the whole sheet in memory, about 13x the file size (18 MB file -> ~233 MB heap to parse, ~900 MB including rendering a batch). The cap stops one upload from exhausting the server.
- **Decision:** the limit is read once at the start of each upload. **Because:** a settings change must never cut off an upload already in progress.
- **Decision:** the request body may be the limit + 1 MB, and the *file part* is checked against the exact limit. **Because:** the multipart boundaries and the subject/body fields would otherwise make a file exactly at the limit fail.
- **Decision:** the frontend reads the limit from `GET /settings` (re-fetched when an upload page opens) and falls back to 10 MB until it loads. The server is the final authority: a tab with an outdated limit gets the server's `413 "file too large: the limit is N MB"`.

## Tasks
- [x] `config.MaxUploadMB` from `MAX_UPLOAD_MB` (default 10, error above 50)
- [x] Settings catalog entry (instant, 1-50)
- [x] Handler: runtime limit, shared `readUpload` for both upload endpoints, `413` names the limit
- [x] Wire initial value and Applier in `main.go`
- [x] Go tests: default limit, exact-limit edge, runtime change, ignored 0, real file under raised limit, parser cases, config load
- [x] Frontend: limit from Settings in the client-side check and the drop-zone hint; setting shown in "Sending behaviour"
- [x] Frontend tests: limit from Settings, `uploadLimitMb` helper
- [x] README, `.env.example`
- [x] Run against a scratch-schema API (see Verification)
- [ ] Parse response: counts + samples instead of full lists
- [ ] Streaming parser (`Rows()` iterator)

## Open questions
- Should the problems preview be capped at 500, or paged from a new endpoint?

## Verification
- `go build ./... && go vet ./... && go test ./...` and the DB tests in a scratch schema.
- `cd frontend && npm run lint && npm run type-check && npx vitest run && npm run build`.
- Manual, against an API on a scratch schema (`api-scratch` and `frontend-scratch` in `.claude/launch.json`, ports 8091 / 5174, schema `scratch_ui_test` created and dropped by hand):
  - Settings -> Sending behaviour shows `MAX_UPLOAD_MB` = 10; save 40 -> "Settings saved".
  - `curl -F file=@samples/12_million_mixed.xlsx .../recipients/parse` -> `200` in ~7 s (78 MB response).
  - 45 MB file -> `413 file too large: the limit is 40 MB`; the UI blocks it before uploading with the same wording.
  - `PUT MAX_UPLOAD_MB=10` with no restart -> the 18 MB file now gets `413 ... 10 MB`; a page left open with 40 shows the server's message; reopening the page shows "Up to 10 MB".
  - `MAX_UPLOAD_MB=51` -> `400`.

## Notes / history
- 2026-11-04: measured the 1M-row sample with the real code in a scratch schema: parse 6.1 s / 233 MB, render 4.0 s / 629 MB, single-transaction insert of 950k rows 8.7 s, one batch = 299 MB on disk, claim + mark-sent ~3 ms per job (~336 jobs/s per worker, DB only). Full audit and proposals (background import, render at send time, batch claiming, rate limiting, counters) were discussed in the session; record them here when that phase starts.
