#!/usr/bin/env bash
# Explicit owner consent is mandatory to delete only disposable QA volumes.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
[[ "${DATAFLOW_QA_ALLOW_DISPOSABLE_RESET:-}" == I_APPROVE_OWNED_QA_VOLUMES ]] ||
{ echo "BLOCKED: explicit disposable QA volume reset permission required" >&2; exit 2; }
"$here/verify-reset-scope.sh"
cd "$root"
docker compose --project-name dataflow-localqa --env-file .env.qa.local \
 -f deploy/local-qa/compose.qa.yml --profile operator down --volumes
echo "PASS: reset only verified owned dataflow-localqa Docker Compose resources"
