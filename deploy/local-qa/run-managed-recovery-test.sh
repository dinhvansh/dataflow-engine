#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd); root=$(cd "$here/../.." && pwd)
"$here/verify-isolation.sh"
cd "$root"
[[ -f .env.qa.local ]] || { echo "QA-local secrets missing" >&2; exit 1; }
for c in dataflow-localqa-clickhouse dataflow-localqa-postgres; do
  [[ "$(docker inspect -f '{{index .Config.Labels "com.dataflow.owner"}}' "$c")" == dataflow-localqa ]] || exit 1
  [[ "$(docker inspect -f '{{index .Config.Labels "com.dataflow.role"}}' "$c")" == local-qa ]] || exit 1
done
set -a; source .env.qa.local; set +a
export DATAFLOW_INTEGRATION_POSTGRES_DSN="postgres://dataflow_qa:${POSTGRES_PASSWORD}@127.0.0.1:15433/dataflow_qa?sslmode=disable"
export DATAFLOW_INTEGRATION_CH_URL=http://127.0.0.1:18123
export DATAFLOW_INTEGRATION_PG_ISOLATED=true
export DATAFLOW_INTEGRATION_CH_WRITER_USER=qa_admin
export DATAFLOW_INTEGRATION_CH_WRITER_PASSWORD="$CLICKHOUSE_ADMIN_PASSWORD"
# The integration test expires only its uniquely allocated synthetic lease,
# reclaims the same logical run, rejects the stale writer, and must finish the
# recovered attempt as succeeded without advancing the checkpoint twice.
go test ./cmd/dataflow -run '^TestManagedQueueIntegration$' -count=1 -v
