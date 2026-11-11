# Feature: Large spreadsheet uploads (1M+ rows)

**Status:** in progress
**Branch:** Phase 1 on `feat/upload-check-storage` (based on: `main` at `773e765`); earlier: `feat/upload-size-limit`, `chore/benchmark-baseline` (both merged)
**Owner / session notes:** started as "parse 1M+ row files"; on 2026-11-08 a 1M-row test crashed the browser tab, and the scope grew into the 4-phase scaling plan below (upload, import, review and send all made to handle 1M+ rows).

## Where we left off
> Update this at the end of every chunk of work, in the same commit. A new session reads this first.
- **Last done:** (2026-11-08) **Phase 1** on `feat/upload-check-storage`: check results stored in new `uploads` + `upload_rows` tables (migration 00005), streaming parser (`recipients.Scan`), `POST /uploads` returning totals only, paged `GET /uploads/{id}/problems`, `GET /uploads/{id}/sample` for the preview, hourly cleanup of expired uploads, `/recipients/parse` retired, `/batches` no longer returns problem lists, jobs list always paged. Frontend: Verify Emails and New Bulk Email use the upload summary, the review table pages with "Load more", the preview reads the sample endpoint. Go tests (with DB tests on a scratch schema), lint, type-check, Vitest and build pass; the 1M sample was checked in the real UI on a scratch schema (see Verification), and benchmarked (see "Phase 1 numbers").
- **Next:** Phases 2-3 below are **replaced** by the [four-table overhaul](four-table-overhaul.md) ([decision 0013](../decisions/0013-four-table-data-model.md)), agreed 2026-11-09. Continue there; this doc stays as the history of Phase 1 and the benchmarks.
- **Blocked on / waiting for the user:** nothing here.
- **Updated:** 2026-11-09 (superseded by the four-table overhaul)

## Why
Staff want to check and send spreadsheets of a million rows or more. The upload limit was a hard-coded 10 MB, and the 1M-row sample (`samples/12_million_mixed.xlsx`) is 18 MB. With the limit raised, a 1M-row check crashed the browser tab (out of memory), and the server held the whole file in memory at every step.

## Scope
**In:**
- Configurable upload size limit, changeable at runtime (done).
- Phases 1-3 of the scaling plan: check results kept on the server and paged, streaming parser, Send from the stored check, render at send time, background import, faster and safer workers, progress counters.

**Out (not now):**
- Phase 4 (separate worker process, object storage, archiving old `email_jobs`) until there is more than one server.
- Server push instead of polling (its own roadmap row, decision 0010).

