#!/usr/bin/env bash
# SvelteKit adapter-cloudflare (C3 workers template): scaffold via sv CLI, bundle worker, slim assets.
set -euo pipefail
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
SCAFFOLD="${APP_DIR}/.cellp-svelte-app"
OVERLAY="${ROOT}/dev/examples/support-sveltekit/wrangler.cellp.jsonc"
[[ -f "$OVERLAY" ]] || { echo "missing overlay ${OVERLAY}" >&2; exit 1; }

log() { echo "prepare-artifact: $*"; }

mkdir -p "$APP_DIR"

if [[ ! -f "${SCAFFOLD}/package.json" ]]; then
  log "scaffold minimal SvelteKit + adapter-cloudflare (workers)"
  rm -rf "$SCAFFOLD"
  npx --yes sv@0.17.0 create "$SCAFFOLD" \
    --template minimal \
    --types ts \
    --add sveltekit-adapter=adapter:cloudflare+cfTarget:workers \
    --no-download-check \
    --install npm \
    --no-dir-check
fi

cd "$SCAFFOLD"

export OVERLAY_PATH="$OVERLAY"

export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-false}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-false}"

# adapter-cloudflare writes the worker to wrangler `main` (not always _worker.js).
# Reset build-time wrangler so output lands under .svelte-kit/cloudflare/.
log "wrangler build config (adapter output → .svelte-kit/cloudflare/_worker.js)"
node <<'NODE'
const fs = require('fs');
const overlay = JSON.parse(
  fs.readFileSync(process.env.OVERLAY_PATH, 'utf8')
);
const p = 'wrangler.jsonc';
const j = {
  name: overlay.name || 'support-sveltekit',
  compatibility_date: overlay.compatibility_date || '2026-01-01',
  compatibility_flags: overlay.compatibility_flags || ['nodejs_compat', 'nodejs_als'],
  main: '.svelte-kit/cloudflare/_worker.js',
  assets: {
    binding: overlay.assets?.binding || 'ASSETS',
    directory: '.svelte-kit/cloudflare',
  },
};
fs.writeFileSync(p, JSON.stringify(j, null, 2) + '\n');
NODE

log "gen + build"
npm run gen
npm run build

WORKER_ENTRY=".svelte-kit/cloudflare/_worker.js"
if [[ ! -f "$WORKER_ENTRY" ]]; then
  WORKER_ENTRY="$(node <<'NODE'
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
const main = j.main || '.svelte-kit/cloudflare/_worker.js';
process.stdout.write(main);
NODE
)"
fi
[[ -f "$WORKER_ENTRY" ]] || {
  echo "missing SvelteKit worker entry (expected .svelte-kit/cloudflare/_worker.js or wrangler main)" >&2
  exit 1
}
log "worker entry: ${WORKER_ENTRY}"

export WORKER_ENTRY
node <<'NODE'
const fs = require('fs');
const overlay = JSON.parse(
  fs.readFileSync(process.env.OVERLAY_PATH, 'utf8')
);
const worker = process.env.WORKER_ENTRY;
const p = 'wrangler.jsonc';
let raw = fs.readFileSync(p, 'utf8');
let j;
try {
  j = JSON.parse(raw);
} catch {
  raw = raw.replace(/^\s*\/\/.*$/gm, '').replace(/,\s*([}\]])/g, '$1');
  j = JSON.parse(raw);
}
j.name = overlay.name || j.name;
j.compatibility_date = overlay.compatibility_date || j.compatibility_date;
j.compatibility_flags = overlay.compatibility_flags || j.compatibility_flags;
j.main = worker;
delete j.no_bundle;
j.assets = j.assets || {};
j.assets.binding = overlay.assets?.binding || j.assets.binding || 'ASSETS';
j.assets.directory = '.svelte-kit/cloudflare';
raw = JSON.stringify(j, null, 2) + '\n';
fs.writeFileSync(p, raw);
NODE

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

log "publish slim artifact paths → ${APP_DIR}"
cp wrangler.jsonc "$APP_DIR/"
rm -rf "${APP_DIR}/.cellp-bundle" "${APP_DIR}/.cellp-assets"
rsync -a .cellp-bundle/ "${APP_DIR}/.cellp-bundle/"
rsync -a .cellp-assets/ "${APP_DIR}/.cellp-assets/"

log "ok: bundled worker + .cellp-assets (C3 svelte / adapter-cloudflare workers)"
