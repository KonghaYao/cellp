#!/usr/bin/env bash
# Install template consumer app + pre-bundle worker for celld.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=dev/scripts/support-pnpm.sh
source "${ROOT}/dev/scripts/support-pnpm.sh"
cellp_ensure_pnpm
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"
APP_DIR="${1:?app dir}"
cd "$APP_DIR"
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-true}"
( cd template && cellp_pnpm_install )
if [[ ! -f wrangler.jsonc ]]; then
  echo "prepare-artifact: missing wrangler.jsonc" >&2
  exit 1
fi
if [[ ! -d template/node_modules/r2-explorer/dashboard ]]; then
  echo "prepare-artifact: missing r2-explorer dashboard assets" >&2
  exit 1
fi
rm -rf .cellp-bundle
pnpm exec --yes wrangler deploy --config wrangler.jsonc --dry-run --outdir .cellp-bundle
test -f .cellp-bundle/index.js
rm -rf .cellp-assets
mkdir -p .cellp-assets
cp -R template/node_modules/r2-explorer/dashboard/. .cellp-assets/
node <<'NODE'
const fs = require('fs');
const p = 'wrangler.jsonc';
let raw = fs.readFileSync(p, 'utf8');
const j = JSON.parse(raw.replace(/\/\/[^\n]*/g, '').replace(/,\s*([}\]])/g, '$1'));
j.main = '.cellp-bundle/index.js';
j.no_bundle = true;
if (j.assets && typeof j.assets === 'object') {
  j.assets.directory = '.cellp-assets';
}
raw = JSON.stringify(j, null, 2) + '\n';
raw = raw.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync(p, raw);
NODE
echo "prepare-artifact: bundled $(wc -c < .cellp-bundle/index.js) bytes → wrangler no_bundle"
