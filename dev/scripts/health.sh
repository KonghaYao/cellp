#!/usr/bin/env bash
# Agent health check — exit 0 if all required services OK
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

set -a
# shellcheck disable=SC1091
source dev/.env 2>/dev/null || source dev/.env.example
set +a

FAIL=0
ok() { echo "OK  $1"; }
bad() { echo "FAIL $1"; FAIL=1; }

if curl -sf "${GATEWAY_URL}/health" >/dev/null; then ok "gateway ${GATEWAY_URL}"; else bad "gateway ${GATEWAY_URL}"; fi

GW_DEEP_CODE=$(curl -sS -o /dev/null -w '%{http_code}' "${GATEWAY_URL}/health/deep" 2>/dev/null || echo "000")
if [[ "$GW_DEEP_CODE" == "200" || "$GW_DEEP_CODE" == "503" ]]; then
  ok "gateway deep health (http=${GW_DEEP_CODE})"
else
  bad "gateway deep health (http=${GW_DEEP_CODE})"
fi

if curl -sf "${PLATFORM_URL}/v1/health" >/dev/null; then ok "platform ${PLATFORM_URL}"; else bad "platform ${PLATFORM_URL}"; fi

if curl -sf "http://127.0.0.1:${CELLD_PORT}/.well-known/celld/health" >/dev/null 2>&1; then
  ok "celld :${CELLD_PORT}"
else
  bad "celld :${CELLD_PORT} (install celld or run up.sh)"
fi

if curl -sf "http://127.0.0.1:${S3_PORT:-9000}/health" >/dev/null 2>&1; then
  ok "rustfs :${S3_PORT:-9000}"
else
  bad "rustfs :${S3_PORT:-9000}"
fi

if bash "${ROOT}/dev/scripts/check-s3-clock-skew.sh"; then
  ok "s3 clock skew"
else
  bad "s3 clock skew (see PD-20260902-04)"
fi

ADMIN="${CELLP_ADMIN_TOKEN:-${PLATFORM_TOKEN:-dev-local-token}}"

# Deep health: registry + RustFS + runtime fleet + queue
DEEP_TMP=$(mktemp)
DEEP_CODE=$(curl -sS -o "$DEEP_TMP" -w '%{http_code}' "${PLATFORM_URL}/v1/health/deep" 2>/dev/null || echo "000")
rm -f "$DEEP_TMP"
if [[ "$DEEP_CODE" == "200" || "$DEEP_CODE" == "503" ]]; then
  ok "platform deep health (http=${DEEP_CODE})"
else
  bad "platform deep health (http=${DEEP_CODE})"
fi

# Runtime route summary (admin)
ROUTES_TMP=$(mktemp)
ROUTES_CODE=$(curl -sS -o "$ROUTES_TMP" -w '%{http_code}' -H "Authorization: Bearer ${ADMIN}" "${PLATFORM_URL}/v1/runtime/routes" 2>/dev/null || echo "000")
rm -f "$ROUTES_TMP"
if [[ "$ROUTES_CODE" == "200" ]]; then
  ok "platform runtime routes"
else
  bad "platform runtime routes (http=${ROUTES_CODE})"
fi

METRICS_CODE=$(curl -sS -o /dev/null -w '%{http_code}' "${PLATFORM_URL}/metrics" 2>/dev/null || echo "000")
if [[ "$METRICS_CODE" == "200" ]]; then
  ok "platform prometheus /metrics"
else
  bad "platform prometheus /metrics (http=${METRICS_CODE})"
fi

if [[ -f dev/data/registry.json ]] || [[ -f dev/data/platform-registry.sqlite ]] || [[ -f dev/data/cellp-registry.sqlite ]] || [[ -f "${REGISTRY_DB:-dev/data/cellp-registry.sqlite}" ]]; then
  ok "registry file"
else
  echo "WARN registry empty (run seed-demo.sh first)"
fi

# Stale cellpd lacks Phase 7 operator routes (KV/queue) — chi returns "404 page not found".
PROBE="${PLATFORM_URL}/v1/projects/${DEV_PROJECT:-demo-app}/versions/__health_probe__/kv/"
PROBE_TMP=$(mktemp)
PROBE_CODE=$(curl -sS -o "$PROBE_TMP" -w '%{http_code}' -H "Authorization: Bearer ${ADMIN}" "$PROBE" 2>/dev/null || echo "000")
PROBE_BODY=$(cat "$PROBE_TMP" 2>/dev/null || true)
rm -f "$PROBE_TMP"
if [[ "$PROBE_CODE" == "404" && "$PROBE_BODY" == *"version_not_found"* ]]; then
  ok "platform operator API (KV routes)"
elif [[ "$PROBE_CODE" == "404" && "$PROBE_BODY" == *"page not found"* ]] || [[ "$PROBE_CODE" == "000" ]]; then
  bad "platform operator API missing — run: ./dev/scripts/build-cellpd.sh && ./dev/scripts/up.sh"
else
  ok "platform operator API (KV routes, http=${PROBE_CODE})"
fi

exit $FAIL
