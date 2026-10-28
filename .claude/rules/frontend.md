---
paths:
  - "frontend/**"
---

# Frontend rules

Before changing anything under `frontend/`, read `docs/frontend/implementation-notes.md` (boundaries) and `docs/frontend/design-system.md` (tokens and components). Pages and states are in `docs/frontend/screens-and-flows.md`.

- Server data goes through TanStack Query via `src/api`; Pinia holds UI state only; components never import axios.
- PrimeVue provides components; Tailwind does layout and spacing. Do not recolour components with Tailwind classes; change the theme preset.
- No hard-coded colours or arbitrary pixel values; use the tokens. Status is always icon + text + colour via `StatusBadge`.
- Only write types and calls for endpoints that exist in the README API table.
- Every interactive element is keyboard reachable with a visible focus ring; errors that need action are never auto-dismissed.
- Finish a phase only when `npm run lint`, `typecheck` and `test` pass and the page was run against the real API; then update the feature doc.
