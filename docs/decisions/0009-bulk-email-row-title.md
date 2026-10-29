# 0009: A Bulk Email row is identified by its filename (for now)

**Date:** 2026-10-29 · **Status:** accepted · **Revisit:** when staff ask to tell uploads apart by subject

## Context
The batch list (`GET /batches`) and batch detail (`GET /batches/{id}`) return the **filename**, not the subject. The subject exists only in the database column `subject_template` (and on each job's rendered `subject`). The first design assumed a row title of "subject, filename underneath".

## Decision (option A)
Show the **filename** as the row title in the list and on the detail page. No backend change.

## Future improvements (not scheduled)
- **Option B: add `subject_template` to the API responses.** Small Go change (store query, handler, test). Then the row could show the subject as the title with the filename underneath. Best when several uploads share a filename or staff think in subjects.
- **Option C: show the subject only on the detail page** (fetch it once for one batch). Smaller change than B, but the list still shows filenames only.
- A user-given **name** per Bulk Email would need a new column and a field in the New Bulk Email wizard.

Tracked as "proposed" in [ROADMAP.md](../ROADMAP.md).
