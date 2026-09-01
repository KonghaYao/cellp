#!/usr/bin/env bash
# Seed demo-app v1 + v2 for Dashboard acceptance (bindings + fake data).
# Usage: ./dev/scripts/seed-demo.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

# shellcheck disable=SC1091
source dev/.env
# shellcheck disable=SC1091
source e2e/scripts/lib.sh

PROJECT="${DEV_PROJECT:-demo-app}"
V1="v1"
V2="v2"
EXAMPLE="${ROOT}/dev/examples/bindings-demo"
DEST_V1="${ARTIFACTS_DIR}/${PROJECT}/${V1}"
DEST_V2="${ARTIFACTS_DIR}/${PROJECT}/${V2}"

export PATH="${ROOT}/web/node_modules/.bin:${ROOT}/dev/examples/counter/node_modules/.bin:${PATH}"

need() { command -v "$1" >/dev/null || { echo "MISSING: $1" >&2; exit 1; }; }
need sqlite3
need python3

require_platform
require_celld

log() { echo "==> $*"; }

urlencode_path() {
  python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1], safe=''))" "$1"
}

require_operator_api() {
  api_status GET "/v1/projects/${PROJECT}/versions/__health_probe__/kv/"
  if [[ "$API_STATUS" == "404" && "$API_BODY" == *"page not found"* ]]; then
    fail "cellpd missing operator API (KV routes). Fix: ./dev/scripts/build-cellpd.sh && ./dev/scripts/up.sh"
  fi
  [[ "$API_STATUS" == "404" && "$API_BODY" == *"version_not_found"* ]] \
    || fail "operator API probe unexpected (HTTP ${API_STATUS}): ${API_BODY}"
}

