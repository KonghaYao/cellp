#!/usr/bin/env bash
# UptimeFlare status page: @cloudflare/next-on-pages → dist/_worker.js + static assets (single Worker slim path).
# Monitoring cron worker + DO remain upstream-only; cellp validates the Pages/Next bundle only.
set -euo pipefail
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
cd "$APP_DIR"

log() { echo "prepare-artifact: $*"; }

export NPM_CONFIG_REGISTRY="${NPM_CONFIG_REGISTRY:-https://registry.npmmirror.com}"
export npm_config_registry="${npm_config_registry:-$NPM_CONFIG_REGISTRY}"
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-false}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-false}"

if [[ ! -f .vercel/output/static/_worker.js/index.js ]]; then
  log "npm install"
  npm install
  log "@cloudflare/next-on-pages build"
  npx @cloudflare/next-on-pages
fi

[[ -f .vercel/output/static/_worker.js/index.js ]] || {
  echo "prepare-artifact: missing .vercel/output/static/_worker.js/index.js" >&2
  exit 1
}

log "stage worker → dist/_worker.js"
rm -rf dist/_worker.js
mkdir -p dist/_worker.js
rsync -a .vercel/output/static/_worker.js/ dist/_worker.js/

log "stage static assets → .cellp-assets"
rm -rf .cellp-assets
mkdir -p .cellp-assets
rsync -a \
  --exclude '_worker.js' \
  --exclude '.assetsignore' \
  .vercel/output/static/ .cellp-assets/

mkdir -p migrations
if [[ -f init.sql && ! -f migrations/0001_init.sql ]]; then
  cp init.sql migrations/0001_init.sql
fi

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
if (!j.d1_databases?.length) {
  j.d1_databases = [
    {
      binding: 'UPTIMEFLARE_D1',
      database_name: 'uptimeflare_d1',
      database_id: '00000000-0000-0000-0000-00000000000c',
    },
  ];
}
raw = JSON.stringify(j, null, 2) + '\n';
raw = raw.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync(p, raw);
NODE

log "ok: next-on-pages dist/_worker.js + .cellp-assets ($(wc -c < dist/_worker.js/index.js | tr -d ' ') bytes worker)"
