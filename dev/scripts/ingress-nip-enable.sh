#!/usr/bin/env bash
# Enable AD-12 LAN preview via nip.io (no per-client /etc/hosts).
# Usage: ./dev/scripts/ingress-nip-enable.sh [LAN_IP]
# Then: ./dev/scripts/reset.sh && ./dev/scripts/up.sh && ./dev/scripts/seed-demo.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

# shellcheck disable=SC1091
source "$(dirname "$0")/lib-lan-ip.sh"

if [[ ! -f dev/.env ]]; then
  cp dev/.env.example dev/.env
  echo "Created dev/.env"
fi

LAN_IP="${1:-}"
if [[ -z "$LAN_IP" ]]; then
  LAN_IP="$(detect_lan_ip)" || {
    echo "Could not detect LAN IP. Pass it explicitly:" >&2
    echo "  $0 192.168.1.10" >&2
    exit 1
  }
fi

NIP_DOMAIN="$(ip_to_nip_domain "$LAN_IP")"

set_env_kv() {
  local file="$1"
  local key="$2"
  local value="$3"
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

set_env_kv dev/.env CELLP_INGRESS_BASE_DOMAIN "$NIP_DOMAIN"
set_env_kv dev/.env GATEWAY_URL "http://${LAN_IP}:8787"

if [[ -f web/.env ]]; then
  set_env_kv web/.env VITE_CELLP_INGRESS_BASE_DOMAIN "$NIP_DOMAIN"
  set_env_kv web/.env VITE_CELLP_GATEWAY_URL "http://${LAN_IP}:8787"
else
  echo "Tip: copy web/.env.example to web/.env to sync Dashboard display, or export:"
  echo "  VITE_CELLP_INGRESS_BASE_DOMAIN=${NIP_DOMAIN}"
  echo "  VITE_CELLP_GATEWAY_URL=http://${LAN_IP}:8787"
fi

cat <<EOF

nip.io ingress enabled
======================
LAN IP:              ${LAN_IP}
CELLP_INGRESS_BASE_DOMAIN=${NIP_DOMAIN}

Example prod Host:   demo-app.${NIP_DOMAIN}
Example preview:     v1.demo-app.${NIP_DOMAIN}
Browser URL:         http://v1.demo-app.${NIP_DOMAIN}:8787/

DNS: nip.io resolves *.${NIP_DOMAIN} → ${LAN_IP} (needs outbound DNS; no /etc/hosts on colleagues' machines).

Required next steps (rebind ingress hostnames in Registry):
  ./dev/scripts/reset.sh
  ./dev/scripts/up.sh
  ./dev/scripts/seed-demo.sh    # or seed-commerce-store.sh

Restart Dashboard dev server if running (Vite reads web/.env).

Verify from another machine on LAN:
  curl -sS -H "Host: demo-app.${NIP_DOMAIN}" "http://${LAN_IP}:8787/health"

EOF
