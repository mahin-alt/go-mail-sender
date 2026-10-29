# 0007: Frontend visual style is Bento Box Grid, logo is the paper plane

**Date:** 2026-10-29 · **Status:** accepted · **Applies to:** `frontend/`

## Decision
- The MailFlow web UI uses the **Bento Box Grid** style: a warm neutral page, big rounded white tiles of different sizes, very large numerals, one black accent tile, a black primary button, and almost no borders.
- **Logo: paper plane + wordmark** (option 2 of 5). Wordmark: **Mail** bold, **Flow** light.
- **Sidebar:** icon + label for every item, no menu groups. **Open by default** (pinned). A button at the bottom of the sidebar ("Collapse sidebar", double chevron) collapses it to a 70px icon rail. While collapsed it is **hoverable**: pointing at it opens it (labels appear), moving the pointer away (or the keyboard focus leaving) collapses it again. The same button then reads "Keep sidebar open"; clicking it pins the sidebar open again. Right after clicking Collapse the sidebar stays collapsed until the pointer leaves and comes back. In every state the page content **moves** with the sidebar (pushed right when it opens, back left when it closes); the sidebar never covers content. Keyboard focus (focus-visible) also opens it. The choice should be remembered per user (localStorage).
- **Font: Plus Jakarta Sans** (Bangla falls back to Noto Sans Bengali). Compared against System, Inter, IBM Plex Sans, DM Sans and Manrope in the preview. Install it as a package (`@fontsource-variable/plus-jakarta-sans`), not from Google Fonts at runtime.
- First nav item is **Dashboard** (not "Overview"). Nav order: Dashboard, Bulk Emails, Single Email, Verify Emails, Failures, Settings.
- No persisted design-system folder (the user deleted the earlier one). **This file plus the preview files are the reference.**

## Why
Four static previews were compared (A Soft UI, B Data-Dense, C Bento, D Swiss) and the user picked C for its look. The icon-only sidebar of the first C preview looked like a collapsed sidebar, so labels were added; the first logo looked like a sidebar button, so five logos were drawn and the paper plane chosen.

## Reference files (mock data, plain HTML/CSS, no build)
- `frontend/design-previews/c-bento.html`: the Dashboard page, with the sidebar modes and logo switcher. This is the visual source of truth until the Vue version exists.
- `frontend/design-previews/c-logos.html`: the five logo options side by side.
- Open them with `cd frontend/design-previews && python3 -m http.server 8765`, then `http://localhost:8765/c-bento.html`.

## What it takes to recreate it in Vue (PrimeVue + Tailwind v4)
**Colours** (define once as theme tokens; never hard-code in components)

| Token | Value | Used for |
|---|---|---|
| page background | `#EFEEEA` | whole page |
| tile | `#FFFFFF` | tiles, sidebar |
| text | `#161616` | body text, primary button, active nav, first KPI tile bg |
| muted text | `#77756F` | labels, captions |
| track | `#E4E2DC` | progress bar track |
| hairline | `#EFEEEA` | table row dividers |
| nav hover / badge bg / footer note | `#F1F0EC` / `#F3F2EE` / `#F5F4F0` | |
| success / danger / warning | `#1F9D63` / `#E5484D` / `#C98A0B` | status text, chart, failed number |
| attention tile | `#FFE9C7`, text `#7A5A1E` | "Needs attention" tile |
| brand orange | `#FF6B2C` | logo only |

Status is always **glyph + text + colour**, in a pill: `✓ Completed`, `↻ Sending`, `○ Queued`, `✕ Failed`, `! Partly failed` (pill bg `#F3F2EE`, 12.5px, weight 600, padding 5px 12px).

**Shape and spacing:** tile radius 24px, nav item and small inner radius 16px, buttons and pills fully round. Tile padding 24px. Grid gap 14px. **No shadows** except the active nav item and nothing else. No gradients, no borders on tiles.

**Layout:** 12-column grid. KPI tiles span 3 each (min height 170px). Chart tile spans 8, "Needs attention" spans 4, the table tile spans 12. Content max width 1280px, page padding 30px 34px. Under 820px everything stacks to a single column and the sidebar becomes a top bar.

**Type:** page title 40px / 700 / letter-spacing -0.02em. KPI number 54px / 700 / -0.03em. Tile title 14px / 600 muted. Body and table 14px. Captions 12 to 12.5px. The preview used the system font; the real font is **not chosen yet** (Inter or similar, Bangla needs Noto Sans Bengali).

**Components to build**
- `AppSidebar` (two states, `open` and `rail`, plus a hover/focus expansion that only exists in `rail`; use a ref and a CSS variable for the width, not CSS `:has()`, in the Vue version): fixed at left 14px, top 14px, bottom 14px, white, radius 24px, padding 18px 10px. Width **70px collapsed, 234px open**. Page left padding **98px collapsed, 262px open**, both animated together with `.22s ease` so the content slides. Items: height 48px, padding 0 15px, gap 14px, icon 20px, text 15px / 500, radius 16px; active = black background + white text; hover = `#F1F0EC`. Labels fade out when collapsed. Also expands on keyboard focus (focus-visible). The toggle button is a full-width row at the bottom: 48px, radius 16px, background `#F5F4F0`, `aria-expanded` set. The brand row is 40px tall with a 32px mark and 11px gap.
- `KpiTile`: small label, huge number, one-line caption. The first tile is black with white text; the "Failed" number is red.
- `StatusBadge`, a thin `ProgressBar` (6px, black fill on the track colour), the stacked sent/failed bar chart (rounded 6px bars), and a plain table inside a tile (no zebra, hairline row dividers).
- Icons: one set (Lucide), 20px, stroke 2.

**Logo (paper plane), 32×32 viewBox:**
```html
<svg width="32" height="32" viewBox="0 0 32 32" fill="none" aria-hidden="true">
  <g stroke="#FF6B2C" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round">
    <path d="M28 4 4 13l9 4 4 9z"/><path d="M28 4 13 17"/>
  </g>
</svg>
```
Wordmark: 20px, letter-spacing -0.02em, `Mail` weight 800 + `Flow` weight 300, colour `#161616`. Collapsed sidebar shows only the plane.

## Data shown must exist in the backend
A Bulk Email row shows its **subject** (`subject_template`) as the title and the **filename** underneath. There is no "name" and no Merchant/Customer type in the database, so the UI does not show them (the recipient-type question stays open in the feature doc). Batch statuses are `pending`, `processing`, `completed`; "Completed with failures" is derived from the job counts. Job statuses are `pending`, `processing`, `sent`, `failed`, `invalid`.

## Still open
Type hierarchy of tile titles vs table column headers (the preview has toggles: tile titles 14 muted / 16 / 18 / 20 bold dark, column headers 12 muted / 11 caps / 12 lighter). KPI labels are a separate role and are not changed with them.
Dark mode (none for the MVP), and the other fixes the user wants to discuss on the preview.
