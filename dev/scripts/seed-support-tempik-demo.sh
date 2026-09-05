#!/usr/bin/env bash
# Tempik on cellp: D1 schema + gateway API prefix patch + redeploy.
# Usage: ./dev/scripts/seed-support-tempik-demo.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

# shellcheck disable=SC1091
source dev/.env
# shellcheck disable=SC1091
source e2e/scripts/lib.sh
# shellcheck disable=SC1091
source dev/scripts/support-pnpm.sh

PROJECT=support-tempik
CORPUS="${ROOT}/dev/support-corpus/support-tempik"
OVERLAY="${ROOT}/dev/examples/support-tempik"

require_platform
require_celld
need() { command -v "$1" >/dev/null || { echo "MISSING: $1" >&2; exit 1; }; }
need sqlite3

[[ -d "$CORPUS" ]] || { echo "Run: ./dev/scripts/deploy-support-app.sh S03 first"; exit 1; }

log() { echo "==> $*"; }

VERSION="${SUPPORT_VERSION:-}"
if [[ -z "$VERSION" ]]; then
  for n in 3 4 5 6 7 8 9 10; do
    v="v${n}"
    st="$(api_get "/v1/projects/${PROJECT}/versions/${v}" 2>/dev/null | jq -r .status 2>/dev/null || echo absent)"
    [[ "$st" == "absent" || "$st" == "null" ]] && { VERSION="$v"; break; }
    [[ "$st" == "failed" || "$st" == "destroyed" ]] && continue
    VERSION="$v"
    break
  done
fi
VERSION="${VERSION:-v3}"
DEST="${ARTIFACTS_DIR}/${PROJECT}/${VERSION}"

log "version ${VERSION}"
rm -rf "$DEST"
mkdir -p "$DEST"

log "stage app (with node_modules if present)"
if [[ -d "${CORPUS}/node_modules" ]]; then
  rsync -a --exclude .git "${CORPUS}/" "$DEST/"
else
  rsync -a --exclude .git --exclude node_modules "${CORPUS}/" "$DEST/"
  log "pnpm install"
  export NPM_CONFIG_IGNORE_SCRIPTS=true
  (cd "$DEST" && cellp_ensure_pnpm && cellp_pnpm_install)
fi

log "seed.db"
bash "${OVERLAY}/seed.sh" "${DEST}/seed.db"
cp "${OVERLAY}/wrangler.cellp.jsonc" "${DEST}/wrangler.jsonc"
bash "${OVERLAY}/patch-web.sh" "${DEST}/src/web"

ensure_project "$PROJECT"
log "POST /versions ${VERSION}"
create_version "$PROJECT" "$VERSION" | jq -r .preview_url
poll_version "$PROJECT" "$VERSION" ready 180 >/dev/null

PREVIEW="${GATEWAY_URL}/${PROJECT}/${VERSION}/"
PROD="${GATEWAY_URL}/${PROJECT}/"

curl -sf -X POST "${PLATFORM_URL}/v1/projects/${PROJECT}/versions/${VERSION}/promote" \
  -H "$(api_auth "$ADMIN_TOKEN")" -H "Content-Type: application/json" -d '{}' >/dev/null

log "smoke API"
curl -sf "${PREVIEW}api/config" | jq -c .
SESSION=$(curl -sf "${PREVIEW}api/session" | jq -r .sessionId)
echo "session=${SESSION}"
curl -sf -X POST "${PREVIEW}api/inboxes" \
  -H "Content-Type: application/json" \
  -H "x-session-id: ${SESSION}" \
  -d '{"random":true}' | jq -c .

echo ""
echo "Tempik demo ready (${VERSION}, promoted)"
echo "  UI:        ${PREVIEW}"
echo "  mailDomain: local.dev (收信需 CF Email，cellp 暂不支持)"
echo "  Dashboard: http://127.0.0.1:${DASHBOARD_PORT:-5190}/projects/${PROJECT}"

open "$PREVIEW" 2>/dev/null || true
