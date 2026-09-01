#!/usr/bin/env bash
# Restart cellpd only (API + Gateway HTTP/TLS). Fast; no compose/celld.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
[[ -f dev/.env ]] || { echo "missing dev/.env"; exit 1; }
# shellcheck disable=SC1091
set -a
source dev/.env
set +a

if [[ -f dev/data/pids/platform.pid ]]; then
  kill "$(cat dev/data/pids/platform.pid)" 2>/dev/null || true
  rm -f dev/data/pids/platform.pid
fi
pkill -f 'dev/data/cellpd' 2>/dev/null || true

[[ -x dev/data/cellpd ]] || { echo "run ./dev/scripts/build-cellpd.sh first"; exit 1; }

export CELLP_REGISTRY_DB="${CELLP_REGISTRY_DB:-${REGISTRY_DB:-./dev/data/cellp-registry.sqlite}}"
export CELLP_DEPLOY_TOKEN="${CELLP_DEPLOY_TOKEN:-${PLATFORM_TOKEN:-dev-local-token}}"
export CELLP_ADMIN_TOKEN="${CELLP_ADMIN_TOKEN:-${PLATFORM_TOKEN:-dev-local-token}}"
export S3_ENDPOINT AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_REGION RUSTFS_ACCESS_KEY RUSTFS_SECRET_KEY OFFSHOOT_STORE
export CELLP_SKIP_CELLD_DIAGNOSE GATEWAY_TLS_PORT GATEWAY_TLS_CERT GATEWAY_TLS_KEY CELLP_PUBLIC_SCHEME_PREVIEW CELLP_PUBLIC_SCHEME_PROD CELLP_INGRESS_BASE_DOMAIN GATEWAY_URL GATEWAY_PORT

nohup ./dev/data/cellpd >>dev/data/logs/cellpd.log 2>&1 &
echo $! > dev/data/pids/platform.pid
echo "cellpd pid $(cat dev/data/pids/platform.pid)"
echo "  HTTP  http://127.0.0.1:${GATEWAY_PORT:-8787}/"
if [[ -n "${GATEWAY_TLS_PORT:-}" && "${GATEWAY_TLS_PORT:-0}" != "0" ]]; then
  echo "  HTTPS https://127.0.0.1:${GATEWAY_TLS_PORT}/"
fi
