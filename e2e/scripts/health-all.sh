#!/usr/bin/env bash
# TP-VE-1 — health checks for platform API, gateway, and celld
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

FAIL=0
check() {
  local name="$1" url="$2"
  if curl -sf "$url" >/dev/null 2>&1; then
    pass "$name → 200"
  else
    echo "FAIL: $name → not 200 (${url})" >&2
    FAIL=1
  fi
}

log "health-all"
check "platform API" "${PLATFORM_URL}/v1/health"
check "gateway" "${GATEWAY_URL}/health"
check "celld" "http://127.0.0.1:${CELLD_PORT}/__celld/health"

exit $FAIL
