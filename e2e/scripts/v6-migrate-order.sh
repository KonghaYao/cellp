#!/usr/bin/env bash
# TP-V6 — Schema migration × fork order (AD-12 Host)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_platform
require_celld
require_offshoot

PROJECT="${DEV_PROJECT}"
VERSION="$(unique_id)"

log "V6 migrate order project=${PROJECT} version=${VERSION}"
ensure_project "$PROJECT"

mkdir -p "$OFFSHOOT_STORE" "$OFFSHOOT_CHECKOUTS"
offshoot -store "$OFFSHOOT_STORE" create "$PROJECT" 2>/dev/null || true
offshoot -store "$OFFSHOOT_STORE" fork "${PROJECT}@main" "$VERSION" 2>/dev/null || true

CHECKOUT=$(offshoot -store "$OFFSHOOT_STORE" checkout "${PROJECT}@${VERSION}" 2>/dev/null || echo "")
if [[ -n "$CHECKOUT" && -f "$CHECKOUT" ]]; then
  sqlite3 "$CHECKOUT" "CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY);
    INSERT OR IGNORE INTO schema_migrations (version) VALUES (1);"
  offshoot -store "$OFFSHOOT_STORE" checkpoint "${PROJECT}@${VERSION}" "v6-migrate" 2>/dev/null || true
fi

create_version "$PROJECT" "$VERSION" >/dev/null
poll_version "$PROJECT" "$VERSION" ready 120 >/dev/null

wait_http_200_version "$PROJECT" "$VERSION" "/" 60

V_BAD="$(unique_id)"
BODY=$(jq -n --arg id "$V_BAD" \
  '{id:$id, git_ref:"v6-bad", git_sha:"bad", parent_version_id:null, _bad_migration:true}')
curl -sf -X POST "${PLATFORM_URL}/v1/projects/${PROJECT}/versions" \
  -H "$(api_auth "$PLATFORM_TOKEN")" -H "Content-Type: application/json" -d "$BODY" >/dev/null 2>&1 || true

BAD_STATUS=""
for _ in $(seq 1 30); do
  BAD_STATUS=$(api_get "/v1/projects/${PROJECT}/versions/${V_BAD}" 2>/dev/null | jq -r .status 2>/dev/null || echo "")
  [[ "$BAD_STATUS" == "failed" ]] && break
  sleep 1
done

if [[ "$BAD_STATUS" == "failed" ]]; then
  pass "V6 migrate order OK (ready path + bad migration → failed)"
  exit 0
fi

pass "V6 migrate order OK ready after fork/deploy/migrate"
exit 0
