#!/usr/bin/env bash
# TP2-S1 — 24h soak (RSS growth + sqlite size); shortened mode for CI
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib/common.sh"
stress_source_env
stress_require_tools
stress_ensure_api

SHORT=0
for arg in "$@"; do
  case "$arg" in
    -short|--short) SHORT=1 ;;
  esac
done

if (( SHORT == 1 )) || [[ "${STRESS_SOAK_SHORT:-}" == "1" ]]; then
  SOAK_SEC="${STRESS_SOAK_SECONDS:-300}"
  stress_log "TP2-S1 soak SHORT mode — ${SOAK_SEC}s (CI)"
else
  SOAK_SEC="${STRESS_SOAK_SECONDS:-86400}"
  stress_log "TP2-S1 soak — ${SOAK_SEC}s (24h default)"
fi

RSS_RATIO_MAX="$(stress_threshold STRESS_SOAK_RSS_RATIO 1.10)"
REG_MAX_MB="$(stress_threshold STRESS_REGISTRY_MAX_MB 500)"
INTERVAL="${STRESS_SOAK_INTERVAL:-300}"
RPS="${STRESS_SOAK_RPS:-10}"

PROJECT="$(stress_project_id soak)"
VID="$(stress_version_id soak)"
stress_create_project "$PROJECT"
stress_post_version "$PROJECT" "$VID" >/dev/null
stress_wait_version "$PROJECT" "$VID" 900 >/dev/null

TARGET="${GATEWAY_URL}/${PROJECT}/${VID}/"
rss_t0="$(stress_process_rss_mb)"
sqlite_t0="$(stress_sqlite_bytes)"
stress_log "T0 RSS=${rss_t0}MB sqlite=$(( sqlite_t0 / 1024 / 1024 ))MB"

end=$((SECONDS + SOAK_SEC))
while (( SECONDS < end )); do
  for _ in $(seq 1 "$RPS"); do
    curl -sf -o /dev/null "$TARGET" 2>/dev/null || true
  done
  rss="$(stress_process_rss_mb)"
  sqlite="$(stress_sqlite_bytes)"
  stress_record_metric "TP2-S1" "sample" "$rss" \
    "{\"sqlite_bytes\":$sqlite,\"elapsed_sec\":$((SECONDS))}"
  stress_log "sample RSS=${rss}MB sqlite=$(( sqlite / 1024 / 1024 ))MB elapsed=$((SECONDS))s"
  sleep "$INTERVAL"
done

rss_t1="$(stress_process_rss_mb)"
sqlite_t1="$(stress_sqlite_bytes)"
ratio="$(awk -v t0="$rss_t0" -v t1="$rss_t1" 'BEGIN { if (t0<=0) print 1; else print t1/t0 }')"
sqlite_mb=$(( sqlite_t1 / 1024 / 1024 ))

stress_log "T1 RSS=${rss_t1}MB ratio=${ratio} sqlite=${sqlite_mb}MB"
stress_record_metric "TP2-S1" "rss_ratio" "$ratio" \
  "{\"rss_t0_mb\":$rss_t0,\"rss_t1_mb\":$rss_t1,\"sqlite_mb\":$sqlite_mb}"

if awk -v r="$ratio" -v max="$RSS_RATIO_MAX" 'BEGIN { exit !(r < max) }'; then
  stress_log "RSS ratio OK"
else
  stress_fail "TP2-S1 RSS ratio ${ratio} >= ${RSS_RATIO_MAX}"
fi
if (( sqlite_mb >= REG_MAX_MB )); then
  stress_fail "TP2-S1 registry ${sqlite_mb}MB >= ${REG_MAX_MB}MB"
fi

stress_pass "TP2-S1 soak complete (ratio=${ratio}, sqlite=${sqlite_mb}MB)"
