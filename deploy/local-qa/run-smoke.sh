#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
"$here/verify-isolation.sh"
curl -fsS http://127.0.0.1:18080/healthz

