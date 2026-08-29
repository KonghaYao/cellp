#!/usr/bin/env bash
# Run TP2 stress suite. Use -short to skip 24h soak.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPTS="${ROOT}/stress/scripts"

SHORT=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    -short|--short) SHORT=1; shift ;;
    -h|--help)
      echo "Usage: $0 [-short]"
      echo "  -short  Skip TP2-S1 24h soak (use soak-24h.sh -short instead)"
      exit 0
      ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

run() {
  local name="$1"
  shift
  echo ""
  echo "========== ${name} =========="
  if "$@"; then
    echo ">>> ${name} OK"
  else
    echo ">>> ${name} FAILED" >&2
    return 1
  fi
}

FAIL=0

"${SCRIPTS}/collect-metrics.sh" "TP2-RUN-start" || FAIL=1

run "TP2-L1 sequential-cd" "${SCRIPTS}/sequential-cd.sh" || FAIL=1
run "TP2-L2/L3 concurrent-cd" "${SCRIPTS}/concurrent-cd.sh" || FAIL=1
run "TP2-L4/L5 gateway-load" "${SCRIPTS}/gateway-load.sh" || FAIL=1

if (( SHORT == 1 )); then
  echo ""
  echo "========== TP2-S1 soak-24h SKIPPED (-short) =========="
  run "TP2-S1 soak (CI short)" "${SCRIPTS}/soak-24h.sh" -short || FAIL=1
else
  run "TP2-S1 soak-24h" "${SCRIPTS}/soak-24h.sh" || FAIL=1
fi

run "TP2-S2 version-limit" "${SCRIPTS}/version-limit.sh" || FAIL=1
run "TP2-S3 ttl-gc" "${SCRIPTS}/ttl-gc.sh" || FAIL=1

# Chaos — after L baseline (phase-5-stress.md)
run "TP2-C1 chaos-celld-kill" "${SCRIPTS}/chaos-celld-kill.sh" || FAIL=1
run "TP2-C2 chaos-rustfs-pause" "${SCRIPTS}/chaos-rustfs-pause.sh" || FAIL=1
run "TP2-C3 chaos-cellpd-restart" "${SCRIPTS}/chaos-cellpd-restart.sh" || FAIL=1
run "TP2-C4 chaos-offshoot-fail" "${SCRIPTS}/chaos-offshoot-fail.sh" || FAIL=1
run "TP2-C5 chaos-sqlite-contention" "${SCRIPTS}/chaos-sqlite-contention.sh" || FAIL=1

run "TP2-D1 data-counter-load" "${SCRIPTS}/data-counter-load.sh" || FAIL=1

"${SCRIPTS}/collect-metrics.sh" "TP2-RUN-end" || FAIL=1

echo ""
if (( FAIL == 0 )); then
  echo "ALL TP2 stress tests PASSED"
  exit 0
else
  echo "TP2 stress suite FAILED — see logs above" >&2
  exit 1
fi
