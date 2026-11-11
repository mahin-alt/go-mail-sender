# Benchmark results

```
date:        2026-11-09T17:00:33+06:00
commit:      c03b967 (bench/baseline-2026-11-08-5-gc03b967-dirty)
go:          go version go1.27.1 linux/amd64
cpu:         12th Gen Intel(R) Core(TM) i5-12400
cores:       12
ram:         14Gi
postgres:    18.6 (Ubuntu 18.6-1.pgdg26.04+2)
file:        samples/12_million_mixed.xlsx (18M)
settings:    WORKER_COUNT=5 SMTP_POOL_SIZE=5 DOMAIN_CHECK_ENABLED=false MAX_UPLOAD_MB=40
smtp:        throwaway Mailpit on 127.0.0.1:1026
```

## Go benchmark: Scan (read every row, nothing kept)

```
cpu: 12th Gen Intel(R) Core(TM) i5-12400
BenchmarkScan-12    	       1	8214672193 ns/op	        36.49 peak-heap-MB	     69507 problem-rows	    880533 valid-rows	3041965480 B/op	69416507 allocs/op
BenchmarkScan-12    	       1	8600084852 ns/op	        36.48 peak-heap-MB	     69507 problem-rows	    880533 valid-rows	3041825416 B/op	69415419 allocs/op
BenchmarkScan-12    	       1	7633320070 ns/op	        37.33 peak-heap-MB	     69507 problem-rows	    880533 valid-rows	3041820296 B/op	69415410 allocs/op
BenchmarkScan-12    	       1	7340544041 ns/op	        36.48 peak-heap-MB	     69507 problem-rows	    880533 valid-rows	3041819520 B/op	69415402 allocs/op
BenchmarkScan-12    	       1	7926506928 ns/op	        70.56 peak-heap-MB	     69507 problem-rows	    880533 valid-rows	3041818896 B/op	69415401 allocs/op
BenchmarkScan-12    	       1	7483781828 ns/op	        37.34 peak-heap-MB	     69507 problem-rows	    880533 valid-rows	3041821392 B/op	69415405 allocs/op
```

## POST /api/v1/verify (upload + background import)

| run | status | upload answer (s) | until imported (s) | server peak memory (MB) | idle memory before (MB) | import time in log (s) | rows | valid | invalid |
|---|---|---|---|---|---|---|---|---|---|
| 1 | 202 | 0.03 | 16.31 | 90 | 21 | 16.24 | 950040 | 880533 | 69507 |
| 2 | 202 | 0.02 | 16.97 | 60 | 20 | 16.89 | 950040 | 880533 | 69507 |
| 3 | 202 | 0.03 | 16.18 | 59 | 20 | 16.03 | 950040 | 880533 | 69507 |

## POST /api/v1/batches/{id}/send (job generation)

| send answer (s) | until every email exists (s) | generation time in log (s) | emails created | server peak memory (MB) | idle before (MB) |
|---|---|---|---|---|---|
| 0.00 | 10.84 | 10.71 | 880533 | 59 | 21 |

Database (note: the schema holds 4 imports of the file at this point, the 3 verify runs plus this one, so about 230 MB per million-row file for `email_recipients`): 916 MB for `email_recipients`, 218 MB for `email_jobs` (tables + indexes), 880533 email rows.

## Workers (WORKER_COUNT=5, SMTP = throwaway Mailpit)

- Emails handled (sent + failed) in 60 s: 18929 -> **315.3 emails/s**
- Already handled when counting started: 268 (counting starts 0.8 s after the emails existed)
- At that rate the 861325 pending emails take about 46 minutes

## POST /api/v1/batches/{id}/cancel

- HTTP 200 in **6.71 s**: 861312 emails cancelled

## Read endpoints on the 1M-row batch (20 requests each)

| endpoint | status | median (ms) | p95 (ms) | response (KB) |
|---|---|---|---|---|
| batch summary `/api/v1/batches/4` | 200 | 0.8 | 1.9 | 0.8 |
| batch list `/api/v1/batches?type=bulk_send&limit=20` | 200 | 0.6 | 0.7 | 0.7 |
| emails, first page `/api/v1/batches/4/emails?limit=50` | 200 | 2.1 | 2.7 | 23.5 |
| emails, deep page `/api/v1/batches/4/emails?limit=50&after=880433` | 200 | 1.8 | 2.5 | 23.4 |
| emails, Sent tab `/api/v1/batches/4/emails?status=sent&limit=50` | 200 | 4.6 | 7.8 | 23.5 |
| emails, search (no match) `/api/v1/batches/4/emails?q=zz-no-match-zz&limit=50` | 200 | 662.1 | 687.8 | 0.0 |
| issues, all problems `/api/v1/batches/4/issues?limit=100` | 200 | 0.6 | 0.8 | 11.9 |
| issues, Bad domain tab `/api/v1/batches/4/issues?reason=bad_domain&limit=100` | 200 | 0.6 | 0.8 | 0.0 |
| preview sample `/api/v1/batches/4/sample` | 200 | 0.9 | 1.2 | 0.7 |
| dashboard stats `/api/v1/stats` | 200 | 55.8 | 66.4 | 0.8 |

## Compared with Phase 1 (2026-11-08)

- Upload answer: 0.03 s instead of 10.3 s (the check now runs in the background); time until checked 16.2-17.0 s vs 10.3 s, because every 1,000-row chunk is a transaction with its checkpoint (crash-safe) instead of one COPY in one transaction.
- Server peak memory while checking: 59-90 MB vs 139-143 MB; sending start (job generation) 59 MB vs 797 MB for the old `POST /batches`.
- Send: emails created in 10.8 s; workers 315 emails/s vs 219 (no subject/body copy per email any more).
- Reads: batch summary 0.8 ms vs 137, list 0.6 vs 424, deep email page 1.8 vs 464 (cursor instead of offset), stats 56 vs 176. Search with no match is unchanged (662 vs 641 ms, it scans the batch).
- Cancel of 861k emails: 6.7 s vs 4.2 s (counters and a second pass for emails created during generation).
