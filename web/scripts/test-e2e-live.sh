#!/usr/bin/env bash
# TP-UI-14 — Dashboard Playwright against local cellpd (:8790).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT/web"

if [[ -f "$ROOT/dev/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT/dev/.env"
  set +a
fi

export VITE_CELLP_API_URL="${PLATFORM_URL:-http://127.0.0.1:8790}"
export VITE_CELLP_ADMIN_TOKEN="${CELLP_ADMIN_TOKEN:-${PLATFORM_TOKEN:-dev-local-token}}"
export CELLP_LIVE_PROJECT="${CELLP_LIVE_PROJECT:-${DEV_PROJECT:-commerce-store}}"

TOKEN="$VITE_CELLP_ADMIN_TOKEN"
HEALTH_URL="${VITE_CELLP_API_URL%/}/v1/health"

if ! curl -sf "$HEALTH_URL" -H "Authorization: Bearer $TOKEN" >/dev/null; then
  echo "cellpd not healthy at $HEALTH_URL" >&2
  echo "Start the stack: $ROOT/dev/scripts/up.sh && $ROOT/dev/scripts/health.sh" >&2
  exit 1
fi

echo "Live E2E → API $VITE_CELLP_API_URL project=$CELLP_LIVE_PROJECT"
npx playwright test -c playwright.live.config.ts "$@"
