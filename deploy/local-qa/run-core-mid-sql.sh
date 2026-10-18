#!/usr/bin/env bash
# Explicit opt-in: the only supported target is a locally owned disposable QA stack.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
cd "$root"
[[ "${DATAFLOW_LIVE_QA_OPT_IN:-}" == "true" ]] || { echo "Set DATAFLOW_LIVE_QA_OPT_IN=true explicitly" >&2; exit 1; }
test -f "$root/.env.qa.local" || { echo "Run deploy/local-qa/up.sh first" >&2; exit 1; }
"$here/verify-isolation.sh"
set -a
source "$root/.env.qa.local"
set +a
# Public synthetic token only; browser traces must NEVER contain a private bearer.
export DATAFLOW_LIVE_API_TOKEN="synthetic-live-browser-token-not-a-secret"
export DATAFLOW_CORE_MID_SQL_OPT_IN="true"
# Buildx Bake forbids reading /etc/ssl directly. Copy ONLY the public CA trust
# bundle into a one-use ignored file inside this verified QA workspace.
# Preserve explicit operator trust roots; never disable certificate validation.
trust_source="${CODEX_PROXY_CERT:-/etc/ssl/certs/ca-certificates.crt}"
[[ -r "$trust_source" ]] || { echo "Trusted CA bundle is unreadable" >&2; exit 1; }
qa_build_ca=$(mktemp "$here/.qa-build-ca.XXXXXXXX")
trap 'rm -f "$qa_build_ca"' EXIT
cp "$trust_source" "$qa_build_ca"
chmod 600 "$qa_build_ca"
export CODEX_PROXY_CERT="$qa_build_ca"
docker compose --project-name dataflow-localqa --env-file .env.qa.local \
  -f deploy/local-qa/compose.qa.yml --profile operator up -d --build --wait dbt-worker api web
for url in http://127.0.0.1:18080/healthz http://127.0.0.1:15173/healthz; do
  curl --fail --silent --show-error "$url" | grep -qx ok
done
export DATAFLOW_BROWSER_BASE_URL=http://127.0.0.1:15173
export DATAFLOW_LIVE_API_URL=http://127.0.0.1:18080
echo "CORE-RUN-6B-3 full-stack live browser test: real API, SQL Server, PostgreSQL, ClickHouse, dbt."
echo "Evidence: qa-results/core-run6b3/"
python -m pytest qa/browser/test_core_mid_sql_live.py -v -s
