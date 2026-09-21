#!/usr/bin/env bash
# TP-D1-BRANCH — parent import → child branch → isolation (B3/B4/B5) (AD-12 Host)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"
# shellcheck disable=SC1091
source "$(dirname "$0")/lib-ingress.sh"

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
B5_PID=""
WATCH=""

cleanup_b5_runtime() {
  if [[ -n "$B5_PID" ]] && kill -0 "$B5_PID" 2>/dev/null; then
    kill "$B5_PID" 2>/dev/null || true
    for _ in $(seq 1 25); do
      kill -0 "$B5_PID" 2>/dev/null || break
      sleep 0.2
    done
    if kill -0 "$B5_PID" 2>/dev/null; then
      kill -9 "$B5_PID" 2>/dev/null || true
    fi
    wait "$B5_PID" 2>/dev/null || true
  fi
  [[ -z "$WATCH" ]] || rm -rf "$WATCH"
}
trap cleanup_b5_runtime EXIT

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

# PARENT via Host
wait_http_200_version "$PROJECT" "$PARENT" "/count" 60
PARENT_COUNT=$(curl_version "$PROJECT" "$PARENT" "/count" | jq -r '.count // empty')
if [[ "$PARENT_COUNT" != "$EXPECTED" ]]; then
  fail "parent worker count=${PARENT_COUNT:-?} expected ${EXPECTED}"
fi
log "parent worker count OK=${PARENT_COUNT}"

# --- child version (branch path, same database_id) ---
mkdir -p "$CHILD_DIR"
copy_d1_seed_bundle "$CHILD_DIR"
create_version "$PROJECT" "$CHILD" "$PARENT" | jq -r .id >/dev/null
poll_version "$PROJECT" "$CHILD" ready 180 >/dev/null

# CHILD via Host
wait_http_200_version "$PROJECT" "$CHILD" "/count" 60
CHILD_COUNT=$(curl_version "$PROJECT" "$CHILD" "/count" | jq -r '.count // empty')
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
CHILD_AFTER=$(curl_version "$PROJECT" "$CHILD" "/count" | jq -r '.count // empty')
if [[ "$CHILD_AFTER" != "$((EXPECTED + 1))" ]]; then
  fail "child count after INSERT=${CHILD_AFTER:-?} expected $((EXPECTED + 1))"
fi
log "child INSERT OK count=${CHILD_AFTER}"

# Parent must still see only the original rows.
PARENT_AFTER=$(curl_version "$PROJECT" "$PARENT" "/count" | jq -r '.count // empty')
if [[ "$PARENT_AFTER" != "$EXPECTED" ]]; then
  fail "parent count after child INSERT=${PARENT_AFTER:-?} expected ${EXPECTED} (isolation broken)"
fi
log "parent isolation OK count=${PARENT_AFTER}"

