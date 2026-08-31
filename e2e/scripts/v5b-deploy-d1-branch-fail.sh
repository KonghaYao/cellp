#!/usr/bin/env bash
# TP-V5B — D1 branch failure → version failed (default deploy fail-closed)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_platform
require_offshoot
require_celld_cli
need sqlite3

rustfs_s3_env

PROJECT="${DEV_PROJECT}"
PARENT="$(unique_id)"
CHILD="$(unique_id)"
D1_EXAMPLE="${E2E_ROOT}/dev/examples/d1-seed"
PARENT_DIR="${ARTIFACTS_DIR}/${PROJECT}/${PARENT}"
CHILD_DIR="${ARTIFACTS_DIR}/${PROJECT}/${CHILD}"
EXPORT="${PARENT_DIR}/seed.db"

copy_d1_seed_bundle() {
  local dest="$1"
  mkdir -p "$dest"
  cp "${D1_EXAMPLE}/index.js" "${dest}/index.js"
  cp "${D1_EXAMPLE}/wrangler.jsonc" "${dest}/wrangler.jsonc"
}

d1_branch_cli_ready() {
  celld d1 branch -h >/dev/null 2>&1 || return 1
  local out
  out="$(celld d1 branch -h 2>&1)" || true
  ! grep -qiE 'unknown.*subcommand: branch' <<<"$out"
}

restart_cellpd() {
  local cellpd_bin="${E2E_ROOT}/dev/data/cellpd"
  [[ -x "$cellpd_bin" ]] || fail "V5B requires cellpd at ${cellpd_bin}"
  local pid_file="${E2E_ROOT}/dev/data/pids/platform.pid"
  if [[ -f "$pid_file" ]]; then
    kill "$(cat "$pid_file")" 2>/dev/null || true
    sleep 1
  fi
  # shellcheck disable=SC2068
  env "$@" "$cellpd_bin" >>"${E2E_ROOT}/dev/data/logs/cellpd.log" 2>&1 &
  echo $! >"$pid_file"
  for _ in $(seq 1 30); do
    curl -sf "${PLATFORM_URL}/v1/health" >/dev/null 2>&1 && return 0
    sleep 1
  done
  fail "V5B cellpd did not become healthy"
}

restore_cellpd() {
  restart_cellpd
}
trap restore_cellpd EXIT

log "V5B D1 branch fail-closed project=${PROJECT} parent=${PARENT} child=${CHILD}"

if ! celld d1 import --help >/dev/null 2>&1; then
  skip "celld d1 import not available"
fi
if ! d1_branch_cli_ready; then
  skip "celld d1 branch not available"
fi

ensure_project "$PROJECT"
mkdir -p "$EVIDENCE_DIR" "$PARENT_DIR"

copy_d1_seed_bundle "$PARENT_DIR"

offshoot -store "$OFFSHOOT_STORE" init 2>/dev/null || true
offshoot -store "$OFFSHOOT_STORE" create "$PROJECT" 2>/dev/null || true
CHECKOUT=$(offshoot -store "$OFFSHOOT_STORE" checkout "${PROJECT}@main")
[[ -n "$CHECKOUT" && -f "$CHECKOUT" ]] || fail "offshoot checkout failed"

offshoot -store "$OFFSHOOT_STORE" export "${PROJECT}@main" "$EXPORT" --force \
  >>"${EVIDENCE_DIR}/v5b-offshoot.log" 2>&1 || fail "offshoot export"

create_version "$PROJECT" "$PARENT" | jq -r .id >/dev/null
poll_version "$PROJECT" "$PARENT" ready 180 >/dev/null
log "parent ready"

"${E2E_ROOT}/dev/scripts/build-cellpd.sh" >/dev/null 2>&1 || true
restart_cellpd CELLP_E2E_INJECT_D1_BRANCH_FAIL=1

mkdir -p "$CHILD_DIR"
copy_d1_seed_bundle "$CHILD_DIR"
create_version "$PROJECT" "$CHILD" "$PARENT" | jq -r .id >/dev/null

STATUS=""
for _ in $(seq 1 180); do
  STATUS=$(api_get "/v1/projects/${PROJECT}/versions/${CHILD}" 2>/dev/null | jq -r .status 2>/dev/null || echo "")
  [[ "$STATUS" == "failed" ]] && break
  [[ "$STATUS" == "ready" ]] && fail "V5B child became ready with D1 branch inject (fail-closed broken)"
  sleep 1
done
[[ "$STATUS" == "failed" ]] || fail "V5B expected child failed (last=${STATUS})"

PREVIEW="${GATEWAY_URL}/${PROJECT}/${CHILD}/"
CODE=$(http_code "$PREVIEW")
[[ "$CODE" != "200" ]] || fail "V5B leaked gateway route for failed child"

pass "V5B D1 branch inject → failed + no leaked route"
exit 0
