#!/usr/bin/env bash
# Commerce production workflow: complex D1 worker + parent import + child branch + load test.
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/../../e2e/scripts/lib.sh"

require_platform
require_offshoot
require_celld_cli
need sqlite3

rustfs_s3_env

COMMERCE_EXAMPLE="${E2E_ROOT}/dev/examples/commerce"
PROJECT="${DEV_PROJECT:-demo-app}"
PARENT="$(unique_id)"
CHILD="$(unique_id)"
DATABASE="commerce"
PARENT_DIR="${ARTIFACTS_DIR}/${PROJECT}/${PARENT}"
CHILD_DIR="${ARTIFACTS_DIR}/${PROJECT}/${CHILD}"
EXPORT="${PARENT_DIR}/seed.db"
STAMP="$(date +%Y%m%d-%H%M%S)"
REPORT="${EVIDENCE_DIR}/commerce-prod-${STAMP}.json"
LOG="${EVIDENCE_DIR}/commerce-prod-${STAMP}.log"

SEED_PRODUCTS="${COMMERCE_SEED_PRODUCTS:-200}"
SEED_CUSTOMERS="${COMMERCE_SEED_CUSTOMERS:-1000}"
SEED_ORDERS_N="${COMMERCE_SEED_ORDERS:-800}"

# Load test knobs
LOAD_DURATION="${COMMERCE_LOAD_DURATION:-30}"
LOAD_PHASES="${COMMERCE_LOAD_PHASES:-10,25,50,100}"
LOAD_RPS="${COMMERCE_LOAD_RPS:-5}"

copy_commerce_bundle() {
  local dest="$1"
  mkdir -p "$dest"
  cp "${COMMERCE_EXAMPLE}/index.js" "${dest}/index.js"
  cp "${COMMERCE_EXAMPLE}/wrangler.jsonc" "${dest}/wrangler.jsonc"
}

seed_stats() {
  sqlite3 "$1" "SELECT
    (SELECT count(*) FROM customers) || '|' ||
    (SELECT count(*) FROM products) || '|' ||
    (SELECT count(*) FROM orders) || '|' ||
    (SELECT count(*) FROM order_items) || '|' ||
    (SELECT coalesce(sum(qty),0) FROM inventory);"
}

log "commerce prod workflow project=${PROJECT} parent=${PARENT} child=${CHILD}" | tee -a "$LOG"

if ! celld d1 import --help >/dev/null 2>&1; then
  fail "celld d1 import not available"
fi
if ! celld d1 branch -h >/dev/null 2>&1; then
  fail "celld d1 branch not available"
fi

ensure_project "$PROJECT"
cleanup_e2e_versions "$PROJECT"
mkdir -p "$EVIDENCE_DIR" "$PARENT_DIR"

copy_commerce_bundle "$PARENT_DIR"
chmod +x "${COMMERCE_EXAMPLE}/seed.sh"

offshoot -store "$OFFSHOOT_STORE" init >>"$LOG" 2>&1 || true
offshoot -store "$OFFSHOOT_STORE" create "$PROJECT" >>"$LOG" 2>&1 || true
CHECKOUT=$(offshoot -store "$OFFSHOOT_STORE" checkout "${PROJECT}@main")
if [[ -z "$CHECKOUT" || ! -f "$CHECKOUT" ]]; then
  fail "offshoot checkout failed"
fi
"${COMMERCE_EXAMPLE}/seed.sh" "$CHECKOUT" "$SEED_PRODUCTS" "$SEED_CUSTOMERS" "$SEED_ORDERS_N" >>"$LOG" 2>&1
offshoot -store "$OFFSHOOT_STORE" checkpoint "${PROJECT}@main" "commerce-${PARENT}" >>"$LOG" 2>&1

IFS='|' read -r SEED_CUSTOMERS SEED_PRODUCTS SEED_ORDERS SEED_ITEMS SEED_INV <<<"$(seed_stats "$CHECKOUT")"
log "offshoot main seed customers=${SEED_CUSTOMERS} products=${SEED_PRODUCTS} orders=${SEED_ORDERS} items=${SEED_ITEMS} inventory_units=${SEED_INV}"

