---
paths:
  - "frontend/**"
---

# Frontend rules

There is no design system or design doc right now. For any visual decision (style, colour, type, layout, components) use the ui-ux-pro-max skill, explain the options and recommend before building. Do not persist a design system unless the user asks. The chosen style is Bento Box Grid with the paper plane logo: follow [docs/decisions/0007-frontend-visual-style.md](../../docs/decisions/0007-frontend-visual-style.md) (tokens, sizes, components) and the reference page `frontend/design-previews/c-bento.html`.

- Server data goes through TanStack Query via `src/api`; Pinia holds UI state only; components never import axios.
- PrimeVue provides components; Tailwind does layout and spacing.
- Only write types and calls for endpoints that exist in the README API table.
- Every interactive element is keyboard reachable with a visible focus ring; errors that need action are never auto-dismissed.
- UI copy says "Bulk Emails"; the product name shown in the web UI is MailFlow, and the first nav page is "Dashboard" (not "Overview").
- Finish a phase only when `npm run lint`, `npm run type-check`, `npx vitest run` and `npm run build` pass and the page was run against the real API (writes only to an isolated schema); then update the feature doc.
