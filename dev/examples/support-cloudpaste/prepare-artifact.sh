#!/usr/bin/env bash
# CloudPaste unified SPA (backend/wrangler.spa.toml): API Worker + frontend/dist as ASSETS.
set -euo pipefail
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
cd "$APP_DIR"
REPO_ROOT="$(cd .. && pwd)"
FRONTEND_DIST="${REPO_ROOT}/frontend/dist"

log() { echo "prepare-artifact: $*"; }

export NPM_CONFIG_REGISTRY="${NPM_CONFIG_REGISTRY:-https://registry.npmmirror.com}"
export npm_config_registry="${npm_config_registry:-$NPM_CONFIG_REGISTRY}"
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-false}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-false}"

if [[ ! -f "${REPO_ROOT}/frontend/package.json" ]]; then
  echo "prepare-artifact: missing frontend/ (expected ${REPO_ROOT})" >&2
  exit 1
fi

if [[ ! -f "${FRONTEND_DIST}/index.html" ]]; then
  log "build frontend (vite)"
  (
    cd "${REPO_ROOT}/frontend"
    npm install
    npm run build
  )
fi
[[ -f "${FRONTEND_DIST}/index.html" ]] || {
  echo "prepare-artifact: missing ${FRONTEND_DIST}/index.html" >&2
  exit 1
}

if [[ ! -f wrangler.jsonc ]]; then
  echo "prepare-artifact: missing wrangler.jsonc (overlay should be applied)" >&2
  exit 1
fi

log "backend npm install"
npm install

mkdir -p .cellp-assets

log "wrangler dry-run bundle"
rm -rf .cellp-bundle
npx --yes wrangler@4 deploy --config wrangler.jsonc --dry-run --outdir .cellp-bundle
if [[ -f .cellp-bundle/_worker.js && ! -f .cellp-bundle/index.js ]]; then
  cp .cellp-bundle/_worker.js .cellp-bundle/index.js
elif [[ -f .cellp-bundle/unified-entry.js && ! -f .cellp-bundle/index.js ]]; then
  cp .cellp-bundle/unified-entry.js .cellp-bundle/index.js
fi
[[ -f .cellp-bundle/index.js ]] || {
  echo "prepare-artifact: missing .cellp-bundle/index.js (have: $(ls .cellp-bundle 2>/dev/null | tr '\n' ' '))" >&2
  exit 1
}

log "stage frontend dist → .cellp-assets"
rm -rf .cellp-assets
mkdir -p .cellp-assets
rsync -a "${FRONTEND_DIST}/" .cellp-assets/
rm -f .cellp-assets/.assetsignore

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
j.name = j.name || 'support-cloudpaste';
j.assets = j.assets || {};
j.assets.directory = '.cellp-assets';
j.assets.binding = j.assets.binding || 'ASSETS';
j.assets.not_found_handling = j.assets.not_found_handling || 'single-page-application';
j.assets.run_worker_first = j.assets.run_worker_first || ['/api/*', '/dav/*'];
delete j.triggers;
let out = JSON.stringify(j, null, 2) + '\n';
out = out.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync(p, out);
NODE

log "bundled $(wc -c < .cellp-bundle/index.js | tr -d ' ') bytes → wrangler no_bundle + SPA assets"
