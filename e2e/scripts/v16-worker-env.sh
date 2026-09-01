#!/usr/bin/env bash
# TP-V16 — Worker env: CD/API override reaches Worker via CELLD_VARS_FILE
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_stack_or_skip

PROJECT="${DEV_PROJECT}"
VID="$(unique_id)"
COUNTER="${E2E_ROOT}/dev/examples/counter"
DEST="${ARTIFACTS_DIR}/${PROJECT}/${VID}"
LOG="${EVIDENCE_DIR}/v16-worker-env-e2e.log"

mkdir -p "$EVIDENCE_DIR"
: >"$LOG"
exec > >(tee -a "$LOG") 2>&1

log "V16 Worker env project=${PROJECT} version=${VID}"

ensure_project "$PROJECT"
cleanup_e2e_versions "$PROJECT"
stage_worker_example "$COUNTER" "$DEST"

create_version "$PROJECT" "$VID" "" '{"env":{"GREETING":"from-e2e"}}' | jq -r .id >/dev/null
poll_version "$PROJECT" "$VID" ready 120 >/dev/null

api_status GET "/v1/projects/${PROJECT}/versions/${VID}/env"
[[ "$API_STATUS" == "200" ]] || fail "GET env → HTTP ${API_STATUS}"
echo "$API_BODY" | jq -e '.vars[] | select(.key=="GREETING" and .value=="from-e2e" and .source=="override")' >/dev/null \
  || fail "GET env missing override GREETING: ${API_BODY}"
echo "$API_BODY" | jq -e '.vars[] | select(.key=="PROJECT_ID" and .source=="platform")' >/dev/null \
  || fail "GET env missing platform PROJECT_ID"

wait_http_200_version "$PROJECT" "$VID" "/" 60
BODY=$(curl_version "$PROJECT" "$VID" "/")
GOT=$(echo "$BODY" | jq -r '.greeting // empty')
[[ "$GOT" == "from-e2e" ]] || fail "worker greeting want=from-e2e got=${GOT} body=${BODY}"

api_status PUT "/v1/projects/${PROJECT}/versions/${VID}/env" '{"vars":{"GREETING":"after-put"}}'
[[ "$API_STATUS" == "200" ]] || fail "PUT env → HTTP ${API_STATUS} ${API_BODY}"

for i in $(seq 1 30); do
  BODY=$(curl_version "$PROJECT" "$VID" "/" 2>/dev/null || true)
  GOT=$(echo "$BODY" | jq -r '.greeting // empty' 2>/dev/null || true)
  if [[ "$GOT" == "after-put" ]]; then
    break
  fi
  sleep 1
done
[[ "$GOT" == "after-put" ]] || fail "worker greeting after PUT want=after-put got=${GOT} body=${BODY}"

log "V16 Worker env PASS"
pass "v16-worker-env"
