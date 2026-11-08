# Feature: Cancel a Bulk Email

**Status:** done
**Branch:** `feat/cancel-batch` (based on: `main` at `26f1dff`)
**Owner / session notes:** asked for after a 1M-row batch sent from Postman could not be stopped (Ctrl+C only paused it; the queue resumed on restart).

## Where we left off
> Update this at the end of every chunk of work, in the same commit. A new session reads this first.
- **Last done:** `cancelled` status for jobs and batches, `POST /api/v1/batches/{id}/cancel`, Cancel button with confirmation on the batch page, Cancelled tab and tile, migration 00004 (status CHECK constraints, `batches.cancelled_at`). Go and frontend checks pass; a real 1M batch on a scratch schema was cancelled from the UI and stopped sending.
- **Next:** nothing for this feature. Possible later: Pause / Resume.
- **Blocked on / waiting for the user:** nothing. Committed on `feat/cancel-batch` and merged into `main` on 2026-11-08.
- **Updated:** 2026-11-08

## Why
There was no way to stop a batch: stopping the server only paused it, because the queue lives in the database.

## Scope
**In:** cancel a pending or processing batch from the API and the UI; cancelled emails are never sent or retried.
**Out (not now):** Pause / Resume (moving cancelled jobs back to pending), cancelling a single email, who cancelled (needs login).

## Design and decisions
- **Decision:** a new status value `cancelled` for jobs **and** batches (the user chose a separate batch status over keeping `completed`). **Because:** a deliberate stop must be told apart from a finished batch; reusing `failed` would let Retry resend everything, reusing `invalid` would mislabel good addresses.
- **Decision:** migration 00004 adds CHECK constraints listing every allowed status. **Because:** the columns were free text; a typo such as `canceled` would have been accepted silently. Down turns cancelled jobs into `invalid` (never sent), not `failed` (which Retry would resend).
- **Decision:** `batches.cancelled_at` records when; `completed_at` stays empty for cancelled batches.
- **Decision:** an email a worker is sending at that moment is not interrupted; it finishes as `sent`, or, if it fails, becomes `cancelled` instead of going back to the queue (MarkJobFailed checks the batch). **Because:** an SMTP conversation cannot be safely cut half way, and a failed in-flight email must not be requeued after a cancel.
- **Decision:** cancel and retry lock the batch row (`SELECT ... FOR UPDATE`) so they cannot interleave; retry on a cancelled batch is `409`. `checkBatchCompletion` only moves `pending`/`processing` batches to `completed`, so a late finishing email cannot overwrite `cancelled`.
- **Decision:** the confirmation says "Stop this Bulk Email?" with "Keep sending" / "Stop sending" (red), because a dialog whose dismiss button is labelled "Cancel" would be ambiguous here. `ConfirmDialog` gained `dismissLabel` and `danger`.

## Tasks
- [x] Migration 00004: status CHECKs, `batches.cancelled_at`
- [x] Store: `CancelBatch`, cancel-aware `MarkJobFailed`, completion check, retry refuses cancelled, `cancelled` counts and `cancelled_at` in progress/list
- [x] API: `POST /api/v1/batches/{id}/cancel` (200 / 404 / 409), retry `409` on cancelled, `batch cancelled` INFO log
- [x] Frontend: types, `Cancelled` badge, Cancel button + dialog, toast, Cancelled tile and job tab, Cancelled tab on the list
- [x] Tests: store (cancel, in-flight send and failure, second cancel, retry refused, unknown batch, finished batch, CHECK constraint), API no-store, UI (button only while sending, confirm flow, cancelled view, list tabs)
- [x] README API table, ROADMAP row

## Verification
- `go build ./... && go vet ./... && go test ./...`; DB tests with `-race` in a scratch schema (includes the migration Up/Down/Up test).
- `cd frontend && npm run lint && npm run type-check && npx vitest run && npm run build`.
- Real run (`api-scratch` + `frontend-scratch`, schema `scratch_ui_test`): queued the 1M-row sample with curl (780,493 pending), opened the batch, pressed Cancel -> Stop sending. Result: `cancelled` with 763,598 cancelled and 16,895 sent; sent did not change 5 s later; second cancel and retry both `409`; the list's Cancelled tab shows it. The cancel request took 3.6 s for 763k rows.

## Notes / history
- 2026-11-04: the Pending/Cancelled tiles clip six-digit numbers at narrow widths (e.g. "773,92…"). Existing layout limit, not changed here.
