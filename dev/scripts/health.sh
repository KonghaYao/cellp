#!/usr/bin/env bash
# Runtime health check — exit 0 if all selected checks pass
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

QUICK="${CELLP_HEALTH_QUICK:-0}"
usage() {
  cat <<'EOF'
Usage: ./dev/scripts/health.sh [--quick]

  --quick  Check only the cellpd API and Gateway readiness endpoints.
           The default remains the full runtime health check.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --quick) QUICK=1 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

set -a
# shellcheck disable=SC1091
source dev/.env 2>/dev/null || source dev/.env.example
set +a

FAIL=0
ok() { echo "OK  $1"; }
bad() { echo "FAIL $1"; FAIL=1; }

if curl -sf "${GATEWAY_URL}/health" >/dev/null; then ok "gateway ${GATEWAY_URL}"; else bad "gateway ${GATEWAY_URL}"; fi

if curl -sf "${PLATFORM_URL}/v1/health" >/dev/null; then ok "platform ${PLATFORM_URL}"; else bad "platform ${PLATFORM_URL}"; fi

if [[ "$QUICK" == "1" ]]; then
  exit $FAIL
fi

GW_DEEP_CODE=$(curl -sS -o /dev/null -w '%{http_code}' "${GATEWAY_URL}/health/deep" 2>/dev/null || echo "000")
if [[ "$GW_DEEP_CODE" == "200" || "$GW_DEEP_CODE" == "503" ]]; then
  ok "gateway deep health (http=${GW_DEEP_CODE})"
else
  bad "gateway deep health (http=${GW_DEEP_CODE})"
fi

ADMIN="${CELLP_ADMIN_TOKEN:-${PLATFORM_TOKEN:-dev-local-token}}"

# Elastic serving uses dynamic per-version celld ports — probe runtime routes, not fixed :8792.
ROUTES_HEALTH_TMP=$(mktemp)
ROUTES_HEALTH_CODE=$(curl -sS -o "$ROUTES_HEALTH_TMP" -w '%{http_code}' -H "Authorization: Bearer ${ADMIN}" "${PLATFORM_URL}/v1/runtime/routes" 2>/dev/null || echo "000")
if [[ "$ROUTES_HEALTH_CODE" == "200" ]]; then
  UPSTREAM_COUNT=$(jq -r '[.routes[]? | select(.celld_health == "ok")] | length' "$ROUTES_HEALTH_TMP" 2>/dev/null || echo "0")
  if [[ "${UPSTREAM_COUNT:-0}" -gt 0 ]]; then
    ok "runtime celld upstreams (${UPSTREAM_COUNT} healthy)"
  else
    TOTAL=$(jq -r '.routes | length' "$ROUTES_HEALTH_TMP" 2>/dev/null || echo "0")
    if [[ "${TOTAL:-0}" -eq 0 ]]; then
      if curl -sf "http://127.0.0.1:${CELLD_PORT}/.well-known/celld/health" >/dev/null 2>&1; then
        ok "celld :${CELLD_PORT} (shared dev probe)"
      else
        echo "WARN no active runtime routes and celld :${CELLD_PORT} down (deploy a version or run up.sh)"
      fi
    else
      bad "runtime celld upstreams (${TOTAL} routes, none healthy)"
    fi
  fi
else
  if curl -sf "http://127.0.0.1:${CELLD_PORT}/.well-known/celld/health" >/dev/null 2>&1; then
    ok "celld :${CELLD_PORT} (fallback; runtime routes HTTP ${ROUTES_HEALTH_CODE})"
  else
    bad "runtime routes (http=${ROUTES_HEALTH_CODE}) and celld :${CELLD_PORT}"
  fi
fi
rm -f "$ROUTES_HEALTH_TMP"

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
