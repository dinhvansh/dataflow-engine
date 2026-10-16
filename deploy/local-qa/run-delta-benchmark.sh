#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd); root=$(cd "$here/../.." && pwd)
"$here/verify-isolation.sh"
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
export DATAFLOW_BENCHMARK_RESULT_FILE="$out/delta_samples.jsonl"
# Do not log env vars, tokens, password values, or ERP connection strings.
go test ./internal/safetable -run '^TestDeltaPerformanceQAIntegration$' -count=1 -timeout=15m -v | tee "$out/go_test.log"
echo "Saved 9 independently measured scratch-only samples under $out"
