#!/usr/bin/env bash
# FileWorker is Cloudflare Pages (functions/ + dist). Bundle for celld via wrangler pages functions build.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=dev/scripts/support-pnpm.sh
source "${ROOT}/dev/scripts/support-pnpm.sh"
cellp_ensure_pnpm
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
APP_DIR="${1:?app dir}"
cd "$APP_DIR"

# shellcheck disable=SC1091
source "${ROOT}/dev/.env" 2>/dev/null || true

export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-true}"

if [[ ! -f wrangler.jsonc ]]; then
  echo "prepare-artifact: missing wrangler.jsonc" >&2
  exit 1
fi

log() { echo "prepare-artifact: $*"; }

log "vue build"
pnpm run build-only

log "pages functions → worker bundle"
rm -rf .cellp-bundle
pnpm exec --yes wrangler@4 pages functions build functions \
  --outdir .cellp-bundle \
  --build-output-directory dist \
  --compatibility-date 2024-03-14 \
  --compatibility-flag nodejs_compat
test -f .cellp-bundle/index.js

log "stage static assets"
rm -rf .cellp-assets
mkdir -p .cellp-assets
cp -R dist/. .cellp-assets/

BUCKET="${SUPPORT_FILEWORKER_BUCKET:-support-fileworker-store}"
ENDPOINT="${S3_ENDPOINT:-http://127.0.0.1:19000}"
AK="${RUSTFS_ACCESS_KEY:-rustfsadmin}"
SK="${RUSTFS_SECRET_KEY:-rustfsadmin}"
if command -v aws >/dev/null 2>&1; then
  AWS_ACCESS_KEY_ID="$AK" AWS_SECRET_ACCESS_KEY="$SK" \
    aws --endpoint-url "$ENDPOINT" s3 mb "s3://${BUCKET}" 2>/dev/null || true
fi

export CELLP_PATCH_BUCKET="$BUCKET" CELLP_PATCH_ENDPOINT="$ENDPOINT" CELLP_PATCH_AK="$AK" CELLP_PATCH_SK="$SK"
node <<'NODE'
const fs = require('fs');
const p = 'wrangler.jsonc';
let raw = fs.readFileSync(p, 'utf8');
let j;
try {
  j = JSON.parse(raw);
} catch {
  raw = raw.replace(/^\s*\/\/.*$/gm, '').replace(/,\s*([}\]])/g, '$1');
  j = JSON.parse(raw);
}
j.main = '.cellp-bundle/index.js';
j.no_bundle = true;
if (j.assets && typeof j.assets === 'object') {
  j.assets.directory = '.cellp-assets';
}
if (!j.vars) j.vars = {};
j.vars.REGION = 'auto';
j.vars.BUCKET = process.env.CELLP_PATCH_BUCKET || 'support-fileworker-store';
j.vars.ENDPOINT = process.env.CELLP_PATCH_ENDPOINT || 'http://127.0.0.1:19000';
j.vars.ACCESS_KEY_ID = process.env.CELLP_PATCH_AK || 'rustfsadmin';
j.vars.SECRET_ACCESS_KEY = process.env.CELLP_PATCH_SK || 'rustfsadmin';
j.vars.PASSWORD = j.vars.PASSWORD || 'cellp-dev-fileworker';
raw = JSON.stringify(j, null, 2) + '\n';
raw = raw.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync(p, raw);
NODE

log "bundled $(wc -c < .cellp-bundle/index.js | tr -d ' ') bytes → wrangler no_bundle"
