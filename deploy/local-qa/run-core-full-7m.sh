#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
[[ "${DATAFLOW_CORE_FULL_7M:-}" == YES_OWNED_DISPOSABLE_7M ]] || exit 2
[[ "${DATAFLOW_QA_ALLOW_7M:-}" == YES_I_ACCEPT_DISPOSABLE_QA_SEEDING ]] || exit 2
[[ "${DATAFLOW_LIVE_QA_OPT_IN:-}" == true ]] || exit 2
bash deploy/local-qa/verify-reset-scope.sh
bash deploy/local-qa/verify-isolation.sh
set -a; source .env.qa.local; set +a
export DATAFLOW_LIVE_API_TOKEN=synthetic-live-browser-token-not-a-secret
export DATAFLOW_QA_LOCAL_OWNER_VERIFIED=true
export DATAFLOW_7M_EVIDENCE="qa-results/core-run7-$(date -u +%Y%m%dT%H%M%SZ)"
python qa/local-acceptance/run_full_7m_monitored.py
