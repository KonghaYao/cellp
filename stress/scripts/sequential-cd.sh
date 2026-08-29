#!/usr/bin/env bash
# TP2-L1 — sequential CD baseline (10 deploys, p95 deploy time)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib/common.sh"
stress_source_env
stress_require_tools
stress_ensure_api

PROJECT="$(stress_project_id l1)"
DEPLOYS=10
P95_MAX="$(stress_threshold STRESS_P95_DEPLOY_SEC 600)"

stress_log "TP2-L1 sequential CD — project=$PROJECT deploys=$DEPLOYS p95_max=${P95_MAX}s"
stress_create_project "$PROJECT"

declare -a durations=()
ready=0
failed=0

for i in $(seq 1 "$DEPLOYS"); do
  vid="$(stress_version_id "$i")"
  t0=$SECONDS
  http="$(stress_post_version "$PROJECT" "$vid")"
  if [[ "$http" != "202" && "$http" != "200" && "$http" != "201" ]]; then
    stress_log "POST $vid -> HTTP $http"
    failed=$((failed + 1))
    continue
  fi
  status="$(stress_wait_version "$PROJECT" "$vid" 900 || echo timeout)"
  elapsed=$((SECONDS - t0))
  durations+=("$elapsed")
  stress_log "  $vid -> $status in ${elapsed}s"
  case "$status" in
    ready) ready=$((ready + 1)) ;;
    *) failed=$((failed + 1)) ;;
  esac
done

total=$((ready + failed))
if (( total == 0 )); then
  stress_fail "no deploy attempts completed"
fi

p95="$(stress_percentile 95 "${durations[@]}")"
stress_record_metric "TP2-L1" "p95_deploy_sec" "$p95" "{\"project\":\"$PROJECT\",\"ready\":$ready,\"failed\":$failed}"

stress_log "results: ${ready}/${DEPLOYS} ready, ${failed} failed, p95=${p95}s"

if (( ready + failed != DEPLOYS )); then
  stress_fail "expected ${DEPLOYS} terminal states, got $((ready + failed))"
fi
if (( failed > 0 )); then
  stress_fail "${failed}/${DEPLOYS} did not reach ready"
fi

if awk -v p95="$p95" -v max="$P95_MAX" 'BEGIN { exit !(p95 <= max) }'; then
  stress_pass "TP2-L1 p95=${p95}s <= ${P95_MAX}s"
else
  stress_fail "TP2-L1 p95=${p95}s exceeds threshold ${P95_MAX}s"
fi

# Seed baseline on first successful L1 if unset
if command -v jq >/dev/null && [[ -f "$STRESS_ENV_JSON" ]]; then
  cur="$(jq -r '.baseline.l1_p95_deploy_sec // 0' "$STRESS_ENV_JSON")"
  if [[ "$cur" == "0" || "$cur" == "null" ]]; then
    rss="$(stress_process_rss_mb)"
    tmp="$(mktemp)"
    jq --argjson p95 "$p95" --argjson rss "$rss" \
      '.baseline.l1_p95_deploy_sec = $p95 | .baseline.rss_t0_mb = $rss' \
      "$STRESS_ENV_JSON" >"$tmp" && mv "$tmp" "$STRESS_ENV_JSON"
    stress_log "updated baseline.l1_p95_deploy_sec=$p95 rss_t0_mb=$rss in stress-env.json"
  fi
fi
