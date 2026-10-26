#!/usr/bin/env bash
# Run once with Codex Local on isolated disposable QA. NEVER run 7m.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
cd "$root"
[[ "${DATAFLOW_QA_ONE_SHOT_OPT_IN:-}" == true ]] ||
{ echo "BLOCKED: DATAFLOW_QA_ONE_SHOT_OPT_IN=true required" >&2; exit 2; }
[[ "${DATAFLOW_QA_ALLOW_DISPOSABLE_RESET:-}" == I_APPROVE_OWNED_QA_VOLUMES ]] ||
{ echo "BLOCKED: owner approval for owned QA volume reset required" >&2; exit 2; }
for cmd in docker go npm python3 pytest git; do
 command -v "$cmd" >/dev/null || { echo "BLOCKED: missing $cmd" >&2; exit 2; }
done
[[ -f .env.qa.local ]] ||
{ echo "BLOCKED: create isolated .env.qa.local first" >&2; exit 2; }
gitsha=$(git rev-parse HEAD)
[[ "$(git branch --show-current)" != main ]] ||
{ echo "BLOCKED: never run on main; check out isolated local test branch" >&2; exit 2; }
[[ -z "$(git status --porcelain)" ]] ||
{ echo "BLOCKED: preserve/reconcile uncommitted changes first" >&2; exit 2; }
git rev-parse --verify refs/remotes/origin/main >/dev/null 2>&1 &&
git merge-base --is-ancestor refs/remotes/origin/main HEAD ||
{ echo "BLOCKED: QA branch is behind origin/main or remote ref missing; fetch/rebase first" >&2; exit 2; }
# Refuse accidentally inherited real integration DSNs during static Go tests.
for key in DATAFLOW_INTEGRATION_POSTGRES_DSN DATAFLOW_INTEGRATION_CH_URL \
 DATAFLOW_INTEGRATION_CH_WRITER_USER DATAFLOW_INTEGRATION_CH_WRITER_PASSWORD \
 DATAFLOW_INTEGRATION_DBT_BIN DATAFLOW_SQLSERVER_DSN DATAFLOW_POSTGRES_DSN \
 DATAFLOW_CH_URL DATAFLOW_CH_USER DATAFLOW_CH_PASSWORD DATAFLOW_API_TOKEN; do
 unset "$key"
