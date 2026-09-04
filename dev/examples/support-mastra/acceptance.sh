#!/usr/bin/env bash
# Strict A05 gate: real Agent + Tool + Workflow + R2 cache + D1 Memory.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck disable=SC1091
source "${ROOT}/e2e/scripts/lib.sh"

PROJECT="support-mastra"
VERSION="${1:?usage: acceptance.sh <version>  e.g. v12}"
HOST="$(preview_host "$PROJECT" "$VERSION")"
FAILURES=0
HTTP_STATUS=""
HTTP_BODY=""

record_failure() {
  echo "FAIL: $*" >&2
  FAILURES=$((FAILURES + 1))
}

request() {
  local method="$1"
  local path="$2"
  local body="${3-}"
  local timeout="${4:-45}"
  local tmp
  local -a args
  tmp="$(mktemp)"
  args=(-sS -o "$tmp" -w '%{http_code}' --max-time "$timeout" -X "$method" -H "Host: ${HOST}")
  if [[ "${CELLP_GATEWAY_CURL_INSECURE:-}" == "1" || "$GATEWAY_URL" == https://* ]]; then
    args+=(-k)
  fi
  if [[ -n "$body" ]]; then
    args+=(-H 'Content-Type: application/json' -d "$body")
  fi
  if ! HTTP_STATUS="$(curl "${args[@]}" "${GATEWAY_URL}${path}")"; then
    HTTP_STATUS="000"
  fi
  HTTP_BODY="$(<"$tmp")"
  rm -f "$tmp"
}

has_string() {
  local value="$1"
  jq -e --arg value "$value" '.. | strings | select(. == $value)' >/dev/null 2>&1 <<<"$HTTP_BODY"
}

log "A05 strict acceptance project=${PROJECT} version=${VERSION} host=${HOST}"
wait_http_200_version "$PROJECT" "$VERSION" "/" 60
request GET "/"
if [[ "$HTTP_STATUS" == "200" && "$HTTP_BODY" == *'id="agent-form"'* && "$HTTP_BODY" == *'id="memory-form"'* ]]; then
  pass "custom Agent/Tool/Workflow/D1/R2 frontend"
else
  record_failure "custom frontend controls missing (HTTP ${HTTP_STATUS})"
fi

request GET "/api/agents"
if [[ "$HTTP_STATUS" == "200" ]] && has_string "weather-agent"; then
  pass "Mastra Agent registry"
else
  record_failure "Agent registry missing weather-agent (HTTP ${HTTP_STATUS})"
fi

request GET "/api/tools"
if [[ "$HTTP_STATUS" == "200" ]] && has_string "get-weather" && has_string "get-forecast-cache"; then
  pass "Mastra Tool registry"
else
  record_failure "Tool registry missing get-weather or get-forecast-cache (HTTP ${HTTP_STATUS})"
fi

agent_marker="CELLP_A05_AGENT_OK"
agent_body="$(jq -nc --arg marker "$agent_marker" '{messages:("Reply with exactly " + $marker + "."),toolChoice:"none"}')"
request POST "/api/agents/weather-agent/generate" "$agent_body" 90
if [[ "$HTTP_STATUS" == "200" ]] && jq -e --arg marker "$agent_marker" '.text | strings | contains($marker)' >/dev/null 2>&1 <<<"$HTTP_BODY"; then
  pass "real Mastra Agent generation"
elif [[ "$HTTP_BODY" == *"429"* || "$HTTP_BODY" == *"rate limit"* || "$HTTP_BODY" == *"Rate limit"* ]]; then
  record_failure "real Agent blocked by external model rate limit (HTTP ${HTTP_STATUS})"
else
  record_failure "real Agent generation/schema (HTTP ${HTTP_STATUS})"
fi

weather_body='{"data":{"location":"Tokyo"}}'
request POST "/api/tools/get-weather/execute" "$weather_body" 60
if [[ "$HTTP_STATUS" == "200" ]] && jq -e '
  .location == "Tokyo" and
  (.temperature | type == "number") and
  (.humidity | type == "number") and
  (.conditions | type == "string")
' >/dev/null 2>&1 <<<"$HTTP_BODY"; then
  pass "real Mastra weather Tool execution"
else
  record_failure "weather Tool execution/schema (HTTP ${HTTP_STATUS})"
fi

cache_namespace="a05-${VERSION}-$(date +%s)-${RANDOM}"
cache_body="$(jq -nc --arg namespace "$cache_namespace" '{data:{city:"Tokyo",cacheNamespace:$namespace}}')"
request POST "/api/tools/get-forecast-cache/execute" "$cache_body" 60
cache_first="$HTTP_BODY"
cache_first_status="$HTTP_STATUS"
request POST "/api/tools/get-forecast-cache/execute" "$cache_body" 60
cache_second="$HTTP_BODY"
cache_second_status="$HTTP_STATUS"
if [[ "$cache_first_status" == "200" && "$cache_second_status" == "200" ]] &&
  jq -e '.cacheHit == false and (.location | type == "string")' >/dev/null 2>&1 <<<"$cache_first" &&
  jq -e '.cacheHit == true' >/dev/null 2>&1 <<<"$cache_second" &&
  [[ "$(jq -cS 'del(.cacheHit)' <<<"$cache_first")" == "$(jq -cS 'del(.cacheHit)' <<<"$cache_second")" ]]; then
  pass "R2 forecast cache miss -> hit"
else
  record_failure "R2 cache miss/hit contract (HTTP ${cache_first_status}/${cache_second_status})"
fi

workflow_namespace="workflow-${cache_namespace}"
workflow_body="$(jq -nc --arg namespace "$workflow_namespace" '{inputData:{city:"Tokyo",cacheNamespace:$namespace}}')"
request POST "/api/workflows/weather-workflow/start-async" "$workflow_body" 120
if [[ "$HTTP_STATUS" == "200" ]] && jq -e '
  .status == "success" and
  (.result.activities | type == "string" and length > 0) and
  (.result.cacheHit == false) and
  (.result.planningSource == "agent" or .result.planningSource == "rate-limit-fallback")
' >/dev/null 2>&1 <<<"$HTTP_BODY"; then
  planning_source="$(jq -r '.result.planningSource' <<<"$HTTP_BODY")"
  pass "Mastra Workflow execution (planningSource=${planning_source})"
else
  record_failure "Workflow execution/schema (HTTP ${HTTP_STATUS})"
fi

suffix="$(date +%s)-${RANDOM}"
thread_id="cellp-a05-thread-${suffix}"
resource_id="cellp-a05-resource-${suffix}"
message_id="cellp-a05-message-${suffix}"
marker="cellp-a05-d1-marker-${suffix}"
thread_body="$(jq -nc --arg thread "$thread_id" --arg resource "$resource_id" '{threadId:$thread,resourceId:$resource,title:"cellp A05 persistence"}')"
request POST "/api/memory/threads?agentId=weather-agent" "$thread_body" 60
if [[ "$HTTP_STATUS" == "200" ]] && jq -e --arg thread "$thread_id" --arg resource "$resource_id" '.id == $thread and .resourceId == $resource' >/dev/null 2>&1 <<<"$HTTP_BODY"; then
  pass "Mastra Memory thread create"
else
  record_failure "Memory thread create/schema (HTTP ${HTTP_STATUS})"
fi

message_body="$(jq -nc --arg id "$message_id" --arg thread "$thread_id" --arg resource "$resource_id" --arg marker "$marker" '{messages:[{id:$id,role:"user",threadId:$thread,resourceId:$resource,type:"text",content:{format:2,parts:[{type:"text",text:$marker}]}}]}')"
request POST "/api/memory/save-messages?agentId=weather-agent" "$message_body" 60
if [[ "$HTTP_STATUS" == "200" ]] && has_string "$message_id"; then
  pass "Mastra Memory message save"
else
  record_failure "Memory message save/schema (HTTP ${HTTP_STATUS})"
fi

request GET "/api/memory/threads/${thread_id}/messages?agentId=weather-agent&resourceId=${resource_id}" "" 60
if [[ "$HTTP_STATUS" == "200" ]] && has_string "$marker"; then
  pass "Mastra Memory message read"
else
  record_failure "Memory message read/persistence (HTTP ${HTTP_STATUS})"
fi

sql="SELECT * FROM mastra_messages WHERE id = '${message_id}' LIMIT 1"
api_status POST "/v1/projects/${PROJECT}/versions/${VERSION}/database/query" "$(jq -nc --arg sql "$sql" '{sql:$sql}')"
if [[ "$API_STATUS" == "200" ]] && jq -e --arg marker "$marker" '.. | strings | select(contains($marker))' >/dev/null 2>&1 <<<"$API_BODY"; then
  pass "D1 marker verified through cellp database API"
else
  record_failure "D1 independent marker query (HTTP ${API_STATUS})"
fi

if (( FAILURES > 0 )); then
  fail "A05 strict acceptance: ${FAILURES} check(s) failed"
fi
pass "A05 Agent + Tool + Workflow + R2 + Memory/D1"
