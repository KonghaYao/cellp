#!/usr/bin/env bash
# TP-V9 — celld KV operator via cellpd :8790 (put/get + sibling empty-start isolation) (AD-12 Host)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"
# shellcheck disable=SC1091
source "$(dirname "$0")/lib-ingress.sh"

require_stack_or_skip

PROJECT="${DEV_PROJECT}"
VA="$(unique_id)"
VB="$(unique_id)"
NS="example-values"
KEY="e2e-greeting"
VA_VALUE="hello-from-va"
VB_VALUE="hello-from-vb"
KV_EXAMPLE="${E2E_ROOT}/celld/examples/kv"
DEST_A="${ARTIFACTS_DIR}/${PROJECT}/${VA}"
DEST_B="${ARTIFACTS_DIR}/${PROJECT}/${VB}"
LOG="${EVIDENCE_DIR}/v9-kv-e2e.log"
JSON="${EVIDENCE_DIR}/v9-kv-e2e.json"

BINDINGS_CODE=0
PUT_A_CODE=0
GET_A_CODE=0
GET_B_EMPTY_CODE=0
PUT_B_CODE=0
GET_A_AFTER_CODE=0
EXIT_CODE=1

write_json() {
  jq -n \
    --arg project "$PROJECT" \
    --arg va "$VA" \
    --arg vb "$VB" \
    --arg ns "$NS" \
    --arg key "$KEY" \
    --argjson exit "$EXIT_CODE" \
    --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --argjson bindings "$BINDINGS_CODE" \
    --argjson put_a "$PUT_A_CODE" \
    --argjson get_a "$GET_A_CODE" \
    --argjson get_b_empty "$GET_B_EMPTY_CODE" \
    --argjson put_b "$PUT_B_CODE" \
    --argjson get_a_after "$GET_A_AFTER_CODE" \
    '{
      project: $project,
      versions: [$va, $vb],
      namespace_id: $ns,
      key: $key,
      exit: $exit,
      timestamp: $ts,
      http: {
        bindings: $bindings,
        put_va: $put_a,
        get_va: $get_a,
        get_vb_empty: $get_b_empty,
        put_vb: $put_b,
        get_va_after: $get_a_after
      }
    }' >"$JSON"
}

trap 'write_json' EXIT

mkdir -p "$EVIDENCE_DIR"
: >"$LOG"
exec > >(tee -a "$LOG") 2>&1

log "V9 KV operator project=${PROJECT} VA=${VA} VB=${VB} ns=${NS}"

if [[ ! -d "$KV_EXAMPLE" ]]; then
  fail "missing ${KV_EXAMPLE} — celld/examples/kv is the KV artifact"
fi

ensure_project "$PROJECT"
cleanup_e2e_versions "$PROJECT"

stage_worker_example "$KV_EXAMPLE" "$DEST_A"
stage_worker_example "$KV_EXAMPLE" "$DEST_B"

create_version "$PROJECT" "$VA" | jq -r .id >/dev/null
poll_version "$PROJECT" "$VA" ready 120 >/dev/null

create_version "$PROJECT" "$VB" | jq -r .id >/dev/null
poll_version "$PROJECT" "$VB" ready 120 >/dev/null

BASE_A="/v1/projects/${PROJECT}/versions/${VA}"
BASE_B="/v1/projects/${PROJECT}/versions/${VB}"

api_status GET "${BASE_A}/bindings"
BINDINGS_CODE="$API_STATUS"
if [[ "$BINDINGS_CODE" != "200" ]]; then
  echo "$API_BODY" >&2
  fail "GET ${BASE_A}/bindings → HTTP ${BINDINGS_CODE} (need T1 ParseBindings) body=${API_BODY}"
fi
if ! echo "$API_BODY" | jq -e --arg ns "$NS" '.kv[] | select(.id==$ns and .binding=="VALUES")' >/dev/null; then
  echo "$API_BODY" >&2
  fail "bindings.kv missing id=${NS} binding=VALUES (got: $(echo "$API_BODY" | jq -c .kv))"
fi
log "bindings VA contains kv id=${NS}"

# Optional list/info — WARN only
api_status GET "${BASE_A}/kv"
if [[ "$API_STATUS" == "200" ]]; then
  log "GET …/kv namespaces=$(echo "$API_BODY" | jq -c .namespaces)"
else
  log "WARN: GET …/kv → HTTP ${API_STATUS}"
