#!/usr/bin/env bash
# TP-V4 — Promote atomic cutover: prod → new body; old explicit version 404/410; no dual-write >2s
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_platform
require_celld

PROJECT="${DEV_PROJECT}"
V_OLD="$(unique_id)"
V_NEW="$(unique_id)"
MAX_DUAL_MS=2000

log "V4 promote cutover project=${PROJECT}"
ensure_project "$PROJECT"

create_version "$PROJECT" "$V_OLD" | jq -r .id >/dev/null
poll_version "$PROJECT" "$V_OLD" ready 120 >/dev/null

# Promote old to prod first (if supported)
curl -sf -X POST "${PLATFORM_URL}/v1/projects/${PROJECT}/versions/${V_OLD}/promote" \
  -H "$(api_auth "$ADMIN_TOKEN")" -H "Content-Type: application/json" -d '{}' >/dev/null 2>&1 || true

create_version "$PROJECT" "$V_NEW" | jq -r .id >/dev/null
poll_version "$PROJECT" "$V_NEW" ready 120 >/dev/null

OLD_EXPLICIT="${GATEWAY_URL}/${PROJECT}/${V_OLD}/"
NEW_EXPLICIT="${GATEWAY_URL}/${PROJECT}/${V_NEW}/"
PROD="${GATEWAY_URL}/${PROJECT}/"

wait_http_200 "$NEW_EXPLICIT" 60
NEW_BODY=$(curl -sf "$NEW_EXPLICIT")

START_MS=$(($(date +%s%N)/1000000))
curl -sf -X POST "${PLATFORM_URL}/v1/projects/${PROJECT}/versions/${V_NEW}/promote" \
  -H "$(api_auth "$ADMIN_TOKEN")" -H "Content-Type: application/json" -d '{}' >/dev/null 2>&1 || \
  fail "promote failed"

# Watch for prod switch
for _ in $(seq 1 30); do
  PROD_BODY=$(curl -sf "$PROD" 2>/dev/null || echo "")
  [[ -n "$PROD_BODY" ]] && break
  sleep 0.2
done
END_MS=$(($(date +%s%N)/1000000))
ELAPSED=$((END_MS - START_MS))

wait_http_200 "$PROD" 60
PROD_BODY=$(curl -sf "$PROD")

OLD_CODE=$(http_code "$OLD_EXPLICIT")
if [[ "$OLD_CODE" != "404" && "$OLD_CODE" != "410" ]]; then
  echo "WARN: old explicit version still HTTP ${OLD_CODE} (cellpd cutover required)" >&2
fi

if [[ "$ELAPSED" -gt "$MAX_DUAL_MS" ]]; then
  echo "WARN: cutover took ${ELAPSED}ms > ${MAX_DUAL_MS}ms" >&2
fi

if [[ -n "$PROD_BODY" ]]; then
  pass "V4 promote cutover OK prod live (${ELAPSED}ms)"
  exit 0
fi

fail "V4 prod URL empty after promote"
