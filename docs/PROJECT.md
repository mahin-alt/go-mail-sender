# Project overview

> Changes rarely. Update it when the goal, the users or the scope change, not for every feature.

## Goal

Let staff send **operational emails** (order updates, notices to merchants and customers) to many recipients from a spreadsheet, see exactly what was sent and what failed, and fix problems, without developer help. Not a marketing tool.

## Users

- **Operations / support staff:** upload a list, send, watch progress, retry failures. Not technical.
- **Developers:** run and configure the service, add features.

## Core use cases

1. **Bulk send:** upload an `.xlsx` with `name` and `email` columns, write a subject and body with `{{.Name}}` / `{{.Email}}`, send. Emails go out in the background.
2. **Check before sending:** see which rows are invalid (empty, malformed, duplicate, domain cannot receive mail) with the row number and reason, without sending anything.
3. **Follow progress:** per-batch counts of sent, failed, pending, processing and invalid, plus a job-level list.
4. **Recover from failures:** retry failed emails (one or a whole batch). Invalid rows are never sent.
5. **Single email:** send one email to one address.
6. **Configure:** change sender, SMTP server and a few behaviour settings from the UI instead of editing files.
7. **Overview:** what went out today, what is queued, what needs attention.

## Glossary

| Term | Meaning |
|---|---|
| **Bulk email** (UI) / **batch** (API, DB) | One uploaded file plus one subject/body template and all the emails it produces |
| **Job** (`email_jobs`) | One email to one recipient inside a batch |
| **Job status** | `pending` (waiting, possibly a retry), `processing`, `sent`, `failed` (gave up after max attempts), `invalid` (rejected at upload, never sent) |
| **Batch status** | `pending`, `processing`, `completed` (nothing pending or processing is left, even if some jobs failed) |
| **Domain check** | DNS lookup that rejects recipients whose domain has no mail server (MX or A record) |
| **TLS mode** | How the connection to the SMTP server is encrypted: `auto`, `none`, `starttls`, `tls` |
| **Override** | A setting changed in the dashboard (stored in `app_settings`); it beats the environment value |

## Architecture in one paragraph

A Go HTTP API (stdlib `net/http`) stores batches and jobs in PostgreSQL. Background workers claim jobs with `FOR UPDATE SKIP LOCKED` and send them through a pooled SMTP client, retrying with backoff. Schema changes are goose migrations applied at startup. A separate Vue 3 single-page dashboard (in `frontend/`, deployed on its own) talks to the API over CORS. In development, email goes to Mailpit.

## Non-goals (for now)

- Marketing features: scheduling, open/click tracking, A/B tests, recurring campaigns.
- Attachments, HTML email, cc/bcc (messages are plain text).
- Multiple users and roles; authentication is deferred ([decision 0002](decisions/0002-no-auth-for-now.md)). **The API must not be exposed beyond localhost until auth exists.**
- Email-provider APIs (SendGrid etc.): SMTP only.

## Where things are

- How to run, API table, migration setup: [README.md](../README.md)
- Feature plans and progress: [ROADMAP.md](ROADMAP.md)
- Why we chose what we chose: [decisions/](decisions/)
