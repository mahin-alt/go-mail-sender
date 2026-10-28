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

go run ./cmd/server   # creates/updates the database tables automatically on startup
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
internal/store/               batches, settings, stats, and the FOR UPDATE SKIP LOCKED job queue
internal/migrations/          the database schema as numbered SQL files (goose), embedded in the binary
cmd/migrate/                  command for running, inspecting and creating migrations by hand
internal/mailer/              SMTP connection pool and message delivery
internal/worker/              background Go worker goroutines with graceful shutdown
internal/api/                 HTTP routes, handlers, and validation
```

---

## Tests

Run all unit tests:
```bash
go test ./...
```


## Database migrations

The database schema is defined by numbered SQL files in [internal/migrations/sql](internal/migrations/sql), applied with [goose](https://github.com/pressly/goose). Goose keeps a table called `goose_db_version` in your database that records which files have already run, and each startup (or `migrate up`) applies only the missing ones, in order.

| File | What it does |
|---|---|
| `00001_create_batches_and_email_jobs.sql` | the original tables and the job-queue indexes |
| `00002_create_app_settings.sql` | settings edited from the dashboard |
| `00003_add_stats_indexes.sql` | indexes that keep the Overview stats fast |

Every file has an **Up** section (apply the change) and a **Down** section (undo it).

**Normal use needs nothing:** the server applies pending migrations when it starts. A database created by the older code adopts them safely, because the first migrations use `IF NOT EXISTS` and existing data is left alone. Several servers starting at once are safe too; a database lock makes one apply them while the others wait.

**Running them by hand:**

```bash
go run ./cmd/migrate status            # which migrations are applied, and when
go run ./cmd/migrate up                # apply ALL pending migrations
go run ./cmd/migrate up-by-one         # apply only the NEXT pending migration
go run ./cmd/migrate up-to 2           # apply pending ones up to and including number 2
go run ./cmd/migrate down -yes         # undo only the LAST applied migration
go run ./cmd/migrate down-to 1 -yes    # undo newest-first until 1 is the newest applied
go run ./cmd/migrate down-to 0 -yes    # undo EVERYTHING (drops all tables and their data)
go run ./cmd/migrate create add_x      # make the next numbered file for a new change
```

Rollbacks (`down`, `down-to`) can delete data, so they refuse to run without `-yes`. Numbers are the leading digits of the file name (`00002_...` is `2`); an unknown number is an error.

**Making a schema change** (for example adding a column):

1. `go run ./cmd/migrate create add_failed_at` creates `00004_add_failed_at.sql`.
2. Write the change under `-- +goose Up` (`ALTER TABLE email_jobs ADD COLUMN failed_at TIMESTAMPTZ;`) and its exact reverse under `-- +goose Down`.
3. Run the tests, then start the server (or `migrate up`). Commit the file with the code that needs it.

Rules of thumb: never edit a migration that has already been applied anywhere (add a new one instead); one focused change per file; build indexes on large tables with `CREATE INDEX CONCURRENTLY` and a `-- +goose NO TRANSACTION` line, as `00003` does.

**Production deploys:** set `AUTO_MIGRATE=false` to run migrations as a separate step (`migrate up`) before starting the new version. The server then refuses to start if the database is behind, instead of failing later on real requests.

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

Tests that need PostgreSQL are skipped unless `TEST_DATABASE_URL` is set. They create the tables themselves (by running the migrations). Because some tests wipe tables or roll migrations back, they refuse to run against the `public` schema of any database whose name does not contain "test". Point `TEST_DATABASE_URL` at a scratch schema or a dedicated test database, and run packages one at a time:

```bash
psql "$DATABASE_URL" -c 'CREATE SCHEMA scratch_test'
TEST_DATABASE_URL="$DATABASE_URL&search_path=scratch_test" go test -p 1 ./...
psql "$DATABASE_URL" -c 'DROP SCHEMA scratch_test CASCADE'
```
