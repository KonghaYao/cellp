#!/usr/bin/env bash
# TP-V5 — Orchestrator failure compensation (AD-12 Host)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_platform

PROJECT="${DEV_PROJECT}"
VERSION="$(unique_id)"

log "V5 saga compensate project=${PROJECT} version=${VERSION}"
ensure_project "$PROJECT"

BODY=$(jq -n --arg id "$VERSION" \
  '{id:$id, git_ref:"v5-fail", git_sha:"fail", parent_version_id:null, _simulate_fail:true}')

curl -sf -X POST "${PLATFORM_URL}/v1/projects/${PROJECT}/versions" \
  -H "$(api_auth "$PLATFORM_TOKEN")" \
  -H "Content-Type: application/json" \
  -H "X-Cellp-Test-Inject: deploy_fail" \
  -d "$BODY" >/dev/null 2>&1 || true

STATUS=""
for _ in $(seq 1 120); do
  STATUS=$(api_get "/v1/projects/${PROJECT}/versions/${VERSION}" 2>/dev/null | jq -r .status 2>/dev/null || echo "")
  [[ "$STATUS" == "failed" ]] && break
  sleep 1
done

[[ "$STATUS" == "failed" ]] || fail "V5 expected failed status (last=${STATUS})"

CODE=$(http_code_version "$PROJECT" "$VERSION" "/")
[[ "$CODE" != "200" ]] || fail "V5 leaked gateway route"

if command -v offshoot >/dev/null 2>&1; then
  offshoot -store "$OFFSHOOT_STORE" destroy "${PROJECT}@${VERSION}" --force 2>/dev/null || true
fi

pass "V5 saga compensate OK failed + no leaked route"
exit 0
