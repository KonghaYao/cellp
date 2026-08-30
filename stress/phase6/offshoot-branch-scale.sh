#!/usr/bin/env bash
# TP-OB / TP-V0b-L — single-project large offshoot SQLite branch scale
#
# Store-only by default (offshoot CLI). Does not require cellpd.
# Env:
#   OB_SIZE_MB=100 OB_FANOUT=50 OB_CHAIN=20 OB_CONCURRENT=4 OB_CHECKOUT_N=8
#   OB_TIER=local|rustfs
#   OB_STORE=./dev/data/offshoot-scale-store  (local) or s3://cellp-offshoot/ob-scale/…
#   OB_SUITE=all|v0bl
set -euo pipefail

PHASE6_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck disable=SC1091
source "${PHASE6_ROOT}/stress/phase6/lib/common.sh"

scale_source_env

GOBIN="$(go env GOPATH 2>/dev/null)/bin"
export PATH="${GOBIN}:${PATH}"

OB_SIZE_MB="${OB_SIZE_MB:-100}"
OB_FANOUT="${OB_FANOUT:-50}"
OB_CHAIN="${OB_CHAIN:-20}"
OB_CONCURRENT="${OB_CONCURRENT:-4}"
OB_CHECKOUT_N="${OB_CHECKOUT_N:-8}"
OB_SUITE="${OB_SUITE:-all}"
OB_TIER="${OB_TIER:-local}"
OB_STORE="${OB_STORE:-}"
OB_METRICS="${OB_METRICS:-${PHASE6_ROOT}/docs/evidence/offshoot-branch-metrics.jsonl}"
OB_REPORT="${OB_REPORT:-${PHASE6_ROOT}/docs/evidence/offshoot-branch-scale-report.md}"
STRESS_RUN_ID="${STRESS_RUN_ID:-$(date +%Y%m%d-%H%M%S)}"
if [[ -z "$OB_STORE" ]]; then
  if [[ "$OB_TIER" == "rustfs" ]]; then
    OB_STORE="s3://cellp-offshoot/ob-scale/${STRESS_RUN_ID}"
  else
    OB_STORE="${PHASE6_ROOT}/dev/data/offshoot-scale-store"
  fi
fi
OB_DB="obscale-${STRESS_RUN_ID}"
WORKDIR="${PHASE6_ROOT}/dev/data/offshoot-scale-work/${STRESS_RUN_ID}"
SEED_DB="${WORKDIR}/seed.db"
EXPORT_DB="${WORKDIR}/export.db"

FAILS=0
NOTES=()

ob_log() { echo "[ob-scale $(date +%H:%M:%S)] $*" >&2; }

ob_fail() {
  ob_log "FAIL: $*"
  NOTES+=("FAIL: $*")
  FAILS=$((FAILS + 1))
}

ob_note() { NOTES+=("$*"); }

ob_now_ms() {
  python3 -c 'import time; print(int(time.time()*1000))'
}

