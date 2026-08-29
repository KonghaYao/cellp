#!/usr/bin/env bash
# TP2-MET-1 — collect RSS, sqlite size, gateway health; append stress-metrics.jsonl
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib/common.sh"
stress_source_env
stress_require_tools

TEST_ID="${1:-TP2-MET-1}"
stress_log "collect-metrics test_id=$TEST_ID"

rss="$(stress_process_rss_mb)"
sqlite_bytes="$(stress_sqlite_bytes)"
sqlite_mb="$(awk -v b="$sqlite_bytes" 'BEGIN { printf "%.2f", b/1024/1024 }')"

gw_ok=0
if curl -sf "${GATEWAY_URL}/health" >/dev/null 2>&1; then gw_ok=1; fi
api_ok=0
if curl -sf "${PLATFORM_URL}/v1/health" >/dev/null 2>&1; then api_ok=1; fi

routes="$(stress_active_routes_count)"

stress_record_metric "$TEST_ID" "rss_mb" "$rss" "{}"
stress_record_metric "$TEST_ID" "sqlite_mb" "$sqlite_mb" "{\"bytes\":$sqlite_bytes}"
stress_record_metric "$TEST_ID" "health" "1" \
  "{\"gateway\":$gw_ok,\"api\":$api_ok,\"active_routes\":\"$routes\"}"

# Hardware snapshot (once) if stress-env.json hardware empty
if [[ -f "$STRESS_ENV_JSON" ]] && command -v jq >/dev/null; then
  cpu_empty="$(jq -r '.hardware.cpu // ""' "$STRESS_ENV_JSON")"
  if [[ -z "$cpu_empty" ]]; then
    cpu_model="$(grep -m1 'model name' /proc/cpuinfo 2>/dev/null | cut -d: -f2 | xargs || echo unknown)"
    ram_gb="$(awk '/MemTotal/ {printf "%.1f", $2/1024/1024}' /proc/meminfo 2>/dev/null || echo 0)"
    disk="$(df -h "$STRESS_ROOT" 2>/dev/null | awk 'NR==2 {print $2" total "$4" free"}' || echo unknown)"
    tmp="$(mktemp)"
    jq --arg cpu "$cpu_model" --argjson ram "$ram_gb" --arg disk "$disk" \
      '.hardware.cpu = $cpu | .hardware.ram_gb = $ram | .hardware.disk = $disk' \
      "$STRESS_ENV_JSON" >"$tmp" && mv "$tmp" "$STRESS_ENV_JSON"
  fi
fi

echo "metrics -> $STRESS_METRICS"
echo "  rss_mb=$rss sqlite_mb=$sqlite_mb routes=$routes gateway=$gw_ok api=$api_ok"
