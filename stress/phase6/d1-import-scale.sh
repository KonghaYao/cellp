#!/usr/bin/env bash
# TP-D1-IMP / G6 — 100 MB incompressible SQLite → celld d1 import (no .dump.sql)
#
# Requires: running celld fleet, wrangler project with d1_databases, celld d1 import CLI (T1).
# Skips cleanly (exit 0) when `celld d1 import` is missing from an older celld build.
#
# Env:
#   D1_IMPORT_SIZE_MB=100
#   D1_DATABASE=guestbook (default from wrangler database_name)
#   D1_METRICS=docs/evidence/d1-import-metrics.jsonl
#   D1_REPORT=docs/evidence/d1-import-scale-report.md
set -euo pipefail

PHASE6_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck disable=SC1091
source "${PHASE6_ROOT}/stress/phase6/lib/common.sh"

scale_source_env

GOBIN="$(go env GOPATH 2>/dev/null)/bin"
export PATH="${GOBIN}:${PATH}"

D1_IMPORT_SIZE_MB="${D1_IMPORT_SIZE_MB:-100}"
D1_METRICS="${D1_METRICS:-${PHASE6_ROOT}/docs/evidence/d1-import-metrics.jsonl}"
D1_REPORT="${D1_REPORT:-${PHASE6_ROOT}/docs/evidence/d1-import-scale-report.md}"
STRESS_RUN_ID="${STRESS_RUN_ID:-$(date +%Y%m%d-%H%M%S)}"
CELLD_PORT="${CELLD_PORT:-8792}"
WORKDIR="${PHASE6_ROOT}/dev/data/d1-import-scale/${STRESS_RUN_ID}"
SEED_DB="${WORKDIR}/seed.db"
DUMP_SQL="${WORKDIR}/seed.dump.sql"

FAILS=0
NOTES=()

d1_log() { echo "[d1-import-scale $(date +%H:%M:%S)] $*" >&2; }

d1_fail() {
  d1_log "FAIL: $*"
  NOTES+=("FAIL: $*")
  FAILS=$((FAILS + 1))
}

d1_note() { NOTES+=("$*"); }

d1_now_ms() {
  python3 -c 'import time; print(int(time.time()*1000))'
}

d1_file_bytes() {
  local path="$1"
  python3 -c 'import os,sys; print(os.path.getsize(sys.argv[1]) if os.path.isfile(sys.argv[1]) else 0)' "$path"
}

