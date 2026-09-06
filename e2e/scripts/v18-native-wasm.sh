#!/usr/bin/env bash
# TP-NATIVE-HTTP/BIND/ERR/ISOL/SMOKE — Native Component through Gateway Host and real KV
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_stack_or_skip

PROJECT="${DEV_PROJECT}"
VA="$(unique_id)"
VB="$(unique_id)"
VS="$(unique_id)"
VD="$(unique_id)"
VJ="$(unique_id)"
NS="example-values"
KEY="native-shared-key"
OP_KEY="native-operator-key"
VA_VALUE="native-va"
VB_VALUE="native-vb"
OP_VALUE="operator-to-native"
SAFETY_DEADLINE_MS="${NATIVE_E2E_DEADLINE_MS:-3000}"
SAFETY_GRACE_MS="${NATIVE_E2E_GRACE_MS:-250}"
SAFETY_MEMORY_BYTES="${NATIVE_E2E_MEMORY_BYTES:-8388608}"
SAFETY_MAX_HOSTCALLS="${NATIVE_E2E_MAX_HOSTCALLS:-4}"
SAFETY_EVIDENCE="${EVIDENCE_DIR}/native-wasm-safety-e2e.md"
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
    --arg vs "$VS" \
    --arg denied "$VD" \
    --arg js "$VJ" \
    --arg host_a "$(preview_host "$PROJECT" "$VA")" \
    --arg host_b "$(preview_host "$PROJECT" "$VB")" \
    --arg host_s "$(preview_host "$PROJECT" "$VS")" \
    --arg namespace "$NS" \
    --arg key "$KEY" \
    --argjson deadline_ms "$SAFETY_DEADLINE_MS" \
    --argjson grace_ms "$SAFETY_GRACE_MS" \
    --argjson exit "$EXIT_CODE" \
    --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    '{project:$project,versions:{native_a:$va,native_b:$vb,native_safety:$vs,denied:$denied,js:$js},gateway_hosts:[$host_a,$host_b,$host_s],namespace:$namespace,key:$key,safety:{request_deadline_ms:$deadline_ms,cancellation_grace_ms:$grace_ms},exit:$exit,timestamp:$timestamp}' \
    >"$JSON"
}
trap 'write_json' EXIT

stage_native() {
  local dest="$1"
  mkdir -p "$dest"
  cp "${NATIVE_EXAMPLE}/wrangler.jsonc" "$dest/wrangler.jsonc"
  cp "${NATIVE_EXAMPLE}/component.wasm" "$dest/component.wasm"
  cp "${NATIVE_EXAMPLE}/component.wasm.sha256" "$dest/component.wasm.sha256"
  local expected actual
  expected=$(tr -d '[:space:]' <"$dest/component.wasm.sha256")
  actual=$(shasum -a 256 "$dest/component.wasm" | cut -d ' ' -f 1)
  [[ "$actual" == "$expected" ]] || fail "Native fixture digest mismatch"
}

stage_native_safety() {
  local dest="$1"
  stage_native "$dest"
  jq --argjson deadline "$SAFETY_DEADLINE_MS" \
    --argjson grace "$SAFETY_GRACE_MS" \
    --argjson memory "$SAFETY_MEMORY_BYTES" \
    --argjson hostcalls "$SAFETY_MAX_HOSTCALLS" \
    '.vars.CELLP_NATIVE_REQUEST_DEADLINE_MS = ($deadline|tostring)
     | .vars.CELLP_NATIVE_CANCELLATION_GRACE_MS = ($grace|tostring)
     | .vars.CELLP_NATIVE_MAX_MEMORY_BYTES = ($memory|tostring)
     | .vars.CELLP_NATIVE_MAX_HOSTCALLS = ($hostcalls|tostring)
     | .vars.CELLP_NATIVE_E2E_HOSTCALL_FAIL = "1"' \
    "$dest/wrangler.jsonc" >"$dest/wrangler.jsonc.tmp"
  mv "$dest/wrangler.jsonc.tmp" "$dest/wrangler.jsonc"
}

