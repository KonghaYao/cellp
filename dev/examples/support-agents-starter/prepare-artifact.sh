#!/usr/bin/env bash
# agents-starter: vite builds client → .cellp-assets; celld bundles src/server.ts (needs node_modules in artifact).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=dev/scripts/support-pnpm.sh
source "${ROOT}/dev/scripts/support-pnpm.sh"
cellp_ensure_pnpm
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"

APP_DIR="${1:?app dir}"
cd "$APP_DIR"

log() { echo "prepare-artifact: $*"; }

export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-true}"

if [[ ! -f dist/client/index.html ]]; then
  log "vite build"
  pnpm exec vite build
fi
[[ -f dist/client/index.html ]] || { echo "missing dist/client (vite build)" >&2; exit 1; }

log "stage client assets → .cellp-assets"
rm -rf .cellp-assets .cellp-bundle
mkdir -p .cellp-assets
rsync -a --exclude '.assetsignore' dist/client/ .cellp-assets/

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
j.main = j.main || 'src/server.ts';
delete j.no_bundle;
j.assets = j.assets || {};
j.assets.directory = '.cellp-assets';
j.assets.not_found_handling = j.assets.not_found_handling || 'single-page-application';
j.assets.run_worker_first = ['/agents/', '/oauth/'];
delete j.ai;
raw = JSON.stringify(j, null, 2) + '\n';
fs.writeFileSync(p, raw);
NODE

log "ok: src/server.ts + .cellp-assets (celld bundle at deploy; node_modules rsynced)"
