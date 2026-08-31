#!/usr/bin/env bash
# ISSUE-03 — promote switches prod pointer; does not merge prod writes after child fork
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
  cp "${D1_EXAMPLE}/wrangler.jsonc" "${dest}/wrangler.jsonc"
}

d1_execute() {
  local bucket="$1"
  local artifact_dir="$2"
  local sql="$3"
  celld d1 execute "$DATABASE" \
    --command "$sql" \
    "$artifact_dir" \
    --bucket "$bucket" --endpoint "$S3_ENDPOINT" --region "$AWS_REGION"
}

log "V17 promote no-merge project=${PROJECT} parent=${PARENT} child=${CHILD}"

if ! celld d1 import --help >/dev/null 2>&1; then
  fail "celld d1 import not available"
fi
if ! celld d1 execute --help >/dev/null 2>&1; then
  fail "celld d1 execute not available"
fi

ensure_project "$PROJECT"
cleanup_e2e_versions "$PROJECT"
mkdir -p "$EVIDENCE_DIR" "$PARENT_DIR"

offshoot -store "$OFFSHOOT_STORE" init 2>/dev/null || true
offshoot -store "$OFFSHOOT_STORE" create "$PROJECT" 2>/dev/null || true
CHECKOUT=$(offshoot -store "$OFFSHOOT_STORE" checkout "${PROJECT}@main")
seed_entries_schema | sqlite3 "$CHECKOUT"
write_fixture_rows "$CHECKOUT"
offshoot -store "$OFFSHOOT_STORE" checkpoint "${PROJECT}@main" "v17-${PARENT}" \
  >>"${EVIDENCE_DIR}/v17-promote-no-merge.log" 2>&1 || fail "offshoot checkpoint"
offshoot -store "$OFFSHOOT_STORE" export "${PROJECT}@main" "$EXPORT" --force \
  >>"${EVIDENCE_DIR}/v17-promote-no-merge.log" 2>&1 || fail "offshoot export"
rm -f "${EXPORT}-wal" "${EXPORT}-shm"
EXPECTED=$(seed_checksum "$EXPORT")

copy_d1_seed_bundle "$PARENT_DIR"
create_version "$PROJECT" "$PARENT" | jq -r .id >/dev/null
poll_version "$PROJECT" "$PARENT" ready 180 >/dev/null

PARENT_URL="${GATEWAY_URL}/${PROJECT}/${PARENT}/count"
PROD_URL="${GATEWAY_URL}/${PROJECT}/count"
wait_http_200 "$PARENT_URL" 60

curl -sf -X POST "${PLATFORM_URL}/v1/projects/${PROJECT}/versions/${PARENT}/promote" \
  -H "$(api_auth "$ADMIN_TOKEN")" -H "Content-Type: application/json" -d '{}' \
  >>"${EVIDENCE_DIR}/v17-promote-no-merge.log" 2>&1 || fail "promote parent to prod"

wait_http_200 "$PROD_URL" 60
PROD_BEFORE=$(curl -sf "$PROD_URL" | jq -r '.count // empty')
if [[ "$PROD_BEFORE" != "$EXPECTED" ]]; then
  fail "prod count before fork=${PROD_BEFORE:-?} expected ${EXPECTED}"
fi

mkdir -p "$CHILD_DIR"
copy_d1_seed_bundle "$CHILD_DIR"
create_version "$PROJECT" "$CHILD" "$PARENT" | jq -r .id >/dev/null
poll_version "$PROJECT" "$CHILD" ready 180 >/dev/null

CHILD_URL="${GATEWAY_URL}/${PROJECT}/${CHILD}/count"
wait_http_200 "$CHILD_URL" 60
CHILD_COUNT=$(curl -sf "$CHILD_URL" | jq -r '.count // empty')
if [[ "$CHILD_COUNT" != "$EXPECTED" ]]; then
  fail "child count at fork=${CHILD_COUNT:-?} expected ${EXPECTED}"
fi

PARENT_BUCKET="s3://cellp-celld/${PROJECT}/${PARENT}"
CHILD_BUCKET="s3://cellp-celld/${PROJECT}/${CHILD}"
: "${S3_ENDPOINT:=http://127.0.0.1:19000}"
: "${AWS_REGION:=us-east-1}"

d1_execute "$PARENT_BUCKET" "$PARENT_DIR" \
  "INSERT INTO entries (name, message, at) VALUES ('after-fork-prod-only', 'v17', $(date +%s))" \
  >>"${EVIDENCE_DIR}/v17-promote-no-merge.log" 2>&1 || fail "prod-only INSERT"

PROD_AFTER_FORK=$(curl -sf "$PROD_URL" | jq -r '.count // empty')
if [[ "$PROD_AFTER_FORK" != "$((EXPECTED + 1))" ]]; then
  fail "prod after fork insert=${PROD_AFTER_FORK:-?} expected $((EXPECTED + 1))"
fi

d1_execute "$CHILD_BUCKET" "$CHILD_DIR" \
  "INSERT INTO entries (name, message, at) VALUES ('child-only', 'v17', $(date +%s))" \
  >>"${EVIDENCE_DIR}/v17-promote-no-merge.log" 2>&1 || fail "child INSERT"

CHILD_AFTER=$(curl -sf "$CHILD_URL" | jq -r '.count // empty')
if [[ "$CHILD_AFTER" != "$((EXPECTED + 1))" ]]; then
  fail "child after insert=${CHILD_AFTER:-?} expected $((EXPECTED + 1))"
fi

curl -sf -X POST "${PLATFORM_URL}/v1/projects/${PROJECT}/versions/${CHILD}/promote" \
  -H "$(api_auth "$ADMIN_TOKEN")" -H "Content-Type: application/json" -d '{}' \
  >>"${EVIDENCE_DIR}/v17-promote-no-merge.log" 2>&1 || fail "promote child"

wait_http_200 "$PROD_URL" 60
PROD_FINAL=$(curl -sf "$PROD_URL" | jq -r '.count // empty')
if [[ "$PROD_FINAL" != "$((EXPECTED + 1))" ]]; then
  fail "prod after promote child=${PROD_FINAL:-?} expected $((EXPECTED + 1)) (child bucket, no prod-only row)"
fi

pass "V17 promote no-merge OK prod count=${PROD_FINAL}"