now_ms() {
  python3 -c 'import time; print(int(time.time() * 1000))'
}

gateway_error_request() {
  local project="$1" version="$2" method="${3:-GET}" path="$4" max_time="${5:-60}"
  shift 5
  local tmp host
  tmp=$(mktemp)
  host=$(preview_host "$project" "$version")
  GATEWAY_ERR_CODE=$(curl -sS -o "$tmp" -w '%{http_code}' --max-time "$max_time" \
    $(gateway_curl_tls_flags) -X "$method" -H "Host: ${host}" "${GATEWAY_URL}${path}" "$@" 2>/dev/null || echo "000")
  GATEWAY_ERR_BODY=$(<"$tmp")
  rm -f "$tmp"
}

assert_gateway_error() {
  local label="$1" code_pattern="$2" body_pattern="$3"
  [[ "$GATEWAY_ERR_CODE" =~ $code_pattern ]] \
    || fail "${label}: expected HTTP ${code_pattern}, got ${GATEWAY_ERR_CODE}: ${GATEWAY_ERR_BODY}"
  printf '%s' "$GATEWAY_ERR_BODY" | grep -Eq "$body_pattern" \
    || fail "${label}: missing error code in body: ${GATEWAY_ERR_BODY}"
  printf '%s' "$GATEWAY_ERR_BODY" | grep -Eqi 'wasmtime|webassembly|backtrace|aws_secret_access_key|authorization:[[:space:]]*bearer' \
    && fail "${label}: response leaked engine or credential detail" || true
}

assert_no_engine_leak() {
  local label="$1" body="$2"
  printf '%s' "$body" | grep -Eqi 'wasmtime|webassembly|backtrace|aws_secret_access_key|authorization:[[:space:]]*bearer' \
    && fail "${label}: response leaked engine or credential detail" || true
}

assert_recovery() {
  local project="$1" version="$2" path="${3:-/health}"
  wait_http_200_version "$project" "$version" "$path" 15
}

