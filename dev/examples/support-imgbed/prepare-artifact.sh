#!/usr/bin/env bash
# CloudFlare-ImgBed Workers path (deploy/worker): generate routes + wrangler dry-run slim bundle.
set -euo pipefail
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
cd "$APP_DIR"
REPO_ROOT="$(cd ../.. && pwd)"

log() { echo "prepare-artifact: $*"; }

if [[ ! -d "${REPO_ROOT}/functions" ]]; then
  echo "prepare-artifact: missing repo functions/ (expected ${REPO_ROOT})" >&2
  exit 1
fi

if [[ ! -f wrangler.jsonc ]]; then
  echo "prepare-artifact: missing wrangler.jsonc" >&2
  exit 1
fi

log "generate worker routes"
node "${REPO_ROOT}/deploy/worker/generate-routes.js"

[[ -d "${REPO_ROOT}/frontend-dist" ]] || {
  echo "prepare-artifact: missing frontend-dist" >&2
  exit 1
}

log "wrangler dry-run bundle"
rm -rf .cellp-bundle
npx --yes wrangler@4 deploy --config wrangler.jsonc --dry-run --outdir .cellp-bundle
if [[ -f .cellp-bundle/_worker.js && ! -f .cellp-bundle/index.js ]]; then
  cp .cellp-bundle/_worker.js .cellp-bundle/index.js
fi
[[ -f .cellp-bundle/index.js ]] || { echo "prepare-artifact: missing .cellp-bundle/index.js" >&2; exit 1; }

log "stage static assets"
rm -rf .cellp-assets
mkdir -p .cellp-assets
rsync -a "${REPO_ROOT}/frontend-dist/" .cellp-assets/

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
j.assets = j.assets || {};
j.assets.directory = '.cellp-assets';
j.assets.binding = j.assets.binding || 'ASSETS';
j.assets.not_found_handling = j.assets.not_found_handling || 'single-page-application';
delete j.images;
raw = JSON.stringify(j, null, 2) + '\n';
raw = raw.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync(p, raw);
NODE

log "bundled $(wc -c < .cellp-bundle/index.js | tr -d ' ') bytes → wrangler no_bundle"
