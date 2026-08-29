#!/usr/bin/env bash
# TP-VE-ALL — Run all e2e acceptance scripts in MANIFEST order
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# shellcheck disable=SC1091
source "${SCRIPT_DIR}/lib.sh"

MANIFEST="${SCRIPT_DIR}/MANIFEST"
GATE_SCRIPTS=(v0a-celld-diagnose.sh v0d-offshoot-attach.sh v0b-offshoot-rustfs.sh)

run_script() {
  local name="$1"
  local path="${SCRIPT_DIR}/${name}"
  if [[ ! -f "$path" ]]; then
    echo "FAIL: missing script ${path}" >&2
    exit 1
  fi
  chmod +x "$path"
  log "run ${name}"
  if ! "$path"; then
    echo "FAIL: ${name} exit non-zero" >&2
    exit 1
  fi
}

log "run-all.sh start RUN_GATES=${RUN_GATES:-0}"

require_platform
cleanup_e2e_versions "$DEV_PROJECT"

if [[ "${RUN_GATES:-0}" == "1" ]]; then
  log "Phase 0 storage gates"
  for s in "${GATE_SCRIPTS[@]}"; do
    run_script "$s"
  done

  # V0c optional skip
  if [[ ! -f "${EVIDENCE_DIR}/v0c-skip.md" ]]; then
    echo "WARN: ${EVIDENCE_DIR}/v0c-skip.md missing (V0c not documented)" >&2
  fi
fi

while IFS= read -r line || [[ -n "$line" ]]; do
  line="${line%%#*}"
  line="${line#"${line%%[![:space:]]*}"}"
  line="${line%"${line##*[![:space:]]}"}"
  [[ -z "$line" ]] && continue
  run_script "$line"
done < "$MANIFEST"

pass "run-all.sh complete"
exit 0
