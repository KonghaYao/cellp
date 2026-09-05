#!/usr/bin/env bash
# Counterscale (pnpm turbo monorepo, packages/server): React Router 7 + wrangler dry-run slim bundle.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=dev/scripts/support-pnpm.sh
source "${ROOT}/dev/scripts/support-pnpm.sh"
cellp_ensure_pnpm
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
cd "$APP_DIR"
REPO_ROOT="$(cd ../.. && pwd)"

log() { echo "prepare-artifact: $*"; }

export NPM_CONFIG_REGISTRY="${NPM_CONFIG_REGISTRY:-https://registry.npmmirror.com}"
export npm_config_registry="${npm_config_registry:-$NPM_CONFIG_REGISTRY}"
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-false}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-false}"

if [[ ! -f wrangler.jsonc ]]; then
  echo "prepare-artifact: missing wrangler.jsonc (overlay should be applied)" >&2
  exit 1
fi

log "pnpm install at repo root"
(
  cd "$REPO_ROOT"
  corepack enable 2>/dev/null || true
  pnpm install --frozen-lockfile 2>/dev/null || pnpm install
)

log "turbo build (tracker + server)"
(
  cd "$REPO_ROOT"
  pnpm run build
)

[[ -d build/client ]] || {
  echo "prepare-artifact: missing build/client (react-router build)" >&2
  exit 1
}

log "wrangler dry-run bundle"
rm -rf .cellp-bundle
pnpm exec --yes wrangler@4 deploy --config wrangler.jsonc --dry-run --outdir .cellp-bundle
if [[ -f .cellp-bundle/app.js && ! -f .cellp-bundle/index.js ]]; then
  cp .cellp-bundle/app.js .cellp-bundle/index.js
elif [[ -f .cellp-bundle/_worker.js && ! -f .cellp-bundle/index.js ]]; then
  cp .cellp-bundle/_worker.js .cellp-bundle/index.js
fi
[[ -f .cellp-bundle/index.js ]] || {
  echo "prepare-artifact: missing .cellp-bundle/index.js (have: $(ls .cellp-bundle 2>/dev/null | tr '\n' ' '))" >&2
  exit 1
}

log "stage build/client → .cellp-assets"
rm -rf .cellp-assets
mkdir -p .cellp-assets
rsync -a build/client/ .cellp-assets/
rm -f .cellp-assets/.assetsignore

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
j.name = j.name || 'support-counterscale';
j.assets = j.assets || {};
j.assets.directory = '.cellp-assets';
j.assets.binding = j.assets.binding || 'ASSETS';
j.assets.not_found_handling = j.assets.not_found_handling || 'single-page-application';
delete j.triggers;
delete j.analytics_engine_datasets;
let out = JSON.stringify(j, null, 2) + '\n';
out = out.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync(p, out);
NODE

log "bundled $(wc -c < .cellp-bundle/index.js | tr -d ' ') bytes → wrangler no_bundle + SPA assets"
