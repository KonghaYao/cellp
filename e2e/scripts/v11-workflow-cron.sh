#!/usr/bin/env bash
# TP-V11 — Workflow readonly instances (not 500) + Cron bindings list
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_stack_or_skip

PROJECT="${DEV_PROJECT}"
VW="$(unique_id)"
VCRON="$(unique_id)"
WF_NAME="report-builder"
WF_EXAMPLE="${E2E_ROOT}/celld/examples/workflow"
CRON_EXAMPLE="${E2E_ROOT}/celld/examples/cron"
DEST_W="${ARTIFACTS_DIR}/${PROJECT}/${VW}"
DEST_C="${ARTIFACTS_DIR}/${PROJECT}/${VCRON}"
LOG="${EVIDENCE_DIR}/v11-workflow-cron-e2e.log"
JSON="${EVIDENCE_DIR}/v11-workflow-cron-e2e.json"
HEALTH_LOG="${EVIDENCE_DIR}/v11-health-path.log"

WF_BINDINGS_CODE=0
WF_LIST_CODE=0
INSTANCES_CODE=0
CRON_BINDINGS_CODE=0
EXIT_CODE=1

write_json() {
  jq -n \
    --arg project "$PROJECT" \
    --arg vw "$VW" \
    --arg vcron "$VCRON" \
    --arg workflow "$WF_NAME" \
    --argjson exit "$EXIT_CODE" \
    --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --argjson wf_bindings "$WF_BINDINGS_CODE" \
    --argjson wf_list "$WF_LIST_CODE" \
    --argjson instances "$INSTANCES_CODE" \
    --argjson cron_bindings "$CRON_BINDINGS_CODE" \
    '{
      project: $project,
      versions: [$vw, $vcron],
      workflow_name: $workflow,
      exit: $exit,
      timestamp: $ts,
      http: {
        workflow_bindings: $wf_bindings,
        workflows_list: $wf_list,
        instances: $instances,
        cron_bindings: $cron_bindings
      }
    }' >"$JSON"
}

trap 'write_json' EXIT

mkdir -p "$EVIDENCE_DIR"
: >"$LOG"
exec > >(tee -a "$LOG") 2>&1

{
  echo "TP-VE-1 $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "celld GET http://127.0.0.1:${CELLD_PORT}/.well-known/celld/health"
  curl -sS -w "\nhttp_code=%{http_code}\n" \
    "http://127.0.0.1:${CELLD_PORT}/.well-known/celld/health" || true
} >"$HEALTH_LOG" 2>&1 || true

log "V11 workflow+cron project=${PROJECT} workflow=${VW} cron=${VCRON}"

if [[ ! -d "$WF_EXAMPLE" ]]; then
  fail "missing ${WF_EXAMPLE}"
fi
if [[ ! -d "$CRON_EXAMPLE" ]]; then
  fail "missing ${CRON_EXAMPLE}"
fi

ensure_project "$PROJECT"
cleanup_e2e_versions "$PROJECT"

# --- Workflow ---
stage_worker_example "$WF_EXAMPLE" "$DEST_W"
create_version "$PROJECT" "$VW" | jq -r .id >/dev/null
poll_version "$PROJECT" "$VW" ready 120 >/dev/null

BASE_W="/v1/projects/${PROJECT}/versions/${VW}"

api_status GET "${BASE_W}/bindings"
WF_BINDINGS_CODE="$API_STATUS"
if [[ "$WF_BINDINGS_CODE" != "200" ]]; then
  echo "$API_BODY" >&2
  fail "GET workflow bindings → HTTP ${WF_BINDINGS_CODE} body=${API_BODY}"
fi
if ! echo "$API_BODY" | jq -e --arg n "$WF_NAME" \
  '.workflows[] | select(.name==$n and .binding=="REPORTS")' >/dev/null; then
  echo "$API_BODY" >&2
  fail "bindings.workflows missing name=${WF_NAME} binding=REPORTS"
fi
log "bindings contain workflow ${WF_NAME}"

api_status GET "${BASE_W}/workflows"
WF_LIST_CODE="$API_STATUS"
if [[ "$WF_LIST_CODE" != "200" ]]; then
  echo "$API_BODY" >&2
  fail "GET …/workflows → HTTP ${WF_LIST_CODE} (need T3) body=${API_BODY}"
fi
log "GET …/workflows 200"

api_status GET "${BASE_W}/workflows/${WF_NAME}/instances"
INSTANCES_CODE="$API_STATUS"
if [[ "$INSTANCES_CODE" == "500" ]]; then
  echo "$API_BODY" >&2
  fail "GET …/workflows/${WF_NAME}/instances → 500 (V11: empty list must be 200, never 500) body=${API_BODY}"
fi
if [[ "$INSTANCES_CODE" != "200" ]]; then
  echo "$API_BODY" >&2
  fail "GET …/instances → HTTP ${INSTANCES_CODE} (T3: 200 empty or 502 celld_cell_list_failed, never 500) body=${API_BODY}"
fi
if ! echo "$API_BODY" | jq -e '.instances | type=="array"' >/dev/null; then
  echo "$API_BODY" >&2
  fail "instances response missing instances[] array"
fi
log "GET …/instances HTTP 200 (n=$(echo "$API_BODY" | jq '.instances | length'))"

# Optional: Gateway create (outbound fetch; do not block)
CREATE_URL="${GATEWAY_URL}/${PROJECT}/${VW}/create?url=https://example.com"
CREATE_CODE=$(http_code "$CREATE_URL")
if [[ "$CREATE_CODE" == "200" ]]; then
  log "optional Gateway /create HTTP 200"
  api_status GET "${BASE_W}/workflows/${WF_NAME}/instances"
  INSTANCES_CODE="$API_STATUS"
  log "instances after create HTTP ${INSTANCES_CODE} n=$(echo "$API_BODY" | jq '.instances | length')"
else
  log "WARN: optional Gateway /create HTTP ${CREATE_CODE} (outbound fetch; not blocking)"
fi

# --- Cron (second destDir / version) ---
stage_worker_example "$CRON_EXAMPLE" "$DEST_C"
create_version "$PROJECT" "$VCRON" | jq -r .id >/dev/null
poll_version "$PROJECT" "$VCRON" ready 120 >/dev/null

BASE_C="/v1/projects/${PROJECT}/versions/${VCRON}"
api_status GET "${BASE_C}/bindings"
CRON_BINDINGS_CODE="$API_STATUS"
if [[ "$CRON_BINDINGS_CODE" != "200" ]]; then
  echo "$API_BODY" >&2
  fail "GET cron bindings → HTTP ${CRON_BINDINGS_CODE} body=${API_BODY}"
fi
if ! echo "$API_BODY" | jq -e '.crons[]? | select(.=="* * * * *")' >/dev/null; then
  echo "$API_BODY" >&2
  fail "bindings.crons missing \"* * * * *\" (got: $(echo "$API_BODY" | jq -c .crons))"
fi
log "bindings.crons contains * * * * *"

# No platform "trigger once" API — 404 is expected.
api_status GET "${BASE_C}/cron/trigger"
if [[ "$API_STATUS" == "404" ]]; then
  log "WARN: no cron trigger API (HTTP 404) — V11 is visibility-only"
else
  log "WARN: GET …/cron/trigger HTTP ${API_STATUS} (not asserted)"
fi

EXIT_CODE=0
pass "V11 workflow instances not 500 + cron bindings OK VW=${VW} VCRON=${VCRON}"
cleanup_e2e_versions "$PROJECT" || true
exit 0
