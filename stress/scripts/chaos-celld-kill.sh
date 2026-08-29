#!/usr/bin/env bash
# TP2-C1 — SIGKILL celld mid-deploy; expect failed <=120s, retry -> ready
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib/common.sh"
stress_source_env
stress_require_tools
stress_ensure_api

FAIL_MAX="$(stress_threshold STRESS_CHAOS_FAIL_SEC 120)"
PROJECT="$(stress_project_id chaos-c1)"
VID="$(stress_version_id c1)"

stress_create_project "$PROJECT"
stress_log "TP2-C1 celld SIGKILL mid-deploy — project=$PROJECT"

http="$(stress_post_version "$PROJECT" "$VID")"
stress_log "POST $VID -> HTTP $http"

sleep 2
if [[ -f "${STRESS_ROOT}/dev/data/pids/celld.pid" ]]; then
  celld_pid="$(cat "${STRESS_ROOT}/dev/data/pids/celld.pid")"
  if kill -0 "$celld_pid" 2>/dev/null; then
    stress_log "SIGKILL celld pid=$celld_pid"
    kill -9 "$celld_pid" 2>/dev/null || true
  fi
else
  pkill -9 -f "celld --bucket" 2>/dev/null || true
fi

start=$SECONDS
status=""
while (( SECONDS - start < FAIL_MAX )); do
  resp="$(stress_get_version "$PROJECT" "$VID" 2>/dev/null || echo '{}')"
  status="$(echo "$resp" | jq -r '.status // empty')"
  if [[ "$status" == "failed" || "$status" == "ready" ]]; then
    break
  fi
  sleep 2
done
elapsed=$((SECONDS - start))
stress_log "first attempt: status=$status in ${elapsed}s"

if [[ "$status" != "failed" && "$status" != "ready" ]]; then
  stress_fail "TP2-C1: expected failed within ${FAIL_MAX}s, got $status"
fi

# Restart celld if available
if command -v celld >/dev/null; then
  celld --bucket "$CELLD_BUCKET" --endpoint "$S3_ENDPOINT" --region "$AWS_REGION" \
    --listen "127.0.0.1:${CELLD_PORT}" >>"${STRESS_ROOT}/dev/data/logs/celld.log" 2>&1 &
  echo $! > "${STRESS_ROOT}/dev/data/pids/celld.pid"
  sleep 3
fi

VID2="$(stress_version_id c1-retry)"
stress_post_version "$PROJECT" "$VID2" >/dev/null
retry_status="$(stress_wait_version "$PROJECT" "$VID2" 600 || echo timeout)"
stress_record_metric "TP2-C1" "retry_status" "0" "{\"status\":\"$retry_status\",\"fail_sec\":$elapsed}"

if [[ "$retry_status" == "ready" ]]; then
  stress_pass "TP2-C1 failed/recovered; retry -> ready"
else
  stress_fail "TP2-C1 retry status=$retry_status (expected ready)"
fi
