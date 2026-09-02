#!/usr/bin/env bash
# Re-promote known support projects after CELLP_INGRESS_BASE_DOMAIN change.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
# shellcheck disable=SC1091
source dev/.env
# shellcheck disable=SC1091
source e2e/scripts/lib.sh

# Projects with ingress prod Host (core + support-batch pass + ready in support-todos).
projects=(
  support-relay
  support-flaremo
  support-r2filebox
  support-edgeever
  support-memos
  support-r2explorer
  support-fileworker
  support-webhookflare
  support-monolith
  support-sonicjs
  support-nodewarden
  support-requestbin
  support-workflows
  support-cfbase
)

resolve_promote_version() {
  local proj="$1"
  local ver
  ver="$(api_get "/v1/projects/${proj}" "$ADMIN_TOKEN" 2>/dev/null | jq -r '.prod_version_id // empty' 2>/dev/null || true)"
  if [[ -n "$ver" ]]; then
    echo "$ver"
    return 0
  fi
  api_get "/v1/projects/${proj}/versions" "$ADMIN_TOKEN" 2>/dev/null \
    | jq -r '[.versions[]? | select(.status=="ready")] | sort_by(.id) | last | .id // empty' 2>/dev/null || true
}

for proj in "${projects[@]}"; do
  ver="$(resolve_promote_version "$proj")"
  if [[ -z "$ver" ]]; then
    echo "SKIP ${proj} (no prod_version_id or ready version)"
    continue
  fi
  st="$(api_get "/v1/projects/${proj}/versions/${ver}" 2>/dev/null | jq -r .status 2>/dev/null || echo absent)"
  if [[ "$st" != "ready" ]]; then
    echo "SKIP ${proj}/${ver} (status=${st})"
    continue
  fi
  out="$(curl -sf -X POST "${PLATFORM_URL}/v1/projects/${proj}/versions/${ver}/promote" \
    -H "$(api_auth "$ADMIN_TOKEN")" -H "Content-Type: application/json" -d '{}')"
  echo "$proj/$ver: $(echo "$out" | jq -r '.prod_url // .status')"
done
