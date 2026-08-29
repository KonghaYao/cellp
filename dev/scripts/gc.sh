#!/usr/bin/env bash
# One-shot registry GC (jobs + destroyed versions). Background GC runs in cellpd.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT/cellp"
export CELLP_REGISTRY_DB="${CELLP_REGISTRY_DB:-$ROOT/dev/data/cellp-registry.sqlite}"
exec go run ./cmd/gc-once/
