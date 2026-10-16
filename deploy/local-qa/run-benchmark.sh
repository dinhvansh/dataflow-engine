#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd); root=$(cd "$here/../.." && pwd)
"$here/verify-isolation.sh"
echo "Measured runner payloads and three-trial raw results: $root/qa-results/20261016-qa001/perf.csv"

