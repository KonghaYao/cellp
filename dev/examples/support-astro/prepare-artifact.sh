#!/usr/bin/env bash
# Astro @astrojs/cloudflare: native dist/_worker.js + static assets; celld bundles.
set -euo pipefail
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
cd "$APP_DIR"

log() { echo "prepare-artifact: $*"; }

export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-true}"

if [[ ! -f dist/_worker.js/index.js ]]; then
  log "astro build"
  npm run build
fi
[[ -f dist/_worker.js/index.js ]] || { echo "missing dist/_worker.js/index.js" >&2; exit 1; }

log "stage static assets → .cellp-assets"
rm -rf .cellp-assets .cellp-bundle
mkdir -p .cellp-assets
rsync -a \
  --exclude '_worker.js' \
  --exclude '.assetsignore' \
  dist/ .cellp-assets/

node <<'NODE'
const fs = require('fs');
const p = 'wrangler.jsonc';
let raw = fs.readFileSync(p, 'utf8');
raw = raw.replace(/\/\/[^\n]*/g, '').replace(/,\s*([}\]])/g, '$1');
const j = JSON.parse(raw);
j.main = 'dist/_worker.js/index.js';
delete j.no_bundle;
j.assets = j.assets || {};
j.assets.directory = '.cellp-assets';
j.assets.binding = j.assets.binding || 'ASSETS';
j.assets.html_handling = j.assets.html_handling || 'auto-trailing-slash';
if (!j.kv_namespaces?.some((k) => k.binding === 'SESSION')) {
  j.kv_namespaces = j.kv_namespaces || [];
  j.kv_namespaces.push({
    binding: 'SESSION',
    id: '00000000-0000-0000-0000-000000000002',
  });
}
raw = JSON.stringify(j, null, 2) + '\n';
raw = raw.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync(p, raw);
NODE

log "ok: dist/_worker.js + .cellp-assets (no app-side cloudflare:workers/caches patches)"
