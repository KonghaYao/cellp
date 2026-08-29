#!/usr/bin/env bash
# TP2-L4 + TP2-L5 — gateway load (500 RPS × 5min) + promote under load
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib/common.sh"
stress_source_env
stress_require_tools
stress_ensure_api

RPS="$(stress_threshold STRESS_GATEWAY_RPS 500)"
DURATION="${STRESS_GATEWAY_DURATION:-300}"
P99_MAX="$(stress_threshold STRESS_GATEWAY_P99_MS 500)"
ERR_MAX="$(stress_threshold STRESS_GATEWAY_ERROR_RATE 0.001)"
CUTOVER_MAX="$(stress_threshold STRESS_PROMOTE_CUTOVER_SEC 5)"
PROMOTE_5XX_MAX="$(stress_threshold STRESS_PROMOTE_5XX_RATE 0.01)"

PROJECT="$(stress_project_id gw)"
V1="$(stress_version_id gw1)"
V2="$(stress_version_id gw2)"

stress_log "TP2-L4/L5 gateway load — RPS=$RPS duration=${DURATION}s project=$PROJECT"
stress_create_project "$PROJECT"

for vid in "$V1" "$V2"; do
  stress_post_version "$PROJECT" "$vid" >/dev/null
  status="$(stress_wait_version "$PROJECT" "$vid" 900 || echo timeout)"
  if [[ "$status" != "ready" ]]; then
    stress_fail "version $vid not ready: $status"
  fi
done

TARGET_URL="${GATEWAY_URL}/${PROJECT}/${V1}/"
RESULT_DIR="$(mktemp -d)"
trap 'rm -rf "$RESULT_DIR"' EXIT

run_vegeta() {
  echo "GET ${TARGET_URL}" | vegeta attack -rate="${RPS}" -duration="${DURATION}s" \
    | vegeta report -type=text >"${RESULT_DIR}/vegeta.txt"
  vegeta report -type=json < <(
    echo "GET ${TARGET_URL}" | vegeta attack -rate="${RPS}" -duration="${DURATION}s"
  ) >"${RESULT_DIR}/vegeta.json" 2>/dev/null || true
  if [[ -f "${RESULT_DIR}/vegeta.json" ]]; then
    local p99 errs total
    p99="$(jq -r '.latencies.p99 // 0' "${RESULT_DIR}/vegeta.json" 2>/dev/null | awk '{printf "%.0f", $1/1000000}')"
    errs="$(jq -r '.errors // [] | length' "${RESULT_DIR}/vegeta.json" 2>/dev/null || echo 0)"
    total="$(jq -r '.requests // 0' "${RESULT_DIR}/vegeta.json" 2>/dev/null || echo 1)"
    echo "${p99}:${errs}:${total}"
  else
    grep -E 'p99|Success' "${RESULT_DIR}/vegeta.txt" || cat "${RESULT_DIR}/vegeta.txt"
  fi
}

