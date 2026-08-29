#!/usr/bin/env bash
# TP2-C3 — cellpd restart mid-orchestrate; job resumes or failed <=300s
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib/common.sh"
stress_source_env
stress_require_tools
stress_ensure_api

RECOVER_MAX="$(stress_threshold STRESS_CHAOS_JOB_RECOVER_SEC 300)"
PROJECT="$(stress_project_id chaos-c3)"
VID="$(stress_version_id c3)"

stress_create_project "$PROJECT"
stress_log "TP2-C3 cellpd restart mid-orchestrate — project=$PROJECT"

http="$(stress_post_version "$PROJECT" "$VID")"
sleep 2

platform_pid=""
if [[ -f "${STRESS_ROOT}/dev/data/pids/platform.pid" ]]; then
  platform_pid="$(cat "${STRESS_ROOT}/dev/data/pids/platform.pid")"
fi
if [[ -z "$platform_pid" ]] || ! kill -0 "$platform_pid" 2>/dev/null; then
  platform_pid="$(pgrep -f 'cellpd|mock-platform/server.mjs' | head -1 || true)"
fi

if [[ -n "$platform_pid" ]]; then
  stress_log "restart platform pid=$platform_pid"
  kill "$platform_pid" 2>/dev/null || true
  sleep 2
  if [[ -x "${STRESS_ROOT}/dev/data/cellpd" ]]; then
    "${STRESS_ROOT}/dev/data/cellpd" >>"${STRESS_ROOT}/dev/data/logs/cellpd.log" 2>&1 &
    echo $! > "${STRESS_ROOT}/dev/data/pids/platform.pid"
  elif [[ -x "${STRESS_ROOT}/cellpd" ]]; then
    "${STRESS_ROOT}/cellpd" >>"${STRESS_ROOT}/dev/data/logs/cellpd.log" 2>&1 &
    echo $! > "${STRESS_ROOT}/dev/data/pids/platform.pid"
  else
    node "${STRESS_ROOT}/dev/mock-platform/server.mjs" \
      >>"${STRESS_ROOT}/dev/data/logs/platform.log" 2>&1 &
    echo $! > "${STRESS_ROOT}/dev/data/pids/platform.pid"
  fi
  for _ in $(seq 1 30); do
    stress_api_health && break
    sleep 1
  done
else
  stress_log "WARN: platform pid not found"
fi

start=$SECONDS
status=""
while (( SECONDS - start < RECOVER_MAX )); do
  resp="$(stress_get_version "$PROJECT" "$VID" 2>/dev/null || echo '{}')"
  status="$(echo "$resp" | jq -r '.status // empty')"
  if [[ "$status" == "ready" || "$status" == "failed" ]]; then
    break
  fi
  sleep 3
done
elapsed=$((SECONDS - start))
stress_record_metric "TP2-C3" "recover_sec" "$elapsed" "{\"status\":\"$status\"}"

if [[ "$status" == "ready" ]]; then
  stress_pass "TP2-C3 job recovered -> ready in ${elapsed}s"
elif [[ "$status" == "failed" && $elapsed -le $RECOVER_MAX ]]; then
  stress_pass "TP2-C3 terminal failed in ${elapsed}s (acceptable)"
else
  stress_fail "TP2-C3 status=$status after ${elapsed}s"
fi
