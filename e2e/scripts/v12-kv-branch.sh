#!/usr/bin/env bash
# TP-V12 — KV branch: parent put → child get; child put → parent unchanged
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_stack_or_skip

PROJECT="${DEV_PROJECT}"
PARENT="$(unique_id)"
CHILD="$(unique_id)"
NS="example-values"
KEY="e2e-branch-key"
PARENT_VALUE="from-parent"
CHILD_VALUE="from-child"
KV_EXAMPLE="${E2E_ROOT}/celld/examples/kv"
DEST_P="${ARTIFACTS_DIR}/${PROJECT}/${PARENT}"
DEST_C="${ARTIFACTS_DIR}/${PROJECT}/${CHILD}"
LOG="${EVIDENCE_DIR}/v12-kv-branch-e2e.log"

mkdir -p "$EVIDENCE_DIR"
: >"$LOG"
exec > >(tee -a "$LOG") 2>&1

log "V12 KV branch project=${PROJECT} parent=${PARENT} child=${CHILD}"

if [[ ! -d "$KV_EXAMPLE" ]]; then
  fail "missing ${KV_EXAMPLE}"
fi

ensure_project "$PROJECT"
cleanup_e2e_versions "$PROJECT"

stage_worker_example "$KV_EXAMPLE" "$DEST_P"
stage_worker_example "$KV_EXAMPLE" "$DEST_C"

create_version "$PROJECT" "$PARENT" | jq -r .id >/dev/null
poll_version "$PROJECT" "$PARENT" ready 120 >/dev/null

BASE_P="/v1/projects/${PROJECT}/versions/${PARENT}"
BASE_C="/v1/projects/${PROJECT}/versions/${CHILD}"

api_status PUT "${BASE_P}/kv/${NS}/keys/${KEY}" "$(jq -n --arg v "$PARENT_VALUE" '{value:$v}')"
[[ "$API_STATUS" == "200" || "$API_STATUS" == "204" ]] || fail "parent PUT → HTTP ${API_STATUS}"

create_version "$PROJECT" "$CHILD" "$PARENT" | jq -r .id >/dev/null
poll_version "$PROJECT" "$CHILD" ready 120 >/dev/null

api_status GET "${BASE_C}/kv/${NS}/keys/${KEY}"
[[ "$API_STATUS" == "200" ]] || fail "child GET parent key → HTTP ${API_STATUS} (expected branch read)"
GOT=$(echo "$API_BODY" | jq -r '.value // empty')
[[ "$GOT" == "$PARENT_VALUE" ]] || fail "child read mismatch: want=${PARENT_VALUE} got=${GOT}"

api_status PUT "${BASE_C}/kv/${NS}/keys/${KEY}" "$(jq -n --arg v "$CHILD_VALUE" '{value:$v}')"
[[ "$API_STATUS" == "200" || "$API_STATUS" == "204" ]] || fail "child PUT → HTTP ${API_STATUS}"

api_status GET "${BASE_P}/kv/${NS}/keys/${KEY}"
GOT_P=$(echo "$API_BODY" | jq -r '.value // empty')
[[ "$GOT_P" == "$PARENT_VALUE" ]] || fail "parent unchanged after child write: got=${GOT_P}"

log "V12 KV branch PASS"
pass "v12-kv-branch"
