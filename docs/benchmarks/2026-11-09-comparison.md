# 1 million rows: old vs new, in plain words

**Date:** 2026-11-09 · **File:** `samples/12_million_mixed.xlsx` (18.8 MB, 950,040 data rows, 880,533 valid, 69,507 invalid) · **Machine:** i5-12400, 12 cores, 14 GB RAM, PostgreSQL 18.6, local throwaway Mailpit · **Domain check:** off in every run (DNS answers change from minute to minute and would make the numbers jump)

Three versions were measured with the same file, the same machine and the same script (`scripts/bench.sh`):

| Name in this report | What it is | Results |
|---|---|---|
| **Baseline** | the code before any large-file work (2026-11-08) | [2026-11-08-baseline](2026-11-08-baseline/results.md) |
| **Phase 1** | check results stored in `uploads` / `upload_rows` (2026-11-08) | [2026-11-08-phase1](2026-11-08-phase1/results.md) |
| **Now** | the four-table overhaul, the code on branch `refactor/database-schema-changes-with-system-overhaul` | [2026-11-09-four-tables](2026-11-09-four-tables/results.md), plus a second run the same day to check the numbers repeat |

The latest code was benchmarked: the only change after the run was a whitespace fix by `gofmt`.

## The short version

- **The server now uses far less memory.** Checking the file peaks at 59-90 MB instead of 450-530 MB (baseline) or about 140 MB (Phase 1). Starting to send peaks at 59 MB instead of about 800 MB. This was the main goal, and it is met.
- **The user never waits for the file any more.** The upload is answered in 0.03 s; the check runs in the background with a live "rows read" counter. Before, the browser waited 6-10 s for the check and 18 s again for sending (the file was read twice).
- **The browser no longer downloads the rows.** The baseline sent 67.5 MB of JSON to the browser for one check; now every answer is a few KB.
- **Reading the data is 100-500 times faster.** Batch summary, lists and pages of emails now answer in about 1-5 ms instead of 40-470 ms.
- **A crash no longer loses work.** If the server dies during the import or while creating the emails, it continues from where it stopped (tested with `kill -9`, result identical to a clean run).
- **Two things got slower:** the background check itself (16.4 s instead of 6-10 s) and the sending speed (315-346 emails/s instead of 475 in the baseline). Both are explained below.

## Step by step

### 1. Reading the spreadsheet (the parser alone, no database)

| | Baseline | Phase 1 | Now |
|---|---|---|---|
| Time | 6.2 s | 6.2 s | 7.3-8.6 s |
| Most memory in use at once | 262-274 MB | 208-213 MB | **36-37 MB** (one run 71 MB) |

**In words:** the old parser collected every row into a big list in memory and also remembered every email address it had seen (to find duplicates). The new one looks at one row, hands it on, and forgets it; duplicates are no longer flagged, so there is nothing to remember. Memory dropped to about a seventh. It is a little slower (about 1-2 s) because rows are handed one by one to a callback instead of being collected in bulk; this is the price of not keeping them.

### 2. Checking a file (upload until the result can be shown)

| | Baseline `POST /recipients/parse` | Phase 1 `POST /uploads` | Now `POST /verify` + background import |
|---|---|---|---|
| Upload answered after | 6.3 s | 10.3 s | **0.03 s** |
| Result ready after | 6.3 s | 10.3 s | 16.2-17.0 s |
| Server peak memory | 454-528 MB | 139-143 MB | **59-90 MB** |
| Data sent to the browser | 67.5 MB | 0 MB (totals only) | 0 MB (totals only) |
| Survives a server crash halfway | no | no (start again) | **yes** (continues from the checkpoint) |
| Rows saved | none | yes, for 24 hours | yes, permanently |

**In words:** before, the browser held the connection open for the whole check. Now the file is saved to disk and the answer comes back at once; a background worker reads it and saves the rows 1,000 at a time, and the page shows how many rows were read so far.

The check takes about 6 s longer than in Phase 1. Phase 1 saved all rows in one single database transaction with PostgreSQL's fast bulk loader (COPY); the new code saves each chunk of 1,000 rows in its own transaction, together with the checkpoint that says "rows up to here are saved". That is what makes it crash-safe, and it costs time: about 950 small transactions instead of one big one.

### 3. Starting to send

| | Baseline `POST /batches` | Phase 1 `POST /batches` | Now `POST /batches/{id}/send` |
|---|---|---|---|
| What the server does | reads the file again, builds every email, saves them | the same | creates one small email record per valid row, from rows already in the database |
| Answered after | 18.0 s | 18.8 s | **0.00 s** (the work runs in the background) |
| Every email ready to send after | 18.0 s | 18.8 s | **10.3-10.8 s** |
| Server peak memory | 814 MB | 797 MB | **59 MB** |
| Survives a server crash halfway | no | no | **yes** |
| A double click on Send | sends twice | sends twice | **refused with 409** |

**In words:** the old code had to read the spreadsheet a second time and build 880,533 complete emails (subject and body for each person) in memory before saving them. Now the rows are already in the database; the server only writes a small "send to row X" record for each, 5,000 at a time, and the subject and body are built at the moment each email is sent.

### 4. Whole journey: upload, check, send

| | Baseline | Phase 1 | Now |
|---|---|---|---|
| Time from upload until every email is ready | about 24 s (6.3 + 18.0, the file is uploaded and read twice) | about 29 s (10.3 + 18.8) | **about 27 s** (16.4 + 10.4, uploaded once) |
| Highest server memory on the way | 814 MB | 797 MB | **90 MB** |

