#!/usr/bin/env bash
# r2filebox: build Vue frontend + worker artifact + D1 seed → cellp.
# Usage: ./dev/scripts/seed-support-r2filebox-demo.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

# shellcheck disable=SC1091
source dev/.env
# shellcheck disable=SC1091
source e2e/scripts/lib.sh

PROJECT=support-r2filebox
SID=S17
CORPUS="${ROOT}/dev/support-corpus/${PROJECT}"
OVERLAY="${ROOT}/dev/examples/${PROJECT}"
LOG="${ROOT}/docs/evidence/support-${SID}.log"
mkdir -p "$(dirname "$LOG")"

exec > >(tee "$LOG") 2>&1
echo "=== seed-support-r2filebox $(date -Iseconds) ==="

if [[ ! -d "${CORPUS}/.git" ]]; then
  echo "==> clone (run deploy-support-app S17 once or clone manually)"
  ./dev/scripts/deploy-support-app.sh S17 || true
fi
[[ -d "${CORPUS}/frontend" ]] || { echo "FAIL: missing ${CORPUS}"; exit 1; }

export NPM_CONFIG_IGNORE_SCRIPTS=true
export npm_config_ignore_scripts=true

echo "==> patch frontend API prefix"
bash "${OVERLAY}/patch-frontend.sh" "${CORPUS}/frontend/src/utils/request.ts"
bash "${OVERLAY}/patch-vite.sh" "${CORPUS}/frontend/vite.config.ts"
bash "${OVERLAY}/patch-crypto-fallback.sh" "${CORPUS}"
bash "${OVERLAY}/patch-public-origin.sh" "${CORPUS}"
bash "${OVERLAY}/patch-download.sh" "${CORPUS}"

echo "==> build frontend (may take 1–3 min)"
if [[ "${SUPPORT_SKIP_BUILD:-}" != "1" ]]; then
  (cd "${CORPUS}/frontend" && npm install --no-audit --no-fund @noble/hashes@^1.7.1 && npm install --no-audit --no-fund && npm run build)
  echo "==> root deps (wrangler/hono for bundle)"
  (cd "${CORPUS}" && npm install --no-audit --no-fund)
else
  echo "SKIP build (SUPPORT_SKIP_BUILD=1)"
fi

ensure_project "$PROJECT"
pick_support_version() {
  local n v st
  for n in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
    v="v${n}"
    st="$(api_get "/v1/projects/${PROJECT}/versions/${v}" 2>/dev/null | jq -r .status 2>/dev/null || echo absent)"
    case "$st" in
      absent|null|gone) VERSION="$v"; return ;;
      failed|destroyed) VERSION="$v"; return ;;
      ready)
        [[ "${SUPPORT_FORCE:-}" == "1" ]] && continue
        VERSION="$v"; return ;;
      deploying|pending|starting) VERSION="$v"; return ;;
      *) ;;
    esac
  done
  VERSION="v20"
}
VERSION="${SUPPORT_VERSION:-}"
if [[ -z "$VERSION" ]]; then
  pick_support_version
else
  echo "==> forced version ${VERSION}"
fi
echo "==> version ${VERSION}"

DEST="${ARTIFACTS_DIR}/${PROJECT}/${VERSION}"
rm -rf "$DEST"
mkdir -p "$DEST"

cp "${OVERLAY}/wrangler.cellp.jsonc" "${CORPUS}/wrangler.jsonc"
rm -f "${CORPUS}/wrangler.toml" "${CORPUS}/wrangler.json"
gw_port="${GATEWAY_URL##*:}"
gw_port="${gw_port%%/*}"
deploy_url="${CELLP_PUBLIC_SCHEME_PREVIEW:-https}://$(preview_host "$PROJECT" "$VERSION"):${gw_port:-8788}"
echo "==> inject PUBLIC_BASE_URL=${deploy_url}"
node -e "
const fs = require('fs');
const p = process.argv[1];
const url = process.argv[2];
let j = JSON.parse(fs.readFileSync(p, 'utf8'));
if (j.vars && 'PUBLIC_BASE_URL' in j.vars) j.vars.PUBLIC_BASE_URL = url;
let raw = JSON.stringify(j, null, 2) + '\n';
raw = raw.replace(/:\\/\\//g, ':\\\\u002f\\\\u002f');
fs.writeFileSync(p, raw);
" "${CORPUS}/wrangler.jsonc" "$deploy_url"
echo "==> prepare worker bundle (wrangler dry-run)"
bash "${OVERLAY}/prepare-artifact.sh" "${CORPUS}"

echo "==> stage slim artifact (bundle + frontend/dist + seed)"
mkdir -p "$DEST/frontend/dist"
cp "${CORPUS}/wrangler.jsonc" "$DEST/"
rsync -a "${CORPUS}/.cellp-bundle/" "$DEST/.cellp-bundle/"
rsync -a "${CORPUS}/frontend/dist/" "$DEST/frontend/dist/"
bash "${OVERLAY}/seed.sh" "${DEST}/seed.db"

echo "==> sync artifact → RustFS"
sync_artifact_to_rustfs "$PROJECT" "$VERSION"

echo "==> POST /versions"
create_version "$PROJECT" "$VERSION" "" "{\"artifact_uri\":\"s3://cellp-artifacts/${PROJECT}/${VERSION}/\"}"
poll_version "$PROJECT" "$VERSION" ready 300

PREVIEW="$(version_preview_url "$PROJECT" "$VERSION" 2>/dev/null || true)"
if [[ -z "$PREVIEW" ]]; then
  gw_port="${GATEWAY_URL##*:}"
  gw_port="${gw_port%%/*}"
  PREVIEW="${CELLP_PUBLIC_SCHEME_PREVIEW:-https}://$(preview_host "$PROJECT" "$VERSION"):${gw_port:-8788}/"
fi
PREVIEW="${PREVIEW%/}/"
gw_port="${GATEWAY_URL##*:}"
gw_port="${gw_port%%/*}"
PROD="${CELLP_PUBLIC_SCHEME_PREVIEW:-https}://$(prod_host "$PROJECT"):${gw_port:-8788}/"

curl -sf -X POST "${PLATFORM_URL}/v1/projects/${PROJECT}/versions/${VERSION}/promote" \
  -H "$(api_auth "$ADMIN_TOKEN")" -H "Content-Type: application/json" -d '{}' >/dev/null

HTTP=$(http_code_version "$PROJECT" "$VERSION" "/")
PROD_HTTP=$(http_code_prod "$PROJECT" "/")
echo ""
echo "OK ${SID} project=${PROJECT} version=${VERSION}"
echo "PREVIEW_URL=${PREVIEW}"
echo "PROD_URL=${PROD}"
echo "PREVIEW_HTTP=${HTTP}"
echo "PROD_HTTP=${PROD_HTTP}"
echo "ADMIN=${PREVIEW}admin"
echo "ADMIN_USER=admin ADMIN_PASS=cellp-dev-r2filebox"
echo "DASHBOARD=http://127.0.0.1:${DASHBOARD_PORT:-5190}/projects/${PROJECT}"
