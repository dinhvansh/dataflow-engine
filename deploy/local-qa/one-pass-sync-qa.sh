#!/usr/bin/env bash
# One-pass user acceptance: disposable services, verified login, UI start,
# real Add/Test/Discover/Bind/Full/Incremental test. No reset, no 7m.
set -euo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root"
[[ "${DATAFLOW_LIVE_QA_OPT_IN:-}" == "true" ]] || {
 echo 'Set DATAFLOW_LIVE_QA_OPT_IN=true explicitly on your isolated QA machine' >&2; exit 1;
}
if [[ -f .env.qa.local ]]; then
 if grep -Eqi 'SRVERPTEST|ERPDBTEST' .env.qa.local; then
  echo 'Real ERP reference detected in QA env; abort' >&2; exit 1;
 fi
fi
echo '[1/4] Boot/verify preserved synthetic SQL Server, PostgreSQL and ClickHouse QA fixtures'
bash deploy/local-qa/up.sh
[[ -f .env.qa.local ]] || { echo 'Missing QA secrets' >&2; exit 1; }
if ! grep -qx 'DATAFLOW_AUTH_SESSION_MODE=local' .env.qa.local; then
 [[ -t 0 ]] || { echo 'One-time local login requires an interactive terminal' >&2; exit 1; }
 echo '[2/4] Configure local admin/operator passwords (one time)'
 bash deploy/local-qa/setup-login.sh
else
 echo '[2/4] Existing local login settings reused; no credential reset'
fi
echo '[3/4] Check bound loopback ports, apply additive migrations, build and start QA UI'
DATAFLOW_LIVE_QA_OPT_IN=true bash deploy/local-qa/start-sync-setup.sh
echo '[4/4] Run isolated real SQL Server + PostgreSQL + ClickHouse Connection/Full/Incremental acceptance'
DATAFLOW_LIVE_QA_OPT_IN=true bash deploy/local-qa/run-runtime-connections-real.sh
echo 'ONE-PASS QA PASS: fixtures verified, API and browser healthy, connection and Full/Incremental acceptance passed.'
echo 'Browse: http://127.0.0.1:15173'
echo 'Open Connections > Airbyte-style Batch Sync Setup. Keep all streams in scratch.'
