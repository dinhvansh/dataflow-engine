#!/usr/bin/env bash
# Read-only reset guard: never change unknown Docker resources.
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd); cd "$root"
test -f .env.qa.local || { echo "BLOCKED: ignored .env.qa.local missing" >&2; exit 2; }
docker info >/dev/null || { echo "BLOCKED: Docker daemon missing" >&2; exit 2; }
for forbidden in SRVERPTEST ERPDBTEST; do
 if grep -qi "$forbidden" .env.qa.local; then echo "BLOCKED: real ERP identifier in QA env" >&2; exit 2; fi
done
while IFS= read -r cid; do
 [[ -z "$cid" ]] && continue
 owner=$(docker inspect -f '{{index .Config.Labels "com.dataflow.owner"}}' "$cid")
 role=$(docker inspect -f '{{index .Config.Labels "com.dataflow.role"}}' "$cid")
 [[ "$owner" == "dataflow-localqa" && "$role" == "local-qa" ]] ||
 { echo "BLOCKED: unowned project container: $cid" >&2; exit 2; }
done < <(docker ps -aq --filter label=com.docker.compose.project=dataflow-localqa)
for suffix in mssql-data pg-data ch-data; do
 name="dataflow-localqa_$suffix"
 if docker volume inspect "$name" >/dev/null 2>&1; then
  project=$(docker volume inspect -f '{{index .Labels "com.docker.compose.project"}}' "$name")
  logical=$(docker volume inspect -f '{{index .Labels "com.docker.compose.volume"}}' "$name")
  [[ "$project" == dataflow-localqa && "$logical" == "$suffix" ]] ||
  { echo "BLOCKED: unknown owner for QA volume $name" >&2; exit 2; }
  while IFS= read -r cid; do
   [[ -z "$cid" ]] && continue
   project=$(docker inspect -f '{{index .Config.Labels "com.docker.compose.project"}}' "$cid")
   [[ "$project" == dataflow-localqa ]] ||
   { echo "BLOCKED: non-QA container mounts volume $name: $cid" >&2; exit 2; }
  done < <(docker ps -aq --filter "volume=$name")
 fi
done
if docker network inspect dataflow-localqa_default >/dev/null 2>&1; then
 project=$(docker network inspect -f '{{index .Labels "com.docker.compose.project"}}' dataflow-localqa_default)
 [[ "$project" == dataflow-localqa ]] ||
 { echo "BLOCKED: unknown QA network owner" >&2; exit 2; }
fi
echo "PASS: only verified disposable dataflow-localqa project resources targeted"
