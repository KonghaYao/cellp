#!/usr/bin/env bash
# TP-OB RustFS round — same ladder as local, on s3://cellp-offshoot
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
set -a
# shellcheck disable=SC1091
source "${ROOT}/dev/.env"
set +a
export OB_TIER=rustfs
export OB_REPORT="${ROOT}/docs/evidence/offshoot-branch-scale-report-rustfs.md"
exec "${ROOT}/stress/phase6/offshoot-branch-scale.sh"
