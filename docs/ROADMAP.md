# Roadmap

> The index of all features. **Keep this file small:** one row per feature, details live in `docs/features/<name>.md`. Edit it only when a feature starts or finishes (keeps parallel branches from conflicting).

Status: `proposed` -> `next` -> `in progress` -> `done` (or `deferred`).

| Feature | Status | Doc | Notes |
|---|---|---|---|
| Dashboard: backend APIs | done | [features/dashboard.md](features/dashboard.md) | batch list, paged jobs, retry, stats, settings, STARTTLS, CORS, goose |
| Dashboard: Vue frontend | in progress (Phases 1-6 done) | [features/dashboard.md](features/dashboard.md), [frontend/](frontend/) | branch `feat/frontend-new-bulk-email`; app shell, Bulk Emails and New Bulk Email done; next Phase 7 Failures |
| Large spreadsheet uploads (1M+ rows) | in progress | [features/large-file-upload.md](features/large-file-upload.md) | `MAX_UPLOAD_MB` done; benchmark baseline merged; Phase 1 merged (`uploads`/`upload_rows`, paged problems, streaming parser). Phases 2-3 are replaced by the four-table overhaul below |
| Four-table overhaul | done (uncommitted) | [features/four-table-overhaul.md](features/four-table-overhaul.md), [decisions/0013](decisions/0013-four-table-data-model.md) | `email_batches`/`email_recipients`/`email_import_jobs`/`email_jobs`, background import, `/verify` + `/batches` API; branch `refactor/database-schema-changes-with-system-overhaul` |
| Structured logging | done (essentials) | [features/logging.md](features/logging.md), [decisions/0012](decisions/0012-structured-logging-slog.md) | branch `feat/structured-logging`; slog, `LOG_*` in `.env`, `logs/server.log`, request log, masked addresses |
| Cancel a Bulk Email | done | [features/cancel-batch.md](features/cancel-batch.md) | branch `feat/cancel-batch`; `cancelled` job and batch status, `POST /batches/{id}/cancel`, Cancel button; migration 00004 adds status CHECKs |
| Configurable chunk sizes | done | [features/configurable-chunk-sizes.md](features/configurable-chunk-sizes.md) | `IMPORT_CHUNK_SIZE` (default 1,000, max 50,000) and `JOB_GENERATION_CHUNK_SIZE` (default 5,000, max 100,000) configurable in `.env` and Settings UI (`ApplyInstant`) |
| Operation timing logs | done | [features/operation-timing-logs.md](features/operation-timing-logs.md) | branch `feat/operation-timing-logs`; fine-grained timings in `import finished` (`parse_time`, `dns_time`, `db_insert_time`) and `email sent` (`claim`, `render`, `send`, `db_mark`, `queued`) |
| Failures endpoint + stuck-job recovery | next | (no doc yet) | cross-batch failed list grouped by reason; requeue jobs stuck in `processing` (also Phase 3 of the large-upload plan); maybe a `failed_at` column |
| Templates | proposed | (no doc yet) | saved subject/body templates, picked in the New Bulk Email wizard |
| Recipients search | proposed | (no doc yet) | "did merchant X get the notice?" |
| Activity log | proposed | (no doc yet) | audit trail of sends, retries, settings changes |
| Subject in Bulk Email list | proposed | [decisions/0009](decisions/0009-bulk-email-row-title.md) | add `subject_template` to the batch API responses so rows can show the subject (options B and C in the decision) |
| Live progress by server push | proposed | [decisions/0010](decisions/0010-live-progress-polling.md) | SSE/WebSocket instead of polling (option D); needs a Go events endpoint |
| Delete a batch | proposed | (no doc yet) | uploads are kept forever after the 4-table overhaul, so the database keeps growing; a Delete button removes a batch with its recipients and jobs (CASCADE). Noted 2026-11-09 |
| Temporary upload files | proposed | (no doc yet) | the overhaul keeps each uploaded `.xlsx` on disk permanently (`UPLOAD_DIR/batch-<id>.xlsx`); later maybe delete it once the import finishes. Noted 2026-11-09 |
| Separate worker process | proposed | (no doc yet) | the overhaul keeps API + import, generation and sending workers in one Go process; later split them (e.g. `cmd/worker`) when imports slow the API, several API copies run, or sending must scale on its own. Noted 2026-11-09 |
| Settings editing policy | proposed (parked) | [decisions/0011](decisions/0011-settings-editing-policy.md) | tiers of who may change which setting, `SETTINGS_EDITABLE` kill switch, pool>=workers check, audit trail; **remind the user** before login or any non-localhost deploy |
| Authentication and roles | deferred | [decisions/0002](decisions/0002-no-auth-for-now.md) | needed before the API leaves localhost |

## Branches

`feat/dashboard-api` and `feat/db-migrations` (backend APIs, goose migrations, project docs) were merged into local `main` on 2026-10-28 by fast-forward. `main` has since been pushed to `origin`.

Frontend Phases 0-2 (`feat/dashboard-frontend`, `feat/frontend-bulk-emails`) were merged into local `main` on 2026-10-29 by fast-forward; `main` is pushed to `origin`. Active: `feat/frontend-new-bulk-email` (Phase 3, cut from `main`). Rule: one short branch per phase from `main`, see [decisions/0008](decisions/0008-branch-per-phase.md).

## Starting a new feature

1. Copy [features/_template.md](features/_template.md) to `features/<name>.md` and fill in goal, scope and first tasks.
2. Add one row above.
3. Branch, then work. Tick checkboxes in the feature doc **in the same commits as the code**.
4. When finished: set the doc's status to `done`, move lasting decisions to `decisions/`, update the README if behaviour changed, update the row above.

Parallel features: one branch, one doc and one session each (a `git worktree` per feature keeps working folders separate). Migrations are numbered, so two branches adding one will collide on the number; whoever merges second renumbers (the migration test catches it).