purge_destroyed_ids() {
  local db="${CELLP_REGISTRY_DB:-${REGISTRY_DB:-./dev/data/cellp-registry.sqlite}}"
  [[ "$db" != /* ]] && db="${ROOT}/${db#./}"
  sqlite3 "$db" "DELETE FROM versions WHERE project_id='${PROJECT}' AND id IN ('${V1}','${V2}') AND status='destroyed';" 2>/dev/null || true
}

destroy_version_if_exists() {
  local vid="$1"
  local status
  status=$(api_get "/v1/projects/${PROJECT}/versions/${vid}" 2>/dev/null | jq -r .status 2>/dev/null || echo "")
  [[ -z "$status" || "$status" == "null" ]] && return 0
  if [[ "$status" == "destroyed" ]]; then
    log "skip destroy ${vid} (already destroyed)"
    return 0
  fi
  log "destroy existing ${vid} (status=${status})"
  api_delete "/v1/projects/${PROJECT}/versions/${vid}" "$ADMIN_TOKEN" >/dev/null 2>&1 || true
  for _ in $(seq 1 60); do
    status=$(api_get "/v1/projects/${PROJECT}/versions/${vid}" 2>/dev/null | jq -r .status 2>/dev/null || echo "")
    [[ "$status" == "destroyed" ]] && return 0
    sleep 1
  done
  fail "timeout destroying ${vid} (last status=${status})"
}

write_guestbook_seed() {
  local db="$1"
  rm -f "$db" "${db}-wal" "${db}-shm"
  sqlite3 "$db" <<'SQL' >/dev/null
PRAGMA journal_mode = DELETE;
CREATE TABLE entries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  message TEXT NOT NULL,
  at INTEGER NOT NULL
);
SQL
  local i now
  now=$(date +%s)
  for i in $(seq 1 24); do
    sqlite3 "$db" "INSERT INTO entries (name, message, at) VALUES ('guest-${i}', 'Bindings demo seed row ${i}', $((now - i)));"
  done
  rm -f "${db}-wal" "${db}-shm"
  local n
  n=$(sqlite3 "$db" "SELECT count(*) FROM entries;")
  log "guestbook seed.db rows=${n}"
}

require_kv_api() {
  local vid="${1:-$V1}"
  api_status GET "/v1/projects/${PROJECT}/versions/${vid}/kv/"
  if [[ "$API_STATUS" == "404" && "$API_BODY" == *"page not found"* ]]; then
    fail "cellpd missing KV operator API (HTTP 404 page not found). Rebuild and restart: ./dev/scripts/build-cellpd.sh && ./dev/scripts/up.sh"
  fi
  [[ "$API_STATUS" == "200" ]] || fail "KV API not ready (GET …/kv/ → HTTP ${API_STATUS}): ${API_BODY}"
}

seed_kv_v1() {
  require_kv_api "$V1"
  local base="/v1/projects/${PROJECT}/versions/${V1}"
  local put_body
  put_body=$(jq -n --arg v "hello-prod" '{value:$v}')
  api_status PUT "${base}/kv/demo-app-cache/keys/greeting" "$put_body"
  [[ "$API_STATUS" == "200" || "$API_STATUS" == "204" ]] \
    || fail "KV PUT greeting → HTTP ${API_STATUS} ${API_BODY}"
  put_body=$(jq -n --arg v "demo-session-42" '{value:$v}')
  api_status PUT "${base}/kv/demo-app-cache/keys/$(urlencode_path 'session:demo')" "$put_body"
  [[ "$API_STATUS" == "200" || "$API_STATUS" == "204" ]] \
    || fail "KV PUT session:demo → HTTP ${API_STATUS} ${API_BODY}"
  put_body=$(jq -n --arg v "Bindings demo seed" '{value:$v}')
  api_status PUT "${base}/kv/demo-app-cache/keys/welcome-banner" "$put_body"
  [[ "$API_STATUS" == "200" || "$API_STATUS" == "204" ]] \
    || fail "KV PUT welcome-banner → HTTP ${API_STATUS} ${API_BODY}"
  log "KV seeded via platform API (greeting, session:demo, welcome-banner)"
}

seed_queue_v1() {
  local i body code
  for i in 1 2 3; do
    body=$(jq -n --arg m "demo-task-${i}" --argjson n "$i" --argjson at "$(date +%s)" '{task:$m, n:$n, at:$at}')
    code=$(curl -s -o /dev/null -w '%{http_code}' -X POST \
      -H "Host: $(preview_host "$PROJECT" "$V1")" -H "Content-Type: application/json" \
      -d "$body" "${GATEWAY_URL}/enqueue" 2>/dev/null || echo "000")
    [[ "$code" == "202" || "$code" == "200" ]] || fail "enqueue ${i} → HTTP ${code}"
  done
  log "queue seeded (3 messages via Worker /enqueue)"
}

seed_workflow_v1() {
  local code body
  code=$(curl -s -o /tmp/cellp-wf-create.json -w '%{http_code}' \
    -H "Host: $(preview_host "$PROJECT" "$V1")" \
    "${GATEWAY_URL}/create?url=https://example.com" 2>/dev/null || echo "000")
  if [[ "$code" == "200" ]]; then
    log "workflow instance created id=$(jq -r .id /tmp/cellp-wf-create.json 2>/dev/null || echo '?')"
  else
    log "WARN: workflow /create → HTTP ${code} (instances list may be empty)"
  fi
}

promote_v1() {
  api_status POST "/v1/projects/${PROJECT}/versions/${V1}/promote" "{}" "$ADMIN_TOKEN"
  [[ "$API_STATUS" == "200" ]] || fail "promote v1 → HTTP ${API_STATUS} ${API_BODY}"
  log "v1 promoted to production"
}

print_summary() {
  echo ""
  echo "Demo ready for Dashboard acceptance:"
  echo "  Project:   ${PROJECT}"
  echo "  Versions:  ${V1} (prod, seeded) · ${V2} (preview, AD-7 empty KV/queue)"
  echo "  Dashboard: http://127.0.0.1:${DASHBOARD_PORT:-5190}/projects/${PROJECT}/storage"
  echo "  KV:        http://127.0.0.1:${DASHBOARD_PORT:-5190}/projects/${PROJECT}/storage/${V1}/kv"
  echo "  Queues:    http://127.0.0.1:${DASHBOARD_PORT:-5190}/projects/${PROJECT}/storage/${V1}/queues"
  echo "  Workflows: http://127.0.0.1:${DASHBOARD_PORT:-5190}/projects/${PROJECT}/storage/${V1}/workflows"
  echo "  D1:        http://127.0.0.1:${DASHBOARD_PORT:-5190}/projects/${PROJECT}/storage/${V1}/browser"
  echo "  Gateway (Host / AD-12):"
  echo "    prod:    curl -H \"Host: $(prod_host "$PROJECT")\" ${GATEWAY_URL}/"
  echo "    preview: curl -H \"Host: $(preview_host "$PROJECT" "$V2")\" ${GATEWAY_URL}/"
  echo "  /etc/hosts: 127.0.0.1 $(prod_host "$PROJECT") $(preview_host "$PROJECT" "$V1") $(preview_host "$PROJECT" "$V2")"
  echo ""
}

# --- main ---
log "seed demo project=${PROJECT}"
require_operator_api

if [[ ! -d "$EXAMPLE" ]]; then
  fail "missing ${EXAMPLE}"
fi

ensure_project "$PROJECT"
destroy_version_if_exists "$V2"
destroy_version_if_exists "$V1"
purge_destroyed_ids

log "stage ${V1} (bindings-demo + guestbook seed)"
stage_worker_example "$EXAMPLE" "$DEST_V1"
write_guestbook_seed "${DEST_V1}/seed.db"

create_version "$PROJECT" "$V1" | jq -r .id >/dev/null
poll_version "$PROJECT" "$V1" ready 180 >/dev/null
log "${V1} ready"

seed_kv_v1
seed_queue_v1
seed_workflow_v1
promote_v1

log "stage ${V2} (preview child — empty KV/queue per AD-7)"
stage_worker_example "$EXAMPLE" "$DEST_V2"
rm -f "${DEST_V2}/seed.db"

create_version "$PROJECT" "$V2" "$V1" | jq -r .id >/dev/null
poll_version "$PROJECT" "$V2" ready 180 >/dev/null
log "${V2} ready"

# Sanity checks
BASE="/v1/projects/${PROJECT}/versions/${V1}"
api_status GET "${BASE}/bindings"
echo "$API_BODY" | jq -e '(.kv|length)>0 and (.queues|length)>0 and (.workflows|length)>0 and (.d1|length)>0 and (.crons|length)>0' >/dev/null \
  || fail "v1 bindings incomplete: $(echo "$API_BODY" | jq -c .)"

api_status GET "${BASE}/kv/demo-app-cache/keys/greeting"
GOT=$(echo "$API_BODY" | jq -r '.value // empty')
[[ "$GOT" == "hello-prod" ]] || fail "KV greeting want hello-prod got ${GOT}"

api_status GET "${BASE}/database/tables/entries/rows?limit=1"
D1_ROWS=$(echo "$API_BODY" | jq '.rows | length' 2>/dev/null || echo 0)
[[ "$D1_ROWS" -ge 1 ]] || fail "D1 entries empty after deploy (orchestrator seed.db import)"

api_status GET "${BASE}/queues/tasks/peek"
PEEK_N=$(echo "$API_BODY" | jq '.messages | length' 2>/dev/null || echo 0)
log "queue peek messages=${PEEK_N}"

print_summary
pass "demo-app ${V1}/${V2} seeded"