# Sanity export before deploy (orchestrator re-exports from forked version branch).
offshoot -store "$OFFSHOOT_STORE" export "${PROJECT}@main" "$EXPORT" --force >>"$LOG" 2>&1 || true
rm -f "${EXPORT}-wal" "${EXPORT}-shm"
verify=$(sqlite3 "$EXPORT" "SELECT count(*) FROM customers;" 2>/dev/null || echo "0")
if [[ "$verify" != "$SEED_CUSTOMERS" ]]; then
  fail "offshoot export verify customers=${verify} expected ${SEED_CUSTOMERS}"
fi

# --- parent deploy (D1 import) ---
create_version "$PROJECT" "$PARENT" | jq -r .id >/dev/null
poll_version "$PROJECT" "$PARENT" ready 300 >/dev/null

PARENT_BASE="${GATEWAY_URL}/${PROJECT}/${PARENT}"
wait_http_200 "${PARENT_BASE}/health" 90
PARENT_STATS=$(curl -sf "${PARENT_BASE}/stats")
log "parent stats: $(echo "$PARENT_STATS" | jq -c .)"

pc=$(echo "$PARENT_STATS" | jq -r '.customers // 0')
if [[ "$pc" != "$SEED_CUSTOMERS" ]]; then
  fail "parent customers=${pc} expected ${SEED_CUSTOMERS}"
fi

# --- child deploy (D1 branch) ---
copy_commerce_bundle "$CHILD_DIR"
create_version "$PROJECT" "$CHILD" "$PARENT" | jq -r .id >/dev/null
poll_version "$PROJECT" "$CHILD" ready 300 >/dev/null

CHILD_BASE="${GATEWAY_URL}/${PROJECT}/${CHILD}"
wait_http_200 "${CHILD_BASE}/health" 90
CHILD_STATS=$(curl -sf "${CHILD_BASE}/stats")
cc=$(echo "$CHILD_STATS" | jq -r '.customers // 0')
if [[ "$cc" != "$SEED_CUSTOMERS" ]]; then
  fail "child branch customers=${cc} expected ${SEED_CUSTOMERS}"
fi
log "child branch stats OK"

# --- branch isolation: write on child only ---
ORDER_BODY=$(jq -n --argjson cid 1 '{customer_id:$cid, items:[{product_id:1, qty:1}]}')
CHILD_ORDER=$(curl -sf -X POST "${CHILD_BASE}/orders" -H "Content-Type: application/json" -d "$ORDER_BODY")
CHILD_ORDER_ID=$(echo "$CHILD_ORDER" | jq -r '.order_id // empty')
if [[ -z "$CHILD_ORDER_ID" ]]; then
  fail "child POST /orders failed: $CHILD_ORDER"
fi
CHILD_AFTER=$(curl -sf "${CHILD_BASE}/stats" | jq -r '.orders')
PARENT_AFTER=$(curl -sf "${PARENT_BASE}/stats" | jq -r '.orders')
if [[ "$CHILD_AFTER" != "$((SEED_ORDERS + 1))" ]]; then
  fail "child orders after write=${CHILD_AFTER} expected $((SEED_ORDERS + 1))"
fi
if [[ "$PARENT_AFTER" != "$SEED_ORDERS" ]]; then
  fail "parent isolation broken orders=${PARENT_AFTER} expected ${SEED_ORDERS}"
fi
log "branch isolation OK (child order ${CHILD_ORDER_ID})"

