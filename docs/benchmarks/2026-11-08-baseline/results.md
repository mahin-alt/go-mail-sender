# Benchmark results

```
date:        2026-11-08T17:56:10+06:00
commit:      86cffbe (bench/baseline-2026-11-08)
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
BenchmarkParseExcelReader-12    	       1	6236225678 ns/op	       262.0 peak-heap-MB	     69507 problem-rows	    880533 valid-rows	3477189624 B/op	69421140 allocs/op
BenchmarkParseExcelReader-12    	       1	6266520514 ns/op	       268.6 peak-heap-MB	     69507 problem-rows	    880533 valid-rows	3477088976 B/op	69420044 allocs/op
BenchmarkParseExcelReader-12    	       1	6319380791 ns/op	       267.4 peak-heap-MB	     69507 problem-rows	    880533 valid-rows	3477034064 B/op	69420037 allocs/op
BenchmarkParseExcelReader-12    	       1	6266281618 ns/op	       273.9 peak-heap-MB	     69507 problem-rows	    880533 valid-rows	3477251128 B/op	69420050 allocs/op
BenchmarkParseExcelReader-12    	       1	6264299950 ns/op	       268.9 peak-heap-MB	     69507 problem-rows	    880533 valid-rows	3476923344 B/op	69420026 allocs/op
BenchmarkParseExcelReader-12    	       1	6221326746 ns/op	       273.7 peak-heap-MB	     69507 problem-rows	    880533 valid-rows	3476214152 B/op	69419974 allocs/op
```

## POST /api/v1/recipients/parse

| run | status | total (s) | response (MB) | server peak memory (MB) | idle memory before (MB) | parse step (s) | valid | problems |
|---|---|---|---|---|---|---|---|---|
| 1 | 200 | 6.26 | 67.5 | 467 | 20 | 5.99 | 880533 | 69507 |
| 2 | 200 | 6.24 | 67.5 | 528 | 20 | 5.99 | 880533 | 69507 |
| 3 | 200 | 6.25 | 67.5 | 454 | 20 | 5.98 | 880533 | 69507 |

## POST /api/v1/batches

| status | total (s) | response (MB) | server peak memory (MB) | idle before (MB) | parse (s) | render (s) | insert (s) | pending | invalid |
|---|---|---|---|---|---|---|---|---|---|
| 202 | 17.96 | 4.1 | 814 | 20 | 6.03 | 3.87 | 7.97 | 880533 | 69507 |

Database after the insert: 259 MB for `email_jobs` (table + indexes), 950040 rows.

## Workers (WORKER_COUNT=5, SMTP = throwaway Mailpit)

- Emails handled (sent + failed) in 60 s: 28576 -> **475.2 emails/s**
- Already handled when counting started: 10 (counting starts 0.8 s after the response)
- At that rate the 880533 pending emails take about 31 minutes

## POST /api/v1/batches/{id}/cancel

- HTTP 200 in **4.34 s**: 851865 emails cancelled

## Read endpoints on the 1M-row batch (20 requests each)

| endpoint | status | median (ms) | p95 (ms) | response (KB) |
|---|---|---|---|---|
| batch progress `/api/v1/batches/1` | 200 | 131.1 | 172.7 | 0.3 |
| batch list `/api/v1/batches?limit=20` | 200 | 421.8 | 453.4 | 0.3 |
| jobs, first page `/api/v1/batches/1/jobs?limit=50&offset=0` | 200 | 43.2 | 48.7 | 24.0 |
| jobs, deep page `/api/v1/batches/1/jobs?limit=50&offset=900000` | 200 | 193.1 | 440.1 | 15.6 |
| jobs, Invalid tab `/api/v1/batches/1/jobs?status=invalid&limit=50&offset=0` | 200 | 147.0 | 214.2 | 15.6 |
| jobs, search (no match) `/api/v1/batches/1/jobs?q=zz-no-match-zz&limit=50&offset=0` | 200 | 527.1 | 589.6 | 0.0 |
| dashboard stats `/api/v1/stats` | 200 | 85.5 | 90.2 | 0.8 |
