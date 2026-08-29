#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

./dev/scripts/down.sh
rm -rf dev/data/artifacts dev/data/offshoot-store dev/data/offshoot-checkouts dev/data/celld-watch dev/data/logs dev/data/pids dev/data/registry.json dev/data/platform-registry.json
mkdir -p dev/data/{artifacts,offshoot-store,offshoot-checkouts,celld-watch,pids,logs}
echo "dev/data reset. Run ./dev/scripts/up.sh"