ob_du_bytes() {
  local path="$1"
  if [[ "$path" == s3://* ]]; then
    echo 0
    return
  fi
  if [[ ! -e "$path" ]]; then
    echo 0
    return
  fi
  du -sk "$path" 2>/dev/null | awk '{print $1 * 1024}'
}

ob_checkout_du_bytes() {
  local dir="${OFFSHOOT_CHECKOUTS:-}"
  if [[ -z "$dir" || ! -e "$dir" ]]; then
    echo 0
    return
  fi
  du -sk "$dir" 2>/dev/null | awk '{print $1 * 1024}'
}

ob_setup_store_env() {
  if [[ "$OB_TIER" == "rustfs" ]]; then
    export AWS_ACCESS_KEY_ID="${RUSTFS_ACCESS_KEY:-${AWS_ACCESS_KEY_ID:-rustfsadmin}}"
    export AWS_SECRET_ACCESS_KEY="${RUSTFS_SECRET_KEY:-${AWS_SECRET_ACCESS_KEY:-rustfsadmin}}"
    export AWS_REGION="${AWS_REGION:-us-east-1}"
    export OFFSHOOT_S3_ENDPOINT="${OFFSHOOT_S3_ENDPOINT:-${S3_ENDPOINT:-http://127.0.0.1:19000}}"
    export OFFSHOOT_S3_PATH_STYLE="${OFFSHOOT_S3_PATH_STYLE:-1}"
    export OFFSHOOT_CHECKOUTS="${OFFSHOOT_CHECKOUTS:-${PHASE6_ROOT}/dev/data/offshoot-checkouts-rustfs-scale}"
    mkdir -p "$OFFSHOOT_CHECKOUTS"
    if ! curl -sf "${OFFSHOOT_S3_ENDPOINT}/minio/health/live" >/dev/null 2>&1 \
      && ! curl -sf "${OFFSHOOT_S3_ENDPOINT}/" >/dev/null 2>&1; then
      echo "FAIL: RustFS not reachable at ${OFFSHOOT_S3_ENDPOINT}" >&2
      exit 1
    fi
  fi
}

ob_metric_extra() {
  printf '%s' "${1:-{}}"
}

ob_file_bytes() {
  local path="$1"
  python3 -c 'import os,sys; print(os.path.getsize(sys.argv[1]) if os.path.isfile(sys.argv[1]) else 0)' "$path"
}

ob_record() {
  local test_id="$1"
  local metric="$2"
  local value extra payload tier_blob
  value="$(printf '%s' "$3" | tr -d '[:space:]')"
  extra='{}'
  if [[ $# -ge 4 && -n "${4:-}" ]]; then
    extra="$4"
  fi
  extra="$(printf '%s' "$extra" | tr -d '\n')"
  tier_blob="$(jq -nc --arg tier "$OB_TIER" --arg store "$OB_STORE" '{tier:$tier, store:$store}')"
  extra="$(jq -nc --argjson a "$extra" --argjson b "$tier_blob" '$a + $b')"
  mkdir -p "$(dirname "$OB_METRICS")" "$(dirname "$SCALE_METRICS")"
  payload="$(jq -nc \
    --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg test "$test_id" \
    --arg metric "$metric" \
    --arg value "$value" \
    --arg extra "$extra" \
    '{ts:$ts,test:$test,metric:$metric,value:($value|tonumber),extra:($extra|fromjson)}')" || {
    ob_log "jq failed test=$test_id metric=$metric value=$value extra=$extra"
    return 1
  }
  echo "$payload" >>"$OB_METRICS"
  echo "$payload" >>"$SCALE_METRICS"
}

ob() {
  offshoot -store "$OB_STORE" "$@"
}

ob_seed() {
  local mb="$1"
  local dest="$2"
  python3 - "$mb" "$dest" <<'PY'
import sqlite3, os, sys
mb = int(sys.argv[1])
dest = sys.argv[2]
if os.path.exists(dest):
    os.remove(dest)
con = sqlite3.connect(dest)
con.execute("PRAGMA journal_mode=OFF")
con.execute("PRAGMA synchronous=OFF")
con.execute("CREATE TABLE blobs(id INTEGER PRIMARY KEY, data BLOB NOT NULL)")
blob = os.urandom(1024 * 1024)
con.execute("BEGIN")
for _ in range(mb):
    con.execute("INSERT INTO blobs(data) VALUES (?)", (blob,))
con.execute("COMMIT")
con.close()
print(os.path.getsize(dest), end="")
PY
}

require_tools() {
  stress_need jq
  stress_need python3
  stress_need sqlite3
  if ! command -v offshoot >/dev/null; then
    echo "FAIL: offshoot CLI not on PATH (go install github.com/sricola/offshoot/cmd/offshoot@latest)" >&2
    exit 1
  fi
}

pct() {
  stress_percentile "$@"
}

write_report() {
  mkdir -p "$(dirname "$OB_REPORT")"
  local status="PASS"
  if (( FAILS > 0 )); then
    status="FAIL (${FAILS})"
  fi
  {
    echo "# Offshoot large-branch scale report"
    echo
    echo "> **Run ID:** ${STRESS_RUN_ID}  "
    echo "> **Date:** $(date -u +%Y-%m-%dT%H:%M:%SZ)  "
    echo "> **Suite:** ${OB_SUITE}  "
    echo "> **Tier:** ${OB_TIER}  "
    echo "> **Result:** ${status}"
    echo
    echo "## Environment"
    echo
    echo "| Item | Value |"
    echo "|------|-------|"
    echo "| offshoot | $(command -v offshoot) |"
    echo "| store | \`${OB_STORE}\` |"
    echo "| db | \`${OB_DB}\` |"
    echo "| size | ${OB_SIZE_MB} MB seed |"
    echo "| fan-out | ${OB_FANOUT} |"
    echo "| chain | ${OB_CHAIN} |"
    echo "| concurrent | ${OB_CONCURRENT} |"
    echo "| checkouts | ${OB_CHECKOUT_N} |"
    echo
    echo "## Notes"
    echo
    for n in "${NOTES[@]}"; do
      echo "- ${n}"
    done
    echo
    echo "## Metrics"
    echo
    echo "JSONL: \`docs/evidence/offshoot-branch-metrics.jsonl\` (this run appended)."
  } >"$OB_REPORT"
}

require_tools
ob_setup_store_env
mkdir -p "$WORKDIR"
if [[ "$OB_STORE" != s3://* ]]; then
  rm -rf "$OB_STORE"
  mkdir -p "$OB_STORE"
fi

ob_log "run=${STRESS_RUN_ID} tier=${OB_TIER} db=${OB_DB} size=${OB_SIZE_MB}MB store=${OB_STORE}"

# --- TP-OB-1 seed ---
t0="$(ob_now_ms)"
seed_bytes="$(ob_seed "$OB_SIZE_MB" "$SEED_DB")"
t1="$(ob_now_ms)"
seed_ms=$((t1 - t0))
ob_log "TP-OB-1 seeded ${seed_bytes} bytes in ${seed_ms}ms"
ob_record "TP-OB-1" "seed_bytes" "$seed_bytes" "{\"mb\":${OB_SIZE_MB},\"ms\":${seed_ms}}"
ob_record "TP-OB-1" "seed_ms" "$seed_ms" "{\"bytes\":${seed_bytes}}"
want=$((OB_SIZE_MB * 1024 * 1024))
if (( seed_bytes < want )); then
  ob_fail "seed file ${seed_bytes} < ${want}"
else
  ob_note "TP-OB-1 seed OK (${seed_bytes} bytes, ${seed_ms} ms)"
fi

ob init
t0="$(ob_now_ms)"
ob create "$OB_DB" --from "$SEED_DB"
t1="$(ob_now_ms)"
import_ms=$((t1 - t0))
ob_record "TP-OB-1" "import_ms" "$import_ms" "{}"
ob_log "create --from took ${import_ms}ms"
store_after_import="$(ob_du_bytes "$OB_STORE")"
ob_record "TP-OB-1" "store_bytes_after_import" "$store_after_import" "{}"

# --- TP-V0b-L / TP-OB export of one fork ---
t0="$(ob_now_ms)"
ob fork "$OB_DB" "v0bl"
t1="$(ob_now_ms)"
v0bl_fork_ms=$((t1 - t0))
t0="$(ob_now_ms)"
ob export "${OB_DB}@v0bl" "$EXPORT_DB" --force
t1="$(ob_now_ms)"
v0bl_export_ms=$((t1 - t0))
export_bytes="$(ob_file_bytes "$EXPORT_DB")"
ob_record "TP-V0b-L" "fork_ms" "$v0bl_fork_ms" "{}"
ob_record "TP-V0b-L" "export_ms" "$v0bl_export_ms" "{\"bytes\":${export_bytes}}"
ob_log "TP-V0b-L fork=${v0bl_fork_ms}ms export=${v0bl_export_ms}ms bytes=${export_bytes}"
if (( export_bytes < want )); then
  ob_fail "TP-V0b-L export ${export_bytes} < ${want}"
else
  ob_note "TP-V0b-L fork+export OK (fork ${v0bl_fork_ms} ms, export ${v0bl_export_ms} ms)"
fi

if [[ "$OB_SUITE" == "v0bl" ]]; then
  write_report
  if (( FAILS > 0 )); then
    exit 1
  fi
  echo "PASS: TP-V0b-L"
  exit 0
fi

# destroy the probe fork so fan-out counts stay clean
ob destroy "${OB_DB}@v0bl" --force || true
ob gc --grace 0s >/dev/null || true

store_baseline="$(ob_du_bytes "$OB_STORE")"
ob_record "TP-OB-2" "store_bytes_before_fanout" "$store_baseline" "{}"

# --- TP-OB-2 fan-out ---
declare -a fanout_ms=()
fanout_ok=0
for i in $(seq 1 "$OB_FANOUT"); do
  name="$(printf 'fan-%03d' "$i")"
  t0="$(ob_now_ms)"
  if ob fork "$OB_DB" "$name" >/dev/null; then
    t1="$(ob_now_ms)"
    d=$((t1 - t0))
    fanout_ms+=("$d")
    fanout_ok=$((fanout_ok + 1))
  else
    t1="$(ob_now_ms)"
    fanout_ms+=("$((t1 - t0))")
    ob_fail "fan-out fork ${name}"
  fi
done
p50="$(pct 50 "${fanout_ms[@]}")"
p95="$(pct 95 "${fanout_ms[@]}")"
p99="$(pct 99 "${fanout_ms[@]}")"
store_after_fanout="$(ob_du_bytes "$OB_STORE")"
added=$((store_after_fanout - store_baseline))
if (( fanout_ok > 0 )); then
  per_fork=$((added / fanout_ok))
else
  per_fork=0
fi
shared_n="$(ob status 2>/dev/null | grep -c 'storage=shared' || true)"
logical=$((seed_bytes * (1 + fanout_ok)))
cow_pct=0
if (( logical > 0 )); then
  cow_pct=$((store_after_fanout * 100 / logical))
fi
ob_log "TP-OB-2 fan-out ${fanout_ok}/${OB_FANOUT} p50=${p50} p95=${p95} p99=${p99} added=${added} per_fork=${per_fork} cow_pct=${cow_pct}% shared=${shared_n}"
ob_record "TP-OB-2" "fanout_ok" "$fanout_ok" "{\"requested\":${OB_FANOUT}}"
ob_record "TP-OB-2" "fork_p50_ms" "$p50" "{}"
ob_record "TP-OB-2" "fork_p95_ms" "$p95" "{}"
ob_record "TP-OB-2" "fork_p99_ms" "$p99" "{}"
ob_record "TP-OB-2" "store_bytes_after_fanout" "$store_after_fanout" "{\"added\":${added},\"per_fork\":${per_fork},\"cow_pct\":${cow_pct},\"shared\":${shared_n}}"

if (( fanout_ok != OB_FANOUT )); then
  ob_fail "fan-out ${fanout_ok}/${OB_FANOUT}"
else
  ob_note "TP-OB-2 fan-out ${fanout_ok}/${OB_FANOUT} p50=${p50}ms p99=${p99}ms"
fi

# CoW gate: added storage per fork should be << 10% of seed (local store only)
limit=$((seed_bytes / 10))
if [[ "$OB_TIER" == "rustfs" ]]; then
  if (( shared_n >= fanout_ok )); then
    ob_note "TP-OB-4 CoW OK (RustFS): ${shared_n} storage=shared branches after fan-out ${fanout_ok}"
  else
    ob_fail "RustFS CoW: shared=${shared_n} < fanout=${fanout_ok}"
  fi
  ob_record "TP-OB-4" "bytes_per_fork" "$per_fork" "{\"limit\":${limit},\"cow_pct\":${cow_pct},\"shared\":${shared_n},\"s3\":true}"
elif (( per_fork > limit )); then
  ob_fail "CoW broken: ${per_fork} bytes/fork > 10% of seed (${limit})"
  ob_record "TP-OB-4" "bytes_per_fork" "$per_fork" "{\"limit\":${limit},\"cow_pct\":${cow_pct}}"
else
  ob_note "TP-OB-4 CoW OK: ${per_fork} bytes/fork (limit ${limit}), store/logical=${cow_pct}%"
  ob_record "TP-OB-4" "bytes_per_fork" "$per_fork" "{\"limit\":${limit},\"cow_pct\":${cow_pct}}"
fi

# --- TP-OB-5 concurrent ---
conc_ok=0
declare -a conc_pids=()
declare -a conc_names=()
for i in $(seq 1 "$OB_CONCURRENT"); do
  name="conc-${i}"
  conc_names+=("$name")
  (
    if ob fork "$OB_DB" "$name" >/dev/null; then
      echo ok >"${WORKDIR}/${name}.ok"
    else
      echo fail >"${WORKDIR}/${name}.ok"
    fi
  ) &
  conc_pids+=("$!")
done
t0="$(ob_now_ms)"
for pid in "${conc_pids[@]}"; do
  wait "$pid" || true
done
t1="$(ob_now_ms)"
conc_wall=$((t1 - t0))
for name in "${conc_names[@]}"; do
  if [[ "$(cat "${WORKDIR}/${name}.ok" 2>/dev/null || true)" == "ok" ]]; then
    conc_ok=$((conc_ok + 1))
  fi
done
ob_record "TP-OB-5" "concurrent_ok" "$conc_ok" "{\"requested\":${OB_CONCURRENT},\"wall_ms\":${conc_wall}}"
ob_log "TP-OB-5 concurrent ${conc_ok}/${OB_CONCURRENT} wall=${conc_wall}ms"
if (( conc_ok != OB_CONCURRENT )); then
  ob_fail "concurrent fork ${conc_ok}/${OB_CONCURRENT}"
else
  ob_note "TP-OB-5 concurrent ${conc_ok}/${OB_CONCURRENT} in ${conc_wall} ms"
fi

# --- TP-OB-3 chain ---
declare -a chain_ms=()
chain_ok=0
parent="main"
for i in $(seq 1 "$OB_CHAIN"); do
  name="$(printf 'chain-%03d' "$i")"
  t0="$(ob_now_ms)"
  if ob fork "${OB_DB}@${parent}" "$name" >/dev/null; then
    t1="$(ob_now_ms)"
    chain_ms+=("$((t1 - t0))")
    chain_ok=$((chain_ok + 1))
    parent="$name"
  else
    t1="$(ob_now_ms)"
    chain_ms+=("$((t1 - t0))")
    ob_fail "chain fork ${name}"
    break
  fi
done
if (( ${#chain_ms[@]} > 0 )); then
  chain_p50="$(pct 50 "${chain_ms[@]}")"
  chain_p99="$(pct 99 "${chain_ms[@]}")"
  chain_first="${chain_ms[0]}"
  chain_last="${chain_ms[$((${#chain_ms[@]} - 1))]}"
else
  chain_p50=0
  chain_p99=0
  chain_first=0
  chain_last=0
fi
store_after_chain="$(ob_du_bytes "$OB_STORE")"
ob_record "TP-OB-3" "chain_ok" "$chain_ok" "{\"requested\":${OB_CHAIN},\"p50\":${chain_p50},\"p99\":${chain_p99},\"first_ms\":${chain_first},\"last_ms\":${chain_last}}"
ob_log "TP-OB-3 chain ${chain_ok}/${OB_CHAIN} first=${chain_first} last=${chain_last} p99=${chain_p99}"
if (( chain_ok != OB_CHAIN )); then
  ob_fail "chain ${chain_ok}/${OB_CHAIN}"
else
  ob_note "TP-OB-3 chain ${chain_ok}/${OB_CHAIN} first=${chain_first}ms last=${chain_last}ms"
fi
if (( chain_first > 0 && chain_last > chain_first * 10 )); then
  ob_fail "chain last fork ${chain_last}ms > 10x first ${chain_first}ms"
fi

# --- TP-OB checkout materialization ---
checkout_ok=0
declare -a checkout_ms=()
du_before_co="$(ob_checkout_du_bytes)"
for i in $(seq 1 "$OB_CHECKOUT_N"); do
  name="$(printf 'fan-%03d' "$i")"
  t0="$(ob_now_ms)"
  if ob checkout "${OB_DB}@${name}" >/dev/null; then
    t1="$(ob_now_ms)"
    checkout_ms+=("$((t1 - t0))")
    checkout_ok=$((checkout_ok + 1))
  else
    t1="$(ob_now_ms)"
    checkout_ms+=("$((t1 - t0))")
    ob_fail "checkout ${name}"
  fi
done
du_after_co="$(ob_checkout_du_bytes)"
co_added=$((du_after_co - du_before_co))
if (( ${#checkout_ms[@]} > 0 )); then
  co_p50="$(pct 50 "${checkout_ms[@]}")"
  co_p99="$(pct 99 "${checkout_ms[@]}")"
else
  co_p50=0
  co_p99=0
fi
ob_record "TP-OB-6a" "checkout_ok" "$checkout_ok" "{\"requested\":${OB_CHECKOUT_N},\"p50\":${co_p50},\"p99\":${co_p99},\"du_added\":${co_added}}"
ob_log "checkout ${checkout_ok}/${OB_CHECKOUT_N} p50=${co_p50} p99=${co_p99} du_added=${co_added}"
ob_note "checkout ${checkout_ok}/${OB_CHECKOUT_N} p50=${co_p50}ms du_added=${co_added} bytes"

# --- TP-OB-6 dump pipeline (SQL materialization cost on an 8MB sample) ---
DUMP_SRC="${WORKDIR}/dump-sample.db"
DUMP_SQL="${WORKDIR}/dump-sample.sql"
if (( OB_SIZE_MB > 8 )); then
  ob_seed 8 "$DUMP_SRC" >/dev/null
else
  DUMP_SRC="$SEED_DB"
fi
t0="$(ob_now_ms)"
if sqlite3 "$DUMP_SRC" ".dump" >"$DUMP_SQL"; then
  t1="$(ob_now_ms)"
  dump_ms=$((t1 - t0))
  dump_bytes="$(ob_file_bytes "$DUMP_SQL")"
  dump_db_bytes="$(ob_file_bytes "$DUMP_SRC")"
  ob_record "TP-OB-6" "sqlite_dump_ms" "$dump_ms" "{\"sql_bytes\":${dump_bytes},\"db_bytes\":${dump_db_bytes}}"
  ob_log "sqlite3 .dump ${dump_ms}ms sql_bytes=${dump_bytes} (sample db ${dump_db_bytes})"
  ob_note "TP-OB-6 sqlite3 .dump ${dump_ms} ms → ${dump_bytes} bytes SQL from ${dump_db_bytes} byte sample. celld d1 --file needs SQL; a 100MB incompressible dump is ~2× hex and is not executed against a live fleet in this harness."
else
  ob_fail "sqlite3 .dump"
fi

# D1 execute requires a running celld fleet; record skip unless explicitly forced.
if [[ "${OB_TRY_D1:-0}" == "1" ]] && command -v celld >/dev/null; then
  ob_log "OB_TRY_D1=1 — attempting celld d1 execute (likely fails without fleet)"
  if celld d1 execute DB --file "$DUMP_SQL" --bucket "${CELLD_BUCKET:-s3://cellp-celld/demo-app}" \
    --endpoint "${S3_ENDPOINT:-http://127.0.0.1:19000}" --region "${AWS_REGION:-us-east-1}" \
    >/tmp/ob-d1.out 2>&1; then
    ob_note "TP-OB-6 D1 execute unexpectedly succeeded"
    ob_record "TP-OB-6" "d1_execute" 1 "{}"
  else
    ob_note "TP-OB-6 D1 execute failed without fleet (expected): $(head -c 200 /tmp/ob-d1.out | tr '\n' ' ')"
    ob_record "TP-OB-6" "d1_execute" 0 "{}"
  fi
else
  ob_note "TP-OB-6 D1 execute skipped (needs running celld fleet); dump cost recorded"
  ob_record "TP-OB-6" "d1_execute" -1 "{\"reason\":\"no-fleet\"}"
fi

# --- TP-OB-7 destroy + gc ---
du_before_gc="$(ob_du_bytes "$OB_STORE")"
status_before="$(ob status 2>/dev/null | grep -c '@' || true)"
t0="$(ob_now_ms)"
while read -r ref; do
  [[ -z "$ref" ]] && continue
  [[ "$ref" == "${OB_DB}@main" ]] && continue
  ob destroy "$ref" --force >/dev/null 2>&1 || true
done < <(ob status 2>/dev/null | awk '$1 ~ /@/ {print $1}')
ob gc --grace 0s >/dev/null || true
ob gc --grace 0s >/dev/null || true
t1="$(ob_now_ms)"
gc_ms=$((t1 - t0))
du_after_gc="$(ob_du_bytes "$OB_STORE")"
shared_after="$(ob status 2>/dev/null | grep -c 'storage=shared' || true)"
ob_record "TP-OB-7" "gc_ms" "$gc_ms" "{\"du_before\":${du_before_gc},\"du_after\":${du_after_gc},\"shared_after\":${shared_after}}"
ob_log "TP-OB-7 gc ${gc_ms}ms du ${du_before_gc} -> ${du_after_gc} shared_after=${shared_after}"
if (( shared_after != 0 )); then
  ob_fail "TP-OB-7 leftover shared branches=${shared_after} (gc grace may still hold objects)"
else
  ob_note "TP-OB-7 destroy+gc OK in ${gc_ms} ms, store ${du_after_gc} bytes, no shared leftovers"
fi

# --- Platform cap note ---
if curl -sf "${PLATFORM_URL:-http://127.0.0.1:8790}/v1/health" >/dev/null 2>&1; then
  ob_note "cellpd API up; ready version count has no hard cap (AD-9). Not deploying ${OB_SIZE_MB}MB into celld fleet."
  ob_record "TP-OB-P" "api_up" 1 "{}"
else
  ob_note "cellpd API down — platform live track skipped (store-only is the many-branch scenario)."
  ob_record "TP-OB-P" "api_up" 0 "{}"
fi

write_report
ob_log "report ${OB_REPORT}"
if (( FAILS > 0 )); then
  echo "FAIL: offshoot branch scale (${FAILS} gates)" >&2
  exit 1
fi
echo "PASS: offshoot branch scale run ${STRESS_RUN_ID}"
exit 0
