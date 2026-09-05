#!/usr/bin/env bash
# Contributor gate: Vitest user-loop flows + optional Playwright live (needs cellpd).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
LOG="$ROOT/docs/evidence/user-loop-vitest-$(date +%Y%m%d).log"
mkdir -p "$ROOT/docs/evidence"

echo "=== user-loop verify $(date -Iseconds) ===" | tee "$LOG"
echo "Vitest (mock API flows under web/src/flows/)..." | tee -a "$LOG"
(
  cd "$ROOT/web"
  pnpm run test
) 2>&1 | tee -a "$LOG"

echo "" | tee -a "$LOG"
echo "Optional Playwright live E2E (real cellpd :8790):" | tee -a "$LOG"
echo "  1. Start stack: $ROOT/dev/scripts/up.sh && $ROOT/dev/scripts/health.sh" | tee -a "$LOG"
echo "  2. Run: cd web && pnpm run test:e2e:live" | tee -a "$LOG"
echo "" | tee -a "$LOG"

set +e
(
  cd "$ROOT/web"
  pnpm run test:e2e:live
) 2>&1 | tee -a "$LOG"
LIVE_EXIT=$?
set -e

if [[ $LIVE_EXIT -ne 0 ]]; then
  echo "Live E2E did not pass (exit $LIVE_EXIT). Skipped for gate — start ./dev/scripts/up.sh and re-run test:e2e:live." | tee -a "$LOG"
fi

echo "Vitest log: $LOG" | tee -a "$LOG"
