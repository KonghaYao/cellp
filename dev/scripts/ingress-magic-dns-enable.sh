#!/usr/bin/env bash
# LAN ingress via public magic DNS (nip.io or sslip.io) — no per-client /etc/hosts.
# Usage:
#   ./dev/scripts/ingress-magic-dns-enable.sh [--nip|--sslip] [LAN_IP]
#   MAGIC_DNS=sslip ./dev/scripts/ingress-magic-dns-enable.sh
# Then: ./dev/scripts/reset.sh && ./dev/scripts/up.sh && ./dev/scripts/seed-demo.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

# shellcheck disable=SC1091
source "$(dirname "$0")/lib-lan-ip.sh"

PROVIDER="${MAGIC_DNS:-auto}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --nip | --nip.io)
      PROVIDER=nip
      shift
      ;;
    --sslip | --sslip.io)
      PROVIDER=sslip
      shift
      ;;
    -h | --help)
      echo "Usage: $0 [--nip|--sslip] [LAN_IP]"
      exit 0
      ;;
    -*)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
    *)
      break
      ;;
  esac
done

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

if [[ "$PROVIDER" == "auto" ]]; then
  if probe_magic_dns "$LAN_IP" sslip; then
    PROVIDER=sslip
  elif probe_magic_dns "$LAN_IP" nip; then
    PROVIDER=nip
  else
    echo "WARN: could not verify magic DNS; defaulting to sslip.io" >&2
    PROVIDER=sslip
  fi
fi

BASE_DOMAIN="$(magic_dns_domain "$LAN_IP" "$PROVIDER")"

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

set_env_kv dev/.env CELLP_INGRESS_BASE_DOMAIN "$BASE_DOMAIN"
set_env_kv dev/.env GATEWAY_URL "http://${LAN_IP}:8787"

if [[ -f web/.env ]]; then
  set_env_kv web/.env VITE_CELLP_INGRESS_BASE_DOMAIN "$BASE_DOMAIN"
  set_env_kv web/.env VITE_CELLP_GATEWAY_URL "http://${LAN_IP}:8787"
fi

cat <<EOF

Magic DNS ingress enabled (${PROVIDER})
=====================================
LAN IP:                    ${LAN_IP}
CELLP_INGRESS_BASE_DOMAIN=${BASE_DOMAIN}

Example prod Host:         demo-app.${BASE_DOMAIN}
Example preview:           v1.demo-app.${BASE_DOMAIN}

Open in browser (required):
  http://demo-app.${BASE_DOMAIN}:8787/
  http://v1.demo-app.${BASE_DOMAIN}:8787/

Common mistakes:
  - Omitting :8787 (browser uses port 80 → connection failed)
  - Using https:// (Gateway dev is http only unless you add TLS)

Next (rebind Registry hostnames):
  ./dev/scripts/reset.sh
  ./dev/scripts/up.sh
  ./dev/scripts/seed-demo.sh

Revert to ingress.local + /etc/hosts:
  ./dev/scripts/ingress-local-revert.sh

Verify:
  curl -sS "http://demo-app.${BASE_DOMAIN}:8787/" | head
  curl -sS -H "Host: demo-app.${BASE_DOMAIN}" "http://${LAN_IP}:8787/health"

EOF
