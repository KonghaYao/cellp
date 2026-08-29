#!/usr/bin/env bash
# TP2-S2 — version limit (6th ready POST -> 429 or documented queue)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib/common.sh"
stress_source_env
stress_require_tools
stress_ensure_api

LIMIT="$(stress_threshold STRESS_VERSION_LIMIT 5)"
ATTEMPTS=$((LIMIT + 1))

PROJECT="$(stress_project_id verlimit)"
stress_create_project "$PROJECT"
stress_log "TP2-S2 version limit — project=$PROJECT limit=$LIMIT expect 429 on attempt $ATTEMPTS"

declare -a codes=()
for i in $(seq 1 "$ATTEMPTS"); do
  vid="$(stress_version_id "vl-$i")"
  code="$(stress_post_version "$PROJECT" "$vid")"
  codes+=("$code")
  stress_log "  POST $vid -> HTTP $code"
  if [[ "$code" == "202" || "$code" == "200" ]]; then
    stress_wait_version "$PROJECT" "$vid" 600 >/dev/null || true
  fi
  sleep 1
done

last="${codes[-1]}"
stress_record_metric "TP2-S2" "last_http" "$last" "{\"attempts\":$ATTEMPTS,\"limit\":$LIMIT}"

if [[ "$last" == "429" ]]; then
  stress_pass "TP2-S2: 6th POST returned 429"
elif [[ "$last" == "202" ]]; then
  stress_log "WARN: platform accepted >${LIMIT} versions — document queue behavior if intentional"
  stress_pass "TP2-S2: queue behavior (no 429) — document in stress-report.md"
else
  stress_fail "TP2-S2: expected 429 or 202 on attempt ${ATTEMPTS}, got HTTP $last"
fi
