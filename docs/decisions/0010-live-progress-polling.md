# 0010: Live progress uses always-on polling; push updates are a future wish

**Date:** 2026-10-29 · **Status:** accepted (polling) · **Future:** option D (server push)

## Decision (option A)
Pages that show progress (Bulk Emails list, batch detail, later the Dashboard) **poll the API at a fixed interval** with TanStack Query's `refetchInterval`: list every **5 s**, batch detail every **3 s**. The intervals live in one place in the code so they are easy to change. Query Client already pauses refetching in background tabs (`refetchIntervalInBackground: false`).

## Options considered
| Option | What it means | Verdict |
|---|---|---|
| **A. Always poll** | Ask the API every few seconds, all the time | **Chosen.** Simplest. Costs some requests while idle, fine for a handful of staff on one API. |
| B. Poll only while something is active | Stop polling when no batch is pending/processing | Recommended by the assistant, not chosen. Easy to switch to later: make `refetchInterval` return `false` when nothing is active. |
| C. Manual refresh button only | Updates on click | Progress looks frozen. Not chosen. |
| **D. Server push (SSE or WebSocket)** | The Go API pushes changes, no polling | **Wanted in the future** (the user is interested). Needs new backend work: an events endpoint, workers publishing progress, CORS/proxy care for long-lived connections. |

## Future improvement: option D
Server-Sent Events are the likely fit (one-way, plain HTTP, auto-reconnect). When picked up: add an SSE endpoint in Go, have the Vue side subscribe and update the TanStack Query cache from events instead of polling. Listed as "proposed" in [ROADMAP.md](../ROADMAP.md).
