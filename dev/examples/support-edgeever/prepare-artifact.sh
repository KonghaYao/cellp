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
echo "prepare-artifact: edgeever worker $(wc -c < .wrangler/edgeever-worker/index.js) bytes, web dist ok"
