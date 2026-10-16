#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd); run=${1:-20261016-qa001}; out="$root/qa-results/$run"
test -f "$out/02_TEST_MATRIX.csv"
set -a; source "$root/.env.qa.local"; set +a
for name in MSSQL_SA_PASSWORD QA_SEED_PASSWORD QA_READER_PASSWORD POSTGRES_PASSWORD CLICKHOUSE_ADMIN_PASSWORD CLICKHOUSE_READER_PASSWORD CLICKHOUSE_WRITER_PASSWORD DATAFLOW_API_TOKEN; do
 value=${!name}
 if grep -RIFq -- "$value" "$out" "$root/deploy/local-qa"; then echo "secret value found for $name" >&2; exit 1; fi
done
(cd "$root" && zip -qr DATAFLOW_LOCAL_QA_RESULTS.zip "qa-results/$run" deploy/local-qa -x '.env.qa.local' '*.git*' '*node_modules*')
sha256sum "$root/DATAFLOW_LOCAL_QA_RESULTS.zip" > "$out/artifacts/zip.sha256"
echo "$root/DATAFLOW_LOCAL_QA_RESULTS.zip"

