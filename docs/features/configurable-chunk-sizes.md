# Feature: Configurable chunk sizes

**Status:** done
**Branch:** `feat/configurable-chunk-sizes`
**Owner / session notes:** makes `IMPORT_CHUNK_SIZE` and `JOB_GENERATION_CHUNK_SIZE` configurable in `.env` and editable at runtime from the Settings page.

## Where we left off
> Update this at the end of every chunk of work, in the same commit. A new session reads this first.
- **Last done:** (2026-11-12) Added `IMPORT_CHUNK_SIZE` (default 1,000, range 1–50,000) and `JOB_GENERATION_CHUNK_SIZE` (default 5,000, range 1–100,000) across config, settings catalog, `importer.Worker`, `worker.Generator`, `cmd/server/main.go`, frontend Settings (`Sending behaviour` tab), `.env.example`, `.env`, `README.md`, `docs/FEATURES.md`, and `docs/ROADMAP.md`. All Go tests (including DB tests on a scratch schema with `-p 1 -race`) and frontend lint, type-check, Vitest, and build pass.
- **Next:** user reviews and commits when ready.
- **Blocked on / waiting for the user:** nothing.
- **Updated:** 2026-11-12

## Why
Previously, the background spreadsheet import chunk size (`1,000` rows per transaction) and job-generation chunk size (`5,000` email jobs per transaction) were hard-coded constants. Making both configurable in `.env` and editable from the Settings UI lets developers and operators tune database transaction size and memory usage without rebuilding the server.

## Scope
**In:**
- `IMPORT_CHUNK_SIZE` and `JOB_GENERATION_CHUNK_SIZE` loaded from environment / `.env` with bounds validation.
- Settings catalog entries (`ApplyInstant`), stored in `app_settings` and pushed live via `Applier`.
- Atomic runtime updates on `importer.Worker` and `worker.Generator` via `SetChunkSize`.
- Explicit `chunk_size` attributes in completion structured logs (`import finished` and `emails created; batch queued for sending`).
- Frontend Settings placement in the `Sending behaviour` tab (`frontend/src/lib/settingsGroups.ts`).
- Unit tests (Go config, settings, API, importer, worker; Vitest settings view) and doc updates.

**Out (not now):**
- Batching email-sending worker claims or batch-counter updates (still 1 job per claim; tracked separately in [ROADMAP.md](../ROADMAP.md)).

## Design and decisions
- **Decision:** Both `IMPORT_CHUNK_SIZE` and `JOB_GENERATION_CHUNK_SIZE` use `ApplyInstant` and live in the **Sending behaviour** tab on the Settings page. **Because:** each import and job-generation run reads its chunk size atomically at the start of the run (`effectiveChunkSize()`), so changes take effect immediately for the next run without restarting the server (same pattern as `MAX_UPLOAD_MB` and `DOMAIN_CHECK_ENABLED`).
- **Decision:** Upper bounds are `50,000` for `IMPORT_CHUNK_SIZE` and `100,000` for `JOB_GENERATION_CHUNK_SIZE` (minimum `1` for both). **Because:** an import chunk holds Go slices and 5 Postgres `unnest` arrays in memory per transaction, while a generation chunk locks the batch row `FOR UPDATE` in PostgreSQL during `INSERT ... SELECT` (which `CancelBatch` waits on). Capping both prevents accidental OOMs or long batch locks.
- **Decision:** Keep `ChunkSize int` as an optional test override alongside an atomic `chunkSize atomic.Int64` updated by `SetChunkSize(int)`. **Because:** production callers (`main.go` and the settings `Applier`) can update chunk sizes concurrently with background workers without data races, while existing single-goroutine tests that set `w.ChunkSize` or `g.ChunkSize` continue to work.

## Tasks
- [x] `internal/config`: `ImportChunkSize`, `JobGenerationChunkSize`, defaults (`1000`, `5000`), max bounds (`50000`, `100000`), startup validation, and tests.
- [x] `internal/importer/worker.go` and `internal/worker/generator.go`: thread-safe `SetChunkSize(int)` and `effectiveChunkSize()`, plus unit tests.
- [x] `internal/settings/catalog.go`: register `IMPORT_CHUNK_SIZE` and `JOB_GENERATION_CHUNK_SIZE` as `ApplyInstant`, plus parser and API tests.
- [x] `cmd/server/main.go`: initialize both workers with effective config and wire them into the settings `Applier`.
- [x] `frontend/src/lib/settingsGroups.ts`: add both keys to the `Sending behaviour` group and update Vitest specs.
- [x] Update `.env.example`, `.env`, `README.md`, `docs/FEATURES.md`, and `docs/ROADMAP.md`.

## Open questions
- None.

## Verification
- `go build ./... && go vet ./... && go test -race ./...`: pass.
- `TEST_DATABASE_URL="$DATABASE_URL&search_path=scratch_chunk_test" go test -p 1 -race ./...`: all packages (`api`, `config`, `domaincheck`, `importer`, `logging`, `mailer`, `migrations`, `recipients`, `settings`, `store`, `template`, `worker`) pass.
- `cd frontend && npm run lint && npm run type-check && npx vitest run && npm run build`: pass (18 test files, 167 tests).