d1_record() {
  local test_id="$1"
  local metric="$2"
  local value extra payload
  value="$(printf '%s' "$3" | tr -d '[:space:]')"
  extra='{}'
  if [[ $# -ge 4 && -n "${4:-}" ]]; then
    extra="$4"
  fi
  extra="$(printf '%s' "$extra" | tr -d '\n')"
  mkdir -p "$(dirname "$D1_METRICS")" "$(dirname "$SCALE_METRICS")"
  payload="$(jq -nc \
    --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg test "$test_id" \
    --arg metric "$metric" \
    --arg value "$value" \
    --arg extra "$extra" \
    '{ts:$ts,test:$test,metric:$metric,value:($value|tonumber),extra:($extra|fromjson)}')" || {
    d1_log "jq failed test=$test_id metric=$metric value=$value extra=$extra"
    return 1
  }
  echo "$payload" >>"$D1_METRICS"
  echo "$payload" >>"$SCALE_METRICS"
}

d1_seed() {
  local mb="$1"
  local dest="$2"
  python3 - "$mb" "$dest" <<'PY'
import sqlite3, os, sys
mb = int(sys.argv[1])
dest = sys.argv[2]
if os.path.exists(dest):
    os.remove(dest)
con = sqlite3.connect(dest)
con.execute("PRAGMA page_size=4096")
con.execute("VACUUM")
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

d1_import_cli_ready() {
  if ! command -v celld >/dev/null 2>&1; then
    return 1
  fi
  local out
  out="$(celld d1 import -h 2>&1)" || true
  if grep -qiE 'import[[:space:]]+DATABASE|celld d1 import' <<<"$out"; then
    return 0
  fi
  if grep -qi 'unknown.*subcommand: import' <<<"$out"; then
    return 1
  fi
  out="$(celld d1 --help 2>&1)" || true
  grep -qE 'd1 import|import DATABASE' <<<"$out"
}

d1_resolve_project() {
  if [[ -n "${D1_PROJECT:-}" ]]; then
    echo "$D1_PROJECT"
    return
  fi
  if [[ -d "${PHASE6_ROOT}/dev/examples/d1-seed" ]]; then
    echo "${PHASE6_ROOT}/dev/examples/d1-seed"
    return
  fi
  echo "${PHASE6_ROOT}/celld/examples/d1"
}

d1_database_name() {
  local project="$1"
  local wrangler="${project}/wrangler.jsonc"
  if [[ ! -f "$wrangler" ]]; then
    echo "${D1_DATABASE:-guestbook}"
    return
  fi
  python3 - "$wrangler" <<'PY'
import re, sys, pathlib
text = pathlib.Path(sys.argv[1]).read_text()
m = re.search(r'"database_name"\s*:\s*"([^"]+)"', text)
print(m.group(1) if m else "guestbook")
PY
}

d1_fleet_healthy() {
  curl -sf "http://127.0.0.1:${CELLD_PORT}/.well-known/celld/health" >/dev/null 2>&1
}

d1_wrangler_has_d1() {
  local project="$1"
  local wrangler="${project}/wrangler.jsonc"
  [[ -f "$wrangler" ]] && grep -q 'd1_databases' "$wrangler"
}

d1_dump_sql_present() {
  [[ -f "$DUMP_SQL" ]] && return 0
  find "$WORKDIR" -name '*.dump.sql' -print -quit 2>/dev/null | grep -q .
}

# Isolated prefix so G3 restore is not poisoned by leftover guestbook epochs
# on the shared demo-app fleet (restore e11 "transaction not available").
D1_BUCKET="s3://cellp-celld/d1-imp-${STRESS_RUN_ID}"
D1_ENDPOINT="${S3_ENDPOINT:-http://127.0.0.1:19000}"
D1_REGION="${AWS_REGION:-us-east-1}"
# SIGKILL leaves a live-looking peer lease; the default 120s first-readiness
# gate then withholds /__celld/health. G3 must not wait that long.
export CELLD_READY_FLEET_GATE_MS="${CELLD_READY_FLEET_GATE_MS:-5000}"

write_report() {
  mkdir -p "$(dirname "$D1_REPORT")"
  local status="PASS"
  if (( FAILS > 0 )); then
    status="FAIL (${FAILS})"
  fi
  {
    echo "# D1 import scale report"
    echo
    echo "> **Run ID:** ${STRESS_RUN_ID}  "
    echo "> **Date:** $(date -u +%Y-%m-%dT%H:%M:%SZ)  "
    echo "> **Result:** ${status}"
    echo
    echo "## Environment"
    echo
    echo "| Item | Value |"
    echo "|------|-------|"
    echo "| celld | $(command -v celld 2>/dev/null || echo missing) |"
    echo "| fleet | :${CELLD_PORT} |"
    echo "| project | \`${D1_PROJECT_DIR:-skipped}\` |"
    echo "| database | \`${D1_DB_NAME:-}\` |"
    echo "| seed | ${D1_IMPORT_SIZE_MB} MB |"
    echo "| bucket | \`${D1_BUCKET:-}\` |"
    echo
    echo "## Notes"
    echo
    for n in "${NOTES[@]}"; do
      echo "- ${n}"
    done
    echo
    echo "## Metrics"
    echo
    echo "JSONL: \`docs/evidence/d1-import-metrics.jsonl\` (this run appended)."
  } >"$D1_REPORT"
}

require_tools() {
  stress_need jq
  stress_need python3
  stress_need sqlite3
  stress_need curl
}

require_tools
d1_log "run=${STRESS_RUN_ID} size=${D1_IMPORT_SIZE_MB}MB metrics=${D1_METRICS}"

if ! d1_import_cli_ready; then
  d1_log "SKIP: celld d1 import not available; recording skip metric"
  d1_note "celld d1 import CLI missing — skipped so CI stays green on older celld builds"
  d1_record "TP-D1-IMP" "d1_import" -1 '{"status":"skipped","reason":"cli-missing"}'
  write_report
  echo "SKIP: celld d1 import not available (exit 0)"
  exit 0
fi

if ! command -v celld >/dev/null; then
  echo "FAIL: celld CLI not on PATH" >&2
  exit 1
fi

D1_PROJECT_DIR="$(d1_resolve_project)"
D1_DB_NAME="${D1_DATABASE:-$(d1_database_name "$D1_PROJECT_DIR")}"

if ! d1_wrangler_has_d1 "$D1_PROJECT_DIR"; then
  echo "FAIL: ${D1_PROJECT_DIR}/wrangler.jsonc missing d1_databases" >&2
  exit 1
fi

mkdir -p "$WORKDIR"
# Isolated watch so G3 wipe does not delete dashboard/demo-app local files.
export CELLD_WATCH="${WORKDIR}/celld-watch"
mkdir -p "$CELLD_WATCH"
if d1_dump_sql_present; then
  d1_fail "pre-run .dump.sql already present in ${WORKDIR}"
fi

# Fresh D1 identity so G3 restore is not poisoned by a prior epoch on guestbook.
D1_RUN_PROJECT="${WORKDIR}/d1-project"
rm -rf "$D1_RUN_PROJECT"
cp -R "$D1_PROJECT_DIR" "$D1_RUN_PROJECT"
python3 - "$D1_RUN_PROJECT/wrangler.jsonc" "$STRESS_RUN_ID" <<'PY'
import json, pathlib, sys, uuid
path = pathlib.Path(sys.argv[1])
run = sys.argv[2]
text = path.read_text()
# strip // comments
lines = []
for line in text.splitlines():
    stripped = line.lstrip()
    if stripped.startswith("//"):
        continue
    lines.append(line)
cfg = json.loads("\n".join(lines))
dbs = cfg.get("d1_databases") or []
if not dbs:
    raise SystemExit("no d1_databases")
dbs[0]["database_id"] = str(uuid.uuid5(uuid.NAMESPACE_URL, f"cellp-d1-import-{run}"))
dbs[0]["database_name"] = dbs[0].get("database_name") or "guestbook"
path.write_text(json.dumps(cfg, indent=2) + "\n")
print(dbs[0]["database_name"])
PY
D1_PROJECT_DIR="$D1_RUN_PROJECT"
D1_DB_NAME="${D1_DATABASE:-$(d1_database_name "$D1_PROJECT_DIR")}"
d1_note "G3 fixture ${D1_PROJECT_DIR} database=${D1_DB_NAME}"

pidfile="${PHASE6_ROOT}/dev/data/pids/celld.pid"
watch="${CELLD_WATCH:-${PHASE6_ROOT}/dev/data/celld-watch}"
if [[ "$watch" != /* ]]; then
  watch="${PHASE6_ROOT}/${watch#./}"
fi
mkdir -p "$watch" "$(dirname "$pidfile")" "${PHASE6_ROOT}/dev/data/logs"
if command -v lsof >/dev/null 2>&1; then
  extra="$(lsof -tiTCP:"${CELLD_PORT}" -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -n "$extra" ]]; then
    d1_log "kill listeners on :${CELLD_PORT}: ${extra}"
    # shellcheck disable=SC2086
    kill $extra 2>/dev/null || true
    sleep 1
  fi
fi
export CELLD_WATCH="$watch"
d1_log "diagnose ${D1_BUCKET}"
celld diagnose --bucket "$D1_BUCKET" --endpoint "$D1_ENDPOINT" --region "$D1_REGION" \
  >>"${PHASE6_ROOT}/dev/data/logs/celld-diagnose.log" 2>&1 || true

d1_log "deploy ${D1_PROJECT_DIR} → ${D1_BUCKET}"
t0="$(d1_now_ms)"
if ! (
  cd "$D1_PROJECT_DIR"
  celld deploy . --bucket "$D1_BUCKET" --endpoint "$D1_ENDPOINT" --region "$D1_REGION" >/dev/null
); then
  echo "FAIL: celld deploy ${D1_PROJECT_DIR} to ${D1_BUCKET}" >&2
  exit 1
fi
t1="$(d1_now_ms)"
deploy_ms=$((t1 - t0))
d1_note "deploy OK (${deploy_ms} ms)"
d1_record "TP-D1-IMP" "deploy_ms" "$deploy_ms" "{\"project\":\"${D1_PROJECT_DIR}\"}"

d1_log "start celld listen=:${CELLD_PORT} bucket=${D1_BUCKET}"
celld --bucket "$D1_BUCKET" --endpoint "$D1_ENDPOINT" --region "$D1_REGION" \
  --listen "127.0.0.1:${CELLD_PORT}" >>"${PHASE6_ROOT}/dev/data/logs/celld.log" 2>&1 &
echo $! >"$pidfile"
disown $! 2>/dev/null || true
healthy=0
for i in $(seq 1 60); do
  if d1_fleet_healthy; then
    healthy=1
    break
  fi
  sleep 1
done
if (( healthy == 0 )); then
  echo "FAIL: celld not healthy on :${CELLD_PORT} after deploy+start (bucket ${D1_BUCKET})" >&2
  tail -20 "${PHASE6_ROOT}/dev/data/logs/celld.log" >&2 || true
  exit 1
fi

# --- seed 100 MB incompressible SQLite (page_size 4096) ---
t0="$(d1_now_ms)"
seed_bytes="$(d1_seed "$D1_IMPORT_SIZE_MB" "$SEED_DB")"
t1="$(d1_now_ms)"
seed_ms=$((t1 - t0))
want=$((D1_IMPORT_SIZE_MB * 1024 * 1024))
d1_log "seeded ${seed_bytes} bytes in ${seed_ms}ms"
d1_record "TP-D1-IMP" "seed_bytes" "$seed_bytes" "{\"mb\":${D1_IMPORT_SIZE_MB},\"ms\":${seed_ms}}"
d1_record "TP-D1-IMP" "seed_ms" "$seed_ms" "{\"bytes\":${seed_bytes}}"
if (( seed_bytes < want )); then
  d1_fail "seed file ${seed_bytes} < ${want}"
else
  d1_note "seed OK (${seed_bytes} bytes, ${seed_ms} ms, page_size=4096)"
fi

# --- celld d1 import ---
if sqlite3 "$SEED_DB" ".dump" >"$DUMP_SQL" 2>/dev/null; then
  dump_bytes="$(d1_file_bytes "$DUMP_SQL")"
  rm -f "$DUMP_SQL"
  d1_note "sqlite3 .dump baseline produced ${dump_bytes} bytes SQL (not used for import)"
  d1_record "TP-D1-IMP" "dump_sql_bytes" "$dump_bytes" '{"note":"baseline-only-removed"}'
else
  d1_note "sqlite3 .dump baseline failed (expected for large seed under dump gate)"
  d1_record "TP-D1-IMP" "dump_sql_bytes" 0 '{"note":"dump-failed"}'
fi

if d1_dump_sql_present; then
  d1_fail ".dump.sql present before import (must not use SQL dump path)"
fi

# --- celld d1 import ---
d1_log "import ${D1_DB_NAME} --file ${SEED_DB}"
t0="$(d1_now_ms)"
import_out="${WORKDIR}/import.out"
import_rc=0
if ! celld d1 import "$D1_DB_NAME" --file "$SEED_DB" "$D1_PROJECT_DIR" \
  --bucket "$D1_BUCKET" --endpoint "$D1_ENDPOINT" --region "$D1_REGION" >"$import_out" 2>&1; then
  import_rc=1
fi
t1="$(d1_now_ms)"
import_ms=$((t1 - t0))
d1_record "TP-D1-IMP" "import_ms" "$import_ms" "{\"bytes\":${seed_bytes},\"rc\":${import_rc}}"
d1_log "import wall=${import_ms}ms rc=${import_rc}"

if (( import_rc != 0 )); then
  d1_fail "celld d1 import failed: $(head -c 300 "$import_out" | tr '\n' ' ')"
else
  d1_note "import OK in ${import_ms} ms"
  d1_record "TP-D1-IMP" "d1_import" 1 '{"status":"ok"}'
fi

if d1_dump_sql_present; then
  d1_fail ".dump.sql created during import (binary import must not materialize SQL)"
else
  d1_note "no .dump.sql after import"
  d1_record "TP-D1-IMP" "dump_sql_created" 0 "{}"
fi

d1_execute() {
  local dest="$1"
  local sql="$2"
  celld d1 execute "$D1_DB_NAME" --command "$sql" "$D1_PROJECT_DIR" \
    --bucket "$D1_BUCKET" --endpoint "$D1_ENDPOINT" --region "$D1_REGION" >"$dest" 2>&1
}

# Import can finish before the D1 cell is queryable (deploy/adoption race).
d1_execute_retry() {
  local dest="$1"
  local sql="$2"
  local i
  sleep 1
  for i in $(seq 1 12); do
    if d1_execute "$dest" "$sql"; then
      return 0
    fi
    sleep 1
  done
  return 1
}

d1_wait_health() {
  local i
  for i in $(seq 1 60); do
    if d1_fleet_healthy; then
      return 0
    fi
    sleep 1
  done
  return 1
}

# G3: new celld process, wipe local watch so restore is from the bucket LTX, not the live file.
d1_stop_celld() {
  local pidfile="${PHASE6_ROOT}/dev/data/pids/celld.pid"
  local pid
  if [[ ! -f "$pidfile" ]]; then
    return 0
  fi
  pid="$(cat "$pidfile")"
  kill "$pid" 2>/dev/null || true
  local i
  for i in $(seq 1 20); do
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.5
  done
  if kill -0 "$pid" 2>/dev/null; then
    d1_log "G3 SIGKILL lingering pid ${pid}"
    kill -9 "$pid" 2>/dev/null || true
    sleep 1
  fi
  rm -f "$pidfile"
}

d1_restart_from_bucket() {
  local pidfile="${PHASE6_ROOT}/dev/data/pids/celld.pid"
  local watch="${CELLD_WATCH:-${PHASE6_ROOT}/dev/data/celld-watch}"
  if [[ "$watch" != /* ]]; then
    watch="${PHASE6_ROOT}/${watch#./}"
  fi
  d1_stop_celld

  # Drop anything still bound to the listen port.
  if command -v lsof >/dev/null 2>&1; then
    local extra
    extra="$(lsof -tiTCP:"${CELLD_PORT}" -sTCP:LISTEN 2>/dev/null || true)"
    if [[ -n "$extra" ]]; then
      d1_log "G3 kill listeners on :${CELLD_PORT}: ${extra}"
      # shellcheck disable=SC2086
      kill $extra 2>/dev/null || true
      sleep 1
      extra="$(lsof -tiTCP:"${CELLD_PORT}" -sTCP:LISTEN 2>/dev/null || true)"
      if [[ -n "$extra" ]]; then
        d1_log "G3 SIGTERM leftover listeners ${extra}"
        # shellcheck disable=SC2086
        kill $extra 2>/dev/null || true
        sleep 2
      fi
    fi
  fi
  if [[ -d "$watch" ]]; then
    d1_log "G3 wipe local watch ${watch}"
    find "$watch" -mindepth 1 -maxdepth 1 -exec rm -rf {} +
  else
    mkdir -p "$watch"
  fi
  export CELLD_WATCH="$watch"
  d1_log "G3 start celld listen=:${CELLD_PORT} watch=${watch}"
  celld --bucket "$D1_BUCKET" --endpoint "$D1_ENDPOINT" --region "$D1_REGION" \
    --listen "127.0.0.1:${CELLD_PORT}" >>"${PHASE6_ROOT}/dev/data/logs/celld.log" 2>&1 &
  echo $! >"$pidfile"
  disown $! 2>/dev/null || true
  d1_wait_health
}

# --- post-import execute (retry: import can beat cell adoption) ---
exec_out="${WORKDIR}/execute.out"
exec_rc=0
t0="$(d1_now_ms)"
if ! d1_execute_retry "$exec_out" 'SELECT 1 AS ok'; then
  exec_rc=1
fi
t1="$(d1_now_ms)"
exec_ms=$((t1 - t0))
d1_record "TP-D1-IMP" "execute_ms" "$exec_ms" "{\"rc\":${exec_rc}}"

blob_count=0
count_out="${WORKDIR}/count.out"
if d1_execute_retry "$count_out" 'SELECT count(*) AS n FROM blobs'; then
  blob_count="$(grep -Eo '[0-9]+' "$count_out" | tail -1 || echo 0)"
  d1_record "TP-D1-IMP" "blob_rows" "$blob_count" "{\"expected\":${D1_IMPORT_SIZE_MB}}"
  if (( blob_count == D1_IMPORT_SIZE_MB )); then
    d1_note "blob row count ${blob_count} matches seed"
  else
    d1_note "blob row count ${blob_count} (expected ${D1_IMPORT_SIZE_MB}); SELECT 1 still required"
  fi
else
  d1_record "TP-D1-IMP" "blob_rows" -1 '{"reason":"count-failed"}'
fi

if (( exec_rc != 0 )); then
  d1_fail "celld d1 execute SELECT 1 failed: $(head -c 300 "$exec_out" | tr '\n' ' ')"
else
  d1_note "execute SELECT 1 OK (${exec_ms} ms)"
fi

# --- G3: kill celld, wipe local watch, restore from bucket, re-query ---
g3_rc=1
g3_ms=0
g3_count=0
if (( import_rc == 0 )); then
  d1_log "G3 restart celld from bucket LTX"
  t0="$(d1_now_ms)"
  if ! d1_restart_from_bucket; then
    d1_fail "G3 celld did not become healthy after watch wipe"
    t1="$(d1_now_ms)"
    g3_ms=$((t1 - t0))
  else
    g3_out="${WORKDIR}/g3-count.out"
    if d1_execute_retry "$g3_out" 'SELECT count(*) AS n FROM blobs'; then
      g3_count="$(grep -Eo '[0-9]+' "$g3_out" | tail -1 || echo 0)"
      t1="$(d1_now_ms)"
      g3_ms=$((t1 - t0))
      if (( g3_count == D1_IMPORT_SIZE_MB )); then
        g3_rc=0
        d1_note "G3 restore OK blob_rows=${g3_count} (${g3_ms} ms)"
        restore_line="$(grep 'restore_plan' "${PHASE6_ROOT}/dev/data/logs/celld.log" 2>/dev/null | tail -1 || true)"
        if [[ -n "$restore_line" ]]; then
          d1_note "G3 ${restore_line}"
        fi
      else
        d1_fail "G3 blob count ${g3_count} != seed ${D1_IMPORT_SIZE_MB}: $(head -c 200 "$g3_out" | tr '\n' ' ')"
      fi
    else
      t1="$(d1_now_ms)"
      g3_ms=$((t1 - t0))
      d1_fail "G3 execute after restore failed: $(head -c 300 "$g3_out" | tr '\n' ' ')"
    fi
  fi
  d1_record "TP-D1-IMP" "g3_ms" "$g3_ms" "{\"rc\":${g3_rc},\"blob_rows\":${g3_count}}"
  d1_record "TP-D1-IMP" "g3_blob_rows" "$g3_count" "{\"expected\":${D1_IMPORT_SIZE_MB}}"
else
  d1_note "G3 skipped because import failed"
fi

write_report
d1_log "report ${D1_REPORT}"
if (( FAILS > 0 )); then
  echo "FAIL: d1 import scale (${FAILS} gates)" >&2
  exit 1
fi
echo "PASS: d1 import scale run ${STRESS_RUN_ID} import_ms=${import_ms}"
exit 0
