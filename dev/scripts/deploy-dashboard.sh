#!/usr/bin/env bash
# Build web SPA and deploy as static assets on local cellp stack.
# Usage: deploy-dashboard.sh [version_id]
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

VERSION="${1:-v-ui1}"
PROJECT="cellp-dashboard"
EXAMPLE="dev/examples/dashboard-static"

set -a
# shellcheck disable=SC1091
source dev/.env
set +a

need() { command -v "$1" >/dev/null || { echo "MISSING: $1" >&2; exit 1; }; }
need curl
need jq
need docker

echo "==> [1/4] build dashboard SPA (flat dist for artifact fetch)"
(
  cd web
  test -f .env || cp .env.example .env
  npm run build
)

echo "==> [2/4] sync dist → ${EXAMPLE}/public/"
mkdir -p "$EXAMPLE/public"
find "$EXAMPLE" -mindepth 1 ! -name wrangler.jsonc ! -name .gitignore -exec rm -rf {} + 2>/dev/null || true
mkdir -p "$EXAMPLE/public"
cp web/dist/index.html web/dist/dashboard.js "$EXAMPLE/public/"
cp web/dist/dashboard.css "$EXAMPLE/public/" 2>/dev/null || true

echo "==> [3/4] upload artifact to s3://${PROJECT}/${VERSION}/"
docker run --rm --network host \
  -v "${ROOT}/${EXAMPLE}:/data:ro" \
  --entrypoint /bin/sh \
  minio/mc:latest \
  -c "
    mc alias set local ${S3_ENDPOINT} ${RUSTFS_ACCESS_KEY} ${RUSTFS_SECRET_KEY} &&
    mc mb -p local/cellp-artifacts 2>/dev/null || true &&
    mc rm -r --force local/cellp-artifacts/${PROJECT}/${VERSION}/ 2>/dev/null || true &&
    mc cp --recursive /data/ local/cellp-artifacts/${PROJECT}/${VERSION}/
  "

echo "==> [4/4] register version via simulate-cd"
export CELLD_ESBUILD="${ROOT}/dev/examples/counter/node_modules/.bin/esbuild"
./dev/scripts/simulate-cd.sh "$PROJECT" "$VERSION" "$EXAMPLE"

PREVIEW="${GATEWAY_URL}/${PROJECT}/${VERSION}/"
echo ""
echo "Dashboard deployed."
echo "  Preview: ${PREVIEW}"
echo "  API:     ${PLATFORM_URL}/v1/projects (Bearer ${PLATFORM_TOKEN})"
