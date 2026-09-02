#!/usr/bin/env bash
# Detect clock skew between this host and the dev S3 (RustFS) endpoint.
# AWS SigV4 and RustFS reject requests when skew is large (RequestTimeTooSkewed).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
# shellcheck disable=SC1091
source "${ROOT}/dev/.env" 2>/dev/null || true

EP="${S3_ENDPOINT:-http://127.0.0.1:${S3_PORT:-19000}}"
MAX_SKEW_SEC="${CELLP_S3_MAX_CLOCK_SKEW_SEC:-900}"

if ! curl -sf "${EP}/health" >/dev/null 2>&1; then
  echo "WARN: S3 endpoint not reachable at ${EP} (skip skew check)"
  exit 0
fi

hdr_date="$(curl -sI "${EP}/health" 2>/dev/null | awk -F': ' 'tolower($1)=="date"{print $2; exit}' | tr -d '\r')"
if [[ -z "$hdr_date" ]]; then
  echo "WARN: no Date header from ${EP} (skip skew check)"
  exit 0
fi

if [[ "$(uname)" == "Darwin" ]]; then
  srv_epoch="$(date -j -f "%a, %d %b %Y %H:%M:%S %Z" "$hdr_date" +%s 2>/dev/null || true)"
else
  srv_epoch="$(date -d "$hdr_date" +%s 2>/dev/null || true)"
fi
host_epoch="$(date +%s)"
if [[ -z "$srv_epoch" ]]; then
  echo "WARN: could not parse server Date: ${hdr_date}"
  exit 0
fi

skew=$(( host_epoch > srv_epoch ? host_epoch - srv_epoch : srv_epoch - host_epoch ))
echo "S3 clock skew: ${skew}s (host vs ${EP} Date header, max ${MAX_SKEW_SEC}s)"

if (( skew > MAX_SKEW_SEC )); then
  echo "FAIL: clock skew ${skew}s > ${MAX_SKEW_SEC}s — fix macOS Date & Time (NTP), then:"
  echo "  ./dev/scripts/up.sh && ./dev/scripts/health.sh"
  echo "See docs/platform-defects-log.md PD-20260902-04"
  exit 1
fi
exit 0
