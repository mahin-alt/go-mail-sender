# Frontend implementation notes

> Architecture and boundaries for `frontend/`. The stack choice itself is in [decision 0004](../decisions/0004-frontend-stack.md) and the PrimeVue version question in [decision 0006](../decisions/0006-primevue-version-and-license.md). Visual rules are in [design-system.md](design-system.md). Status: **approved** (2026-10-28).

## Who owns what state

| Kind of state | Where | Example |
|---|---|---|
| Server data (anything the API returns) | **TanStack Query** | batches, jobs, stats, settings |
| UI state shared across pages | **Pinia** | sidebar collapsed, the `File` carried from Verify to the wizard, wizard draft (subject/body) |
| State of one page, in the URL | **Vue Router query** | table filters, page number, selected tab |
| State of one component | local `ref` | open/closed dialog, input draft |

Never copy API responses into Pinia. If two components need the same server data, both call the same query composable; Query dedupes and caches.

## Layout of `frontend/` (created at scaffold)

```
frontend/
  src/
    api/          axios instance, one file per endpoint group, typed functions
    types/        hand-written API types (no OpenAPI spec exists)
    queries/      TanStack composables: useBatches, useBatchJobs, useStats ...
    stores/       Pinia stores (UI only)
    router/
    views/        one per route
    components/   shared: StatusBadge, ProblemsTable, ProgressBlock, PageHeader
    styles/       main.css (Tailwind + PrimeUI imports, font imports), theme preset
  .env.example    VITE_API_BASE
```

## API client

- One axios instance; `baseURL` from `VITE_API_BASE`. Development calls the Go API **directly** and relies on CORS ([decision 0003](../decisions/0003-separate-frontend-deploy.md)); the dev origin `http://localhost:5173` must be in `CORS_ALLOWED_ORIGINS` (it is the default).
- An interceptor turns `{"error": "..."}` into a typed `ApiError` so screens show the server's message.
- Types mirror the Go JSON tags (`internal/store`, `internal/api`). A type is only written for an endpoint that exists.
- Query keys are arrays in one file (`['batches', filters]`, `['batch', id]`, `['batch-jobs', id, filters]`, `['stats', tz]`, `['settings']`). Mutations invalidate by key (a retry invalidates the batch and its jobs).
- Polling uses `refetchInterval` as a function that returns `false` once the batch is `completed`. Default `refetchIntervalInBackground: false` so hidden tabs do not poll.
- Retries: queries retry network errors twice, never `4xx`. Mutations never auto-retry (a send must not repeat).

## Styling setup (to verify against the docs at scaffold time)

- Tailwind CSS v4 through the `@tailwindcss/vite` plugin (no `tailwind.config.js` for v4; configuration is in CSS).
- `tailwindcss-primeui` imported in the same CSS file as `tailwindcss` (its CSS edition is for Tailwind v4).
- PrimeVue configured with Aura via `definePreset`, `cssLayer` enabled with the layer order `theme, base, primevue` so Tailwind utilities can override PrimeVue without `!important`, and `darkModeSelector: false`. The official page describes this in prose only; copy the exact option names from the installed version's docs, then check one overridden component in the browser.
- PrimeVue components are imported per component (tree-shaking), not registered globally.

## Boundaries (what each library must not do)

- **PrimeVue** supplies interactive components; do not rebuild a DataTable, Dialog or FileUpload by hand.
- **Tailwind** does layout and spacing; it does not recolour components (change the preset instead).
- **Pinia** never holds API data.
- **TanStack Query** is the only place that calls the API (through `src/api`). Components do not import axios.
- **Icons** (Lucide) only where they carry meaning, see the design brief.

## Quality gates (per phase)

`npm run lint`, `npm run type-check`, `npx vitest run` and `npm run build` pass; the page works against the real Go API and Mailpit; keyboard-only pass on new flows; the feature doc checklist and "Where we left off" updated in the same commit.

