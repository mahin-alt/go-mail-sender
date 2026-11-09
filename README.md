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

A Postman collection with every endpoint is in [postman/go-mail-sender.postman_collection.json](postman/go-mail-sender.postman_collection.json) (Postman: Import -> the file). Requests that create something save their id (`upload_id`, `batch_id`, `job_id`) for the next ones; `baseUrl` defaults to `http://localhost:8080`. Requests marked "really sends" send email through the configured SMTP server.

| Method & Path | Purpose | Request | Response |
|---|---|---|---|
| `GET /healthz` | Liveness check | - | `200 {"status":"ok"}` |
| `POST /api/v1/emails` | Synchronous single email | JSON: `{"to","subject","body"}` | `200 {"message":"email sent"}` |
| `POST /api/v1/uploads` | **Check a spreadsheet** (nothing is sent). The file is read row by row and every row is kept for 24 hours with its kind: `ok`, `unverified` (sent, domain not checked), or skipped: `empty_email`, `empty_name`, `invalid_email`, `duplicate`, `bad_domain`. Returns totals only, never the rows | `multipart/form-data`: `file` (up to `MAX_UPLOAD_MB`) | `201 {"upload_id":1,"filename":"...","status":"ready","total_rows":950040,"valid_count":780493,"problem_count":169547,"unverified_count":0,"problem_counts":{"empty_email":69507,"bad_domain":100040,...},"expires_at":"..."}`, `400` bad file, `413` over the limit |
| `GET /api/v1/uploads/{id}` | **Totals of a checked file** | URL parameter `id` | `200` same shape as above, `404` unknown or expired |
| `GET /api/v1/uploads/{id}/problems` | **One page of a checked file's problem rows**, in row order. Pass `next_after` back as `after` for the next page | Query: `kind` (comma separated; default every skipped kind; `unverified` allowed), `q` (name/email), `after` (row number), `limit` (default 100, max 500) | `200 {"items":[{"row":4,"name":"...","email":"...","kind":"bad_domain","reason":"..."}],"next_after":616}` (`null` on the last page) |
| `GET /api/v1/uploads/{id}/sample` | **First rows that will be sent**, for the email preview | Query: `limit` (default 5, max 20) | `200 {"items":[{"row":2,"name":"...","email":"...","kind":"ok"}]}` |
| `POST /api/v1/batches` | **Queue background email campaign** (the file is checked again; the rows are not returned, the job list shows them) | `multipart/form-data`: `file` (up to `MAX_UPLOAD_MB`), `subject`, `body` | `202 {"batch_id":1,"total_rows":42,"valid_count":38,"invalid_count":4,"unverified_count":0}` |
| `GET /api/v1/batches/{id}` | **Check campaign progress & counts** | URL parameter `id` | `200 {"batch_id":1,"status":"completed","sent":38,"invalid":4,"created_at":"...","completed_at":"..."}` |
| `GET /api/v1/batches` | **List batches with job counts** | Query: `status`, `q` (filename), `limit` (default 20, max 100), `offset` | `200 {"items":[{"id":1,"filename":"...","status":"completed","total":42,"sent":38,"failed":0,"invalid":4}],"total":1,"limit":20,"offset":0}` |
| `GET /api/v1/batches/{id}/jobs` | **Paged job list** (always paged) | Query: `status`, `q` (email/name), `limit` (default 20, max 100), `offset` | `200 {"items":[{"id":4,"recipient_email":"...","status":"invalid","last_error":"..."}],"total":42,"limit":20,"offset":0}` |
| `POST /api/v1/batches/{id}/retry` | **Requeue all failed jobs in a batch** (invalid and cancelled rows are skipped) | none | `200 {"retried":3}`, `409` if the batch was cancelled |
| `POST /api/v1/batches/{id}/cancel` | **Stop a batch that is still sending.** Unsent (`pending`) emails become `cancelled` and are never sent or retried; an email being sent right now finishes | none | `200 {"cancelled":630036}`, `404`, `409` if the batch already finished or was cancelled |
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
internal/logging/             log/slog setup (levels, text/json, optional log file) and email masking
internal/template/            personalized template engine using text/template
internal/recipients/          Excel .xlsx parser and validation
internal/store/               batches, settings, stats, and the FOR UPDATE SKIP LOCKED job queue
internal/migrations/          the database schema as numbered SQL files (goose), embedded in the binary
internal/mailer/              SMTP connection pool and message delivery
internal/worker/              background Go worker goroutines with graceful shutdown
internal/api/                 HTTP routes, handlers, validation, CORS and the request log
```

---

## Logging

The server logs with Go's built-in `log/slog`: every line has a level and `key=value` fields.

| `.env` | Values | Meaning |
|---|---|---|
| `LOG_LEVEL` | `debug`, `info` (default), `warn`, `error` | The least serious level written (a minimum: `debug` shows the most). `info` shows the story: startup, requests that change something, every rejected or failed request, batch created / completed with timings, retries and failures. `debug` adds the dashboard's polling reads and one line per email sent, so use it only while investigating |
| `LOG_FORMAT` | `text` (default), `json` | `text` is easy to read; `json` is for log tools |
| `LOG_FILE` | a path, e.g. `logs/server.log` | Every line is also appended to this file (the folder is created). Empty = terminal only. `logs/` is gitignored |

Changing them needs a restart. Read the file with `tail -f logs/server.log`, or `grep level=ERROR logs/server.log`.

```
level=INFO  msg="recipients parsed" valid=780493 problems=169547 unverified=0 parse=6.191s domain_check=467ms total=6.707s
level=INFO  msg="batch created" batch_id=1 filename=9_mixed_everything.xlsx pending=3 invalid=5 unverified=0 parse=1ms domain_check=0s render=0s insert=4ms total=5ms
level=INFO  msg=request req_id=6ecc38cf method=POST path=/api/v1/batches status=202 duration=5ms
level=DEBUG msg="email sent" job_id=1 batch_id=1 to=a***@example.com worker=4 send=3ms queued=4ms
level=INFO  msg="batch completed" batch_id=1 filename=9_mixed_everything.xlsx sent=3 failed=0 invalid=5 duration=16ms rate=191.1/s
level=WARN  msg=request req_id=e2da5ecd method=POST path=/api/v1/uploads status=413 duration=30ms
```

- **Request log:** one line per API call (not `/healthz`). 5xx is ERROR, 4xx WARN; successful POST/PUT/DELETE are INFO; successful GET/HEAD and CORS preflights are DEBUG (the dashboard polls every few seconds). The same `req_id` is returned in the `X-Request-ID` response header, so a failed request seen in the browser can be found in the log.
- **Timings (INFO):** `recipients parsed` and `batch created` split the upload into `parse`, `domain_check`, `render`, `insert` and `total`; `batch completed` gives `duration` (upload to last email) and `rate` (emails handled per second).
- **Emails:** a send is DEBUG with `send` (SMTP time) and `queued` (how long it waited for a worker); a retry is WARN and a final failure ERROR, both with `send`. Addresses are masked (`a***@gmail.com`).
- **Never logged:** passwords and other secrets, full email addresses, email bodies.
- **No rotation yet:** the file grows until you delete it (a restart appends). In production prefer `LOG_FILE=` empty and let systemd or Docker keep the logs.

---

## Tests

Run all unit tests:
```bash
go test ./...
```

Benchmarks with the 1M-row sample (parser plus the API on a scratch schema, about 6 minutes): `scripts/bench.sh`. How to read and compare results: [docs/benchmarks/README.md](docs/benchmarks/README.md).

## Database migrations

The database schema is defined by numbered SQL files in [internal/migrations/sql](internal/migrations/sql), applied with [goose](https://github.com/pressly/goose). Goose keeps a table called `goose_db_version` in your database that records which files have already run, and only the missing ones are applied, in order.

| File | What it does |
|---|---|
| `00001_create_batches_and_email_jobs.sql` | the original tables and the job-queue indexes |
| `00002_create_app_settings.sql` | settings edited from the dashboard |
| `00003_add_stats_indexes.sql` | indexes that keep the Overview stats fast |

Every file has an **Up** section (apply the change) and a **Down** section (undo it).

**Normal use needs nothing:** the server applies pending migrations when it starts. A database created by the older code adopts them safely, because the first migrations use `IF NOT EXISTS` and existing data is left alone. Several servers starting at once are safe too; a database lock makes one apply them while the others wait. The goose command-line tool below is for everything else: checking status, stepping through, rolling back and creating new migrations.

### Setting up the goose command (once per developer)

1. **Install the pinned version** (needs Go; the project uses goose **v3.29.0**, keep everyone on the same one):

   ```bash
   go install github.com/pressly/goose/v3/cmd/goose@v3.29.0
   ```

   Then check it works and prints `goose version: v3.29.0`:

   ```bash
   goose -version
   ```

   If the command is not found, add Go's bin folder to your `PATH`: `export PATH="$PATH:$(go env GOPATH)/bin"` (put it in your shell profile).

   *Alternatives:* on a Mac, `brew install goose` (not pinned, you get the latest); on Windows or without Go, download a binary from the [releases page](https://github.com/pressly/goose/releases). Whatever you use, make sure `goose -version` matches the version above.

2. **Configure it.** Goose reads settings from a `.env` file in the current folder. Copy the three `GOOSE_*` lines from `.env.example` into your `.env`, with `GOOSE_DBSTRING` set to the same URL as `DATABASE_URL`:

   ```
   GOOSE_DRIVER=postgres
   GOOSE_DBSTRING=postgres://localhost:5432/sendmail?sslmode=disable
   GOOSE_MIGRATION_DIR=internal/migrations/sql
   ```

3. **Run goose from the repository root**, because the migrations path is relative to it.

### Commands

| What you want | Command |
|---|---|
| See what is applied and what is pending | `goose status` |
| Show the current version number | `goose version` |
| Apply **all** pending migrations | `goose up` |
| Apply only the **next** one | `goose up-by-one` |
| Apply up to and including number 2 | `goose up-to 2` |
| Undo only the **last** applied one | `goose down` |
| Undo newest-first until 1 is the newest applied | `goose down-to 1` |
| Undo **everything** (drops all tables and their data) | `goose reset` |
| Undo the last one and re-apply it | `goose redo` |
| Check the migration files for mistakes | `goose validate` |
| Create the next numbered file | `goose -s create add_failed_at sql` |

Numbers are the leading digits of the file name (`00002_...` is `2`). Note the option `-s` goes **before** `create`; it makes the file name `00004_add_failed_at.sql` instead of a timestamp.

### Read this before running destructive commands

- **Goose asks for no confirmation.** `goose reset`, `goose down-to ...` and `goose down` run immediately, and rolling back `00001` deletes the `batches` and `email_jobs` tables **and all their data**. Run `goose status` first.
- **Use your development database only.** Keep the URL of any shared, staging or production database out of the `.env` that goose reads, so a mistyped command can only hit your own.
- **A number that does not exist is not an error:** `goose up-to 99` simply applies everything. Check `goose status` for the real numbers.

### Making a schema change (for example adding a column)

1. `goose -s create add_failed_at sql` creates `internal/migrations/sql/00004_add_failed_at.sql`.
2. Write the change under `-- +goose Up` (`ALTER TABLE email_jobs ADD COLUMN failed_at TIMESTAMPTZ;`) and its exact reverse under `-- +goose Down`.
3. Run `goose validate`, apply it with `goose up`, try `goose down` and `goose up` again to prove the rollback works, then run the tests. Commit the file together with the code that needs it.

Rules of thumb: never edit a migration that has already been applied anywhere (add a new one instead); one focused change per file; build indexes on large tables with `CREATE INDEX CONCURRENTLY` and a `-- +goose NO TRANSACTION` line, as `00003` does.

### Production deploys

Set `AUTO_MIGRATE=false` to run migrations as a separate step before starting the new version. The server then refuses to start if the database is behind, instead of failing later on real requests. In that setup, pass the goose settings as real environment variables (`GOOSE_DRIVER`, `GOOSE_DBSTRING`, `GOOSE_MIGRATION_DIR`) in your deploy step rather than through a `.env` file, and run `goose up`.

## SMTP encryption and runtime settings

**`SMTP_TLS_MODE`** controls encryption to the mail server:

| Mode | Behaviour | Typical use |
|---|---|---|
| `auto` (default) | Upgrade with STARTTLS if the server offers it, otherwise plain | works for Mailpit and most servers |
| `none` | Never encrypt | local test servers |
| `starttls` | Require STARTTLS, fail if not offered | real providers on port 587 |
| `tls` | Encrypted from the first byte (implicit TLS) | real providers on port 465 |

Certificates are verified against the system trust store. To trust a private CA or a self-signed certificate in development, point `SSL_CERT_FILE` at it.

**Settings overrides.** Selected env settings can be changed from the dashboard (`/api/v1/settings`). Saved values are stored in the `app_settings` table and override the environment; resetting a setting deletes the override. SMTP, sender, domain check, retry and upload size (`MAX_UPLOAD_MB`) settings apply instantly; worker count, pool size and poll interval are saved but reported with `restart_required` until the server restarts. `DATABASE_URL` and the HTTP address/timeouts are never editable.

**Upload size.** `MAX_UPLOAD_MB` (default 10, allowed 1-50) is the largest spreadsheet both upload endpoints accept; larger files get `413 {"error":"file too large: the limit is N MB"}`. It can be changed on the Settings page without a restart; uploads already in progress keep the limit they started with. The ceiling exists because parsing holds the whole sheet in memory (roughly 13x the file size).

The SMTP password is stored encrypted (AES-256-GCM) with the key in `SETTINGS_ENCRYPTION_KEY` (`openssl rand -base64 32`). Without the key the password cannot be edited from the dashboard. Keep the key stable: if it changes, stored secrets are ignored and the environment value is used.

> **These endpoints have no authentication yet.** Do not expose the API beyond localhost until login is added.

## Running the database tests

Tests that need PostgreSQL are skipped unless `TEST_DATABASE_URL` is set. They create the tables themselves (by running the migrations). Because some tests wipe tables or roll migrations back, they refuse to run against the `public` schema of any database whose name does not contain "test". Point `TEST_DATABASE_URL` at a scratch schema or a dedicated test database, and run packages one at a time:

```bash
psql "$DATABASE_URL" -c 'CREATE SCHEMA scratch_test'
TEST_DATABASE_URL="$DATABASE_URL&search_path=scratch_test" go test -p 1 ./...
psql "$DATABASE_URL" -c 'DROP SCHEMA scratch_test CASCADE'
```
