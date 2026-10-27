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
if [[ "${MOCK_PORT_OCCUPIED:-true}" == true ]]; then
 for port in 11433 18080 15173; do
  if [[ "$*" == *:$port* ]]; then
   printf 'LISTEN 0 10 127.0.0.1:%s 0.0.0.0:*\n' "$port"
  fi
 done
fi
SCRIPT
cat > "$fake/docker" <<'SCRIPT'
#!/usr/bin/env bash
case "${1:-}" in
 info) exit 0;;
 ps)
  if [[ "$*" == *--format* && "$*" == *Names* ]]; then
   printf '%s\n' dataflow-localqa-mssql dataflow-localqa-api-1 dataflow-localqa-web-1
  else echo mock-container-id; fi;;
 inspect)
  container="${!#}"
  case "$container" in
   dataflow-localqa-mssql|dataflow-localqa-api-1|dataflow-localqa-web-1|mock-container-id) ;;
   *) exit 3;;
  esac
  if [[ "$*" == *Config.Labels* ]]; then echo "${MOCK_OWNER:-dataflow-localqa local-qa}"
  elif [[ "$*" == *State.Running* ]]; then echo "${MOCK_RUNNING:-true}"
  elif [[ "$*" == *NetworkSettings.Ports* ]]; then
   case "$container" in
    dataflow-localqa-mssql) echo "${MOCK_BINDING:-127.0.0.1:11433}";;
    dataflow-localqa-api-1) echo '127.0.0.1:18080';;
    dataflow-localqa-web-1) echo '127.0.0.1:15173';;
   esac
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
