#!/usr/bin/env bash
# Hono cloudflare-workers (create-hono): slim wrangler bundle + public assets.
set -euo pipefail
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
SCAFFOLD="${APP_DIR}/cellp-hono-app"
C3_TEMPLATE="${APP_DIR}/../../hono/workers/templates"
OVERLAY="${ROOT}/dev/examples/support-hono/wrangler.cellp.jsonc"
[[ -f "$OVERLAY" ]] || { echo "missing overlay ${OVERLAY}" >&2; exit 1; }

log() { echo "prepare-artifact: $*"; }

mkdir -p "$APP_DIR"

if [[ ! -f "${SCAFFOLD}/package.json" ]]; then
  log "scaffold create-hono cloudflare-workers"
  rm -rf "$SCAFFOLD"
  mkdir -p "$SCAFFOLD"
  (
    cd "$SCAFFOLD"
    npx --yes create-hono@0.19.4 . \
      --template cloudflare-workers \
      --install \
      --pm npm
  )
fi

cd "$SCAFFOLD"
[[ -f src/index.ts ]] || { echo "missing src/index.ts after create-hono" >&2; exit 1; }

if [[ -d "$C3_TEMPLATE" ]]; then
  log "overlay C3 hono workers template ( /message + public )"
  rsync -a "${C3_TEMPLATE}/src/" ./src/
  if [[ -d "${C3_TEMPLATE}/public" ]]; then
    mkdir -p public
    rsync -a "${C3_TEMPLATE}/public/" ./public/
  fi
fi

export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-false}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-false}"

export OVERLAY_PATH="$OVERLAY"
log "wrangler build config (src/index.ts + nodejs_compat)"
node <<'NODE'
const fs = require('fs');
const overlay = JSON.parse(fs.readFileSync(process.env.OVERLAY_PATH, 'utf8'));
const p = 'wrangler.jsonc';
let raw = fs.readFileSync(p, 'utf8');
let j;
try {
  j = JSON.parse(raw);
} catch {
  raw = raw.replace(/^\s*\/\/.*$/gm, '').replace(/,\s*([}\]])/g, '$1');
  j = JSON.parse(raw);
}
j.name = overlay.name || j.name || 'support-hono';
j.compatibility_date = overlay.compatibility_date || j.compatibility_date || '2026-09-03';
j.compatibility_flags = overlay.compatibility_flags || j.compatibility_flags || ['nodejs_compat'];
j.main = 'src/index.ts';
delete j.no_bundle;
j.assets = j.assets || {};
j.assets.binding = overlay.assets?.binding || j.assets.binding || 'ASSETS';
j.assets.directory = j.assets.directory || './public';
fs.writeFileSync(p, JSON.stringify(j, null, 2) + '\n');
NODE

log "wrangler dry-run bundle"
node <<'NODE'
const fs = require('fs');
const p = 'wrangler.jsonc';
let raw = fs.readFileSync(p, 'utf8');
let j;
try { j = JSON.parse(raw); } catch {
  raw = raw.replace(/^\s*\/\/.*$/gm, '').replace(/,\s*([}\]])/g, '$1');
  j = JSON.parse(raw);
}
j.main = 'src/index.ts';
delete j.no_bundle;
fs.writeFileSync(p, JSON.stringify(j, null, 2) + '\n');
NODE
rm -rf .cellp-bundle
npx --yes wrangler@4 deploy --config wrangler.jsonc --dry-run --outdir .cellp-bundle
if [[ -f .cellp-bundle/_worker.js && ! -f .cellp-bundle/index.js ]]; then
  cp .cellp-bundle/_worker.js .cellp-bundle/index.js
fi
[[ -f .cellp-bundle/index.js ]] || { echo "missing .cellp-bundle/index.js" >&2; exit 1; }

log "stage public → .cellp-assets"
rm -rf .cellp-assets
mkdir -p .cellp-assets
if [[ -d public ]]; then
  rsync -a public/ .cellp-assets/
fi

node <<'NODE'
const fs = require('fs');
const overlay = JSON.parse(fs.readFileSync(process.env.OVERLAY_PATH, 'utf8'));
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
j.main = '.cellp-bundle/index.js';
j.no_bundle = true;
j.assets = j.assets || {};
j.assets.binding = overlay.assets?.binding || 'ASSETS';
j.assets.directory = '.cellp-assets';
raw = JSON.stringify(j, null, 2) + '\n';
raw = raw.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync(p, raw);
NODE

log "publish slim artifact paths → ${APP_DIR}"
cp wrangler.jsonc "$APP_DIR/"
rm -rf "${APP_DIR}/.cellp-bundle" "${APP_DIR}/.cellp-assets"
rsync -a .cellp-bundle/ "${APP_DIR}/.cellp-bundle/"
rsync -a .cellp-assets/ "${APP_DIR}/.cellp-assets/"

log "ok: hono worker bundle + .cellp-assets"
