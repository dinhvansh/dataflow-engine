#!/usr/bin/env bash
# Opt-in LOCAL-only 7m raw managed-incremental benchmark. Does not contact ERP.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
cd "$root"
[[ "${DATAFLOW_QA_ALLOW_7M:-}" == "YES_I_ACCEPT_DISPOSABLE_QA_SEEDING" ]] || {
 echo "Refusing resource-intensive 7m fixture without DATAFLOW_QA_ALLOW_7M=YES_I_ACCEPT_DISPOSABLE_QA_SEEDING" >&2
 exit 2
}
[[ -f .env.qa.local ]] || { echo "Missing ignored local QA credentials" >&2; exit 2; }
"$here/verify-isolation.sh"
# The default remains 16 GiB. Owner-local Windows QA can explicitly opt in
# to a 2 GiB C: floor; this exception is recorded with the result.
disk_guard_bytes=${DATAFLOW_QA_MIN_FREE_BYTES:-17179869184}
[[ "$disk_guard_bytes" =~ ^[0-9]+$ ]] || { echo 'DATAFLOW_QA_MIN_FREE_BYTES must be an integer' >&2; exit 2; }
if (( disk_guard_bytes < 17179869184 )); then
 [[ "$disk_guard_bytes" == 2147483648 && "${DATAFLOW_QA_ALLOW_LOW_DISK:-}" == YES_I_ACCEPT_NONCERTIFYING_LOW_DISK ]] || {
  echo 'Low-disk QA requires the explicit 2 GiB owner opt-in' >&2; exit 2;
 }
 command -v powershell.exe >/dev/null 2>&1 || { echo 'Windows C: watchdog unavailable' >&2; exit 2; }
 echo 'OWNER LOW-DISK QA: C: floor 2 GiB; this run is not production certification' >&2
fi
DATAFLOW_QA_MIN_FREE_BYTES="$disk_guard_bytes" "$here/check-disk-capacity.sh" "$root"
for c in dataflow-localqa-mssql dataflow-localqa-postgres dataflow-localqa-clickhouse; do
 [[ "$(docker inspect -f '{{index .Config.Labels "com.dataflow.owner"}}' "$c")" == "dataflow-localqa" ]] || exit 1
 [[ "$(docker inspect -f '{{index .Config.Labels "com.dataflow.role"}}' "$c")" == "local-qa" ]] || exit 1
done
[[ "$(docker network inspect dataflow-localqa-net -f '{{index .Labels "com.dataflow.owner"}}')" == "dataflow-localqa" ]] || {
 echo "QA Docker network owner mismatch" >&2; exit 1;
}
watch_pid=
if (( disk_guard_bytes < 17179869184 )); then
 c_free_bytes(){ powershell.exe -NoProfile -Command '(Get-Volume -DriveLetter C).SizeRemaining' | tr -d '\r[:space:]'; }
 c_free=$(c_free_bytes)
 [[ "$c_free" =~ ^[0-9]+$ ]] && (( c_free > 2147483648 )) || { echo 'C: is at or below the 2 GiB floor' >&2; exit 1; }
 watch_c(){
  while :; do
   c_free=$(c_free_bytes || true)
   if [[ ! "$c_free" =~ ^[0-9]+$ ]] || (( c_free <= 2362232012 )); then
    echo 'C: approaching the 2 GiB floor; stopping only owned dataflow-localqa QA containers' >&2
    docker stop dataflow-localqa-mssql dataflow-localqa-postgres dataflow-localqa-clickhouse >/dev/null || true
    return
   fi
   sleep 15
  done
 }
 watch_c & watch_pid=$!
fi
stop_watch(){ if [[ -n "$watch_pid" ]]; then kill "$watch_pid" 2>/dev/null || true; wait "$watch_pid" 2>/dev/null || true; fi; }
trap stop_watch EXIT
set -a; source .env.qa.local; set +a
mkdir -p qa-results
umask 077
out="$root/qa-results/core-seven-million-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$out"
echo "SEEDING *ONLY* SQL Server DataFlowFixtureQA.Erp.PartTranScale7M on owned disposable QA network"
sql_mount="$here/sql"
if command -v cygpath >/dev/null 2>&1; then sql_mount=$(cygpath -w "$sql_mount"); fi
MSYS_NO_PATHCONV=1 docker run --rm --network dataflow-localqa-net \
 -v "$sql_mount:/qa/sql:ro" -e MSSQL_SA_PASSWORD \
 --entrypoint /bin/bash mcr.microsoft.com/mssql/server:2022-latest \
 -c 'exec /opt/mssql-tools18/bin/sqlcmd -C -b -S mssql -U sa -P "$MSSQL_SA_PASSWORD" -d DataFlowFixtureQA -i /qa/sql/006_scale_7m.sql' \
 >"$out/seed_result.log"
