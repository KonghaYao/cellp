#!/usr/bin/env bash
# Local CD simulation — deploy example worker + register version
# Usage: simulate-cd.sh <project> <version_id>
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

PROJECT="${1:?project id}"
VERSION="${2:?version id}"
EXAMPLE="${3:-dev/examples/counter}"

set -a
# shellcheck disable=SC1091
source dev/.env
set +a

need() { command -v "$1" >/dev/null || { echo "MISSING $1" >&2; exit 1; }; }
need celld
need curl
need jq

echo "==> [1/5] offshoot data branch (if available)"
if command -v offshoot >/dev/null 2>&1; then
  mkdir -p "$OFFSHOOT_STORE" "$OFFSHOOT_CHECKOUTS"
  offshoot --store "$OFFSHOOT_STORE" create "$PROJECT" 2>/dev/null || true
  offshoot --store "$OFFSHOOT_STORE" fork "$PROJECT" main "$VERSION" 2>/dev/null || \
    offshoot --store "$OFFSHOOT_STORE" fork "$PROJECT" "$PROJECT" "$VERSION" 2>/dev/null || \
    echo "WARN: offshoot fork skipped (seed main first manually)"
  EXPORT="$ROOT/dev/data/artifacts/$PROJECT/$VERSION/seed.db"
  mkdir -p "$(dirname "$EXPORT")"
  offshoot --store "$OFFSHOOT_STORE" export "$PROJECT@$VERSION" "$EXPORT" 2>/dev/null || true
else
  echo "SKIP offshoot — not installed"
fi

echo "==> [2/5] celld deploy $EXAMPLE for $PROJECT/$VERSION"
export CELLD_VAR_PROJECT_ID="$PROJECT"
export CELLD_VAR_VERSION_ID="$VERSION"

(
  cd "$EXAMPLE"
  celld deploy . --bucket "$CELLD_BUCKET" --endpoint "$S3_ENDPOINT" --region "$AWS_REGION"
)

# Restart celld to pick up deploy
if [[ -f dev/data/pids/celld.pid ]]; then
  kill "$(cat dev/data/pids/celld.pid)" 2>/dev/null || true
  sleep 1
fi
celld --bucket "$CELLD_BUCKET" --endpoint "$S3_ENDPOINT" --region "$AWS_REGION" \
  --listen "127.0.0.1:${CELLD_PORT}" >>dev/data/logs/celld.log 2>&1 &
echo $! > dev/data/pids/celld.pid

for i in $(seq 1 60); do
  curl -sf "http://127.0.0.1:${CELLD_PORT}/__celld/health" >/dev/null && break
  sleep 1
done

echo "==> [3/5] register version in platform API"
RESP=$(curl -sf -X POST "${PLATFORM_URL}/v1/projects/${PROJECT}/versions" \
  -H "Authorization: Bearer ${PLATFORM_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"id\":\"${VERSION}\",\"git_ref\":\"local\",\"git_sha\":\"local\",\"parent_version_id\":null}")

echo "$RESP" | jq .

PREVIEW=$(echo "$RESP" | jq -r .preview_url)
echo "==> [4/5] preview URL: $PREVIEW"

echo "==> [5/5] smoke test"
HTTP=$(curl -sf -o /tmp/cell-preview-body.json -w '%{http_code}' "${PREVIEW}" || echo "000")
if [[ "$HTTP" == "200" ]]; then
  echo "OK smoke test HTTP 200"
  cat /tmp/cell-preview-body.json
  echo ""
  exit 0
else
  echo "FAIL smoke test HTTP $HTTP"
  exit 1
fi
