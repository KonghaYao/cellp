#!/usr/bin/env bash
# TP-V0d — offshoot attach probe against RustFS S3 store
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_offshoot
offshoot_rustfs_env

LOG="${EVIDENCE_DIR}/v0d-$(date +%Y%m%d-%H%M%S).log"
STORE="${OFFSHOOT_STORE}"
DB="v0d-probe-$$"

log "offshoot attach probe store=${STORE}"

# Attach-time CAS probe runs on every store-touching command (see offshoot docs).
# init + create exercises attach without rejecting unconditional-write backends.
{
  echo "store=${STORE}"
  echo "endpoint=${OFFSHOOT_S3_ENDPOINT}"
  offshoot -store "$STORE" init
  offshoot -store "$STORE" create "$DB"
  offshoot -store "$STORE" status
} >"$LOG" 2>&1 || {
  cat "$LOG" >&2
  fail "offshoot attach probe failed — see ${LOG}"
}

pass "offshoot attach probe OK (see ${LOG})"
exit 0
