#!/usr/bin/env bash
# pi-worker monorepo: hello-agent depends on workspace:pi-worker — build package + wrangler bundle.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=dev/scripts/support-pnpm.sh
source "${ROOT}/dev/scripts/support-pnpm.sh"
cellp_ensure_pnpm
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
MONO_ROOT="$(cd "$APP_DIR/../.." && pwd)"
PKG_DIR="${MONO_ROOT}/packages/pi-worker"
OVERLAY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

log() { echo "prepare-artifact: $*"; }

export NPM_CONFIG_REGISTRY="${NPM_CONFIG_REGISTRY:-https://registry.npmmirror.com}"
export npm_config_registry="${npm_config_registry:-$NPM_CONFIG_REGISTRY}"
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-true}"

log "build pi-worker package"
cd "$PKG_DIR"
if [[ ! -d node_modules/typescript ]]; then
  cellp_pnpm_install --no-workspaces
fi
./node_modules/.bin/tsc -p tsconfig.build.json
[[ -f dist/index.js ]] || { echo "prepare-artifact: missing packages/pi-worker/dist" >&2; exit 1; }

CELLP_HELLO_SRC="${OVERLAY_DIR}/hello-agent.src/index.ts"
if [[ -f "$CELLP_HELLO_SRC" ]]; then
  log "cellp hello-agent overlay src"
  cp "$CELLP_HELLO_SRC" "$APP_DIR/src/index.ts"
fi

log "link hello-agent → file:../../packages/pi-worker"
cd "$APP_DIR"
node <<'NODE'
const fs = require('fs');
const p = 'package.json';
const j = JSON.parse(fs.readFileSync(p, 'utf8'));
j.dependencies = j.dependencies || {};
j.dependencies['pi-worker'] = 'file:../../packages/pi-worker';
fs.writeFileSync(p, JSON.stringify(j, null, 2) + '\n');
NODE
cellp_pnpm_install --no-workspaces

[[ -f wrangler.jsonc ]] || { echo "prepare-artifact: missing wrangler.jsonc" >&2; exit 1; }

log "wrangler dry-run bundle"
rm -rf .cellp-bundle
pnpm exec wrangler deploy --config wrangler.jsonc --dry-run --outdir .cellp-bundle
test -f .cellp-bundle/index.js

node <<'NODE'
const fs = require('fs');
const p = 'wrangler.jsonc';
let raw = fs.readFileSync(p, 'utf8');
const j = JSON.parse(raw.replace(/\/\/[^\n]*/g, '').replace(/,\s*([}\]])/g, '$1'));
j.main = '.cellp-bundle/index.js';
j.no_bundle = true;
raw = JSON.stringify(j, null, 2) + '\n';
raw = raw.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync(p, raw);
NODE

log "bundled $(wc -c < .cellp-bundle/index.js | tr -d ' ') bytes → wrangler no_bundle"