## Design and decisions
- **Decision:** the limit is a Settings entry `MAX_UPLOAD_MB` that applies **instantly** (option A), not on restart (option B). **Because:** it is read on every request, so nothing needs a restart; the user wants to change it back and forth (10 -> 50 -> 40) without downtime. Same pattern as `DOMAIN_CHECK_ENABLED` (an atomic value in the handler, updated by the settings Applier).
- **Decision:** range 1-50 MB; `.env` values above 50 stop the server at startup with a clear error. **Because:** parsing holds the whole sheet in memory, about 13x the file size (18 MB file -> ~233 MB heap to parse, ~900 MB including rendering a batch). The cap stops one upload from exhausting the server.
- **Decision:** the limit is read once at the start of each upload. **Because:** a settings change must never cut off an upload already in progress.
- **Decision:** the request body may be the limit + 1 MB, and the *file part* is checked against the exact limit. **Because:** the multipart boundaries and the subject/body fields would otherwise make a file exactly at the limit fail.
- **Decision:** the frontend reads the limit from `GET /settings` (re-fetched when an upload page opens) and falls back to 10 MB until it loads. The server is the final authority: a tab with an outdated limit gets the server's `413 "file too large: the limit is N MB"`.
- **Decision (2026-11-08, user):** no response carries valid emails, and every problem row must stay viewable. **So:** checking a file returns totals only; problems come from their own paged endpoint (a colleague's suggestion), which needs the check result kept on the server under an `upload_id`.
- **Decision (2026-11-08, user):** check results live in new tables `uploads` (one row per checked file, totals) and `upload_rows` (one row per spreadsheet row with its `kind`), not in `batches` + `email_jobs` with a draft status. **Because:** `email_jobs` stays "only emails meant to be sent", so workers, the Bulk Emails list, the dashboard and stats need no draft filtering (26 queries read those tables); checks that are never sent cannot touch the sending queue; different lifetimes (uploads expire after 24 h, batches are history) and different columns. **Cost:** two more tables and, in Phase 2, a copy step at Send.
- **Decision:** `upload_rows.kind` values: `ok`, `unverified` (sent), `empty_email`, `empty_name`, `invalid_email`, `duplicate`, `bad_domain` (skipped). Fixed words the frontend can rely on, instead of reading the kind from the reason text. The reason text stays the same as before for people.
- **Decision:** keyset paging (`after=<row>`, `next_after` in the answer) for problems, not `OFFSET`. **Because:** the index jumps straight to the row, so page 900 is as fast as page 1 (measured: 3 ms after row 900,000). Tab counts come from the totals saved once, never from `COUNT(*)`.
- **Decision:** the email preview gets its names from `GET /uploads/{id}/sample` (first 5 sendable rows), fetched only when the preview shows.
- **Decision:** rows are written with Postgres `COPY` in chunks of 5,000 while the file streams, all in one transaction (a failed or cut-off check leaves nothing behind). `database/sql` has no COPY, so the store borrows the pgx connection (`conn.Raw`).
- **Decision:** `POST /recipients/parse` is removed (its answer grew with the file); `POST /uploads` replaces it. `POST /batches` keeps its multipart form until Phase 2 but no longer returns problem lists. `GET /batches/{id}/jobs` is always paged (the plain-array form could return 1M jobs; nothing used it).
- **Decision:** the review table loads 100 rows per page with a "Load more" button and an "N of total" counter, reusing the existing table, tabs and secondary button (no new visual design).
- **Decision:** expired uploads are deleted by an hourly loop in the server; `GET` on an expired upload is `404` even before the loop runs.

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

**Phase 1: check results on the server, paged review** (done 2026-11-08)
- [x] Migration 00005: `uploads` (totals, status, sha256, expiry, `batch_id` for Phase 2) and `upload_rows` (row, name, email, kind, reason) with partial indexes for paging
- [x] Streaming parser `recipients.Scan` (`Rows()` iterator instead of `GetRows`), reporting each row's `kind`; `ParseExcel*` rebuilt on top of it (so `/batches` streams too)
- [x] Store: `ImportUpload` (COPY in chunks, one transaction, totals counted on the way), `GetUpload`, `ListUploadRows` (keyset), `DeleteExpiredUploads`
- [x] `POST /uploads` (totals only), `GET /uploads/{id}`, `GET /uploads/{id}/problems` (kind filter, search, `after`, `limit`), `GET /uploads/{id}/sample`
- [x] `/recipients/parse` removed; `/batches` without problem lists; `GET /batches/{id}/jobs` always paged
- [x] Hourly cleanup of expired uploads (`main.go`)
- [x] Frontend: uploads API + query hooks, review table paged with "Load more" and counts from the totals, preview from the sample endpoint, Verify -> New Bulk Email hands over the summary
- [x] Tests: parser kinds, API with an in-memory store, store SQL on a scratch schema, migration table list, Vitest (paging, tab requests)
- [x] README API table, benchmark script follows the new endpoint, 1M sample in the UI on a scratch schema, Phase 1 benchmark

**Phase 2: Send from the stored check**
- [ ] `POST /batches` with JSON `{upload_id, subject, body}`: `INSERT INTO email_jobs ... SELECT FROM upload_rows`, no re-upload or second parse; set `uploads.batch_id`
- [ ] Render at send time: workers render from `batches.subject_template` / `body_template`; `email_jobs.subject/body` stop being filled (needed for the SQL copy above)
- [ ] Domain check on the unique domains only, in parallel, with a cache
- [ ] Same file again (sha256) within the expiry reuses the upload
- [ ] Background import: `POST /uploads` saves the file and answers `202` with `status: importing`; the page polls `GET /uploads/{id}`; survives a closed tab; an unfinished import restarts after a server restart
- [ ] Frontend: Send uses `upload_id`; import progress; Verify -> New Bulk Email needs only the `upload_id`
- [ ] "Download problems" (original rows + a Problem column), optional grouped summary

**Phase 3: sending at scale**
- [ ] Workers claim up to N jobs per query and mark them sent/failed in bulk
- [ ] Send rate limit setting (emails per second) for the SMTP provider
- [ ] Stuck-job reaper: `processing` with an old `locked_at` goes back to `pending` (shared with the "Failures endpoint + stuck-job recovery" roadmap row)
- [ ] Counter columns on `batches` (`sent_count`, `failed_count`, ...) updated with the job updates; progress, list and stats read them instead of `COUNT(*)`
- [ ] Keyset paging (`after=`) for `GET /batches/{id}/jobs`; a cheaper search than `ILIKE '%x%'`
- [ ] `Idempotency-Key` on `POST /batches` so a double click or a retry cannot create two batches

## Open questions
- Phase 2: keep the multipart `POST /batches` for Postman/scripts (it would create an upload internally), or remove it once the frontend sends `upload_id`?
- Phase 2: after Send, delete the upload at once (saves space) or keep it until it expires so the problems page still works? (Recommended: keep until expiry.)
- Phase 2: dedup in a Go map while streaming (today; about 60 MB per 1M emails) or in SQL after `COPY`? Go map until files reach about 10M rows.

## Scaling plan (proposed 2026-11-08; Phase 1 done)

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

### Routes: before and after the plan
| Route | Used by (frontend) | Phase 1 (done) | Phase 2 | Phase 3 |
|---|---|---|---|---|
| `POST /recipients/parse` | (was) Verify Emails, New Bulk Email check | removed | - | - |
| `POST /uploads` | Verify Emails, New Bulk Email check | new: check once, keep rows, totals only | `202` + background import | - |
| `GET /uploads/{id}` | - | new: totals | import progress | - |
| `GET /uploads/{id}/problems` | review table | new: keyset-paged problems by `kind` | - | - |
| `GET /uploads/{id}/sample` | email preview | new | - | - |
| `POST /batches` (multipart) | New Bulk Email Send | no problem lists in the answer | JSON `{upload_id, subject, body}` | `Idempotency-Key` |
| `GET /batches`, `GET /batches/{id}` | Bulk Emails list/detail, polling | - | - | read counters |
| `GET /batches/{id}/jobs` | Bulk Email detail table | always paged | - | keyset `after=` |
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

## Phase 1 numbers (2026-11-08)
Full results: [docs/benchmarks/2026-11-08-phase1/results.md](../benchmarks/2026-11-08-phase1/results.md), same machine and settings as the baseline (domain check off).
| What | Baseline | Phase 1 |
|---|---|---|
| Check a file | `POST /recipients/parse`: 6.25 s, **67.5 MB** answer, server peak 454-528 MB | `POST /uploads`: 10.3 s, **353 bytes**, server peak **139-143 MB** |
| Parser alone (benchstat, 6 runs) | peak heap 256 MiB | **200 MiB (-22%)**, time -1%, allocated -4% |
| `POST /batches` | 17.96 s, peak 814 MB, 4.1 MB answer | 18.75 s, peak 797 MB, **~0 MB answer** (rest unchanged until Phase 2) |
| Problems page / page after row 900k / search | (no such endpoint: everything in one answer) | 46 / 3 / 127 ms (scratch UI check) |

- The check is about 4 s slower because it now saves all 950k rows (COPY) so they can be paged; in exchange the answer is 353 bytes instead of 67.5 MB and the server needs a quarter of the memory. Phase 2's Send reuses these rows instead of parsing again, which wins the time back.
- Parser peak heap only drops 22% in the Go benchmark because `ParseExcelReader` still collects every row into slices (for `/batches`) and excelize keeps the shared-strings table in memory; `POST /uploads` does not collect rows, hence 140 MB for the whole server.
- **Noise warning:** worker speed and the batch read endpoints did not change in Phase 1 (same code) but measured 475 / 219 / 16 / 331 emails/s across four runs on 2026-11-08, and reads up to 2-3x slower in some runs. Per-email SMTP time was 0.5 ms (debug log), so the time goes to the database side of each claim, which varies between runs on this machine. Compare those numbers only with several runs back to back. A real cause found while checking, for Phase 3: the claim query walks the primary key from the start and skips every row already sent (`Rows Removed by Filter: 29999` after 30k sent, ~8 ms per claim), so claiming gets slower as a batch progresses.

## Verification
- `go build ./... && go vet ./... && go test ./...` and the DB tests in a scratch schema.
- `cd frontend && npm run lint && npm run type-check && npx vitest run && npm run build`.
- Manual, against an API on a scratch schema (`api-scratch` and `frontend-scratch` in `.claude/launch.json`, ports 8091 / 5174, schema `scratch_ui_test` created and dropped by hand):
  - Settings -> Sending behaviour shows `MAX_UPLOAD_MB` = 10; save 40 -> "Settings saved".
  - (until Phase 1) `curl -F file=@samples/12_million_mixed.xlsx .../recipients/parse` -> `200` in ~7 s (78 MB response).
  - 45 MB file -> `413 file too large: the limit is 40 MB`; the UI blocks it before uploading with the same wording.
  - `PUT MAX_UPLOAD_MB=10` with no restart -> the 18 MB file now gets `413 ... 10 MB`; a page left open with 40 shows the server's message; reopening the page shows "Up to 10 MB".
  - `MAX_UPLOAD_MB=51` -> `400`.

- Phase 1, same scratch setup (domain check on, as in `.env`), 2026-11-08:
  - `curl -F file=@samples/12_million_mixed.xlsx .../uploads` -> `201` in 11.5 s, **353-byte** answer (950,040 rows, 780,493 valid, 169,547 problems: 69,507 empty email + 100,040 bad domain), server memory peak 142 MB.
  - Problems: first page 46 ms, a page after row 900,000 3 ms, Invalid tab 21 ms, search with no match 127 ms; sample 1 ms; unknown upload `404`.
  - In the UI: Verify Emails shows the tiles and tab counts, 100 rows then 200 after "Load more" ("200 of 169,547"), the Invalid tab loads its own rows, browser JS heap 32 MB, no console errors. "Continue to New Bulk Email" shows the preview with real names (1 of 5) and Send enabled (not pressed: the scratch API sends to the real Mailpit).

## Notes / history
- 2026-11-04: measured the 1M-row sample with the real code in a scratch schema: parse 6.1 s / 233 MB, render 4.0 s / 629 MB, single-transaction insert of 950k rows 8.7 s, one batch = 299 MB on disk, claim + mark-sent ~3 ms per job (~336 jobs/s per worker, DB only). Full audit and proposals (background import, render at send time, batch claiming, rate limiting, counters) were discussed in the session; record them here when that phase starts.
- 2026-11-08: a 1M-row check crashed the browser tab (out of memory). Full audit, 4-phase plan and route plan recorded under "Scaling plan". Phase 4 (later): separate worker process, object storage for files, archiving/partitioning old `email_jobs`.
