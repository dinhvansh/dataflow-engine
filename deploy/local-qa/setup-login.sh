#!/usr/bin/env bash
# One-time operator-only password setup. Never print passwords or hashes to CI.
set -euo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root"
[[ -t 0 ]] || { echo "Run this from a local interactive terminal" >&2; exit 1; }
file="$root/.env.qa.local"
[[ -f "$file" ]] || { echo "Create the isolated .env.qa.local first; refusing other environments" >&2; exit 1; }
[[ "$(stat -c %a "$file")" == 600 ]] || { echo "Expected chmod 600 on private .env.qa.local" >&2; exit 1; }
bash deploy/local-qa/verify-isolation.sh
read -r -s -p "New OPERATOR password (min 12 chars): " operator_pass
echo
read -r -s -p "New ADMIN password (different, min 12 chars): " admin_pass
echo
[[ "${#operator_pass}" -ge 12 && "${#admin_pass}" -ge 12 && "$operator_pass" != "$admin_pass" ]] || {
 echo "Passwords must be different and at least 12 characters" >&2; exit 1;
}
operator_hash="$(printf '%s\n' "$operator_pass" | go run ./cmd/dataflow-auth-hash)"
admin_hash="$(printf '%s\n' "$admin_pass" | go run ./cmd/dataflow-auth-hash)"
unset operator_pass admin_pass
tmp="$(mktemp "$root/.env.qa.local.auth.XXXXXXXX")"
trap 'rm -f "$tmp"' EXIT
chmod 600 "$tmp"
grep -Ev '^(DATAFLOW_AUTH_SESSION_MODE|DATAFLOW_LOCAL_OPERATOR_PASSWORD_HASH|DATAFLOW_LOCAL_ADMIN_PASSWORD_HASH|DATAFLOW_ENABLE_RUNTIME_CONNECTIONS_LAB|DATAFLOW_ENABLE_LIVE_CONNECTION_TEST)=' "$file" > "$tmp" || :
{
 printf "DATAFLOW_AUTH_SESSION_MODE=local\n"
 printf "DATAFLOW_ENABLE_RUNTIME_CONNECTIONS_LAB=true\n"
 printf "DATAFLOW_ENABLE_LIVE_CONNECTION_TEST=true\n"
 # Single-quoted Compose dotenv values retain literal bcrypt $ characters.
 printf "DATAFLOW_LOCAL_OPERATOR_PASSWORD_HASH='%s'\n" "$operator_hash"
 printf "DATAFLOW_LOCAL_ADMIN_PASSWORD_HASH='%s'\n" "$admin_hash"
} >> "$tmp"
mv "$tmp" "$file"
echo "Local login configured. Restart only the approved isolated QA API/Web stack."
echo "Sign in with username operator or admin and the password you just set."
