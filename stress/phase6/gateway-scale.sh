#!/usr/bin/env bash
# Phase 6A — dev-scoped gateway load baseline (NOT the 100k RPS production gate).
#
# Records cached-route GET latency/error metrics to docs/evidence/scale-metrics.jsonl.
# Full run defaults mirror stress/scripts/gateway-load.sh at laptop-safe RPS (~500).
# Use -short for CI (~60s, lower RPS).
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib/common.sh"

SHORT=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    -short)
      SHORT=1
      shift
      ;;
    -h|--help)
      cat <<'EOF'
Usage: gateway-scale.sh [-short]

Dev gateway load baseline for Phase 6A evidence. Does NOT target 100k RPS.

Env: SCALE_GATEWAY_DEV_RPS, SCALE_GATEWAY_DEV_DURATION, SCALE_GATEWAY_PROJECT,
     SCALE_GATEWAY_VERSION, SCALE_GATEWAY_FIXTURE (skip deploy if route works)
EOF
      exit 0
      ;;
    *)
      echo "unknown arg: $1 (try -short)" >&2
      exit 1
      ;;
  esac
done

scale_source_env
stress_require_tools
stress_ensure_api

if (( SHORT == 1 )); then
  RPS="${SCALE_GATEWAY_DEV_RPS:-50}"
  DURATION="${SCALE_GATEWAY_DEV_DURATION:-60}"
else
  RPS="$(scale_threshold SCALE_GATEWAY_DEV_RPS 500)"
  DURATION="${SCALE_GATEWAY_DEV_DURATION:-120}"
fi

P99_MAX="$(scale_threshold SCALE_GATEWAY_P99_MS 500)"
ERR_MAX="$(scale_threshold SCALE_GATEWAY_ERROR_RATE 0.001)"

RESULT_DIR="$(mktemp -d)"
trap 'rm -rf "$RESULT_DIR"' EXIT

resolve_target_url() {
  local proj ver source="deployed"

  if [[ -n "${SCALE_GATEWAY_PROJECT:-}" && -n "${SCALE_GATEWAY_VERSION:-}" ]]; then
    proj="$SCALE_GATEWAY_PROJECT"
    ver="$SCALE_GATEWAY_VERSION"
    source="configured"
  elif [[ -f "$REGISTRY_DB" ]] && command -v sqlite3 >/dev/null; then
    local row
    row="$(sqlite3 -separator '|' "$REGISTRY_DB" \
      "SELECT project_id, version_id FROM routes WHERE active=1 ORDER BY project_id, version_id LIMIT 1;" 2>/dev/null || true)"
    if [[ -n "$row" ]]; then
      IFS='|' read -r proj ver <<<"$row"
      source="registry"
    fi
  fi

  if [[ -n "${proj:-}" && -n "${ver:-}" ]]; then
    local url="${GATEWAY_URL}/${proj}/${ver}/"
    if curl -sf -o /dev/null "$url" 2>/dev/null; then
      scale_log "using cached route fixture (${source}): ${proj}/${ver}"
      TARGET_URL="$url"
      FIXTURE_PROJECT="$proj"
      FIXTURE_VERSION="$ver"
      FIXTURE_SOURCE="$source"
      return 0
    fi
    scale_log "cached route ${proj}/${ver} not reachable — deploying fixture"
  fi

  if [[ "${SCALE_GATEWAY_FIXTURE:-1}" == "0" ]]; then
    scale_log "no cached route and SCALE_GATEWAY_FIXTURE=0"
    return 1
  fi

  local fixture_idx="${SCALE_GATEWAY_FIXTURE_INDEX:-99999}"
  proj="$(scale_project_id "$fixture_idx")"
  ver="$(scale_version_id "$fixture_idx" 1)"
  stress_assert_prefix "$proj"
  scale_log "deploying gateway fixture project=${proj} version=${ver}"
  stress_create_project "$proj"
  stress_post_version "$proj" "$ver" >/dev/null
  local status
  status="$(stress_wait_version "$proj" "$ver" 900 || echo timeout)"
  if [[ "$status" != "ready" ]]; then
    stress_fail "gateway fixture ${proj}/${ver} not ready: ${status}"
  fi

  TARGET_URL="${GATEWAY_URL}/${proj}/${ver}/"
  FIXTURE_PROJECT="$proj"
  FIXTURE_VERSION="$ver"
  FIXTURE_SOURCE="deployed"
}

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
    echo "0:0:0"
  fi
}

