#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd); root=$(cd "$here/../.." && pwd); cd "$root"
"$here/verify-isolation.sh"
echo "Use the pinned test runner and QA-only variables documented in qa-results; integration tests refuse databases without DATAFLOW_INTEGRATION_PG_ISOLATED=true."
docker run --rm -v "$root:/src" -w /src dataflow-localqa-test-runner go test ./...