## Version snapshot (npm, 2026-10-28; pin exact versions at scaffold)

| Package | Latest | Note |
|---|---|---|
| vue | 3.5.44 | |
| vite | 8.3.5 | |
| tailwindcss, @tailwindcss/vite | 4.3.3 | |
| tailwindcss-primeui | 0.6.1 | |
| primevue | **5.0.4 (latest, needs a license key)** / 4.5.5 (`v4-stable`, MIT) | **chosen: 4.5.5** (decision 0006) |
| @primeuix/themes | 3.1.0 (pairs with PrimeVue 5) / 2.0.3 (v4 line, MIT) | use the line that matches the PrimeVue major |
| @tanstack/vue-query | 5.105.0 | |
| pinia | 4.1.0 | decision 0004 was written assuming an older major; read its release notes first |
| vue-router | 5.4.0 | decision 0004 said "4"; read the migration notes first |
| chart.js / vue-chartjs | 4.6.1 / 5.3.4 | |
| @fontsource-variable/ibm-plex-sans, /noto-sans-bengali | 5.3.0 | |
| @lucide/vue | 1.63.0 | `lucide-vue-next` is deprecated in favour of this package |

If a major version turns out to need code changes we did not plan for, record it in the feature doc and pick the older supported major rather than absorbing churn mid-build.

## Phase 0 results (2026-10-28)

- Scaffolded with the official `create-vue@3.24.0` (`--ts --router --pinia --vitest --eslint --prettier --bare`), then added the rest. It brings **oxlint + ESLint** together (`npm run lint` runs both) and `vue-tsc` for `npm run type-check`; tests run with `npx vitest run`.
- Installed exactly: primevue 4.5.5, @primeuix/themes 2.0.3, tailwindcss + @tailwindcss/vite 4.3.3, tailwindcss-primeui 0.6.1, @tanstack/vue-query 5.105.0, axios 1.21.0, chart.js 4.6.1, vue-chartjs 5.3.4, @lucide/vue 1.63.0, the Fontsource fonts (IBM Plex Sans Variable and Noto Sans Bengali Variable) 5.3.0. The scaffold's own packages (vue, vue-router 5, pinia 4, vite 8, vitest 4, TypeScript 6) keep the generator's ranges; `package-lock.json` pins what is installed. Needs Node `^22.18` or `>=24.12`.
- **Layering confirmed in a browser.** `src/styles/main.css` declares `@layer theme, base, primevue, components, utilities;` first, and `src/main.ts` passes the same order to PrimeVue's `cssLayer`. A `bg-red-600` utility on a PrimeVue Button overrode it with no `!important`.
- **Fonts confirmed.** Bangla renders with Noto Sans Bengali; the Bengali subset downloads only when Bengali characters are on the page (Fontsource splits by `unicode-range`), and Latin stays on the Latin font (Inter at the time of the check; swapped to IBM Plex Sans afterwards, see below).
- **Dark mode off confirmed:** with the OS in dark mode the app stays light.
- **Contrast fix:** Aura's default primary button (white on blue-500) measured 3.68:1, below the 4.5:1 target. The preset now uses blue-600 (5.17:1). The status tags measured 4.52-6.92:1 (the amber "Skipped" tag is the tightest at 4.52, keep an eye on it).
- `npm audit`: 2 moderate findings, both in `vitest`'s `@vitest/mocker`, dev-only (0 in production dependencies). Revisit when a patched Vitest is out.
- `vue-demi` (pulled in by TanStack Query) has an install script npm reports as not yet allowed; the app works without it since the library selects Vue 3 by default.
- `/dev/stack-check` (dev server only) is the verification page for this phase; delete it when it stops being useful.
- **Font changed to IBM Plex Sans (user's choice, 2026-10-28).** Replaces Inter; Bangla still falls through to Noto Sans Bengali. Re-checked in the browser after the swap.
