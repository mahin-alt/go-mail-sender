# Features and use cases

A plain list of what MailFlow (go-mail-sender) does today and what is planned.
Status as of 2026-11-09 (branch `refactor/database-schema-changes-with-system-overhaul`, the four-table overhaul). For how each part works see the
links; for plans and progress see [ROADMAP.md](ROADMAP.md).

**In one sentence:** staff upload a spreadsheet of people, write one message,
and the service emails everyone in the background, shows what was sent and
what failed, and lets them fix problems without developer help.

## Who uses it

- **Operations / support staff** (not technical): send order updates and
  notices to customers and merchants, watch progress, retry failures.
- **Developers:** run and configure the service, add features.

## Use cases

| # | "As staff, I want to..." | How, in the app |
|---|---|---|
| 1 | **email everyone in a spreadsheet** with a personal greeting | New Bulk Email: upload `.xlsx` -> write subject and message with `{{.Name}}` / `{{.Email}}` -> Send |
| 2 | **check a spreadsheet before sending**, to see which rows are wrong and why | Verify Emails: upload -> counts + every problem row with its row number and reason; nothing is sent; "Continue to New Bulk Email" sends that same checked file later |
| 3 | **see what my email will look like** for real people in the file | New Bulk Email: preview with names from the file, flip through 5 recipients |
| 4 | **follow a Bulk Email while it sends** | Bulk Email page: live counts of sent, failed, pending, invalid, cancelled, and the list of every email |
| 5 | **stop a Bulk Email** I sent by mistake | Bulk Email page: Cancel ("Stop sending"); unsent emails are never sent |
| 6 | **resend emails that failed** | Bulk Email page: Retry all failed, or retry one email |
| 7 | **find one person's email** in a big Bulk Email | Bulk Email page: search by name or email, filter by status |
| 8 | **send one email** to one address | Single Email page |
| 9 | **see the day at a glance**: sent today, failed, queued, what needs attention | Dashboard |
| 10 | **change the sender or SMTP server** without editing files or restarting | Settings page, with a "send test email" button |
| 11 | **send to very large lists** (1 million rows) without the page or server falling over | Upload answers at once and the file is checked in the background (about 17 s for 950k rows, server under 70 MB); problems and emails load a page at a time |

## Features today

### Spreadsheet upload and checking
- `.xlsx` files: first sheet, a header row with `name` and `email` (any order, any capitalisation).
- Size limit `MAX_UPLOAD_MB` (default 10, up to 50), changeable in Settings without a restart; the page refuses a too-big file before uploading.
- Every row is checked and gets a result:
  - **sent:** `ok`, `unverified` (looks fine, but the domain could not be checked in time)
  - **skipped:** `empty_email`, `empty_name`, `invalid_email`, `bad_domain` (the domain cannot receive mail)
  - a repeated address is **not** flagged: every row gets its email
- Fully empty rows are ignored; row numbers match Excel.
- Domain check (DNS/MX lookup), switchable on/off in Settings.
- Large files: the upload is streamed to disk (`UPLOAD_DIR`) and answered with `202`; a background worker reads it row by row and saves chunks (`IMPORT_CHUNK_SIZE`, default 1,000; jobs created in chunks of `JOB_GENERATION_CHUNK_SIZE`, default 5,000) with a checkpoint, so a server crash or restart continues where it stopped. The page shows "N rows read" while it works.
- Checked files are kept permanently (rows in the database, the `.xlsx` on disk).

### Reviewing a checked file
- Tiles: Ready to send, Will be skipped, Could not verify.
- Tabs: All problems, Invalid, Duplicate, Bad domain, Unverified, each with its count.
- Problems table: row, name, email, short reason; 100 rows at a time with **Load more**.
- "Continue to New Bulk Email" opens the same checked batch (no second upload or check).

