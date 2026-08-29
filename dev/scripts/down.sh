#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

set -a
# shellcheck disable=SC1091
source dev/.env 2>/dev/null || source dev/.env.example
set +a

echo "==> stop host processes"
for f in platform celld offshoot; do
  if [[ -f "dev/data/pids/${f}.pid" ]]; then
    kill "$(cat "dev/data/pids/${f}.pid")" 2>/dev/null || true
    rm -f "dev/data/pids/${f}.pid"
  fi
done

echo "==> docker compose down"
docker compose -f dev/docker-compose.yml --env-file dev/.env down 2>/dev/null || true

echo "Dev stack stopped."
