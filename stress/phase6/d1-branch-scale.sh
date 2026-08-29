#!/usr/bin/env bash
# TP-D1-BRANCH / B2/B5/B6 — parent import → child branch object volume gate
#
# Env:
#   D1_IMPORT_SIZE_MB=8 (override 100 for large run)
#   D1_BRANCH_METRICS=docs/evidence/d1-branch-metrics.jsonl
#   D1_BRANCH_REPORT=docs/evidence/d1-branch-scale-report.md
set -euo pipefail

PHASE6_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck disable=SC1091
source "${PHASE6_ROOT}/stress/phase6/lib/common.sh"

scale_source_env

GOBIN="$(go env GOPATH 2>/dev/null)/bin"
export PATH="${GOBIN}:${PATH}"

D1_IMPORT_SIZE_MB="${D1_IMPORT_SIZE_MB:-8}"
D1_BRANCH_METRICS="${D1_BRANCH_METRICS:-${PHASE6_ROOT}/docs/evidence/d1-branch-metrics.jsonl}"
D1_BRANCH_REPORT="${D1_BRANCH_REPORT:-${PHASE6_ROOT}/docs/evidence/d1-branch-scale-report.md}"
STRESS_RUN_ID="${STRESS_RUN_ID:-$(date +%Y%m%d-%H%M%S)}"
CELLD_PORT="${CELLD_PORT:-8792}"
WORKDIR="${PHASE6_ROOT}/dev/data/d1-branch-scale/${STRESS_RUN_ID}"
SEED_DB="${WORKDIR}/seed.db"

FAILS=0
NOTES=()

d1_log() { echo "[d1-branch-scale $(date +%H:%M:%S)] $*" >&2; }

d1_fail() {
  d1_log "FAIL: $*"
  NOTES+=("FAIL: $*")
  FAILS=$((FAILS + 1))
}

d1_note() { NOTES+=("$*"); }

