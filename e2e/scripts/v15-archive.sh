#!/usr/bin/env bash
# TP-V15 — Archive/wake: 6 versions no 429; archive preview 503; wake 200; archive prod 422
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_stack_or_skip

PROJECT="${DEV_PROJECT}-archive"
COUNTER="${E2E_ROOT}/dev/examples/counter"
LOG="${EVIDENCE_DIR}/v15-archive-e2e.log"

mkdir -p "$EVIDENCE_DIR"
: >"$LOG"
exec > >(tee -a "$LOG") 2>&1

log "V15 archive project=${PROJECT}"

ensure_project "$PROJECT"
cleanup_project_e2e_all() {
  local project="$1"
  local ids
  ids=$(api_get "/v1/projects/${project}/versions" "$ADMIN_TOKEN" 2>/dev/null \
    | jq -r '.versions[]? | select(.id|startswith("v-e2e-")) | .id' 2>/dev/null || true)
  for vid in $ids; do
    [[ -z "$vid" ]] && continue
    api_delete "/v1/projects/${project}/versions/${vid}" "$ADMIN_TOKEN" >/dev/null 2>&1 || true
  done
}

cleanup_project_e2e_all "$PROJECT"

IDS=()
for i in $(seq 1 6); do
  VID="$(unique_id)"
  IDS+=("$VID")
  DEST="${ARTIFACTS_DIR}/${PROJECT}/${VID}"
  stage_worker_example "$COUNTER" "$DEST"
  create_version "$PROJECT" "$VID" | jq -e .id >/dev/null
done

# Explicit 7th deploy must not hit removed ready cap
EXTRA="$(unique_id)"
DEST="${ARTIFACTS_DIR}/${PROJECT}/${EXTRA}"
stage_worker_example "$COUNTER" "$DEST"
HTTP_CODE=$(curl -s -o /dev/null -w '%{http_code}' -X POST \
  -H "Authorization: Bearer ${DEPLOY_TOKEN:-$PLATFORM_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"id\":\"${EXTRA}\",\"git_ref\":\"main\"}" \
  "${PLATFORM_URL}/v1/projects/${PROJECT}/versions")
[[ "$HTTP_CODE" == "202" || "$HTTP_CODE" == "200" ]] || fail "7th version create → HTTP ${HTTP_CODE} (must not be 429)"

for VID in "${IDS[@]}"; do
  poll_version "$PROJECT" "$VID" ready 180 >/dev/null
done

TARGET="${IDS[0]}"
PROD="${IDS[1]}"
api_status POST "/v1/projects/${PROJECT}/versions/${PROD}/promote" ""
[[ "$API_STATUS" == "200" ]] || fail "promote → HTTP ${API_STATUS}"

api_status POST "/v1/projects/${PROJECT}/versions/${PROD}/archive" ""
[[ "$API_STATUS" == "422" ]] || fail "archive prod → HTTP ${API_STATUS} (want 422)"

api_status POST "/v1/projects/${PROJECT}/versions/${TARGET}/archive" ""
[[ "$API_STATUS" == "200" ]] || fail "archive non-prod → HTTP ${API_STATUS}"

TGT_HOST="$(preview_host "$PROJECT" "$TARGET")"
ARCHIVE_CODE=$(http_code_gateway_host "$TGT_HOST" "/")
ARCHIVE_BODY=$(curl -s -H "Host: ${TGT_HOST}" "${GATEWAY_URL}/" 2>/dev/null || true)
[[ "$ARCHIVE_CODE" == "503" ]] || fail "archived preview → HTTP ${ARCHIVE_CODE}"
echo "$ARCHIVE_BODY" | grep -q version_archived || fail "503 body missing version_archived: ${ARCHIVE_BODY}"

api_status POST "/v1/projects/${PROJECT}/versions/${TARGET}/wake" ""
[[ "$API_STATUS" == "200" ]] || fail "wake → HTTP ${API_STATUS}"
poll_version "$PROJECT" "$TARGET" ready 180 >/dev/null
wait_http_200_host "$TGT_HOST" "/" 60

log "V15 archive PASS"
pass "v15-archive"