# --- load test on child (mixed read/write) ---
run_load_phase() {
  local workers="$1"
  local rps="$2"
  local duration="$3"
  local base="$4"
  local result_dir
  result_dir=$(mktemp -d)
  local start end elapsed

  start=$SECONDS
  for w in $(seq 1 "$workers"); do
    (
      local ok=0 fail=0
      local deadline=$((SECONDS + duration))
      while (( SECONDS < deadline )); do
        local roll=$((RANDOM % 100))
        local code
        if (( roll < 70 )); then
          code=$(curl -s -o /dev/null -w '%{http_code}' "${base}/stats" 2>/dev/null || echo "000")
        elif (( roll < 90 )); then
          code=$(curl -s -o /dev/null -w '%{http_code}' "${base}/products?limit=10" 2>/dev/null || echo "000")
        else
          local body
          body=$(jq -n --argjson cid $(( (RANDOM % 100) + 1 )) --argjson pid $(( (RANDOM % 50) + 1 )) \
            '{customer_id:$cid, items:[{product_id:$pid, qty:1}]}')
          code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "${base}/orders" \
            -H "Content-Type: application/json" -d "$body" 2>/dev/null || echo "000")
        fi
        if [[ "$code" == "200" || "$code" == "201" ]]; then
          ok=$((ok + 1))
        else
          fail=$((fail + 1))
        fi
        sleep "$(awk -v rps="$rps" 'BEGIN { printf "%.4f", 1/rps }')"
      done
      echo "${ok} ${fail}" >"${result_dir}/w${w}"
    ) &
  done
  wait
  end=$SECONDS
  elapsed=$((end - start))
  [[ "$elapsed" -lt 1 ]] && elapsed=1

  local total_ok=0 total_fail=0
  for w in $(seq 1 "$workers"); do
    read -r ok fail <"${result_dir}/w${w}"
    total_ok=$((total_ok + ok))
    total_fail=$((total_fail + fail))
  done
  rm -rf "$result_dir"

  local total=$((total_ok + total_fail))
  local err_pct=0
  if (( total > 0 )); then
    err_pct=$(( total_fail * 100 / total ))
  fi
  local achieved_rps=$(( total_ok / elapsed ))
  echo "${workers}|${rps}|${duration}|${total_ok}|${total_fail}|${err_pct}|${achieved_rps}|${elapsed}"
}

LOAD_RESULTS=()
IFS=',' read -ra PHASES <<<"$LOAD_PHASES"
for workers in "${PHASES[@]}"; do
  log "load phase workers=${workers} rps/worker=${LOAD_RPS} duration=${LOAD_DURATION}s"
  line=$(run_load_phase "$workers" "$LOAD_RPS" "$LOAD_DURATION" "$CHILD_BASE")
  LOAD_RESULTS+=("$line")
  log "  result: $line (ok|fail|err%|achieved_rps|elapsed)"
done

FINAL_STATS=$(curl -sf "${CHILD_BASE}/stats")
DB_META=$(api_get "/v1/projects/${PROJECT}/versions/${CHILD}/database" 2>/dev/null || echo '{}')

jq -n \
  --arg stamp "$STAMP" \
  --arg project "$PROJECT" \
  --arg parent "$PARENT" \
  --arg child "$CHILD" \
  --argjson seed "$(jq -n \
    --argjson customers "$SEED_CUSTOMERS" \
    --argjson products "$SEED_PRODUCTS" \
    --argjson orders "$SEED_ORDERS" \
    --argjson items "$SEED_ITEMS" \
    --argjson inventory "$SEED_INV" \
    '{customers:$customers,products:$products,orders:$orders,order_items:$items,inventory_units:$inventory}')" \
  --argjson parent_stats "$PARENT_STATS" \
  --argjson child_stats "$CHILD_STATS" \
  --argjson final_stats "$FINAL_STATS" \
  --argjson db_meta "$DB_META" \
  --arg load_results "$(printf '%s\n' "${LOAD_RESULTS[@]}")" \
  --argjson load_duration "$LOAD_DURATION" \
  --argjson load_rps "$LOAD_RPS" \
  '{
    stamp: $stamp,
    workflow: "commerce-prod",
    project: $project,
    parent_version: $parent,
    child_version: $child,
    seed: $seed,
    parent_stats: $parent_stats,
    child_stats: $child_stats,
    final_stats: $final_stats,
    database: $db_meta,
    load: {
      duration_s: $load_duration,
      rps_per_worker: $load_rps,
      phases: ($load_results | split("\n") | map(select(length>0)) | map(split("|") | {
        workers: (.[0]|tonumber),
        target_rps_per_worker: (.[1]|tonumber),
        duration_s: (.[2]|tonumber),
        ok: (.[3]|tonumber),
        fail: (.[4]|tonumber),
        error_pct: (.[5]|tonumber),
        achieved_ok_rps: (.[6]|tonumber),
        elapsed_s: (.[7]|tonumber)
      }))
    },
    branch_isolation: {
      child_order_id: ($child_stats.orders + 1),
      parent_orders_unchanged: ($parent_stats.orders == $seed.orders)
    }
  }' >"$REPORT"

log "report written: $REPORT"
cat "$REPORT" | jq '{seed, branch_isolation, load: .load.phases, final_stats, database: {database_id, branch_method, status}}'

pass "commerce prod workflow parent=${PARENT} child=${CHILD} report=${REPORT}"
