# Feature: Dashboard

**Status:** in progress (backend APIs done; Vue frontend not started)
**Branches:** `feat/dashboard-api` (from `main`), then `feat/db-migrations` stacked on top
**Frontend location:** `frontend/` (not created yet)

## Why
Staff need a web interface to send and monitor bulk emails instead of calling the API by hand. See [PROJECT.md](../PROJECT.md).

## Scope
**In (MVP):** Overview, Bulk Emails (new / list / detail), Verify Emails, Failures, Single Email, editable Settings.
**Out for now:** Templates, Recipients search, Activity Log page, authentication, roles ([decision 0002](../decisions/0002-no-auth-for-now.md)).

## Menus
```
Overview
SEND     Bulk Emails (New | All | Detail) | Single Email
PREPARE  Verify Emails | Templates
MONITOR  Failures | Recipients | Activity Log
Settings
```
- **Overview:** tiles (sent today, failed today, queued, in progress), 14-day chart, needs-attention list. Data: `GET /api/v1/stats`.
- **New Bulk Email (wizard):** upload `.xlsx` -> data-quality report (`POST /recipients/parse`) -> compose with `{{.Name}}`/`{{.Email}}` chips and live preview -> review -> send (`POST /batches`).
- **All Bulk Emails / Detail:** `GET /batches`; detail shows counts, a jobs table (`/batches/{id}/jobs?limit=`), Retry failed.
- **Verify Emails:** parse-only report.
- **Failures:** cross-batch failed jobs grouped by reason, bulk retry (**endpoint not built yet**).
- **Settings:** list/edit/reset, SMTP test email.

## Design and decisions
- **Settings are overrides on top of the environment.** An `app_settings` row beats the env value; "reset" deletes the row. **Because** `.env` is fragile to rewrite and edits need an audit-able, reversible home.
- **Instant vs restart.** SMTP, sender, domain check and retry count apply immediately; worker count, pool size and poll interval are saved but flagged `restart_required`. **Because** workers and the pool are sized at startup.
- **SMTP changes are verified before saving.** The server connects and logs in with the new values and refuses the save on failure. **Because** a typo must never silently break all sending.
- **SMTP password is write-only:** AES-256-GCM in the database (key `SETTINGS_ENCRYPTION_KEY`), never returned or logged. `DATABASE_URL` is never exposed.
- **TLS modes** `auto` (default) | `none` | `starttls` | `tls`. `auto` upgrades only if the server offers STARTTLS, so Mailpit keeps working unchanged. The SMTP pool can be reconfigured live; old connections are retired.
- **Retry** requeues only `failed` jobs with fresh attempts, wakes the workers, and reopens a completed batch. `invalid` rows are never retried (no rendered email exists).
- **Paged jobs endpoint keeps the old plain-array response** when no `limit`/`offset`/`q` is sent, so existing callers do not break.
- **Stats:** `tz` query param decides where "today" starts. A failure's time is the job's `updated_at` (there is no `failed_at` column yet). "Needs attention" = batches with unresolved failed jobs, or >= 20% invalid rows within 30 days. Small partial indexes keep the queries cheap.
- **Migrations** are goose files applied at startup; existing databases adopt them safely ([decision 0001](../decisions/0001-use-goose-migrations.md)).

## Tasks

### Backend
- [x] Batch list `GET /batches` with paging/filters (`3d44171`)
- [x] CORS middleware, `CORS_ALLOWED_ORIGINS` (`3d44171`)
- [x] Apply `MAX_JOB_RETRIES` (was ignored) (`3d44171`)
- [x] Paged + searchable jobs, retry endpoints (`dac4d65`)
- [x] STARTTLS / implicit TLS, `SMTP_TLS_MODE`, live pool reconfigure (`2827673`)
- [x] Settings table + endpoints + SMTP test-send (`d83d752`)
- [x] Stats endpoint (`241583f`)
- [x] Goose migrations, startup auto-migrate, goose CLI docs (`2894069`, `8cf24d0`)
- [ ] Failures endpoint: cross-batch failed jobs, grouped by reason
- [ ] Recover jobs stuck in `processing` after a crash
- [ ] Optional: `failed_at` column (first new migration) for exact failure times

### Frontend (nothing started)
- [ ] Scaffold `frontend/`: Vue 3 + TypeScript + Vite, PrimeVue, Pinia, TanStack Query, axios, vue-chartjs, Lucide, ESLint + Prettier, Vitest ([decision 0004](../decisions/0004-frontend-stack.md))
- [ ] App shell: layout, sidebar, router, API client, typed API shapes
- [ ] Bulk Emails: list and detail pages
- [ ] New Bulk Email wizard
- [ ] Settings page
- [ ] Overview page
- [ ] Verify Emails page
- [ ] Single Email page
- [ ] Failures page (needs the Failures endpoint)

## Open questions
- Recipient type per upload (Merchant vs Customer)? Affects filters and the data model; undecided.
- Add `failed_at` now, or keep using `updated_at`?
- Where do Templates live (new table) and how are they versioned?

## Verification
- `go build ./... && go vet ./... && go test ./...`; DB tests per [CLAUDE.md](../../CLAUDE.md).
- End-to-end on a fresh isolated database was done for: single email, small and 1,000-row bulk sends, failure + retry, stats, settings (all passed).

## Notes / history
- The settings endpoints can change where mail is sent from and have **no auth**. Do not expose the API beyond localhost.
- A custom `migrate` command was built and then removed in favour of the official goose CLI (`d2b75b4` is superseded by `8cf24d0`).
