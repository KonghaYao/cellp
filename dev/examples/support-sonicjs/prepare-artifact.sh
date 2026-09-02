#!/usr/bin/env bash
set -euo pipefail
APP_DIR="${1:?app dir}"
cd "$APP_DIR"
rm -rf .cellp-bundle
npx wrangler deploy --dry-run --outdir=.cellp-bundle
test -f .cellp-bundle/index.js
node -e "
const fs = require('fs');
const p = 'wrangler.jsonc';
const j = JSON.parse(fs.readFileSync(p, 'utf8'));
j.main = '.cellp-bundle/index.js';
fs.writeFileSync(p, JSON.stringify(j, null, 2) + '\n');
"
echo "prepare-artifact: sonicjs bundle $(wc -c < .cellp-bundle/index.js) bytes"
