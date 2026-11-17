#!/usr/bin/env bash
# Benchmarks three servers with redis-benchmark and writes the results to result.md,
# next to this script:
#   - real Redis              on port 5000 (no persistence: --save "" --appendonly no)
#   - Redis-k02 single-thread on port 4000
#   - Redis-k02 multithread   on port 4000
# Every test runs once per server, each time on a freshly started server (empty keyspace).
# The script builds Redis-k02 and starts/stops every server itself, so ports 4000 and 5000
# must be free before you run it.
#
# Redis-k02 is Linux only (epoll), so run it inside WSL:
#   wsl -d Ubuntu bash -lc 'bash /mnt/f/Git/Redis-k02/benchmark/run.sh'
# Needs go, redis-server, redis-cli, redis-benchmark and ss. Takes about 10 minutes.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)" # module root, where go.mod is
CONFIG="$ROOT/internal/config/config.go"
OUT="$HERE/result.md"
OUR_PORT=4000 # must match config.Port
REDIS_PORT=5000
VERSIONS=(real single multi)

TMP="$(mktemp -d)"
BIN="$TMP/redis-k02"
PID=""     # pid of the server currently running
TEST_NO=0
PREFILL=() # redis-benchmark args run on every server before the next test only

die() {
	echo "error: $*" >&2
	exit 1
}

port_of() { if [ "$1" = real ]; then echo "$REDIS_PORT"; else echo "$OUR_PORT"; fi; }

start_server() { # start_server <real|single|multi>
	local v=$1 port
	port=$(port_of "$v")
	case $v in
	real) redis-server --port "$REDIS_PORT" --save "" --appendonly no >"$TMP/$v.log" 2>&1 & ;;
	single) "$BIN" <<<1 >"$TMP/$v.log" 2>&1 & ;; # <<<1 answers the "which version" prompt
	multi) "$BIN" <<<2 >"$TMP/$v.log" 2>&1 & ;;
	esac
	PID=$!
	for _ in $(seq 50); do
		redis-cli -p "$port" PING >/dev/null 2>&1 && return 0
		sleep 0.1
	done
	cat "$TMP/$v.log" >&2
	die "$v server did not start on port $port"
}

stop_server() { # SIGTERM = graceful shutdown, for both Redis and Redis-k02
	[ -n "$PID" ] || return 0
	kill -TERM "$PID" 2>/dev/null || true
	for _ in $(seq 50); do
		kill -0 "$PID" 2>/dev/null || break
		sleep 0.1
	done
	kill -KILL "$PID" 2>/dev/null || true
	wait "$PID" 2>/dev/null || true
	PID=""
}

