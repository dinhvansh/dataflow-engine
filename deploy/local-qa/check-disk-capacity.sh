#!/usr/bin/env bash
set -euo pipefail

root=${1:-$(cd "$(dirname "$0")/../.." && pwd)}
minimum=${DATAFLOW_QA_MIN_FREE_BYTES:-4294967296}
[[ "$minimum" =~ ^[0-9]+$ ]] || { echo 'DATAFLOW_QA_MIN_FREE_BYTES must be an integer' >&2; exit 2; }

check_path() {
  local path=$1 label=$2 available_kb available
  available_kb=$(df -Pk "$path" | awk 'NR==2 {print $4}')
  [[ "$available_kb" =~ ^[0-9]+$ ]] || { echo "Cannot determine free space for $label" >&2; exit 2; }
  available=$((available_kb * 1024))
  if (( available < minimum )); then
    echo "QA disk guard: $label has $available bytes free; requires at least $minimum" >&2
    exit 1
  fi
}

check_path "$root" workspace
docker_root=$(docker info --format '{{.DockerRootDir}}' 2>/dev/null || true)
if [[ -n "$docker_root" && -e "$docker_root" ]]; then
  check_path "$docker_root" docker-root
fi
echo 'QA disk capacity guard PASS'
