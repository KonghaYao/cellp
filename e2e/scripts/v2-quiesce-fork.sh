#!/usr/bin/env bash
# TP-V2 — Quiesce + checkpoint consistency fork
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_offshoot

PROJECT="v2-quiesce-$$"
PARENT="main"
CHILD="child-$$"
STORE="${OFFSHOOT_STORE:-./dev/data/offshoot-store}"
LOG="${EVIDENCE_DIR}/v2-$(date +%Y%m%d-%H%M%S).log"

log "V2 quiesce fork store=${STORE} db=${PROJECT}"

{
  offshoot -store "$STORE" init 2>/dev/null || true
  offshoot -store "$STORE" create "$PROJECT" 2>/dev/null || true
  CHECKOUT=$(offshoot -store "$STORE" checkout "$PROJECT")
  sqlite3 "$CHECKOUT" "CREATE TABLE IF NOT EXISTS events (id INTEGER PRIMARY KEY AUTOINCREMENT, msg TEXT);
    INSERT INTO events (msg) VALUES ('before-quiesce');"
  offshoot -store "$STORE" checkpoint "$PROJECT" pre-fork
  # Simulate quiesce: no writes after checkpoint
  offshoot -store "$STORE" fork "${PROJECT}@main" "$CHILD"
  CHILD_PATH=$(offshoot -store "$STORE" checkout "${PROJECT}@${CHILD}")
  # Post-fork write on parent only (should not appear in child)
  sqlite3 "$CHECKOUT" "INSERT INTO events (msg) VALUES ('after-fork-parent-only');"
  offshoot -store "$STORE" checkpoint "$PROJECT" post-fork-parent
  COUNT=$(sqlite3 "$CHILD_PATH" "SELECT COUNT(*) FROM events WHERE msg='after-fork-parent-only';")
  echo "child_post_fork_writes=${COUNT}"
} >"$LOG" 2>&1

CHILD_WRITES=$(grep 'child_post_fork_writes=' "$LOG" | tail -1 | cut -d= -f2)
if [[ "${CHILD_WRITES:-1}" == "0" ]]; then
  pass "V2 quiesce fork OK — child has no post-fork parent writes (see ${LOG})"
  exit 0
fi

cat "$LOG" >&2
fail "V2 child version contains post-fork parent writes"
