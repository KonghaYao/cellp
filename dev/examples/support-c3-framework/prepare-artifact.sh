#!/usr/bin/env bash
# C3 full-stack frameworks (solid | qwik | waku) on Workers — scaffold via create-cloudflare.
set -euo pipefail
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
FRAMEWORK="${CELP_C3_FRAMEWORK:?set CELP_C3_FRAMEWORK (solid|qwik|waku)}"
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
SCAFFOLD="${APP_DIR}/cellp-${FRAMEWORK}-app"
OVERLAY="${ROOT}/dev/examples/support-c3-framework/wrangler.${FRAMEWORK}.cellp.jsonc"
[[ -f "$OVERLAY" ]] || { echo "missing overlay ${OVERLAY}" >&2; exit 1; }

log() { echo "prepare-artifact: $*"; }

mkdir -p "$APP_DIR"

if [[ ! -f "${SCAFFOLD}/package.json" ]]; then
  log "scaffold create-cloudflare framework=${FRAMEWORK} platform=workers"
  rm -rf "$SCAFFOLD"
  mkdir -p "$SCAFFOLD"
  (
    cd "$APP_DIR"
    CI=1 npx --yes create-cloudflare@2.51.0 "${SCAFFOLD##*/}" \
      --framework="${FRAMEWORK}" \
      --platform=workers \
      --lang=ts \
      --no-git \
      --install=pnpm \
      --accept-defaults \
      2>/dev/null || CI=1 npx --yes create-cloudflare@2.51.0 "${SCAFFOLD##*/}" \
      --framework="${FRAMEWORK}" \
      --platform=workers \
      --lang=ts \
      --no-git \
      --install=npm \
      --accept-defaults
  )
  [[ -f "${SCAFFOLD}/package.json" ]] || { echo "C3 scaffold failed for ${FRAMEWORK}" >&2; exit 1; }
fi

cd "$SCAFFOLD"

export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-false}"
log "build"
if grep -q '"build"' package.json; then
  npm run build
fi

log "wrangler dry-run bundle"
rm -rf .cellp-bundle
CFG="wrangler.jsonc"
[[ -f wrangler.toml ]] && CFG="wrangler.toml"
npx --yes wrangler@4 deploy --config "$CFG" --dry-run --outdir .cellp-bundle
if [[ -f .cellp-bundle/_worker.js && ! -f .cellp-bundle/index.js ]]; then
  cp .cellp-bundle/_worker.js .cellp-bundle/index.js
fi
[[ -f .cellp-bundle/index.js ]] || { echo "missing .cellp-bundle/index.js" >&2; exit 1; }

ASSET_SRC=""
for d in dist/client public .output/public .vinxi/build/client; do
  if [[ -d "$d" ]]; then ASSET_SRC="$d"; break; fi
done

rm -rf .cellp-assets
mkdir -p .cellp-assets
if [[ -n "$ASSET_SRC" ]]; then
  log "stage assets from ${ASSET_SRC}"
  rsync -a "$ASSET_SRC/" .cellp-assets/
fi

export OVERLAY_PATH="$OVERLAY"
node <<'NODE'
const fs = require('fs');
const overlay = JSON.parse(fs.readFileSync(process.env.OVERLAY_PATH, 'utf8'));
const candidates = ['wrangler.jsonc', 'wrangler.toml'];
const p = candidates.find((c) => fs.existsSync(c));
if (!p) throw new Error('no wrangler config');
let raw = fs.readFileSync(p, 'utf8');
let j;
if (p.endsWith('.jsonc')) {
  try { j = JSON.parse(raw); } catch {
    raw = raw.replace(/^\s*\/\/.*$/gm, '').replace(/,\s*([}\]])/g, '$1');
    j = JSON.parse(raw);
  }
} else {
  throw new Error('toml patch not implemented; use jsonc overlay after build');
}
j.name = overlay.name || j.name;
j.compatibility_date = overlay.compatibility_date || j.compatibility_date;
j.compatibility_flags = overlay.compatibility_flags || j.compatibility_flags || ['nodejs_compat'];
j.main = '.cellp-bundle/index.js';
j.no_bundle = true;
j.assets = j.assets || {};
j.assets.binding = overlay.assets?.binding || 'ASSETS';
j.assets.directory = '.cellp-assets';
fs.writeFileSync('wrangler.jsonc', JSON.stringify(j, null, 2) + '\n');
NODE

log "publish slim artifact paths → ${APP_DIR}"
cp wrangler.jsonc "$APP_DIR/"
rm -rf "${APP_DIR}/.cellp-bundle" "${APP_DIR}/.cellp-assets"
rsync -a .cellp-bundle/ "${APP_DIR}/.cellp-bundle/"
rsync -a .cellp-assets/ "${APP_DIR}/.cellp-assets/"

log "ok: ${FRAMEWORK} bundle + .cellp-assets"
