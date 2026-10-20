#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
cd "$root"
"$root/deploy/local-qa/check-disk-capacity.sh" "$root"
[[ -f .env.qa.local ]] || { echo 'Missing isolated .env.qa.local' >&2; exit 1; }
for forbidden in SRVERPTEST ERPDBTEST; do
 if grep -qi "$forbidden" .env.qa.local; then
  echo "Forbidden QA host/database: $forbidden" >&2; exit 1
 fi
done

# Existing containers are permitted ONLY when owned by this disposable project
# and exposing exactly the expected container port on loopback. This allows
# up.sh -> setup-login.sh -> start-sync-setup.sh -> preflight.sh to work
# without falsely claiming that the stack's own ports are port collisions.
declare -A ports=(
 [11433]="dataflow-localqa-mssql:1433"
 [15433]="dataflow-localqa-postgres:5432"
 [18123]="dataflow-localqa-clickhouse:8123"
 [18080]="dataflow-localqa-api:8080"
 [15173]="dataflow-localqa-web:8080"
)
if docker ps -a --format '{{.Names}}' | grep -E '^dataflow-localqa-' | grep -q .; then
 while IFS= read -r id; do
  [[ -n "$id" ]] || continue
  owner=$(docker inspect -f '{{index .Config.Labels "com.dataflow.owner"}} {{index .Config.Labels "com.dataflow.role"}}' "$id")
  [[ "$owner" == "dataflow-localqa local-qa" ]] || {
   echo "Refusing unowned QA-named container $id" >&2;exit 1;
  }
 done < <(docker ps -a --filter 'name=^/dataflow-localqa-' --format '{{.ID}}')
fi

for port in 11433 15433 18123 18080 15173; do
 listen=false
 if command -v ss >/dev/null 2>&1; then
  if ss -ltn "sport = :$port" | tail -n +2 | grep -q .; then listen=true; fi
 elif command -v powershell.exe >/dev/null 2>&1; then
  if ! powershell.exe -NoProfile -Command "if (Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue) { exit 1 }"; then listen=true; fi
 else
  echo 'BLOCKED: no supported listening-port checker' >&2;exit 1
 fi
 if [[ "$listen" != true ]]; then continue; fi
 spec="${ports[$port]}"
 name="${spec%:*}"
 container_port="${spec#*:}"
 label=$(docker inspect -f '{{index .Config.Labels "com.dataflow.owner"}} {{index .Config.Labels "com.dataflow.role"}}' "$name" 2>/dev/null || true)
 running=$(docker inspect -f '{{.State.Running}}' "$name" 2>/dev/null || true)
 # Port inspection is exact; do not trust names/labels alone.
 binding=$(docker inspect -f "{{range index .NetworkSettings.Ports \"$container_port/tcp\"}}{{.HostIp}}:{{.HostPort}}{{end}}" "$name" 2>/dev/null || true)
 if [[ "$label" != "dataflow-localqa local-qa" || "$running" != true || "$binding" != "127.0.0.1:$port" ]]; then
  echo "Port $port is occupied by an unverified service (expected owned $name on loopback)" >&2;exit 1
 fi
done
echo 'QA preflight PASS (including safe reuse of owned QA container ports)'
