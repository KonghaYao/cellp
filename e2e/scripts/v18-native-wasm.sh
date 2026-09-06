#!/usr/bin/env bash
# TP-NATIVE-HTTP/BIND/ERR/ISOL/SMOKE — Native Component through Gateway Host and real KV
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_stack_or_skip

PROJECT="${DEV_PROJECT}"
VA="$(unique_id)"
VB="$(unique_id)"
VD="$(unique_id)"
VJ="$(unique_id)"
NS="example-values"
KEY="native-shared-key"
OP_KEY="native-operator-key"
VA_VALUE="native-va"
VB_VALUE="native-vb"
OP_VALUE="operator-to-native"
NATIVE_EXAMPLE="${E2E_ROOT}/dev/examples/native-wasm"
JS_EXAMPLE="${E2E_ROOT}/dev/examples/counter"
LOG="${EVIDENCE_DIR}/native-wasm-e2e.log"
JSON="${EVIDENCE_DIR}/native-wasm-e2e.json"
EXIT_CODE=1

write_json() {
  jq -n \
    --arg project "$PROJECT" \
    --arg va "$VA" \
    --arg vb "$VB" \
    --arg denied "$VD" \
    --arg js "$VJ" \
    --arg host_a "$(preview_host "$PROJECT" "$VA")" \
    --arg host_b "$(preview_host "$PROJECT" "$VB")" \
    --arg namespace "$NS" \
    --arg key "$KEY" \
    --argjson exit "$EXIT_CODE" \
    --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    '{project:$project,versions:{native_a:$va,native_b:$vb,denied:$denied,js:$js},gateway_hosts:[$host_a,$host_b],namespace:$namespace,key:$key,exit:$exit,timestamp:$timestamp}' \
    >"$JSON"
}
trap 'write_json' EXIT

stage_native() {
  local dest="$1"
  mkdir -p "$dest"
  cp "${NATIVE_EXAMPLE}/wrangler.jsonc" "$dest/wrangler.jsonc"
  cp "${NATIVE_EXAMPLE}/component.wasm" "$dest/component.wasm"
  cp "${NATIVE_EXAMPLE}/component.wasm.sha256" "$dest/component.wasm.sha256"
  (cd "$dest" && shasum -a 256 -c component.wasm.sha256 >/dev/null) \
    || fail "Native fixture digest mismatch"
}

operator_value() {
  local body="$1" value encoding
  value=$(printf '%s' "$body" | jq -r '.value // empty')
  encoding=$(printf '%s' "$body" | jq -r '.encoding // "utf-8"')
  if [[ "$encoding" == "base64" ]]; then
    value=$(printf '%s' "$value" | base64 -d 2>/dev/null || printf '%s' "$value" | base64 -D)
  fi
  printf '%s' "$value"
}

mkdir -p "$EVIDENCE_DIR"
: >"$LOG"
exec > >(tee -a "$LOG") 2>&1

[[ -f "${NATIVE_EXAMPLE}/component.wasm" && -f "${NATIVE_EXAMPLE}/wrangler.jsonc" ]] \
  || fail "missing Native fixture ${NATIVE_EXAMPLE}"

log "Native Wasm E2E project=${PROJECT} VA=${VA} VB=${VB} denied=${VD} js=${VJ}"
ensure_project "$PROJECT"
cleanup_e2e_versions "$PROJECT"

stage_native "${ARTIFACTS_DIR}/${PROJECT}/${VA}"
stage_native "${ARTIFACTS_DIR}/${PROJECT}/${VB}"
stage_native "${ARTIFACTS_DIR}/${PROJECT}/${VD}"
# Remove only the authorization declaration. The guest still imports cellp:kv@0.1
# and attempts to open VALUES, proving the capability snapshot fails closed.
jq 'del(.kv_namespaces)' "${ARTIFACTS_DIR}/${PROJECT}/${VD}/wrangler.jsonc" \
  >"${ARTIFACTS_DIR}/${PROJECT}/${VD}/wrangler.jsonc.tmp"
mv "${ARTIFACTS_DIR}/${PROJECT}/${VD}/wrangler.jsonc.tmp" \
  "${ARTIFACTS_DIR}/${PROJECT}/${VD}/wrangler.jsonc"
stage_worker_example "$JS_EXAMPLE" "${ARTIFACTS_DIR}/${PROJECT}/${VJ}"

create_version "$PROJECT" "$VA" | jq -r .id >/dev/null
poll_version "$PROJECT" "$VA" ready 120 >/dev/null
create_version "$PROJECT" "$VB" | jq -r .id >/dev/null
poll_version "$PROJECT" "$VB" ready 120 >/dev/null

