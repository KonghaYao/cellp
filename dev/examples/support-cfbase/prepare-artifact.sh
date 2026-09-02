#!/usr/bin/env bash
# SvelteKit adapter-cloudflare: pre-bundle worker + stage client assets (no node_modules rsync).
set -euo pipefail
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
cd "$APP_DIR"

export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-true}"

if [[ ! -f wrangler.jsonc ]]; then
  echo "prepare-artifact: missing wrangler.jsonc" >&2
  exit 1
fi
if [[ ! -f .svelte-kit/cloudflare/_worker.js ]]; then
  echo "prepare-artifact: run npm run build first (.svelte-kit/cloudflare/_worker.js missing)" >&2
  exit 1
fi

log() { echo "prepare-artifact: $*"; }

log "wrangler dry-run bundle"
rm -rf .cellp-bundle
npx --yes wrangler@4 deploy --config wrangler.jsonc --dry-run --outdir .cellp-bundle
if [[ -f .cellp-bundle/_worker.js && ! -f .cellp-bundle/index.js ]]; then
  cp .cellp-bundle/_worker.js .cellp-bundle/index.js
fi
test -f .cellp-bundle/index.js

log "stage static assets from .svelte-kit/cloudflare"
rm -rf .cellp-assets
mkdir -p .cellp-assets
rsync -a \
  --exclude '_worker.js' \
  --exclude '.assetsignore' \
  .svelte-kit/cloudflare/ .cellp-assets/

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
  j.assets.binding = j.assets.binding || 'ASSETS';
}
raw = JSON.stringify(j, null, 2) + '\n';
raw = raw.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync(p, raw);
NODE

log "bundled $(wc -c < .cellp-bundle/index.js | tr -d ' ') bytes → wrangler no_bundle + .cellp-assets"
