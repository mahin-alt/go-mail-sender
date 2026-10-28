# Frontend design system

> Tokens and component conventions. Small on purpose: only what the pages in [screens-and-flows.md](screens-and-flows.md) need. Status: **approved** (2026-10-28).

## Who owns what

| Concern | Owner |
|---|---|
| Look of components (buttons, inputs, tables, dialogs): colours, radii, focus rings | **PrimeVue theme** (design tokens) |
| Page layout, spacing between things, responsive behaviour, one-off alignment | **Tailwind utilities** |
| Brand/status colours | **One source:** the PrimeVue preset. Tailwind reads them through `tailwindcss-primeui` (`bg-primary`, `text-muted-color`, ...). Do not define a second palette in Tailwind |

Rule of thumb: if it is the inside of a component, change the PrimeVue token; if it is the space around components, use Tailwind. Avoid overriding PrimeVue internals with Tailwind classes.

## Typography

- **Latin:** IBM Plex Sans Variable (chosen by the user on 2026-10-28, replacing Inter). **Bangla:** Noto Sans Bengali Variable. Both **self-hosted** from npm (`@fontsource-variable/ibm-plex-sans`, `@fontsource-variable/noto-sans-bengali`), no Google Fonts CDN (works on an internal network, no third-party request, versions pinned).
- Stack: `"IBM Plex Sans Variable", "Noto Sans Bengali Variable", system-ui, sans-serif`. The browser uses Plex for Latin (Plex has no Bengali glyphs) and falls through to Noto for Bengali characters.
- Load only weights used (variable fonts: one file each). Use `font-display: swap`. Confirm the Bengali file is split by `unicode-range`, so Latin-only pages do not download it.
- Sizes (px / line-height): body 14/20, table cell 13/18, small/help 12/16, page title 20/28 semibold, section title 16/24 semibold. No heading larger than 20px.
- Numbers in tables and counts: `font-variant-numeric: tabular-nums`, right-aligned.
- Bangla text needs more line height than Latin; where user data may be Bangla (names, email previews) use line-height at least 1.6.
- Email bodies and previews: monospace-free plain text, `white-space: pre-wrap` (messages are plain text).

## Colour

Light theme only for the MVP. PrimeVue preset (`src/theme/preset.ts`): Aura extended with `definePreset`, `primary` = blue palette with the action colour at **blue-600** (white text on blue-500 is only 3.7:1), `surface` = slate palette. Dark mode off (`darkModeSelector: false`).

Semantic status colours (text on tinted background; every pair must reach **4.5:1** at 12-13px, to be verified with a contrast checker at scaffold time):

| Meaning | Where | Hue | Icon | Label |
|---|---|---|---|---|
| Sent / success | job `sent`, completed with no failures | green | check-circle | Sent |
| Failed / error | job `failed`, send errors | red | x-circle | Failed |
| Invalid (never sent) | job `invalid` | amber | alert-triangle | Skipped (invalid) |
| Pending / retrying | `pending`; show "Retry 2 of 4" when `attempts > 0` | slate | clock | Pending / Retrying |
| Processing | `processing`, batch in progress | blue | loader (spins only while shown) | Sending |
| Warning (needs attention) | Overview list, settings needing restart | amber | alert-triangle | text states why |

Invalid is amber, not red, on purpose: "failed" means the system tried and could not deliver (act: retry or fix SMTP); "invalid" means the row was bad (act: fix the file). Different actions, different colours.

Surfaces: page background `surface-50`, panels `surface-0` with a 1px `surface-200` border, text `surface-900`, secondary text `surface-600`.

## Spacing, size, shape

- 4px base unit; use Tailwind's default scale (`p-2`=8, `p-4`=16, `gap-6`=24). No arbitrary values (`p-[13px]`).
- Page padding 24px; space between sections 24px; between related controls 8-12px.
- Control height 32px (compact) for table toolbars and filters, 36px for form fields. Interactive targets at least 24x24px (WCAG 2.2 criterion 2.5.8); aim for 32px.
- Table: PrimeVue `size="small"` (about 36px rows). Zebra striping off; use row hover and a 1px divider.
- Radius: 6px controls and badges, 8px panels and dialogs. Nothing fully rounded except the avatar-less status dot.
- Shadow: none on panels. A single soft shadow on floating layers (menus, dialogs, toasts).

## Component conventions

**Status badge** (`StatusBadge`): icon + text label, never colour alone; one component maps status string to hue/icon/label so every table agrees. Non-interactive. Screen readers read the label.

**Tables** (PrimeVue DataTable, lazy/server-side paging for jobs and batches):
- Fixed toolbar above: search, status filter, row count ("Showing 21-40 of 1,000"). Filter state lives in the URL query string so it survives refresh and can be shared.
- Sticky header, compact rows, ellipsis with a tooltip for long values, `last_error` expands in a row detail rather than truncating silently.
- Numeric columns right-aligned. Primary column (filename, email) left.
- Empty state says why and what to do ("No failed emails in this bulk email").
- Row actions are text buttons or an icon button with an `aria-label` and a tooltip. Destructive or bulk actions are never hidden in a row menu.

**Forms:** labels above fields, help text below, validation message under the field in red with an icon, shown on blur and on submit. A form-level error summary appears at the top of the form after a failed submit and takes focus. Required fields marked in the label text.

**File upload:** a single drop zone (PrimeVue FileUpload in custom mode), accepts `.xlsx` only, shows file name and size, upload progress through axios, and a clear "Replace file". Server errors are shown inline in the zone, not as a toast.

**Dialogs:** only for confirmations that need a decision (send, retry all, reset a setting). Title states the action; body states the effect with numbers; primary button repeats the verb ("Send 38 emails"), never "OK". Focus lands on the safe button (Cancel).

**Notifications (toast):** short success confirmations only ("Test email sent to ..."), auto-dismiss after 5 seconds, bottom-right, `aria-live=polite`. Errors that need action use an inline message or page banner and do not auto-dismiss.

**Loading:** skeletons for tables and tiles on first load; spinner on buttons for in-flight actions (button disabled while pending); background refetches show no spinner (a small "Updated 3s ago" text is enough).

**Progress:** PrimeVue ProgressBar with text "380 of 1,000 sent" next to it; the bar alone carries no meaning. Announce completion once with `aria-live`.

## Accessibility baseline (WCAG 2.2 AA)

- Everything reachable and operable by keyboard; visible focus ring (PrimeVue default, never removed); logical tab order.
- Colour never the only signal. Contrast 4.5:1 for text, 3:1 for UI boundaries and icons.
- Landmarks: `nav`, `main`, one `h1` per page. Tables have proper headers.
- Form fields have associated labels; errors linked with `aria-describedby`.
- Live regions for progress and toast messages; do not move focus on background updates.
- Respect `prefers-reduced-motion` (stop spinners' extra motion, no slide transitions).
- Test: keyboard-only pass of each flow, axe checks in component tests where cheap.

## Responsive

Desktop-first (staff use laptops). Breakpoints: 1280+ full layout; 1024-1279 sidebar collapses to icons; below 1024 sidebar becomes a drawer and tables scroll horizontally inside their panel. The wizard and Settings remain usable on a tablet; phone is not a target for the MVP.
