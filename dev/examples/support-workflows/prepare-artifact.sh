#!/usr/bin/env bash
# Vite+Workflow template: patch worker for ASSETS, bundle, stage client for celld.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=dev/scripts/support-pnpm.sh
source "${ROOT}/dev/scripts/support-pnpm.sh"
cellp_ensure_pnpm
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"
APP_DIR="${1:?app dir}"
cd "$APP_DIR"

node <<'NODE'
const fs = require('fs');
const workerPath = 'worker/index.ts';
let src = fs.readFileSync(workerPath, 'utf8');
if (!src.includes('ASSETS?: { fetch')) {
  const needle = 'return Response.json({ error: "Not Found" }, { status: 404 });';
  const replacement = `const assets = (env as { ASSETS?: { fetch: (req: Request) => Promise<Response> } }).ASSETS;
\t\tif (assets) {
\t\t\treturn assets.fetch(request);
\t\t}
\t\treturn Response.json({ error: "Not Found" }, { status: 404 });`;
  const legacy = `\t\tif (env.ASSETS) {
\t\t\treturn env.ASSETS.fetch(request);
\t\t}
\t\treturn Response.json({ error: "Not Found" }, { status: 404 });`;
  if (src.includes(legacy)) {
    src = src.replace(legacy, replacement.trim().replace(/\n/g, '\n\t\t'));
  } else if (src.includes(needle)) {
    src = src.replace(needle, replacement);
  } else {
    console.error('prepare-artifact: worker/index.ts patch anchor missing');
    process.exit(1);
  }
  fs.writeFileSync(workerPath, src);
}
NODE

cellp_pnpm_install
pnpm run build
[[ -f dist/support_workflows/index.js ]] || { echo "missing dist/support_workflows/index.js" >&2; exit 1; }
[[ -f dist/client/index.html ]] || { echo "missing dist/client" >&2; exit 1; }

rm -rf .cellp-assets
mkdir -p .cellp-assets
rsync -a --exclude '.assetsignore' dist/client/ .cellp-assets/

node <<'NODE'
const fs = require('fs');
const p = 'wrangler.jsonc';
let raw = fs.readFileSync(p, 'utf8');
raw = raw.replace(/\/\/[^\n]*/g, '').replace(/,\s*([}\]])/g, '$1');
const j = JSON.parse(raw);
j.main = 'dist/support_workflows/index.js';
j.no_bundle = true;
j.assets = j.assets || {};
j.assets.directory = '.cellp-assets';
j.assets.binding = 'ASSETS';
j.assets.not_found_handling = 'single-page-application';
j.assets.run_worker_first = true;
fs.writeFileSync(p, JSON.stringify(j, null, 2) + '\n');
NODE

echo "prepare-artifact: workflows worker + client assets ok"
