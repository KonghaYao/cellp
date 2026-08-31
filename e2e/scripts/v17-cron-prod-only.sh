#!/usr/bin/env bash
# TP-V17 — Cron ticks only on prod when two ready versions share the same wrangler crons (AD-11)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_stack_or_skip
command -v celld >/dev/null 2>&1 || skip "celld not in PATH"

PROJECT="${DEV_PROJECT}"
V_PROD="$(unique_id)"
V_PREVIEW="$(unique_id)"
CRON_EXAMPLE="${E2E_ROOT}/celld/examples/cron"
DEST_PROD="${ARTIFACTS_DIR}/${PROJECT}/${V_PROD}"
DEST_PREVIEW="${ARTIFACTS_DIR}/${PROJECT}/${V_PREVIEW}"
LOG="${EVIDENCE_DIR}/v17-cron-prod-only-e2e.log"
JSON="${EVIDENCE_DIR}/v17-cron-prod-only-e2e.json"
EXIT_CODE=1

celld_log() {
  local vid="$1"
  echo "${TMPDIR:-/tmp}/celld-${PROJECT}-${vid}.log"
}

count_ticks() {
  local f="$1"
  if [[ ! -f "$f" ]]; then
    echo 0
    return
  fi
  grep -c 'e2e-cron-tick' "$f" 2>/dev/null || echo 0
}

write_json() {
  jq -n \
    --arg project "$PROJECT" \
    --arg vprod "$V_PROD" \
    --arg vpreview "$V_PREVIEW" \
    --argjson exit "$EXIT_CODE" \
    --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    '{
      project: $project,
      prod_version: $vprod,
      preview_version: $vpreview,
      exit: $exit,
      timestamp: $ts
    }' >"$JSON"
}

trap 'write_json' EXIT

mkdir -p "$EVIDENCE_DIR"
: >"$LOG"
exec > >(tee -a "$LOG") 2>&1

log "V17 cron prod-only project=${PROJECT} prod=${V_PROD} preview=${V_PREVIEW}"

if [[ ! -d "$CRON_EXAMPLE" ]]; then
  fail "missing ${CRON_EXAMPLE}"
fi

ensure_project "$PROJECT"
cleanup_e2e_versions "$PROJECT"

stage_cron() {
  local dest="$1"
  stage_worker_example "$CRON_EXAMPLE" "$dest"
  sed -i.bak 's/console.log("cron"/console.log("e2e-cron-tick"/' "${dest}/index.js"
  rm -f "${dest}/index.js.bak"
}

stage_cron "$DEST_PROD"
create_version "$PROJECT" "$V_PROD" | jq -r .id >/dev/null
poll_version "$PROJECT" "$V_PROD" ready 180 >/dev/null

PROD_PTR=$(api_get "/v1/projects/${PROJECT}" "$ADMIN_TOKEN" | jq -r .prod_version_id)
if [[ "$PROD_PTR" != "$V_PROD" ]]; then
  fail "expected prod_version_id=${V_PROD}, got ${PROD_PTR}"
fi
log "prod_version_id=${V_PROD}"

stage_cron "$DEST_PREVIEW"
create_version "$PROJECT" "$V_PREVIEW" | jq -r .id >/dev/null
poll_version "$PROJECT" "$V_PREVIEW" ready 180 >/dev/null

PROD_PTR=$(api_get "/v1/projects/${PROJECT}" "$ADMIN_TOKEN" | jq -r .prod_version_id)
if [[ "$PROD_PTR" != "$V_PROD" ]]; then
  fail "preview deploy must not change prod; got ${PROD_PTR}"
fi

for vid in "$V_PROD" "$V_PREVIEW"; do
  api_status GET "/v1/projects/${PROJECT}/versions/${vid}/bindings"
  if [[ "$API_STATUS" != "200" ]]; then
    fail "bindings ${vid} HTTP ${API_STATUS}"
  fi
  if ! echo "$API_BODY" | jq -e '.crons[]? | select(.=="* * * * *")' >/dev/null; then
    fail "bindings ${vid} missing cron expression"
  fi
done
log "both versions declare * * * * * in bindings"

LOG_PROD="$(celld_log "$V_PROD")"
LOG_PREVIEW="$(celld_log "$V_PREVIEW")"
BASE_PROD=$(count_ticks "$LOG_PROD")
BASE_PREVIEW=$(count_ticks "$LOG_PREVIEW")
log "baseline ticks prod=${BASE_PROD} preview=${BASE_PREVIEW} (logs: ${LOG_PROD} ${LOG_PREVIEW})"

log "waiting 90s for minute cron..."
sleep 90

AFTER_PROD=$(count_ticks "$LOG_PROD")
AFTER_PREVIEW=$(count_ticks "$LOG_PREVIEW")
DELTA_PROD=$((AFTER_PROD - BASE_PROD))
DELTA_PREVIEW=$((AFTER_PREVIEW - BASE_PREVIEW))
log "after wait ticks prod=${AFTER_PROD} (+${DELTA_PROD}) preview=${AFTER_PREVIEW} (+${DELTA_PREVIEW})"

if [[ "$DELTA_PROD" -lt 1 ]]; then
  fail "prod should tick at least once in 90s (delta=${DELTA_PROD}); check celld log ${LOG_PROD}"
fi
if [[ "$DELTA_PREVIEW" -gt 0 ]]; then
  fail "preview must not tick (delta=${DELTA_PREVIEW}); check ${LOG_PREVIEW}"
fi

EXIT_CODE=0
pass "V17 cron prod-only OK prod=${V_PROD} preview=${V_PREVIEW}"
cleanup_e2e_versions "$PROJECT" || true
exit 0
