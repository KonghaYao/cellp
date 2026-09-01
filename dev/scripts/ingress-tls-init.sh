#!/usr/bin/env bash
# Generate dev Gateway TLS cert (mkcert preferred). Does not restart cellpd.
# Usage: ./dev/scripts/ingress-tls-init.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
# shellcheck disable=SC1091
source dev/.env 2>/dev/null || true
# shellcheck disable=SC1091
source "${ROOT}/dev/scripts/lib-lan-ip.sh" 2>/dev/null || true

CERT_DIR="${ROOT}/dev/data/certs"
mkdir -p "$CERT_DIR"
CERT="${CERT_DIR}/gateway.pem"
KEY="${CERT_DIR}/gateway-key.pem"
BASE="${CELLP_INGRESS_BASE_DOMAIN:-ingress.local}"
LAN="$(lan_ip 2>/dev/null || echo 127.0.0.1)"

names=(localhost 127.0.0.1 "$LAN" "*.${BASE}")
for proj in support-r2filebox demo-app commerce-store support-relay support-pastebin; do
  names+=("${proj}.${BASE}")
  for ver in v1 v2 v3 v4 v5 v6; do
    names+=("${ver}.${proj}.${BASE}")
  done
done
if [[ -n "${CELLP_TLS_EXTRA_SAN:-}" ]]; then
  IFS=',' read -r -a extra <<<"${CELLP_TLS_EXTRA_SAN}"
  for e in "${extra[@]}"; do
    e="${e// /}"
    [[ -n "$e" ]] && names+=("$e")
  done
fi

if command -v mkcert >/dev/null 2>&1; then
  echo "==> mkcert (${#names[@]} SANs, base=${BASE})"
  mkcert -install >/dev/null 2>&1 || true
  mkcert -cert-file "$CERT" -key-file "$KEY" "${names[@]}"
else
  echo "==> openssl self-signed (install mkcert for trusted local HTTPS)"
  SAN=""
  for n in "${names[@]}"; do
    if [[ "$n" == *.* && "$n" != *"*"* ]]; then
      SAN="${SAN}DNS:${n},"
    elif [[ "$n" == *"*"* ]]; then
      SAN="${SAN}DNS:${n},"
    fi
  done
  SAN="${SAN}IP:127.0.0.1,IP:${LAN}"
  openssl req -x509 -newkey rsa:2048 -nodes \
    -keyout "$KEY" -out "$CERT" -days 825 \
    -subj "/CN=cellp-dev-gateway" \
    -addext "subjectAltName=${SAN}"
fi

chmod 600 "$KEY" 2>/dev/null || true
echo "Wrote ${CERT}"
echo "Next: ./dev/scripts/ingress-tls-enable.sh && ./dev/scripts/build-cellpd.sh && ./dev/scripts/up.sh"
