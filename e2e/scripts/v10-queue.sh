#!/usr/bin/env bash
# TP-V10 — celld Queue operator via cellpd :8790 (info/peek/purge force + sibling empty queue)
# Example: dev/examples/queue (producer-only — celld forbids consumer + export fetch()).
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_stack_or_skip

PROJECT="${DEV_PROJECT}"
VA="$(unique_id)"
VC="$(unique_id)"
QUEUE="tasks"
MARKER="v10-e2e-${VA}"
QUEUE_EXAMPLE="${E2E_ROOT}/dev/examples/queue"
DEST_A="${ARTIFACTS_DIR}/${PROJECT}/${VA}"
DEST_C="${ARTIFACTS_DIR}/${PROJECT}/${VC}"
LOG="${EVIDENCE_DIR}/v10-queue-e2e.log"
JSON="${EVIDENCE_DIR}/v10-queue-e2e.json"

BINDINGS_CODE=0
INFO_CODE=0
PEEK_CODE=0
PURGE_NOFORCE_CODE=0
PURGE_FORCE_CODE=0
CHILD_PEEK_CODE=0
PAUSE_CODE=0
EXIT_CODE=1

write_json() {
  jq -n \
    --arg project "$PROJECT" \
    --arg va "$VA" \
    --arg vc "$VC" \
    --arg queue "$QUEUE" \
    --argjson exit "$EXIT_CODE" \
    --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --argjson bindings "$BINDINGS_CODE" \
    --argjson info "$INFO_CODE" \
    --argjson peek "$PEEK_CODE" \
    --argjson purge_noforce "$PURGE_NOFORCE_CODE" \
    --argjson purge_force "$PURGE_FORCE_CODE" \
    --argjson child_peek "$CHILD_PEEK_CODE" \
    --argjson pause "$PAUSE_CODE" \
    '{
      project: $project,
      versions: [$va, $vc],
      queue: $queue,
      exit: $exit,
      timestamp: $ts,
      http: {
        bindings: $bindings,
        info: $info,
        peek: $peek,
        purge_noforce: $purge_noforce,
        purge_force: $purge_force,
        child_peek: $child_peek,
        pause: $pause
      },
      note: "producer-only example; no queues.consumers (celld: consumer cannot export fetch)"
    }' >"$JSON"
}

trap 'write_json' EXIT

peek_has_marker() {
  local raw="$1"
  local marker="$2"
  python3 -c '
import json, sys, base64
raw, marker = sys.argv[1], sys.argv[2]
try:
    data = json.loads(raw)
except Exception:
    sys.exit(1)
msgs = data.get("messages", data) if isinstance(data, dict) else data
if msgs is None:
    msgs = []
if isinstance(msgs, dict):
    msgs = [msgs]
for m in msgs:
    if not isinstance(m, dict):
        continue
    b64 = m.get("bodyBase64") or m.get("body_base64") or ""
    if b64:
        try:
            body = base64.b64decode(b64).decode("utf-8", "replace")
        except Exception:
            body = ""
        if marker in body:
            sys.exit(0)
    blob = json.dumps(m)
    if marker in blob:
        sys.exit(0)
if marker in raw:
    sys.exit(0)
sys.exit(1)
' "$raw" "$marker"
}

peek_count() {
  echo "$1" | jq 'if type=="object" then ((.messages // []) | length) elif type=="array" then length else 0 end'
}

mkdir -p "$EVIDENCE_DIR"
: >"$LOG"
exec > >(tee -a "$LOG") 2>&1

need python3

log "V10 Queue operator project=${PROJECT} VA=${VA} VC=${VC} queue=${QUEUE}"

if [[ ! -d "$QUEUE_EXAMPLE" ]]; then
  fail "missing ${QUEUE_EXAMPLE} — celld/examples has no queue; use dev/examples/queue"
fi

ensure_project "$PROJECT"
cleanup_e2e_versions "$PROJECT"

stage_worker_example "$QUEUE_EXAMPLE" "$DEST_A"
stage_worker_example "$QUEUE_EXAMPLE" "$DEST_C"

create_version "$PROJECT" "$VA" | jq -r .id >/dev/null
poll_version "$PROJECT" "$VA" ready 120 >/dev/null

BASE_A="/v1/projects/${PROJECT}/versions/${VA}"
BASE_C="/v1/projects/${PROJECT}/versions/${VC}"

api_status GET "${BASE_A}/bindings"
BINDINGS_CODE="$API_STATUS"
if [[ "$BINDINGS_CODE" != "200" ]]; then
  echo "$API_BODY" >&2
  fail "GET ${BASE_A}/bindings → HTTP ${BINDINGS_CODE} (need T1 ParseBindings) body=${API_BODY}"
fi
if ! echo "$API_BODY" | jq -e --arg q "$QUEUE" '.queues[] | select(.name==$q and .binding=="TASKS")' >/dev/null; then
  echo "$API_BODY" >&2
  fail "bindings.queues missing name=${QUEUE} binding=TASKS (got: $(echo "$API_BODY" | jq -c .queues))"
fi
log "bindings VA contains queue name=${QUEUE}"