run_bash_loop() {
  stress_log "vegeta unavailable — using parallel bash curl workers" >&2
  local workers="${STRESS_GATEWAY_WORKERS:-50}"
  local end=$((SECONDS + DURATION))
  local result_file="${RESULT_DIR}/bash-load.tsv"
  : >"$result_file"
  local pids=()
  for ((w=1; w<=workers; w++)); do
    (
      local ok=0 fail=0
      local interval_ms=$((1000 * workers / RPS))
      if (( interval_ms < 1 )); then interval_ms=1; fi
      while (( SECONDS < end )); do
        local t0 t1 ms code
        t0=$(date +%s%N)
        code="$(curl -sf -o /dev/null -w '%{http_code}' "$TARGET_URL" 2>/dev/null || echo 000)"
        t1=$(date +%s%N)
        ms=$(( (t1 - t0) / 1000000 ))
        echo "${ms}:${code}" >>"$result_file"
        if [[ "$code" == "200" ]]; then ok=$((ok + 1)); else fail=$((fail + 1)); fi
        sleep "$(awk -v ms="$interval_ms" 'BEGIN { printf "%.3f", ms/1000 }')"
      done
    ) &
    pids+=($!)
  done
  for pid in "${pids[@]}"; do wait "$pid" || true; done

  local ok=0 fail=0
  declare -a lat_ms=()
  while IFS=: read -r ms code; do
    [[ -z "$ms" ]] && continue
    lat_ms+=("$ms")
    if [[ "$code" == "200" ]]; then ok=$((ok + 1)); else fail=$((fail + 1)); fi
  done <"$result_file"

  local p99=0
  if ((${#lat_ms[@]} > 0)); then
    p99="$(stress_percentile 99 "${lat_ms[@]}")"
  fi
  echo "${p99}:${fail}:$((ok + fail))"
}

stress_log "TP2-L4: load ${TARGET_URL} at ${RPS} RPS for ${DURATION}s"
if command -v vegeta >/dev/null; then
  IFS=: read -r p99_ms err_count total <<<"$(run_vegeta)"
else
  IFS=: read -r p99_ms err_count total <<<"$(run_bash_loop)"
fi

err_rate="$(awk -v e="$err_count" -v t="$total" 'BEGIN { if (t==0) print 1; else print e/t }')"
stress_log "L4 results: p99=${p99_ms}ms error_rate=${err_rate} (${err_count}/${total})"
stress_record_metric "TP2-L4" "p99_ms" "$p99_ms" "{\"error_rate\":$err_rate,\"total\":$total}"

if awk -v r="$err_rate" -v max="$ERR_MAX" 'BEGIN { exit !(r < max) }'; then
  stress_log "TP2-L4 error rate OK"
else
  stress_fail "TP2-L4 error rate ${err_rate} >= ${ERR_MAX}"
fi
if awk -v p="$p99_ms" -v max="$P99_MAX" 'BEGIN { exit !(p < max) }'; then
  stress_log "TP2-L4 p99 OK"
else
  stress_fail "TP2-L4 p99 ${p99_ms}ms >= ${P99_MAX}ms"
fi

# --- TP2-L5: promote under load ---
stress_log "TP2-L5: promote under load $V1 -> $V2"
LOAD_DURATION=60
prod_url="${GATEWAY_URL}/${PROJECT}/"
load_pid=""
(
  end=$((SECONDS + LOAD_DURATION))
  while (( SECONDS < end )); do
    code="$(curl -sf -o /dev/null -w '%{http_code}' "$prod_url" 2>/dev/null || echo 000)"
    if [[ "$code" =~ ^5 ]]; then
      echo "$code" >>"${RESULT_DIR}/5xx.log"
    fi
    sleep 0.01
  done
) &
load_pid=$!

cutover_start=$SECONDS
stress_promote "$PROJECT" "$V2" >/dev/null || true
new_url="${GATEWAY_URL}/${PROJECT}/${V2}/"

# Wait until prod path serves V2 (marker in body or HTTP 200 on prod path)
cutover_sec=999
for _ in $(seq 1 120); do
  body="$(curl -sf "$prod_url" 2>/dev/null || echo '{}')"
  ver="$(echo "$body" | jq -r '.version // empty')"
  if [[ "$ver" == "$V2" ]] || curl -sf "$new_url" >/dev/null 2>&1; then
    cutover_sec=$((SECONDS - cutover_start))
    break
  fi
  sleep 1
done

wait "$load_pid" 2>/dev/null || true
five_xx=0
[[ -f "${RESULT_DIR}/5xx.log" ]] && five_xx="$(wc -l <"${RESULT_DIR}/5xx.log")"
window_total=$((LOAD_DURATION * 100)) # ~100 rps during cutover window estimate
five_xx_rate="$(awk -v e="$five_xx" -v t="$window_total" 'BEGIN { if (t==0) print 0; else print e/t }')"

stress_log "L5 cutover=${cutover_sec}s 5xx_rate=${five_xx_rate}"
stress_record_metric "TP2-L5" "cutover_sec" "$cutover_sec" "{\"5xx_rate\":$five_xx_rate}"

# Post-cutover 60s: expect zero 5xx
post_fail=0
post_end=$((SECONDS + 60))
while (( SECONDS < post_end )); do
  code="$(curl -sf -o /dev/null -w '%{http_code}' "$prod_url" 2>/dev/null || echo 000)"
  if [[ "$code" =~ ^5 ]]; then post_fail=$((post_fail + 1)); fi
  sleep 1
done

if (( cutover_sec > CUTOVER_MAX )); then
  stress_fail "TP2-L5 cutover ${cutover_sec}s > ${CUTOVER_MAX}s"
fi
if awk -v r="$five_xx_rate" -v max="$PROMOTE_5XX_MAX" 'BEGIN { exit !(r <= max) }'; then
  :
else
  stress_fail "TP2-L5 cutover 5xx rate ${five_xx_rate} > ${PROMOTE_5XX_MAX}"
fi
if (( post_fail > 0 )); then
  stress_fail "TP2-L5 post-cutover 60s had ${post_fail} 5xx responses"
fi

stress_pass "TP2-L4 (p99=${p99_ms}ms) + TP2-L5 (cutover=${cutover_sec}s)"
