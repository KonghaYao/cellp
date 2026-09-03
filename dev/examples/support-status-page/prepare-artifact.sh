#!/usr/bin/env bash
# cf-workers-status-page: flareact client + webpack worker → wrangler slim bundle for celld.
set -euo pipefail
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
cd "$APP_DIR"

log() { echo "prepare-artifact: $*"; }

if [[ ! -f wrangler.jsonc ]]; then
  echo "prepare-artifact: missing wrangler.jsonc (cellp overlay should be applied first)" >&2
  exit 1
fi

export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-false}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-false}"

log "npm install (--legacy-peer-deps for flareact/react peer)"
npm install --legacy-peer-deps

# celld has no cf-ray; cron getCheckLocation() would throw on GET-driven checks later.
if [[ -f src/functions/helpers.js ]]; then
  node <<'NODE'
const fs = require('fs');
const p = 'src/functions/helpers.js';
let s = fs.readFileSync(p, 'utf8');
const needle = `export async function getCheckLocation() {
  const res = await fetch('https://cloudflare-dns.com/dns-query', {
    method: 'OPTIONS',
  })
  return res.headers.get('cf-ray').split('-')[1]
}`;
const repl = `export async function getCheckLocation() {
  try {
    const res = await fetch('https://cloudflare-dns.com/dns-query', {
      method: 'OPTIONS',
    })
    const ray = res.headers.get('cf-ray')
    if (ray && ray.includes('-')) return ray.split('-')[1]
  } catch (e) {}
  return 'LAB'
}`;
if (s.includes(needle)) {
  fs.writeFileSync(p, s.replace(needle, repl));
  console.log('prepare-artifact: patched getCheckLocation for celld (no cf-ray)');
}
NODE
fi

export NODE_OPTIONS="${NODE_OPTIONS:---openssl-legacy-provider --max-old-space-size=8192}"

log "postcss + flareact client build"
npm run css
npx flareact build

[[ -f out/_flareact/static/build-manifest.json ]] || {
  echo "prepare-artifact: missing flareact client build-manifest" >&2
  exit 1
}

log "flareact worker webpack (dist/main.js)"
IS_WORKER=true NODE_ENV=production npx webpack \
  --config node_modules/flareact/configs/webpack.worker.config.js \
  --mode production

[[ -f dist/main.js ]] || {
  echo "prepare-artifact: missing dist/main.js" >&2
  exit 1
}

log "wrangler dry-run bundle"
rm -rf .cellp-bundle
npx --yes wrangler@4 deploy --config wrangler.jsonc --dry-run --outdir .cellp-bundle
if [[ -f .cellp-bundle/index.js && ! -f .cellp-bundle/main.js ]]; then
  cp .cellp-bundle/index.js .cellp-bundle/main.js
fi
if [[ -f .cellp-bundle/main.js && ! -f .cellp-bundle/index.js ]]; then
  cp .cellp-bundle/main.js .cellp-bundle/index.js
fi
[[ -f .cellp-bundle/main.js ]] || {
  echo "prepare-artifact: missing .cellp-bundle/main.js" >&2
  exit 1
}

# celld loads ESM default { fetch }. Flareact webpack + wrangler leave a
# Service Worker IIFE with no default (celld: "default not object").
log "wrap Service Worker IIFE → ESM default { fetch, scheduled }"
ENTRY_SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/cellp-entry.mjs"
cp "$ENTRY_SRC" .cellp-bundle/index.js
if ! grep -q "export default" .cellp-bundle/index.js; then
  echo "prepare-artifact: ESM wrapper missing export default" >&2
  exit 1
fi

log "stage static assets (out/ → .cellp-assets)"
rm -rf .cellp-assets
mkdir -p .cellp-assets
rsync -a out/ .cellp-assets/

node <<'NODE'
const fs = require('fs');
const p = 'wrangler.jsonc';
let j = JSON.parse(fs.readFileSync(p, 'utf8'));
j.main = '.cellp-bundle/index.js';
j.no_bundle = true;
j.assets = j.assets || {};
j.assets.directory = '.cellp-assets';
j.assets.binding = j.assets.binding || 'ASSETS';
j.assets.run_worker_first = true;
let raw = JSON.stringify(j, null, 2) + '\n';
raw = raw.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync(p, raw);
NODE

log "bundled $(wc -c < .cellp-bundle/index.js | tr -d ' ') bytes → wrangler no_bundle"
