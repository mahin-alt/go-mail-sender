# Roadmap

> The index of all features. **Keep this file small:** one row per feature, details live in `docs/features/<name>.md`. Edit it only when a feature starts or finishes (keeps parallel branches from conflicting).

Status: `proposed` -> `next` -> `in progress` -> `done` (or `deferred`).

| Feature | Status | Doc | Notes |
|---|---|---|---|
| Dashboard: backend APIs | done | [features/dashboard.md](features/dashboard.md) | batch list, paged jobs, retry, stats, settings, STARTTLS, CORS, goose |
| Dashboard: Vue frontend | in progress (Phases 0-1 done) | [features/dashboard.md](features/dashboard.md), [frontend/](frontend/) | branch `feat/dashboard-frontend`; design approved, PrimeVue 4.5.5; scaffold and app shell done, next is Phase 2 (Bulk Emails list and detail) |
| Failures endpoint + stuck-job recovery | next | (no doc yet) | cross-batch failed list grouped by reason; requeue jobs stuck in `processing`; maybe a `failed_at` column |
| Templates | proposed | (no doc yet) | saved subject/body templates, picked in the New Bulk Email wizard |
| Recipients search | proposed | (no doc yet) | "did merchant X get the notice?" |
| Activity log | proposed | (no doc yet) | audit trail of sends, retries, settings changes |
| Authentication and roles | deferred | [decisions/0002](decisions/0002-no-auth-for-now.md) | needed before the API leaves localhost |

## Branches

`feat/dashboard-api` and `feat/db-migrations` (backend APIs, goose migrations, project docs) were merged into local `main` on 2026-10-28 by fast-forward. Local `main` is ahead of `origin/main`; **push pending** (the user pushes from their own terminal).

Active: `feat/dashboard-frontend` (from `main`).

## Starting a new feature

1. Copy [features/_template.md](features/_template.md) to `features/<name>.md` and fill in goal, scope and first tasks.
2. Add one row above.
3. Branch, then work. Tick checkboxes in the feature doc **in the same commits as the code**.
4. When finished: set the doc's status to `done`, move lasting decisions to `decisions/`, update the README if behaviour changed, update the row above.

Parallel features: one branch, one doc and one session each (a `git worktree` per feature keeps working folders separate). Migrations are numbered, so two branches adding one will collide on the number; whoever merges second renumbers (the migration test catches it).
