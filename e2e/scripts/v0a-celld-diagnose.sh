#!/usr/bin/env bash
# TP-V0a — RustFS × celld conditional write probe
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_celld_cli
rustfs_s3_env

LOG="${EVIDENCE_DIR}/v0a-$(date +%Y%m%d-%H%M%S).log"
BUCKET="${CELLD_BUCKET:-s3://cellp-celld/demo-app}"

log "celld diagnose bucket=${BUCKET} endpoint=${S3_ENDPOINT}"
if ! celld diagnose --bucket "$BUCKET" --endpoint "$S3_ENDPOINT" --region "$AWS_REGION" | tee "$LOG"; then
  fail "celld diagnose exited non-zero — see ${LOG}"
fi

if grep -q 'ok bucket conditional write' "$LOG"; then
  pass "ok bucket conditional write (see ${LOG})"
  exit 0
fi

fail "output missing 'ok bucket conditional write' — see ${LOG}"
