#!/usr/bin/env bash
# Seed commerce-store v1 — standalone D1 commerce API for local dev / Dashboard.
# Usage: ./dev/scripts/seed-commerce-store.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

# shellcheck disable=SC1091
source dev/.env
# shellcheck disable=SC1091
source e2e/scripts/lib.sh

PROJECT="${COMMERCE_PROJECT:-commerce-store}"
VERSION="${COMMERCE_VERSION:-v1}"
EXAMPLE="${ROOT}/dev/examples/commerce"
DEST="${ARTIFACTS_DIR}/${PROJECT}/${VERSION}"

need() { command -v "$1" >/dev/null || { echo "MISSING: $1" >&2; exit 1; }; }
need sqlite3

require_platform
require_celld

log() { echo "==> $*"; }

purge_destroyed() {
  local db="${CELLP_REGISTRY_DB:-${REGISTRY_DB:-./dev/data/cellp-registry.sqlite}}"
  [[ "$db" != /* ]] && db="${ROOT}/${db#./}"
  sqlite3 "$db" "DELETE FROM versions WHERE project_id='${PROJECT}' AND id='${VERSION}' AND status='destroyed';" 2>/dev/null || true
}

destroy_version_if_exists() {
  local status
  status=$(api_get "/v1/projects/${PROJECT}/versions/${VERSION}" 2>/dev/null | jq -r .status 2>/dev/null || echo "")
  [[ -z "$status" || "$status" == "null" ]] && return 0
  if [[ "$status" == "destroyed" ]]; then
    log "skip destroy ${VERSION} (already destroyed)"
    return 0
  fi
  log "destroy existing ${VERSION} (status=${status})"
  api_delete "/v1/projects/${PROJECT}/versions/${VERSION}" "$ADMIN_TOKEN" >/dev/null 2>&1 || true
  for _ in $(seq 1 60); do
    status=$(api_get "/v1/projects/${PROJECT}/versions/${VERSION}" 2>/dev/null | jq -r .status 2>/dev/null || echo "")
    [[ "$status" == "destroyed" ]] && return 0
    sleep 1
  done
  fail "timeout destroying ${VERSION} (last status=${status})"
}

print_summary() {
  echo ""
  echo "Commerce store ready:"
  echo "  Project:   ${PROJECT}"
  echo "  Version:   ${VERSION} (production)"
  echo "  API:       ${GATEWAY_URL}/${PROJECT}/${VERSION}/health"
  echo "  Stats:     ${GATEWAY_URL}/${PROJECT}/${VERSION}/stats"
  echo "  Products:  ${GATEWAY_URL}/${PROJECT}/${VERSION}/products"
  echo "  Dashboard: http://127.0.0.1:5173/projects/${PROJECT}"
  echo "  D1:        http://127.0.0.1:5173/projects/${PROJECT}/storage/${VERSION}/browser"
  echo ""
}

log "seed project=${PROJECT} version=${VERSION}"

[[ -d "$EXAMPLE" ]] || fail "missing ${EXAMPLE}"

ensure_project "$PROJECT"
destroy_version_if_exists
purge_destroyed

log "stage commerce worker + D1 seed"
stage_worker_example "$EXAMPLE" "$DEST"
"${EXAMPLE}/seed.sh" "${DEST}/seed.db" 80 200 400

create_version "$PROJECT" "$VERSION" | jq -r .id >/dev/null
poll_version "$PROJECT" "$VERSION" ready 180 >/dev/null
log "${VERSION} ready"

api_status POST "/v1/projects/${PROJECT}/versions/${VERSION}/promote" "{}" "$ADMIN_TOKEN"
[[ "$API_STATUS" == "200" ]] || fail "promote → HTTP ${API_STATUS} ${API_BODY}"
log "${VERSION} promoted to production"

wait_http_200 "${GATEWAY_URL}/${PROJECT}/${VERSION}/health" 60

api_status GET "/v1/projects/${PROJECT}/versions/${VERSION}/database/tables/products/rows?limit=1"
ROWS=$(echo "$API_BODY" | jq '.rows | length' 2>/dev/null || echo 0)
[[ "$ROWS" -ge 1 ]] || fail "D1 products empty after deploy"

STATS=$(curl -sf "${GATEWAY_URL}/${PROJECT}/${VERSION}/stats")
echo "$STATS" | jq -e '.products >= 1 and .customers >= 1 and .orders >= 1' >/dev/null \
  || fail "stats incomplete: ${STATS}"

print_summary
pass "${PROJECT}/${VERSION} seeded"
