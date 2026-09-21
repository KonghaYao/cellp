
# --- AD-12 Host ingress (Gateway routes by Host, not path) ---
: "${CELLP_INGRESS_BASE_DOMAIN:=ingress.local}"

ingress_base_domain() {
  echo "${CELLP_INGRESS_BASE_DOMAIN}"
}

prod_host() {
  local project="${1:-$DEV_PROJECT}"
  echo "${project}.$(ingress_base_domain)"
}

preview_host() {
  local project="$1"
  local version="$2"
  echo "${version}.${project}.$(ingress_base_domain)"
}

version_preview_url() {
  local project="$1"
  local version="$2"
  api_get "/v1/projects/${project}/versions/${version}" "$ADMIN_TOKEN" | jq -r '.preview_url // empty'
}

# Dev Gateway TLS (mkcert): curl needs -k when GATEWAY_URL is https.
gateway_curl_tls_flags() {
  if [[ "${CELLP_GATEWAY_CURL_INSECURE:-}" == "1" || "${GATEWAY_URL}" == https://* ]]; then
    echo -k
  fi
}

curl_gateway_host() {
  local host="$1"
  local path="${2:-/}"
  # shellcheck disable=SC2046
  curl -sf $(gateway_curl_tls_flags) -H "Host: ${host}" "${GATEWAY_URL}${path}"
}

http_code_gateway_host() {
  local host="$1"
  local path="${2:-/}"
  # shellcheck disable=SC2046
  curl -s -o /dev/null -w '%{http_code}' $(gateway_curl_tls_flags) -H "Host: ${host}" "${GATEWAY_URL}${path}" 2>/dev/null || echo "000"
}

# --- Scale-to-zero cold contract (AD-15) ---
# A version that just scaled to zero can still be named by the Gateway's previous routing
# snapshot for one revision poll. A first request that lands on that retired endpoint
# gets the documented cold answer: 503 + X-Cellp-Reason + Retry-After, never a 502. Only
# that contract is retried here; every other status (502/500/404/…) stays a hard failure.
: "${CELLP_COLD_RETRY_TIMEOUT:=60}"

# cold_reason_retryable HEADERS_FILE — true when the 503 carries a cold-start reason.
cold_reason_retryable() {
  local reason
  reason=$(awk 'BEGIN{IGNORECASE=1} /^X-Cellp-Reason:/ {print $2; exit}' "$1" | tr -d '\r')
  case "$reason" in
    wake_retry|wake_queue_full|wake_timeout|control_unavailable|capacity_exhausted) return 0 ;;
    *) return 1 ;;
  esac
}

cold_retry_after_secs() {
  local value
  value=$(awk 'BEGIN{IGNORECASE=1} /^Retry-After:/ {print $2; exit}' "$1" | tr -d '\r')
  case "$value" in
    ''|*[!0-9]*) echo 1 ;;
    *) [[ "$value" -gt 5 ]] && echo 5 || echo "$value" ;;
  esac
}

# gateway_cold_request METHOD HOST PATH OUTFILE [curl args...]
# Writes the response body to OUTFILE and echoes the final HTTP status code. Retries only
# while the Gateway answers the cold contract above.
gateway_cold_request() {
  local method="$1" host="$2" path="$3" out="$4"
  shift 4
  local deadline=$((SECONDS + CELLP_COLD_RETRY_TIMEOUT))
  local headers code attempt=0
  headers=$(mktemp)
  while :; do
    attempt=$((attempt + 1))
    # shellcheck disable=SC2046
    code=$(curl -sS -o "$out" -D "$headers" -w '%{http_code}' -X "$method" \
      $(gateway_curl_tls_flags) -H "Host: ${host}" "${GATEWAY_URL}${path}" "$@" 2>/dev/null || echo "000")
    if [[ "$code" != "503" ]] || ! cold_reason_retryable "$headers"; then
      break
    fi
    if (( SECONDS >= deadline )); then
      break
    fi
    log "cold retry attempt=${attempt} METHOD=${method} path=${path} status=${code} reason=$(awk 'BEGIN{IGNORECASE=1} /^X-Cellp-Reason:/ {print $2; exit}' "$headers" | tr -d '\r')" >&2
    sleep "$(cold_retry_after_secs "$headers")"
  done
  rm -f "$headers"
  echo "$code"
}