done
export DATAFLOW_QA_MIN_FREE_BYTES=${DATAFLOW_QA_MIN_FREE_BYTES:-4294967296}
"$here/verify-reset-scope.sh"
"$here/check-disk-capacity.sh" "$root"
out="$root/qa-results/local-core8b-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$out/logs"
printf '%s\n' "$gitsha" >"$out/git-sha.txt"
git branch --show-current >"$out/git-branch.txt"
git status --short >"$out/git-status.txt"
git rev-list --left-right --count HEAD...refs/remotes/origin/main >"$out/git-divergence.txt" 2>/dev/null || true
printf 'case\tstatus\tseconds\tlog\n' >"$out/matrix.tsv"
printf '%s\n' STATIC-SAFETY STATIC-RESET-NEGATIVE STATIC-EVIDENCE-CONTRACT STATIC-GO STATIC-WEB UI-MOCK PREP-8B CORE-8B-E2E PREP-CLAIM CORE-6B-CLAIM PREP-DQ CORE-6B-DQ PREP-MID-SQL CORE-6B-MID-SQL PREP-NETWORK CORE-6A-NETWORK VERIFY-EVIDENCE >"$out/case-catalog.txt"
finalize(){
 rc=$?
 python3 qa/local-acceptance/v8b/report.py "$out" "$rc" || true
 echo "QA evidence: $out ; exit=$rc" >&2
}
trap finalize EXIT
case_run(){
 id="$1"; shift
 start=$(date +%s%3N)
 echo "START $id"
 set +e
 "$@" >"$out/logs/$id.log" 2>&1
 rc=$?
 set -e
 elapsed_ms=$(( $(date +%s%3N) - start ))
 printf -v elapsed '%d.%03d' "$((elapsed_ms/1000))" "$((elapsed_ms%1000))"
 # The shell exit code alone is not evidence: each live case must create
 # a fresh, nonempty structured JSON artifact since it started.
 if (( rc == 0 )); then
  evidence=""
  case "$id" in
   CORE-8B-E2E) evidence="qa-results/core-run5/operator-result.json";;
   CORE-6B-CLAIM) evidence="qa-results/core-run6b/fencing-result.json";;
   CORE-6B-DQ) evidence="qa-results/core-run6b-dq/negative-dq-concurrency.json";;
   CORE-6B-MID-SQL) evidence="qa-results/core-run6b3/mid-sql-result.json";;
   CORE-6A-NETWORK) evidence="qa-results/core-run6/fault-result.json";;
  esac
  if [[ -n "$evidence" ]]; then
   if [[ ! -s "$evidence" ]] || (( $(stat -c %Y "$evidence") < start/1000 )); then
    echo "FAIL $id: fresh structured QA evidence missing: $evidence" >>"$out/logs/$id.log"
    rc=3
   fi
  fi
 fi
 if (( rc != 0 )); then
  printf '%s\tFAIL\t%s\t%s\n' "$id" "$elapsed" "logs/$id.log" >>"$out/matrix.tsv"
  echo "FAIL $id: retain QA volumes for debugging; see $out/logs/$id.log" >&2
  exit "$rc"
 fi
 printf '%s\tPASS\t%s\t%s\n' "$id" "$elapsed" "logs/$id.log" >>"$out/matrix.tsv"
 echo "PASS $id ($elapsed sec)"
}
fresh(){
 "$here/reset-owned-qa.sh" || return $?
 "$here/up.sh" || return $?
 "$here/verify-isolation.sh" || return $?
}
case_run STATIC-SAFETY "$here/verify-reset-scope.sh"
# Verify the fail-closed Docker reset behavior with a FAKE Docker binary.
# No real network/volume can be changed by these negative safety tests.
case_run STATIC-RESET-NEGATIVE python3 qa/local-acceptance/v8b/test_reset_scope_guard.py
# Verify the canonical evidence schema before any test can modify QA volumes.
case_run STATIC-EVIDENCE-CONTRACT python3 qa/local-acceptance/verify_core_final_evidence.py --contract
case_run STATIC-GO bash -c 'go test -race ./... && go vet ./... && go build ./cmd/...'
case_run STATIC-WEB bash -c 'cd web && npm ci --ignore-scripts && npm run build'
case_run UI-MOCK python3 qa/local-acceptance/v8b/run_mock_browser.py
case_run PREP-8B fresh
case_run CORE-8B-E2E env DATAFLOW_LIVE_QA_OPT_IN=true "$here/run-core-operator.sh"
case_run PREP-CLAIM fresh
case_run CORE-6B-CLAIM env DATAFLOW_LIVE_QA_OPT_IN=true "$here/run-core-6b.sh"
case_run PREP-DQ fresh
case_run CORE-6B-DQ env DATAFLOW_LIVE_QA_OPT_IN=true "$here/run-core-6b-dq.sh"
case_run PREP-MID-SQL fresh
case_run CORE-6B-MID-SQL env DATAFLOW_LIVE_QA_OPT_IN=true "$here/run-core-mid-sql.sh"
case_run PREP-NETWORK fresh
case_run CORE-6A-NETWORK env DATAFLOW_LIVE_QA_OPT_IN=true "$here/run-core-faults.sh"
# Materialize the canonical report before checking actual JSON-based evidence.
python3 qa/local-acceptance/v8b/report.py "$out" 0
case_run VERIFY-EVIDENCE python3 qa/local-acceptance/verify_core_final_evidence.py --report "$out"
echo "Core 8B one-shot suite complete; structured evidence verified, 7m benchmark NOT RUN."
