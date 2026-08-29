#!/usr/bin/env bash
# Phase 6A-T5 — sustained load on cursor-paginated list APIs (D2/D3)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib/common.sh"

scale_source_env
stress_require_tools
stress_ensure_api

SCALE_LOAD_RPS="${SCALE_LOAD_RPS:-100}"
SCALE_LOAD_DURATION="${SCALE_LOAD_DURATION:-60}"
SCALE_LOAD_LIMIT="${SCALE_LOAD_LIMIT:-50}"
SCALE_LOAD_TARGET="${SCALE_LOAD_TARGET:-projects}"  # projects | versions | both
SCALE_LOAD_PROJECT="${SCALE_LOAD_PROJECT:-}"
SCALE_LOAD_ERR_MAX="$(scale_threshold SCALE_LIST_ERROR_RATE 0.001)"
SCALE_LOAD_P99_MAX="$(scale_threshold SCALE_LIST_P99_MS 200)"

build_targets_file() {
  local tf="$1"
  : >"$tf"
  local project_id
  project_id="$(resolve_load_project)"

  case "$SCALE_LOAD_TARGET" in
    projects)
      echo "GET ${PLATFORM_URL}/v1/projects?limit=${SCALE_LOAD_LIMIT}" >>"$tf"
      ;;
    versions)
      echo "GET ${PLATFORM_URL}/v1/projects/${project_id}/versions?limit=${SCALE_LOAD_LIMIT}" >>"$tf"
      ;;
    both)
      echo "GET ${PLATFORM_URL}/v1/projects?limit=${SCALE_LOAD_LIMIT}" >>"$tf"
      echo "GET ${PLATFORM_URL}/v1/projects/${project_id}/versions?limit=${SCALE_LOAD_LIMIT}" >>"$tf"
      ;;
    *)
      stress_fail "unknown SCALE_LOAD_TARGET=${SCALE_LOAD_TARGET} (use projects|versions|both)"
      ;;
  esac
}

resolve_load_project() {
  if [[ -n "$SCALE_LOAD_PROJECT" ]]; then
    stress_assert_prefix "$SCALE_LOAD_PROJECT"
    echo "$SCALE_LOAD_PROJECT"
    return
  fi
  local body pid
  body="$(scale_fetch_json "${PLATFORM_URL}/v1/projects?limit=10")"
  pid="$(echo "$body" | jq -r '.projects[]?.id // empty' | grep "^${STRESS_PROJECT_PREFIX}-" | head -1 || true)"
  if [[ -z "$pid" ]]; then
    pid="$(scale_project_id 1)"
  fi
  echo "$pid"
}

run_vegeta() {
  local tf="$1"
  local out_dir="$2"
  vegeta attack -rate="${SCALE_LOAD_RPS}" -duration="${SCALE_LOAD_DURATION}s" \
    -header="Authorization: Bearer ${PLATFORM_TOKEN}" \
    <"$tf" | tee "${out_dir}/results.bin" | vegeta report -type=text >"${out_dir}/report.txt"
  vegeta report -type=json <"${out_dir}/results.bin" >"${out_dir}/report.json"
  local p50 p95 p99 errs total
  p50="$(jq -r '.latencies.p50 // 0' "${out_dir}/report.json" | awk '{printf "%.0f", $1/1000000}')"
  p95="$(jq -r '.latencies.p95 // 0' "${out_dir}/report.json" | awk '{printf "%.0f", $1/1000000}')"
  p99="$(jq -r '.latencies.p99 // 0' "${out_dir}/report.json" | awk '{printf "%.0f", $1/1000000}')"
  errs="$(jq -r '.errors // [] | length' "${out_dir}/report.json")"
  total="$(jq -r '.requests // 0' "${out_dir}/report.json")"
  echo "${p50}:${p95}:${p99}:${errs}:${total}"
}

