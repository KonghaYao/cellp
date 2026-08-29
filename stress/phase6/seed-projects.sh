#!/usr/bin/env bash
# Phase 6A-T5 — seed N projects via API (metadata only, no deploy)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib/common.sh"

scale_source_env
stress_require_tools
stress_ensure_api

SCALE_SEED_N="${SCALE_SEED_N:-1000}"
SCALE_SEED_BATCH="${SCALE_SEED_BATCH:-50}"
SCALE_SEED_START="${SCALE_SEED_START:-1}"
DRY_RUN="${SCALE_SEED_DRY_RUN:-0}"

scale_log "seed-projects: N=${SCALE_SEED_N} batch=${SCALE_SEED_BATCH} prefix=${STRESS_PROJECT_PREFIX} run=${STRESS_RUN_ID}"

result_dir="$(mktemp -d)"
trap 'rm -rf "$result_dir"' EXIT

seed_one() {
  local idx="$1"
  local pid
  pid="$(scale_project_id "$idx")"
  stress_assert_prefix "$pid"

  if [[ "$DRY_RUN" == "1" ]]; then
    scale_log "DRY RUN would create ${pid}"
    echo dry >"${result_dir}/${idx}"
    return 0
  fi

  local code
  code="$(curl -s -o /dev/null -w '%{http_code}' \
    -X POST "${PLATFORM_URL}/v1/projects" \
    -H "$(stress_auth_header)" \
    -H "Content-Type: application/json" \
    -d "{\"id\":\"${pid}\"}" 2>/dev/null || echo "000")"

  case "$code" in
    201)
      echo created >"${result_dir}/${idx}"
      ;;
    409|200)
      echo skipped >"${result_dir}/${idx}"
      ;;
    *)
      echo failed >"${result_dir}/${idx}"
      scale_log "WARN: ${pid} HTTP ${code}" >&2
      ;;
  esac
}

count_results() {
  local status="$1"
  local n=0
  local f
  shopt -s nullglob
  for f in "${result_dir}"/*; do
    [[ -f "$f" ]] || continue
    if [[ "$(<"$f")" == "$status" ]]; then
      n=$((n + 1))
    fi
  done
  echo "$n"
}

end=$((SCALE_SEED_START + SCALE_SEED_N - 1))
for ((i = SCALE_SEED_START; i <= end; i += SCALE_SEED_BATCH)); do
  batch_end=$((i + SCALE_SEED_BATCH - 1))
  if (( batch_end > end )); then batch_end=$end; fi
  pids=()
  for ((j = i; j <= batch_end; j++)); do
    seed_one "$j" &
    pids+=($!)
  done
  for pid in "${pids[@]}"; do
    wait "$pid" || echo failed >"${result_dir}/wait-${pid}-$$"
  done
  created=$(count_results created)
  skipped=$(count_results skipped)
  failed=$(count_results failed)
  scale_log "progress: $((batch_end - SCALE_SEED_START + 1))/${SCALE_SEED_N} (created=${created} skipped=${skipped} failed=${failed})"
done

created=$(count_results created)
skipped=$(count_results skipped)
failed=$(count_results failed)

scale_record_metric "TP6-A5-seed" "projects_created" "$created" \
  "{\"requested\":$SCALE_SEED_N,\"skipped\":$skipped,\"failed\":$failed,\"run_id\":\"${STRESS_RUN_ID}\"}"

if (( failed > 0 )); then
  stress_fail "seed-projects: ${failed} failures (created=${created} skipped=${skipped})"
fi

stress_pass "seed-projects: created=${created} skipped=${skipped} prefix=${STRESS_PROJECT_PREFIX}-${STRESS_RUN_ID}-*"
