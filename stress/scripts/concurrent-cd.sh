#!/usr/bin/env bash
# TP2-L2 + TP2-L3 — concurrent CD (same project ×3, multi-project ×3×2)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib/common.sh"
stress_source_env
stress_require_tools
stress_ensure_api

MAX_SEC="$(stress_threshold STRESS_CONCURRENT_CD_SEC 900)"
stress_log "TP2-L2/L3 concurrent CD — max_sec=${MAX_SEC}"

# --- TP2-L2: 3 concurrent deploys, same project ---
L2_PROJECT="$(stress_project_id l2)"
stress_create_project "$L2_PROJECT"
declare -a l2_pids=()
declare -a l2_vids=()

stress_log "TP2-L2: 3 concurrent deploys on $L2_PROJECT"
for i in 1 2 3; do
  vid="$(stress_version_id "l2-$i")"
  l2_vids+=("$vid")
  (
    t0=$SECONDS
    http="$(stress_post_version "$L2_PROJECT" "$vid")"
    status="$(stress_wait_version "$L2_PROJECT" "$vid" "$MAX_SEC" || echo timeout)"
    elapsed=$((SECONDS - t0))
    echo "${vid}:${status}:${elapsed}:${http}" >"/tmp/stress-l2-${vid}.result"
  ) &
  l2_pids+=($!)
done

for pid in "${l2_pids[@]}"; do wait "$pid" || true; done

l2_ok=0
for vid in "${l2_vids[@]}"; do
  if [[ -f "/tmp/stress-l2-${vid}.result" ]]; then
    IFS=: read -r _ status elapsed http <"/tmp/stress-l2-${vid}.result"
    stress_log "  L2 $vid -> $status in ${elapsed}s (POST $http)"
    if [[ "$status" == "ready" ]] && (( elapsed <= MAX_SEC )); then
      l2_ok=$((l2_ok + 1))
    fi
  fi
done

routes="$(stress_active_routes_count)"
stress_record_metric "TP2-L2" "concurrent_ready" "$l2_ok" "{\"project\":\"$L2_PROJECT\",\"active_routes\":\"$routes\"}"

if (( l2_ok != 3 )); then
  stress_fail "TP2-L2: expected 3/3 ready within ${MAX_SEC}s, got ${l2_ok}/3 (active routes=$routes)"
fi
stress_log "TP2-L2 PASS — active routes=$routes"

# --- TP2-L3: 3 projects × 2 versions, no cross-talk ---
stress_log "TP2-L3: 3 projects × 2 versions"
l3_fail=0
for p in 1 2 3; do
  proj="$(stress_project_id "mp${p}")"
  stress_create_project "$proj"
  for v in 1 2; do
    vid="$(stress_version_id "mp${p}-${v}")"
    http="$(stress_post_version "$proj" "$vid")"
    status="$(stress_wait_version "$proj" "$vid" "$MAX_SEC" || echo timeout)"
    if [[ "$status" != "ready" ]]; then
      stress_log "  $proj/$vid -> $status (POST $http)"
      l3_fail=$((l3_fail + 1))
      continue
    fi
    gw_url="${GATEWAY_URL}/${proj}/${vid}/"
    body="$(curl -sf "$gw_url" 2>/dev/null || echo '{}')"
    proj_field="$(echo "$body" | jq -r '.project // empty')"
    if [[ "$proj_field" != "$proj" && -n "$proj_field" ]]; then
      stress_log "  CROSSTALK: $gw_url body.project=$proj_field expected $proj"
      l3_fail=$((l3_fail + 1))
    else
      stress_log "  $proj/$vid OK (body project=${proj_field:-n/a})"
    fi
  done
done

stress_record_metric "TP2-L3" "cross_talk_failures" "$l3_fail" "{}"
if (( l3_fail > 0 )); then
  stress_fail "TP2-L3: ${l3_fail} cross-talk or deploy failures"
fi

stress_pass "TP2-L2 (3/3 concurrent) + TP2-L3 (3×2 no cross-talk)"
