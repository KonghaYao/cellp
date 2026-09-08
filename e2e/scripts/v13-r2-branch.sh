#!/usr/bin/env bash
# TP-V13 — R2 branch: fallback, overwrite, delete tombstone, and replacement put
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_stack_or_skip
require_celld_cli

PROJECT="${DEV_PROJECT}"
PARENT="$(unique_id)"
CHILD="$(unique_id)"
KEY="e2e-r2-branch.txt"
BULK_KEY="e2e-r2-bulk.bin"
PARENT_BODY="parent-r2-body"
CHILD_BODY="child-r2-body"
BULK_PARENT_BODY="parent-r2-bulk-body"
R2_EXAMPLE="${E2E_ROOT}/celld/examples/r2"
DEST_P="${ARTIFACTS_DIR}/${PROJECT}/${PARENT}"
DEST_C="${ARTIFACTS_DIR}/${PROJECT}/${CHILD}"
LOG="${EVIDENCE_DIR}/v13-r2-branch-e2e.log"
BULK_TMP="$(mktemp -d "${TMPDIR:-/tmp}/cellp-v13-r2-bulk.XXXXXX")"
trap 'rm -rf "$BULK_TMP"' EXIT

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

GW="${GATEWAY_URL}"
HOST_P="$(preview_host "$PROJECT" "$PARENT")"
HOST_C="$(preview_host "$PROJECT" "$CHILD")"

curl_version_method PUT "$PROJECT" "$PARENT" "/${KEY}" -d "$PARENT_BODY" >/dev/null
curl_version_method PUT "$PROJECT" "$PARENT" "/${BULK_KEY}" -d "$BULK_PARENT_BODY" >/dev/null

create_version "$PROJECT" "$CHILD" "$PARENT" | jq -r .id >/dev/null
poll_version "$PROJECT" "$CHILD" ready 120 >/dev/null

CHILD_GET=$(curl_gateway_host "$HOST_C" "/${KEY}" || true)
[[ "$CHILD_GET" == "$PARENT_BODY" ]] || fail "child GET after branch: want=${PARENT_BODY} got=${CHILD_GET}"

curl_version_method PUT "$PROJECT" "$CHILD" "/${KEY}" -d "$CHILD_BODY" >/dev/null
CHILD_GET=$(curl_gateway_host "$HOST_C" "/${KEY}" || true)
[[ "$CHILD_GET" == "$CHILD_BODY" ]] || fail "child overwrite: want=${CHILD_BODY} got=${CHILD_GET}"
PARENT_GET=$(curl_gateway_host "$HOST_P" "/${KEY}" || true)
[[ "$PARENT_GET" == "$PARENT_BODY" ]] || fail "parent unchanged: want=${PARENT_BODY} got=${PARENT_GET}"

curl_version_method DELETE "$PROJECT" "$CHILD" "/${KEY}" >/dev/null
CHILD_CODE=$(http_code_gateway_host "$HOST_C" "/${KEY}")
[[ "$CHILD_CODE" == "404" ]] || fail "child delete tombstone: want=404 got=${CHILD_CODE}"
curl_version_method PUT "$PROJECT" "$CHILD" "/${KEY}" -d "$CHILD_BODY" >/dev/null
CHILD_GET=$(curl_gateway_host "$HOST_C" "/${KEY}" || true)
[[ "$CHILD_GET" == "$CHILD_BODY" ]] || fail "child replacement after delete: want=${CHILD_BODY} got=${CHILD_GET}"
PARENT_GET=$(curl_gateway_host "$HOST_P" "/${KEY}" || true)
[[ "$PARENT_GET" == "$PARENT_BODY" ]] || fail "parent unchanged after child replacement: want=${PARENT_BODY} got=${PARENT_GET}"

curl_version_method DELETE "$PROJECT" "$CHILD" "/${BULK_KEY}" >/dev/null
CHILD_CODE=$(http_code_gateway_host "$HOST_C" "/${BULK_KEY}")
[[ "$CHILD_CODE" == "404" ]] || fail "child bulk key tombstone: want=404 got=${CHILD_CODE}"
printf 'cellp-r2-bulk\0binary-payload\n' >"${BULK_TMP}/payload.bin"
jq -n --arg key "$BULK_KEY" --arg file "${BULK_TMP}/payload.bin" \
  '[{key: $key, file: $file}]' >"${BULK_TMP}/manifest.json"
celld r2 bulk put example-files --filename "${BULK_TMP}/manifest.json" \
  --bucket "s3://cellp-celld/${PROJECT}/${CHILD}" \
  --endpoint "$S3_ENDPOINT" --region "$AWS_REGION" --json >/dev/null
# `cmp` proves the operator-imported bytes cross the real fleet bucket and the
# deployed JavaScript R2 binding without text or UTF-8 transformation.
curl -fsS $(gateway_curl_tls_flags) -H "Host: ${HOST_C}" \
  "${GATEWAY_URL}/${BULK_KEY}" -o "${BULK_TMP}/worker-readback.bin"
cmp "${BULK_TMP}/payload.bin" "${BULK_TMP}/worker-readback.bin" >/dev/null \
  || fail "operator bulk bytes differ from Worker R2 readback"
PARENT_GET=$(curl_gateway_host "$HOST_P" "/${BULK_KEY}" || true)
[[ "$PARENT_GET" == "$BULK_PARENT_BODY" ]] || fail "parent changed after child bulk import: want=${BULK_PARENT_BODY} got=${PARENT_GET}"

log "V13 R2 branch PASS"
pass "v13-r2-branch"