### Bulk Emails (sending)
- Subject and message templates with `{{.Name}}` and `{{.Email}}`; unknown fields such as `{{.Phone}}` are flagged and block sending.
- Confirmation before sending ("N valid recipients will receive emails, M invalid entries will be excluded").
- Emails are queued in PostgreSQL and sent in the background by several workers through a pool of SMTP connections.
- Failed sends are retried automatically with a growing delay (`MAX_JOB_RETRIES`, default 3); invalid rows are saved but never sent.
- Cancel: stops a batch; an email being sent at that moment finishes.
- Retry: all failed emails of a batch, or one email.

### Bulk Emails (watching)
- List of all Bulk Emails, newest first, with status tabs (Not sent, Sending, Completed, With failures, Cancelled), filename search and paging. Uploaded files that were not sent yet show as "Not sent" and can be finished later ("Write and send").
- Bulk Email page: progress bar and counts, refreshed automatically while sending; table of every email with status filter, search and "Load more"; details drawer per email.

### Single email
- Send one plain-text email right away.

### Dashboard
- Sent and failed today, queue (pending, retrying, processing), daily chart (up to 90 days, in your time zone), Bulk Emails that need attention (failed or invalid rows).

### Settings
- Editable from the UI: sender address, SMTP host/port/encryption/username/password, domain check, retries, upload limit, import and job-generation chunk sizes, workers, SMTP pool, poll interval.
- Each setting shows whether it applies instantly or after a restart, and whether it comes from `.env` or was changed in the UI.
- SMTP changes are tested with a real connection before saving; "Send test email" button.
- The SMTP password is stored encrypted and never shown or logged.

### Behind the scenes (for developers)
- Go API (standard library HTTP), PostgreSQL (four tables: `email_batches`, `email_recipients`, `email_import_jobs`, `email_jobs`), goose migrations applied at startup.
- One Go process runs the API and the background workers (import, job generation, sending); crash-safe through checkpoints and leases in the database.
- Vue 3 dashboard (separate deploy), TanStack Query, PrimeVue, Tailwind.
- Structured logs (`log/slog`) to the console and `logs/server.log`, email addresses masked, a request log line per call.
- JSON errors (`{"error": "..."}`), CORS for the dashboard's address.
- Benchmarks with the 1M-row sample: `scripts/bench.sh` ([docs/benchmarks/](benchmarks/README.md)).
- Postman collection of every API route: [postman/](../postman/go-mail-sender.postman_collection.json).
- Dev email goes to Mailpit (`localhost:8025`).

## Pages (dashboard)

| Page | For |
|---|---|
| Dashboard | the day at a glance |
| Bulk Emails | list of everything sent |
| Bulk Email (detail) | progress, every email, retry, cancel |
| New Bulk Email | upload, review, write, preview, send |
| Verify Emails | check a file without sending |
| Single Email | one email to one address |
| Settings | sender, SMTP, behaviour; test email |
| Failures | placeholder (planned) |

## Planned

| Feature | Status | What it adds |
|---|---|---|
| Delete a batch | proposed | remove a batch with its rows and emails (uploads are permanent now) |
| Temporary upload files | proposed | delete the saved `.xlsx` once its import finishes |
| Separate worker process | proposed | run import, generation and sending outside the API process |
| Faster sending | planned | claim emails in groups, send-rate limit |
| Failures page | next | failed emails across all Bulk Emails, grouped by reason |
| Templates | proposed | saved subject/body templates to pick from |
| Recipients search | proposed | "did merchant X get the notice?" across all Bulk Emails |
| Activity log | proposed | who sent, retried or changed settings, and when |
| Subject in the Bulk Emails list | proposed | show each Bulk Email's subject in the list |
| Live progress by server push | proposed | updates without polling |
| Login and roles | deferred | needed before the API leaves localhost |

## Not planned (non-goals)

- Marketing features: scheduling, open/click tracking, A/B tests, recurring campaigns.
- Attachments, HTML email, cc/bcc (emails are plain text).
- Email-provider APIs such as SendGrid (SMTP only).
- **Use on a network beyond your own computer** until login exists.
