# 0013: Four-table data model with background import

**Date:** 2026-11-09 · **Status:** accepted, built 2026-11-09 · **Replaces:** the `uploads` / `upload_rows` + `batches` / `email_jobs` model from the large-upload Phase 1

## Decision
Data lives in four tables:

- `email_batches`: one row per uploaded file, of type `verification` or `bulk_send`, with its status, counters, subject/body templates and job-generation progress.
- `email_recipients`: one row per spreadsheet row, valid, unverified or invalid. **This is the source of truth for both Verify Emails and New Bulk Email**; a verified batch can be sent later without uploading again.
- `email_import_jobs`: one background import task per batch, with a checkpoint and a lease so an interrupted import resumes.
- `email_jobs`: one row per email to deliver, created only after the user clicks Send. It points at a recipient and holds no copy of the name, address, subject or body; the email is built at send time.

The upload request saves the file to disk and answers `202` straight away; workers in the same Go process do the import, the job generation and the sending. The API is `/verify` and `/batches`.

Details, schema and the task list: [features/four-table-overhaul.md](../features/four-table-overhaul.md).

## Options considered
| Option | Verdict |
|---|---|
| Keep Phase 1 (`uploads` + separate `batches`/`email_jobs` with copied rows) | Not chosen: two copies of every row, the upload waits for parsing, sending duplicates subject/body per job. |
| Four tables, generation state on `email_batches` | **Chosen.** One copy of each row, recoverable import and generation, small summary reads. |
| Four tables plus a `email_job_generation_jobs` table | Not now: one generation per batch does not need it. |

## Consequences
- The migration drops the old tables (the user backs up first); `app_settings` is kept.
- Uploaded files are kept on disk permanently for now; making them temporary later only changes the import worker.
- Uploads are permanent, so a "delete batch" feature and a separate worker process are on the [ROADMAP](../ROADMAP.md).
- UI copy still says "Bulk Emails"; the API says `batches`, the database `email_batches` and `email_jobs` (update CLAUDE.md conventions when built).
