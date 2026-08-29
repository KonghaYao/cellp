#!/usr/bin/env bash
# TP-VE-4 — Orchestrator failure compensation
# Expects cellpd to honor X-Cellp-Test-Inject: deploy_fail (or _simulate_fail in body)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_platform

PROJECT="${DEV_PROJECT}"
VERSION="$(unique_id)"

log "VE-4 fail compensate project=${PROJECT} version=${VERSION}"
ensure_project "$PROJECT"

BODY=$(jq -n \
  --arg id "$VERSION" \
  '{id:$id, git_ref:"e2e-fail", git_sha:"fail", parent_version_id:null, _simulate_fail:true}')

HTTP=$(curl -sf -o /tmp/ve-fail.json -w '%{http_code}' -X POST \
  "${PLATFORM_URL}/v1/projects/${PROJECT}/versions" \
  -H "$(api_auth "$PLATFORM_TOKEN")" \
  -H "Content-Type: application/json" \
  -H "X-Cellp-Test-Inject: deploy_fail" \
  -d "$BODY" 2>/dev/null || echo "000")

if [[ "$HTTP" == "000" ]]; then
  fail "POST version failed to connect"
fi

# Poll for failed (or immediate failed from mock that ignores inject)
STATUS=""
for _ in $(seq 1 120); do
  STATUS=$(api_get "/v1/projects/${PROJECT}/versions/${VERSION}" 2>/dev/null | jq -r .status 2>/dev/null || echo "")
  [[ "$STATUS" == "failed" ]] && break
  [[ "$STATUS" == "ready" ]] && {
    echo "WARN: inject not supported — version became ready (cellpd required for VE-4)" >&2
    fail "VE-4 requires cellpd deploy_fail injection (got ready)"
  }
  sleep 1
done

[[ "$STATUS" == "failed" ]] || fail "expected status=failed (last=${STATUS})"

# No leaked preview route
PREVIEW="${GATEWAY_URL}/${PROJECT}/${VERSION}/"
CODE=$(http_code "$PREVIEW")
if [[ "$CODE" == "200" ]]; then
  fail "leaked gateway route still 200 for failed version"
fi

pass "VE-4 fail compensate OK status=failed no leaked route"
exit 0
