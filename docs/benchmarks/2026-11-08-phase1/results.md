# Benchmark results

```
date:        2026-11-08T19:36:32+06:00
commit:      773e765 (bench/baseline-2026-11-08-1-g773e765-dirty)
go:          go version go1.27.1 linux/amd64
cpu:         12th Gen Intel(R) Core(TM) i5-12400
cores:       12
ram:         14Gi
postgres:    18.6 (Ubuntu 18.6-1.pgdg26.04+2)
file:        samples/12_million_mixed.xlsx (18M)
settings:    WORKER_COUNT=5 SMTP_POOL_SIZE=5 DOMAIN_CHECK_ENABLED=false MAX_UPLOAD_MB=40
smtp:        throwaway Mailpit on 127.0.0.1:1026
```

## Go benchmark: ParseExcelReader

```
cpu: 12th Gen Intel(R) Core(TM) i5-12400
BenchmarkParseExcelReader-12    	       1	6203791926 ns/op	       211.3 peak-heap-MB	     69507 problem-rows	    880533 valid-rows	3339727296 B/op	69421123 allocs/op
BenchmarkParseExcelReader-12    	       1	6194317498 ns/op	       207.7 peak-heap-MB	     69507 problem-rows	    880533 valid-rows	3339084104 B/op	69419983 allocs/op
BenchmarkParseExcelReader-12    	       1	6188889922 ns/op	       209.8 peak-heap-MB	     69507 problem-rows	    880533 valid-rows	3339417504 B/op	69420018 allocs/op
BenchmarkParseExcelReader-12    	       1	6192623517 ns/op	       210.1 peak-heap-MB	     69507 problem-rows	    880533 valid-rows	3339466808 B/op	69420019 allocs/op
BenchmarkParseExcelReader-12    	       1	6260088101 ns/op	       209.9 peak-heap-MB	     69507 problem-rows	    880533 valid-rows	3339684752 B/op	69420033 allocs/op
BenchmarkParseExcelReader-12    	       1	6193280570 ns/op	       212.8 peak-heap-MB	     69507 problem-rows	    880533 valid-rows	3339793656 B/op	69420034 allocs/op
```

## POST /api/v1/uploads

| run | status | total (s) | response (MB) | server peak memory (MB) | idle memory before (MB) | server time (s) | valid | problems |
|---|---|---|---|---|---|---|---|---|
| 1 | 201 | 10.40 | 0.000 | 143 | 20 | 10.40 | 880533 | 69507 |
| 2 | 201 | 10.26 | 0.000 | 142 | 20 | 10.26 | 880533 | 69507 |
| 3 | 201 | 10.26 | 0.000 | 139 | 20 | 10.26 | 880533 | 69507 |

## POST /api/v1/batches

| status | total (s) | response (MB) | server peak memory (MB) | idle before (MB) | parse (s) | render (s) | insert (s) | pending | invalid |
|---|---|---|---|---|---|---|---|---|---|
| 202 | 18.75 | 0.0 | 797 | 20 | 6.21 | 4.12 | 8.37 | 880533 | 69507 |

Database after the insert: 259 MB for `email_jobs` (table + indexes), 950040 rows.

## Workers (WORKER_COUNT=5, SMTP = throwaway Mailpit)

- Emails handled (sent + failed) in 60 s: 13161 -> **218.9 emails/s**
- Already handled when counting started: 0 (counting starts 0.9 s after the response)
- At that rate the 880533 pending emails take about 67 minutes

## POST /api/v1/batches/{id}/cancel

- HTTP 200 in **4.19 s**: 867260 emails cancelled

## Read endpoints on the 1M-row batch (20 requests each)

| endpoint | status | median (ms) | p95 (ms) | response (KB) |
|---|---|---|---|---|
| batch progress `/api/v1/batches/1` | 200 | 136.6 | 198.2 | 0.3 |
| batch list `/api/v1/batches?limit=20` | 200 | 423.7 | 468.4 | 0.3 |
| jobs, first page `/api/v1/batches/1/jobs?limit=50&offset=0` | 200 | 43.3 | 54.0 | 24.1 |
| jobs, deep page `/api/v1/batches/1/jobs?limit=50&offset=900000` | 200 | 463.8 | 574.9 | 15.7 |
| jobs, Invalid tab `/api/v1/batches/1/jobs?status=invalid&limit=50&offset=0` | 200 | 391.9 | 600.0 | 15.7 |
| jobs, search (no match) `/api/v1/batches/1/jobs?q=zz-no-match-zz&limit=50&offset=0` | 200 | 640.5 | 691.1 | 0.0 |
| dashboard stats `/api/v1/stats` | 200 | 175.9 | 217.7 | 0.8 |
