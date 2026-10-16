#!/usr/bin/env bash
set -euo pipefail
for service in clickhouse postgres; do
 c="dataflow-localqa-$service"
 [[ "$(docker inspect -f '{{index .Config.Labels "com.dataflow.owner"}}' "$c")" == dataflow-localqa ]] || { echo "Refusing $c" >&2; exit 1; }
done
echo 'Guards PASS. Fault commands must target only the two verified container names above; see qa-results artifacts for executed outage timeline.'

