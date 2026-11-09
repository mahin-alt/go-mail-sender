# Roadmap

> The index of all features. **Keep this file small:** one row per feature, details live in `docs/features/<name>.md`. Edit it only when a feature starts or finishes (keeps parallel branches from conflicting).

Status: `proposed` -> `next` -> `in progress` -> `done` (or `deferred`).

| Feature | Status | Doc | Notes |
|---|---|---|---|
| Dashboard: backend APIs | done | [features/dashboard.md](features/dashboard.md) | batch list, paged jobs, retry, stats, settings, STARTTLS, CORS, goose |
| Dashboard: Vue frontend | in progress (Phases 1-6 done) | [features/dashboard.md](features/dashboard.md), [frontend/](frontend/) | branch `feat/frontend-new-bulk-email`; app shell, Bulk Emails and New Bulk Email done; next Phase 7 Failures |
| Large spreadsheet uploads (1M+ rows) | in progress | [features/large-file-upload.md](features/large-file-upload.md) | `MAX_UPLOAD_MB` done; benchmark baseline merged; Phase 1 done on `feat/upload-check-storage` (check results in `uploads`/`upload_rows`, paged problems, streaming parser); next Phase 2 (Send from the stored check, render at send time), Phase 3 sending at scale |
| Structured logging | done (essentials) | [features/logging.md](features/logging.md), [decisions/0012](decisions/0012-structured-logging-slog.md) | branch `feat/structured-logging`; slog, `LOG_*` in `.env`, `logs/server.log`, request log, masked addresses |
| Cancel a Bulk Email | done | [features/cancel-batch.md](features/cancel-batch.md) | branch `feat/cancel-batch`; `cancelled` job and batch status, `POST /batches/{id}/cancel`, Cancel button; migration 00004 adds status CHECKs |
| Failures endpoint + stuck-job recovery | next | (no doc yet) | cross-batch failed list grouped by reason; requeue jobs stuck in `processing` (also Phase 3 of the large-upload plan); maybe a `failed_at` column |
| Templates | proposed | (no doc yet) | saved subject/body templates, picked in the New Bulk Email wizard |
| Recipients search | proposed | (no doc yet) | "did merchant X get the notice?" |
| Activity log | proposed | (no doc yet) | audit trail of sends, retries, settings changes |
| Subject in Bulk Email list | proposed | [decisions/0009](decisions/0009-bulk-email-row-title.md) | add `subject_template` to the batch API responses so rows can show the subject (options B and C in the decision) |
| Live progress by server push | proposed | [decisions/0010](decisions/0010-live-progress-polling.md) | SSE/WebSocket instead of polling (option D); needs a Go events endpoint |
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
