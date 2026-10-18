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
| `POST /api/v1/batches` | **Queue background email campaign** | `multipart/form-data`: `file`, `subject`, `body` | `202 {"batch_id":1,"total_recipients":42,"status":"pending"}` |
| `GET /api/v1/batches/{id}` | **Check campaign progress & counts** | URL parameter `id` | `200 {"batch_id":1,"status":"completed","sent":42,"pending":0}` |

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
