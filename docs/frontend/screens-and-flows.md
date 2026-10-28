# Screens and flows

> Page hierarchy, interactions and states for the MVP. Built only on endpoints that exist (see the API table in the [README](../../README.md)). Anything needing a missing endpoint is marked. Status: **approved** (2026-10-28).

## Navigation (sidebar, grouped)

```
Overview
SEND      Bulk Emails        (list; "New bulk email" is a button on that page)
          Single Email
PREPARE   Verify Emails
MONITOR   Failures           (hidden until the Failures endpoint exists)
Settings
```

Templates, Recipients and Activity Log are not in the MVP and are not shown. Routes: `/`, `/bulk-emails`, `/bulk-emails/new`, `/bulk-emails/:id`, `/single-email`, `/verify`, `/failures`, `/settings`. The top of every page: title, a breadcrumb when nested, the primary action on the right.

## Screens

### Overview (`GET /stats?tz=<browser tz>`)
- Row of 4 figures, each a link: **Sent today**, **Failed today** (-> Failures when built, else the newest batch with failures), **Queued** (pending + retrying), **Sending now** (processing). Figures are numbers with a label, not decorated cards.
- One chart: sent vs failed per day (14 days, zero-filled by the API). Purpose: spot a bad day.
- **Needs attention** list: batches with failed jobs or a high invalid rate; each row says why ("3 failed", "27% invalid rows") and links to that bulk email. Empty state: "Nothing needs attention."
- Auto-refresh every 15 s while the tab is visible.

### Bulk Emails list (`GET /batches`)
Table: filename, status, progress (sent/total), failed, invalid, created, completed. Filters: status, filename search (`q`). Paged server-side, filters in the URL. Row click opens the detail. Primary button: **New bulk email**. Empty state: "No bulk emails yet. Upload a spreadsheet to send your first." with the button.

### Bulk Email detail (`GET /batches/{id}`, `/batches/{id}/jobs`)
- Header: filename, status badge, created/completed times, **Retry failed** (only when `failed > 0`; confirm dialog states the count; calls `POST /batches/{id}/retry`).
- Progress: bar + "380 of 1,000 sent". Counts strip: Sent, Failed, Pending, Sending, Invalid, each clickable to filter the table.
- Jobs table (lazy): email, name, status badge, attempts ("2 of 4"), last error, updated. Search email/name. Row detail shows full `last_error`. Per-row **Retry** on failed rows (`POST /jobs/{id}/retry`).
- Polls every 2 s while status is `pending` or `processing`, stops when `completed`; never polls in a hidden tab.
- Completed with failures is a **partial-failure** state: banner "Completed. 12 emails failed" with the Retry action, not a green "success".

### New Bulk Email (wizard, 4 steps)
1. **Upload**: drop zone for `.xlsx`; calls `POST /recipients/parse` right away. Shows progress; parse errors inline.
2. **Review recipients**: summary line "38 valid, 4 invalid" and a problems table (row number, name, email, reason) with a **Download problems (CSV)** button built in the browser. Valid recipients in a second tab. If there are zero valid rows, the Next button is disabled and the message says to fix the file and replace it.
3. **Write message**: subject, body (plain text). Insert chips for `{{.Name}}` and `{{.Email}}`. Live preview rendered for the first valid recipient (client-side preview only; the server renders for real). Warn on unknown placeholders.
4. **Confirm and send**: exact summary "Send 38 emails from contacts.xlsx. 4 invalid rows will be skipped." Primary button "Send 38 emails" -> `POST /batches` (re-upload of the same file with subject and body). On `202`, go to the detail page.
- Back never loses entered text; leaving the wizard with unsaved work asks for confirmation.

### Verify Emails
Steps 1-2 of the wizard on their own, nothing is sent. Same report. A secondary button "Start a bulk email with this file" carries the file into the wizard at step 3.

### Single Email (`POST /emails`)
Form: To, Subject, Body. Synchronous, so the Send button shows a spinner until the SMTP result. Success: toast "Email sent to ..." and the form clears. Failure: inline error with the server's reason, form kept.

### Settings (`GET/PUT /settings`, `POST /settings/smtp/test`)
- Grouped: **Sender and SMTP**, **Behaviour**, **System (read-only)**. Each row: label, value, a source tag (`env` or `override`), "Applies instantly" or "Needs restart".
- Editing sets a draft; one **Save changes** bar appears (all-or-nothing, as the API is). "Reset to default" per row.
- SMTP password is write-only: shows "Set" or "Not set", never the value; a "Change" action opens an empty field.
- **Send test email** panel: recipient field, uses unsaved edits (the API accepts them); result shown inline with the SMTP reason on failure. Saving SMTP changes can be refused by the server after a live connection check; that error is shown at the top of the form and the draft is kept.
- Settings that need a restart show a persistent notice after saving.
- A permanent warning near SMTP fields: there is no login yet, so anyone who can reach the API can change them ([decision 0002](../decisions/0002-no-auth-for-now.md)).

### Failures (blocked on backend)
Cross-batch failed jobs grouped by error reason, with bulk retry. Needs the Failures endpoint; until then not shown in the menu.

## State matrix (applies to every data screen)

| State | Treatment |
|---|---|
| First load | Skeleton in place of the table/tiles |
| Empty | Sentence explaining why + the next action |
| Background refresh | No spinner; small "Updated Xs ago" |
| Validation error | Inline under the field, form-level summary after submit |
| API error (`{"error":...}`) | Inline message or page banner with the server's text and a **Retry** button; no auto-dismiss |
| Network down / API unreachable | Page banner "Cannot reach the server", queries keep last data with a stale marker |
| Success | Toast for quick confirmations, navigation for flow completion |
| Partial failure | Explicit counts ("26 sent, 12 failed") and the action to fix; never a plain success |
| Long-running | Progress with counts, not an indeterminate spinner |

## Open questions (need an answer before or during the build)

1. **Max upload size:** answered. The server rejects uploads over **10 MB** (`maxUploadBytes` in `internal/api/handler.go`); there is no row-count limit. The upload zone shows "Max 10 MB" and checks size before sending. Plain-text JSON bodies (single email) are capped at 1 MB.
2. **Recipient type** (Merchant vs Customer per upload): still undecided; would add a field in step 1 and a filter on the list.
3. **Is the UI English only?** Assumed yes; Bangla only needs to render correctly in data (names, email text). Confirm.
4. **Preview data:** the parse endpoint returns name and email only, so the live preview can only fill those placeholders. Fine unless templates gain more fields.
5. **Failures endpoint shape** is not designed yet; the Failures screen is a sketch.
6. **Dark mode:** deferred. Say so if someone needs it earlier.
