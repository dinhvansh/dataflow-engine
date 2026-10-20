#!/usr/bin/env bash
# Run on CI checkout ONLY: fake Docker/ss, no actual container or volume touch.
set -euo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root"
[[ ! -e .env.qa.local ]] || { echo "Refusing to modify existing QA owner env" >&2; exit 1; }
fake="$(mktemp -d)"
cleanup(){ rm -f .env.qa.local; rm -rf "$fake"; }
trap cleanup EXIT
printf 'DATAFLOW_TEST_TARGET_DATABASE=scratch\n' > .env.qa.local
cat > "$fake/ss" <<'SCRIPT'
#!/usr/bin/env bash
printf 'State Recv-Q Send-Q Local Address:Port Peer Address:Port\n'
if [[ "$*" == *:11433* && "${MOCK_PORT_OCCUPIED:-true}" == true ]]; then
 printf 'LISTEN 0 10 127.0.0.1:11433 0.0.0.0:*\n'
fi
SCRIPT
cat > "$fake/docker" <<'SCRIPT'
#!/usr/bin/env bash
case "${1:-}" in
 info) exit 0;;
 ps)
  if [[ "$*" == *--format* && "$*" == *Names* ]]; then echo dataflow-localqa-mssql; else echo mock-container-id; fi;;
 inspect)
  if [[ "$*" == *Config.Labels* ]]; then echo "${MOCK_OWNER:-dataflow-localqa local-qa}"
  elif [[ "$*" == *State.Running* ]]; then echo "${MOCK_RUNNING:-true}"
  elif [[ "$*" == *NetworkSettings.Ports* ]]; then echo "${MOCK_BINDING:-127.0.0.1:11433}"
  else exit 3; fi;;
 *) echo "Unexpected fake Docker call: $*" >&2;exit 3;;
esac
SCRIPT
chmod +x "$fake/ss" "$fake/docker"
export PATH="$fake:$PATH"
export DATAFLOW_QA_MIN_FREE_BYTES=0
if ! bash deploy/local-qa/preflight.sh > "$fake/ok.log" 2>&1; then cat "$fake/ok.log";exit 1;fi
if MOCK_BINDING=0.0.0.0:11433 bash deploy/local-qa/preflight.sh > "$fake/unsafe.log" 2>&1; then
 echo 'Unsafe wildcard binding was accepted' >&2;exit 1
fi
if MOCK_OWNER='unknown other' bash deploy/local-qa/preflight.sh > "$fake/unowned.log" 2>&1; then
 echo 'Unowned container was accepted' >&2;exit 1
fi
if MOCK_RUNNING=false bash deploy/local-qa/preflight.sh > "$fake/stopped.log" 2>&1; then
 echo 'Stopped owned container was accepted as listener' >&2;exit 1
fi
echo 'Owned QA reuse preflight regression PASS'
