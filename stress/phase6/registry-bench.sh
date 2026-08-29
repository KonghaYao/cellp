#!/usr/bin/env bash
# Phase 6A-T5 — benchmark ListProjects / ListVersions latency (p50/p95/p99)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib/common.sh"

scale_source_env
stress_require_tools
stress_ensure_api

SCALE_BENCH_SAMPLES="${SCALE_BENCH_SAMPLES:-200}"
SCALE_BENCH_LIMIT="${SCALE_BENCH_LIMIT:-50}"
SCALE_BENCH_PROJECT="${SCALE_BENCH_PROJECT:-}"

# TP6-A5 thresholds (override via scale-env.json)
LIST_PROJECTS_P99_MAX="$(scale_threshold SCALE_LIST_PROJECTS_P99_MS 200)"
LIST_VERSIONS_P99_MAX="$(scale_threshold SCALE_LIST_VERSIONS_P99_MS 100)"

report_percentiles() {
  local label="$1"
  shift
  local -a lat=("$@")
  local n=${#lat[@]}
  if (( n == 0 )); then
    scale_log "${label}: no samples"
    echo "0:0:0:0"
    return
  fi
  local p50 p95 p99
  p50="$(stress_percentile 50 "${lat[@]}")"
  p95="$(stress_percentile 95 "${lat[@]}")"
  p99="$(stress_percentile 99 "${lat[@]}")"
  scale_log "${label}: n=${n} p50=${p50}ms p95=${p95}ms p99=${p99}ms"
  echo "${p50}:${p95}:${p99}:${n}"
}

bench_list_projects() {
  local -a lat=()
  local cursor=""
  local i
  for ((i = 1; i <= SCALE_BENCH_SAMPLES; i++)); do
    local url="${PLATFORM_URL}/v1/projects?limit=${SCALE_BENCH_LIMIT}"
    if [[ -n "$cursor" ]]; then
      url="${url}&cursor=${cursor}"
    fi
    local t0 body
    t0="$(scale_curl_ms "$url")"
    body="$(scale_fetch_json "$url" 2>/dev/null || echo '{}')"
    lat+=("$t0")
    cursor="$(scale_next_cursor "$body")"
    if [[ -z "$cursor" ]]; then
      cursor=""
    fi
  done
  report_percentiles "ListProjects(limit=${SCALE_BENCH_LIMIT})" "${lat[@]}"
}

bench_list_versions() {
  local project_id="$1"
  stress_assert_prefix "$project_id"
  local -a lat=()
  local cursor=""
  local i
  for ((i = 1; i <= SCALE_BENCH_SAMPLES; i++)); do
    local url="${PLATFORM_URL}/v1/projects/${project_id}/versions?limit=${SCALE_BENCH_LIMIT}"
    if [[ -n "$cursor" ]]; then
      url="${url}&cursor=${cursor}"
    fi
    local t0 body
    t0="$(scale_curl_ms "$url")"
    body="$(scale_fetch_json "$url" 2>/dev/null || echo '{}')"
    lat+=("$t0")
    cursor="$(scale_next_cursor "$body")"
    if [[ -z "$cursor" ]]; then
      cursor=""
    fi
  done
  report_percentiles "ListVersions(${project_id}, limit=${SCALE_BENCH_LIMIT})" "${lat[@]}"
}

resolve_bench_project() {
  if [[ -n "$SCALE_BENCH_PROJECT" ]]; then
    stress_assert_prefix "$SCALE_BENCH_PROJECT"
    echo "$SCALE_BENCH_PROJECT"
    return
  fi
  local body
  body="$(scale_fetch_json "${PLATFORM_URL}/v1/projects?limit=1")"
  local pid
  pid="$(echo "$body" | jq -r '.projects[]?.id // empty' | grep "^${STRESS_PROJECT_PREFIX}-" | head -1 || true)"
  if [[ -z "$pid" ]]; then
    pid="$(scale_project_id 1)"
    scale_log "No existing scale-seed project — using ${pid} (run seed-projects.sh first)"
  fi
  echo "$pid"
}

scale_log "registry-bench: samples=${SCALE_BENCH_SAMPLES} limit=${SCALE_BENCH_LIMIT}"

IFS=: read -r lp50 lp95 lp99 ln <<<"$(bench_list_projects)"
scale_record_metric "TP6-A5" "list_projects_p99_ms" "$lp99" \
  "{\"p50\":$lp50,\"p95\":$lp95,\"samples\":$ln,\"limit\":$SCALE_BENCH_LIMIT}"

bench_project="$(resolve_bench_project)"
IFS=: read -r vp50 vp95 vp99 vn <<<"$(bench_list_versions "$bench_project")"
scale_record_metric "TP6-A5" "list_versions_p99_ms" "$vp99" \
  "{\"p50\":$vp50,\"p95\":$vp95,\"samples\":$vn,\"project\":\"${bench_project}\",\"limit\":$SCALE_BENCH_LIMIT}"

gate_ok=1
if awk -v p="$lp99" -v max="$LIST_PROJECTS_P99_MAX" 'BEGIN { exit !(p < max) }'; then
  scale_log "ListProjects p99 OK (${lp99}ms < ${LIST_PROJECTS_P99_MAX}ms)"
else
  scale_log "ListProjects p99 FAIL (${lp99}ms >= ${LIST_PROJECTS_P99_MAX}ms)"
  gate_ok=0
fi

if awk -v p="$vp99" -v max="$LIST_VERSIONS_P99_MAX" 'BEGIN { exit !(p < max) }'; then
  scale_log "ListVersions p99 OK (${vp99}ms < ${LIST_VERSIONS_P99_MAX}ms)"
else
  scale_log "ListVersions p99 FAIL (${vp99}ms >= ${LIST_VERSIONS_P99_MAX}ms)"
  gate_ok=0
fi

cat <<EOF

=== TP6-A5 Registry Bench Summary ===
ListProjects  p50=${lp50}ms p95=${lp95}ms p99=${lp99}ms (gate <${LIST_PROJECTS_P99_MAX}ms)
ListVersions  p50=${vp50}ms p95=${vp95}ms p99=${vp99}ms project=${bench_project} (gate <${LIST_VERSIONS_P99_MAX}ms)
EOF

if (( gate_ok == 0 )); then
  stress_fail "TP6-A5 registry-bench thresholds not met"
fi

stress_pass "TP6-A5 registry-bench (projects p99=${lp99}ms versions p99=${vp99}ms)"
