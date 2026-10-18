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
# A large source + raw events must not exhaust or delete unrelated volumes.
DATAFLOW_QA_MIN_FREE_BYTES=17179869184 "$here/check-disk-capacity.sh" "$root"
for c in dataflow-localqa-mssql dataflow-localqa-postgres dataflow-localqa-clickhouse; do
 [[ "$(docker inspect -f '{{index .Config.Labels "com.dataflow.owner"}}' "$c")" == "dataflow-localqa" ]] || exit 1
 [[ "$(docker inspect -f '{{index .Config.Labels "com.dataflow.role"}}' "$c")" == "local-qa" ]] || exit 1
done
[[ "$(docker network inspect dataflow-localqa-net -f '{{index .Labels "com.dataflow.owner"}}')" == "dataflow-localqa" ]] || {
 echo "QA Docker network owner mismatch" >&2; exit 1;
}
set -a; source .env.qa.local; set +a
mkdir -p qa-results
umask 077
out="$root/qa-results/core-seven-million-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$out"
echo "SEEDING *ONLY* SQL Server DataFlowFixtureQA.Erp.PartTranScale7M on owned disposable QA network"
docker run --rm --network dataflow-localqa-net \
 -v "$here/sql:/qa/sql:ro" -e MSSQL_SA_PASSWORD \
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
trap stop_monitor EXIT
echo "RUNNING actual SELECT-only SQL Server 7m → Go queue → ClickHouse raw → PostgreSQL checkpoint"
go test ./cmd/dataflow -run '^TestCoreRun7mRealManagedBenchmarkIntegration$' \
 -count=1 -timeout=40m -v | tee "$out/actual_benchmark.log"
grep -q 'QA_REAL_7M_BENCH_PASS' "$out/actual_benchmark.log" || {
 echo "Benchmark was skipped or failed; cannot claim 7m PASS" >&2; exit 1;
}
stop_monitor
trap - EXIT
echo "Exact measured 7m rows/s and Docker resource data (NO synthetic/claimed results): $out"
