#!/usr/bin/env bash
# Start approved disposable QA Sync Setup UI. No reset/teardown/ERP access.
set -euo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root"
[[ "${DATAFLOW_LIVE_QA_OPT_IN:-}" == "true" ]] || {
 echo "Explicit DATAFLOW_LIVE_QA_OPT_IN=true required" >&2; exit 1;
}
[[ -f .env.qa.local ]] || { echo "Missing ignored QA env; run deploy/local-qa/up.sh first" >&2; exit 1; }
bash deploy/local-qa/verify-isolation.sh
bash deploy/local-qa/preflight.sh
set -a
source .env.qa.local
set +a
[[ "${DATAFLOW_ENABLE_RUNTIME_CONNECTIONS_LAB:-false}" == "true" ]] || {
 echo "Set DATAFLOW_ENABLE_RUNTIME_CONNECTIONS_LAB=true in ignored QA env" >&2; exit 1;
}
[[ -n "${DATAFLOW_CONNECTION_ADMIN_TOKEN:-}" && -n "${DATAFLOW_QA_CONNECTION_KEY_B64:-}" ]] || {
 echo "Missing private lab admin token or encryption key" >&2; exit 1;
}
# Use generated private QA bearer; never print it or embed in browser code.
export DATAFLOW_LIVE_API_TOKEN="${DATAFLOW_API_TOKEN:?private QA API token missing}"
compose=(docker compose --project-name dataflow-localqa --env-file .env.qa.local -f deploy/local-qa/compose.qa.yml)
# Preserved QA PostgreSQL volumes do not rerun init scripts.
# Apply additive migrations; no reset, no unrelated Docker mutation.
for migration in 031_connection_runtime_profiles.sql 032_sync_stream_selected_columns.sql 033_qa_stream_modes_destinations.sql 034_source_key_and_bootstrap_cursor.sql; do
 "${compose[@]}" exec -T postgres psql -v ON_ERROR_STOP=1 -U dataflow_qa -d dataflow_qa -f "/docker-entrypoint-initdb.d/$migration" >/dev/null
done
# Trusted CA bundle for reproducible builds, matching existing QA launcher.
trust="${CODEX_PROXY_CERT:-/etc/ssl/certs/ca-certificates.crt}"
[[ -r "$trust" ]] || { echo "Readable CA trust bundle required" >&2; exit 1; }
tmp_ca="$(mktemp "$root/deploy/local-qa/.qa-build-ca.XXXXXXXX")"
trap 'rm -f "$tmp_ca"' EXIT
cp "$trust" "$tmp_ca"; chmod 600 "$tmp_ca"
export CODEX_PROXY_CERT="$tmp_ca"
"${compose[@]}" --profile operator up -d --build --wait dbt-worker api web
for url in http://127.0.0.1:18080/healthz http://127.0.0.1:15173/healthz; do
 curl --fail --silent --show-error "$url" | grep -qx ok
done
echo "QA Sync Setup UI ready: http://127.0.0.1:15173"
echo "Use only DataFlowFixtureQA + ClickHouse scratch. No real ERP endpoint."
