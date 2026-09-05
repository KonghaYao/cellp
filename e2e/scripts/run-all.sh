#!/usr/bin/env bash
# TP-VE-ALL — Run all e2e acceptance scripts in MANIFEST order
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MANIFEST="${SCRIPT_DIR}/MANIFEST"
GATE_SCRIPTS=(v0a-celld-diagnose.sh v0d-offshoot-attach.sh v0b-offshoot-rustfs.sh)
MANIFEST_SCRIPTS=()
ONLY_SCRIPTS=()
ONLY_COUNT=0
SKIP_CLEANUP="${E2E_SKIP_CLEANUP:-0}"

usage() {
  cat <<'EOF'
Usage: ./e2e/scripts/run-all.sh [options]

Options:
  --only NAME[,NAME...]  Run selected scripts in gate/MANIFEST order.
  --skip-cleanup         Keep existing v-e2e-* versions, including cleanup
                         requested by test scripts.
  --list                 List selectable script names and exit.
  -h, --help             Show this help.

Environment equivalents:
  E2E_ONLY=NAME[,NAME...]  E2E_SKIP_CLEANUP=1  RUN_GATES=1

Without --only/E2E_ONLY the complete MANIFEST still runs. RUN_GATES=1 also
runs the required storage gates, including celld diagnose.
EOF
}

while IFS= read -r line || [[ -n "$line" ]]; do
  line="${line%%#*}"
  line="${line#"${line%%[![:space:]]*}"}"
  line="${line%"${line##*[![:space:]]}"}"
  [[ -z "$line" ]] && continue
  MANIFEST_SCRIPTS+=("$line")
done < "$MANIFEST"

add_only_scripts() {
  local raw="$1" name found=0
  raw="${raw//,/ }"
  for name in $raw; do
    found=1
    [[ "$name" == *.sh ]] || name="${name}.sh"
    ONLY_SCRIPTS+=("$name")
    ONLY_COUNT=$((ONLY_COUNT + 1))
  done
  if [[ "$found" -eq 0 ]]; then
    echo "--only requires at least one script name" >&2
    exit 2
  fi
}

[[ -z "${E2E_ONLY:-}" ]] || add_only_scripts "$E2E_ONLY"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --only)
      [[ $# -ge 2 ]] || { echo "--only requires a script name" >&2; exit 2; }
      add_only_scripts "$2"
      shift
      ;;
    --only=*) add_only_scripts "${1#*=}" ;;
    --skip-cleanup) SKIP_CLEANUP=1 ;;
    --list)
      printf '%s\n' "${GATE_SCRIPTS[@]}" "${MANIFEST_SCRIPTS[@]}"
      exit 0
      ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

is_known_script() {
  local wanted="$1" name
  for name in "${GATE_SCRIPTS[@]}" "${MANIFEST_SCRIPTS[@]}"; do
    [[ "$name" == "$wanted" ]] && return 0
  done
  return 1
}

is_selected() {
  local wanted="$1" name
  [[ "$ONLY_COUNT" -eq 0 ]] && return 0
  for name in "${ONLY_SCRIPTS[@]}"; do
    [[ "$name" == "$wanted" ]] && return 0
  done
  return 1
}

if [[ "$ONLY_COUNT" -gt 0 ]]; then
  for name in "${ONLY_SCRIPTS[@]}"; do
    if ! is_known_script "$name"; then
      echo "unknown e2e script: ${name}" >&2
      echo "run ./e2e/scripts/run-all.sh --list to see valid names" >&2
      exit 2
    fi
  done
fi

# shellcheck disable=SC1091
source "${SCRIPT_DIR}/lib.sh"

if [[ "$SKIP_CLEANUP" == "1" ]]; then
  export E2E_SKIP_CLEANUP=1
fi

run_script() {
  local name="$1"
  local path="${SCRIPT_DIR}/${name}"
  local started=$SECONDS
  if [[ ! -f "$path" ]]; then
    echo "FAIL: missing script ${path}" >&2
    exit 1
  fi
  chmod +x "$path"
  log "run ${name}"
  if ! "$path"; then
    echo "FAIL: ${name} exit non-zero after $((SECONDS - started))s" >&2
    exit 1
  fi
  log "done ${name} ($((SECONDS - started))s)"
}

SUITE_STARTED=$SECONDS

if [[ "$ONLY_COUNT" -eq 0 ]]; then
  log "run-all.sh start RUN_GATES=${RUN_GATES:-0}"
  if [[ "${RUN_GATES:-0}" != "1" ]]; then
    log "skip Phase 0 storage gates (set RUN_GATES=1 to include celld diagnose)"
  fi
else
  log "targeted e2e start scripts=${ONLY_SCRIPTS[*]} (not TP-VE-ALL)"
fi

RUN_MANIFEST=0
for name in "${MANIFEST_SCRIPTS[@]}"; do
  if is_selected "$name"; then
    RUN_MANIFEST=1
    break
  fi
done

if [[ "$RUN_MANIFEST" -eq 1 ]]; then
  require_platform
  if [[ "$ONLY_COUNT" -eq 0 ]]; then
    if [[ "$SKIP_CLEANUP" == "1" ]]; then
      log "skip pre-run e2e version cleanup"
    else
      cleanup_e2e_versions "$DEV_PROJECT"
    fi
  fi
fi

if [[ "${RUN_GATES:-0}" == "1" ]]; then
  log "Phase 0 storage gates"
  for name in "${GATE_SCRIPTS[@]}"; do
    run_script "$name"
  done

  # V0c optional skip
  if [[ ! -f "${EVIDENCE_DIR}/v0c-skip.md" ]]; then
    echo "WARN: ${EVIDENCE_DIR}/v0c-skip.md missing (V0c not documented)" >&2
  fi
elif [[ "$ONLY_COUNT" -gt 0 ]]; then
  for name in "${GATE_SCRIPTS[@]}"; do
    is_selected "$name" && run_script "$name"
  done
fi

for name in "${MANIFEST_SCRIPTS[@]}"; do
  is_selected "$name" && run_script "$name"
done

if [[ "$ONLY_COUNT" -eq 0 ]]; then
  pass "run-all.sh complete ($((SECONDS - SUITE_STARTED))s)"
else
  pass "targeted e2e complete ($((SECONDS - SUITE_STARTED))s)"
fi
exit 0
