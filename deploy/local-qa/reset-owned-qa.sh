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
remaining_containers=$(docker ps -aq --filter label=com.docker.compose.project=dataflow-localqa)
remaining_volumes=$(docker volume ls --format '{{.Name}}' --filter label=com.docker.compose.project=dataflow-localqa)
remaining_networks=$(docker network ls --format '{{.Name}}' --filter label=com.docker.compose.project=dataflow-localqa)
if [[ -n "$remaining_containers" || -n "$remaining_volumes" || -n "$remaining_networks" ]]; then
 echo "BLOCKED: QA reset incomplete; disposable project resources remain" >&2
 echo "Containers: $remaining_containers; volumes: $remaining_volumes; networks: $remaining_networks" >&2
 exit 1
fi
echo "PASS: reset only verified owned dataflow-localqa Docker Compose resources"
