#!/usr/bin/env bash
# TP-V1 — offshoot export → celld D1 binary import → Worker returns fixture count (AD-12 Host)
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
VERSION="$(unique_id)"
FIXTURE_COUNT=42
# wrangler d1_databases[0].database_name — NOT the export/offshoot path
DATABASE="guestbook"
D1_EXAMPLE="${E2E_ROOT}/dev/examples/d1-seed"
DEST_DIR="${ARTIFACTS_DIR}/${PROJECT}/${VERSION}"
EXPORT="${DEST_DIR}/seed.db"

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

log "V1 D1 import project=${PROJECT} version=${VERSION} database=${DATABASE}"
ensure_project "$PROJECT"
cleanup_e2e_versions "$PROJECT"
mkdir -p "$EVIDENCE_DIR" "$DEST_DIR"
# Orchestrator deploys destDir when wrangler.jsonc is present (else counter, no D1).
cp "${D1_EXAMPLE}/index.js" "${DEST_DIR}/index.js"
python3 - "$D1_EXAMPLE/wrangler.jsonc" "${DEST_DIR}/wrangler.jsonc" "$VERSION" <<'PY'
import json, pathlib, sys, uuid
src, dest, version = sys.argv[1], sys.argv[2], sys.argv[3]
text = pathlib.Path(src).read_text()
lines = [ln for ln in text.splitlines() if not ln.lstrip().startswith("//")]
cfg = json.loads("\n".join(lines))
dbs = cfg.get("d1_databases") or []
if not dbs:
    raise SystemExit("d1-seed wrangler missing d1_databases")
dbs[0]["database_id"] = str(uuid.uuid5(uuid.NAMESPACE_URL, f"cellp-e2e-d1-{version}"))
pathlib.Path(dest).write_text(json.dumps(cfg, indent=2) + "\n")
PY

# Seed offshoot main so orchestrator fork+export carries the fixture rows.
offshoot -store "$OFFSHOOT_STORE" init 2>/dev/null || true
offshoot -store "$OFFSHOOT_STORE" create "$PROJECT" 2>/dev/null || true
CHECKOUT=$(offshoot -store "$OFFSHOOT_STORE" checkout "${PROJECT}@main")
if [[ -z "$CHECKOUT" || ! -f "$CHECKOUT" ]]; then
  fail "offshoot checkout ${PROJECT}@main failed"
fi
seed_entries_schema | sqlite3 "$CHECKOUT"
write_fixture_rows "$CHECKOUT"
offshoot -store "$OFFSHOOT_STORE" checkpoint "${PROJECT}@main" "v1-seed" \
  >>"${EVIDENCE_DIR}/v1-offshoot-export.log" 2>&1 || fail "offshoot checkpoint ${PROJECT}@main"

if ! offshoot -store "$OFFSHOOT_STORE" export "${PROJECT}@main" "$EXPORT" --force \
  >>"${EVIDENCE_DIR}/v1-offshoot-export.log" 2>&1; then
  fail "offshoot export ${PROJECT}@main → ${EXPORT}"
fi

rm -f "${EXPORT}-wal" "${EXPORT}-shm"

EXPECTED=$(seed_checksum "$EXPORT")
if [[ "$EXPECTED" != "$FIXTURE_COUNT" ]]; then
  fail "seed.db checksum mismatch: expected ${FIXTURE_COUNT} rows, got ${EXPECTED}"
fi
log "seed.db entries=${EXPECTED}"

if ! celld d1 import --help >/dev/null 2>&1; then
  fail "celld d1 import not available — merge T1b (see docs/plans/D1-IMPORT-RPC.md)"
fi

# Product path: orchestrator Deploy + D1Execute(import) on the version bucket.
# Do not import into the shared demo-app fleet (poisoned guestbook epochs).
create_version "$PROJECT" "$VERSION" | jq -r .id >/dev/null
poll_version "$PROJECT" "$VERSION" ready 120 >/dev/null

VERSION_BUCKET="s3://cellp-celld/${PROJECT}/${VERSION}"
if celld d1 execute --help >/dev/null 2>&1; then
  D1_QUERY_LOG="${EVIDENCE_DIR}/v1-d1-query.log"
  D1_BODY=$(celld d1 execute "$DATABASE" \
    --command "SELECT count(*) AS n FROM entries" "$DEST_DIR" \
    --bucket "$VERSION_BUCKET" --endpoint "$S3_ENDPOINT" --region "$AWS_REGION" \
    --json 2>>"$D1_QUERY_LOG" || true)
  D1_COUNT=$(echo "$D1_BODY" | jq -r 'if type == "array" then .[0].n // .[0].N else .n // .N end' 2>/dev/null || true)
  if [[ -n "$D1_COUNT" && "$D1_COUNT" != "null" ]]; then
    if [[ "$D1_COUNT" != "$EXPECTED" ]]; then
      fail "D1 query checksum mismatch: seed=${EXPECTED} d1=${D1_COUNT}"
    fi
    log "D1 query checksum OK entries=${D1_COUNT} bucket=${VERSION_BUCKET}"
  else
    log "WARN: could not parse celld d1 execute JSON — relying on Worker /count"
  fi
fi

wait_http_200_version "$PROJECT" "$VERSION" "/count" 60
BODY=$(curl_version "$PROJECT" "$VERSION" "/count")
WORKER_COUNT=$(echo "$BODY" | jq -r '.count // empty')

if [[ "$WORKER_COUNT" == "$EXPECTED" ]]; then
  pass "V1 D1 import OK seed=${EXPECTED} worker=${WORKER_COUNT}"
  exit 0
fi

echo "$BODY" >&2
fail "V1 D1 import checksum mismatch: seed=${EXPECTED} worker=${WORKER_COUNT:-unknown}"
