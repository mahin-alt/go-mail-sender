# go-mail-sender

A tiny HTTP API that sends email over SMTP (Mailtrap sandbox in development). Standard library only, plus `godotenv`.

## Run it

```bash
cp .env.example .env   # fill in your Mailtrap credentials
go run ./cmd/server
```

```bash
curl -i -X POST localhost:8080/api/v1/emails \
  -H 'Content-Type: application/json' \
  -d '{"to":"someone@example.com","subject":"Hello","body":"Sent from my Go API"}'
```

Test: `go test ./...`

## Designing the API first

Before writing a handler, write down the **contract**: what a client sends and what it gets back.
Do this first because:

- **The contract is the hard-to-change part.** Once clients depend on a URL or JSON shape, changing it breaks them. Internals (SMTP vs. an email API) can change freely.
- **It forces you to answer the real questions early:** who calls this, what are the *things* (resources), what can go wrong, and what should the client *not* have to know?
- **It's cheap to iterate.** Editing a table in a README costs nothing; editing shipped code and clients does.
- **It becomes your checklist and your tests.** Each row below maps to a handler branch and a test case.

Questions we answered for this project:

| Question | Decision |
|---|---|
| What is the resource? | An *email* → noun URL `/emails`, not a verb like `/sendMail`. POST means "create one". |
| Can the contract evolve? | Version in the URL: `/api/v1/...`. |
| What does the client control? | `to`, `subject`, `body`. **Not** `from` - the server owns the sender, so nobody can spoof it. |
| What does the client *not* see? | SMTP details. Real errors are logged server-side only. |
| How are errors reported? | Correct HTTP status + one consistent JSON shape: `{"error": "..."}`. |

### The contract

| Method & path | Purpose |
|---|---|
| `GET /healthz` | Liveness check → `200 {"status":"ok"}` |
| `POST /api/v1/emails` | Send an email |

`POST /api/v1/emails` request:

```json
{ "to": "someone@example.com", "subject": "Hello", "body": "plain text body" }
```

| Status | When | Body |
|---|---|---|
| `200` | Accepted by the SMTP server | `{"message":"email sent"}` |
| `400` | Bad JSON, unknown field, missing/invalid `to`, empty subject/body, line breaks in `to`/`subject` | `{"error":"..."}` |
| `405` | Wrong HTTP method | - |
| `413` | Body larger than 1 MB | `{"error":"..."}` |
| `502` | SMTP server failed (not the client's fault) | `{"error":"failed to send email"}` |

Rule of thumb: **4xx = the client should fix its request; 5xx = the server (or something it depends on) failed.**

## Project layout

```
cmd/server/main.go        entry point - only wires things together
internal/config/          reads settings from env / .env
internal/mailer/          Mailer interface + SMTP implementation (no HTTP knowledge)
internal/api/             HTTP handlers, validation, routes (no SMTP knowledge)
```

- `cmd/<name>/` = one folder per executable.
- `internal/` = the Go compiler forbids other modules from importing it, so it's your private code.
- Dependencies point one way: `main → api → mailer → config`. The `api` package only knows the `Mailer` *interface*, which is why tests can use a fake.

## Deliberately left out (good next steps)

Authentication (API key), rate limiting, HTML emails/attachments, sending in the background with a queue,
request logging middleware, graceful shutdown (`srv.Shutdown`), config validation at startup.
