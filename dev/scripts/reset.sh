#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

./dev/scripts/down.sh
rm -rf dev/data/artifacts dev/data/offshoot-store dev/data/offshoot-checkouts dev/data/celld-watch dev/data/logs dev/data/pids dev/data/registry.json dev/data/platform-registry.json
rm -f dev/data/cellp-registry.sqlite dev/data/cellp-registry.sqlite-wal dev/data/cellp-registry.sqlite-shm
mkdir -p dev/data/{artifacts,offshoot-store,offshoot-checkouts,celld-watch,pids,logs}
echo "dev/data reset (SQLite registry + artifacts + offshoot + celld-watch). AD-12: ingress_bindings recreated on next deploy."
echo "Next: ./dev/scripts/build-cellpd.sh && ./dev/scripts/up.sh && ./dev/scripts/seed-demo.sh"
