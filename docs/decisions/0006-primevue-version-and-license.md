# 0006: PrimeVue version and license (and adding Tailwind)

**Status:** accepted (2026-10-28): option A, PrimeVue 4.5.5

## Context
Decision 0004 chose PrimeVue. While preparing the design, the current release turned out to be **PrimeVue 5.x**, which is **not MIT**: it ships under the "PrimeUI License", needs an offline-verified license key, and may show a license notice without one. A free Community tier exists with eligibility limits (for 5.0.0 the file lists annual revenue under $1M, fewer than 5 developers, fewer than 10 employees and limited outside funding; later copies of the license add individuals, students, non-profits and non-commercial open source). We did not read the official terms page, only the license text shipped in the packages and secondary sources, and the wording differs between 5.0.x copies. PrimeVue **4.5.5** (npm tag `v4-stable`) is **MIT** and needs no key.

The user also chose **Tailwind CSS** alongside PrimeVue (the earlier recommendation was PrimeVue alone). PrimeVue supports this with the official `tailwindcss-primeui` plugin; its CSS edition works with Tailwind v4.

## Options
- **A. PrimeVue 4.5.5 (MIT).** No key, no eligibility question, no renewal. Stable and still tagged as the v4 line. Cost: it will eventually need a migration to v5 (or a different library) and may get fewer new features.
- **B. PrimeVue 5.x with a Community license.** Newest, but only if the company is eligible, and it adds a yearly eligibility confirmation and a key to manage in every environment. If the company later grows past the limits it becomes a paid license.
- **C. A different component library** (e.g. a headless one). Avoids the issue but means building a DataTable, stepper and upload ourselves; rejected in decision 0004 for that reason.

## Recommendation
**A (PrimeVue 4.5.5 + `@primeuix/themes` 2.x)**, for a small internal tool: the licence risk and key handling are avoided and nothing in the MVP needs v5. Revisit when v4 is no longer maintained. If the user prefers B, someone with authority must confirm the company qualifies under the official terms first.

## Decision
**PrimeVue 4.5.5 with `@primeuix/themes` 2.x (both MIT), pinned exactly.** Tailwind (v4 + `tailwindcss-primeui`) is accepted regardless of the PrimeVue version.

## Consequences
- Tailwind v4 is configured in CSS with the Vite plugin.
- Whichever PrimeVue major is chosen, the matching `@primeuix/themes` major must be installed.
