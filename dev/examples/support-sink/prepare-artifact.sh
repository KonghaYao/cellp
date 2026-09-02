#!/usr/bin/env bash
# Sink (Nuxt / Nitro cloudflare-module): pnpm build + wrangler dry-run slim bundle for celld.
set -euo pipefail
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
cd "$APP_DIR"

log() { echo "prepare-artifact: $*"; }

if [[ ! -f wrangler.jsonc ]]; then
  echo "prepare-artifact: missing wrangler.jsonc" >&2
  exit 1
fi

export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-false}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-false}"

if ! command -v pnpm >/dev/null 2>&1; then
  corepack enable 2>/dev/null || true
  corepack prepare pnpm@11.11.0 --activate 2>/dev/null || true
fi

if [[ ! -f .env ]]; then
  log "write minimal .env for build"
  cat > .env <<'EOF'
NUXT_SITE_TOKEN=cellp-dev-sink-site-token-min-32-chars
NUXT_PUBLIC_PREVIEW_MODE=true
NUXT_API_CORS=true
NUXT_DISABLE_AUTO_BACKUP=true
EOF
fi

log "pnpm install"
pnpm install --frozen-lockfile 2>/dev/null || pnpm install

log "pnpm build"
NODE_OPTIONS="${NODE_OPTIONS:---max-old-space-size=8192}" pnpm run build

[[ -f .output/server/index.mjs ]] || {
  echo "prepare-artifact: missing .output/server/index.mjs" >&2
  exit 1
}
[[ -d .output/public ]] || {
  echo "prepare-artifact: missing .output/public" >&2
  exit 1
}

node <<'NODE'
const fs = require('fs');
const p = 'wrangler.jsonc';
let raw = fs.readFileSync(p, 'utf8');
const j = JSON.parse(raw.replace(/\/\/[^\n]*/g, '').replace(/,\s*([}\]])/g, '$1'));
j.main = './.output/server/index.mjs';
if (j.assets && typeof j.assets === 'object') {
  j.assets.directory = './.output/public/';
  j.assets.binding = j.assets.binding || 'ASSETS';
}
fs.writeFileSync(p, JSON.stringify(j, null, 2) + '\n');
NODE

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
rsync -a .output/public/ .cellp-assets/

node <<'NODE'
const fs = require('fs');
const p = 'wrangler.jsonc';
let j = JSON.parse(fs.readFileSync(p, 'utf8'));
j.main = '.cellp-bundle/index.js';
j.no_bundle = true;
j.assets = j.assets || {};
j.assets.directory = '.cellp-assets';
j.assets.binding = j.assets.binding || 'ASSETS';
let raw = JSON.stringify(j, null, 2) + '\n';
raw = raw.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync(p, raw);
NODE

log "bundled $(wc -c < .cellp-bundle/index.js | tr -d ' ') bytes → wrangler no_bundle"
