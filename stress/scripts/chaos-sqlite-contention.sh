#!/usr/bin/env bash
# TP2-C5 — SQLite contention: concurrent promote + route updates; no hung >60s
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib/common.sh"
stress_source_env
stress_require_tools
stress_ensure_api

HUNG_MAX="$(stress_threshold STRESS_SQLITE_HUNG_SEC 60)"
WORKERS="${STRESS_SQLITE_WORKERS:-20}"
PROJECT="$(stress_project_id chaos-c5)"

stress_create_project "$PROJECT"
declare -a vids=()
for i in 1 2 3 4; do
  vid="$(stress_version_id "c5-$i")"
  vids+=("$vid")
  stress_post_version "$PROJECT" "$vid" >/dev/null
  stress_wait_version "$PROJECT" "$vid" 600 >/dev/null
done

stress_log "TP2-C5 SQLite contention — ${WORKERS} workers, hung_max=${HUNG_MAX}s"

result_dir="$(mktemp -d)"
trap 'rm -rf "$result_dir"' EXIT

for w in $(seq 1 "$WORKERS"); do
  (
    t0=$SECONDS
    vid="${vids[$((w % ${#vids[@]}))]}"
    stress_promote "$PROJECT" "$vid" >/dev/null
    curl -sf "${GATEWAY_URL}/${PROJECT}/${vid}/" >/dev/null 2>&1 || true
    elapsed=$((SECONDS - t0))
    echo "$elapsed" >"${result_dir}/w${w}.elapsed"
  ) &
done

wait

max_elapsed=0
hung=0
for w in $(seq 1 "$WORKERS"); do
  if [[ -f "${result_dir}/w${w}.elapsed" ]]; then
    e="$(cat "${result_dir}/w${w}.elapsed")"
    if (( e > max_elapsed )); then max_elapsed=$e; fi
    if (( e > HUNG_MAX )); then hung=$((hung + 1)); fi
  else
    hung=$((hung + 1))
  fi
done

stress_record_metric "TP2-C5" "max_elapsed_sec" "$max_elapsed" "{\"hung\":$hung,\"workers\":$WORKERS}"

if (( hung > 0 )); then
  stress_fail "TP2-C5: ${hung} workers hung > ${HUNG_MAX}s (max=${max_elapsed}s)"
fi
stress_pass "TP2-C5 SQLite contention OK (max=${max_elapsed}s)"
