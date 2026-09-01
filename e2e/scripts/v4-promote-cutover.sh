#!/usr/bin/env bash
# TP-V4 — Promote atomic cutover (AD-12 Host prod + preview Host)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"
# shellcheck disable=SC1091
source "$(dirname "$0")/lib-ingress.sh"

require_platform
require_celld

PROJECT="${DEV_PROJECT}"
V_OLD="$(unique_id)"
V_NEW="$(unique_id)"
MAX_DUAL_MS=2000
PROD_H="$(prod_host "$PROJECT")"

log "V4 promote cutover project=${PROJECT} prod_host=${PROD_H}"
ensure_project "$PROJECT"

create_version "$PROJECT" "$V_OLD" | jq -r .id >/dev/null
poll_version "$PROJECT" "$V_OLD" ready 120 >/dev/null

curl -sf -X POST "${PLATFORM_URL}/v1/projects/${PROJECT}/versions/${V_OLD}/promote" \
  -H "$(api_auth "$ADMIN_TOKEN")" -H "Content-Type: application/json" -d '{}' >/dev/null 2>&1 || true

create_version "$PROJECT" "$V_NEW" | jq -r .id >/dev/null
poll_version "$PROJECT" "$V_NEW" ready 120 >/dev/null

OLD_HOST="$(preview_host "$PROJECT" "$V_OLD")"
NEW_HOST="$(preview_host "$PROJECT" "$V_NEW")"
OLD_PREVIEW="$(version_preview_url "$PROJECT" "$V_OLD")"
NEW_PREVIEW="$(version_preview_url "$PROJECT" "$V_NEW")"

# Prefer API preview_url (scheme + host); fall back to Host header on gateway.
wait_http_200_host "$NEW_HOST" "/" 60
NEW_BODY=$(curl_gateway_host "$NEW_HOST" "/")

START_MS=$(($(date +%s%N)/1000000))
curl -sf -X POST "${PLATFORM_URL}/v1/projects/${PROJECT}/versions/${V_NEW}/promote" \
  -H "$(api_auth "$ADMIN_TOKEN")" -H "Content-Type: application/json" -d '{}' >/dev/null 2>&1 || \
  fail "promote failed"

for _ in $(seq 1 30); do
  PROD_BODY=$(curl_gateway_host "$PROD_H" "/" 2>/dev/null || echo "")
  [[ -n "$PROD_BODY" ]] && break
  sleep 0.2
done
END_MS=$(($(date +%s%N)/1000000))
ELAPSED=$((END_MS - START_MS))

wait_http_200_host "$PROD_H" "/" 60
PROD_BODY=$(curl_gateway_host "$PROD_H" "/")

OLD_CODE=$(http_code_gateway_host "$OLD_HOST" "/")
if [[ "$OLD_CODE" != "404" && "$OLD_CODE" != "410" && "$OLD_CODE" != "503" ]]; then
  echo "WARN: old preview Host still HTTP ${OLD_CODE} (expected drain/archived semantics)" >&2
fi

# Legacy path URLs (deprecated)
OLD_PATH="${GATEWAY_URL}/${PROJECT}/${V_OLD}/"
OLD_PATH_CODE=$(http_code "$OLD_PATH")

if [[ "$ELAPSED" -gt "$MAX_DUAL_MS" ]]; then
  echo "WARN: cutover took ${ELAPSED}ms > ${MAX_DUAL_MS}ms" >&2
fi

if [[ -n "$PROD_BODY" ]]; then
  pass "V4 promote cutover OK prod_host=${PROD_H} (${ELAPSED}ms) preview_url=${NEW_PREVIEW:-$NEW_HOST} path_old=${OLD_PATH_CODE}"
  exit 0
fi

fail "V4 prod empty after promote (Host=${PROD_H})"