run_bash_loop() {
  scale_log "vegeta unavailable — parallel bash curl workers"
  local workers="${SCALE_GATEWAY_WORKERS:-50}"
  local end=$((SECONDS + DURATION))
  local result_file="${RESULT_DIR}/bash-load.tsv"
  : >"$result_file"
  local pids=()
  for ((w = 1; w <= workers; w++)); do
    (
      while (( SECONDS < end )); do
        local t0 t1 ms code
        t0=$(date +%s%N)
        code="$(curl -sf -o /dev/null -w '%{http_code}' "$TARGET_URL" 2>/dev/null || echo 000)"
        t1=$(date +%s%N)
        ms=$(( (t1 - t0) / 1000000 ))
        echo "${ms}:${code}" >>"$result_file"
        local interval_ms=$((1000 * workers / RPS))
        if (( interval_ms < 1 )); then interval_ms=1; fi
        sleep "$(awk -v ms="$interval_ms" 'BEGIN { printf "%.3f", ms/1000 }')"
      done
    ) &
    pids+=($!)
  done
  for pid in "${pids[@]}"; do wait "$pid" || true; done

  declare -a lat_ms=()
  local fail=0 total=0
  while IFS=: read -r ms code; do
    [[ -z "$ms" ]] && continue
    lat_ms+=("$ms")
    total=$((total + 1))
    if [[ "$code" != "200" ]]; then fail=$((fail + 1)); fi
  done <"$result_file"

  local p99=0
  if ((${#lat_ms[@]} > 0)); then
    p99="$(stress_percentile 99 "${lat_ms[@]}")"
  fi
  echo "${p99}:${fail}:${total}"
}

resolve_target_url || stress_fail "no gateway target URL"

scale_log "gateway-scale: dev baseline rps=${RPS} duration=${DURATION}s short=${SHORT} url=${TARGET_URL}"
scale_log "NOTE: production gate is 100k RPS — this script records local dev evidence only"

if command -v vegeta >/dev/null; then
  IFS=: read -r p99_ms err_count total <<<"$(run_vegeta)"
else
  IFS=: read -r p99_ms err_count total <<<"$(run_bash_loop)"
fi

err_rate="$(awk -v e="$err_count" -v t="$total" 'BEGIN { if (t==0) print 1; else print e/t }')"
scale_log "results: p99=${p99_ms}ms error_rate=${err_rate} (${err_count}/${total})"

scale_record_metric "TP6-A5-gateway" "gateway_p99_ms" "$p99_ms" \
  "{\"rps\":${RPS},\"duration\":${DURATION},\"error_rate\":${err_rate},\"total\":${total},\"short\":${SHORT},\"fixture\":\"${FIXTURE_PROJECT}/${FIXTURE_VERSION}\",\"fixture_source\":\"${FIXTURE_SOURCE}\"}"
scale_record_metric "TP6-A5-gateway" "gateway_rps_cached" "$RPS" \
  "{\"p99_ms\":${p99_ms},\"error_rate\":${err_rate},\"total\":${total},\"short\":${SHORT}}"

gate_ok=1
if awk -v r="$err_rate" -v max="$ERR_MAX" 'BEGIN { exit !(r < max) }'; then
  scale_log "error rate OK"
else
  scale_log "error rate FAIL (${err_rate} >= ${ERR_MAX})"
  gate_ok=0
fi
if awk -v p="$p99_ms" -v max="$P99_MAX" 'BEGIN { exit !(p < max) }'; then
  scale_log "p99 OK"
else
  scale_log "p99 FAIL (${p99_ms}ms >= ${P99_MAX}ms)"
  gate_ok=0
fi

cat <<EOF

=== TP6-A5 Gateway Dev Baseline ===
target:      ${TARGET_URL}
fixture:     ${FIXTURE_PROJECT}/${FIXTURE_VERSION} (${FIXTURE_SOURCE})
rps:         ${RPS}
duration:    ${DURATION}s
p99_ms:      ${p99_ms}
error_rate:  ${err_rate} (${err_count}/${total})
prod_gate:   100k RPS (NOT exercised here)
EOF

if (( gate_ok == 0 )); then
  stress_fail "gateway-scale dev thresholds not met"
fi

stress_pass "gateway-scale dev baseline (rps=${RPS} p99=${p99_ms}ms err_rate=${err_rate})"
