#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
DATAFLOW_QA_MIN_FREE_BYTES=1 "$here/check-disk-capacity.sh" "$here" >/dev/null
if DATAFLOW_QA_MIN_FREE_BYTES=999999999999999 "$here/check-disk-capacity.sh" "$here" >/dev/null 2>&1; then
  echo 'disk guard accepted an impossible threshold' >&2
  exit 1
fi
if DATAFLOW_QA_MIN_FREE_BYTES=invalid "$here/check-disk-capacity.sh" "$here" >/dev/null 2>&1; then
  echo 'disk guard accepted a nonnumeric threshold' >&2
  exit 1
fi
echo 'disk capacity regression PASS'
