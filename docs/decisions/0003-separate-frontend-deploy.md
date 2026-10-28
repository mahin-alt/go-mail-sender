# 0003: Frontend deployed separately; monorepo with `frontend/`

**Status:** accepted

## Context
The Vue dashboard could be embedded in the Go binary (one deployable, no CORS) or deployed on its own.

## Decision
- Deploy the frontend **separately**. The Go API therefore sends CORS headers for origins listed in `CORS_ALLOWED_ORIGINS` (default `http://localhost:5173`).
- Keep one repository: add `frontend/` next to the existing Go code. Do **not** move the Go code into `backend/` (it would churn `go.mod`, imports, README and CI for no functional gain).
- The frontend reads the API address from `VITE_API_BASE`; in development Vite proxies to `:8080`.

## Consequences
- Two things to deploy and version together; the README API table is the contract between them.
- No SPA-fallback code in the Go server.
