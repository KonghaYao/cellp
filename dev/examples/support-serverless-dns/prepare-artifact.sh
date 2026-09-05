#!/usr/bin/env bash
# serverless-dns: webpack dist/worker.js (ESM) → wrangler dry-run slim bundle for celld.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=dev/scripts/support-pnpm.sh
source "${ROOT}/dev/scripts/support-pnpm.sh"
cellp_ensure_pnpm
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
cd "$APP_DIR"

log() { echo "prepare-artifact: $*"; }

if [[ ! -f wrangler.jsonc ]]; then
  echo "prepare-artifact: missing wrangler.jsonc (cellp overlay should be applied first)" >&2
  exit 1
fi

export NPM_CONFIG_REGISTRY="${NPM_CONFIG_REGISTRY:-https://registry.npmmirror.com}"
export npm_config_registry="${npm_config_registry:-$NPM_CONFIG_REGISTRY}"
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-false}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-false}"

ensure_blocklist_configs() {
  if [[ -f src/u6-basicconfig.json && -s src/u6-filetag.json ]]; then
    log "blocklist configs present"
    return 0
  fi
  if ! command -v wget >/dev/null 2>&1; then
    echo "prepare-artifact: wget required for src/build/pre.sh (brew install wget)" >&2
    exit 1
  fi
  log "src/build/pre.sh → u6-basicconfig + u6-filetag"
  ./src/build/pre.sh || true
  if [[ -f src/u6-basicconfig.json && ! -s src/u6-filetag.json ]]; then
    local ts
    ts="$(node -e "console.log(JSON.parse(require('fs').readFileSync('src/u6-basicconfig.json','utf8')).timestamp)")"
    log "fetch filetag for timestamp ${ts}"
    wget -q "https://cfstore.rethinkdns.com/blocklists/${ts}/u6/filetag.json" -O src/u6-filetag.json
  fi
  [[ -f src/u6-basicconfig.json && -s src/u6-filetag.json ]] || {
    echo "prepare-artifact: missing src/u6-basicconfig.json or src/u6-filetag.json" >&2
    exit 1
  }
}

ensure_blocklist_configs

log "patch dnsutil request timeout cap (blocklist cold-start on cellp)"
node <<'NODE'
const fs = require('fs');
const p = 'src/commons/dnsutil.js';
let s = fs.readFileSync(p, 'utf8');
if (s.includes('120000; // 120s (cellp overlay)')) {
  console.log('prepare-artifact: dnsutil cap already patched');
} else {
  const from = 'const _maxRequestTimeout = 30000; // 30s';
  const to = 'const _maxRequestTimeout = 120000; // 120s (cellp overlay)';
  if (!s.includes(from)) {
    console.error('prepare-artifact: dnsutil cap patch mismatch');
    process.exit(1);
  }
  fs.writeFileSync(p, s.replace(from, to));
}
NODE

log "cellp_pnpm_install"
cellp_pnpm_install --ignore-scripts

log "webpack build → dist/worker.js"
pnpm run build

[[ -f dist/worker.js ]] || {
  echo "prepare-artifact: missing dist/worker.js" >&2
  exit 1
}

log "wrangler dry-run bundle"
rm -rf .cellp-bundle
pnpm exec --yes wrangler@4 deploy --config wrangler.jsonc --dry-run --outdir .cellp-bundle
if [[ -f .cellp-bundle/worker.js && ! -f .cellp-bundle/index.js ]]; then
  cp .cellp-bundle/worker.js .cellp-bundle/index.js
fi
if [[ -f .cellp-bundle/index.js && ! -f .cellp-bundle/worker.js ]]; then
  cp .cellp-bundle/index.js .cellp-bundle/worker.js
fi
[[ -f .cellp-bundle/index.js ]] || {
  echo "prepare-artifact: missing .cellp-bundle/index.js (have: $(ls .cellp-bundle 2>/dev/null || true))" >&2
  exit 1
}

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
j.name = j.name || 'support-serverless-dns';
fs.writeFileSync(p, JSON.stringify(j, null, 2) + '\n');
NODE

log "bundled $(wc -c < .cellp-bundle/index.js | tr -d ' ') bytes → wrangler no_bundle"