export DATAFLOW_BENCHMARK_7M=true
export DATAFLOW_QA_LOCAL_OWNER_VERIFIED=true
export DATAFLOW_BENCHMARK_QA_CONFIRM=RUN_SEVEN_MILLION_DISPOSABLE_ROWS
export DATAFLOW_INTEGRATION_PG_ISOLATED=true
export DATAFLOW_INTEGRATION_MSSQL_ISOLATED=true
export DATAFLOW_INTEGRATION_CH_ISOLATED=true
export DATAFLOW_INTEGRATION_POSTGRES_DSN="postgres://dataflow_qa:$POSTGRES_PASSWORD@127.0.0.1:15433/dataflow_qa?sslmode=disable"
export DATAFLOW_INTEGRATION_SQLSERVER_DSN="sqlserver://qa_erp_reader:$QA_READER_PASSWORD@127.0.0.1:11433?database=DataFlowFixtureQA&encrypt=true&TrustServerCertificate=true"
export DATAFLOW_INTEGRATION_CH_URL=http://127.0.0.1:18123
export DATAFLOW_INTEGRATION_CH_USER=qa_admin
export DATAFLOW_INTEGRATION_CH_PASSWORD="$CLICKHOUSE_ADMIN_PASSWORD"
resource="$out/docker_stats.csv"
printf 'utc_time,container,cpu_percent,memory_usage,block_io\n' >"$resource"
monitor(){
 while :; do
  docker stats --no-stream \
   --format '{{.Name}},{{.CPUPerc}},{{.MemUsage}},{{.BlockIO}}' \
   dataflow-localqa-mssql dataflow-localqa-postgres dataflow-localqa-clickhouse \
   | awk -v ts="$(date -u +%FT%TZ)" '{print ts "," $0}' >>"$resource" || true
  sleep 3
 done
}
monitor & monitor_pid=$!
stop_monitor(){ kill "$monitor_pid" 2>/dev/null || true; wait "$monitor_pid" 2>/dev/null || true; }
trap 'stop_monitor; stop_watch' EXIT
echo "RUNNING actual SELECT-only SQL Server 7m → Go queue → ClickHouse raw → PostgreSQL checkpoint"
go test ./cmd/dataflow -run '^TestCoreRun7mRealManagedBenchmarkIntegration$' \
 -count=1 -timeout=40m -v | tee "$out/actual_benchmark.log"
grep -q 'QA_REAL_7M_BENCH_PASS' "$out/actual_benchmark.log" && grep -q 'QA_REAL_7M_INCREMENTAL_REPLAY_PASS' "$out/actual_benchmark.log" || {
 echo "Benchmark was skipped or failed; cannot claim 7m PASS" >&2; exit 1;
}
stop_monitor
stop_watch
trap - EXIT
# Create a machine-readable evidence summary ONLY after both actual marker lines.
# Derive values from the captured log, never from assumed benchmark rates.
python3 - "$out" <<'PY'
import json, pathlib, re, subprocess, sys, datetime, platform
d=pathlib.Path(sys.argv[1])
s=(d/"actual_benchmark.log").read_text()
first=re.search(r'QA_REAL_7M_BENCH_PASS[^\n]*rows=(\d+)[^\n]*elapsed_seconds=([\d.]+)[^\n]*rows_per_second=([\d.]+)[^\n]*go_heap_alloc_bytes=(\d+)',s)
second=re.search(r'QA_REAL_7M_INCREMENTAL_REPLAY_PASS[^\n]*rows_written=(\d+)[^\n]*checkpoint_generation=(\d+)[^\n]*replay_elapsed_seconds=([\d.]+)[^\n]*original_rows_unchanged=(\d+)',s)
if not first or not second: raise SystemExit("refusing incomplete/unmeasured benchmark results")
rows,elapsed,rps,heap=first.groups()
newrows,gen,seconds,preserved=second.groups()
if (int(rows),int(newrows),int(gen),int(preserved)) != (7000000,0,2,7000000):
 raise SystemExit("invariant mismatch: cannot claim benchmark success")
stats=d/"docker_stats.csv"
if not stats.exists() or len(stats.read_text().splitlines())<2:
 raise SystemExit("Docker resource measurements missing")
result={
 "gate":"CORE-RUN-7","acceptance":"REAL OWNER-QA HARDWARE RESULT; NOT PRODUCTION CERTIFICATION",
 "source":"DataFlowFixtureQA.Erp.PartTranScale7M SELECT-only",
 "destination":"ClickHouse scratch.dataflow_raw_events","state":"PostgreSQL checkpoint",
 "timestamp_utc":datetime.datetime.now(datetime.timezone.utc).isoformat(),
 "hostname":platform.node(),"kernel":platform.release(),"architecture":platform.machine(),
 "source_rows":int(rows),"initial_elapsed_seconds":float(elapsed),
 "initial_rows_per_second":float(rps),"go_heap_alloc_bytes_at_end":int(heap),
 "incremental_replay_rows_written":int(newrows),"replay_elapsed_seconds":float(seconds),
 "checkpoint_generation":int(gen),"original_rows_remain":int(preserved),
 "resource_samples":len(stats.read_text().splitlines())-1,
 "evidence_files":["seed_result.log","actual_benchmark.log","docker_stats.csv"],
 "caveats":["hosted CI is not a 7m benchmark","no production data/ERP writes","not a full 7m DQ/dbt mart materialization"]
}
(d/"benchmark-result.json").write_text(json.dumps(result,indent=2)+"\n")
print(json.dumps({"gate":result["gate"],"status":"measured QA PASS","rows":int(rows),"rows_per_second":float(rps),"replay_rows":int(newrows),"evidence_dir":str(d)}))
PY
echo "Exact measured 7m rows/s and Docker resource data (NO synthetic/claimed results): $out"
