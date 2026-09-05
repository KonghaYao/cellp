#!/usr/bin/env bash
# TP-V4b — Promote aborts when offshoot promote fails (AD-12 prod Host)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"
# shellcheck disable=SC1091
source "$(dirname "$0")/lib-ingress.sh"

require_platform
require_celld

CELLPD_BIN="${E2E_ROOT}/dev/data/cellpd"
PIDFILE="${E2E_ROOT}/dev/data/pids/platform.pid"
CELLPD_LOG="${E2E_ROOT}/dev/data/logs/cellpd.log"

restart_cellpd() {
  [[ -x "$CELLPD_BIN" ]] || fail "cellpd missing at ${CELLPD_BIN} — run ./dev/scripts/build-cellpd.sh"
  if [[ -f "$PIDFILE" ]]; then
    local pid
    pid="$(cat "$PIDFILE")"
    if kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      for _ in $(seq 1 350); do
        kill -0 "$pid" 2>/dev/null || break
        sleep 0.1
      done
      if kill -0 "$pid" 2>/dev/null; then
        kill -9 "$pid" 2>/dev/null || true
      fi
    fi
  fi
  if [[ $# -gt 0 ]]; then
    env "$@" "$CELLPD_BIN" >>"$CELLPD_LOG" 2>&1 &
  else
    "$CELLPD_BIN" >>"$CELLPD_LOG" 2>&1 &
  fi
  echo $! >"$PIDFILE"
  for _ in $(seq 1 120); do
    curl -sf "${PLATFORM_URL}/v1/health" >/dev/null 2>&1 && return 0
    sleep 1
  done
  fail "cellpd unhealthy after restart"
}

restore_cellpd() {
  restart_cellpd
}
trap restore_cellpd EXIT

PROJECT="${DEV_PROJECT}"
V_OLD="$(unique_id)"
V_NEW="$(unique_id)"

log "V4b promote offshoot gate project=${PROJECT} old=${V_OLD} new=${V_NEW}"
ensure_project "$PROJECT"

create_version "$PROJECT" "$V_OLD" | jq -r .id >/dev/null
poll_version "$PROJECT" "$V_OLD" ready 120 >/dev/null
create_version "$PROJECT" "$V_NEW" | jq -r .id >/dev/null
poll_version "$PROJECT" "$V_NEW" ready 120 >/dev/null

api_status POST "/v1/projects/${PROJECT}/versions/${V_OLD}/promote" '{}'
[[ "$API_STATUS" == "200" ]] || fail "initial promote ${V_OLD} HTTP ${API_STATUS}: ${API_BODY}"

wait_http_200_prod "$PROJECT" "/" 60
PROD_VERSION_BEFORE=$(curl_prod "$PROJECT" "/" | jq -r '.version // empty')
PROD_ID_BEFORE=$(api_get "/v1/projects/${PROJECT}" | jq -r .prod_version_id)

[[ "$PROD_ID_BEFORE" == "$V_OLD" ]] || fail "expected prod ${V_OLD}, got ${PROD_ID_BEFORE}"
[[ "$PROD_VERSION_BEFORE" == "$V_OLD" ]] || fail "expected prod body version ${V_OLD}, got ${PROD_VERSION_BEFORE:-?}"

restart_cellpd CELLP_SKIP_CONTROLLER_GUARD=1 CELLP_E2E_INJECT_OFFSHOOT_PROMOTE_FAIL=1

api_status POST "/v1/projects/${PROJECT}/versions/${V_NEW}/promote" '{}'
if [[ "$API_STATUS" == "200" || "$API_STATUS" == "202" || "$API_STATUS" == "204" ]]; then
  fail "promote should fail with inject, got HTTP ${API_STATUS}"
fi
if ! echo "$API_BODY" | jq -e '.error == "offshoot_promote_failed"' >/dev/null 2>&1; then
  echo "$API_BODY" >&2
  fail "expected error offshoot_promote_failed (HTTP ${API_STATUS})"
fi

PROD_ID_AFTER=$(api_get "/v1/projects/${PROJECT}" | jq -r .prod_version_id)
[[ "$PROD_ID_AFTER" == "$V_OLD" ]] || fail "prod_version_id changed to ${PROD_ID_AFTER}"

PROD_VERSION_AFTER=$(curl_prod "$PROJECT" "/" | jq -r '.version // empty')
[[ "$PROD_VERSION_AFTER" == "$PROD_VERSION_BEFORE" ]] || fail "prod body version changed to ${PROD_VERSION_AFTER:-?} after failed promote"

pass "V4b promote offshoot gate OK (prod stayed ${V_OLD})"
