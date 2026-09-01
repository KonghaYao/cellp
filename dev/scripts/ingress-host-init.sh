#!/usr/bin/env bash
# Unify AD-12 dev host mode: sync dev/.env + web/.env (local vs magic DNS).
# Usage: ./dev/scripts/ingress-host-init.sh [local|magic] [LAN_IP]
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

MODE="${1:-local}"
LAN_IP="${2:-}"

[[ -f dev/.env ]] || cp dev/.env.example dev/.env

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

ensure_public_schemes() {
  set_env_kv dev/.env CELLP_PUBLIC_SCHEME_PREVIEW http
  set_env_kv dev/.env CELLP_PUBLIC_SCHEME_PROD http
}

sync_web_ingress_domain() {
  local base="$1"
  touch web/.env 2>/dev/null || true
  [[ -f web/.env ]] || cp web/.env.example web/.env 2>/dev/null || touch web/.env
  set_env_kv web/.env VITE_CELLP_INGRESS_BASE_DOMAIN "$base"
}

case "$MODE" in
  local | ingress.local)
    set_env_kv dev/.env CELLP_INGRESS_BASE_DOMAIN ingress.local
    set_env_kv dev/.env GATEWAY_URL "http://127.0.0.1:8787"
    ensure_public_schemes
    sync_web_ingress_domain ingress.local
    cat <<EOF

cellp ingress: mode **local** (ingress.local)
============================================
Add to /etc/hosts (127.0.0.1 or your LAN IP for colleagues):

  127.0.0.1 demo-app.ingress.local v1.demo-app.ingress.local

Browser: http://demo-app.ingress.local:8787/

Doc: dev/INGRESS-HOST.md

Then: ./dev/scripts/reset.sh && ./dev/scripts/up.sh && ./dev/scripts/seed-demo.sh

EOF
    ;;
  magic | nip | sslip)
    args=()
    [[ "$MODE" == nip ]] && args=(--nip)
    [[ "$MODE" == sslip ]] && args=(--sslip)
    exec "$ROOT/dev/scripts/ingress-magic-dns-enable.sh" "${args[@]}" ${LAN_IP:+"$LAN_IP"}
    ;;
  *)
    echo "Usage: $0 [local|magic|nip|sslip] [LAN_IP]" >&2
    exit 1
    ;;
esac
