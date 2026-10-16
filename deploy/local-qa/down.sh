#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd); root=$(cd "$here/../.." && pwd); cd "$root"
for id in $(docker ps -a --filter label=com.dataflow.owner=dataflow-localqa --format '{{.ID}}'); do
  [[ "$(docker inspect -f '{{index .Config.Labels "com.dataflow.role"}}' "$id")" == local-qa ]] || { echo 'Refusing unlabeled resource' >&2; exit 1; }
done
docker compose --project-name dataflow-localqa --env-file .env.qa.local -f deploy/local-qa/compose.qa.yml down