run_curl_loop() {
  local tf="$1"
  local out_dir="$2"
  scale_log "vegeta unavailable — curl pagination loop (${SCALE_LOAD_RPS} rps × ${SCALE_LOAD_DURATION}s)"
  local result="${out_dir}/latencies.tsv"
  : >"$result"
  local end=$((SECONDS + SCALE_LOAD_DURATION))
  local workers="${SCALE_LOAD_WORKERS:-20}"
  local interval_ms=$((1000 * workers / SCALE_LOAD_RPS))
  if (( interval_ms < 1 )); then interval_ms=1; fi

  mapfile -t urls < <(grep -E '^GET ' "$tf" | awk '{print $2}')
  local pids=()
  for ((w = 1; w <= workers; w++)); do
    (
      local url_idx=$(( (w - 1) % ${#urls[@]} ))
      while (( SECONDS < end )); do
        local url="${urls[$url_idx]}"
        local cursor=""
        local page=0
        while [[ -n "$url" || $page -eq 0 ]]; do
          local req_url="$url"
          if [[ $page -gt 0 && -n "$cursor" ]]; then
            if [[ "$req_url" == *'?'* ]]; then
              req_url="${req_url}&cursor=${cursor}"
            else
              req_url="${req_url}?cursor=${cursor}"
            fi
          fi
          local t0 t1 ms code body
          t0=$(date +%s%N)
          body="$(curl -s -w '\n%{http_code}' -H "Authorization: Bearer ${PLATFORM_TOKEN}" "$req_url" 2>/dev/null || printf '\n000')"
          t1=$(date +%s%N)
          code="$(echo "$body" | tail -1)"
          ms=$(( (t1 - t0) / 1000000 ))
          echo "${ms}:${code}" >>"$result"
          if [[ "$code" != "200" ]]; then
            break
          fi
          cursor="$(scale_next_cursor "$(echo "$body" | sed '$d')")"
          if [[ -z "$cursor" ]]; then
            break
          fi
          page=$((page + 1))
        done
        sleep "$(awk -v ms="$interval_ms" 'BEGIN { printf "%.3f", ms/1000 }')"
      done
    ) &
    pids+=($!)
  done
  for pid in "${pids[@]}"; do wait "$pid" || true; done

  declare -a lat=()
  local errs=0 total=0
  while IFS=: read -r ms code; do
    [[ -z "$ms" ]] && continue
    lat+=("$ms")
    total=$((total + 1))
    if [[ "$code" != "200" ]]; then errs=$((errs + 1)); fi
  done <"$result"

  local p50=0 p95=0 p99=0
  if ((${#lat[@]} > 0)); then
    p50="$(stress_percentile 50 "${lat[@]}")"
    p95="$(stress_percentile 95 "${lat[@]}")"
    p99="$(stress_percentile 99 "${lat[@]}")"
  fi
  echo "${p50}:${p95}:${p99}:${errs}:${total}"
}

scale_log "list-api-load: target=${SCALE_LOAD_TARGET} rps=${SCALE_LOAD_RPS} duration=${SCALE_LOAD_DURATION}s limit=${SCALE_LOAD_LIMIT}"

RESULT_DIR="$(mktemp -d)"
trap 'rm -rf "$RESULT_DIR"' EXIT
TARGETS="${RESULT_DIR}/targets.txt"
build_targets_file "$TARGETS"

if command -v vegeta >/dev/null; then
  IFS=: read -r p50 p95 p99 errs total <<<"$(run_vegeta "$TARGETS" "$RESULT_DIR")"
else
  IFS=: read -r p50 p95 p99 errs total <<<"$(run_curl_loop "$TARGETS" "$RESULT_DIR")"
fi

err_rate="$(awk -v e="$errs" -v t="$total" 'BEGIN { if (t==0) print 1; else print e/t }')"
scale_log "results: p50=${p50}ms p95=${p95}ms p99=${p99}ms err_rate=${err_rate} (${errs}/${total})"

scale_record_metric "TP6-A5-load" "list_api_p99_ms" "$p99" \
  "{\"p50\":$p50,\"p95\":$p95,\"error_rate\":$err_rate,\"total\":$total,\"target\":\"${SCALE_LOAD_TARGET}\"}"

gate_ok=1
if awk -v r="$err_rate" -v max="$SCALE_LOAD_ERR_MAX" 'BEGIN { exit !(r < max) }'; then
  scale_log "error rate OK"
else
  scale_log "error rate FAIL (${err_rate} >= ${SCALE_LOAD_ERR_MAX})"
  gate_ok=0
fi
if awk -v p="$p99" -v max="$SCALE_LOAD_P99_MAX" 'BEGIN { exit !(p < max) }'; then
  scale_log "p99 OK"
else
  scale_log "p99 FAIL (${p99}ms >= ${SCALE_LOAD_P99_MAX}ms)"
  gate_ok=0
fi

if (( gate_ok == 0 )); then
  stress_fail "list-api-load thresholds not met"
fi

stress_pass "list-api-load p99=${p99}ms err_rate=${err_rate}"
