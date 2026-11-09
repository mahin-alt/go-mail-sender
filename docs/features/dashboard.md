# Feature: Dashboard

**Status:** in progress (backend APIs done and merged to `main`; frontend: bare scaffold with the stack installed, all UI code removed on request)
**Branches:** backend and frontend Phases 0-1 are merged into local `main` (fast-forward; pushed to `origin`). Frontend Phase 2 merged too; Phase 3: `feat/frontend-new-bulk-email` (from `main`)
**Frontend location:** `frontend/`. No design system or frontend design docs (deleted on purpose; visual decisions come from the ui-ux-pro-max skill). The web UI will be branded **MailFlow**.

## Where we left off
> Update this at the end of every chunk of work, in the same commit. A new session reads this first.
- **Last done (2026-11-03):** small UI changes (committed and merged into `main` from `refactor/save-send-buttons`): the Settings save button now reads just **Save** (it was "Test connection and save", easily mistaken for "Send test email"; the connection is still tested by the server before saving), and the New Bulk Email **Send** button is now the same style as Save (one size up), at the end of the page and not sticky; its confirmation dialog now reads **Confirm** (centred) / "N valid recipients will receive emails. M invalid entries will be excluded from sending." / **Cancel** and **Send**, and confirmation dialogs can no longer be dragged. 155 unit tests, lint, type-check and build pass; checked in the browser. Also verified, with no code change: choosing STARTTLS against Mailpit is refused correctly because Mailpit does not offer STARTTLS (use `auto` or `none` locally).
- **Last done (2026-11-02):** **Less on-screen text refactor** (committed and merged into `main` from `refactor/trim-ui-text`; the old wordy UI is kept on branch `snapshot/verbose-ui-text`, see decision 0007 "Restoring the old text"). Removed page descriptions, the Settings banner/descriptions/tags, the review and verify notices, the Send bar text and the Single Email footer sentence; added the read-only From row; shortened skip reasons (no email address, "domain does not accept mail"); moved the sidebar collapse control to an icon beside the logo. 149 unit tests, lint, type-check and build pass; checked in the browser against a real API (Settings, Single Email, Verify with a real file, Continue to New Bulk Email, sidebar open / collapsed / hover).
- **Before that (2026-11-02):** Phase 5 Dashboard built, verified, committed (`48cfb83`) and merged into `main`.
- **Failures is hidden from the sidebar for now** (user's request, 2026-11-02): its route still exists as a placeholder; to show it again add `icon: TriangleAlert` to the route's `meta` in `frontend/src/router/routes.ts` (and the test list in `App.spec.ts`).
- **Next (needs the user's go-ahead):** more refactors the user has in mind; then Phase 7 (Failures page; needs a backend endpoint for failed emails across all Bulk Emails, grouped by reason, plus recovery of emails stuck in "processing"). Still pending: the user's list of responsive issues, and the decision 0011 reminder.
- **Blocked on / waiting for the user:** (1) the user's next refactors; the list of responsive issues is still pending.
- **Updated:** 2026-11-01

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
- **New Bulk Email (wizard):** upload `.xlsx` -> data-quality report (`POST /uploads`, problems paged from `GET /uploads/{id}/problems`; `POST /recipients/parse` until 2026-11-08) -> compose with `{{.Name}}`/`{{.Email}}` chips and live preview -> review -> send (`POST /batches`).
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
- **Visual style (2026-10-29):** Bento Box Grid, paper plane logo; full spec in [decision 0007](../decisions/0007-frontend-visual-style.md). Font: Plus Jakarta Sans. Nav label "Overview" is now "Dashboard".
- **Frontend choices (2026-10-28):** merge backend to `main` first, then branch; npm; dev calls the API directly via CORS (no proxy); PrimeVue **plus Tailwind v4**; light theme only for the MVP; **Because:** see [decisions 0003, 0004, 0006](../decisions/).
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

### Frontend
Everything below was built and verified, then removed on 2026-10-29 at the user's request; all boxes are open again.
- [x] PrimeVue 4.5.5 chosen ([0006](../decisions/0006-primevue-version-and-license.md)); design docs and design system deleted on purpose, to be redone with the ui-ux-pro-max skill
- [x] Phase 0: scaffold `frontend/` with the stack (kept)
- [x] Phase 1: app shell, API client, typed API shapes, `StatusBadge`
- [x] Phase 2: Bulk Emails list and detail (polling, Retry)
- [x] Phase 3: New Bulk Email page (one-page layout)
- [x] Phase 4: Settings with SMTP test email (tabs)
- [x] Phase 5: Dashboard
- [x] Phase 6: Verify Emails and Single Email
- [ ] Phase 7: Failures page (needs the backend Failures endpoint)

## Open questions
- **REMIND THE USER:** on 2026-11-01 the user said the New Bulk Email page (and possibly other pages) has **responsive issues and some other problems**, details to be given later. Ask for them at the start of the next session or before Phase 5, and fix on a small branch.
- Optional improvements discussed for the Review step (not decided): a "Did you mean gmail.com?" hint for typo domains, a checkbox to exclude unverified rows, a warning on the Send bar when unverified rows will be sent.
- Recipient type per upload (Merchant vs Customer)? Affects filters and the data model; undecided.
- Add `failed_at` now, or keep using `updated_at`?
- Where do Templates live (new table) and how are they versioned?

## Verification
- `go build ./... && go vet ./... && go test ./...`; DB tests per [CLAUDE.md](../../CLAUDE.md).
- End-to-end on a fresh isolated database was done for: single email, small and 1,000-row bulk sends, failure + retry, stats, settings (all passed).

## Notes / history
- The settings endpoints can change where mail is sent from and have **no auth**. Do not expose the API beyond localhost.
- A custom `migrate` command was built and then removed in favour of the official goose CLI (`d2b75b4` is superseded by `8cf24d0`).
