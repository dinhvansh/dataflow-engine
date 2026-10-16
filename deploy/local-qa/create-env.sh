#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
envfile="$root/.env.qa.local"
if [[ -e "$envfile" ]]; then echo "Using existing ignored $envfile"; exit 0; fi
secret(){ openssl rand -base64 36 | tr -dc 'A-Za-z0-9_' | head -c 28; }
umask 077
{
  printf 'MSSQL_SA_PASSWORD=Q9!%s\n' "$(secret)"
  printf 'QA_SEED_PASSWORD=Q8!%s\n' "$(secret)"
  printf 'QA_READER_PASSWORD=Q7!%s\n' "$(secret)"
  printf 'POSTGRES_PASSWORD=%s\n' "$(secret)"
  printf 'CLICKHOUSE_ADMIN_PASSWORD=%s\n' "$(secret)"
  printf 'CLICKHOUSE_READER_PASSWORD=%s\n' "$(secret)"
  printf 'CLICKHOUSE_WRITER_PASSWORD=%s\n' "$(secret)"
  printf 'DATAFLOW_API_TOKEN=%s\n' "$(secret)$(secret)"
} >"$envfile"
chmod 600 "$envfile"
echo "Created ignored QA secrets at $envfile"

