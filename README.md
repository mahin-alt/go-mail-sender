# go-mail-sender

An asynchronous, scalable email processing API in Go. Accepts Excel files (`.xlsx`), parses recipients, renders personalized greetings via Go templates, and dispatches emails using background Go worker goroutines backed by PostgreSQL (`FOR UPDATE SKIP LOCKED`) and a connection-pooled SMTP engine (Mailpit / Mailtrap / production).

## Features

- **Excel Parsing**: Automatically extracts recipient names and emails from `.xlsx` files with row-level validation.
- **Batches & Bulk Jobs**: Upload an Excel file to create a single Batch with individual, trackable email jobs.
- **Personalized Templating**: Uses Go's `text/template` engine to customize subjects and bodies with `{{.Name}}` and `{{.Email}}`.
- **PostgreSQL Job Queue**: Concurrency-safe job claiming using `SELECT ... FOR UPDATE SKIP LOCKED` (no duplicate sends, no lock contention).
- **SMTP Connection Pool**: Configurable persistent connection pool to reuse SMTP connections and protect mail servers from socket exhaustion.
- **Configurable Concurrency**: Tune worker count, pool size, and poll interval via `.env`.
- **Local Mailpit & Mailtrap Support**: Seamlessly works with local Mailpit (no auth required) or Mailtrap/production.
- **Graceful Shutdown**: On `SIGINT` / `SIGTERM`, finishes in-flight emails, drains worker pools, and closes connections cleanly.

---

## Quickstart

### 1. Prerequisites
- **PostgreSQL** running locally on port 5432 (database named `sendmail` or custom `DATABASE_URL`).
- **Mailpit** running on port 1025 (or Mailtrap sandbox).

### 2. Configure & Run

```bash
cp .env.example .env
# Edit .env with your DATABASE_URL and SMTP settings

go run ./cmd/server
```

---

## API Contract

| Method & Path | Purpose | Request | Response |
|---|---|---|---|
| `GET /healthz` | Liveness check | - | `200 {"status":"ok"}` |
| `POST /api/v1/emails` | Synchronous single email | JSON: `{"to","subject","body"}` | `200 {"message":"email sent"}` |
| `POST /api/v1/recipients/parse` | Preview parsed Excel recipients | `multipart/form-data`: `file` | `200 {"valid":[...],"problems":[...]}` |
| `POST /api/v1/batches` | **Queue background email campaign** | `multipart/form-data`: `file`, `subject`, `body` | `202 {"batch_id":1,"total_rows":42,"valid_count":38,"invalid_count":4}` |
| `GET /api/v1/batches/{id}` | **Check campaign progress & counts** | URL parameter `id` | `200 {"batch_id":1,"status":"completed","sent":38,"invalid":4}` |
| `GET /api/v1/batches/{id}/jobs` | **List jobs for a batch (filter by status)** | Query: `?status=invalid` | `200 [{"id":4,"recipient_email":"...","status":"invalid","last_error":"..."}]` |
| `GET /api/v1/batches` | **List batches with job counts** | Query: `status`, `q` (filename), `limit` (default 20, max 100), `offset` | `200 {"items":[{"id":1,"filename":"...","status":"completed","total":42,"sent":38,"failed":0,"invalid":4}],"total":1,"limit":20,"offset":0}` |
| `GET /api/v1/batches/{id}/jobs?limit=` | **Paged job list** (adding `limit`, `offset` or `q` switches to the paged form) | Query: `status`, `q` (email/name), `limit`, `offset` | `200 {"items":[...],"total":42,"limit":20,"offset":0}` |
| `POST /api/v1/batches/{id}/retry` | **Requeue all failed jobs in a batch** (invalid rows are skipped) | none | `200 {"retried":3}` |
| `POST /api/v1/jobs/{id}/retry` | **Requeue one failed job** | none | `200 {"retried":1}`, `404` if not failed |
| `GET /api/v1/stats` | **Overview numbers**: today's sent/failed, queue, daily series, batches needing attention | Query: `days` (1-90, default 14), `tz` (IANA name, default `UTC`) | `200 {"today":{"sent":59,"failed":5},"queue":{"pending":53,"retrying":3,"processing":5},"failed_total":8,"active_batches":1,"daily":[{"date":"2026-10-27","sent":59,"failed":5}],"needs_attention":[{"batch_id":2,"reasons":["failed_jobs"],...}]}` |
| `GET /api/v1/settings` | **List editable settings** with effective value, source (`env`/`override`), `apply` (`instant`/`restart`/`readonly`) and `restart_required`. Secrets are never returned | none | `200 {"items":[{"key":"SMTP_HOST","value":"localhost","source":"env","apply":"instant",...}]}` |
| `PUT /api/v1/settings` | **Change or reset settings.** All-or-nothing. Changing SMTP settings first opens a real connection with the new values and refuses the save if it fails | JSON `{"values":{"SMTP_HOST":"smtp.example.com"},"reset":["MAIL_FROM"]}` | `200 {"items":[...]}`, `400 {"error":"..."}` |
| `POST /api/v1/settings/smtp/test` | **Send a real test email** over a fresh connection. Omitted fields use the current settings, so unsaved edits can be tried first | JSON `{"to":"me@example.com","port":"587","tls_mode":"starttls"}` | `200 {"message":"test email sent to ..."}`, `502` with the SMTP reason |

