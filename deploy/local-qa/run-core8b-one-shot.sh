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
printf '%s\n' STATIC-SAFETY STATIC-GO STATIC-WEB CORE-8B-E2E CORE-6B-CLAIM CORE-6B-DQ CORE-6B-MID-SQL CORE-6A-NETWORK >"$out/case-catalog.txt"
finalize(){
 rc=$?
 python3 qa/local-acceptance/v8b/report.py "$out" "$rc" || true
 echo "QA evidence: $out ; exit=$rc" >&2
}
trap finalize EXIT
case_run(){
 id="$1"; shift
 start=$(date +%s)
 echo "START $id"
 set +e
 "$@" >"$out/logs/$id.log" 2>&1
 rc=$?
 set -e
 elapsed=$(( $(date +%s) - start ))
 if (( rc != 0 )); then
  printf '%s\tFAIL\t%s\t%s\n' "$id" "$elapsed" "logs/$id.log" >>"$out/matrix.tsv"
  echo "FAIL $id: retain QA volumes for debugging; see $out/logs/$id.log" >&2
  exit "$rc"
 fi
 printf '%s\tPASS\t%s\t%s\n' "$id" "$elapsed" "logs/$id.log" >>"$out/matrix.tsv"
 echo "PASS $id ($elapsed sec)"
}
fresh(){
 "$here/reset-owned-qa.sh"
 "$here/up.sh"
 "$here/verify-isolation.sh"
}
case_run STATIC-SAFETY "$here/verify-reset-scope.sh"
case_run STATIC-GO go test ./...
case_run STATIC-WEB bash -c 'cd web && npm ci --ignore-scripts && npm run build'
fresh
case_run CORE-8B-E2E env DATAFLOW_LIVE_QA_OPT_IN=true "$here/run-core-operator.sh"
fresh
case_run CORE-6B-CLAIM env DATAFLOW_LIVE_QA_OPT_IN=true "$here/run-core-6b.sh"
fresh
case_run CORE-6B-DQ env DATAFLOW_LIVE_QA_OPT_IN=true "$here/run-core-6b-dq.sh"
fresh
case_run CORE-6B-MID-SQL env DATAFLOW_LIVE_QA_OPT_IN=true "$here/run-core-mid-sql.sh"
fresh
case_run CORE-6A-NETWORK env DATAFLOW_LIVE_QA_OPT_IN=true "$here/run-core-faults.sh"
echo "Core 8B one-shot suite complete; 7m benchmark NOT RUN."
