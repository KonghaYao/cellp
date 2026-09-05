#!/usr/bin/env bash
# Remix @remix-run/cloudflare (templates/remix-starter-template): wrangler bundle + slim assets.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=dev/scripts/support-pnpm.sh
source "${ROOT}/dev/scripts/support-pnpm.sh"
cellp_ensure_pnpm
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
cd "$APP_DIR"

log() { echo "prepare-artifact: $*"; }

export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-true}"

if [[ ! -f build/server/index.js ]]; then
  log "remix build"
  pnpm run build
fi
[[ -f build/server/index.js ]] || { echo "missing build/server/index.js" >&2; exit 1; }

log "stage static client → .cellp-assets (no .assetsignore)"
rm -rf .cellp-assets .cellp-bundle
mkdir -p .cellp-assets
rsync -a \
  --exclude '.assetsignore' \
  build/client/ .cellp-assets/

log "wrangler dry-run bundle"
pnpm exec --yes wrangler@4 deploy --config wrangler.jsonc --dry-run --outdir .cellp-bundle
if [[ -f .cellp-bundle/_worker.js && ! -f .cellp-bundle/index.js ]]; then
  cp .cellp-bundle/_worker.js .cellp-bundle/index.js
fi
if [[ -f .cellp-bundle/server.js && ! -f .cellp-bundle/index.js ]]; then
  cp .cellp-bundle/server.js .cellp-bundle/index.js
fi
[[ -f .cellp-bundle/index.js ]] || { echo "missing .cellp-bundle/index.js" >&2; exit 1; }

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
if (!j.compatibility_flags?.includes('nodejs_compat')) {
  j.compatibility_flags = [...(j.compatibility_flags || []), 'nodejs_compat'];
}
raw = JSON.stringify(j, null, 2) + '\n';
raw = raw.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync(p, raw);
NODE

log "ok: bundled worker + .cellp-assets (Remix cloudflare template; no celld .assetsignore)"
