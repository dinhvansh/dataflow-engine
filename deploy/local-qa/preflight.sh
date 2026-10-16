#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
cd "$root"
for port in 11433 15433 18123 18080 15173; do
  if ss -ltn "sport = :$port" | tail -n +2 | grep -q .; then echo "Port $port is already in use" >&2; exit 1; fi
done
if docker ps -a --format '{{.Names}}' | grep -E '^dataflow-localqa-' | grep -q .; then
  bad=$(docker ps -a --filter 'name=^/dataflow-localqa-' --format '{{.ID}}' | while read -r id; do docker inspect -f '{{index .Config.Labels "com.dataflow.owner"}}' "$id"; done | grep -vc '^dataflow-localqa$' || true)
  [[ "$bad" == 0 ]] || { echo 'Refusing containers without QA ownership label' >&2; exit 1; }
fi
for forbidden in SRVERPTEST ERPDBTEST; do
  if grep -qi "$forbidden" .env.qa.local 2>/dev/null; then echo "Forbidden host/database in QA env: $forbidden" >&2; exit 1; fi
done
echo 'QA preflight PASS'