record_safety_line() {
  printf '%s\n' "$1" >>"$SAFETY_EVIDENCE"
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

log "Native Wasm E2E project=${PROJECT} VA=${VA} VB=${VB} safety=${VS} denied=${VD} js=${VJ}"
ensure_project "$PROJECT"
cleanup_e2e_versions "$PROJECT"

stage_native "${ARTIFACTS_DIR}/${PROJECT}/${VA}"
stage_native "${ARTIFACTS_DIR}/${PROJECT}/${VB}"
stage_native "${ARTIFACTS_DIR}/${PROJECT}/${VD}"
stage_native_safety "${ARTIFACTS_DIR}/${PROJECT}/${VS}"
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
create_version "$PROJECT" "$VS" | jq -r .id >/dev/null
poll_version "$PROJECT" "$VS" ready 120 >/dev/null

# TP-NATIVE-HTTP/BIND: guest write through the formal Gateway Host path, then
# prove the same bytes through both guest HTTP and the cellpd operator API.
curl_version_method PUT "$PROJECT" "$VA" "/${KEY}" --data-binary "$VA_VALUE" >/dev/null
GATEWAY_VALUE=$(curl_version "$PROJECT" "$VA" "/${KEY}")
[[ "$GATEWAY_VALUE" == "$VA_VALUE" ]] || fail "Native Gateway response mismatch"
log "Gateway Host Native HTTP PASS host=$(preview_host "$PROJECT" "$VA")"

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

# TP-NATIVE-SAFETY: Gateway Host resource/error/cancellation on real Native runtime.
: >"$SAFETY_EVIDENCE"
record_safety_line "# Native Wasm safety E2E (Gateway Host)"
record_safety_line "- date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
record_safety_line "- project: ${PROJECT}"
record_safety_line "- safety version: ${VS}"
record_safety_line "- gateway host: $(preview_host "$PROJECT" "$VS")"
record_safety_line "- request_deadline_ms: ${SAFETY_DEADLINE_MS}"
record_safety_line "- cancellation_grace_ms: ${SAFETY_GRACE_MS}"
record_safety_line "- max_memory_bytes: ${SAFETY_MEMORY_BYTES}"
record_safety_line ""

SAFETY_HOST="$(preview_host "$PROJECT" "$VS")"

gateway_error_request "$PROJECT" "$VS" GET "/probe/trap" 30
assert_gateway_error "guest trap" '^(4|5)[0-9][0-9]$' 'guest_trap'
assert_recovery "$PROJECT" "$VS" "/health"
record_safety_line "| trap + recovery | PASS | host=${SAFETY_HOST} code=${GATEWAY_ERR_CODE} |"
log "Native trap/recovery PASS"

t0=$(now_ms)
gateway_error_request "$PROJECT" "$VS" GET "/probe/deadline" 20
t1=$(now_ms)
DEADLINE_ELAPSED=$((t1 - t0))
assert_gateway_error "deadline" '^(4|5)[0-9][0-9]$' 'deadline_exceeded'
[[ "$DEADLINE_ELAPSED" -ge $((SAFETY_DEADLINE_MS - 1500)) && "$DEADLINE_ELAPSED" -le $((SAFETY_DEADLINE_MS + 6000)) ]] \
  || fail "deadline elapsed ${DEADLINE_ELAPSED}ms outside [${SAFETY_DEADLINE_MS}-1500, ${SAFETY_DEADLINE_MS}+6000]"
assert_recovery "$PROJECT" "$VS" "/health"
record_safety_line "| deadline + recovery | PASS | elapsed_ms=${DEADLINE_ELAPSED} bound~${SAFETY_DEADLINE_MS} |"
log "Native deadline/recovery PASS elapsed=${DEADLINE_ELAPSED}ms"

gateway_error_request "$PROJECT" "$VS" GET "/probe/memory" 30
assert_gateway_error "memory limit" '^(4|5)[0-9][0-9]$' 'resource_exhausted'
assert_recovery "$PROJECT" "$VS" "/health"
record_safety_line "| memory + recovery | PASS | code=${GATEWAY_ERR_CODE} |"
log "Native memory/recovery PASS"

gateway_error_request "$PROJECT" "$VS" PUT "/probe/kv-hostcall-fail" 30 --data-binary "probe"
assert_gateway_error "hostcall failure" '^(4|5)[0-9][0-9]$' 'hostcall_failed'
assert_recovery "$PROJECT" "$VS" "/health"
record_safety_line "| hostcall failure + recovery | PASS | code=${GATEWAY_ERR_CODE} |"
log "Native hostcall failure/recovery PASS"

gateway_error_request "$PROJECT" "$VS" GET "/probe/hostcall-flood" 30
assert_gateway_error "hostcall quota" '^(4|5)[0-9][0-9]$' 'resource_exhausted'
assert_recovery "$PROJECT" "$VS" "/health"
record_safety_line "| hostcall quota + recovery | PASS | code=${GATEWAY_ERR_CODE} |"
log "Native hostcall quota/recovery PASS"

curl_version_method PUT "$PROJECT" "$VS" "/warm-key" --data-binary "warm" >/dev/null

t0=$(now_ms)
GATEWAY_ERR_CODE=$(curl -sS -o /dev/null -w '%{http_code}' --max-time 0.25 \
  $(gateway_curl_tls_flags) -H "Host: ${SAFETY_HOST}" "${GATEWAY_URL}/probe/deadline" 2>/dev/null || echo "000")
sleep 0.35
assert_recovery "$PROJECT" "$VS" "/health"
t1=$(now_ms)
CPU_CANCEL_ELAPSED=$((t1 - t0))
[[ "$CPU_CANCEL_ELAPSED" -lt $((SAFETY_DEADLINE_MS + 500)) ]] \
  || fail "client disconnect CPU cancel recovery too slow: ${CPU_CANCEL_ELAPSED}ms"
record_safety_line "| client disconnect CPU bounded stop | PASS | recovery_ms=${CPU_CANCEL_ELAPSED} grace=${SAFETY_GRACE_MS} |"
log "Native client-disconnect CPU bounded stop PASS elapsed=${CPU_CANCEL_ELAPSED}ms"

t0=$(now_ms)
GATEWAY_ERR_CODE=$(curl -sS -o /dev/null -w '%{http_code}' --max-time 0.25 \
  $(gateway_curl_tls_flags) -H "Host: ${SAFETY_HOST}" "${GATEWAY_URL}/probe/kv-spin" 2>/dev/null || echo "000")
sleep 0.35
assert_recovery "$PROJECT" "$VS" "/health"
t1=$(now_ms)
KV_CANCEL_ELAPSED=$((t1 - t0))
[[ "$KV_CANCEL_ELAPSED" -lt $((SAFETY_DEADLINE_MS + 500)) ]] \
  || fail "client disconnect KV hostcall recovery too slow: ${KV_CANCEL_ELAPSED}ms"
record_safety_line "| client disconnect KV hostcall bounded stop | PASS | recovery_ms=${KV_CANCEL_ELAPSED} grace=${SAFETY_GRACE_MS} |"
log "Native client-disconnect KV hostcall bounded stop PASS elapsed=${KV_CANCEL_ELAPSED}ms"

# TP-NATIVE-ERR: undeclared capability is denied without killing celld; a
# subsequent request that does not open KV must still pass through the Gateway.
create_version "$PROJECT" "$VD" | jq -r .id >/dev/null
poll_version "$PROJECT" "$VD" ready 120 >/dev/null
DENIED_BODY_FILE=$(mktemp)
DENIED_CODE=$(curl -sS -o "$DENIED_BODY_FILE" -w '%{http_code}' \
  $(gateway_curl_tls_flags) -H "Host: $(preview_host "$PROJECT" "$VD")" \
  "${GATEWAY_URL}/${KEY}" 2>/dev/null || echo "000")
DENIED_BODY=$(<"$DENIED_BODY_FILE")
rm -f "$DENIED_BODY_FILE"
[[ "$DENIED_CODE" =~ ^(4|5)[0-9][0-9]$ ]] \
  || fail "undeclared KV expected stable error, got HTTP ${DENIED_CODE}"
printf '%s' "$DENIED_BODY" | grep -Eq 'capability_denied|guest_trap' \
  || fail "undeclared KV response missing normalized error code: ${DENIED_BODY}"
printf '%s' "$DENIED_BODY" | grep -Eqi 'wasmtime|webassembly|backtrace|aws_secret_access_key|authorization:[[:space:]]*bearer' \
  && fail "undeclared KV response leaked engine or credential detail" || true
wait_http_200_version "$PROJECT" "$VA" "/${KEY}" 30
api_status GET "/v1/runtime/routes"
[[ "$API_STATUS" == "200" ]] || fail "runtime routes -> HTTP ${API_STATUS}"
printf '%s' "$API_BODY" | jq -e --arg project "$PROJECT" --arg version "$VD" \
  '.routes[] | select(.project_id==$project and .version_id==$version and .celld_health=="ok")' >/dev/null \
  || fail "denied Native version is not healthy after error: ${API_BODY}"
log "undeclared KV denial and per-version recovery PASS HTTP=${DENIED_CODE}"

# TP-NATIVE-SMOKE: prove the historical JS/V8 path still deploys and serves.
create_version "$PROJECT" "$VJ" | jq -r .id >/dev/null
poll_version "$PROJECT" "$VJ" ready 120 >/dev/null
wait_http_200_version "$PROJECT" "$VJ" "/" 60
JS_BODY=$(curl_version "$PROJECT" "$VJ" "/")
printf '%s' "$JS_BODY" | jq -e . >/dev/null || fail "JS smoke response is not JSON: ${JS_BODY}"
log "JS/V8 smoke PASS"

EXIT_CODE=0
pass "v18-native-wasm HTTP + KV + safety + denied/recovery + isolation + JS smoke"
cleanup_e2e_versions "$PROJECT" || true
