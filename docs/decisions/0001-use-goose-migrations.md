# 0001: Use goose migrations (official CLI, pinned install)

**Status:** accepted

## Context
The schema was created by `CREATE TABLE IF NOT EXISTS` at startup plus a hand-kept `schema.sql`. That cannot alter existing tables, tracks no versions, and the two copies can drift.

## Decision
- Numbered SQL migrations in `internal/migrations/sql/`, run by **goose**, embedded in the server binary and applied at startup under a Postgres advisory lock (`AUTO_MIGRATE=true` by default). `AUTO_MIGRATE=false` makes the server refuse to start while migrations are pending, for deploys that migrate as a separate step.
- The first migrations use `IF NOT EXISTS` so databases made by the old code adopt them without changes.
- Manual work uses the **official goose CLI**, installed with `go install github.com/pressly/goose/v3/cmd/goose@v3.29.0` and configured through `GOOSE_*` variables in `.env` (documented in the README). No Makefile, no custom wrapper.

## Consequences
- Schema changes are reviewed as code; every environment converges.
- The goose CLI asks for no confirmation on destructive commands (`reset`, `down-to`), and `up-to` with an unknown number applies everything. The README warns about both; keep shared/production URLs out of the `.env` goose reads.
- Developers must keep their goose version in step with the pinned one.
- Parallel branches adding migrations collide on the number; the migration test catches it and the second merger renumbers.
- A custom `migrate` command was tried first and removed because goose's own CLI already provides everything.
