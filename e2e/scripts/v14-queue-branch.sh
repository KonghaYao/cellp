#!/usr/bin/env bash
# TP-V14 — Queue branch: parent enqueue → child peek sees snapshot
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_stack_or_skip

PROJECT="${DEV_PROJECT}"
PARENT="$(unique_id)"
CHILD="$(unique_id)"
QUEUE_EXAMPLE="${E2E_ROOT}/dev/examples/queue"
DEST_P="${ARTIFACTS_DIR}/${PROJECT}/${PARENT}"
DEST_C="${ARTIFACTS_DIR}/${PROJECT}/${CHILD}"
LOG="${EVIDENCE_DIR}/v14-queue-branch-e2e.log"

mkdir -p "$EVIDENCE_DIR"
: >"$LOG"
exec > >(tee -a "$LOG") 2>&1

need python3

log "V14 Queue branch project=${PROJECT} parent=${PARENT} child=${CHILD}"

if [[ ! -d "$QUEUE_EXAMPLE" ]]; then
  skip "missing ${QUEUE_EXAMPLE} — producer-only queue example required"
fi

ensure_project "$PROJECT"
cleanup_e2e_versions "$PROJECT"

stage_worker_example "$QUEUE_EXAMPLE" "$DEST_P"
stage_worker_example "$QUEUE_EXAMPLE" "$DEST_C"

create_version "$PROJECT" "$PARENT" | jq -r .id >/dev/null
poll_version "$PROJECT" "$PARENT" ready 120 >/dev/null

BASE_P="/v1/projects/${PROJECT}/versions/${PARENT}"
BASE_C="/v1/projects/${PROJECT}/versions/${CHILD}"

api_status GET "${BASE_P}/bindings"
[[ "$API_STATUS" == "200" ]] || fail "parent bindings HTTP ${API_STATUS}"
QUEUE_NAME=$(echo "$API_BODY" | jq -r '.queues[0].name // empty')
[[ -n "$QUEUE_NAME" ]] || fail "no queue in parent bindings"

GW="${GATEWAY_URL}/${PROJECT}"
curl -fsS -X POST "${GW}/${PARENT}/enqueue" -H 'Content-Type: application/json' \
  -d '{"body":"branch-test"}' >/dev/null || fail "parent enqueue via gateway"

create_version "$PROJECT" "$CHILD" "$PARENT" | jq -r .id >/dev/null
poll_version "$PROJECT" "$CHILD" ready 120 >/dev/null

api_status GET "${BASE_C}/queues/${QUEUE_NAME}/peek"
[[ "$API_STATUS" == "200" ]] || fail "child peek → HTTP ${API_STATUS}"
if ! peek_has_marker "$API_BODY" "branch-test"; then
  fail "child peek missing enqueued message body=${API_BODY}"
fi

log "V14 Queue branch PASS"
pass "v14-queue-branch"