cleanup() {
	stop_server
	rm -rf "$TMP"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

# bench <port> <redis-benchmark args...>: prints one CSV row per benchmarked command:
#   "name","rps","avg","min","p50","p95","p99","max"   (latencies in ms)
# --csv must come BEFORE the args: everything after a custom command is part of that command.
bench() {
	local port=$1 out
	shift
	for _ in 1 2; do
		out=$(redis-benchmark -p "$port" --csv "$@" 2>/dev/null | tr -d '\r' | grep -v '^"test"' || true)
		# redis-benchmark computes requests/sec from the wall clock, which WSL2 sometimes
		# steps backwards, so a run can report a negative value: run it again once.
		grep -q '^"[^"]*","-' <<<"$out" || break
		echo "   got a negative requests/sec, running it again" >&2
	done
	echo "$out"
}

# tables <single.csv> <multi.csv> <real.csv>: for every benchmarked command (SET, GET, ...)
# a "### <CMD> requests" heading and a markdown table with one row per server and one
# column per metric. In each column the better of single-thread vs multithread is bold;
# real Redis is only a reference row, so it is never bold.
tables() {
	awk -F, '
		function commas(x,   s, r) {
			s = sprintf("%.0f", x); r = ""
			while (length(s) > 3) { r = "," substr(s, length(s) - 2) r; s = substr(s, 1, length(s) - 3) }
			return s r
		}
		# best returns the better of single (1) and multi (2). higher = 1 if a higher value is better.
		function best(a, name, higher,   x, y) {
			x = a[name, 1]; y = a[name, 2]
			if (x == "") return y
			if (y == "") return x
			return (higher ? x + 0 >= y + 0 : x + 0 <= y + 0) ? x : y
		}
		# cell formats one value of server i: commas for throughput, bold if it is the best.
		function cell(x, b, i, isrps,   s) {
			if (x == "") return "n/a"
			s = isrps ? commas(x) : x
			return (i <= 2 && x + 0 == b + 0) ? "**" s "**" : s
		}
		{ gsub(/"/, "") }
		$1 == "test" || NF < 8 { next }
		!($1 in seen) { seen[$1] = 1; order[++n] = $1 }
		{ rps[$1, v] = $2; avg[$1, v] = $3; p50[$1, v] = $5; p99[$1, v] = $7 }
		END {
			label[1] = "Single-thread"; label[2] = "Multithread"; label[3] = "Real Redis (reference)"
			for (k = 1; k <= n; k++) {
				name = order[k]; split(name, w, " ")
				print "### " w[1] " requests"
				print ""
				print "| | Throughput (req/s) | p50 latency (ms) | Mean latency (ms) | p99 latency (ms) |"
				print "|---|---:|---:|---:|---:|"
				brps = best(rps, name, 1); bp50 = best(p50, name, 0); bavg = best(avg, name, 0); bp99 = best(p99, name, 0)
				for (i = 1; i <= 3; i++) {
					print "| " label[i] " | " cell(rps[name, i], brps, i, 1) " | " cell(p50[name, i], bp50, i, 0) \
						" | " cell(avg[name, i], bavg, i, 0) " | " cell(p99[name, i], bp99, i, 0) " |"
				}
				print ""
			}
		}' v=1 "$1" v=2 "$2" v=3 "$3"
}

# run_test <title> <description> <redis-benchmark args...>
# Runs the benchmark once on each server and appends its section to the report.
run_test() {
	local title=$1 desc=$2 v port
	shift 2
	TEST_NO=$((TEST_NO + 1))
	echo ">> Test $TEST_NO: $title" >&2
	for v in "${VERSIONS[@]}"; do
		port=$(port_of "$v")
		echo "   $v (port $port)" >&2
		start_server "$v"
		if ((${#PREFILL[@]})); then
			redis-benchmark -p "$port" -q "${PREFILL[@]}" >/dev/null 2>&1 || echo "   prefill failed" >&2
		fi
		bench "$port" "$@" >"$TMP/t$TEST_NO-$v.csv"
		stop_server
	done
	{
		echo "## $TEST_NO. $title"
		echo
		echo "$desc"
		echo
		echo '```'
		if ((${#PREFILL[@]})); then echo "redis-benchmark -p PORT ${PREFILL[*]}   # prefill"; fi
		echo "redis-benchmark -p PORT $*"
		echo '```'
		echo
		tables "$TMP/t$TEST_NO-single.csv" "$TMP/t$TEST_NO-multi.csv" "$TMP/t$TEST_NO-real.csv"
	} >>"$TMP/body.md"
	PREFILL=()
}

# --- preflight ---

for tool in go redis-server redis-cli redis-benchmark ss; do
	command -v "$tool" >/dev/null || die "$tool not found on PATH"
done
for port in "$OUR_PORT" "$REDIS_PORT"; do
	if ss -Hltn "( sport = :$port )" | grep -q .; then
		die "port $port is already in use - stop the server running there first (this script starts its own)"
	fi
done
MAX_KEYS=$(grep -oE 'MaxKeys *= *[0-9]+' "$CONFIG" | grep -oE '[0-9]+$' || echo "?")
if [[ $MAX_KEYS =~ ^[0-9]+$ ]] && ((MAX_KEYS < 200000)); then
	echo "warning: config.MaxKeys = $MAX_KEYS, but the tests use up to 100,000 keys, so Redis-k02 will" >&2
	echo "         evict keys - work real Redis doesn't do. Set it to e.g. 10000000 while benchmarking." >&2
fi

echo ">> Building Redis-k02" >&2
(cd "$ROOT" && go build -o "$BIN" .)

# --- tests ---

run_test 'Latency: 1 client' \
	'200,000 SET, then 200,000 GET, from **1 client**, on random keys out of 100,000. Only one request is in flight at a time, so requests/sec is just 1 / latency: compare the latency columns.' \
	-t set,get -n 200000 -r 100000 -c 1

run_test 'Normal load: 50 clients' \
	'1,000,000 SET, then 1,000,000 GET, from **50 clients** (one benchmark thread, the redis-benchmark default), on random keys out of 100,000.' \
	-t set,get -n 1000000 -r 100000 -c 50

run_test 'High concurrency: 200 clients' \
	'2,000,000 SET, then 2,000,000 GET, from **200 clients** spread over 4 benchmark threads, on random keys out of 100,000.' \
	-t set,get -n 2000000 -r 100000 -c 200 --threads 4

run_test 'Hot key: 200 clients on 1 key' \
	'1,000,000 SET, then 1,000,000 GET, from **200 clients** over 4 benchmark threads, all on **the same key** (no `-r`). In the multithread server every request goes to the same worker.' \
	-t set,get -n 1000000 -c 200 --threads 4

run_test 'Pipelining: 50 clients, 16 commands per round trip' \
	'500,000 SET, then 500,000 GET, from **50 clients**, each sending **16 commands at once** (`-P 16`) before reading the replies, on random keys out of 100,000. redis-benchmark times each batch only until its first reply, so here compare throughput, not latency.' \
	-t set,get -n 500000 -r 100000 -c 50 -P 16

PREFILL=(-n 500000 -r 1000 -c 50 SADD set:__rand_int__ m:__rand_int__)
run_test 'Heavy command: SMEMBERS on ~400-member sets' \
	'Prefill: 500,000 SADD from 50 clients into 1,000 sets, with members picked at random out of 1,000, so each set ends up with ~400 members. Then 200,000 SMEMBERS from **200 clients** over 4 benchmark threads, on random sets. Each reply is ~8 KB, so every request costs the server real CPU.' \
	-n 200000 -r 1000 -c 200 --threads 4 SMEMBERS set:__rand_int__

# --- report ---

NM=$(grep -oE 'io handlers=[0-9]+, workers=[0-9]+' "$TMP/multi.log" | head -n 1 || true)
CPU=$(grep -m1 'model name' /proc/cpuinfo | cut -d: -f2 | sed 's/^ *//')
REDIS_VERSION=$(redis-server --version | grep -oE 'v=[^ ]+' | cut -d= -f2)
{
	cat <<EOF
# Benchmark: real Redis vs Redis-k02 single-thread vs Redis-k02 multithread

Generated by \`benchmark/run.sh\` on $(date '+%Y-%m-%d %H:%M') (took $((SECONDS / 60)) min).

- **Machine:** $CPU, $(nproc) CPUs, $(uname -sr)
- **Real Redis $REDIS_VERSION** on port $REDIS_PORT, started as \`redis-server --port $REDIS_PORT --save "" --appendonly no\` (no persistence, like Redis-k02).
- **Redis-k02** on port $OUR_PORT: single-thread, and multithread (${NM:-io handlers/workers unknown}). MaxKeys = $MAX_KEYS.
- \`PORT\` in the commands is $REDIS_PORT for real Redis and $OUR_PORT for Redis-k02.
- Each test ran **once** per server, each time on a freshly started server (empty keyspace), so expect a few % noise.
- redis-benchmark ran on the same machine, so it competes with the server for CPU.
- Throughput is requests per second; latencies are in ms.
- **Bold** = the better of single-thread vs multithread in each column (higher throughput, lower latency). Real Redis is a reference point, not a competitor, so it is never bold.

EOF
	cat "$TMP/body.md"
} >"$OUT"
echo ">> Wrote $OUT" >&2
