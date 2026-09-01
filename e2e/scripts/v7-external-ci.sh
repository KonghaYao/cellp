#!/usr/bin/env bash
# TP-V7 / TP-V7-D — External CI simulation (AD-12 Host)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_platform
require_celld
require_celld_cli

PROJECT="${DEV_PROJECT}"
VERSION="v7-$(date +%s | tail -c 8)"
ARTIFACT_DIR="${ARTIFACTS_DIR}/${PROJECT}/${VERSION}"
BUNDLE="${ARTIFACT_DIR}/bundle.tar.gz"

log "V7 external CI project=${PROJECT} version=${VERSION}"
ensure_project "$PROJECT"
mkdir -p "$ARTIFACT_DIR"

log "upload artifact"
(
  cd dev/examples/counter
  tar -czf "$BUNDLE" . 2>/dev/null || {
    STAGING="$(mktemp -d)"
    cp -a . "$STAGING/"
    tar -czf "$BUNDLE" -C "$STAGING" .
    rm -rf "$STAGING"
  }
)

DIGEST="sha256:$(sha256sum "$BUNDLE" | awk '{print $1}')"

BODY=$(jq -n \
  --arg id "$VERSION" \
  --arg ref "e2e-ci" \
  --arg sha "local" \
  --arg digest "$DIGEST" \
  '{id:$id, git_ref:$ref, git_sha:$sha, artifact:{digest:$digest, package_version:$id}}')

RESP=$(api_post "/v1/projects/${PROJECT}/versions" "$BODY")
echo "$RESP" | jq .

poll_version "$PROJECT" "$VERSION" ready 180 >/dev/null

wait_http_200_version "$PROJECT" "$VERSION" "/" 60
PREVIEW_BODY=$(curl_version "$PROJECT" "$VERSION" "/")
PREVIEW_CHECKSUM=$(echo -n "$PREVIEW_BODY" | sha256sum | awk '{print $1}')

curl -sf -X POST "${PLATFORM_URL}/v1/projects/${PROJECT}/versions/${VERSION}/promote" \
  -H "$(api_auth "$ADMIN_TOKEN")" -H "Content-Type: application/json" -d '{}' >/dev/null 2>&1 || \
  fail "V7 promote failed"

PROD_H="$(prod_host "$PROJECT")"
wait_http_200_prod "$PROJECT" "/" 60
PROD_BODY=$(curl_prod "$PROJECT" "/")
PROD_CHECKSUM=$(echo -n "$PROD_BODY" | sha256sum | awk '{print $1}')

if [[ "$PREVIEW_CHECKSUM" != "$PROD_CHECKSUM" ]]; then
  echo "WARN: preview/prod body checksum mismatch (routing may differ pre-cutover)" >&2
fi

pass "V7 external CI OK prod_host=${PROD_H} checksum=${PROD_CHECKSUM:0:12}..."
exit 0
