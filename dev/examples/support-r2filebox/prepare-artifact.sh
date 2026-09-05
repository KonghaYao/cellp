#!/usr/bin/env bash
# Pre-bundle worker with wrangler; celld deploy OOM on full esbuild in low-memory dev.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=dev/scripts/support-pnpm.sh
source "${ROOT}/dev/scripts/support-pnpm.sh"
cellp_ensure_pnpm
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"
APP_DIR="${1:?app dir}"
cd "$APP_DIR"
if [[ ! -f wrangler.jsonc ]]; then
  echo "prepare-artifact: missing wrangler.jsonc" >&2
  exit 1
fi
rm -rf .cellp-bundle
pnpm exec --yes wrangler deploy --config wrangler.jsonc --dry-run --outdir .cellp-bundle
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
echo "prepare-artifact: bundled $(wc -c < .cellp-bundle/index.js) bytes → wrangler no_bundle"
