#!/usr/bin/env bash
# TP-D1-BRANCH — parent import → child branch → isolation (B3/B4/B5)
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
FIXTURE_COUNT=42
DATABASE="guestbook"
D1_EXAMPLE="${E2E_ROOT}/dev/examples/d1-seed"
PARENT_DIR="${ARTIFACTS_DIR}/${PROJECT}/${PARENT}"
CHILD_DIR="${ARTIFACTS_DIR}/${PROJECT}/${CHILD}"
EXPORT="${PARENT_DIR}/seed.db"

seed_entries_schema() {
  cat <<'SQL'
PRAGMA page_size=4096;
CREATE TABLE IF NOT EXISTS entries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  message TEXT NOT NULL,
  at INTEGER NOT NULL
);
DELETE FROM entries;
SQL
}

write_fixture_rows() {
  local db="$1"
  local i now
  now=$(date +%s)
  for i in $(seq 1 "$FIXTURE_COUNT"); do
    sqlite3 "$db" "INSERT INTO entries (name, message, at) VALUES ('seed-${i}', 'fixture', $((now + i)));"
  done
}

seed_checksum() {
  sqlite3 "$1" "SELECT count(*) FROM entries;"
}

copy_d1_seed_bundle() {
  local dest="$1"
  mkdir -p "$dest"
  cp "${D1_EXAMPLE}/index.js" "${dest}/index.js"
  # Fixed database_id from d1-seed — parent and child must share scope.
  cp "${D1_EXAMPLE}/wrangler.jsonc" "${dest}/wrangler.jsonc"
}

d1_branch_cli_ready() {
  if ! celld d1 branch -h >/dev/null 2>&1; then
    return 1
  fi
  local out
  out="$(celld d1 branch -h 2>&1)" || true
  ! grep -qiE 'unknown.*subcommand: branch' <<<"$out"
}

log "D1 branch e2e project=${PROJECT} parent=${PARENT} child=${CHILD}"

if ! celld d1 import --help >/dev/null 2>&1; then
  fail "celld d1 import not available — parent seed requires T1 import path"
fi
if ! d1_branch_cli_ready; then
  fail "celld d1 branch not available — merge T3 (see docs/plans/D1-BRANCH-RPC.md)"
fi

ensure_project "$PROJECT"
cleanup_e2e_versions "$PROJECT"
mkdir -p "$EVIDENCE_DIR" "$PARENT_DIR"

copy_d1_seed_bundle "$PARENT_DIR"

# Seed offshoot main so parent orchestrator import carries fixture rows.
offshoot -store "$OFFSHOOT_STORE" init 2>/dev/null || true
offshoot -store "$OFFSHOOT_STORE" create "$PROJECT" 2>/dev/null || true
CHECKOUT=$(offshoot -store "$OFFSHOOT_STORE" checkout "${PROJECT}@main")
if [[ -z "$CHECKOUT" || ! -f "$CHECKOUT" ]]; then
  fail "offshoot checkout ${PROJECT}@main failed"
fi
seed_entries_schema | sqlite3 "$CHECKOUT"
write_fixture_rows "$CHECKOUT"
offshoot -store "$OFFSHOOT_STORE" checkpoint "${PROJECT}@main" "d1-branch-${PARENT}" \
  >>"${EVIDENCE_DIR}/d1-branch-offshoot.log" 2>&1 || fail "offshoot checkpoint"

if ! offshoot -store "$OFFSHOOT_STORE" export "${PROJECT}@main" "$EXPORT" --force \
  >>"${EVIDENCE_DIR}/d1-branch-offshoot.log" 2>&1; then
  fail "offshoot export → ${EXPORT}"
fi
rm -f "${EXPORT}-wal" "${EXPORT}-shm"
EXPECTED=$(seed_checksum "$EXPORT")
if [[ "$EXPECTED" != "$FIXTURE_COUNT" ]]; then
  fail "parent seed.db expected ${FIXTURE_COUNT} rows, got ${EXPECTED}"
fi
log "parent seed.db entries=${EXPECTED}"

# --- parent version (root import path) ---
create_version "$PROJECT" "$PARENT" | jq -r .id >/dev/null
poll_version "$PROJECT" "$PARENT" ready 180 >/dev/null

PARENT_URL="${GATEWAY_URL}/${PROJECT}/${PARENT}/count"
wait_http_200 "$PARENT_URL" 60
PARENT_COUNT=$(curl -sf "$PARENT_URL" | jq -r '.count // empty')
if [[ "$PARENT_COUNT" != "$EXPECTED" ]]; then
  fail "parent worker count=${PARENT_COUNT:-?} expected ${EXPECTED}"
fi
log "parent worker count OK=${PARENT_COUNT}"

# --- child version (branch path, same database_id) ---
mkdir -p "$CHILD_DIR"
copy_d1_seed_bundle "$CHILD_DIR"
create_version "$PROJECT" "$CHILD" "$PARENT" | jq -r .id >/dev/null
poll_version "$PROJECT" "$CHILD" ready 180 >/dev/null

CHILD_URL="${GATEWAY_URL}/${PROJECT}/${CHILD}/count"
wait_http_200 "$CHILD_URL" 60
CHILD_COUNT=$(curl -sf "$CHILD_URL" | jq -r '.count // empty')
if [[ "$CHILD_COUNT" != "$EXPECTED" ]]; then
  fail "child worker count=${CHILD_COUNT:-?} expected ${EXPECTED} (branch from parent)"