d1_now_ms() {
  python3 -c 'import time; print(int(time.time()*1000))'
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
  mkdir -p "$(dirname "$D1_BRANCH_METRICS")" "$(dirname "$SCALE_METRICS")"
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
  echo "$payload" >>"$D1_BRANCH_METRICS"
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

d1_branch_cli_ready() {
  if ! command -v celld >/dev/null 2>&1; then
    return 1
  fi
  local out
  out="$(celld d1 branch -h 2>&1)" || true
  if grep -qiE 'branch[[:space:]]+DATABASE|celld d1 branch' <<<"$out"; then
    return 0
  fi
  if grep -qi 'unknown.*subcommand: branch' <<<"$out"; then
    return 1
  fi
  out="$(celld d1 --help 2>&1)" || true
  grep -qE 'd1 branch|branch DATABASE' <<<"$out"
}

d1_import_cli_ready() {
  celld d1 import -h >/dev/null 2>&1
}

d1_resolve_project() {
  if [[ -n "${D1_PROJECT:-}" ]]; then
    echo "$D1_PROJECT"
    return
  fi
  echo "${PHASE6_ROOT}/dev/examples/d1-seed"
}

d1_database_name() {
  local project="$1"
  python3 - "$project/wrangler.jsonc" <<'PY'
import re, sys, pathlib
text = pathlib.Path(sys.argv[1]).read_text()
m = re.search(r'"database_name"\s*:\s*"([^"]+)"', text)
print(m.group(1) if m else "guestbook")
PY
}

d1_fleet_healthy() {
  curl -sf "http://127.0.0.1:${CELLD_PORT}/.well-known/celld/health" >/dev/null 2>&1
}

write_report() {
  mkdir -p "$(dirname "$D1_BRANCH_REPORT")"
  local status="${1:-}"
  if [[ -z "$status" ]]; then
    status="PASS"
    if (( FAILS > 0 )); then
      status="FAIL (${FAILS})"
    fi
  fi
  {
    echo "# D1 branch scale report"
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
    echo "| seed | ${D1_IMPORT_SIZE_MB} MB |"
    echo "| project | \`${D1_CELL_PROJECT:-}\` |"
    echo "| parent bucket | \`${PARENT_BUCKET:-}\` |"
    echo "| child bucket | \`${CHILD_BUCKET:-}\` |"
    echo
    echo "## Notes"
    echo
    for n in "${NOTES[@]}"; do
      echo "- ${n}"
    done
    echo
    echo "## Metrics"
    echo
    echo "JSONL: \`docs/evidence/d1-branch-metrics.jsonl\`"
  } >"$D1_BRANCH_REPORT"
}

# List objects under s3://bucket/prefix via ListObjectsV2 (no aws CLI / boto3).
s3_list_objects() {
  python3 - "$1" <<'PY'
import hashlib, hmac, os, sys, urllib.parse, urllib.request, xml.etree.ElementTree as ET
from datetime import datetime, timezone

spec = sys.argv[1]
if not spec.startswith("s3://"):
    raise SystemExit(f"not an s3 uri: {spec}")
rest = spec[5:]
bucket, _, key_prefix = rest.partition("/")
if key_prefix and not key_prefix.endswith("/"):
    key_prefix += "/"
endpoint = os.environ.get("D1_ENDPOINT") or os.environ.get("S3_ENDPOINT") or ""
if not endpoint:
    raise SystemExit("D1_ENDPOINT/S3_ENDPOINT unset")
access = os.environ["AWS_ACCESS_KEY_ID"]
secret = os.environ["AWS_SECRET_ACCESS_KEY"]
region = os.environ.get("AWS_REGION", "us-east-1")
parsed = urllib.parse.urlparse(endpoint)
host, scheme = parsed.netloc, parsed.scheme or "http"

def sign(key: bytes, msg: str) -> bytes:
    return hmac.new(key, msg.encode(), hashlib.sha256).digest()

def list_page(token=None):
    query_items = [("list-type", "2"), ("prefix", key_prefix)]
    if token:
        query_items.append(("continuation-token", token))
    query_items.sort()
    query = urllib.parse.urlencode(query_items)
    now = datetime.now(timezone.utc)
    amz_date = now.strftime("%Y%m%dT%H%M%SZ")
    datestamp = amz_date[:8]
    payload_hash = hashlib.sha256(b"").hexdigest()
    canonical_uri = f"/{bucket}"
    canonical_headers = (
        f"host:{host}\n"
        f"x-amz-content-sha256:{payload_hash}\n"
        f"x-amz-date:{amz_date}\n"
    )
    signed_headers = "host;x-amz-content-sha256;x-amz-date"
    canonical_request = (
        f"GET\n{canonical_uri}\n{query}\n{canonical_headers}\n"
        f"{signed_headers}\n{payload_hash}"
    )
    scope = f"{datestamp}/{region}/s3/aws4_request"
    string_to_sign = (
        "AWS4-HMAC-SHA256\n"
        f"{amz_date}\n{scope}\n"
        f"{hashlib.sha256(canonical_request.encode()).hexdigest()}"
    )
    k_date = sign(("AWS4" + secret).encode(), datestamp)
    k_region = hmac.new(k_date, region.encode(), hashlib.sha256).digest()
    k_service = hmac.new(k_region, b"s3", hashlib.sha256).digest()
    k_signing = hmac.new(k_service, b"aws4_request", hashlib.sha256).digest()
    signature = hmac.new(k_signing, string_to_sign.encode(), hashlib.sha256).hexdigest()
    auth = (
        f"AWS4-HMAC-SHA256 Credential={access}/{scope}, "
        f"SignedHeaders={signed_headers}, Signature={signature}"
    )
    url = f"{scheme}://{host}{canonical_uri}?{query}"
    req = urllib.request.Request(
        url,
        headers={
            "x-amz-date": amz_date,
            "x-amz-content-sha256": payload_hash,
            "Authorization": auth,
        },
    )
    with urllib.request.urlopen(req) as resp:
        return resp.read()

ns = {"s3": "http://s3.amazonaws.com/doc/2006-03-01/"}
token = None
while True:
    body = list_page(token)
    root = ET.fromstring(body)
    for contents in root.findall("s3:Contents", ns) or root.findall("Contents"):
        key_el = contents.find("s3:Key", ns)
        size_el = contents.find("s3:Size", ns)
        if key_el is None:
            key_el = contents.find("Key")
        if size_el is None:
            size_el = contents.find("Size")
        if key_el is None or key_el.text is None:
            continue
        size = int((size_el.text if size_el is not None else "0") or "0")
        print(f"{size}\t{key_el.text}")
    truncated = root.findtext("s3:IsTruncated", default="false", namespaces=ns)
    if truncated is None:
        truncated = root.findtext("IsTruncated") or "false"
    if truncated.lower() != "true":
        break
    token = root.findtext("s3:NextContinuationToken", namespaces=ns) or root.findtext(
        "NextContinuationToken"
    )
    if not token:
        break
PY
}

prefix_list_bytes() {
  local bucket="$1"
  s3_list_objects "$bucket" | awk -F '\t' '{sum += $1} END {print sum+0}'
}

has_min_txid_one_ltx() {
  local bucket="$1"
  # celld keys: {prefix}cells/<scope>/ltx/e<epoch>/{level:04x}/{min16}-{max16}.ltx
  # Origin snapshots are MinTXID==1 at L0 (0000) or L9 (0009).
  s3_list_objects "$bucket" | awk -F '\t' '
    $2 ~ /\/cells\/.*\/ltx\/e[0-9]+\/(0000|0009)\/0000000000000001-/ { found=1 }
    END { exit found ? 0 : 1 }
  '
}

has_base_json() {
  local bucket="$1"
  s3_list_objects "$bucket" | awk -F '\t' '
    $2 ~ /\/base\.json$/ { found=1 }
    END { exit found ? 0 : 1 }
  '
}

require_tools() {
  stress_need jq
  stress_need python3
  stress_need sqlite3
  stress_need curl
}

require_tools
d1_log "run=${STRESS_RUN_ID} size=${D1_IMPORT_SIZE_MB}MB"

if ! d1_branch_cli_ready; then
  d1_log "SKIP: celld d1 branch not available"
  d1_note "celld d1 branch CLI missing — skipped (metric -1)"
  d1_record "TP-D1-BRANCH" "d1_branch" -1 '{"status":"skipped","reason":"cli-missing"}'
  write_report "SKIP"
  echo "SKIP: celld d1 branch not available (exit 0)"
  exit 0
fi

if ! d1_import_cli_ready; then
  echo "FAIL: celld d1 import not available (parent seed requires import)" >&2
  exit 1
fi

D1_ENDPOINT="${S3_ENDPOINT:-http://127.0.0.1:19000}"
D1_REGION="${AWS_REGION:-us-east-1}"
export CELLD_READY_FLEET_GATE_MS="${CELLD_READY_FLEET_GATE_MS:-5000}"
export AWS_ACCESS_KEY_ID="${RUSTFS_ACCESS_KEY:-rustfsadmin}"
export AWS_SECRET_ACCESS_KEY="${RUSTFS_SECRET_KEY:-rustfsadmin}"
export AWS_REGION="$D1_REGION"

# Same S3 project + distinct versions — required by validate_parent_bucket.
D1_CELL_PROJECT="d1-br-${STRESS_RUN_ID}"
PARENT_BUCKET="s3://cellp-celld/${D1_CELL_PROJECT}/parent"
CHILD_BUCKET="s3://cellp-celld/${D1_CELL_PROJECT}/child"
D1_PROJECT_DIR="$(d1_resolve_project)"
D1_DB_NAME="${D1_DATABASE:-$(d1_database_name "$D1_PROJECT_DIR")}"

mkdir -p "$WORKDIR"
export CELLD_WATCH="${WORKDIR}/celld-watch"
mkdir -p "$CELLD_WATCH"

# Fresh project copy; one database_id shared by parent import and child branch.
D1_RUN_PROJECT="${WORKDIR}/d1-project"
rm -rf "$D1_RUN_PROJECT"
cp -R "$D1_PROJECT_DIR" "$D1_RUN_PROJECT"
python3 - "$D1_RUN_PROJECT/wrangler.jsonc" "$STRESS_RUN_ID" <<'PY'
import json, pathlib, sys, uuid
path = pathlib.Path(sys.argv[1])
run = sys.argv[2]
lines = [ln for ln in path.read_text().splitlines() if not ln.lstrip().startswith("//")]
cfg = json.loads("\n".join(lines))
dbs = cfg.get("d1_databases") or []
if not dbs:
    raise SystemExit("no d1_databases")
dbs[0]["database_id"] = str(uuid.uuid5(uuid.NAMESPACE_URL, f"cellp-d1-branch-{run}"))
dbs[0]["database_name"] = dbs[0].get("database_name") or "guestbook"
path.write_text(json.dumps(cfg, indent=2) + "\n")
print(dbs[0]["database_name"])
PY
D1_PROJECT_DIR="$D1_RUN_PROJECT"
D1_DB_NAME="${D1_DATABASE:-$(d1_database_name "$D1_PROJECT_DIR")}"

pidfile="${PHASE6_ROOT}/dev/data/pids/celld-branch.pid"
mkdir -p "$(dirname "$pidfile")" "${PHASE6_ROOT}/dev/data/logs"
d1_stop_celld() {
  local pid
  [[ -f "$pidfile" ]] || return 0
  pid="$(cat "$pidfile")"
  kill "$pid" 2>/dev/null || true
  sleep 1
  kill -9 "$pid" 2>/dev/null || true
  rm -f "$pidfile"
}

d1_start_celld() {
  local bucket="$1"
  local version_id="$2"
  d1_stop_celld
  if command -v lsof >/dev/null 2>&1; then
    local extra
    extra="$(lsof -tiTCP:"${CELLD_PORT}" -sTCP:LISTEN 2>/dev/null || true)"
    if [[ -n "$extra" ]]; then
      d1_log "kill listeners on :${CELLD_PORT}: ${extra}"
      # shellcheck disable=SC2086
      kill $extra 2>/dev/null || true
      sleep 1
    fi
  fi
  export CELLD_VAR_PROJECT_ID="${D1_CELL_PROJECT}"
  export CELLD_VAR_VERSION_ID="${version_id}"
  export CELLD_READY_FLEET_GATE_MS="${CELLD_READY_FLEET_GATE_MS:-5000}"
  celld --bucket "$bucket" --endpoint "$D1_ENDPOINT" --region "$D1_REGION" \
    --listen "127.0.0.1:${CELLD_PORT}" >>"${PHASE6_ROOT}/dev/data/logs/celld-branch.log" 2>&1 &
  echo $! >"$pidfile"
  disown $! 2>/dev/null || true
  local i
  for i in $(seq 1 60); do
    if d1_fleet_healthy; then
      return 0
    fi
    sleep 1
  done
  return 1
}

d1_log "deploy parent ${D1_PROJECT_DIR} → ${PARENT_BUCKET}"
(
  cd "$D1_PROJECT_DIR"
  celld deploy . --bucket "$PARENT_BUCKET" --endpoint "$D1_ENDPOINT" --region "$D1_REGION" >/dev/null
)
if ! d1_start_celld "$PARENT_BUCKET" parent; then
  echo "FAIL: parent celld not healthy" >&2
  exit 1
fi

seed_bytes="$(d1_seed "$D1_IMPORT_SIZE_MB" "$SEED_DB")"
want=$((D1_IMPORT_SIZE_MB * 1024 * 1024))
d1_record "TP-D1-BRANCH" "seed_bytes" "$seed_bytes" "{\"mb\":${D1_IMPORT_SIZE_MB}}"
if (( seed_bytes < want )); then
  d1_fail "seed ${seed_bytes} < ${want}"
fi

d1_log "parent import ${D1_DB_NAME}"
t0="$(d1_now_ms)"
if ! celld d1 import "$D1_DB_NAME" --file "$SEED_DB" "$D1_PROJECT_DIR" \
  --bucket "$PARENT_BUCKET" --endpoint "$D1_ENDPOINT" --region "$D1_REGION" \
  >"${WORKDIR}/parent-import.out" 2>&1; then
  d1_fail "parent import: $(head -c 200 "${WORKDIR}/parent-import.out" | tr '\n' ' ')"
  write_report
  exit 1
fi
import_ms=$(( $(d1_now_ms) - t0 ))
d1_note "parent import OK (${import_ms} ms)"
d1_record "TP-D1-BRANCH" "parent_import_ms" "$import_ms" "{}"

parent_prefix_bytes="$(prefix_list_bytes "$PARENT_BUCKET")"
d1_record "TP-D1-BRANCH" "parent_snapshot_bytes" "$parent_prefix_bytes" "{}"
d1_note "parent prefix bytes=${parent_prefix_bytes}"

# Child fleet on isolated bucket.
d1_log "deploy child → ${CHILD_BUCKET}"
(
  cd "$D1_PROJECT_DIR"
  celld deploy . --bucket "$CHILD_BUCKET" --endpoint "$D1_ENDPOINT" --region "$D1_REGION" >/dev/null
)
if ! d1_start_celld "$CHILD_BUCKET" child; then
  d1_fail "child celld not healthy"
  write_report
  exit 1
fi

d1_log "child branch from ${PARENT_BUCKET}"
t0="$(d1_now_ms)"
if ! CELLD_VAR_PROJECT_ID="${D1_CELL_PROJECT}" CELLD_VAR_VERSION_ID=child \
  celld d1 branch "$D1_DB_NAME" \
  --parent-bucket "$PARENT_BUCKET" \
  "$D1_PROJECT_DIR" \
  --bucket "$CHILD_BUCKET" \
  --endpoint "$D1_ENDPOINT" --region "$D1_REGION" \
  >"${WORKDIR}/child-branch.out" 2>&1; then
  d1_fail "child branch: $(head -c 200 "${WORKDIR}/child-branch.out" | tr '\n' ' ')"
  write_report
  exit 1
fi
branch_ms=$(( $(d1_now_ms) - t0 ))
d1_record "TP-D1-BRANCH" "branch_ms" "$branch_ms" "{}"
d1_note "branch OK (${branch_ms} ms)"

# B6: measure child prefix at branch complete, before child SQL.
child_prefix_bytes="$(prefix_list_bytes "$CHILD_BUCKET")"
d1_record "TP-D1-BRANCH" "child_prefix_bytes" "$child_prefix_bytes" '{"phase":"post-branch-pre-sql"}'
threshold=$(( want / 5 ))
if (( child_prefix_bytes > threshold )); then
  d1_fail "child prefix ${child_prefix_bytes} > 20% of seed (${threshold})"
else
  d1_note "B6 child prefix ${child_prefix_bytes} <= 20% seed (${threshold})"
fi

if has_min_txid_one_ltx "$CHILD_BUCKET"; then
  d1_fail "child prefix has min_txid==1 full LTX (forbidden)"
else
  d1_note "B2 no min_txid==1 LTX on child prefix"
  d1_record "TP-D1-BRANCH" "child_min_txid_one" 0 "{}"
fi

if has_base_json "$CHILD_BUCKET"; then
  d1_note "B2 child prefix has base.json"
  d1_record "TP-D1-BRANCH" "child_has_base_json" 1 "{}"
else
  d1_fail "child prefix missing base.json"
fi

# B5: kill child celld, wipe watch, new process — not a warm restart.
d1_stop_celld
if command -v lsof >/dev/null 2>&1; then
  extra="$(lsof -tiTCP:"${CELLD_PORT}" -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -n "$extra" ]]; then
    # shellcheck disable=SC2086
    kill $extra 2>/dev/null || true
    sleep 1
  fi
fi
watch="$CELLD_WATCH"
if [[ -d "$watch" ]]; then
  d1_log "B5 wipe local watch ${watch}"
  find "$watch" -mindepth 1 -maxdepth 1 -exec rm -rf {} +
else
  mkdir -p "$watch"
fi
if ! d1_start_celld "$CHILD_BUCKET" child; then
  d1_fail "B5 celld not healthy after watch wipe"
else
  exec_out="${WORKDIR}/b5-count.out"
  if celld d1 execute "$D1_DB_NAME" --command 'SELECT count(*) AS n FROM blobs' "$D1_PROJECT_DIR" \
    --bucket "$CHILD_BUCKET" --endpoint "$D1_ENDPOINT" --region "$D1_REGION" >"$exec_out" 2>&1; then
    b5_count="$(grep -Eo '[0-9]+' "$exec_out" | tail -1 || echo 0)"
    d1_record "TP-D1-BRANCH" "b5_blob_rows" "$b5_count" "{\"expected\":${D1_IMPORT_SIZE_MB}}"
    if (( b5_count == D1_IMPORT_SIZE_MB )); then
      d1_note "B5 restore OK blob_rows=${b5_count}"
    else
      d1_fail "B5 blob count ${b5_count} != ${D1_IMPORT_SIZE_MB}"
    fi
  else
    d1_fail "B5 execute after restore: $(head -c 200 "$exec_out" | tr '\n' ' ')"
  fi
fi

# Handoff on child stop must not publish a MinTXID==1 full snapshot.
if has_min_txid_one_ltx "$CHILD_BUCKET"; then
  d1_fail "child prefix has min_txid==1 full LTX after B5 (handoff snapshot leak)"
else
  d1_note "B2 still no min_txid==1 LTX after B5"
  d1_record "TP-D1-BRANCH" "child_min_txid_one_post_b5" 0 "{}"
fi
post_b5_bytes="$(prefix_list_bytes "$CHILD_BUCKET")"
d1_record "TP-D1-BRANCH" "child_prefix_bytes" "$post_b5_bytes" '{"phase":"post-b5"}'
if (( post_b5_bytes > threshold )); then
  d1_fail "child prefix after B5 ${post_b5_bytes} > 20% of seed (${threshold})"
else
  d1_note "B6 child prefix after B5 ${post_b5_bytes} <= 20% seed (${threshold})"
fi

write_report
d1_log "report ${D1_BRANCH_REPORT}"
if (( FAILS > 0 )); then
  echo "FAIL: d1 branch scale (${FAILS} gates)" >&2
  exit 1
fi
echo "PASS: d1 branch scale run ${STRESS_RUN_ID} branch_ms=${branch_ms}"
exit 0
