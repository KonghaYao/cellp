#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=dev/scripts/support-pnpm.sh
source "${ROOT}/dev/scripts/support-pnpm.sh"
cellp_ensure_pnpm
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"
APP_DIR="${1:?app dir}"
cd "$APP_DIR"
rm -rf .cellp-bundle
pnpm exec wrangler deploy --dry-run --outdir=.cellp-bundle
test -f .cellp-bundle/index.js
node -e "
const fs = require('fs');
const p = 'wrangler.jsonc';
const j = JSON.parse(fs.readFileSync(p, 'utf8'));
j.main = '.cellp-bundle/index.js';
fs.writeFileSync(p, JSON.stringify(j, null, 2) + '\n');
"
echo "prepare-artifact: sonicjs bundle $(wc -c < .cellp-bundle/index.js) bytes"
