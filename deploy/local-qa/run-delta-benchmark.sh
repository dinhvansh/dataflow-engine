#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd); root=$(cd "$here/../.." && pwd)
"$here/verify-isolation.sh"
# One-million-row copy-on-write benchmarks require an explicit free-space gate.
"$here/check-disk-capacity.sh" "$root"
cd "$root"
if [[ ! -f .env.qa.local ]]; then echo "QA-local secrets missing" >&2; exit 1; fi
set -a; source .env.qa.local; set +a
# Prevent both accidental remote access and unowned container benchmark runs.
for c in dataflow-localqa-clickhouse dataflow-localqa-postgres dataflow-localqa-mssql; do
  [[ "$(docker inspect -f '{{index .Config.Labels "com.dataflow.owner"}}' "$c")" == "dataflow-localqa" ]] || exit 1
  [[ "$(docker inspect -f '{{index .Config.Labels "com.dataflow.role"}}' "$c")" == "local-qa" ]] || exit 1
done
run_id=$(date -u +%Y%m%dT%H%M%SZ)
out="$root/qa-results/delta-$run_id"
umask 077
mkdir -p "$out"
export DATAFLOW_BENCHMARK_QA=true
export DATAFLOW_INTEGRATION_CH_ISOLATED=true
export DATAFLOW_INTEGRATION_CH_URL=http://127.0.0.1:18123
export DATAFLOW_INTEGRATION_CH_USER=qa_admin
export DATAFLOW_INTEGRATION_CH_PASSWORD="$CLICKHOUSE_ADMIN_PASSWORD"
resource_csv="$out/docker_resources.csv"
printf 'timestamp,container,cpu_percent,memory_usage,block_io\n' >"$resource_csv"
monitor_resources(){
  while :; do
    docker stats --no-stream --format '{{.Name}},{{.CPUPerc}},{{.MemUsage}},{{.BlockIO}}' \
      dataflow-localqa-clickhouse dataflow-localqa-postgres \
      | awk -v ts="$(date -u +%FT%TZ)" '{print ts "," $0}' >>"$resource_csv" || return
    sleep 1
  done
}
monitor_resources & monitor_pid=$!
cleanup_monitor(){ kill "$monitor_pid" 2>/dev/null || true; wait "$monitor_pid" 2>/dev/null || true; }
trap cleanup_monitor EXIT
ch_events(){
  curl --fail --silent --show-error --user "qa_admin:$CLICKHOUSE_ADMIN_PASSWORD" \
    --data-binary "SELECT event,value FROM system.events WHERE event IN ('ReadBufferFromFileDescriptorReadBytes','WriteBufferFromFileDescriptorWriteBytes') FORMAT CSV" \
    http://127.0.0.1:18123/
}
ch_events >"$out/clickhouse_io_before.csv"
# Do not log env vars, tokens, password values, or ERP connection strings.
for baseline in 100000 1000000; do
  export DATAFLOW_BENCHMARK_BASELINE_ROWS="$baseline"
  export DATAFLOW_BENCHMARK_RESULT_FILE="$out/delta_samples_${baseline}.jsonl"
  go test ./internal/safetable -run '^TestDeltaPerformanceQAIntegration$' -count=1 -timeout=30m -v \
    | tee "$out/go_test_${baseline}.log"
done
ch_events >"$out/clickhouse_io_after.csv"
cleanup_monitor
trap - EXIT
python3 - "$resource_csv" "$out/resource_summary.json" <<'PY'
import csv,json,re,sys
scale={'B':1,'KiB':1024,'MiB':1024**2,'GiB':1024**3}
peak_cpu={};peak_mem={}
with open(sys.argv[1],newline='') as f:
  for row in csv.DictReader(f):
    name=row['container']; cpu=float(row['cpu_percent'].rstrip('%'))
    value,unit=re.match(r'([0-9.]+)([KMG]iB|B)',row['memory_usage'].split('/')[0].strip()).groups()
    peak_cpu[name]=max(peak_cpu.get(name,0),cpu)
    peak_mem[name]=max(peak_mem.get(name,0),float(value)*scale[unit])
with open(sys.argv[2],'w') as f: json.dump({'peak_cpu_percent':peak_cpu,'peak_memory_bytes':peak_mem},f,indent=2)
PY
echo "Saved 18 measured scratch-only samples plus publish, rollback, CPU, RAM and ClickHouse I/O under $out"
