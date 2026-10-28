# 0004: Frontend stack

**Status:** accepted

## Decision
Vue 3 single-page app (Composition API, `<script setup>`):

| Concern | Choice |
|---|---|
| Language / build | TypeScript, Vite |
| Routing | Vue Router 4 |
| Client (UI) state | Pinia |
| Server state, caching, polling | TanStack Query (vue-query) |
| HTTP | axios (also gives upload progress) |
| UI components | PrimeVue (DataTable, Stepper, FileUpload, Toast); **version and license: [0006](0006-primevue-version-and-license.md)** |
| Utility CSS | Tailwind CSS v4 + `tailwindcss-primeui` (added 2026-10-28) |
| Package manager | npm |
| Charts | vue-chartjs |
| Icons | Lucide |
| Lint / format | ESLint + Prettier |
| Tests | Vitest + Vue Test Utils |

## Why
- A table-heavy dashboard with a wizard and charts: PrimeVue ships the DataTable, stepper and file upload, so far less assembly than headless libraries.
- Pinia is for UI state only (filters, preferences); API data belongs in TanStack Query, which handles caching, retries and `refetchInterval` polling for batch progress.
- TypeScript because the API response shapes are known and stable (`Job`, `Batch`, stats, settings).

## Design docs
Visual direction, tokens, screens and the boundaries between these libraries: [docs/frontend/](../frontend/). Version check on 2026-10-28 found newer majors of Vue Router (5) and Pinia (4) than assumed here; pin versions at scaffold after reading their migration notes.

## Consequences
- API types are hand-written (the backend has no OpenAPI spec yet).
- Progress is polling first; SSE can come later.
- Jobs and parse-problem lists need lazy/virtualized tables (files can have 1,000+ rows).
