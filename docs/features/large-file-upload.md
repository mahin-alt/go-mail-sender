# Feature: Large spreadsheet uploads (1M+ rows)

**Status:** in progress
**Branch:** `feat/upload-size-limit` (based on: `main` at `0a7a573`)
**Owner / session notes:** started as "parse 1M+ row files"; on 2026-11-08 a 1M-row test crashed the browser tab, and the scope grew into the 4-phase scaling plan below (upload, import, review and send all made to handle 1M+ rows).

## Where we left off
> Update this at the end of every chunk of work, in the same commit. A new session reads this first.
- **Last done:** (2026-11-08) Audit of the whole upload -> parse -> store -> send -> display path after a 1M-row file crashed the browser tab. The audit, the 4-phase plan and the route plan are recorded under "Scaling plan" below. A disconnect test on a scratch schema confirmed that a request cut off half way saves nothing (see "Tab closed half way").
  Then (branch `chore/benchmark-baseline`): benchmark tooling (`scripts/bench.sh`, `internal/recipients/parse_bench_test.go`, guide in `docs/benchmarks/README.md`) and the baseline run on the 1M sample in `docs/benchmarks/2026-11-08-baseline/` (summary under "Baseline numbers" below).
- **Next:** rewrite Phases 1-2 below to the newer requirements (see Open questions: no valid emails in any response, every problem must stay viewable, problems from their own paged endpoint), then start Phase 1 on a new branch from `main`.
- **Blocked on / waiting for the user:** (1) commit the benchmark branch and move the tag `bench/baseline-2026-11-08` onto that commit (today it points at `86cffbe`, which does not contain the benchmark tooling, so a re-run from the tag would need the files copied in); (2) storage choice for check results: separate `uploads` + `upload_rows` tables (recommended) or `batches` + `email_jobs` with a `draft` status; (3) where the email preview gets its 5 names (recommended: `GET /uploads/{id}/sample`).
- **Updated:** 2026-11-08 (benchmark baseline)

## Why
Staff want to check and send spreadsheets of a million rows or more. The upload limit was a hard-coded 10 MB, and the 1M-row sample (`samples/12_million_mixed.xlsx`) is 18 MB. With the limit raised, a 1M-row check crashed the browser tab (out of memory), and the server held the whole file in memory at every step.

## Scope
**In:**
- Configurable upload size limit, changeable at runtime (done).
- Phases 1-3 of the scaling plan: small responses, streaming parser, background import, paged review, render at send time, faster and safer workers, progress counters.

**Out (not now):**
- Phase 4 (separate worker process, object storage, archiving old `email_jobs`) until there is more than one server.
- Server push instead of polling (its own roadmap row, decision 0010).

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

**Phase 1: small responses + streaming parser** (same routes, new response shape)
- [ ] `RowError` gets a `code` (`empty_email`, `empty_name`, `invalid_email`, `duplicate`, `bad_domain`, `unverified`); the frontend reads the code instead of guessing from the reason text (`lib/problems.ts`)
- [ ] `POST /recipients/parse` and `POST /batches`: `valid_count`, `valid_sample` (5), `problem_count`, `problem_counts` per code, `problems` (first 500), `unverified_count`, `unverified` (first 500)
- [ ] Frontend: Review report and New Bulk Email read the counts; tab counts come from `problem_counts`; the table shows at most 500 rows plus a "showing 500 of N" line
- [ ] Streaming parser: `f.Rows()` instead of `f.GetRows()`; measure heap on the 1M sample before/after
- [ ] `GET /batches/{id}/jobs` without paging params gets a default `limit` (today it returns every job, 1M rows); check nothing outside the frontend uses the plain-array form
- [ ] Tests (Go + Vitest), README API table, scratch-schema run with the 1M sample in the UI

