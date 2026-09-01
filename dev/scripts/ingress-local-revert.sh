#!/usr/bin/env bash
# Revert to default ingress.local (requires /etc/hosts on each client for LAN).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

[[ -f dev/.env ]] || cp dev/.env.example dev/.env

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

set_env_kv dev/.env CELLP_INGRESS_BASE_DOMAIN "ingress.local"
set_env_kv dev/.env GATEWAY_URL "http://127.0.0.1:8787"

if [[ -f web/.env ]]; then
  set_env_kv web/.env VITE_CELLP_INGRESS_BASE_DOMAIN "ingress.local"
  set_env_kv web/.env VITE_CELLP_GATEWAY_URL "http://127.0.0.1:8787"
fi

cat <<'EOF'
Reverted to ingress.local
=========================
Add to /etc/hosts on each machine (127.0.0.1 for same host, or your LAN IP for colleagues):
  127.0.0.1 demo-app.ingress.local v1.demo-app.ingress.local

Then: ./dev/scripts/reset.sh && ./dev/scripts/up.sh && ./dev/scripts/seed-demo.sh
EOF
