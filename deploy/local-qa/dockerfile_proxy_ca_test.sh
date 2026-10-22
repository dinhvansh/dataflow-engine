#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
api="$root/deploy/local-qa/Dockerfile.api"
web="$root/deploy/local-qa/Dockerfile.web"
compose="$root/deploy/local-qa/compose.qa.yml"

grep -Fqx 'RUN --mount=type=secret,id=proxy_ca,target=/etc/ssl/cert.pem,required=true \' "$api"
grep -Fq 'apk add --no-cache ca-certificates' "$api"
grep -Fqx 'RUN --mount=type=secret,id=proxy_ca,required=true \' "$web"
grep -Fq 'NODE_EXTRA_CA_CERTS=/run/secrets/proxy_ca npm ci --strict-ssl=true' "$web"
grep -Fqx 'COPY --chmod=644 deploy/local-qa/nginx.core-qa.conf /etc/nginx/conf.d/default.conf' "$web"
awk '/^  web:/{in_web=1} in_web && /^      secrets: \[proxy_ca\]$/{found=1} in_web && /^  dbt-worker:/{exit} END{exit !found}' "$compose"
grep -Fq 'DATAFLOW_ENABLE_INCREMENTAL_TEST: "true"' "$compose"
[[ "$(grep -Fc 'wget -Y off -qO- http://127.0.0.1:8080/healthz' "$compose")" == 2 ]]
if grep -Eq '^(COPY|ADD).*proxy_ca' "$api" "$web"; then
  echo 'Proxy CA must remain a BuildKit secret and must not be copied into an image layer' >&2
  exit 1
fi
echo 'Local QA API runtime-stage proxy CA contract PASS'
