#!/usr/bin/env bash
# Real Add -> encrypted Save -> SQL Server/CH driver Test -> Bind acceptance.
# Runs only against the explicitly owned disposable dataflow-localqa stack.
set -euo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root"
[[ "${DATAFLOW_LIVE_QA_OPT_IN:-}" == "true" ]] || {
 echo "Explicit QA opt-in required" >&2; exit 1;
}
[[ -f .env.qa.local ]] || { echo "Missing ignored disposable QA env" >&2; exit 1; }
bash deploy/local-qa/verify-isolation.sh
set -a
source .env.qa.local
set +a
export DATAFLOW_RUNTIME_QA_OPERATOR_VERIFIED=true
export DATAFLOW_INTEGRATION_PG_ISOLATED=true
export DATAFLOW_INTEGRATION_MSSQL_ISOLATED=true
export DATAFLOW_INTEGRATION_CH_ISOLATED=true
export DATAFLOW_INTEGRATION_POSTGRES_DSN="postgres://dataflow_qa:${POSTGRES_PASSWORD}@127.0.0.1:15433/dataflow_qa?sslmode=disable"
export DATAFLOW_INTEGRATION_SQLSERVER_DSN="sqlserver://qa_erp_reader:${QA_READER_PASSWORD}@127.0.0.1:11433?database=DataFlowFixtureQA&encrypt=true&TrustServerCertificate=true"
export DATAFLOW_INTEGRATION_CH_URL="http://127.0.0.1:18123"
export DATAFLOW_INTEGRATION_CH_USER="qa_dbt_writer"
export DATAFLOW_INTEGRATION_CH_PASSWORD="${CLICKHOUSE_WRITER_PASSWORD}"
export DATAFLOW_QA_LOCAL_OWNER_VERIFIED=false
echo "Running isolated encrypted Add/Test/Bind real service acceptance (never 7m)"
go test ./cmd/dataflow -run '^TestRuntimeProfilesRealDisposableQA$' -count=1 -timeout=5m
