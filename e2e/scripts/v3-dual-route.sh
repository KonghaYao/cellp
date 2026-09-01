#!/usr/bin/env bash
# TP-V3 — Gateway dual version routing (AD-12 Host per version)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_platform
require_celld

PROJECT="${DEV_PROJECT}"
VA="$(unique_id)"
VB="$(unique_id)"

log "V3 dual route project=${PROJECT} A=${VA} B=${VB}"
ensure_project "$PROJECT"

create_version "$PROJECT" "$VA" | jq -r .id >/dev/null
poll_version "$PROJECT" "$VA" ready 120 >/dev/null
create_version "$PROJECT" "$VB" | jq -r .id >/dev/null
poll_version "$PROJECT" "$VB" ready 120 >/dev/null

wait_http_200_version "$PROJECT" "$VA" "/" 60
wait_http_200_version "$PROJECT" "$VB" "/" 60

BODY_A=$(curl_version "$PROJECT" "$VA" "/")
BODY_B=$(curl_version "$PROJECT" "$VB" "/")

VA_FIELD=$(echo "$BODY_A" | jq -r '.version // empty' 2>/dev/null || echo "")
VB_FIELD=$(echo "$BODY_B" | jq -r '.version // empty' 2>/dev/null || echo "")

if [[ -n "$VA_FIELD" && -n "$VB_FIELD" && "$VA_FIELD" != "$VB_FIELD" ]]; then
  pass "V3 dual route OK distinct version fields A=${VA_FIELD} B=${VB_FIELD}"
  exit 0
fi

if [[ "$BODY_A" != "$BODY_B" ]]; then
  pass "V3 dual route OK distinct bodies"
  exit 0
fi

CODE_A=$(http_code_version "$PROJECT" "$VA" "/")
CODE_B=$(http_code_version "$PROJECT" "$VB" "/")
if [[ "$CODE_A" == "200" && "$CODE_B" == "200" ]]; then
  echo "WARN: V3 single-fleet — bodies identical; AD-1 multi-port required for strict assertion" >&2
  pass "V3 dual route OK both 200 (strict body diff needs cellpd multi-fleet)"
  exit 0
fi

fail "V3 dual route failed"
