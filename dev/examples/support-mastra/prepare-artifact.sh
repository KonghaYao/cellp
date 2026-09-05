#!/usr/bin/env bash
# Mastra: mastra build → wrangler dry-run bundle → celld (single .cellp-bundle).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=dev/scripts/support-pnpm.sh
source "${ROOT}/dev/scripts/support-pnpm.sh"
cellp_ensure_pnpm
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"

APP_DIR="${1:?app dir}"
OVERLAY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$APP_DIR"

log() { echo "prepare-artifact: $*"; }

export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-true}"

if [[ ! -d node_modules ]]; then
  log "cellp_pnpm_install"
  cellp_pnpm_install --legacy-peer-deps
fi

log "mastra build (CloudflareDeployer → .mastra/output)"
pnpm run build

[[ -f .mastra/output/index.mjs ]] || { echo "missing .mastra/output/index.mjs" >&2; exit 1; }

log "merge wrangler (deployer config + cellp D1/R2)"
node "${OVERLAY_DIR}/merge-wrangler.mjs" "$APP_DIR" "${OVERLAY_DIR}/wrangler.cellp.jsonc"

log "wrangler dry-run bundle (~18MiB; gzip ~3.5MiB)"
rm -rf .cellp-bundle
(
  cd .mastra/output
  pnpm exec wrangler deploy --config wrangler.cellp.json --dry-run --outdir "${APP_DIR}/.cellp-bundle"
)
test -f .cellp-bundle/index.js

export SUPPORT_RSYNC_NO_NODE=1
log "ok: .cellp-bundle/index.js + wrangler.jsonc ($(wc -c < .cellp-bundle/index.js | tr -d ' ') bytes)"
