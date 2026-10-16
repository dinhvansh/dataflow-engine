#!/usr/bin/env bash
set -euo pipefail

: "${DATAFLOW_API_TOKEN:?Set DATAFLOW_API_TOKEN}"
API="${DATAFLOW_API_URL:-http://127.0.0.1:8080}"
SCHEMA="${DATAFLOW_SOURCE_SCHEMA:-Erp}"
TABLE="${DATAFLOW_SOURCE_TABLE:-PartTran}"
TARGET_DB="${DATAFLOW_TEST_TARGET_DATABASE:-scratch}"

command -v jq >/dev/null || { echo 'jq is required'; exit 2; }
command -v curl >/dev/null || { echo 'curl is required'; exit 2; }

echo "[1/5] Check API health"
curl --fail --silent --show-error "$API/healthz"; echo

echo "[2/5] Discover source columns"
curl --fail --silent --show-error -H "Authorization: Bearer $DATAFLOW_API_TOKEN" \
 "$API/api/v1/source/columns?schema=$SCHEMA&table=$TABLE" |
 jq '[.[] | select(.name=="SysRevID" or .name=="SysRowID")]'

echo "[3/5] Register managed stream"
body=$(jq -n --arg s "$SCHEMA" --arg t "$TABLE" --arg db "$TARGET_DB" \
 '{source_schema:$s,source_table:$t,target_database:$db,target_table:"dataflow_raw_events",batch_size:10000,workers:2,queue_capacity:4}')
stream=$(curl --fail --silent --show-error -X POST "$API/api/v1/managed/streams" \
 -H "Authorization: Bearer $DATAFLOW_API_TOKEN" -H 'Content-Type: application/json' \
 -d "$body" | tee /dev/stderr | jq -er '.id')

echo "[4/5] Trigger asynchronous load: $stream"
run=$(curl --fail --silent --show-error -X POST "$API/api/v1/managed/runs" \
 -H "Authorization: Bearer $DATAFLOW_API_TOKEN" -H 'Content-Type: application/json' \
 -d "$(jq -n --arg id "$stream" '{stream_id:$id}')" | tee /dev/stderr | jq -er '.run_id')

echo "[5/5] Poll for completion: $run"
for attempt in $(seq 1 180); do
  response=$(curl --fail --silent --show-error -H "Authorization: Bearer $DATAFLOW_API_TOKEN" \
    "$API/api/v1/managed/runs?id=$run")
  status=$(jq -r '.status' <<<"$response")
  echo "[$attempt] status=$status rows=$(jq -r '.rows_written' <<<"$response")"
  if [[ "$status" == "succeeded" ]]; then
    jq '.' <<<"$response"
    echo 'PASS: managed run succeeded. Verify actual ClickHouse data, then run again to test incremental cursor.'
    exit 0
  fi
  if [[ "$status" == "failed" ]]; then jq '.' <<<"$response";exit 1;fi
  sleep 2
done
echo "Run still active after 6 minutes. Inspect status manually: $run"
exit 3
