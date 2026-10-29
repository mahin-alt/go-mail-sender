# 0008: One short branch per phase, cut from `main`

**Date:** 2026-10-29 · **Status:** accepted

## Decision
Each frontend phase (or other self-contained piece of work) gets its **own short-lived branch cut from `main`**, e.g. `feat/frontend-bulk-emails`. It is merged into `main` (by pull request) before the next one starts, then deleted. No long-lived "frontend" branch, and no sub-branches stacked on a feature branch.

Exception while a previous phase is not merged yet: the next branch is cut from the previous one (so it contains it). After the previous phase is merged, the new branch's pull request shows only its own commits. Phase 2's branch `feat/frontend-bulk-emails` was cut from `feat/dashboard-frontend` for this reason.

## Options considered
| Option | What it is | Why not / why |
|---|---|---|
| A. One long branch | Everything on `feat/dashboard-frontend` until the end | One huge review at the end, hard to revert one part. **Not chosen.** |
| **B. Branch per phase from `main`** | Small branches, merged often | The usual standard: small reviews, easy to revert, `main` always works. **Chosen.** |
| C. Sub-branches off the frontend branch | Each page branches off `feat/dashboard-frontend` and merges back | Adds a second layer of merging, and the frontend branch still has to be merged into `main` later. **Not chosen** (could be reconsidered for several people working in parallel). |

## How
1. Finish the phase on its branch (tests pass, feature doc updated, committed).
2. The user pushes the branch and opens a pull request into `main`; merge it.
3. `git switch main && git pull`, then cut the next branch from `main`.
