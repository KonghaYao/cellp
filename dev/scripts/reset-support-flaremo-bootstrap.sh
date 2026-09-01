#!/usr/bin/env bash
# Dev-only: wipe FlareMo owner auth + bootstrap so /setup can run again.
# Uses cellpd D1 query API — do not use on production.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
# shellcheck disable=SC1091
source dev/.env
# shellcheck disable=SC1091
source e2e/scripts/lib.sh

PROJECT="${SUPPORT_PROJECT:-support-flaremo}"
VERSION="${1:-v6}"
API="${PLATFORM_URL}/v1/projects/${PROJECT}/versions/${VERSION}/database/query"
GATEWAY_HOST="$(prod_host "$PROJECT")"

d1_query() {
  local sql="$1"
  curl -sf -H "$(api_auth "$ADMIN_TOKEN")" "$API" \
    -H "Content-Type: application/json" \
    -d "$(jq -nc --arg sql "$sql" '{sql:$sql}')"
}

bootstrap_status() {
  curl -sS -H "Host: ${GATEWAY_HOST}" \
    "${GATEWAY_URL}/api/auth/flaremo/bootstrap/status"
}

log "before:"
bootstrap_status | jq .

log "wiping auth + domain users (dev re-setup)"
d1_query "DELETE FROM auth_sessions" >/dev/null || true
d1_query "DELETE FROM auth_accounts" >/dev/null || true
d1_query "DELETE FROM auth_apikeys" >/dev/null || true
d1_query "DELETE FROM auth_verifications" >/dev/null || true
d1_query "DELETE FROM auth_user_links" >/dev/null || true
d1_query "DELETE FROM auth_users" >/dev/null || true
d1_query "DELETE FROM auth_bootstrap" >/dev/null || true
d1_query "DELETE FROM users" >/dev/null || true

log "after:"
bootstrap_status | jq .
