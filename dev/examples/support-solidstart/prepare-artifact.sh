#!/usr/bin/env bash
set -euo pipefail
export CELP_C3_FRAMEWORK=solid
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
exec bash "${ROOT}/dev/examples/support-c3-framework/prepare-artifact.sh" "$1"