# curl_version_method_cold METHOD PROJECT VERSION PATH [curl args...]
# Like curl_version_method, but tolerates the cold contract and fails on anything else.
curl_version_method_cold() {
  local method="$1" project="$2" version="$3" path="$4"
  shift 4
  local out code
  out=$(mktemp)
  code=$(gateway_cold_request "$method" "$(preview_host "$project" "$version")" "$path" "$out" "$@")
  [[ "$code" == 2* ]] && cat "$out"
  rm -f "$out"
  [[ "$code" == 2* ]]
}

# curl_gateway_host_cold HOST PATH — body on stdout once the cold contract clears.
curl_gateway_host_cold() {
  local out code
  out=$(mktemp)
  code=$(gateway_cold_request GET "$1" "${2:-/}" "$out")
  [[ "$code" == 2* ]] && cat "$out"
  rm -f "$out"
  [[ "$code" == 2* ]]
}

# http_code_gateway_host_cold HOST PATH — final status code after the cold contract clears.
http_code_gateway_host_cold() {
  local out code
  out=$(mktemp)
  code=$(gateway_cold_request GET "$1" "${2:-/}" "$out")
  rm -f "$out"
  echo "$code"
}

wait_http_200_host() {
  local host="$1"
  local path="${2:-/}"
  local timeout="${3:-60}"
  local i code
  for i in $(seq 1 "$timeout"); do
    code=$(http_code_gateway_host "$host" "$path")
    if [[ "$code" == "200" ]]; then
      return 0
    fi
    sleep 1
  done
  fail "expected HTTP 200 from Host=${host} ${GATEWAY_URL}${path} (last=${code})"
}

wait_http_gone_host() {
  local host="$1"
  local path="${2:-/}"
  local timeout="${3:-120}"
  local i code
  for i in $(seq 1 "$timeout"); do
    code=$(http_code_gateway_host "$host" "$path")
    if [[ "$code" == "404" || "$code" == "410" || "$code" == "503" ]]; then
      return 0
    fi
    sleep 1
  done
  fail "expected HTTP 404/410/503 from Host=${host} (last=${code})"
}

curl_version() {
  curl_gateway_host "$(preview_host "$1" "$2")" "${3:-/}"
}

curl_version_method() {
  local method="$1"
  local project="$2"
  local version="$3"
  local path="${4:-/}"
  shift 4
  curl -fsS -X "$method" $(gateway_curl_tls_flags) -H "Host: $(preview_host "$project" "$version")" "${GATEWAY_URL}${path}" "$@"
}

http_code_version() {
  http_code_gateway_host "$(preview_host "$1" "$2")" "${3:-/}"
}

wait_http_200_version() {
  wait_http_200_host "$(preview_host "$1" "$2")" "${3:-/}" "${4:-60}"
}

wait_http_gone_version() {
  wait_http_gone_host "$(preview_host "$1" "$2")" "${3:-/}" "${4:-120}"
}

curl_prod() {
  curl_gateway_host "$(prod_host "$1")" "${2:-/}"
}

http_code_prod() {
  http_code_gateway_host "$(prod_host "$1")" "${2:-/}"
}

wait_http_200_prod() {
  wait_http_200_host "$(prod_host "$1")" "${2:-/}" "${3:-60}"
}

# Legacy path URL (deprecated); use only when INGRESS_HOST_ONLY=0 fallback.
gateway_path_preview() {
  local project="$1"
  local version="$2"
  local path="${3:-/}"
  echo "${GATEWAY_URL}/${project}/${version}${path}"
}

gateway_path_prod() {
  local project="$1"
  local path="${2:-/}"
  echo "${GATEWAY_URL}/${project}${path}"
}
