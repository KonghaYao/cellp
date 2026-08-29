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

if curl -sf "${PLATFORM_URL}/v1/health" >/dev/null; then ok "platform ${PLATFORM_URL}"; else bad "platform ${PLATFORM_URL}"; fi

if curl -sf "http://127.0.0.1:${CELLD_PORT}/__celld/health" >/dev/null 2>&1; then
  ok "celld :${CELLD_PORT}"
else
  bad "celld :${CELLD_PORT} (install celld or run up.sh)"
fi

if command -v docker >/dev/null 2>&1 && docker compose -f dev/docker-compose.yml ps valkey 2>/dev/null | grep -q running; then
  ok "valkey :6379"
elif curl -sf "${VALKEY_URL/redis:\/\/127.0.0.1:6379}" >/dev/null 2>&1; then
  ok "valkey :6379"
else
  echo "WARN valkey not running (optional for M2 e2e)"
fi

if curl -sf "http://127.0.0.1:${S3_PORT:-9000}/health" >/dev/null 2>&1; then
  ok "rustfs :${S3_PORT:-9000}"
else
  bad "rustfs :${S3_PORT:-9000}"
fi

if [[ -f dev/data/registry.json ]] || [[ -f dev/data/platform-registry.sqlite ]] || [[ -f dev/data/cellp-registry.sqlite ]] || [[ -f "${REGISTRY_DB:-dev/data/cellp-registry.sqlite}" ]]; then
  ok "registry file"
else
  echo "WARN registry empty (run simulate-cd first)"
fi

exit $FAIL
