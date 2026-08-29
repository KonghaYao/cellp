#!/usr/bin/env bash
# TP2-C2 — pause RustFS 30s mid-deploy; no dual-primary; recover + retry
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib/common.sh"
stress_source_env
stress_require_tools
stress_ensure_api

PAUSE_SEC="${STRESS_RUSTFS_PAUSE_SEC:-30}"
PROJECT="$(stress_project_id chaos-c2)"
VID="$(stress_version_id c2)"

stress_create_project "$PROJECT"
stress_log "TP2-C2 RustFS pause ${PAUSE_SEC}s — project=$PROJECT"

http="$(stress_post_version "$PROJECT" "$VID")"
stress_log "POST $VID -> HTTP $http"

container=""
container="$(docker compose -f "${STRESS_ROOT}/dev/docker-compose.yml" ps -q rustfs 2>/dev/null | head -1 || true)"
if [[ -z "$container" ]]; then
  container="$(docker ps --filter "name=rustfs" -q | head -1 || true)"
fi

if [[ -n "$container" ]]; then
  stress_log "docker pause $container for ${PAUSE_SEC}s"
  docker pause "$container" 2>/dev/null || true
  sleep "$PAUSE_SEC"
  docker unpause "$container" 2>/dev/null || true
  stress_log "RustFS unpaused"
else
  stress_log "WARN: RustFS container not found — simulating pause with sleep"
  sleep "$PAUSE_SEC"
fi

status="$(stress_wait_version "$PROJECT" "$VID" 600 || echo timeout)"
stress_log "deploy status after pause: $status"

# Check no duplicate active routes for same project/version
routes="$(stress_active_routes_count)"
if command -v sqlite3 >/dev/null && [[ -f "$REGISTRY_DB" ]]; then
  dupes="$(sqlite3 "$REGISTRY_DB" \
    "SELECT count(*) FROM routes WHERE project_id LIKE '${STRESS_PROJECT_PREFIX}%' AND active=1 GROUP BY project_id, version_id HAVING count(*)>1;" 2>/dev/null || echo 0)"
  if [[ -n "$dupes" && "$dupes" != "0" ]]; then
    stress_fail "TP2-C2: duplicate active routes detected"
  fi
fi

VID2="$(stress_version_id c2-retry)"
stress_post_version "$PROJECT" "$VID2" >/dev/null
retry="$(stress_wait_version "$PROJECT" "$VID2" 600 || echo timeout)"
stress_record_metric "TP2-C2" "retry" "0" "{\"status\":\"$retry\",\"routes\":\"$routes\"}"

if [[ "$retry" == "ready" ]]; then
  stress_pass "TP2-C2 RustFS pause recovered; retry -> ready"
else
  stress_fail "TP2-C2 retry status=$retry"
fi