---

## Examples

### 1. Queue an Email Campaign from an Excel File

```bash
curl -i -X POST http://localhost:8080/api/v1/batches \
  -F "file=@samples/1_valid.xlsx" \
  -F "subject=Hello {{.Name}}" \
  -F "body=Hi {{.Name}},\n\nWelcome to our platform!"
```

Response:
```json
HTTP/1.1 202 Accepted
Content-Type: application/json

{
  "batch_id": 1,
  "filename": "1_valid.xlsx",
  "total_recipients": 5,
  "status": "pending",
  "message": "batch accepted and email jobs queued for processing"
}
```

### 2. Check Campaign Progress

```bash
curl -i http://localhost:8080/api/v1/batches/1
```

Response:
```json
HTTP/1.1 200 OK
Content-Type: application/json

{
  "batch_id": 1,
  "filename": "1_valid.xlsx",
  "status": "completed",
  "total": 5,
  "sent": 5,
  "failed": 0,
  "pending": 0,
  "processing": 0
}
```

---

## Architecture & Project Layout

```
cmd/server/main.go            entry point - wires config, database, smtp pool, worker pool, http server
internal/config/              reads settings from process env / .env
internal/template/            personalized template engine using text/template
internal/recipients/          Excel .xlsx parser and validation
internal/store/               PostgreSQL schema, batches, and FOR UPDATE SKIP LOCKED job queue
internal/mailer/              SMTP connection pool and message delivery
internal/worker/              background Go worker goroutines with graceful shutdown
internal/api/                 HTTP routes, handlers, and validation
schema.sql                    PostgreSQL database schema
```

---

## Tests

Run all unit tests:
```bash
go test ./...
```


## SMTP encryption and runtime settings

**`SMTP_TLS_MODE`** controls encryption to the mail server:

| Mode | Behaviour | Typical use |
|---|---|---|
| `auto` (default) | Upgrade with STARTTLS if the server offers it, otherwise plain | works for Mailpit and most servers |
| `none` | Never encrypt | local test servers |
| `starttls` | Require STARTTLS, fail if not offered | real providers on port 587 |
| `tls` | Encrypted from the first byte (implicit TLS) | real providers on port 465 |

Certificates are verified against the system trust store. To trust a private CA or a self-signed certificate in development, point `SSL_CERT_FILE` at it.

**Settings overrides.** Selected env settings can be changed from the dashboard (`/api/v1/settings`). Saved values are stored in the `app_settings` table and override the environment; resetting a setting deletes the override. SMTP, sender, domain check and retry settings apply instantly; worker count, pool size and poll interval are saved but reported with `restart_required` until the server restarts. `DATABASE_URL` and the HTTP address/timeouts are never editable.

The SMTP password is stored encrypted (AES-256-GCM) with the key in `SETTINGS_ENCRYPTION_KEY` (`openssl rand -base64 32`). Without the key the password cannot be edited from the dashboard. Keep the key stable: if it changes, stored secrets are ignored and the environment value is used.

> **These endpoints have no authentication yet.** Do not expose the API beyond localhost until login is added.

## Running the database tests

Tests that need PostgreSQL are skipped unless `TEST_DATABASE_URL` is set. Point it at a scratch schema or a database with "test" in its name (the settings tests refuse to run against the `public` schema of any other database), and run packages one at a time:

```bash
psql "$DATABASE_URL" -c 'CREATE SCHEMA scratch_test'
TEST_DATABASE_URL="$DATABASE_URL&search_path=scratch_test" go test -p 1 ./...
psql "$DATABASE_URL" -c 'DROP SCHEMA scratch_test CASCADE'
```
