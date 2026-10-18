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
docker compose --project-name dataflow-localqa --env-file .env.qa.local \
  -f deploy/local-qa/compose.qa.yml --profile operator up -d --build --wait dbt-worker api web
for url in http://127.0.0.1:18080/healthz http://127.0.0.1:15173/healthz; do
  curl --fail --silent --show-error "$url" | grep -qx ok
done
export DATAFLOW_BROWSER_BASE_URL=http://127.0.0.1:15173
export DATAFLOW_LIVE_API_URL=http://127.0.0.1:18080
echo "CORE-RUN-5 full-stack live browser test: real API, SQL Server, PostgreSQL, ClickHouse, dbt."
echo "Evidence: qa-results/core-run5/"
python -m pytest qa/browser/test_core_live.py -v -s