**Phase 2: upload once, import in the background**
- [ ] Migration: `uploads` table (id, filename, sha256, status, counts, error, created_at, expires_at) and `upload_rows` (upload_id, row, name, email, code, reason)
- [ ] `POST /uploads`: save the file to disk, return `202 {upload_id}`; same sha256 within the expiry returns the existing upload
- [ ] Import job: stream rows, validate, dedup, `COPY` into `upload_rows` in chunks; survives a closed tab; on server restart an unfinished import starts again
- [ ] Domain check on the unique domains only, in parallel, with a cache; rows updated by domain in SQL
- [ ] `GET /uploads/{id}` (status, progress, counts, sample) and `GET /uploads/{id}/problems?code=&after=&limit=` (keyset paging)
- [ ] `POST /batches` with JSON `{upload_id, subject, body}`: `INSERT INTO email_jobs ... SELECT FROM upload_rows`, no re-upload
- [ ] Render at send time: workers render from `batches.subject_template` / `body_template`; `email_jobs.subject/body` stop being filled
- [ ] Frontend: upload progress, "Checking N rows..." progress by polling, paged problems table, Verify -> New Bulk Email hands over `upload_id` instead of the File
- [ ] Cleanup of expired uploads, files and rows

**Phase 3: sending at scale**
- [ ] Workers claim up to N jobs per query and mark them sent/failed in bulk
- [ ] Send rate limit setting (emails per second) for the SMTP provider
- [ ] Stuck-job reaper: `processing` with an old `locked_at` goes back to `pending` (shared with the "Failures endpoint + stuck-job recovery" roadmap row)
- [ ] Counter columns on `batches` (`sent_count`, `failed_count`, ...) updated with the job updates; progress, list and stats read them instead of `COUNT(*)`
- [ ] Keyset paging (`after=`) for `GET /batches/{id}/jobs`; a cheaper search than `ILIKE '%x%'`
- [ ] `Idempotency-Key` on `POST /batches` so a double click or a retry cannot create two batches

## Open questions
- **Requirements from the user (2026-11-08), which replace "first 500 problems" in Phase 1:** no valid emails in any response; every problem row must be viewable on Verify Emails; a colleague suggested fetching problems from their own paged endpoint (keyset `after=row`), which needs check results stored server-side under an `upload_id`. That also removes the double parse: Send takes `upload_id` instead of the file.
- Store check results in new `uploads` + `upload_rows` tables (recommended: `email_jobs` stays "only emails meant to be sent", no draft filtering in the 26 queries that read `batches`/`email_jobs`) or in `batches` + `email_jobs` with a `draft` status plus `row_num` and `problem_code` columns?
- Email preview names without valid emails in the response: `GET /uploads/{id}/sample?limit=5` (recommended), a made-up example name, or the first valid row only?
- Phase 1: cap the problems list at 500 or 1,000? (Phase 2 replaces the cap with paging.)
- Phase 2: keep the multipart `POST /batches` for Postman/scripts (it would create an upload internally and answer `202` with an importing batch), or remove it once the frontend moved?
- Phase 2: keep `POST /recipients/parse` as a small synchronous check for small files, or retire it in favour of `POST /uploads`?
- Phase 2: dedup in a Go map while streaming (simple, about 60 MB per 1M emails) or in SQL after `COPY` (no memory limit, more SQL)? Go map recommended until files reach about 10M rows.

## Scaling plan (proposed 2026-11-08, not started)

### Audit: what happens to a 1M-row file today
| # | Where | Today | Problem |
|---|---|---|---|
| 1 | `recipients.parseFile` | `GetRows` loads every row | heap about 13x file size (18 MB -> 233 MB) |
| 2 | `POST /recipients/parse` | returns every valid row and problem | 78 MB JSON; UI uses a count and 5 rows |
| 3 | `ReviewReport.vue` | renders every problem row, filters the full list 3 times for tab counts | 170k `<tr>` elements: this crashed the tab |
| 4 | Verify -> New Bulk Email | the file is uploaded and parsed again on Send | double work |
| 5 | `verifyDomains` | one DNS lookup after another, inside the request | many domains -> minutes, then the 120 s frontend timeout |
| 6 | `CreateBatch` | renders subject + body for every row up front | ~630 MB RAM; the same body stored 1M times (299 MB per batch) |
| 7 | `CreateBatchWithJobs` | one transaction inside the HTTP request | cut-off request saves nothing; double click = two batches |
| 8 | worker | 4 DB round trips per email | ~3 ms/job, about 50 min of DB work per worker for 1M |
| 9 | worker | nothing resets jobs stuck in `processing` | a crash leaves them stuck forever |
| 10 | worker | no send rate limit | providers throttle or block (e.g. 14/s -> 1M takes ~20 h anyway) |
| 11 | progress, list, stats | `COUNT(*) FILTER` over a batch's jobs on every poll | scans 1M rows every few seconds |
| 12 | `GET /batches/{id}/jobs` | `OFFSET` + `COUNT(*)` + `ILIKE`; unpaged mode returns all jobs | slow deep pages; 1M-row response possible |

