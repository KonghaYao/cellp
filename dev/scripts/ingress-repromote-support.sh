#!/usr/bin/env bash
# Re-promote known support projects after CELLP_INGRESS_BASE_DOMAIN change.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
# shellcheck disable=SC1091
source dev/.env
# shellcheck disable=SC1091
source e2e/scripts/lib.sh

pairs=(
  "support-flaremo:v3"
  "support-r2filebox:v15"
)

for p in "${pairs[@]}"; do
  proj="${p%%:*}"
  ver="${p##*:}"
  st="$(api_get "/v1/projects/${proj}/versions/${ver}" 2>/dev/null | jq -r .status 2>/dev/null || echo absent)"
  if [[ "$st" != "ready" ]]; then
    echo "SKIP ${proj}/${ver} (status=${st})"
    continue
  fi
  out="$(curl -sf -X POST "${PLATFORM_URL}/v1/projects/${proj}/versions/${ver}/promote" \
    -H "$(api_auth "$ADMIN_TOKEN")" -H "Content-Type: application/json" -d '{}')"
  echo "$proj/$ver: $(echo "$out" | jq -r '.prod_url // .status')"
done
