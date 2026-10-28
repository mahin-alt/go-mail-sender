# go-mail-sender

A Go service that sends operational emails (order updates, merchant and customer notices) from uploaded `.xlsx` files, plus a Vue dashboard for staff (separate deploy, lives in `frontend/`, Phase 0 scaffold done).

## Read first (in this order)

1. [docs/PROJECT.md](docs/PROJECT.md): goal, users, use cases, glossary, non-goals
2. [docs/ROADMAP.md](docs/ROADMAP.md): every feature and its status
3. `docs/features/<feature>.md`: start with its **"Where we left off"** section (last done / next / blocked), then the plan, decisions and checklist. The feature is usually the one matching the current git branch; also check `git status` and `git log -5`
4. [docs/decisions/](docs/decisions/): project-wide decisions and why (goose, no auth yet, separate frontend...)
5. [README.md](README.md): how to run it, the API table, migrations setup
6. Frontend work only: [docs/frontend/](docs/frontend/) (design brief, design system, screens, implementation notes); path rules in `.claude/rules/frontend.md`

## Working rules

- **Keep "Where we left off" current.** After finishing any chunk of work, update that section in the feature doc (last done, next, blocked, date) and tick the checkboxes, in the same commit as the code. Do it before ending the turn, also when work is paused half done (say what is unfinished and why).
- **Ask before starting the next piece of work.** When a task is finished, report it and propose the next step as one clear question. Do not begin the next roadmap item, branch or feature until the user says yes.
- **Explain options and recommend.** When there are several ways to do something (design, architecture, library, approach), lay out the realistic options in plain beginner language: what each is, its trade-offs, and when it fits. Then give a clear recommendation and why, and let the user choose before building. Record the outcome in the feature doc, or in `docs/decisions/` if it is project-wide.

- **Definition of done:** tests pass, the checkbox in the feature doc is ticked, docs/README updated if behaviour changed. Commit only when the user asks.
- **Never write test data to the developer's real database** (the `DATABASE_URL` in `.env`). Read-only checks against it are fine. Use a scratch schema or an isolated Postgres for anything that writes.
- **Database tests** only run with `TEST_DATABASE_URL`, in a scratch schema or a database whose name contains "test" (the tests refuse the `public` schema otherwise), **one package at a time** (`-p 1`):
  ```bash
  psql "$DATABASE_URL" -c 'CREATE SCHEMA scratch_test'
  TEST_DATABASE_URL="$DATABASE_URL&search_path=scratch_test" go test -p 1 -race ./...
  psql "$DATABASE_URL" -c 'DROP SCHEMA scratch_test CASCADE'
  ```
  Gotcha: Go's pgx accepts `search_path` in the URL, but **`psql` does not**. To inspect a scratch schema with psql use `PGOPTIONS="-c search_path=scratch_test" psql "$DATABASE_URL"` or schema-qualified names.
- **Test servers:** use spare ports and stop them by PID (`kill <pid>`). Never `pkill -f` (it can match your own shell). Avoid `rm` through shell variables.
- **Check the official docs / CLI of a tool before writing custom tooling around it.**
- **Explain for a beginner.** The user is learning: say what was built, why, and the expected results. When asked for a TL;DR, give short bullets.
- **Commit style:** conventional commits, e.g. `feat(api): ...`, `fix(mailer): ...`, `refactor(db): ...`, `docs: ...`. Small, focused commits.

## Commands

```bash
go build ./... && go vet ./... && go test ./...   # fast checks (DB tests are skipped without TEST_DATABASE_URL)
go run ./cmd/server                               # run the API (applies migrations on startup)
```

Frontend (`cd frontend`, Node 22.18+): `npm install`, `npm run dev` (:5173), `npm run lint`, `npm run type-check`, `npx vitest run`, `npm run build`. See [frontend/README.md](frontend/README.md).

Dev services: PostgreSQL on `localhost:5432` (database `sendmail`), Mailpit SMTP on `:1025` with its UI on `:8025`.

## Database migrations

Goose, numbered SQL files in `internal/migrations/sql/`, embedded in the server binary, applied automatically at startup (`AUTO_MIGRATE=true`). Use the **goose CLI** for manual work (setup is in the README). Never edit a migration that has already been applied; add a new one with `goose -s create <name> sql`. A test enforces numbering (no gaps or duplicates) and that every migration has a Down section. See [decision 0001](docs/decisions/0001-use-goose-migrations.md).

## Conventions

- UI copy says **"Bulk Emails"**; the API and database keep the names `batches` and `email_jobs`.
- API errors are JSON: `{"error": "message"}`. Validate input before touching the store.
- Secrets (SMTP password) are never returned by the API or written to logs.

## Known quirks (not ours to fix unless asked)

- `go.sum` is gitignored, so teammates run `go mod download` once after pulling.
- `gofmt -l` flags `cmd/server/main.go` (an old doc comment) and `internal/recipients/recipients_test.go`. Both predate current work; do not reformat unrelated files.
