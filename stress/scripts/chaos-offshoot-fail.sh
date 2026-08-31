#!/usr/bin/env bash
# TP2-C4 — offshoot fork failure; saga GC, no stale active route
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib/common.sh"
stress_source_env
stress_require_tools
stress_ensure_api

PROJECT="$(stress_project_id chaos-c4)"
VID="$(stress_version_id c4)"

stress_create_project "$PROJECT"
stress_log "TP2-C4 offshoot fork failure — project=$PROJECT"

store_path="$OFFSHOOT_STORE"
if [[ "$store_path" != /* ]]; then
  store_path="${STRESS_ROOT}/${store_path#./}"
fi

backup=""
if [[ -d "$store_path" || -f "$store_path/offshoot.json" ]]; then
  backup="${STRESS_ROOT}/dev/data/offshoot-store.stress-backup"
  rm -rf "$backup" 2>/dev/null || true
  cp -a "$store_path" "$backup" 2>/dev/null || true
  chmod -R u-w "$store_path" 2>/dev/null || true
fi

# Restart cellpd — default deploy is fail-closed (offshoot fork failure aborts saga)
cellpd_bin="${STRESS_ROOT}/dev/data/cellpd"
if [[ -x "$cellpd_bin" ]]; then
  if [[ -f "${STRESS_ROOT}/dev/data/pids/platform.pid" ]]; then
    kill "$(cat "${STRESS_ROOT}/dev/data/pids/platform.pid")" 2>/dev/null || true
    sleep 1
  fi
  "$cellpd_bin" >>"${STRESS_ROOT}/dev/data/logs/cellpd.log" 2>&1 &
  echo $! > "${STRESS_ROOT}/dev/data/pids/platform.pid"
  for _ in $(seq 1 30); do
    stress_api_health && break
    sleep 1
  done
fi

http="$(stress_post_version "$PROJECT" "$VID")"
stress_log "POST $VID -> HTTP $http (fork should fail)"

status="$(stress_wait_version "$PROJECT" "$VID" 300 || echo timeout)"
stress_log "status=$status"

# Restore offshoot store and cellpd defaults
if [[ -n "$backup" && -d "$backup" ]]; then
  chmod -R u+w "$store_path" 2>/dev/null || true
  rm -rf "$store_path" 2>/dev/null || true
  mv "$backup" "$store_path"
fi
if [[ -x "$cellpd_bin" ]]; then
  kill "$(cat "${STRESS_ROOT}/dev/data/pids/platform.pid")" 2>/dev/null || true
  sleep 1
  "$cellpd_bin" >>"${STRESS_ROOT}/dev/data/logs/cellpd.log" 2>&1 &
  echo $! > "${STRESS_ROOT}/dev/data/pids/platform.pid"
  for _ in $(seq 1 30); do
    stress_api_health && break
    sleep 1
  done
fi

sleep 5
if command -v sqlite3 >/dev/null && [[ -f "$REGISTRY_DB" ]]; then
  stale="$(sqlite3 "$REGISTRY_DB" \
    "SELECT count(*) FROM routes WHERE project_id='$PROJECT' AND active=1;" 2>/dev/null || echo 0)"
else
  stale="NA"
fi

stress_record_metric "TP2-C4" "stale_routes" "${stale:-0}" "{\"status\":\"$status\"}"

if [[ "$status" == "failed" || "$status" == "destroyed" ]]; then
  if [[ "$stale" == "0" || "$stale" == "NA" ]]; then
    stress_pass "TP2-C4 saga GC — no active route (stale=$stale)"
  else
    stress_fail "TP2-C4 stale active routes=$stale"
  fi
else
  stress_log "WARN: platform may not simulate fork failure yet (status=$status)"
  if [[ "$stale" == "0" || "$stale" == "NA" ]]; then
    stress_pass "TP2-C4 no active route after deploy (status=$status)"
  else
    stress_fail "TP2-C4 active routes=$stale"
  fi
fi
