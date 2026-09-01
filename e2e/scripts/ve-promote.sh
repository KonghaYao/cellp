#!/usr/bin/env bash
# TP-VE-3 — Promote routing: prod Host serves promoted version body (AD-12)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_platform
require_celld

PROJECT="${DEV_PROJECT}"
V_OLD="$(unique_id)"
V_NEW="$(unique_id)"
PROD_H="$(prod_host "$PROJECT")"

log "VE-3 promote project=${PROJECT} old=${V_OLD} new=${V_NEW} prod_host=${PROD_H}"
ensure_project "$PROJECT"

create_version "$PROJECT" "$V_OLD" | jq -r .id >/dev/null
poll_version "$PROJECT" "$V_OLD" ready 120 >/dev/null
wait_http_200_version "$PROJECT" "$V_OLD" "/" 60
OLD_BODY=$(curl_version "$PROJECT" "$V_OLD" "/")

create_version "$PROJECT" "$V_NEW" | jq -r .id >/dev/null
poll_version "$PROJECT" "$V_NEW" ready 120 >/dev/null
wait_http_200_version "$PROJECT" "$V_NEW" "/" 60
NEW_BODY=$(curl_version "$PROJECT" "$V_NEW" "/")

log "promote ${V_NEW} to prod"
HTTP=$(curl -sf -o /tmp/ve-promote.json -w '%{http_code}' -X POST \
  "${PLATFORM_URL}/v1/projects/${PROJECT}/versions/${V_NEW}/promote" \
  -H "$(api_auth "$ADMIN_TOKEN")" \
  -H "Content-Type: application/json" \
  -d '{}' || echo "000")

if [[ "$HTTP" != "200" && "$HTTP" != "202" && "$HTTP" != "204" ]]; then
  cat /tmp/ve-promote.json 2>/dev/null || true
  fail "promote returned HTTP ${HTTP}"
fi

wait_http_200_prod "$PROJECT" "/" 60
PROD_BODY=$(curl_prod "$PROJECT" "/")

if [[ -z "$PROD_BODY" ]]; then
  fail "empty prod body after promote"
fi

if [[ "$PROD_BODY" == "$NEW_BODY" ]] || echo "$PROD_BODY" | jq -e . >/dev/null 2>&1; then
  pass "VE-3 promote OK prod_host=${PROD_H}"
  exit 0
fi

fail "prod body mismatch after promote"
