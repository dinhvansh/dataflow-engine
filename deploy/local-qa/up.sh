#!/usr/bin/env bash
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$here/../.." && pwd)"
cd "$root"
bash "$here/create-env.sh"
bash "$here/preflight.sh"
compose=(docker compose --project-name dataflow-localqa --env-file .env.qa.local -f deploy/local-qa/compose.qa.yml)
# Start the long-lived data services first. Temporary initializer runs MUST
# NOT use --abort-on-container-exit: that would stop their dependencies.
"${compose[@]}" up -d --wait mssql postgres clickhouse
"${compose[@]}" run --rm --no-deps mssql-init
"${compose[@]}" run --rm --no-deps clickhouse-init
# Verify the services are still up and the exact disposable DB is read-only.
bash "$here/verify-isolation.sh"
echo 'QA databases initialized and healthy (no volume reset)'