api_status GET "${BASE_A}/queues/${QUEUE}"
INFO_CODE="$API_STATUS"
if [[ "$INFO_CODE" != "200" ]]; then
  echo "$API_BODY" >&2
  fail "GET …/queues/${QUEUE} info → HTTP ${INFO_CODE} (need T2 QueueInfo) body=${API_BODY}"
fi
log "queue info HTTP 200"

ENQUEUE_URL="${GATEWAY_URL}/${PROJECT}/${VA}/enqueue"
wait_http_200 "${GATEWAY_URL}/${PROJECT}/${VA}/" 60
ENQ_BODY=$(jq -n --arg marker "$MARKER" '{ping:true, marker:$marker}')
ENQ_CODE=$(curl -sS -o /tmp/v10-enqueue.json -w '%{http_code}' -X POST \
  -H "Content-Type: application/json" -d "$ENQ_BODY" "$ENQUEUE_URL" || echo "000")
if [[ "$ENQ_CODE" != "202" && "$ENQ_CODE" != "200" ]]; then
  cat /tmp/v10-enqueue.json >&2 || true
  fail "Gateway POST /enqueue → HTTP ${ENQ_CODE} (worker send failed)"
fi
log "enqueued marker=${MARKER} via Gateway HTTP ${ENQ_CODE}"

api_status GET "${BASE_A}/queues/${QUEUE}/peek?limit=10"
PEEK_CODE="$API_STATUS"
if [[ "$PEEK_CODE" != "200" ]]; then
  echo "$API_BODY" >&2
  fail "GET …/peek → HTTP ${PEEK_CODE} (need T2 QueuePeek) body=${API_BODY}"
fi
PEEK_VA="$API_BODY"
if ! peek_has_marker "$PEEK_VA" "$MARKER"; then
  echo "$PEEK_VA" >&2
  fail "peek did not contain enqueued marker=${MARKER} (decode bodyBase64)"
fi
log "peek VA contains marker (bodyBase64 decodes)"

# Optional: pause then peek still visible (VALIDATION V10). OpenAPI: pause → 200.
api_status POST "${BASE_A}/queues/${QUEUE}/pause" "{}"
PAUSE_CODE="$API_STATUS"
if [[ "$PAUSE_CODE" == "200" ]]; then
  api_status GET "${BASE_A}/queues/${QUEUE}/peek?limit=10"
  if [[ "$API_STATUS" == "200" ]] && peek_has_marker "$API_BODY" "$MARKER"; then
    log "pause then peek still visible (OpenAPI pause 200)"
  else
    log "WARN: after pause, peek HTTP ${API_STATUS} missing marker (recorded in evidence)"
  fi
  api_status POST "${BASE_A}/queues/${QUEUE}/resume" "{}" || true
else
  log "WARN: POST pause → HTTP ${PAUSE_CODE} (optional; OpenAPI allows 200 for broker pause without consumer)"
fi

create_version "$PROJECT" "$VC" | jq -r .id >/dev/null
poll_version "$PROJECT" "$VC" ready 120 >/dev/null

api_status GET "${BASE_C}/queues/${QUEUE}/peek?limit=10"
CHILD_PEEK_CODE="$API_STATUS"
if [[ "$CHILD_PEEK_CODE" != "200" ]]; then
  echo "$API_BODY" >&2
  fail "GET child …/peek → HTTP ${CHILD_PEEK_CODE} body=${API_BODY}"
fi
if peek_has_marker "$API_BODY" "$MARKER"; then
  echo "$API_BODY" >&2
  fail "child VC peek contained parent marker — queue must not be shared (AD-7)"
fi
log "child VC peek does not contain VA marker (empty start)"

api_status POST "${BASE_A}/queues/${QUEUE}/purge" "{}"
PURGE_NOFORCE_CODE="$API_STATUS"
if [[ "$PURGE_NOFORCE_CODE" != "400" ]]; then
  echo "$API_BODY" >&2
  fail "POST purge without force expected 400, got HTTP ${PURGE_NOFORCE_CODE} body=${API_BODY}"
fi
log "purge without force → 400"

api_status POST "${BASE_A}/queues/${QUEUE}/purge" '{"force":true}'
PURGE_FORCE_CODE="$API_STATUS"
if [[ "$PURGE_FORCE_CODE" != "200" ]]; then
  echo "$API_BODY" >&2
  fail "POST purge {force:true} → HTTP ${PURGE_FORCE_CODE} (need T2 QueuePurge) body=${API_BODY}"
fi

api_status GET "${BASE_A}/queues/${QUEUE}/peek?limit=10"
if [[ "$API_STATUS" != "200" ]]; then
  fail "peek after purge → HTTP ${API_STATUS}"
fi
COUNT=$(peek_count "$API_BODY")
if [[ "$COUNT" != "0" ]] && peek_has_marker "$API_BODY" "$MARKER"; then
  echo "$API_BODY" >&2
  fail "peek after purge still has marker=${MARKER} count=${COUNT}"
fi
log "purge force → 200; peek count=${COUNT}"

EXIT_CODE=0
pass "V10 Queue info/peek/purge isolation OK VA=${VA} VC=${VC}"
cleanup_e2e_versions "$PROJECT" || true
exit 0
