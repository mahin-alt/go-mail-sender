#!/usr/bin/env bash
# Benchmark the parser and the API with one big spreadsheet, and write the
# results to a folder so a later run can be compared with this one.
#
#   scripts/bench.sh [xlsx-file] [results-dir]
#
# Defaults: samples/12_million_mixed.xlsx and
# docs/benchmarks/<date>-<commit>. See docs/benchmarks/README.md.
#
# What it touches (and cleans up afterwards):
#   - a scratch schema `scratch_bench_test` in the DATABASE_URL Postgres
#     (your real tables are never written to)
#   - a throwaway Mailpit on 127.0.0.1:1026 (SMTP) / :8026 (UI), so your own
#     Mailpit on :1025 is not filled with benchmark emails
#   - the API on 127.0.0.1:8095, built to bin/bench-server
#
# Environment knobs: BENCH_SKIP_GO=1, BENCH_SKIP_E2E=1, GO_BENCH_COUNT (6),
# PARSE_RUNS (3), READ_RUNS (20), WORKER_SECONDS (60), WORKERS (5), BENCH_LOG_LEVEL (info).
set -euo pipefail

cd "$(dirname "$0")/.."

FILE=${1:-samples/12_million_mixed.xlsx}
OUT=${2:-docs/benchmarks/$(date +%F)-$(git rev-parse --short HEAD)}
GO_BENCH_COUNT=${GO_BENCH_COUNT:-6}
PARSE_RUNS=${PARSE_RUNS:-3}
READ_RUNS=${READ_RUNS:-20}
WORKER_SECONDS=${WORKER_SECONDS:-60}
WORKERS=${WORKERS:-5}

SCHEMA=scratch_bench_test
API_ADDR=127.0.0.1:8095
API=http://$API_ADDR
SMTP_ADDR=127.0.0.1:1026
MAILPIT_UI=127.0.0.1:8026
RESPONSE_TMP=bin/bench-response.json

[[ -f $FILE ]] || { echo "no such file: $FILE" >&2; exit 1; }
[[ -f .env ]] || { echo ".env not found (needs DATABASE_URL)" >&2; exit 1; }
mkdir -p "$OUT" bin

set -a
# shellcheck disable=SC1091
. ./.env
set +a
[[ $DATABASE_URL == *\?* ]] && SEP='&' || SEP='?'
BENCH_DB_URL="$DATABASE_URL${SEP}search_path=$SCHEMA"

log() { printf '[bench %s] %s\n' "$(date +%T)" "$*"; }
psql_q() { psql "$DATABASE_URL" -X -qtA -c "$1"; }

