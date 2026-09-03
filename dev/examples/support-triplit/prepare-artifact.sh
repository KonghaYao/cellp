#!/usr/bin/env bash
# Triplit cf-worker-server (yarn monorepo): build workspace deps + wrangler dry-run slim bundle.
set -euo pipefail
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
cd "$APP_DIR"
REPO_ROOT="$(cd ../.. && pwd)"

log() { echo "prepare-artifact: $*"; }

export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-false}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-false}"

if [[ ! -f wrangler.jsonc ]]; then
  echo "prepare-artifact: missing wrangler.jsonc" >&2
  exit 1
fi

log "yarn install at repo root"
(
  cd "$REPO_ROOT"
  corepack enable 2>/dev/null || true
  yarn install --immutable 2>/dev/null || yarn install
)

log "build @triplit/db + @triplit/server (workspace deps)"
(
  cd "$REPO_ROOT"
  yarn turbo build --filter=@triplit/db --filter=@triplit/server --force
)

log "wrangler dry-run bundle"
rm -rf .cellp-bundle
npx --yes wrangler@4 deploy --config wrangler.jsonc --dry-run --outdir .cellp-bundle
if [[ -f .cellp-bundle/_worker.js && ! -f .cellp-bundle/index.js ]]; then
  cp .cellp-bundle/_worker.js .cellp-bundle/index.js
elif [[ -f .cellp-bundle/standard.js && ! -f .cellp-bundle/index.js ]]; then
  cp .cellp-bundle/standard.js .cellp-bundle/index.js
fi
[[ -f .cellp-bundle/index.js ]] || { echo "prepare-artifact: missing .cellp-bundle/index.js (have: $(ls .cellp-bundle 2>/dev/null))" >&2; exit 1; }

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
j.name = j.name || 'support-triplit';
fs.writeFileSync(p, JSON.stringify(j, null, 2) + '\n');
NODE

log "bundled $(wc -c < .cellp-bundle/index.js | tr -d ' ') bytes → wrangler no_bundle"
