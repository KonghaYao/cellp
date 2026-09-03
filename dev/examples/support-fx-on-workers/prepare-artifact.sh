#!/usr/bin/env bash
# fx-on-workers: wrangler bundle (wasm) → celld esbuild re-bundle (no no_bundle; celld no_bundle drops wasm).
set -euo pipefail
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
OVERLAY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$APP_DIR"

if [[ ! -f wrangler.jsonc ]]; then
  echo "prepare-artifact: missing wrangler.jsonc" >&2
  exit 1
fi

log() { echo "prepare-artifact: $*"; }

log "cellp fx overlay (HTTP /api/prompt)"
cp "${OVERLAY_DIR}/cellp-overlay/index.js" ./src/index.js
node "${OVERLAY_DIR}/cellp-overlay/patch-session.mjs" ./src/session.js

if [[ -f "${OVERLAY_DIR}/cellp-overlay/inject-wrangler-local-secrets.mjs" ]]; then
  AI_GATEWAY_API_KEY="${AI_GATEWAY_API_KEY:-}" FX_MODEL="${FX_MODEL:-minimax/minimax-m3-free}" \
    node "${OVERLAY_DIR}/cellp-overlay/inject-wrangler-local-secrets.mjs" ./wrangler.jsonc
fi

log "ensure fx-term.wasm in src/"
if [[ ! -f src/fx-term.wasm ]]; then
  test -f node_modules/libfx/fx-term.wasm
  cp node_modules/libfx/fx-term.wasm src/fx-term.wasm
fi

# Wrangler needs CompiledWasm rules; celld overlay omits them.
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
j.main = 'src/index.js';
j.rules = [{ type: 'CompiledWasm', globs: ['**/*.wasm'], fallthrough: true }];
delete j.no_bundle;
fs.writeFileSync(p, JSON.stringify(j, null, 2) + '\n');
NODE

log "wrangler dry-run bundle (~2.2 MiB gzip)"
rm -rf .cellp-bundle
npx --yes wrangler@4 deploy --config wrangler.jsonc --dry-run --outdir .cellp-bundle
test -f .cellp-bundle/index.js

node <<'NODE'
const fs = require('fs');
const p = 'wrangler.jsonc';
let raw = fs.readFileSync(p, 'utf8');
let j = JSON.parse(raw);
delete j.rules;
delete j.no_bundle;
j.main = '.cellp-bundle/index.js';
if (!j.vars) j.vars = {};
if (!j.vars.ACCESS_KEY) j.vars.ACCESS_KEY = 'cellp-dev-fx-on-workers';
raw = JSON.stringify(j, null, 2) + '\n';
raw = raw.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync(p, raw);
NODE

log "bundled $(wc -c < .cellp-bundle/index.js | tr -d ' ') bytes → celld esbuild from .cellp-bundle (wasm siblings kept)"
