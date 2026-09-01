#!/usr/bin/env bash
# TP-VE-5 / TP-API-5 — Destroy lifecycle (AD-12 Host)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_platform
require_celld

PROJECT="${DEV_PROJECT}"
VERSION="$(unique_id)"

log "VE-5 destroy project=${PROJECT} version=${VERSION}"
ensure_project "$PROJECT"

create_version "$PROJECT" "$VERSION" | jq -r .id >/dev/null
poll_version "$PROJECT" "$VERSION" ready 120 >/dev/null

wait_http_200_version "$PROJECT" "$VERSION" "/" 60

log "DELETE version ${VERSION}"
HTTP=$(curl -sf -o /tmp/ve-destroy.json -w '%{http_code}' -X DELETE \
  "${PLATFORM_URL}/v1/projects/${PROJECT}/versions/${VERSION}" \
  -H "$(api_auth "$ADMIN_TOKEN")" 2>/dev/null || echo "000")

if [[ "$HTTP" != "200" && "$HTTP" != "202" && "$HTTP" != "204" ]]; then
  cat /tmp/ve-destroy.json 2>/dev/null || true
  fail "DELETE returned HTTP ${HTTP}"
fi

STATUS=""
for _ in $(seq 1 120); do
  STATUS=$(api_get "/v1/projects/${PROJECT}/versions/${VERSION}" 2>/dev/null | jq -r .status 2>/dev/null || echo "gone")
  [[ "$STATUS" == "draining" || "$STATUS" == "destroyed" ]] && break
  [[ "$STATUS" == "gone" || "$STATUS" == "null" ]] && STATUS="destroyed" && break
  sleep 1
done

[[ "$STATUS" == "draining" || "$STATUS" == "destroyed" ]] || fail "expected draining/destroyed (last=${STATUS})"

for _ in $(seq 1 120); do
  STATUS=$(api_get "/v1/projects/${PROJECT}/versions/${VERSION}" 2>/dev/null | jq -r .status 2>/dev/null || echo "destroyed")
  [[ "$STATUS" == "destroyed" || "$STATUS" == "gone" || "$STATUS" == "null" ]] && break
  sleep 1
done

wait_http_gone_version "$PROJECT" "$VERSION" "/" 120

pass "VE-5 destroy OK gateway gone within 120s"
exit 0
