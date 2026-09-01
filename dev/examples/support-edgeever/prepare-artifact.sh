#!/usr/bin/env bash
set -euo pipefail
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
elif command -v npx >/dev/null; then
  npx --yes esbuild .wrangler/edgeever-worker/index.js \
    --bundle --format=esm --platform=neutral \
    --outfile=.cellp-bundle/index.js
else
  echo "prepare-artifact: need bun or npx for esbuild" >&2
  exit 1
fi
echo "prepare-artifact: cellp bundle $(wc -c < .cellp-bundle/index.js) bytes, web dist ok"
