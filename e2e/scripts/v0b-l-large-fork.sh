#!/usr/bin/env bash
# TP-V0b-L — ≥100MB offshoot fork + export (local store)
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
export OB_SIZE_MB="${OB_SIZE_MB:-100}"
export OB_SUITE=v0bl
exec "${ROOT}/stress/phase6/offshoot-branch-scale.sh"
