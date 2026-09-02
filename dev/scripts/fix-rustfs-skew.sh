#!/usr/bin/env bash
# Recover from RustFS RequestTimeTooSkewed / celld node-lease fence (dev only).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
# shellcheck disable=SC1091
source dev/.env 2>/dev/null || true

echo "==> restart RustFS (sync container clock with host)"
if command -v docker >/dev/null 2>&1; then
  docker compose -f dev/docker-compose.yml --env-file dev/.env restart rustfs 2>/dev/null || true
  sleep 2
fi

bash dev/scripts/check-s3-clock-skew.sh || {
  echo "Still skewed — enable macOS 'Set time and date automatically', then re-run this script."
  exit 1
}

echo "==> restart cellpd (clears stale gateway upstream)"
bash dev/scripts/restart-cellpd.sh
echo "OK: re-deploy affected versions or: curl -X POST .../versions/{id}/wake"