fi
api_status GET "${BASE_A}/kv/${NS}"
if [[ "$API_STATUS" != "200" ]]; then
  log "WARN: GET …/kv/${NS} info → HTTP ${API_STATUS}"
fi

PUT_BODY=$(jq -n --arg v "$VA_VALUE" '{value:$v}')
api_status PUT "${BASE_A}/kv/${NS}/keys/${KEY}" "$PUT_BODY"
PUT_A_CODE="$API_STATUS"
if [[ "$PUT_A_CODE" != "204" && "$PUT_A_CODE" != "200" ]]; then
  echo "$API_BODY" >&2
  fail "PUT VA key ${KEY} → HTTP ${PUT_A_CODE} (need T2 KvPut) body=${API_BODY}"
fi

api_status GET "${BASE_A}/kv/${NS}/keys/${KEY}"
GET_A_CODE="$API_STATUS"
if [[ "$GET_A_CODE" != "200" ]]; then
  echo "$API_BODY" >&2
  fail "GET VA key ${KEY} → HTTP ${GET_A_CODE} (need T2 KvGet) body=${API_BODY}"
fi
GOT=$(echo "$API_BODY" | jq -r '.value // empty')
ENC=$(echo "$API_BODY" | jq -r '.encoding // "utf-8"')
if [[ "$ENC" == "base64" ]]; then
  GOT=$(printf '%s' "$GOT" | base64 -d 2>/dev/null || printf '%s' "$GOT" | base64 -D)
fi
if [[ "$GOT" != "$VA_VALUE" ]]; then
  fail "GET VA value mismatch: want=${VA_VALUE} got=${GOT}"
fi
log "PUT/GET VA ok value=${VA_VALUE}"

api_status GET "${BASE_B}/kv/${NS}/keys/${KEY}"
GET_B_EMPTY_CODE="$API_STATUS"
if [[ "$GET_B_EMPTY_CODE" != "404" ]]; then
  echo "$API_BODY" >&2
  fail "GET VB key ${KEY} expected 404 (AD-7 empty start), got HTTP ${GET_B_EMPTY_CODE} body=${API_BODY}"
fi
log "VB isolation: GET ${KEY} → 404"

PUT_B_BODY=$(jq -n --arg v "$VB_VALUE" '{value:$v}')
api_status PUT "${BASE_B}/kv/${NS}/keys/${KEY}" "$PUT_B_BODY"
PUT_B_CODE="$API_STATUS"
if [[ "$PUT_B_CODE" != "204" && "$PUT_B_CODE" != "200" ]]; then
  echo "$API_BODY" >&2
  fail "PUT VB key ${KEY} → HTTP ${PUT_B_CODE} body=${API_BODY}"
fi

api_status GET "${BASE_A}/kv/${NS}/keys/${KEY}"
GET_A_AFTER_CODE="$API_STATUS"
GOT_AFTER=$(echo "$API_BODY" | jq -r '.value // empty')
ENC_AFTER=$(echo "$API_BODY" | jq -r '.encoding // "utf-8"')
if [[ "$ENC_AFTER" == "base64" ]]; then
  GOT_AFTER=$(printf '%s' "$GOT_AFTER" | base64 -d 2>/dev/null || printf '%s' "$GOT_AFTER" | base64 -D)
fi
if [[ "$GET_A_AFTER_CODE" != "200" || "$GOT_AFTER" != "$VA_VALUE" ]]; then
  fail "VA value mutated by VB write: http=${GET_A_AFTER_CODE} got=${GOT_AFTER} want=${VA_VALUE}"
fi
log "VA still ${VA_VALUE} after VB PUT"

VA_HOST="$(preview_host "$PROJECT" "$VA")"
WORKER_PATH="/${KEY}"
WORKER_CODE=$(http_code "$WORKER_URL")
if [[ "$WORKER_CODE" == "200" ]]; then
  WORKER_BODY=$(curl_gateway_host "$VA_HOST" "$WORKER_PATH" || true)
  if [[ "$WORKER_BODY" != "$VA_VALUE" ]]; then
    log "WARN: Worker GET ${KEY} body=${WORKER_BODY} (API assertion is authoritative)"
  else
    log "Worker cross-check GET ${KEY} ok"
  fi
else
  log "WARN: Worker GET ${KEY} HTTP ${WORKER_CODE} (API assertion is authoritative)"
fi

EXIT_CODE=0
pass "V9 KV put/get + sibling isolation OK VA=${VA} VB=${VB}"
cleanup_e2e_versions "$PROJECT" || true
exit 0
