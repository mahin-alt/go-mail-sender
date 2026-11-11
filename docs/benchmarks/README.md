# Benchmarks

Numbers to compare before and after a performance change, measured with the
1M-row sample (`samples/12_million_mixed.xlsx`).

| Folder | Code | What it is |
|---|---|---|
| [2026-11-08-baseline/](2026-11-08-baseline/results.md) | tag `bench/baseline-2026-11-08` | before the large-upload scaling work |
| [2026-11-08-phase1/](2026-11-08-phase1/results.md) | branch `feat/upload-check-storage` | large-upload Phase 1: `POST /uploads`, paged problems, streaming parser |
| [2026-11-09-comparison.md](2026-11-09-comparison.md) | - | **plain-words comparison of baseline, Phase 1 and the four-table overhaul, with what to look out for** |
| [2026-11-09-four-tables/](2026-11-09-four-tables/results.md) | branch `refactor/database-schema-changes-with-system-overhaul` | four-table overhaul: background import (`POST /verify`, `POST /batches` + wait), Send + job generation, cursor-paged emails |

Each folder holds `results.md` (the summary to read), `env.txt` (machine and
settings), `go-bench.txt` (raw Go benchmark output, for benchstat) and the
server logs of each step.

## What is measured

| Part | How | Numbers |
|---|---|---|
| Parser (`recipients.Scan`; `ParseExcelReader` before 2026-11-09) | Go benchmark, 6 runs | time, memory allocated (`B/op`), allocations, **peak heap MB** |
| Check a file: `POST /verify` + wait until imported (`POST /uploads` in Phase 1, `POST /recipients/parse` in the baseline) | fresh API process, 3 runs | time until imported, **server peak memory** (Linux `VmHWM`), import time from the log |
| `POST /batches` + wait + `POST /batches/{id}/send` | fresh API process | time until every email exists, generation time from the log, server peak memory, database size (`email_jobs` and `batches` + `/jobs` before 2026-11-09) |
| Workers | count emails handled in 60 s | emails per second |
| `POST /batches/{id}/cancel` | once, on the 1M batch | time |
| Read endpoints (summary, list, emails pages, search, issues, sample, stats) | 20 requests each on the 1M batch | median and p95 (the slow 5%) in ms |

Domain check is off (DNS answers change minute to minute, which would make the
numbers jump for reasons unrelated to the code). Emails go to a throwaway
Mailpit on port 1026, not your own Mailpit on 1025.

## Running it

Needs: `.env` with `DATABASE_URL`, Postgres running, `mailpit` and `jq`
installed. Nothing is written to your real tables: the script uses a scratch
schema `scratch_bench_test` and drops it at the end.

```bash
scripts/bench.sh                                   # everything, ~6 minutes
```

Results land in `docs/benchmarks/<date>-<commit>/`. Other forms:

```bash
scripts/bench.sh samples/12_million_mixed.xlsx docs/benchmarks/2026-11-20-phase1   # choose the folder name
BENCH_SKIP_E2E=1 scripts/bench.sh                  # only the Go parser benchmark (~1 minute)
BENCH_SKIP_GO=1 scripts/bench.sh                   # only the API part
READ_RUNS=50 WORKER_SECONDS=120 scripts/bench.sh   # more samples
```

The Go benchmark on its own (skipped unless `BENCH_XLSX` is set, so normal
`go test ./...` stays fast):

```bash
BENCH_XLSX=$PWD/samples/12_million_mixed.xlsx \
  go test ./internal/recipients -run '^$' -bench Scan -benchtime 1x -count 6
```

- `-run '^$'` runs no normal tests, only benchmarks
- `-benchtime 1x` parses the file once per run (one parse takes several seconds)
- `-count 6` repeats it 6 times, so benchstat can tell a real change from noise

Worker speed (emails/s) and the batch read endpoints vary a lot between runs on
a busy laptop (475 to 16 emails/s with identical code on 2026-11-08): run them
several times before drawing conclusions. `BENCH_LOG_LEVEL=debug` makes the
server log every email's SMTP time, which tells SMTP and database time apart.

If a run is interrupted, the scratch schema may be left behind and the next run
refuses to start; drop it with
`psql "$DATABASE_URL" -c 'DROP SCHEMA scratch_bench_test CASCADE'`.

## Comparing after a change

1. Run `scripts/bench.sh` on the new code, with a folder name that says what changed.
2. Go numbers: benchstat (official Go tool) prints old vs new with the % change
   and whether the difference is real (`p=` below 0.05) or noise (`~`):

   ```bash
   go run golang.org/x/perf/cmd/benchstat@latest \
     docs/benchmarks/2026-11-08-baseline/go-bench.txt \
     docs/benchmarks/<new-folder>/go-bench.txt
   ```

3. API numbers: open both `results.md` files side by side.
4. Copy the before/after table into the feature doc of the change.

**Fair comparisons:** numbers depend on the machine and on what else is running.
For an important comparison, run the baseline again on the same machine, right
before the new code, from its tag in a separate folder:

```bash
git worktree add ../bench-baseline bench/baseline-2026-11-08
(cd ../bench-baseline && cp ../go-mail-sender/.env . && scripts/bench.sh samples/12_million_mixed.xlsx ../go-mail-sender/docs/benchmarks/2026-11-20-baseline-rerun)
git worktree remove ../bench-baseline
```

Close heavy programs (browsers, builds) while benchmarking, and run the two
versions one after the other, not at the same time (they share ports and the
schema name).
