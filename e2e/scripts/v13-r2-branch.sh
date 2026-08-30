#!/usr/bin/env bash
# TP-V13 — R2 branch: parent put via Worker → child GET; child overwrite → parent unchanged
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_stack_or_skip
require_celld_cli

PROJECT="${DEV_PROJECT}"
PARENT="$(unique_id)"
CHILD="$(unique_id)"
KEY="e2e-r2-branch.txt"
PARENT_BODY="parent-r2-body"
CHILD_BODY="child-r2-body"
R2_EXAMPLE="${E2E_ROOT}/celld/examples/r2"
DEST_P="${ARTIFACTS_DIR}/${PROJECT}/${PARENT}"
DEST_C="${ARTIFACTS_DIR}/${PROJECT}/${CHILD}"
LOG="${EVIDENCE_DIR}/v13-r2-branch-e2e.log"

mkdir -p "$EVIDENCE_DIR"
: >"$LOG"
exec > >(tee -a "$LOG") 2>&1

log "V13 R2 branch project=${PROJECT} parent=${PARENT} child=${CHILD}"

if [[ ! -d "$R2_EXAMPLE" ]]; then
  skip "missing ${R2_EXAMPLE} — celld r2 example required"
fi

ensure_project "$PROJECT"
cleanup_e2e_versions "$PROJECT"

stage_worker_example "$R2_EXAMPLE" "$DEST_P"
stage_worker_example "$R2_EXAMPLE" "$DEST_C"

create_version "$PROJECT" "$PARENT" | jq -r .id >/dev/null
poll_version "$PROJECT" "$PARENT" ready 120 >/dev/null

GW="${GATEWAY_URL}/${PROJECT}"
PREVIEW_P="${GW}/${PARENT}/"
PREVIEW_C="${GW}/${CHILD}/"

# celld/examples/r2: PUT /KEY writes, GET /KEY reads
curl -fsS -X PUT "${PREVIEW_P}${KEY}" -d "$PARENT_BODY" >/dev/null

create_version "$PROJECT" "$CHILD" "$PARENT" | jq -r .id >/dev/null
poll_version "$PROJECT" "$CHILD" ready 120 >/dev/null

CHILD_GET=$(curl -fsS "${PREVIEW_C}${KEY}" || true)
[[ "$CHILD_GET" == "$PARENT_BODY" ]] || fail "child GET after branch: want=${PARENT_BODY} got=${CHILD_GET}"

curl -fsS -X PUT "${PREVIEW_C}${KEY}" -d "$CHILD_BODY" >/dev/null
PARENT_GET=$(curl -fsS "${PREVIEW_P}${KEY}" || true)
[[ "$PARENT_GET" == "$PARENT_BODY" ]] || fail "parent unchanged: want=${PARENT_BODY} got=${PARENT_GET}"

log "V13 R2 branch PASS"
pass "v13-r2-branch"
