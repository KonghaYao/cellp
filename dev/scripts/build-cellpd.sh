#!/usr/bin/env bash
# Build cellpd binary into dev/data/
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT/cellp"
GOTOOLCHAIN=local go mod tidy
GOTOOLCHAIN=local go build -o "$ROOT/dev/data/cellpd" ./cmd/cellpd
echo "Built $ROOT/dev/data/cellpd"
