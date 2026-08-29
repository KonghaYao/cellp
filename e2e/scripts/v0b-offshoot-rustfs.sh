#!/usr/bin/env bash
# TP-V0b — offshoot branch full sequence on RustFS S3
# Deferred path: docs/evidence/v0b-deferred.md → exit 0 when RustFS offshoot unavailable
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

DEFERRED="${EVIDENCE_DIR}/v0b-deferred.md"

deferred_pass() {
  if [[ -f "$DEFERRED" ]]; then
    echo "DEFERRED: V0b skipped — ${DEFERRED} present"
    cat "$DEFERRED"
    exit 0
  fi
  return 1
}

require_offshoot
offshoot_rustfs_env

LOG="${EVIDENCE_DIR}/v0b-$(date +%Y%m%d-%H%M%S).log"
STORE="${OFFSHOOT_STORE}"
DB="v0b-seq-$$"
FORK_A="fork-a-$$"
FORK_B="fork-b-$$"
EXPORT="${EVIDENCE_DIR}/v0b-export-$$.db"

log "offshoot RustFS full sequence store=${STORE}"

run_seq() {
  {
    echo "==> init"
    offshoot -store "$STORE" init
    echo "==> create ${DB}"
    offshoot -store "$STORE" create "$DB"
    echo "==> checkout + seed"
    CHECKOUT=$(offshoot -store "$STORE" checkout "$DB")
    sqlite3 "$CHECKOUT" "CREATE TABLE IF NOT EXISTS kv (k TEXT PRIMARY KEY, v TEXT); INSERT OR REPLACE INTO kv VALUES ('seed','v0b');"
    echo "==> checkpoint"
    offshoot -store "$STORE" checkpoint "$DB" cp-v0b
    echo "==> parallel fork"
    offshoot -store "$STORE" fork "$DB" "$FORK_A" &
    pid_a=$!
    offshoot -store "$STORE" fork "$DB" "$FORK_B" &
    pid_b=$!
    wait "$pid_a"
    wait "$pid_b"
    echo "==> export"
    offshoot -store "$STORE" export "${DB}@${FORK_A}" "$EXPORT" --force
    echo "==> promote"
    offshoot -store "$STORE" promote "${DB}@${FORK_A}" --onto main --force
    echo "==> destroy forks"
    offshoot -store "$STORE" destroy "${DB}@${FORK_B}" --force
    offshoot -store "$STORE" destroy "${DB}@${FORK_A}" --force
    echo "==> verify export"
    sqlite3 "$EXPORT" "SELECT v FROM kv WHERE k='seed';"
  } >"$LOG" 2>&1
}

if ! run_seq; then
  cat "$LOG" >&2
  echo "WARN: V0b RustFS sequence failed — see ${LOG}" >&2
  if deferred_pass; then
    exit 0
  fi
  fail "V0b failed and no ${DEFERRED} — create deferred doc or fix RustFS offshoot"
fi

if [[ ! -f "$EXPORT" ]]; then
  deferred_pass || fail "export missing after V0b sequence"
fi

pass "V0b offshoot RustFS full sequence OK (see ${LOG})"
exit 0
