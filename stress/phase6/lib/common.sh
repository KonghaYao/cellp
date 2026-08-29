#!/usr/bin/env bash
# Phase 6 scale harness — extends stress/scripts/lib/common.sh
set -euo pipefail

PHASE6_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
PHASE6_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# shellcheck disable=SC1091
source "${PHASE6_ROOT}/stress/scripts/lib/common.sh"

# Safety: only scale-seed-* projects may be created or mutated by phase6 scripts.
STRESS_PROJECT_PREFIX="scale-seed"
export STRESS_PROJECT_PREFIX

SCALE_ENV_JSON="${SCALE_ENV_JSON:-${PHASE6_ROOT}/docs/evidence/scale-env.json}"
SCALE_METRICS="${SCALE_METRICS:-${PHASE6_ROOT}/docs/evidence/scale-metrics.jsonl}"

scale_source_env() {
  stress_source_env
  PLATFORM_TOKEN="${PLATFORM_TOKEN:-${CELLP_ADMIN_TOKEN:-}}"
  export PLATFORM_TOKEN
}

scale_threshold() {
  local key="$1"
  local default="$2"
  if [[ -f "$SCALE_ENV_JSON" ]] && command -v jq >/dev/null; then
    local v
    v="$(jq -r ".thresholds.${key} // .baseline.${key} // empty" "$SCALE_ENV_JSON" 2>/dev/null || true)"
    if [[ -n "$v" && "$v" != "null" ]]; then
      echo "$v"
      return
    fi
  fi
  echo "$default"
}

scale_record_metric() {
  local test_id="$1"
  local metric="$2"
  local value="$3"
  local extra="${4-"{}"}"
  mkdir -p "$(dirname "$SCALE_METRICS")"
  jq -nc \
    --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg test "$test_id" \
    --arg metric "$metric" \
    --argjson value "$value" \
    --argjson extra "$extra" \
    '{ts:$ts,test:$test,metric:$metric,value:$value,extra:$extra}' \
    >>"$SCALE_METRICS"
}

scale_project_id() {
  local index="$1"
  printf '%s-%s-%05d' "${STRESS_PROJECT_PREFIX}" "${STRESS_RUN_ID}" "$index"
}

scale_version_id() {
  local project_index="$1"
  local version_index="$2"
  printf 'v-%s-p%05d-%06d' "${STRESS_RUN_ID}" "$project_index" "$version_index"
}

scale_curl_ms() {
  local url="$1"
  local auth="${2:-1}"
  local args=(-s -o /dev/null -w '%{time_total}')
  if [[ "$auth" == "1" ]]; then
    args+=(-H "$(stress_auth_header)")
  fi
  local t
  t="$(curl "${args[@]}" "$url" 2>/dev/null || echo "999")"
  awk -v t="$t" 'BEGIN { printf "%.0f", t * 1000 }'
}

scale_fetch_json() {
  local url="$1"
  curl -sf "$url" -H "$(stress_auth_header)"
}

scale_next_cursor() {
  local body="$1"
  jq -r '.next_cursor // .pagination.next_cursor // empty' <<<"$body"
}

scale_require_sqlite() {
  stress_need sqlite3
  if [[ ! -f "$REGISTRY_DB" ]]; then
    echo "FAIL: registry db not found at ${REGISTRY_DB}" >&2
    exit 1
  fi
}

scale_log() {
  echo "[phase6 $(date +%H:%M:%S)] $*" >&2
}