### Tab closed half way (verified 2026-11-08)
Everything happens inside one HTTP request. Scratch schema test: `POST /batches` with the 1M sample, client cut off after 12 s. Server log: `create batch failed err="insert jobs chunk: context canceled"`, `status=500 duration=11.996s`; afterwards 0 batches, 0 jobs. Go cancels the request's context when the client goes away; the insert uses that context, so the transaction rolls back. The 12 s of parsing and rendering were wasted, and the user sees no result. The frontend's 120 s axios timeout behaves the same way as closing the tab. A Verify Emails result lives only in the tab's memory (`useRecipientCheck`, `pendingCheck` store), so a reload means uploading again. Phase 2 fixes both: the file and the import live on the server under an `upload_id`.

### Routes: today and after the plan
| Route | Used by (frontend) | Phase 1 | Phase 2 | Phase 3 |
|---|---|---|---|---|
| `POST /recipients/parse` | Verify Emails, New Bulk Email check | summary response | open question: keep for small files or retire | - |
| `POST /batches` (multipart) | New Bulk Email Send | summary response | JSON `{upload_id, subject, body}`; multipart is an open question | `Idempotency-Key` |
| `POST /uploads` | - | - | new: save file, `202 {upload_id}` | - |
| `GET /uploads/{id}` | - | - | new: status, progress, counts, sample | - |
| `GET /uploads/{id}/problems` | - | - | new: keyset-paged problems by `code` | - |
| `GET /batches`, `GET /batches/{id}` | Bulk Emails list/detail, polling | - | - | read counters |
| `GET /batches/{id}/jobs` | Bulk Email detail table | default `limit` when unpaged | - | keyset `after=` |
| `POST /batches/{id}/retry`, `/cancel`, `POST /jobs/{id}/retry` | Bulk Email detail | - | - | keep counters in step |
| `GET /stats` | Dashboard | - | - | read counters |
| `POST /emails`, settings routes, `/healthz` | Single Email, Settings | unchanged | `MAX_UPLOAD_MB` cap can rise (streaming) | send rate setting |

## Baseline numbers (2026-11-08)
Full results: [docs/benchmarks/2026-11-08-baseline/results.md](../benchmarks/2026-11-08-baseline/results.md). 1M sample, domain check **off** (so 880,533 valid / 69,507 problems; with the domain check on, about 100k rows more fail on bad domains), i5-12400, Postgres 18.
| What | Baseline |
|---|---|
| Parser (Go benchmark) | 6.26 s, **peak heap ~270 MB**, 3.5 GB allocated, 69M allocations |
| `POST /recipients/parse` | 6.25 s, **67.5 MB response**, server peak memory 454-528 MB |
| `POST /batches` | 17.96 s (parse 6.0 + render 3.9 + insert 8.0), server peak memory **814 MB**, 4.1 MB response, `email_jobs` 259 MB on disk |
| Workers (5, local Mailpit) | **475 emails/s** -> ~31 min for 880k |
| Cancel the 1M batch | 4.34 s |
| Batch progress / list / stats | median 131 / 422 / 86 ms |
| Jobs: first page / offset 900k / Invalid tab / search with no match | median 43 / 193 / 147 / 527 ms |

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
- 2026-11-08: a 1M-row check crashed the browser tab (out of memory). Full audit, 4-phase plan and route plan recorded under "Scaling plan". Phase 4 (later): separate worker process, object storage for files, archiving/partitioning old `email_jobs`.
