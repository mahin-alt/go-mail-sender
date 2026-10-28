# Mail Sender dashboard (frontend)

Vue 3 + TypeScript + Vite single-page app for the Go mail-sender API. Design and architecture: [docs/frontend/](../docs/frontend/).

```bash
cd frontend
npm install
cp .env.example .env.local   # VITE_API_BASE, default http://localhost:8080
npm run dev                  # http://localhost:5173 (the API must allow this origin via CORS_ALLOWED_ORIGINS)
```

| Command | What it does |
|---|---|
| `npm run dev` | dev server on :5173 |
| `npm run lint` | oxlint + ESLint (auto-fixes) |
| `npm run type-check` | `vue-tsc` |
| `npm run test:unit` | Vitest (`npx vitest run` for a single pass) |
| `npm run format` | Prettier |
| `npm run build` | type-check + production build into `dist/` |

To use the real API, start the Go server from the repo root (`go run ./cmd/server`, port 8080). The menu footer shows whether it is reachable.

In dev, `/dev/stack-check` shows fonts (English + Bangla), the theme, and the PrimeVue/Tailwind layering.
Dependencies are pinned exactly (`.npmrc` has `save-exact=true`); commit `package-lock.json`.
