#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

set -a
# shellcheck disable=SC1091
source dev/.env 2>/dev/null || source dev/.env.example
set +a

echo "==> stop host processes"
if [[ -f dev/data/pids/platform.pid ]]; then
  pid="$(cat dev/data/pids/platform.pid)"
  if kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null || true
    for _ in $(seq 1 350); do
      kill -0 "$pid" 2>/dev/null || break
      sleep 0.1
    done
    if kill -0 "$pid" 2>/dev/null; then
      echo "WARN: cellpd did not stop gracefully; forcing pid ${pid}" >&2
      kill -9 "$pid" 2>/dev/null || true
    fi
  fi
  rm -f dev/data/pids/platform.pid
fi

# Per-version cellds are spawned by cellpd; killing cellpd alone can leave listeners on 127.0.0.1:8803+.
if command -v pgrep >/dev/null 2>&1; then
  while read -r pid; do
    [[ -z "$pid" ]] && continue
    kill "$pid" 2>/dev/null || true
  done < <(pgrep -f '[/]celld --bucket' 2>/dev/null || true)
  sleep 0.5
  while read -r pid; do
    [[ -z "$pid" ]] && continue
    kill -9 "$pid" 2>/dev/null || true
  done < <(pgrep -f '[/]celld --bucket' 2>/dev/null || true)
fi

for f in celld offshoot; do
  if [[ -f "dev/data/pids/${f}.pid" ]]; then
    kill "$(cat "dev/data/pids/${f}.pid")" 2>/dev/null || true
    rm -f "dev/data/pids/${f}.pid"
  fi
done

echo "==> docker compose down"
docker compose -f dev/docker-compose.yml --env-file dev/.env down 2>/dev/null || true

echo "Dev stack stopped."
