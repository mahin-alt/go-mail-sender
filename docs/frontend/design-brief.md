# Frontend design brief

> Why the UI looks and behaves the way it does. Product scope lives in [PROJECT.md](../PROJECT.md) and [features/dashboard.md](../features/dashboard.md); this file only adds the design direction. Status: **approved** (2026-10-28).

## Who uses it and what they need

Operations and support staff, not technical, who send order and merchant/customer notices from spreadsheets. Their questions, in order of frequency:

1. "Did my send finish, and did everything go out?" (progress, counts)
2. "Which rows are wrong and why, before I send?" (data-quality report)
3. "Something failed. What was the error, and can I retry?" (failure inspection)
4. "Is the system healthy right now?" (queue, today's numbers)

The risk that shapes the design is **sending a wrong email to many real people**. Confirmations, previews and clear counts matter more than visual polish.

## Visual direction: calm operations console

- **Dense but readable.** Tables are the main surface. Compact rows, left-aligned text, numbers right-aligned in a tabular figure style, generous whitespace only between sections.
- **Neutral surfaces, one accent.** Grey-blue neutrals, a single blue primary for actions. Colour is reserved for **status**, so a coloured thing always means "look at this".
- **Flat.** Borders instead of shadows. Shadows only on things that float (menus, dialogs, toasts).
- **Plain language.** "Sent", "Failed", "Skipped (invalid)", never internal terms. UI says Bulk Emails, API says batches ([decision 0005](../decisions/0005-ui-naming.md)).
- **Light theme only for the MVP.** Tokens are structured so dark can be added later without rewrites. Dark mode is not free (every status colour pair needs re-checking), and no user asked for it.

## Why these choices suit this product

| Choice | Reason |
|---|---|
| Dense tables, small row height | 1,000-row files are normal; staff scan for the odd failure |
| Status = colour **and** icon **and** text | Operators must not miss a failure; colour alone fails for colour-blind users (WCAG 1.4.1) |
| Confirm dialog showing exact counts before send | Sending is irreversible and goes to real recipients |
| Errors persist until dismissed or fixed | A failed send reason must not vanish after 4 seconds |
| No marketing visuals (hero banners, gradients, campaign "performance" charts) | Not a marketing tool; see non-goals in PROJECT.md |

## Evidence consulted

- PrimeVue theming and Tailwind integration: official PrimeVue docs ([Tailwind](https://primevue.dev/tailwind), [styled mode](https://primevue.dev/theming/styled)). Findings: the `tailwindcss-primeui` CSS plugin targets Tailwind v4; `cssLayer` lets utilities override component styles without `!important`; PrimeVue's default dark-mode selector is `system`, so it must be turned off explicitly for a light-only app.
- Bangla fonts: Fontsource pages for [Noto Sans Bengali](https://fontsource.org/fonts/noto-sans-bengali/about) (variable font, weights 100-900, covers Bengali plus Basic Latin) and [Hind Siliguri](https://fontsource.org/fonts/hind-siliguri/about) (includes Latin). One third-party report lists a malformed Bengali digit one in Hind Siliguri's regular weights, so it is not the default choice.
- Status badges and tables: public design-system guidance (e.g. a council dashboard failing WCAG 1.4.1 for colour-only RAG status; badge specs recommending text labels and a 24px target minimum for interactive controls under WCAG 2.2 criterion 2.5.8). These were secondary sources; check [WCAG 2.2](https://www.w3.org/TR/WCAG22/) itself when in doubt.
- Limits of this research: no authoritative source was found for pairing Inter with a Bangla font, or for real transactional-email tools' failure screens. The type pairing below is a reasoned default that **must be checked visually** with real Bangla text during Phase 0.

## Anti-patterns (do not build these)

- Gradients, glassmorphism, decorative illustrations, oversized hero headings.
- A card around everything; a grid of identical KPI tiles with no action attached. Overview tiles must link to the thing they count.
- Charts without a decision behind them. The one chart (sent vs failed per day) exists to spot a bad day.
- Disabled "coming soon" menu items. A menu entry appears only when its page works.
- Toasts for errors that need action; auto-dismissing anything the user must read.
- Animations other than progress indication and standard overlay transitions; honour `prefers-reduced-motion`.
- Icons used as decoration. An icon is present only when it carries meaning (status) or labels an action.
- Hard-coded colours or pixel values outside the tokens in [design-system.md](design-system.md).

## Assumptions to confirm

See "Open questions" in [screens-and-flows.md](screens-and-flows.md).
