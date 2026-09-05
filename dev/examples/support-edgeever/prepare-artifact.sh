#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=dev/scripts/support-pnpm.sh
source "${ROOT}/dev/scripts/support-pnpm.sh"
cellp_ensure_pnpm
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"
APP_DIR="${1:?app dir}"
cd "$APP_DIR"
[[ -f .wrangler/edgeever-worker/index.js ]] || {
  echo "prepare-artifact: run bun run build:worker first" >&2
  exit 1
}
[[ -d apps/web/dist ]] || {
  echo "prepare-artifact: run bun run build:web first" >&2
  exit 1
}
mkdir -p .cellp-bundle
if command -v bun >/dev/null; then
  bun x esbuild .wrangler/edgeever-worker/index.js \
    --bundle --format=esm --platform=neutral \
    --outfile=.cellp-bundle/index.js
elif command -v pnpm exec >/dev/null; then
  pnpm exec --yes esbuild .wrangler/edgeever-worker/index.js \
    --bundle --format=esm --platform=neutral \
    --outfile=.cellp-bundle/index.js
else
  echo "prepare-artifact: need bun or pnpm exec for esbuild" >&2
  exit 1
fi
echo "prepare-artifact: cellp bundle $(wc -c < .cellp-bundle/index.js) bytes, web dist ok"
