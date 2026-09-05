#!/usr/bin/env bash
# Preview smoke before promote (S40): static root/chunk, dynamic SSR/API, Next 404.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
PROJECT="${1:?project}"
VERSION="${2:?version}"
# shellcheck disable=SC1091
source "${ROOT}/dev/.env" 2>/dev/null || true
# shellcheck disable=SC1091
source "${ROOT}/e2e/scripts/lib-ingress.sh"
GW="${GATEWAY_URL:-http://127.0.0.1:8787}"
HOST="$(preview_host "$PROJECT" "$VERSION")"

curl_host() {
  curl -sS -H "Host: ${HOST}" "${GW}${1}"
}
code_host() {
  curl -sS -o /dev/null -w '%{http_code}' -H "Host: ${HOST}" "${GW}${1}"
}

log() { echo "smoke-preview: $*"; }

ROOT_CODE="$(code_host /)"
[[ "$ROOT_CODE" == "200" ]] || { log "FAIL GET / HTTP ${ROOT_CODE}"; exit 1; }
ROOT_BODY="$(curl_host /)"
echo "$ROOT_BODY" | grep -q 'Hello, Next.js!' || { log "FAIL / missing Hello, Next.js!"; exit 1; }

ASSET="$(echo "$ROOT_BODY" | grep -oE '/_next/static/[^" ]+' | head -1 || true)"
[[ -n "$ASSET" ]] || { log "FAIL no static asset in / HTML"; exit 1; }
ASSET_CODE="$(code_host "$ASSET")"
[[ "$ASSET_CODE" == "200" ]] || { log "FAIL ${ASSET} HTTP ${ASSET_CODE}"; exit 1; }

DYN_CODE="$(code_host /dynamic)"
[[ "$DYN_CODE" == "200" ]] || { log "FAIL GET /dynamic HTTP ${DYN_CODE}"; exit 1; }
DYN_BODY_1="$(curl_host /dynamic)"
echo "$DYN_BODY_1" | grep -q 'S40 dynamic route' || { log "FAIL /dynamic heading"; exit 1; }
DYN_TS_1="$(echo "$DYN_BODY_1" | grep -oE 'data-cellp-ts="[^"]+"' | head -1 | cut -d'"' -f2 || true)"
sleep 0.02
DYN_BODY_2="$(curl_host /dynamic)"
DYN_TS_2="$(echo "$DYN_BODY_2" | grep -oE 'data-cellp-ts="[^"]+"' | head -1 | cut -d'"' -f2 || true)"
[[ -n "$DYN_TS_1" && -n "$DYN_TS_2" && "$DYN_TS_1" != "$DYN_TS_2" ]] || {
  log "FAIL /dynamic did not produce distinct render timestamps"
  exit 1
}

API_CODE="$(code_host /api/health)"
[[ "$API_CODE" == "200" ]] || { log "FAIL GET /api/health HTTP ${API_CODE}"; exit 1; }
API_BODY_1="$(curl_host /api/health)"
echo "$API_BODY_1" | jq -e '
  .marker == "cellp-support-next-basic-v2" and
  .pathname == "/api/health" and
  (.ts | type == "string" and length > 0)
' >/dev/null || { log "FAIL /api/health payload"; exit 1; }
API_TS_1="$(echo "$API_BODY_1" | jq -r .ts)"
sleep 0.02
API_BODY_2="$(curl_host /api/health)"
API_TS_2="$(echo "$API_BODY_2" | jq -er '.ts | select(type == "string" and length > 0)' 2>/dev/null || true)"
[[ -n "$API_TS_2" && "$API_TS_1" != "$API_TS_2" ]] || {
  log "FAIL /api/health did not produce distinct timestamps"
  exit 1
}

NOT_FOUND_CODE="$(code_host /cellp-s40-missing-route)"
[[ "$NOT_FOUND_CODE" == "404" ]] || {
  log "FAIL missing route HTTP ${NOT_FOUND_CODE}, wanted 404"
  exit 1
}
NOT_FOUND_BODY="$(curl_host /cellp-s40-missing-route)"
echo "$NOT_FOUND_BODY" | grep -q 'This page could not be found' || {
  log "FAIL missing route did not render Next.js 404"
  exit 1
}

log "OK Host=${HOST} /=${ROOT_CODE} asset=${ASSET_CODE} /dynamic=${DYN_CODE} /api/health=${API_CODE} missing=${NOT_FOUND_CODE}"
