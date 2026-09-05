#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=dev/scripts/support-pnpm.sh
source "${ROOT}/dev/scripts/support-pnpm.sh"
cellp_ensure_pnpm
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"
export CELP_C3_FRAMEWORK=waku
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
exec bash "${ROOT}/dev/examples/support-c3-framework/prepare-artifact.sh" "$1"
