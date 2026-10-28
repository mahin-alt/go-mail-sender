# Feature: Dashboard

**Status:** in progress (backend APIs done and merged to `main`; frontend design approved, not started)
**Branches:** backend merged into `main` (fast-forward, not pushed). Frontend work: `feat/dashboard-frontend` (from `main`)
**Frontend location:** `frontend/` (not created yet). Design docs: [docs/frontend/](../frontend/)

## Where we left off
> Update this at the end of every chunk of work, in the same commit. A new session reads this first.
- **Last done:** merged backend into `main`; wrote the frontend design docs ([docs/frontend/](../frontend/)); user approved the design and chose PrimeVue 4.5.5 ([decision 0006](../decisions/0006-primevue-version-and-license.md)); docs committed on `main` locally (push to `origin` pending: needs the user's credentials). No frontend code yet.
- **Next (needs the user's go-ahead):** Phase 0, scaffold `frontend/` (pin versions; read Vue Router 5 and Pinia 4 migration notes first).
- **Blocked on / waiting for the user:** (1) the user must run `git push origin main`; (2) the go-ahead to start Phase 0.
- **Updated:** 2026-10-28

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
- **Frontend choices (2026-10-28):** merge backend to `main` first, then branch; npm; dev calls the API directly via CORS (no proxy); PrimeVue **plus Tailwind v4**; light theme only for the MVP. **Because:** see [decisions 0003, 0004, 0006](../decisions/) and [docs/frontend/design-brief.md](../frontend/design-brief.md).
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

### Frontend (design proposed, no code yet)
One phase at a time; each ends with lint, typecheck, tests, a real run against the Go API and Mailpit, and a docs update. Pages are specified in [screens-and-flows.md](../frontend/screens-and-flows.md).
- [x] Design docs written and approved: [docs/frontend/](../frontend/)
- [x] PrimeVue version decided: 4.5.5 ([0006](../decisions/0006-primevue-version-and-license.md))
- [ ] Phase 0: scaffold `frontend/` (Vue 3, TypeScript, Vite, Tailwind v4, PrimeVue + theme preset, fonts, Pinia, TanStack Query, axios, ESLint + Prettier, Vitest); check Bangla rendering and PrimeVue/Tailwind layering
- [ ] Phase 1: app shell: layout, sidebar, router, API client, typed API shapes, StatusBadge
- [ ] Phase 2: Bulk Emails list and detail (polling, Retry)
- [ ] Phase 3: New Bulk Email wizard
- [ ] Phase 4: Settings page with SMTP test email
- [ ] Phase 5: Overview page
- [ ] Phase 6: Verify Emails and Single Email
- [ ] Phase 7: Failures page (needs the Failures endpoint)

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
