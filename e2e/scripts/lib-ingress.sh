
# --- AD-12 Host ingress (Gateway routes by Host, not path) ---
: "${CELLP_INGRESS_BASE_DOMAIN:=ingress.local}"

ingress_base_domain() {
  echo "${CELLP_INGRESS_BASE_DOMAIN}"
}

# Stable prod Host FQDN (matches cellp config.ProdHost).
prod_host() {
  local project="${1:-$DEV_PROJECT}"
  echo "${project}.$(ingress_base_domain)"
}

# Preview Host from version id + project (matches config.PreviewHost labels).
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

curl_gateway_host() {
  local host="$1"
  local path="${2:-/}"
  curl -sf -H "Host: ${host}" "${GATEWAY_URL}${path}"
}

http_code_gateway_host() {
  local host="$1"
  local path="${2:-/}"
  curl -s -o /dev/null -w '%{http_code}' -H "Host: ${host}" "${GATEWAY_URL}${path}" 2>/dev/null || echo "000"
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