fi
log "child worker count OK=${CHILD_COUNT}"

# Child INSERT via celld d1 execute (isolated child bucket).
CHILD_BUCKET="s3://cellp-celld/${PROJECT}/${CHILD}"
: "${S3_ENDPOINT:=http://127.0.0.1:19000}"
: "${AWS_REGION:=us-east-1}"
if ! celld d1 execute --help >/dev/null 2>&1; then
  fail "celld d1 execute not available — needed for B4 isolation INSERT"
fi
celld d1 execute "$DATABASE" \
  --command "INSERT INTO entries (name, message, at) VALUES ('child-only', 'branch-test', $(date +%s))" \
  "$CHILD_DIR" \
  --bucket "$CHILD_BUCKET" --endpoint "$S3_ENDPOINT" --region "$AWS_REGION" \
  >>"${EVIDENCE_DIR}/d1-branch-child-insert.log" 2>&1 \
  || fail "child INSERT via celld d1 execute"
CHILD_AFTER=$(curl -sf "$CHILD_URL" | jq -r '.count // empty')
if [[ "$CHILD_AFTER" != "$((EXPECTED + 1))" ]]; then
  fail "child count after INSERT=${CHILD_AFTER:-?} expected $((EXPECTED + 1))"
fi
log "child INSERT OK count=${CHILD_AFTER}"

# Parent must still see only the original rows.
PARENT_AFTER=$(curl -sf "$PARENT_URL" | jq -r '.count // empty')
if [[ "$PARENT_AFTER" != "$EXPECTED" ]]; then
  fail "parent count after child INSERT=${PARENT_AFTER:-?} expected ${EXPECTED} (isolation broken)"
fi
log "parent isolation OK count=${PARENT_AFTER}"

# B5: kill child celld, wipe CELLD_WATCH, start a new process on the same port.
# Warm restart / wiping files under a live process does not prove bucket restore.
REGISTRY_DB="${CELLP_REGISTRY_DB:-${E2E_ROOT}/dev/data/cellp-registry.sqlite}"
if [[ "$REGISTRY_DB" != /* ]]; then
  REGISTRY_DB="${E2E_ROOT}/${REGISTRY_DB#./}"
fi
CHILD_PORT=$(sqlite3 "$REGISTRY_DB" \
  "SELECT upstream_port FROM routes WHERE project_id='${PROJECT}' AND version_id='${CHILD}';")
if [[ -z "$CHILD_PORT" || "$CHILD_PORT" == "0" ]]; then
  fail "B5: no upstream_port in registry for ${PROJECT}/${CHILD}"
fi
WATCH="${E2E_ROOT}/dev/data/celld-watch/${PROJECT}/${CHILD}"
log "B5 kill child celld :${CHILD_PORT} then wipe ${WATCH}"
if command -v lsof >/dev/null 2>&1; then
  extra="$(lsof -tiTCP:"${CHILD_PORT}" -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -n "$extra" ]]; then
    # shellcheck disable=SC2086
    kill $extra 2>/dev/null || true
    sleep 1
    extra="$(lsof -tiTCP:"${CHILD_PORT}" -sTCP:LISTEN 2>/dev/null || true)"
    if [[ -n "$extra" ]]; then
      # shellcheck disable=SC2086
      kill -9 $extra 2>/dev/null || true
      sleep 1
    fi
  fi
fi
if [[ -d "$WATCH" ]]; then
  find "$WATCH" -mindepth 1 -maxdepth 1 -exec rm -rf {} +
else
  mkdir -p "$WATCH"
fi
export CELLD_WATCH="$WATCH"
export CELLD_VAR_PROJECT_ID="$PROJECT"
export CELLD_VAR_VERSION_ID="$CHILD"
export CELLD_READY_FLEET_GATE_MS="${CELLD_READY_FLEET_GATE_MS:-5000}"
celld --bucket "$CHILD_BUCKET" --endpoint "$S3_ENDPOINT" --region "$AWS_REGION" \
  --listen "127.0.0.1:${CHILD_PORT}" \
  >>"${EVIDENCE_DIR}/d1-branch-b5-celld.log" 2>&1 &
B5_PID=$!
disown "$B5_PID" 2>/dev/null || true
healthy=0
for _ in $(seq 1 60); do
  if curl -sf "http://127.0.0.1:${CHILD_PORT}/.well-known/celld/health" >/dev/null 2>&1; then
    healthy=1
    break
  fi
  sleep 1
done
if [[ "$healthy" != "1" ]]; then
  fail "B5 new celld not healthy on :${CHILD_PORT} after watch wipe (pid ${B5_PID})"
fi
wait_http_200 "$CHILD_URL" 90
B5_COUNT=$(curl -sf "$CHILD_URL" | jq -r '.count // empty')
if [[ "$B5_COUNT" != "$CHILD_AFTER" ]]; then
  fail "B5 restore count=${B5_COUNT:-?} expected ${CHILD_AFTER}"
fi
log "B5 wipe-watch restore OK count=${B5_COUNT}"

pass "D1 branch parent=${EXPECTED} child=${CHILD_COUNT} isolation OK"
