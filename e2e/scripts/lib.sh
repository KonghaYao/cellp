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
: "${DEPLOY_TOKEN:=${PLATFORM_TOKEN}}"
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
  ids=$(api_get "/v1/projects/${project}/versions" "$ADMIN_TOKEN" 2>/dev/null \
    | jq -r '.versions[]? | select(.id|startswith("v-e2e-")) | select(.status=="ready" or .status=="failed") | .id' 2>/dev/null || true)
  for vid in $ids; do
    [[ -z "$vid" ]] && continue
    api_delete "/v1/projects/${project}/versions/${vid}" "$ADMIN_TOKEN" >/dev/null 2>&1 || true
  done
}

log() { echo "==> $*"; }
pass() { echo "PASS: $*"; }
fail() { echo "FAIL: $*" >&2; exit 1; }
skip() { echo "SKIP: $*"; exit 0; }

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

api_put() {
  local path="$1"
  local body="$2"
  local token="${3:-$PLATFORM_TOKEN}"
  curl -sf -X PUT "${PLATFORM_URL}${path}" \
    -H "$(api_auth "$token")" \
    -H "Content-Type: application/json" \
    -d "$body"
}

# api_status METHOD path [json_body] [token]
# Sets API_STATUS and API_BODY (does not fail on 4xx/5xx). Do not wrap in $() — that would lose globals.
api_status() {
  local method="$1"
  local path="$2"
  local token="${4:-$ADMIN_TOKEN}"
  local tmp
  tmp=$(mktemp)
  local args=(-sS -o "$tmp" -w '%{http_code}' -X "$method" -H "$(api_auth "$token")")
  if [[ $# -ge 3 ]]; then
    args+=(-H "Content-Type: application/json" -d "$3")
  fi
  API_STATUS=$(curl "${args[@]}" "${PLATFORM_URL}${path}" 2>/dev/null || echo "000")
  API_BODY=$(cat "$tmp" 2>/dev/null || true)
  rm -f "$tmp"
}

api_delete() {
  local path="$1"
  local token="${2:-$ADMIN_TOKEN}"
  curl -sf -X DELETE "${PLATFORM_URL}${path}" \
    -H "$(api_auth "$token")"
}

# Copy a Worker example (index.js + wrangler.jsonc) into orchestrator destDir.
# Does not rewrite binding ids — KV {ns} / queue names stay verbatim.
stage_worker_example() {
  local src="$1"
  local dest="$2"
  mkdir -p "$dest"
  if [[ ! -f "${src}/index.js" || ! -f "${src}/wrangler.jsonc" ]]; then
    fail "example missing index.js or wrangler.jsonc: ${src}"
  fi
  cp "${src}/index.js" "${dest}/index.js"
  cp "${src}/wrangler.jsonc" "${dest}/wrangler.jsonc"
}

# Standalone runs may skip when the stack is down. run-all.sh already require_platform.
require_stack_or_skip() {
  if curl -sf "${PLATFORM_URL}/v1/health" >/dev/null 2>&1 &&
    curl -sf "http://127.0.0.1:${CELLD_PORT}/.well-known/celld/health" >/dev/null 2>&1; then
    return 0
  fi
  echo "SKIP: celld/stack not running — ${PLATFORM_URL}/v1/health and :${CELLD_PORT}/.well-known/celld/health"
  echo "      start with ./dev/scripts/up.sh or ./dev/scripts/up-native.sh"
  exit 0
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
  local extra="${4:-}"
  if [[ -z "$extra" ]]; then
    extra="{}"
  fi
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

# peek_has_marker JSON MARKER — true when any peek message body (incl. bodyBase64) contains MARKER.
peek_has_marker() {
  local raw="$1"
  local marker="$2"
  python3 -c '
import json, sys, base64
raw, marker = sys.argv[1], sys.argv[2]
try:
    data = json.loads(raw)
except Exception:
    sys.exit(1)
msgs = data.get("messages", data) if isinstance(data, dict) else data
if msgs is None:
    msgs = []
if isinstance(msgs, dict):
    msgs = [msgs]
for m in msgs:
    if not isinstance(m, dict):
        continue
    b64 = m.get("bodyBase64") or m.get("body_base64") or ""
    if b64:
        try:
            body = base64.b64decode(b64).decode("utf-8", "replace")
        except Exception:
            body = ""
        if marker in body:
            sys.exit(0)
    blob = json.dumps(m)
    if marker in blob:
        sys.exit(0)
if marker in raw:
    sys.exit(0)
sys.exit(1)
' "$raw" "$marker"
}

require_celld() {
  if ! curl -sf "http://127.0.0.1:${CELLD_PORT}/.well-known/celld/health" >/dev/null 2>&1; then
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

# Upload local artifact dir to RustFS so orch Fetch(s3://...) succeeds (dev CD).
sync_artifact_to_rustfs() {
  local project="$1"
  local version="$2"
  local src="${ARTIFACTS_DIR}/${project}/${version}"
  local bucket="${ARTIFACTS_BUCKET:-cellp-artifacts}"
  local endpoint="${S3_ENDPOINT:-http://127.0.0.1:19000}"
  [[ -d "$src" ]] || fail "artifact dir missing: $src"
  export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-rustfsadmin}"
  export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-rustfsadmin}"
  export AWS_REGION="${AWS_REGION:-us-east-1}"
  if ! command -v aws >/dev/null 2>&1; then
    if command -v docker >/dev/null 2>&1; then
      docker run --rm --network host \
        -e "AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID}" \
        -e "AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY}" \
        -e "AWS_REGION=${AWS_REGION}" \
        -v "${src}:/src:ro" \
        amazon/aws-cli:2.15.0 s3 sync "/src" "s3://${bucket}/${project}/${version}/" \
        --endpoint-url "$endpoint" --only-show-errors
      return 0
    fi
    echo "WARN: aws CLI and docker missing — orch may fail to fetch s3:// artifact" >&2
    return 0
  fi
  aws s3 sync "$src" "s3://${bucket}/${project}/${version}/" \
    --endpoint-url "$endpoint" --only-show-errors
}
