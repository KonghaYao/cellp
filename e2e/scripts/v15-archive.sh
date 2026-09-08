#!/usr/bin/env bash
# TP-V15 — Archive/wake: 6 versions no 429; archive preview 503; wake 200; archive prod 422 (AD-12 Host)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"
# shellcheck disable=SC1091
source "$(dirname "$0")/lib-ingress.sh"

require_stack_or_skip

PROJECT="${DEV_PROJECT}-archive"
COUNTER="${E2E_ROOT}/dev/examples/counter"
LOG="${EVIDENCE_DIR}/v15-archive-e2e.log"

mkdir -p "$EVIDENCE_DIR"
: >"$LOG"
exec > >(tee -a "$LOG") 2>&1

log "V15 archive project=${PROJECT}"

# Safe JSON snippet for promote failure diagnostics (no tokens/secrets).
api_body_diag() {
  local raw="${1:-}"
  if [[ -z "$raw" ]]; then
    echo "(empty)"
    return 0
  fi
  echo "$raw" | jq -c 'if type == "object" then del(.token, .access_token, .refresh_token, .password, .secret) else . end' 2>/dev/null \
    || echo "$raw" | head -c 1024
}

assert_promote_ok() {
  local vid="$1"
  local label="$2"
  api_status POST "/v1/projects/${PROJECT}/versions/${vid}/promote" '{}'
  local promote_http="$API_STATUS"
  local promote_body="$API_BODY"
  if [[ "$promote_http" == "200" ]]; then
    api_status GET "/v1/projects/${PROJECT}/versions/${vid}"
    log "promote ${label} ${vid} HTTP ${promote_http} version_GET HTTP ${API_STATUS}"
    return 0
  fi
  api_status GET "/v1/projects/${PROJECT}/versions/${vid}"
  local ver_get_http="$API_STATUS"
  local ver_snapshot
  ver_snapshot=$(echo "$API_BODY" | jq -c '{id,status,error: (.error // empty)}' 2>/dev/null || api_body_diag "$API_BODY")
  {
    echo "DIAG: project=${PROJECT} promote ${label} version=${vid} POST HTTP ${promote_http} body=$(api_body_diag "$promote_body")"
    echo "DIAG: GET /versions/${vid} HTTP ${ver_get_http} ${ver_snapshot}"
  } >&2
  fail "promote ${label} ${vid} HTTP ${promote_http}"
}

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
assert_promote_ok "$PROD" "prod"

api_status POST "/v1/projects/${PROJECT}/versions/${PROD}/archive" ""
[[ "$API_STATUS" == "422" ]] || fail "archive prod → HTTP ${API_STATUS} (want 422)"

api_status POST "/v1/projects/${PROJECT}/versions/${TARGET}/archive" ""
[[ "$API_STATUS" == "200" ]] || fail "archive non-prod → HTTP ${API_STATUS}"

TGT_HOST="$(preview_host "$PROJECT" "$TARGET")"
ARCHIVE_CODE=$(http_code_gateway_host "$TGT_HOST" "/")
# curl_gateway_host uses --fail and intentionally suppresses HTTP error bodies.
# shellcheck disable=SC2046
ARCHIVE_BODY=$(curl -sS --max-time 5 $(gateway_curl_tls_flags) -H "Host: ${TGT_HOST}" "${GATEWAY_URL}/" 2>/dev/null || true)
[[ "$ARCHIVE_CODE" == "503" ]] || fail "archived preview → HTTP ${ARCHIVE_CODE}"
echo "$ARCHIVE_BODY" | grep -q version_archived || fail "503 body missing version_archived: ${ARCHIVE_BODY}"

api_status POST "/v1/projects/${PROJECT}/versions/${TARGET}/wake" ""
[[ "$API_STATUS" == "200" ]] || fail "wake → HTTP ${API_STATUS}"
poll_version "$PROJECT" "$TARGET" ready 180 >/dev/null
wait_http_200_version "$PROJECT" "$TARGET" "/" 60

log "V15 archive PASS"
pass "v15-archive"
