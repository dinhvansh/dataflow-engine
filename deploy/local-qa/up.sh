#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd); root=$(cd "$here/../.." && pwd)
"$here/create-env.sh"
"$here/preflight.sh"
cd "$root"
docker compose --project-name dataflow-localqa --env-file .env.qa.local -f deploy/local-qa/compose.qa.yml up -d --wait mssql postgres clickhouse
docker compose --project-name dataflow-localqa --env-file .env.qa.local -f deploy/local-qa/compose.qa.yml up --abort-on-container-exit --exit-code-from mssql-init mssql-init
docker compose --project-name dataflow-localqa --env-file .env.qa.local -f deploy/local-qa/compose.qa.yml up --abort-on-container-exit --exit-code-from clickhouse-init clickhouse-init
echo 'QA databases initialized'

