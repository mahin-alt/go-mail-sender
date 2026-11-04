# 0012: Structured logging with log/slog, optional log file

**Date:** 2026-11-04 · **Status:** accepted

## Context
The server logged with the standard `log` package: plain text, no levels, no request log, and two lines per email with the full address (about 2 million lines for a 1M-row batch, all holding customer addresses).

## Decision
- Use Go's built-in **`log/slog`** (no new dependency). `logging.Setup` installs it as the default logger and also captures any remaining `log.Printf` lines (they come out as INFO).
- Configured in `.env`, read at startup (a restart applies changes): `LOG_LEVEL` (debug | info | warn | error, default info), `LOG_FORMAT` (text | json, default text), `LOG_FILE` (empty = terminal only).
- **Log file:** the user chose `logs/server.log`. Every line goes to the terminal **and** the file (`io.MultiWriter`); the folder is created; `logs/` is gitignored. A file that cannot be opened stops the server at startup rather than running without logs.
- **Request log** middleware: one line per request with method, path, status, duration and an 8-character request ID, also returned as `X-Request-ID` (exposed through CORS). 5xx ERROR, 4xx WARN; successful changes (POST, PUT, ...) INFO; successful reads (GET, HEAD) and CORS preflights DEBUG, because the dashboard polls every 3-5 s and those lines buried everything else at `info`. `/healthz` skipped.
- **Worker:** successful sends are DEBUG with `send` (SMTP time) and `queued` (time from due to claimed, measured by the database clock); a retry is WARN, a final failure ERROR, both with `send`; one INFO summary when a batch completes with `duration` and `rate`. Email addresses are masked (`a***@gmail.com`).
- **Timings at INFO:** `recipients parsed` and `batch created` log `parse`, `domain_check`, (`render`, `insert` for batches) and `total`, so "where did the time go" is answered without switching to debug.

## Options considered
| Option | Verdict |
|---|---|
| `log/slog` | **Chosen.** Standard library, levels, text and JSON. |
| zap / zerolog | Faster at very high volume, but a dependency we do not need yet. |
| Terminal only (12-factor) | Kept as an option (`LOG_FILE=` empty), recommended for production under systemd or Docker. |
| App-managed log file | Chosen for local development because it is easiest to find and search. |

## Not done yet
- Log rotation (the file grows forever): use `logrotate`, or a library such as lumberjack, when it matters.
- Linking handler error lines to their request ID (would need a logger in the request context).
- Log level on the Settings page (instant change); shipping logs to a central tool; frontend logs.
