# MailFlow (frontend)

Vue 3 + TypeScript + Vite single-page app for the Go mail-sender API.

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

Phase 1 is built: the MailFlow shell (sidebar, header, routes), the API client and `StatusBadge`. Pages are placeholders until their phase. Look and tokens: [decision 0007](../docs/decisions/0007-frontend-visual-style.md); the static reference page is `design-previews/c-bento.html`.
Dependencies are pinned exactly (`.npmrc` has `save-exact=true`); commit `package-lock.json`.