SERVER_PID=""
MAILPIT_PID=""
SCHEMA_CREATED=""
cleanup() {
	[[ -n $SERVER_PID ]] && kill "$SERVER_PID" 2>/dev/null && wait "$SERVER_PID" 2>/dev/null || true
	[[ -n $MAILPIT_PID ]] && kill "$MAILPIT_PID" 2>/dev/null && wait "$MAILPIT_PID" 2>/dev/null || true
	[[ -n $SCHEMA_CREATED ]] && psql_q "DROP SCHEMA IF EXISTS $SCHEMA CASCADE" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# ---------------------------------------------------------------- environment
{
	echo "date:        $(date -Is)"
	echo "commit:      $(git rev-parse --short HEAD) ($(git describe --tags --always --dirty))"
	echo "go:          $(go version)"
	echo "cpu:         $(lscpu | sed -n 's/^Model name:[[:space:]]*//p')"
	echo "cores:       $(nproc)"
	echo "ram:         $(free -h | awk '/^Mem:/ {print $2}')"
	echo "postgres:    $(psql_q 'SHOW server_version')"
	echo "file:        $FILE ($(du -h "$FILE" | cut -f1))"
	echo "settings:    WORKER_COUNT=$WORKERS SMTP_POOL_SIZE=$WORKERS DOMAIN_CHECK_ENABLED=false MAX_UPLOAD_MB=40"
	echo "smtp:        throwaway Mailpit on $SMTP_ADDR"
} >"$OUT/env.txt"
log "results go to $OUT"

# ------------------------------------------------------------ Go benchmark
if [[ -z ${BENCH_SKIP_GO:-} ]]; then
	log "Go benchmark: ParseExcelReader x$GO_BENCH_COUNT (about 10 s each)"
	BENCH_XLSX="$PWD/$FILE" go test ./internal/recipients -run '^$' \
		-bench ParseExcelReader -benchtime 1x -count "$GO_BENCH_COUNT" -timeout 30m \
		| tee "$OUT/go-bench.txt"
fi

[[ -n ${BENCH_SKIP_E2E:-} ]] && { log "done (end-to-end skipped)"; exit 0; }

# ------------------------------------------------------------- end-to-end
if [[ -n $(psql_q "SELECT 1 FROM pg_namespace WHERE nspname = '$SCHEMA'") ]]; then
	echo "schema $SCHEMA already exists (a previous run was interrupted?). Drop it first:" >&2
	echo "  psql \"\$DATABASE_URL\" -c 'DROP SCHEMA $SCHEMA CASCADE'" >&2
	exit 1
fi
psql_q "CREATE SCHEMA $SCHEMA" >/dev/null
SCHEMA_CREATED=1

log "building the API"
go build -o bin/bench-server ./cmd/server

mailpit --smtp "$SMTP_ADDR" --listen "$MAILPIT_UI" >"$OUT/mailpit.log" 2>&1 &
MAILPIT_PID=$!

# start_server <name>: a fresh API process, so its peak memory belongs to one test.
start_server() {
	DATABASE_URL="$BENCH_DB_URL" HTTP_ADDR="$API_ADDR" \
		SMTP_HOST=127.0.0.1 SMTP_PORT=${SMTP_ADDR##*:} SMTP_USERNAME= SMTP_PASSWORD= \
		WORKER_COUNT=$WORKERS SMTP_POOL_SIZE=$WORKERS DOMAIN_CHECK_ENABLED=false MAX_UPLOAD_MB=40 \
		LOG_FILE= LOG_FORMAT=json LOG_LEVEL=${BENCH_LOG_LEVEL:-info} \
		bin/bench-server >"$OUT/server-$1.log" 2>&1 &
	SERVER_PID=$!
	for _ in $(seq 120); do
		curl -sf "$API/healthz" >/dev/null && return 0
		kill -0 "$SERVER_PID" 2>/dev/null || { echo "server exited, see $OUT/server-$1.log" >&2; exit 1; }
		sleep 0.5
	done
	echo "server did not start" >&2
	exit 1
}
stop_server() {
	kill "$SERVER_PID" && wait "$SERVER_PID" 2>/dev/null || true
	SERVER_PID=""
}
# peak_mb: the highest memory (resident set) the API process has used so far.
peak_mb() { awk '/^VmHWM:/ {printf "%.0f", $2 / 1024}' "/proc/$SERVER_PID/status"; }
# log_field <log> <msg> <field>: a field of the last matching JSON log line.
log_field() { jq -r --arg m "$2" "select(.msg == \$m) | .$3" "$1" | tail -1; }
secs() { awk -v ns="$1" 'BEGIN {printf "%.2f", ns / 1e9}'; }
# stats <file of seconds>: "median p95" in milliseconds.
stats() {
	sort -n "$1" | awk '{a[NR] = $1 * 1000} END {
		m = (NR % 2) ? a[(NR + 1) / 2] : (a[NR / 2] + a[NR / 2 + 1]) / 2
		p = a[int(NR * 0.95 + 0.999)]
		printf "%.1f %.1f\n", m, p }'
}

RESULTS="$OUT/results.md"
{
	echo "# Benchmark results"
	echo
	echo '```'
	cat "$OUT/env.txt"
	echo '```'
	if [[ -f $OUT/go-bench.txt ]]; then
		echo
		echo "## Go benchmark: ParseExcelReader"
		echo
		echo '```'
		grep -E '^(Benchmark|cpu)' "$OUT/go-bench.txt"
		echo '```'
	fi
} >"$RESULTS"

# --- 1. Check a file ----------------------------------------------------
# The check endpoint: POST /uploads since the large-upload Phase 1, before that
# POST /recipients/parse (the baseline). Whichever this code has is measured.
if grep -q '"POST /api/v1/uploads"' internal/api/routes.go; then
	CHECK_PATH=/api/v1/uploads CHECK_MSG='upload checked' CHECK_TIME=total
else
	CHECK_PATH=/api/v1/recipients/parse CHECK_MSG='recipients parsed' CHECK_TIME=parse
fi
log "POST $CHECK_PATH x$PARSE_RUNS (fresh server each time)"
{
	echo
	echo "## POST $CHECK_PATH"
	echo
	echo "| run | status | total (s) | response (MB) | server peak memory (MB) | idle memory before (MB) | server time (s) | valid | problems |"
	echo "|---|---|---|---|---|---|---|---|---|"
} >>"$RESULTS"
for run in $(seq "$PARSE_RUNS"); do
	start_server "parse-$run"
	idle=$(peak_mb)
	read -r code total size < <(curl -s -o /dev/null -w '%{http_code} %{time_total} %{size_download}\n' \
		-F "file=@$FILE" "$API$CHECK_PATH")
	peak=$(peak_mb)
	sleep 0.5 # let the log line land
	L="$OUT/server-parse-$run.log"
	printf '| %s | %s | %.2f | %.3f | %s | %s | %s | %s | %s |\n' "$run" "$code" "$total" \
		"$(awk -v b="$size" 'BEGIN {print b / 1048576}')" "$peak" "$idle" \
		"$(secs "$(log_field "$L" "$CHECK_MSG" "$CHECK_TIME")")" \
		"$(log_field "$L" "$CHECK_MSG" valid)" "$(log_field "$L" "$CHECK_MSG" problems)" >>"$RESULTS"
	stop_server
done

# --- 2. POST /batches ----------------------------------------------------
log "POST /batches (the workers start sending to the throwaway Mailpit)"
start_server batches
idle=$(peak_mb)
read -r code total size < <(curl -s -o "$RESPONSE_TMP" -w '%{http_code} %{time_total} %{size_download}\n' \
	-F "file=@$FILE" -F 'subject=Hello {{.Name}}' \
	-F $'body=Hi {{.Name}},\n\nThis is a benchmark email for {{.Email}}.' \
	"$API/api/v1/batches")
t_created=$(date +%s.%N)
peak=$(peak_mb)
BATCH_ID=$(jq -r .batch_id "$RESPONSE_TMP")
: >"$RESPONSE_TMP" # keep bin/ small; the file is overwritten next run anyway
[[ $code == 202 && $BATCH_ID != null ]] || { echo "POST /batches failed: HTTP $code" >&2; exit 1; }
sleep 0.5
L="$OUT/server-batches.log"
{
	echo
	echo "## POST /api/v1/batches"
	echo
	echo "| status | total (s) | response (MB) | server peak memory (MB) | idle before (MB) | parse (s) | render (s) | insert (s) | pending | invalid |"
	echo "|---|---|---|---|---|---|---|---|---|---|"
	printf '| %s | %.2f | %.1f | %s | %s | %s | %s | %s | %s | %s |\n' "$code" "$total" \
		"$(awk -v b="$size" 'BEGIN {print b / 1048576}')" "$peak" "$idle" \
		"$(secs "$(log_field "$L" 'batch created' parse)")" \
		"$(secs "$(log_field "$L" 'batch created' render)")" \
		"$(secs "$(log_field "$L" 'batch created' insert)")" \
		"$(log_field "$L" 'batch created' pending)" "$(log_field "$L" 'batch created' invalid)"
	echo
	echo "Database after the insert: $(psql_q "SELECT pg_size_pretty(pg_total_relation_size('$SCHEMA.email_jobs'))") for \`email_jobs\` (table + indexes), $(psql_q "SELECT count(*) FROM $SCHEMA.email_jobs") rows."
} >>"$RESULTS"

# --- 3. Workers ----------------------------------------------------------
sent_now() { curl -s "$API/api/v1/batches/$BATCH_ID" | jq -r '.sent + .failed'; }
log "workers: counting emails sent for $WORKER_SECONDS s"
s0=$(sent_now)
t0=$(date +%s.%N)
sleep "$WORKER_SECONDS"
s1=$(sent_now)
t1=$(date +%s.%N)
rate=$(awk -v a="$s0" -v b="$s1" -v x="$t0" -v y="$t1" 'BEGIN {printf "%.1f", (b - a) / (y - x)}')
pending=$(log_field "$L" 'batch created' pending)
{
	echo
	echo "## Workers (WORKER_COUNT=$WORKERS, SMTP = throwaway Mailpit)"
	echo
	echo "- Emails handled (sent + failed) in $WORKER_SECONDS s: $((s1 - s0)) -> **$rate emails/s**"
	echo "- Already handled when counting started: $s0 (counting starts $(awk -v a="$t_created" -v b="$t0" 'BEGIN {printf "%.1f", b - a}') s after the response)"
	awk -v n="$pending" -v r="$rate" 'BEGIN { if (r > 0) printf "- At that rate the %d pending emails take about %.0f minutes\n", n, n / r / 60 }'
} >>"$RESULTS"

# --- 4. Cancel -----------------------------------------------------------
log "POST /batches/$BATCH_ID/cancel"
read -r code total < <(curl -s -o "$RESPONSE_TMP" -w '%{http_code} %{time_total}\n' -X POST "$API/api/v1/batches/$BATCH_ID/cancel")
cancelled=$(jq -r 'if .cancelled then "\(.cancelled) emails cancelled" else .error end' "$RESPONSE_TMP")
sleep 2 # let in-flight emails finish so the reads below see a still batch
{
	echo
	echo "## POST /api/v1/batches/{id}/cancel"
	echo
	printf -- '- HTTP %s in **%.2f s**: %s\n' "$code" "$total" "$cancelled"
} >>"$RESULTS"

# --- 5. Reads on the 1M batch --------------------------------------------
log "read endpoints x$READ_RUNS each"
{
	echo
	echo "## Read endpoints on the 1M-row batch ($READ_RUNS requests each)"
	echo
	echo "| endpoint | status | median (ms) | p95 (ms) | response (KB) |"
	echo "|---|---|---|---|---|"
} >>"$RESULTS"
bench_get() { # bench_get <label> <path>
	local times=bin/bench-times.txt code="" size=""
	: >"$times"
	for _ in $(seq "$READ_RUNS"); do
		read -r code t size < <(curl -s -o /dev/null -w '%{http_code} %{time_total} %{size_download}\n' "$API$2")
		echo "$t" >>"$times"
	done
	read -r med p95 < <(stats "$times")
	printf '| %s `%s` | %s | %s | %s | %.1f |\n' "$1" "$2" "$code" "$med" "$p95" \
		"$(awk -v b="$size" 'BEGIN {print b / 1024}')" >>"$RESULTS"
}
bench_get "batch progress" "/api/v1/batches/$BATCH_ID"
bench_get "batch list" "/api/v1/batches?limit=20"
bench_get "jobs, first page" "/api/v1/batches/$BATCH_ID/jobs?limit=50&offset=0"
bench_get "jobs, deep page" "/api/v1/batches/$BATCH_ID/jobs?limit=50&offset=900000"
bench_get "jobs, Invalid tab" "/api/v1/batches/$BATCH_ID/jobs?status=invalid&limit=50&offset=0"
bench_get "jobs, search (no match)" "/api/v1/batches/$BATCH_ID/jobs?q=zz-no-match-zz&limit=50&offset=0"
bench_get "dashboard stats" "/api/v1/stats"

stop_server
log "done: $RESULTS"
