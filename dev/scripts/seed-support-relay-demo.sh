#!/usr/bin/env bash
# Redeploy support-relay with D1 seed + static admin (index.html) + demo short links.
# Usage: ./dev/scripts/seed-support-relay-demo.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

# shellcheck disable=SC1091
source dev/.env
# shellcheck disable=SC1091
source e2e/scripts/lib.sh

PROJECT=support-relay
VERSION=v2
RELAY="${ROOT}/dev/support-corpus/support-relay"
OVERLAY="${ROOT}/dev/examples/support-relay"
DEST="${ARTIFACTS_DIR}/${PROJECT}/${VERSION}"

need() { command -v "$1" >/dev/null || { echo "MISSING: $1" >&2; exit 1; }; }
need sqlite3

require_platform
require_celld

[[ -d "$RELAY" ]] || { echo "Run: ./dev/scripts/deploy-support-app.sh S01 first"; exit 1; }

log() { echo "==> $*"; }

mkdir -p "$DEST"
log "seed.db"
bash "${OVERLAY}/seed.sh" "${DEST}/seed.db"

log "stage worker + static admin"
cp "${RELAY}/worker.js" "${DEST}/worker.js"
cp -R "${OVERLAY}/static" "${DEST}/static"
cp "${OVERLAY}/wrangler.cellp.jsonc" "${DEST}/wrangler.jsonc"

ensure_project "$PROJECT"
log "POST /versions ${VERSION}"
create_version "$PROJECT" "$VERSION" | jq -r .preview_url
poll_version "$PROJECT" "$VERSION" ready 300 >/dev/null

PREVIEW="${GATEWAY_URL}/${PROJECT}/${VERSION}/"
PROD="${GATEWAY_URL}/${PROJECT}/"

curl -sf -X POST "${PLATFORM_URL}/v1/projects/${PROJECT}/versions/${VERSION}/promote" \
  -H "$(api_auth "$ADMIN_TOKEN")" -H "Content-Type: application/json" -d '{}' >/dev/null

log "smoke short links"
for slug in demo cellp relay; do
  code=$(http_code "${PROD}${slug}")
  echo "  GET /${slug} → HTTP ${code}"
done

ADMIN="${PROD}"
ADMIN_CODE=$(http_code "$ADMIN")
echo ""
echo "Relay demo ready (version ${VERSION}, promoted)"
echo "  管理台 (index.html):  ${ADMIN}"
echo "  短链 demo → example:  ${PROD}demo"
echo "  短链 cellp 文档:      ${PROD}cellp"
echo "  短链上游仓库:         ${PROD}relay"
echo "  后台 Token:           cellp-dev-relay-admin"
echo "  Dashboard:            http://127.0.0.1:${DASHBOARD_PORT:-5190}/projects/${PROJECT}"

open "$ADMIN" "${PROD}demo" "${PROD}cellp" 2>/dev/null || true
