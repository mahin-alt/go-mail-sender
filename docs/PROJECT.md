# Project overview

> Changes rarely. Update it when the goal, the users or the scope change, not for every feature.

## Goal

Let staff send **operational emails** (order updates, notices to merchants and customers) to many recipients from a spreadsheet, see exactly what was sent and what failed, and fix problems, without developer help. Not a marketing tool.

## Users

- **Operations / support staff:** upload a list, send, watch progress, retry failures. Not technical.
- **Developers:** run and configure the service, add features.

## Core use cases

1. **Bulk send:** upload an `.xlsx` with `name` and `email` columns, write a subject and body with `{{.Name}}` / `{{.Email}}`, send. Emails go out in the background.
2. **Check before sending:** see which rows are invalid (empty, malformed, domain cannot receive mail) with the row number and reason, without sending anything; then send that same checked file without uploading it again.
3. **Follow progress:** per-batch counts of sent, failed, pending, processing, cancelled and invalid, plus a paged list of every email.
4. **Recover from failures:** retry failed emails (one or a whole batch). Invalid rows are never sent.
5. **Single email:** send one email to one address.
6. **Configure:** change sender, SMTP server and a few behaviour settings from the UI instead of editing files.
7. **Overview:** what went out today, what is queued, what needs attention.

## Glossary

| Term | Meaning |
|---|---|
| **Bulk email** (UI) / **batch** (API, `email_batches`) | One uploaded file: its rows, counters, and (once sent) its subject/body templates. Type `verification` (Verify Emails) or `bulk_send` |
| **Recipient** (`email_recipients`) | One spreadsheet row, `valid`, `unverified` (domain could not be checked, still sent) or `invalid` with a reason (`empty_email`, `empty_name`, `invalid_email`, `bad_domain`). Duplicates are not flagged |
| **Import job** (`email_import_jobs`) | The background reading of a batch's file, with a checkpoint so it resumes after a crash |
| **Job / email** (`email_jobs`) | One email to deliver to one recipient row, created after Send; built from the templates at send time |
| **Job status** | `pending` (waiting, possibly a retry), `processing`, `sent`, `failed` (gave up), `cancelled` |
| **Batch status** | `importing`, `import_failed`, `draft` (imported, not sent), `generating_jobs`, `queued`, `sending`, `completed`, `completed_with_errors`, `failed`, `cancelled` |
| **Domain check** | DNS lookup that rejects recipients whose domain has no mail server (MX or A record) |
| **TLS mode** | How the connection to the SMTP server is encrypted: `auto`, `none`, `starttls`, `tls` |
| **Override** | A setting changed in the dashboard (stored in `app_settings`); it beats the environment value |

## Architecture in one paragraph

A Go HTTP API (stdlib `net/http`) stores batches, their rows and their emails in PostgreSQL (four tables, decision 0013). Uploads are streamed to disk and imported by a background worker in checkpointed chunks; after Send a generator creates the emails. Background workers claim jobs with `FOR UPDATE SKIP LOCKED` and send them through a pooled SMTP client, retrying with backoff. Schema changes are goose migrations applied at startup. A separate Vue 3 single-page dashboard (in `frontend/`, deployed on its own) talks to the API over CORS. In development, email goes to Mailpit.

## Non-goals (for now)

- Marketing features: scheduling, open/click tracking, A/B tests, recurring campaigns.
- Attachments, HTML email, cc/bcc (messages are plain text).
- Multiple users and roles; authentication is deferred ([decision 0002](decisions/0002-no-auth-for-now.md)). **The API must not be exposed beyond localhost until auth exists.**
- Email-provider APIs (SendGrid etc.): SMTP only.

## Where things are

- How to run, API table, migration setup: [README.md](../README.md)
- Feature plans and progress: [ROADMAP.md](ROADMAP.md)
- Why we chose what we chose: [decisions/](decisions/)
