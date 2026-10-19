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

# Compose down --volumes must never remove an unknown volume belonging to the
# QA project (including a stale volume from a different config revision).
# Check the ENTIRE project inventory, not only three expected names.
for name in $(docker volume ls --format '{{.Name}}' --filter label=com.docker.compose.project=dataflow-localqa); do
 case "$name" in
  dataflow-localqa_mssql-data|dataflow-localqa_pg-data|dataflow-localqa_ch-data) ;;
  *) echo "BLOCKED: unexpected project QA volume $name; owner review required" >&2; exit 2;;
 esac
done

# Likewise, fail closed on an additional project-labeled network, which may
# host workloads outside the exact reviewed dataflow-localqa-net topology.
for name in $(docker network ls --format '{{.Name}}' --filter label=com.docker.compose.project=dataflow-localqa); do
 [[ "$name" == dataflow-localqa-net ]] ||
 { echo "BLOCKED: unexpected project QA network $name; owner review required" >&2; exit 2; }
done

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
# compose.qa.yml declares an explicit network name, NOT project_default.
if docker network inspect dataflow-localqa-net >/dev/null 2>&1; then
 project=$(docker network inspect -f '{{index .Labels "com.docker.compose.project"}}' dataflow-localqa-net)
 owner=$(docker network inspect -f '{{index .Labels "com.dataflow.owner"}}' dataflow-localqa-net)
 role=$(docker network inspect -f '{{index .Labels "com.dataflow.role"}}' dataflow-localqa-net)
 [[ "$project" == dataflow-localqa && "$owner" == dataflow-localqa && "$role" == local-qa ]] ||
 { echo "BLOCKED: unknown QA network ownership" >&2; exit 2; }
 # Every connected container must belong to the verified disposable project.
 for cid in $(docker network inspect -f '{{range $id,$container := .Containers}}{{$id}} {{end}}' dataflow-localqa-net); do
  project=$(docker inspect -f '{{index .Config.Labels "com.docker.compose.project"}}' "$cid")
  [[ "$project" == dataflow-localqa ]] ||
  { echo "BLOCKED: foreign container connected to QA network: $cid" >&2; exit 2; }
 done
fi
echo "PASS: only verified disposable dataflow-localqa project resources targeted"
