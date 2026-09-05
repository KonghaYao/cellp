#!/usr/bin/env bash
# cellp preview harness for @opennextjs/cloudflare official Playwright e2e (app/pages/app-pages routers).
#
# Usage:
#   ./dev/scripts/run-opennext-official-e2e.sh [--list]
#   ./dev/scripts/run-opennext-official-e2e.sh [--collect-only] [--fast]
#   ./dev/scripts/run-opennext-official-e2e.sh [--only app-router|pages-router|app-pages-router] [--skip-build]
#
# Exit: 0 all pass · 1 test/deploy/build failure · 2 bad args
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

# shellcheck disable=SC1091
source e2e/scripts/lib.sh
# shellcheck disable=SC1091
source dev/scripts/support-pnpm.sh
# shellcheck disable=SC1091
source dev/scripts/prepare-opennext-official-e2e.sh

usage() {
  cat <<'USAGE'
Usage: run-opennext-official-e2e.sh [options]

Options:
  --only FIXTURE     Run one fixture (app-router | pages-router | app-pages-router)
  --skip-build       Reuse existing .open-next in fixture (still runs prepare/deploy)
  --collect-only     Validate Playwright test counts only (119 listed, 5 declared skip, 114 runnable)
  --list             Print fixtures, projects, and expected counts
  --fast             Opt-in: skip workspace pnpm install when node_modules present
  -h, --help         This help

Default: all three fixtures; full install + rebuild .open-next; preview deploy only (never promote).
USAGE
}

SELECTED=()
SKIP_BUILD=0
COLLECT_ONLY=0
LIST_MODE=0
FAST=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --only)
      [[ $# -ge 2 ]] || { echo "FAIL: --only requires a fixture" >&2; exit 2; }
      oncf_valid_fixture "$2" || { echo "FAIL: unknown fixture $2" >&2; exit 2; }
      SELECTED+=("$2")
      shift 2
      ;;
    --skip-build) SKIP_BUILD=1; shift ;;
    --collect-only) COLLECT_ONLY=1; shift ;;
    --list) LIST_MODE=1; shift ;;
    --fast) FAST=1; shift ;;
    -h | --help) usage; exit 0 ;;
    *)
      echo "FAIL: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ ${#SELECTED[@]} -eq 0 ]]; then
  SELECTED=("${ONCF_FIXTURES[@]}")
fi

RUN_ID="$(date +%Y%m%d-%H%M%S)-$$"
SUMMARY_LOG="${EVIDENCE_DIR}/opennext-official-e2e-${RUN_ID}.log"
mkdir -p "$EVIDENCE_DIR"

oncf_ensure_clone

if [[ "$LIST_MODE" == "1" ]]; then
  echo "clone: ${ONCF_CLONE_DIR} @ ${ONCF_COMMIT} (${ONCF_TAG})"
  echo "fixtures: ${SELECTED[*]}"
  for f in "${SELECTED[@]}"; do
    echo "  ${f} → project $(oncf_project_id "$f") dir $(oncf_fixture_dir "$f")"
    echo "    expect listed=$(oncf_expect_listed "$f") declared_skip=$(oncf_declared_skip_count "$f") runnable=$(oncf_expect_runnable "$f")"
  done
  echo "totals: listed=${ONCF_EXPECT_LISTED_TOTAL} declared_skip=${ONCF_EXPECT_SKIP_TOTAL} runnable=${ONCF_EXPECT_RUNNABLE_TOTAL}"
  exit 0
fi

oncf_install_workspace "$FAST"

if [[ "$COLLECT_ONLY" == "1" ]]; then
  ONCF_COLLECT_FIXTURES="${SELECTED[*]}"
  export ONCF_COLLECT_FIXTURES
  set +e
  {
    echo "=== opennext-official collect-only ${RUN_ID} $(date -Iseconds) ==="
    oncf_validate_collect_counts
  } 2>&1 | tee "$SUMMARY_LOG"
  rc=${PIPESTATUS[0]}
  set -e
  exit "$rc"
fi

oncf_ensure_browser
require_stack_or_skip
require_platform

{
  echo "=== opennext-official e2e ${RUN_ID} $(date -Iseconds) ==="
  echo "fixtures: ${SELECTED[*]} skip_build=${SKIP_BUILD} fast=${FAST}"

  OVERALL=0
  for fixture in "${SELECTED[@]}"; do
    VERSION="v-oncf-${fixture}-$(date +%s)-${RANDOM}"
    PROJECT="$(oncf_project_id "$fixture")"
    BUILD_RC=0
    DEPLOY_RC=0
    PW_RC=0

    echo "--- fixture=${fixture} project=${PROJECT} version=${VERSION} ---"

    set +e
    oncf_build_fixture "$fixture" "$SKIP_BUILD"
    BUILD_RC=$?
    set -e
    if [[ "$BUILD_RC" -ne 0 ]]; then
      echo "RESULT ${fixture} build=${BUILD_RC} deploy=skipped playwright=skipped"
      OVERALL=1
      continue
    fi

    set +e
    oncf_stage_and_register_preview "$fixture" "$VERSION"
    DEPLOY_RC=$?
    set -e
    if [[ "$DEPLOY_RC" -ne 0 ]]; then
      echo "RESULT ${fixture} build=0 deploy=${DEPLOY_RC} playwright=skipped"
      OVERALL=1
      continue
    fi

    set +e
    oncf_run_playwright "$fixture" "$VERSION"
    PW_RC=$?
    set -e
    echo "RESULT ${fixture} build=0 deploy=0 playwright=${PW_RC}"
    [[ "$PW_RC" -eq 0 ]] || OVERALL=1
  done

  echo "=== summary exit=${OVERALL} log=${SUMMARY_LOG} ==="
  exit "$OVERALL"
} 2>&1 | tee "$SUMMARY_LOG"

exit "${PIPESTATUS[0]}"
