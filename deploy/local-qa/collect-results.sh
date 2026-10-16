#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd); run=${1:-20261016-qa001}; out="$root/qa-results/$run"
test -f "$out/02_TEST_MATRIX.csv"
grep -RIlE '(MSSQL_SA_PASSWORD=|QA_READER_PASSWORD=|DATAFLOW_API_TOKEN=)' "$out" && { echo 'secret-like assignment found' >&2; exit 1; } || true
(cd "$root" && zip -qr DATAFLOW_LOCAL_QA_RESULTS.zip "qa-results/$run" deploy/local-qa -x '.env.qa.local' '*.git*' '*node_modules*')
sha256sum "$root/DATAFLOW_LOCAL_QA_RESULTS.zip" > "$out/artifacts/zip.sha256"
echo "$root/DATAFLOW_LOCAL_QA_RESULTS.zip"

