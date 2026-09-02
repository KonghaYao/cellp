#!/usr/bin/env bash
# Nuxt Nitro cloudflare_module (C3 workers template): nuxi scaffold, bundle worker, slim assets.
set -euo pipefail
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
SCAFFOLD="${APP_DIR}/.cellp-nuxt-app"
OVERLAY="${ROOT}/dev/examples/support-nuxt/wrangler.cellp.jsonc"
[[ -f "$OVERLAY" ]] || { echo "missing overlay ${OVERLAY}" >&2; exit 1; }

log() { echo "prepare-artifact: $*"; }

mkdir -p "$APP_DIR"

if [[ ! -f "${SCAFFOLD}/package.json" ]]; then
  log "scaffold minimal Nuxt (nitro cloudflare_module)"
  rm -rf "$SCAFFOLD"
  (
    cd "$APP_DIR"
    CI=1 npx --yes nuxi@3.25.0 init .cellp-nuxt-app \
      --packageManager npm \
      --no-install \
      --no-gitInit \
      -f || true
  )
  [[ -f "${SCAFFOLD}/package.json" ]] || {
    echo "nuxi init did not produce ${SCAFFOLD}/package.json" >&2
    exit 1
  }
fi

cd "$SCAFFOLD"

node <<'NODE'
const fs = require('fs');
const p = 'package.json';
const j = JSON.parse(fs.readFileSync(p, 'utf8'));
j.dependencies = j.dependencies || {};
j.dependencies.nuxt = '3.15.4';
j.dependencies.vue = '^3.5.13';
j.dependencies['vue-router'] = '^4.5.0';
fs.writeFileSync(p, JSON.stringify(j, null, 2) + '\n');
NODE

cat > nuxt.config.ts <<'EOF'
export default defineNuxtConfig({
  compatibilityDate: '2024-11-01',
  devtools: { enabled: false },
  nitro: {
    preset: 'cloudflare_module',
    cloudflare: {
      deployConfig: true,
      nodeCompat: true,
    },
  },
})
EOF

cp "$OVERLAY" wrangler.jsonc
node <<'NODE'
const fs = require('fs');
const p = 'wrangler.jsonc';
const j = JSON.parse(fs.readFileSync(p, 'utf8'));
j.main = './.output/server/index.mjs';
j.assets = j.assets || {};
j.assets.directory = './.output/public/';
j.assets.binding = j.assets.binding || 'ASSETS';
fs.writeFileSync(p, JSON.stringify(j, null, 2) + '\n');
NODE

export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-false}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-false}"

log "install + build"
npm install
npm rebuild esbuild 2>/dev/null || true
npm run build

[[ -f .output/server/index.mjs ]] || {
  echo "missing .output/server/index.mjs" >&2
  exit 1
}

log "wrangler dry-run bundle"
rm -rf .cellp-bundle
npx --yes wrangler@4 deploy --config wrangler.jsonc --dry-run --outdir .cellp-bundle
if [[ -f .cellp-bundle/_worker.js && ! -f .cellp-bundle/index.js ]]; then
  cp .cellp-bundle/_worker.js .cellp-bundle/index.js
fi
[[ -f .cellp-bundle/index.js ]] || { echo "missing .cellp-bundle/index.js" >&2; exit 1; }

log "stage static assets from .output/public"
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

log "publish slim artifact paths → ${APP_DIR}"
cp wrangler.jsonc "$APP_DIR/"
rm -rf "${APP_DIR}/.cellp-bundle" "${APP_DIR}/.cellp-assets"
rsync -a .cellp-bundle/ "${APP_DIR}/.cellp-bundle/"
rsync -a .cellp-assets/ "${APP_DIR}/.cellp-assets/"

log "ok: bundled worker + .cellp-assets (C3 nuxt / nitro cloudflare_module)"
