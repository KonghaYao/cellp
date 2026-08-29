#!/usr/bin/env bash
# Shared helpers for stress harness — source from stress/scripts/*.sh
set -euo pipefail

STRESS_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
STRESS_SCRIPTS="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

STRESS_ENV_JSON="${STRESS_ENV_JSON:-${STRESS_ROOT}/docs/evidence/stress-env.json}"
STRESS_METRICS="${STRESS_METRICS:-${STRESS_ROOT}/docs/evidence/stress-metrics.jsonl}"
STRESS_PROJECT_PREFIX="${STRESS_PROJECT_PREFIX:-stress-demo}"
# Lowercase only — offshoot store rejects uppercase (e.g. ISO8601 "T").
STRESS_RUN_ID="${STRESS_RUN_ID:-$(date +%Y%m%d-%H%M%S)}"

stress_source_env() {
  cd "$STRESS_ROOT"
  set -a
  # shellcheck disable=SC1091
  if [[ -f dev/.env ]]; then
    source dev/.env
  else
    source dev/.env.example
  fi
  set +a
  PLATFORM_URL="${PLATFORM_URL:-http://127.0.0.1:8790}"
  GATEWAY_URL="${GATEWAY_URL:-http://127.0.0.1:8787}"
  REGISTRY_DB="${REGISTRY_DB:-./dev/data/cellp-registry.sqlite}"
  if [[ "$REGISTRY_DB" != /* ]]; then
    REGISTRY_DB="${STRESS_ROOT}/${REGISTRY_DB#./}"
  fi
}

stress_need() {
  local cmd="$1"
  command -v "$cmd" >/dev/null || {
    echo "MISSING required command: $cmd" >&2
    exit 1
  }
}

stress_require_tools() {
  stress_need curl
  stress_need jq
}

stress_assert_prefix() {
  local id="$1"
  if [[ "$id" != "${STRESS_PROJECT_PREFIX}"* ]]; then
    echo "REFUSE: project id '$id' must start with '${STRESS_PROJECT_PREFIX}'" >&2
    exit 1
  fi
}

stress_auth_header() {
  echo "Authorization: Bearer ${PLATFORM_TOKEN:?PLATFORM_TOKEN not set}"
}

stress_api_health() {
  curl -sf "${PLATFORM_URL}/v1/health" >/dev/null
}

stress_ensure_api() {
  if ! stress_api_health; then
    echo "FAIL: cellpd API not reachable at ${PLATFORM_URL}" >&2
    echo "Start stack: ./dev/scripts/up.sh" >&2
    exit 1
  fi
}

stress_threshold() {
  local key="$1"
  local default="$2"
  if [[ -f "$STRESS_ENV_JSON" ]] && command -v jq >/dev/null; then
    local v
    v="$(jq -r ".thresholds.${key} // .baseline.${key} // empty" "$STRESS_ENV_JSON" 2>/dev/null || true)"
    if [[ -n "$v" && "$v" != "null" ]]; then
      echo "$v"
      return
    fi
  fi
  echo "$default"
}

stress_record_metric() {
  local test_id="$1"
  local metric="$2"
  local value="$3"
  local extra="${4-"{}"}"
  mkdir -p "$(dirname "$STRESS_METRICS")"
  jq -nc \
    --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg test "$test_id" \
    --arg metric "$metric" \
    --argjson value "$value" \
    --argjson extra "$extra" \
    '{ts:$ts,test:$test,metric:$metric,value:$value,extra:$extra}' \
    >>"$STRESS_METRICS"
}

stress_project_id() {
  local suffix="${1:-}"
  if [[ -n "$suffix" ]]; then
    echo "${STRESS_PROJECT_PREFIX}-${suffix}-${STRESS_RUN_ID}"
  else
    echo "${STRESS_PROJECT_PREFIX}-${STRESS_RUN_ID}"
  fi
}

stress_version_id() {
  local suffix="${1:-1}"
  if [[ "$suffix" =~ ^[0-9]+$ ]]; then
    printf 'v-%s-%03d' "$STRESS_RUN_ID" "$suffix"
  else
    printf 'v-%s-%s' "$STRESS_RUN_ID" "$suffix"
  fi
}

stress_create_project() {
  local project_id="$1"
  stress_assert_prefix "$project_id"
  curl -sf -X POST "${PLATFORM_URL}/v1/projects" \
    -H "$(stress_auth_header)" \
    -H "Content-Type: application/json" \
    -d "{\"id\":\"${project_id}\"}" >/dev/null 2>&1 || true
}

stress_post_version() {
  local project_id="$1"
  local version_id="$2"
  local parent="${3:-}"
  stress_assert_prefix "$project_id"
  local body
  if [[ -n "$parent" ]]; then
    body="$(jq -nc --arg id "$version_id" --arg parent "$parent" \
      '{id:$id, git_ref:"stress", git_sha:"stress", parent_version_id:$parent}')"
  else
    body="$(jq -nc --arg id "$version_id" \
      '{id:$id, git_ref:"stress", git_sha:"stress", parent_version_id:null}')"
  fi
  local http
  http="$(curl -s -o /tmp/stress-post-version.json -w '%{http_code}' \
    -X POST "${PLATFORM_URL}/v1/projects/${project_id}/versions" \
    -H "$(stress_auth_header)" \
    -H "Content-Type: application/json" \
    -d "$body")"
  echo "$http"
}

stress_get_version() {
  local project_id="$1"
  local version_id="$2"
  curl -sf "${PLATFORM_URL}/v1/projects/${project_id}/versions/${version_id}" \
    -H "$(stress_auth_header)"
}

stress_wait_version() {
  local project_id="$1"
  local version_id="$2"
  local timeout_sec="${3:-900}"
  local start=$SECONDS
  local status=""
  while (( SECONDS - start < timeout_sec )); do
    local resp
    resp="$(stress_get_version "$project_id" "$version_id" 2>/dev/null || echo '{}')"
    status="$(echo "$resp" | jq -r '.status // empty')"
    case "$status" in
      ready|failed|destroyed)
        echo "$status"
        return 0
        ;;
    esac
    sleep 2
  done
  echo "timeout"
  return 1
}

stress_promote() {
  local project_id="$1"
  local version_id="$2"
  curl -sf -X POST \
    "${PLATFORM_URL}/v1/projects/${project_id}/versions/${version_id}/promote" \
    -H "$(stress_auth_header)" \
    -H "Content-Type: application/json" \
    -d '{}' 2>/dev/null || echo "FAIL"
}

stress_active_routes_count() {
  if [[ ! -f "$REGISTRY_DB" ]]; then
    echo "0"
    return
  fi
  if ! command -v sqlite3 >/dev/null; then
    echo "NA"
    return
  fi
  sqlite3 "$REGISTRY_DB" "SELECT count(*) FROM routes WHERE active=1;" 2>/dev/null || echo "NA"
}

stress_process_rss_mb() {
  local pattern="${1:-cellpd}"
  local pid
  pid="$(pgrep -f "$pattern" 2>/dev/null | head -1 || true)"
  if [[ -z "$pid" ]]; then
    # mock platform fallback
    pid="$(pgrep -f 'mock-platform/server.mjs' 2>/dev/null | head -1 || true)"
  fi
  if [[ -z "$pid" ]] || [[ ! -r "/proc/${pid}/status" ]]; then
    echo "0"
    return
  fi
  awk '/VmRSS/ {printf "%.1f", $2/1024}' "/proc/${pid}/status"
}

stress_sqlite_bytes() {
  if [[ -f "$REGISTRY_DB" ]]; then
    stat -c '%s' "$REGISTRY_DB" 2>/dev/null || stat -f '%z' "$REGISTRY_DB" 2>/dev/null || echo "0"
  else
    echo "0"
  fi
}

stress_percentile() {
  local p="$1"
  shift
  local sorted n idx
  sorted="$(printf '%s\n' "$@" | sort -n | sed '/^$/d')"
  n=$(printf '%s\n' "$sorted" | wc -l | tr -d ' ')
  if (( n == 0 )); then
    echo "0"
    return
  fi
  idx=$(( (p * n + 99) / 100 - 1 ))
  if (( idx < 0 )); then idx=0; fi
  if (( idx >= n )); then idx=$((n - 1)); fi
  printf '%s\n' "$sorted" | sed -n "$((idx + 1))p"
}

stress_log() {
  echo "[stress $(date +%H:%M:%S)] $*"
}

stress_fail() {
  echo "FAIL: $*" >&2
  exit 1
}

stress_pass() {
  echo "PASS: $*"
}