# B5: cold restore from S3 only.
#
# The single scheduler+agent track owns the version's celld processes, and replica ports
# come from a pool shared by every version, so the previous "kill whatever listens on the
# child's registered port and start our own celld there" stole another version's process
# and collided with the manager's own replacement. Retire the child through the platform
# instead (archive stops its replicas and discards their ephemeral watches, wake makes it
# serve again) and let the platform bring a brand-new process up with a brand-new watch:
# that is the same S3-only restore, driven through the production path rather than around
# it. The assertions are unchanged and stronger: the count is read through the Gateway
# Host route served by the platform's own replica, and the new replica's celld log must
# show a remote restore.
REGISTRY_DB="${CELLP_REGISTRY_DB:-${E2E_ROOT}/dev/data/cellp-registry.sqlite}"
if [[ "$REGISTRY_DB" != /* ]]; then
  REGISTRY_DB="${E2E_ROOT}/${REGISTRY_DB#./}"
fi
B5_PREV_REPLICA=$(sqlite3 "$REGISTRY_DB" \
  "SELECT replica_id FROM runtime_replicas WHERE project_id='${PROJECT}' AND version_id='${CHILD}' AND state NOT IN ('stopped','failed') ORDER BY updated_at DESC LIMIT 1;")
log "B5 archive+wake ${PROJECT}/${CHILD} (prev replica ${B5_PREV_REPLICA:-none}); platform restores from ${CHILD_BUCKET} with a fresh watch"
api_status POST "/v1/projects/${PROJECT}/versions/${CHILD}/archive" ""
[[ "$API_STATUS" == "200" ]] || fail "B5 archive child → HTTP ${API_STATUS}"
stopped=0
for _ in $(seq 1 60); do
  remaining=$(sqlite3 "$REGISTRY_DB" \
    "SELECT COUNT(*) FROM runtime_replicas WHERE project_id='${PROJECT}' AND version_id='${CHILD}' AND state NOT IN ('stopped','failed');")
  if [[ "${remaining:-1}" == "0" ]]; then
    stopped=1
    break
  fi
  sleep 1
done
[[ "$stopped" == "1" ]] || fail "B5 child replica did not retire after archive"
api_status POST "/v1/projects/${PROJECT}/versions/${CHILD}/wake" ""
[[ "$API_STATUS" == "200" ]] || fail "B5 wake child → HTTP ${API_STATUS}"
poll_version "$PROJECT" "$CHILD" ready 120 >/dev/null
# The platform serves the child again from a fresh replica: cold activation plus the
# Gateway Host route, with the whole child state restored from the bucket.
wait_http_200_version "$PROJECT" "$CHILD" "/count" 180
B5_COUNT=$(curl_version "$PROJECT" "$CHILD" "/count" | jq -r '.count // empty')
if [[ "$B5_COUNT" != "$CHILD_AFTER" ]]; then
  fail "B5 restore count=${B5_COUNT:-?} expected ${CHILD_AFTER}"
fi
B5_NEW_REPLICA=$(sqlite3 "$REGISTRY_DB" \
  "SELECT replica_id FROM runtime_replicas WHERE project_id='${PROJECT}' AND version_id='${CHILD}' AND state NOT IN ('stopped','failed') ORDER BY updated_at DESC LIMIT 1;")
if [[ -z "$B5_NEW_REPLICA" ]]; then
  fail "B5: no live replica after wake"
fi
if [[ -n "$B5_PREV_REPLICA" && "$B5_NEW_REPLICA" == "$B5_PREV_REPLICA" ]]; then
  fail "B5: wake reused the retired replica ${B5_PREV_REPLICA} (no fresh process)"
fi
# Evidence must come from the new replica's own celld log (the log name carries the
# replica id) and must be a remote restore: a local reuse would mean the state came from
# something other than the bucket.
# celldLogPath encodes the whole "<base64url(version)>.<base64url(replica)>" component a
# second time, so the replica id is only reachable through the doubly-encoded name.
B5_REPLICA_TOKEN=$(python3 -c 'import base64,sys
e=lambda v: base64.urlsafe_b64encode(v.encode()).decode().rstrip("=")
print(e(e(sys.argv[1])+"."+e(sys.argv[2])))' "$CHILD" "$B5_NEW_REPLICA")
B5_LOG_EVIDENCE=0
while IFS= read -r f; do
  [[ -n "$f" && -f "$f" ]] || continue
  [[ "$(basename "$f")" == *"$B5_REPLICA_TOKEN"* ]] || continue
  # A branch child restores through its parent ("restored branched remote replica"); a
  # plain cell reads "restored remote replica". Either way the state came from the bucket.
  if grep -qE "restored (branched )?remote replica" "$f" 2>/dev/null; then
    if grep -qE "resumed clean local replica|reused local eviction snapshot" "$f" 2>/dev/null; then
      fail "B5: new replica served from a local image instead of the bucket ($(basename "$f"))"
    fi
    B5_LOG_EVIDENCE=1
    log "B5 remote-restore evidence in the new replica's log $(basename "$f")"
    break
  fi
done < <(celld_log_paths "$PROJECT" "$CHILD")
if [[ "$B5_LOG_EVIDENCE" != "1" ]]; then
  # The replica's own log is authoritative; enumerating it directly keeps the assertion
  # honest even if the shared helper is unavailable.
  while IFS= read -r f; do
    [[ -n "$f" && -f "$f" ]] || continue
    [[ "$(basename "$f")" == *"$B5_REPLICA_TOKEN"* ]] || continue
    if grep -qE "restored (branched )?remote replica" "$f" 2>/dev/null \
      && ! grep -qE "resumed clean local replica|reused local eviction snapshot" "$f" 2>/dev/null; then
      B5_LOG_EVIDENCE=1
      log "B5 remote-restore evidence in the new replica's log $(basename "$f")"
      break
    fi
  done < <(ls -1t "${TMPDIR:-/tmp}"/celld-*.log 2>/dev/null)
fi
[[ "$B5_LOG_EVIDENCE" == "1" ]] || fail "B5: no remote-restore evidence in new replica ${B5_NEW_REPLICA}'s celld log"
PARENT_STILL=$(curl_version "$PROJECT" "$PARENT" "/count" | jq -r '.count // empty')
[[ "$PARENT_STILL" == "$EXPECTED" ]] || fail "B5 parent count changed: ${PARENT_STILL:-?} expected ${EXPECTED}"
log "B5 cold S3-only restore OK count=${B5_COUNT} parent=${PARENT_STILL} replica ${B5_PREV_REPLICA:-none} → ${B5_NEW_REPLICA}"

pass "D1 branch parent=${EXPECTED} child=${CHILD_COUNT} isolation OK"
