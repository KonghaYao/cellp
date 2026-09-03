#!/usr/bin/env bash
# Cloudflare templates next-starter-template (@opennextjs/cloudflare): prebuild + wrangler bundle + slim assets.
set -euo pipefail
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
cd "$APP_DIR"

log() { echo "prepare-artifact: $*"; }

export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-false}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-false}"

if [[ ! -d node_modules ]]; then
  log "npm ci"
  npm ci
fi

if [[ ! -f .open-next/worker.js ]]; then
  log "next build"
  npm run build
  log "opennextjs-cloudflare build"
  npx opennextjs-cloudflare build
fi
[[ -f .open-next/worker.js ]] || { echo "missing .open-next/worker.js" >&2; exit 1; }

log "wrangler dry-run bundle"
rm -rf .cellp-bundle
npx --yes wrangler@4 deploy --config wrangler.jsonc --dry-run --outdir .cellp-bundle
if [[ -f .cellp-bundle/worker.js && ! -f .cellp-bundle/index.js ]]; then
  cp .cellp-bundle/worker.js .cellp-bundle/index.js
fi
[[ -f .cellp-bundle/index.js ]] || { echo "missing .cellp-bundle/index.js" >&2; exit 1; }

log "stage .open-next/assets → .cellp-assets"
rm -rf .cellp-assets
mkdir -p .cellp-assets
rsync -a .open-next/assets/ .cellp-assets/

node <<'NODE'
const fs = require('fs');
const p = 'wrangler.jsonc';
let raw = fs.readFileSync(p, 'utf8');
let j;
try {
  j = JSON.parse(raw);
} catch {
  raw = raw.replace(/^\s*\/\/.*$/gm, '').replace(/\/\*[\s\S]*?\*\//g, '').replace(/,\s*([}\]])/g, '$1');
  j = JSON.parse(raw);
}
j.name = 'support-opennext';
j.main = '.cellp-bundle/index.js';
j.no_bundle = true;
j.assets = j.assets || {};
j.assets.directory = '.cellp-assets';
j.assets.binding = j.assets.binding || 'ASSETS';
if (!j.compatibility_flags?.includes('nodejs_compat')) {
  j.compatibility_flags = [...(j.compatibility_flags || []), 'nodejs_compat'];
}
delete j.observability;
delete j.upload_source_maps;
delete j.$schema;
raw = JSON.stringify(j, null, 2) + '\n';
raw = raw.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync(p, raw);
NODE

log "ok: OpenNext bundled worker + .cellp-assets (no celld re-bundle of .open-next tree)"
