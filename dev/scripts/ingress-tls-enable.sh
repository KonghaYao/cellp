#!/usr/bin/env bash
# Point dev/.env at Gateway HTTPS listener (8788). Run ingress-tls-init.sh first.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
[[ -f dev/.env ]] || cp dev/.env.example dev/.env
# shellcheck disable=SC1091
source dev/.env 2>/dev/null || true
# shellcheck disable=SC1091
source "${ROOT}/dev/scripts/lib-lan-ip.sh" 2>/dev/null || true

TLS_PORT="${GATEWAY_TLS_PORT:-8788}"
LAN="$(lan_ip 2>/dev/null || echo 127.0.0.1)"
CERT="${GATEWAY_TLS_CERT:-./dev/data/certs/gateway.pem}"
KEY="${GATEWAY_TLS_KEY:-./dev/data/certs/gateway-key.pem}"

set_env_kv() {
  local file="$1" key="$2" value="$3"
  if grep -q "^${key}=" "$file" 2>/dev/null; then
    if [[ "$(uname)" == Darwin ]]; then
      sed -i '' "s|^${key}=.*|${key}=${value}|" "$file"
    else
      sed -i "s|^${key}=.*|${key}=${value}|" "$file"
    fi
  else
    echo "${key}=${value}" >>"$file"
  fi
}

set_env_kv dev/.env GATEWAY_TLS_PORT "$TLS_PORT"
set_env_kv dev/.env GATEWAY_TLS_CERT "$CERT"
set_env_kv dev/.env GATEWAY_TLS_KEY "$KEY"
set_env_kv dev/.env GATEWAY_URL "https://${LAN}:${TLS_PORT}"
set_env_kv dev/.env CELLP_PUBLIC_SCHEME_PREVIEW https
set_env_kv dev/.env CELLP_PUBLIC_SCHEME_PROD https
set_env_kv web/.env VITE_CELLP_GATEWAY_URL "https://${LAN}:${TLS_PORT}" 2>/dev/null || true

cat <<EOF

cellp dev Gateway HTTPS enabled
===============================
  HTTP (legacy):  http://${LAN}:8787/
  HTTPS (browser): https://${LAN}:${TLS_PORT}/

Use API preview_url with https + Host, e.g.:
  https://v3.support-r2filebox.${CELLP_INGRESS_BASE_DOMAIN:-ingress.local}:${TLS_PORT}/

Restart: ./dev/scripts/build-cellpd.sh && ./dev/scripts/up.sh
Redeploy/promote so preview_url picks up https (or re-seed).

EOF
