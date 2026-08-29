#!/usr/bin/env bash
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
echo "=== platform ==="
tail -n 20 "$ROOT/dev/data/logs/platform.log" 2>/dev/null || echo "(no log)"
echo "=== celld ==="
tail -n 30 "$ROOT/dev/data/logs/celld.log" 2>/dev/null || echo "(no log)"
echo "=== celld deploy ==="
tail -n 20 "$ROOT/dev/data/logs/celld-deploy.log" 2>/dev/null || echo "(no log)"