The total time is about the same; the difference is that nothing waits on the browser, nothing is uploaded twice, and memory stays under 100 MB.

### 5. Sending (5 workers, local Mailpit, 60 seconds counted)

| | Baseline | Phase 1 | Now (run 1) | Now (run 2) |
|---|---|---|---|---|
| Emails per second | **475** | 219 | 315 | 346 |
| 880,533 emails would take | 31 min | 67 min | 47 min | 42 min |

**In words:** this is the one real step back compared with the baseline. Each email now costs the database more work:

- Claiming an email is a small transaction (pick the email, lock its batch, mark it, move the batch counters) instead of one statement, so a cancelled batch can never send and nothing deadlocks.
- Each result (sent or failed) also updates the batch's counters (sent, pending...) in the same transaction. All 5 workers update the same batch row, so they wait for each other a little.
- The worker builds the subject and body for every email (before, they were built once at upload).

In return, the batch page reads its numbers in under 1 ms instead of counting a million rows each time (step 7). With a real email provider, the provider's speed limit (often well below 300 per second) usually decides the sending speed, not this. Phase 1's 219 shows these numbers also move a lot between runs and code versions, so compare with care.

### 6. Cancel

| | Baseline | Phase 1 | Now |
|---|---|---|---|
| Stopping about 860,000 unsent emails | 4.3 s | 4.2 s | 6.6-6.7 s |

A little slower, because cancel now also corrects the batch counters and runs a second pass for emails created while it waited. It is still one click and a few seconds.

### 7. Reading data on the 1-million-row batch (median of 20 requests)

| What the page asks for | Baseline | Phase 1 | Now |
|---|---|---|---|
| Batch summary (polled every few seconds) | 131 ms | 137 ms | **0.4-0.8 ms** |
| Bulk Emails list | 422 ms | 424 ms | **0.6-0.7 ms** |
| First page of emails | 43 ms | 43 ms | **2-5 ms** |
| A page near the end of the emails | 193 ms | 464 ms | **2-5 ms** |
| One status tab (Sent / Failed / Invalid) | 147 ms | 392 ms | **4-5 ms** |
| Problem rows of the file | - | under 1 ms | **0.5-0.7 ms** |
| Dashboard numbers | 86 ms | 176 ms | **56-57 ms** |
| Search with no match | 527 ms | 641 ms | 662-672 ms (unchanged) |

**In words:** the old pages counted all million emails on every refresh. Now the batch keeps its own counters (updated together with the emails), and lists jump straight to the right place with a "start after ID X" cursor instead of skipping thousands of rows. Only searching for text that matches nothing is still slow: the database has to look at every row to be sure.

### 8. Database size

| | Baseline / Phase 1 | Now |
|---|---|---|
| One million-row file, checked only | rows kept 24 h (Phase 1) | about **230 MB** in `email_recipients`, kept permanently |
| The same file, sent | 259 MB in `email_jobs` (each email with its own subject and body) | 230 MB rows + **218 MB** `email_jobs` |
| The uploaded `.xlsx` on disk | not kept | 18.8 MB per upload, kept permanently |

The email records are smaller than before (no copy of the subject and body), but the spreadsheet rows are now kept forever as the single source of truth, so a sent file takes about 450 MB in total.

## What to look out for

1. **Sending speed (most important).** 315-346 emails/s is below the baseline's 475/s. Before a big real send, measure against the real SMTP provider: if the provider allows more than about 300 per second, the next improvements are to claim emails in small groups and to update the batch counters in groups instead of once per email.
2. **Disk space grows with every upload.** Uploads are permanent now: about 230 MB of database per million-row file (about 450 MB once sent) plus the `.xlsx` on disk. Watch the database disk, and plan the "Delete a batch" and "Temporary upload files" ROADMAP items before heavy use.
3. **The check takes 16 s in the background.** That is fine for one file, but there is one import worker, so two big uploads at the same moment are checked one after the other (the second waits about 16 s more). If this becomes a problem: bigger chunks (5,000 instead of 1,000) or COPY inside each chunk.
4. **The domain check was off in these runs.** With it on, each different email domain costs one DNS lookup (up to 3 s when DNS is slow), done one after another. A file with thousands of different domains can take minutes to check. The 1M sample has only 17 domains, so it hardly matters there.
5. **Search with no match stays slow** (about 0.7 s on a million emails). It does not grow worse with this change; a text index (PostgreSQL trigram) would fix it if staff search a lot.
6. **Memory still depends on the file, not the rows.** The spreadsheet library keeps the compressed file and its table of distinct texts in memory, so a file with many unique names uses more. `MAX_UPLOAD_MB` (here 40 MB) is the safety limit; keep it.
7. **Counters are trusted for the numbers on screen.** They are updated in the same transaction as the rows and every test checks them against a fresh count, but nothing re-checks them on a schedule yet (`ReconcileBatchCounters` exists for that).
8. **Numbers move between runs.** One machine, local Mailpit, 60-second sending window: the sending rate moved 10% between two runs on the same code, and peak memory between 59 and 90 MB. Compare trends, not single digits.

## How to repeat

```bash
scripts/bench.sh samples/12_million_mixed.xlsx docs/benchmarks/$(date +%F)-mytest
```

About 5-6 minutes; uses a scratch schema and a throwaway Mailpit, never your real tables. See [README.md](README.md).
