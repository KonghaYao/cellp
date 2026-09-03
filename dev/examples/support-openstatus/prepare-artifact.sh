#!/usr/bin/env bash
# OpenStatus astro-status-page (@astrojs/cloudflare): dist/_worker.js + static assets.
set -euo pipefail
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
OVERLAY="${ROOT}/dev/examples/support-openstatus/wrangler.cellp.jsonc"
ASTRO_TEMPLATE="https://github.com/openstatusHQ/astro-status-page.git"
BUILD_DIR="$APP_DIR"

log() { echo "prepare-artifact: $*"; }

if [[ ! -f "${APP_DIR}/astro.config.mjs" ]]; then
  BUILD_DIR="${APP_DIR}/.cellp-astro-status-page"
  if [[ ! -f "${BUILD_DIR}/astro.config.mjs" ]]; then
    log "openstatus monorepo has no wrangler; clone official astro-status-page Workers template"
    rm -rf "$BUILD_DIR"
    GIT_TERMINAL_PROMPT=0 git clone --depth 1 "$ASTRO_TEMPLATE" "$BUILD_DIR"
  fi
fi

cd "$BUILD_DIR"

export NPM_CONFIG_REGISTRY="${NPM_CONFIG_REGISTRY:-https://registry.npmmirror.com}"
export npm_config_registry="${npm_config_registry:-$NPM_CONFIG_REGISTRY}"
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-false}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-false}"

WORKER_ENTRY="dist/server/entry.mjs"
if [[ ! -f "$WORKER_ENTRY" ]]; then
  log "restore upstream wrangler.toml for astro build (deploy applies overlay too early)"
  rm -f wrangler.jsonc
  [[ -f wrangler.toml ]] || git checkout HEAD -- wrangler.toml 2>/dev/null || true
  log "pnpm install + astro build"
  corepack enable 2>/dev/null || true
  pnpm install --frozen-lockfile 2>/dev/null || pnpm install
  pnpm run build
fi

[[ -f "$WORKER_ENTRY" ]] || { echo "missing ${WORKER_ENTRY}" >&2; exit 1; }

log "stage static assets → .cellp-assets"
rm -rf .cellp-assets .cellp-bundle
mkdir -p .cellp-assets
rsync -a dist/client/ .cellp-assets/
rm -f .cellp-assets/.assetsignore

export OVERLAY_PATH="$OVERLAY"
node <<'NODE'
const fs = require('fs');
const overlay = JSON.parse(fs.readFileSync(process.env.OVERLAY_PATH, 'utf8'));
const j = { ...overlay };
j.main = 'dist/server/entry.mjs';
j.no_bundle = true;
delete j.images;
j.assets = j.assets || {};
j.assets.directory = '.cellp-assets';
j.assets.binding = j.assets.binding || 'ASSETS';
j.assets.html_handling = j.assets.html_handling || 'auto-trailing-slash';
if (!j.kv_namespaces?.some((k) => k.binding === 'SESSION')) {
  j.kv_namespaces = j.kv_namespaces || [];
  j.kv_namespaces.push({
    binding: 'SESSION',
    id: '00000000-0000-0000-0000-00000000000f',
  });
}
j.vars = {
  ...(j.vars || {}),
  API_KEY: j.vars?.API_KEY || 'cellp-openstatus-smoke',
  STATUS_PAGE_ID: j.vars?.STATUS_PAGE_ID || 'cellp',
};
let raw = JSON.stringify(j, null, 2) + '\n';
raw = raw.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync('wrangler.jsonc', raw);
NODE
rm -f wrangler.toml

if [[ "$BUILD_DIR" != "$APP_DIR" ]]; then
  log "publish astro artifact → ${APP_DIR}"
  cp wrangler.jsonc "$APP_DIR/"
  rm -rf "${APP_DIR}/dist" "${APP_DIR}/.cellp-assets"
  rsync -a dist/ "$APP_DIR/dist/"
  rsync -a .cellp-assets/ "$APP_DIR/.cellp-assets/"
fi

log "ok: dist/server/entry.mjs + .cellp-assets"
