#!/usr/bin/env bash
# TP2-S3 — TTL expiry -> destroyed; active route removed within 300s
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib/common.sh"
stress_source_env
stress_require_tools
stress_ensure_api

GC_MAX="$(stress_threshold STRESS_TTL_GC_SEC 300)"
TTL_SEC="${STRESS_TTL_SECONDS:-60}"

PROJECT="$(stress_project_id ttl)"
VID="$(stress_version_id ttl)"
stress_create_project "$PROJECT"
stress_log "TP2-S3 TTL GC — project=$PROJECT version=$VID ttl=${TTL_SEC}s gc_max=${GC_MAX}s"

http="$(stress_post_version "$PROJECT" "$VID")"
if [[ "$http" != "202" && "$http" != "200" ]]; then
  stress_fail "POST version failed HTTP $http"
fi
stress_wait_version "$PROJECT" "$VID" 600 >/dev/null

# Destroy via DELETE (cellpd API — no POST /destroy route)
destroy_http="$(curl -s -o /tmp/stress-destroy.json -w '%{http_code}' \
  -X DELETE "${PLATFORM_URL}/v1/projects/${PROJECT}/versions/${VID}" \
  -H "$(stress_auth_header)" 2>/dev/null || echo 000)"

stress_log "destroy DELETE HTTP $destroy_http"
sleep "$TTL_SEC"

start=$SECONDS
status=""
while (( SECONDS - start < GC_MAX )); do
  resp="$(stress_get_version "$PROJECT" "$VID" 2>/dev/null || echo '{}')"
  status="$(echo "$resp" | jq -r '.status // empty')"
  routes="$(stress_active_routes_count)"
  route_rows=0
  if command -v sqlite3 >/dev/null && [[ -f "$REGISTRY_DB" ]]; then
    route_rows="$(sqlite3 "$REGISTRY_DB" \
      "SELECT count(*) FROM routes WHERE project_id='${PROJECT}' AND version_id='${VID}' AND active=1;" 2>/dev/null || echo 0)"
  fi
  if [[ "$status" == "destroyed" ]]; then
    if [[ "$route_rows" == "0" || "$route_rows" == "NA" ]]; then
      elapsed=$((SECONDS - start))
      stress_record_metric "TP2-S3" "gc_sec" "$elapsed" "{\"status\":\"$status\",\"route_rows\":$route_rows}"
      stress_pass "TP2-S3 destroyed in ${elapsed}s, project routes=$route_rows (total active=$routes)"
      exit 0
    fi
  fi
  sleep 5
done

stress_fail "TP2-S3: status=$status after ${GC_MAX}s, project routes=${route_rows:-?} (total active=$(stress_active_routes_count))"
