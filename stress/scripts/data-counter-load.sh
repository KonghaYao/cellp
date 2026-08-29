#!/usr/bin/env bash
# TP2-D1 — concurrent counter load: 100 workers × 5min; count = total successful requests
#
# Expected formula:
#   final_n == successful_requests
# Each GET to the counter worker increments durable state by 1.
# With N concurrent workers each issuing R req/s for D seconds:
#   expected_n ≈ N * R * D (minus failed requests)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib/common.sh"
stress_source_env
stress_require_tools
stress_ensure_api

WORKERS="${STRESS_COUNTER_WORKERS:-100}"
DURATION="${STRESS_COUNTER_DURATION:-300}"
RPS_PER_WORKER="${STRESS_COUNTER_RPS:-2}"

PROJECT="$(stress_project_id counter)"
VID="$(stress_version_id counter)"
stress_create_project "$PROJECT"
stress_post_version "$PROJECT" "$VID" >/dev/null
stress_wait_version "$PROJECT" "$VID" 600 >/dev/null

TARGET="${GATEWAY_URL}/${PROJECT}/${VID}/"
stress_log "TP2-D1 counter load — workers=$WORKERS duration=${DURATION}s rps/worker=$RPS_PER_WORKER"
stress_log "URL=$TARGET"

result_dir="$(mktemp -d)"
trap 'rm -rf "$result_dir"' EXIT

for w in $(seq 1 "$WORKERS"); do
  (
    ok=0
    end=$((SECONDS + DURATION))
    while (( SECONDS < end )); do
      if curl -sf -o /dev/null "$TARGET" 2>/dev/null; then
        ok=$((ok + 1))
      fi
      sleep "$(awk -v rps="$RPS_PER_WORKER" 'BEGIN { printf "%.3f", 1/rps }')"
    done
    echo "$ok" >"${result_dir}/w${w}.ok"
  ) &
done
wait

total_ok=0
for w in $(seq 1 "$WORKERS"); do
  if [[ -f "${result_dir}/w${w}.ok" ]]; then
    ok="$(cat "${result_dir}/w${w}.ok")"
    total_ok=$((total_ok + ok))
  fi
done

final_body="$(curl -sf "$TARGET" 2>/dev/null || echo '{}')"
final_n="$(echo "$final_body" | jq -r '.n // 0')"
stress_log "successful_requests=$total_ok final counter n=$final_n"

stress_record_metric "TP2-D1" "final_n" "$final_n" \
  "{\"expected\":$total_ok,\"workers\":$WORKERS,\"duration\":$DURATION}"

# Allow small drift from in-flight requests at sample time
drift=$(( final_n - total_ok ))
if (( drift < 0 )); then drift=$(( -drift )); fi
tolerance=$(( WORKERS * 2 ))
if (( drift <= tolerance )); then
  stress_pass "TP2-D1 counter consistent (n=$final_n expected≈$total_ok drift=$drift)"
else
  stress_fail "TP2-D1 counter mismatch n=$final_n expected≈$total_ok drift=$drift"
fi