# TP-NATIVE-HTTP: only the formal Gateway Host path counts.
wait_http_200_version "$PROJECT" "$VA" "/health" 60
HEALTH_BODY=$(curl_version "$PROJECT" "$VA" "/health")
[[ "$HEALTH_BODY" == $'native-http-v1\nhello-native' || "$HEALTH_BODY" == $'native-http-v1\nhello-native\n' ]] \
  || fail "Native health body mismatch: ${HEALTH_BODY}"
log "Gateway Host Native HTTP PASS host=$(preview_host "$PROJECT" "$VA")"

# TP-NATIVE-BIND: guest write -> cellpd operator read.
curl_version_method PUT "$PROJECT" "$VA" "/${KEY}" --data-binary "$VA_VALUE" >/dev/null
api_status GET "/v1/projects/${PROJECT}/versions/${VA}/kv/${NS}/keys/${KEY}"
[[ "$API_STATUS" == "200" ]] || fail "operator GET guest value -> HTTP ${API_STATUS}: ${API_BODY}"
[[ "$(operator_value "$API_BODY")" == "$VA_VALUE" ]] \
  || fail "operator did not observe guest KV write"

# cellpd operator write -> guest read.
api_status PUT "/v1/projects/${PROJECT}/versions/${VA}/kv/${NS}/keys/${OP_KEY}" \
  "$(jq -n --arg value "$OP_VALUE" '{value:$value}')"
[[ "$API_STATUS" == "200" || "$API_STATUS" == "204" ]] \
  || fail "operator PUT -> HTTP ${API_STATUS}: ${API_BODY}"
GUEST_VALUE=$(curl_version "$PROJECT" "$VA" "/${OP_KEY}")
[[ "$GUEST_VALUE" == "$OP_VALUE" ]] || fail "guest did not observe operator KV write"
log "real KV guest/operator bidirectional PASS"

# TP-NATIVE-ISOL: sibling starts empty, may use the same key, and cannot mutate VA.
api_status GET "/v1/projects/${PROJECT}/versions/${VB}/kv/${NS}/keys/${KEY}"
[[ "$API_STATUS" == "404" ]] || fail "VB operator GET expected 404, got ${API_STATUS}: ${API_BODY}"
[[ "$(http_code_version "$PROJECT" "$VB" "/${KEY}")" == "404" ]] \
  || fail "VB guest GET expected 404"
curl_version_method PUT "$PROJECT" "$VB" "/${KEY}" --data-binary "$VB_VALUE" >/dev/null
[[ "$(curl_version "$PROJECT" "$VB" "/${KEY}")" == "$VB_VALUE" ]] \
  || fail "VB guest value mismatch"
[[ "$(curl_version "$PROJECT" "$VA" "/${KEY}")" == "$VA_VALUE" ]] \
  || fail "VA value changed after VB write"
log "two-version same-key isolation PASS"

# TP-NATIVE-ERR: undeclared capability is denied without killing celld; a
# subsequent request that does not open KV must still pass through the Gateway.
create_version "$PROJECT" "$VD" | jq -r .id >/dev/null
poll_version "$PROJECT" "$VD" ready 120 >/dev/null
DENIED_CODE=$(http_code_version "$PROJECT" "$VD" "/${KEY}")
[[ "$DENIED_CODE" =~ ^(4|5)[0-9][0-9]$ ]] \
  || fail "undeclared KV expected stable error, got HTTP ${DENIED_CODE}"
wait_http_200_version "$PROJECT" "$VD" "/health" 30
curl -sf "http://127.0.0.1:${CELLD_PORT}/.well-known/celld/health" >/dev/null \
  || fail "celld unhealthy after denied Native request"
log "undeclared KV denial and recovery PASS HTTP=${DENIED_CODE}"

# TP-NATIVE-SMOKE: prove the historical JS/V8 path still deploys and serves.
create_version "$PROJECT" "$VJ" | jq -r .id >/dev/null
poll_version "$PROJECT" "$VJ" ready 120 >/dev/null
wait_http_200_version "$PROJECT" "$VJ" "/" 60
JS_BODY=$(curl_version "$PROJECT" "$VJ" "/")
printf '%s' "$JS_BODY" | jq -e . >/dev/null || fail "JS smoke response is not JSON: ${JS_BODY}"
log "JS/V8 smoke PASS"

EXIT_CODE=0
pass "v18-native-wasm HTTP + KV + denied/recovery + isolation + JS smoke"
cleanup_e2e_versions "$PROJECT" || true
