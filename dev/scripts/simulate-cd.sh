#!/usr/bin/env bash
# Local CD simulation — deploy example worker + register version (Host ingress).
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
# shellcheck disable=SC1091
source e2e/scripts/lib-ingress.sh
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

echo "==> [3/5] register version in platform API"
RESP=$(curl -sf -X POST "${PLATFORM_URL}/v1/projects/${PROJECT}/versions" \
  -H "Authorization: Bearer ${PLATFORM_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"id\":\"${VERSION}\",\"git_ref\":\"local\",\"git_sha\":\"local\",\"parent_version_id\":null}")

echo "$RESP" | jq .

PREVIEW=$(echo "$RESP" | jq -r .preview_url)
PREVIEW_HOST=$(preview_host "$PROJECT" "$VERSION")
echo "==> [4/5] preview URL: ${PREVIEW:-Host ${PREVIEW_HOST} on ${GATEWAY_URL}}"

echo "==> [5/5] smoke test (Host ingress)"
for _ in $(seq 1 120); do
  STATUS=$(curl -sf -H "Authorization: Bearer ${CELLP_ADMIN_TOKEN:-$PLATFORM_TOKEN}" \
    "${PLATFORM_URL}/v1/projects/${PROJECT}/versions/${VERSION}" | jq -r .status)
  [[ "$STATUS" == "ready" ]] && break
  sleep 1
done
[[ "${STATUS:-}" == "ready" ]] || { echo "FAIL version not ready (status=${STATUS:-?})"; exit 1; }

READY_PREVIEW=$(version_preview_url "$PROJECT" "$VERSION")
SMOKE_HOST="$PREVIEW_HOST"
if [[ -n "$READY_PREVIEW" ]]; then
  SMOKE_HOST=$(python3 -c "import urllib.parse; print(urllib.parse.urlparse('$READY_PREVIEW').hostname or '')" 2>/dev/null || echo "$PREVIEW_HOST")
fi

HTTP=$(curl -sf -o /tmp/cell-preview-body.json -w '%{http_code}' \
  -H "Host: ${SMOKE_HOST}" "${GATEWAY_URL}/" || echo "000")
if [[ "$HTTP" == "200" ]]; then
  echo "OK smoke test HTTP 200 Host=${SMOKE_HOST}"
  cat /tmp/cell-preview-body.json
  echo ""
  exit 0
else
  echo "FAIL smoke test HTTP $HTTP Host=${SMOKE_HOST}"
  exit 1
fi
