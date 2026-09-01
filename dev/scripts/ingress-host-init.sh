#!/usr/bin/env bash
# AD-12 dev ingress: lvh.me (loopback). Sync dev/.env + web/.env.
# Usage: ./dev/scripts/ingress-host-init.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

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

set_env_kv dev/.env CELLP_INGRESS_BASE_DOMAIN lvh.me
set_env_kv dev/.env GATEWAY_URL "http://127.0.0.1:8787"
set_env_kv dev/.env CELLP_PUBLIC_SCHEME_PREVIEW http
set_env_kv dev/.env CELLP_PUBLIC_SCHEME_PROD http
touch web/.env 2>/dev/null || true
[[ -f web/.env ]] || cp web/.env.example web/.env 2>/dev/null || touch web/.env
set_env_kv web/.env VITE_CELLP_INGRESS_BASE_DOMAIN lvh.me

cat <<EOF

cellp ingress: lvh.me → 127.0.0.1 (stable when LAN IP changes)
==============================================================
  http://demo-app.lvh.me:8787/

  ./dev/scripts/restart-cellpd.sh
  ./dev/scripts/ingress-repromote-support.sh

Doc: dev/INGRESS-HOST.md

EOF
