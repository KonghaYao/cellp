#!/usr/bin/env bash
# Shared helpers for cellp e2e acceptance scripts.
# shellcheck disable=SC2034
set -euo pipefail

E2E_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$E2E_ROOT"

if [[ -f dev/.env ]]; then
  set -a
  # shellcheck disable=SC1091
  source dev/.env
  set +a
else
  echo "FAIL: dev/.env missing — cp dev/.env.example dev/.env" >&2
  exit 1
fi

: "${PLATFORM_URL:=http://127.0.0.1:8790}"
: "${GATEWAY_URL:=http://127.0.0.1:8787}"
: "${PLATFORM_TOKEN:=dev-local-token}"
: "${ADMIN_TOKEN:=${PLATFORM_TOKEN}}"
: "${DEV_PROJECT:=demo-app}"
: "${CELLD_PORT:=8792}"
: "${EVIDENCE_DIR:=docs/evidence}"

# Normalize paths so scripts can cd into subdirectories safely.
if [[ "${ARTIFACTS_DIR}" != /* ]]; then
  ARTIFACTS_DIR="${E2E_ROOT}/${ARTIFACTS_DIR#./}"
fi
if [[ "${EVIDENCE_DIR}" != /* ]]; then
  EVIDENCE_DIR="${E2E_ROOT}/${EVIDENCE_DIR#./}"
fi
if [[ "${OFFSHOOT_STORE}" != /* && "${OFFSHOOT_STORE}" != s3://* ]]; then
  OFFSHOOT_STORE="${E2E_ROOT}/${OFFSHOOT_STORE#./}"
fi

mkdir -p "$EVIDENCE_DIR" dev/data/{pids,logs,artifacts}

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "FAIL: missing required command: $1" >&2
    exit 1
  }
}

need curl
need jq

cleanup_e2e_versions() {
  local project="${1:-$DEV_PROJECT}"
  local ids
  ids=$(api_get "/v1/projects/${project}" "$ADMIN_TOKEN" 2>/dev/null \
    | jq -r '.versions[]? | select(.id|startswith("v-e2e-")) | select(.status=="ready" or .status=="failed") | .id' 2>/dev/null || true)
  for vid in $ids; do
    [[ -z "$vid" ]] && continue
    api_delete "/v1/projects/${project}/versions/${vid}" "$ADMIN_TOKEN" >/dev/null 2>&1 || true
  done
}

log() { echo "==> $*"; }
pass() { echo "PASS: $*"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

unique_id() {
  echo "v-e2e-$(date +%s)-$RANDOM"
}

api_auth() {
  echo "Authorization: Bearer ${1:-$PLATFORM_TOKEN}"
}

api_get() {
  local path="$1"
  local token="${2:-$PLATFORM_TOKEN}"
  curl -sf -H "$(api_auth "$token")" "${PLATFORM_URL}${path}"
}

api_post() {
  local path="$1"
  local body="$2"
  local token="${3:-$PLATFORM_TOKEN}"
  curl -sf -X POST "${PLATFORM_URL}${path}" \
    -H "$(api_auth "$token")" \
    -H "Content-Type: application/json" \
    -d "$body"
}

api_delete() {
  local path="$1"
  local token="${2:-$ADMIN_TOKEN}"
  curl -sf -X DELETE "${PLATFORM_URL}${path}" \
    -H "$(api_auth "$token")"
}

http_code() {
  curl -s -o /dev/null -w '%{http_code}' "$1" 2>/dev/null || echo "000"
}

ensure_project() {
  local project="${1:-$DEV_PROJECT}"
  api_post "/v1/projects" "{\"id\":\"${project}\"}" >/dev/null 2>&1 || true
}

poll_version() {
  local project="$1"
  local version="$2"
  local want="${3:-ready}"
  local timeout="${4:-120}"
  local i status
  for i in $(seq 1 "$timeout"); do
    status=$(api_get "/v1/projects/${project}/versions/${version}" 2>/dev/null | jq -r .status 2>/dev/null || echo "")
    if [[ "$status" == "$want" ]]; then
      echo "$status"
      return 0
    fi
    if [[ "$status" == "failed" && "$want" != "failed" ]]; then
      api_get "/v1/projects/${project}/versions/${version}" | jq . >&2 || true
      fail "version ${version} failed (wanted ${want})"
    fi
    sleep 1
  done
  fail "timeout waiting for ${version} status=${want} (last=${status:-unknown})"
}

create_version() {
  local project="$1"
  local version="$2"
  local parent="${3:-}"
  local extra="${4:-{}}"
  local body
  if [[ -n "$parent" ]]; then
    body=$(jq -n --arg id "$version" --arg parent "$parent" --argjson extra "$extra" \
      '{id:$id, git_ref:"e2e", git_sha:"local", parent_version_id:$parent} + $extra')
  else
    body=$(jq -n --arg id "$version" --argjson extra "$extra" \
      '{id:$id, git_ref:"e2e", git_sha:"local", parent_version_id:null} + $extra')
  fi
  api_post "/v1/projects/${project}/versions" "$body"
}

wait_http_200() {
  local url="$1"
  local timeout="${2:-60}"
  local i code
  for i in $(seq 1 "$timeout"); do
    code=$(http_code "$url")
    if [[ "$code" == "200" ]]; then
      return 0
    fi
    sleep 1
  done
  fail "expected HTTP 200 from ${url} (last=${code})"
}

wait_http_gone() {
  local url="$1"
  local timeout="${2:-120}"
  local i code
  for i in $(seq 1 "$timeout"); do
    code=$(http_code "$url")
    # 404/410 = route removed; 503 = inactive/draining (cellpd gateway AD-2)
    if [[ "$code" == "404" || "$code" == "410" || "$code" == "503" ]]; then
      return 0
    fi
    sleep 1
  done
  fail "expected HTTP 404/410/503 from ${url} within ${timeout}s (last=${code})"
}

require_celld() {
  if ! curl -sf "http://127.0.0.1:${CELLD_PORT}/__celld/health" >/dev/null 2>&1; then
    fail "celld not healthy on :${CELLD_PORT} — run dev/scripts/up.sh or up-native.sh"
  fi
}

require_platform() {
  if ! curl -sf "${PLATFORM_URL}/v1/health" >/dev/null 2>&1; then
    fail "platform API not healthy at ${PLATFORM_URL} — run dev/scripts/up.sh or up-native.sh"
  fi
}

require_offshoot() {
  command -v offshoot >/dev/null 2>&1 || fail "offshoot CLI not installed"
}

require_celld_cli() {
  command -v celld >/dev/null 2>&1 || fail "celld CLI not installed"
}

rustfs_s3_env() {
  export AWS_ACCESS_KEY_ID="${RUSTFS_ACCESS_KEY:-rustfsadmin}"
  export AWS_SECRET_ACCESS_KEY="${RUSTFS_SECRET_KEY:-rustfsadmin}"
  export AWS_REGION="${AWS_REGION:-us-east-1}"
}

offshoot_rustfs_env() {
  rustfs_s3_env
  export OFFSHOOT_STORE="${OFFSHOOT_RUSTFS_STORE:-s3://cellp-offshoot/e2e}"
  export OFFSHOOT_S3_ENDPOINT="${S3_ENDPOINT:-http://127.0.0.1:${S3_PORT:-19000}}"
  export OFFSHOOT_S3_PATH_STYLE="${OFFSHOOT_S3_PATH_STYLE:-1}"
  export OFFSHOOT_CHECKOUTS="${OFFSHOOT_CHECKOUTS:-./dev/data/offshoot-checkouts-rustfs}"
  mkdir -p "$OFFSHOOT_CHECKOUTS"
}
