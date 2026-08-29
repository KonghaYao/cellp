#!/usr/bin/env bash
# TP-VE-2 — CD loop: POST version → poll ready → curl gateway preview
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_platform
require_celld

PROJECT="${DEV_PROJECT}"
VERSION="$(unique_id)"

log "VE-2 CD loop project=${PROJECT} version=${VERSION}"
ensure_project "$PROJECT"

RESP=$(create_version "$PROJECT" "$VERSION")
echo "$RESP" | jq .

POLL=$(echo "$RESP" | jq -r .poll_url)
[[ "$POLL" != "null" && -n "$POLL" ]] || fail "missing poll_url"

poll_version "$PROJECT" "$VERSION" ready 120 >/dev/null

PREVIEW=$(echo "$RESP" | jq -r .preview_url)
[[ -n "$PREVIEW" && "$PREVIEW" != "null" ]] || PREVIEW="${GATEWAY_URL}/${PROJECT}/${VERSION}/"

wait_http_200 "$PREVIEW" 60
BODY=$(curl -sf "$PREVIEW")
echo "$BODY" | jq . 2>/dev/null || echo "$BODY"

pass "VE-2 CD loop OK preview=${PREVIEW}"
exit 0
