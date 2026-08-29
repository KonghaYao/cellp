#!/usr/bin/env bash
# TP-D1-BRANCH-MULTI — cellp orchestrator: 100 MB parent + N sibling branches
# Measures S3 prefix volume and local celld-watch footprint; verifies isolation.
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"

require_platform
require_offshoot
require_celld_cli
need sqlite3
need python3

rustfs_s3_env
# This gate measures local watch footprint; default runtime uses ephemeral watch (S3 durable).
export CELLP_CELLD_WATCH_PERSIST=1
export S3_ENDPOINT="${S3_ENDPOINT:-http://127.0.0.1:19000}"
export D1_ENDPOINT="$S3_ENDPOINT"

SIZE_MB="${D1_BRANCH_MULTI_SIZE_MB:-100}"
BRANCH_COUNT="${D1_BRANCH_MULTI_COUNT:-3}"
PROJECT="${D1_BRANCH_MULTI_PROJECT:-demo-app}"
PARENT="$(unique_id)"
DATABASE="guestbook"
D1_EXAMPLE="${E2E_ROOT}/dev/examples/d1-seed"
REPORT_JSON="${EVIDENCE_DIR}/d1-branch-multi-100mb.json"
METRICS_JSONL="${EVIDENCE_DIR}/d1-branch-multi-metrics.jsonl"

: "${S3_ENDPOINT:=http://127.0.0.1:19000}"
: "${AWS_REGION:=us-east-1}"

CHILD_IDS=()
CHILD_LABELS=(A B C D E F G H)
FAILS=0

mb_log() { echo "[d1-multi $(date +%H:%M:%S)] $*" >&2; }
mb_fail() { mb_log "FAIL: $*"; FAILS=$((FAILS + 1)); }

mb_record() {
  local metric="$1" value="$2" extra="${3:-}"
  mkdir -p "$(dirname "$METRICS_JSONL")"
  if [[ -z "$extra" ]]; then
    extra='{}'
  fi
  jq -nc \
    --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg test "TP-D1-BRANCH-MULTI" \
    --arg metric "$metric" \
    --arg value "$value" \
    --argjson extra "$extra" \
    '{ts:$ts,test:$test,metric:$metric,value:($value|tonumber),extra:$extra}' \
    >>"$METRICS_JSONL"
}

copy_d1_seed_bundle() {
  local dest="$1"
  mkdir -p "$dest"
  cp "${D1_EXAMPLE}/index.js" "${dest}/index.js"
  cp "${D1_EXAMPLE}/wrangler.jsonc" "${dest}/wrangler.jsonc"
}

s3_prefix_bytes() {
  python3 - "$1" <<'PY'
import hashlib, hmac, os, sys, urllib.parse, urllib.request, xml.etree.ElementTree as ET
from datetime import datetime, timezone

spec = sys.argv[1]
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

def sign(key, msg):
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
        f"AWS4-HMAC-SHA256\n{amz_date}\n{scope}\n"
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
    req = urllib.request.Request(url, headers={
        "x-amz-date": amz_date,
        "x-amz-content-sha256": payload_hash,
        "Authorization": auth,
    })
    with urllib.request.urlopen(req) as resp:
        return resp.read()

ns = {"s3": "http://s3.amazonaws.com/doc/2006-03-01/"}
cells = allb = 0
token = None
while True:
    root = ET.fromstring(list_page(token))
    for contents in root.findall("s3:Contents", ns) or root.findall("Contents"):
        key_el = contents.find("s3:Key", ns) or contents.find("Key")
        size_el = contents.find("s3:Size", ns) or contents.find("Size")
        if key_el is None or key_el.text is None:
            continue
        size = int((size_el.text if size_el is not None else "0") or "0")
        allb += size
        if "/cells/" in key_el.text:
            cells += size
    truncated = root.findtext("s3:IsTruncated", default="false", namespaces=ns) or "false"
    if truncated.lower() != "true":
        break
    token = root.findtext("s3:NextContinuationToken", namespaces=ns) or root.findtext("NextContinuationToken")
    if not token:
        break
print(f"{allb}\t{cells}")
PY
}

du_bytes() {
  local path="$1"
  if [[ ! -e "$path" ]]; then
    echo 0
    return
  fi
  du -sk "$path" 2>/dev/null | awk '{print $1 * 1024}'
}

d1_blob_count() {
  local project="$1" version="$2" project_dir="$3"
  local bucket="s3://cellp-celld/${project}/${version}"
  local out="${EVIDENCE_DIR}/d1-multi-count-${version}.out"
  if ! celld d1 execute "$DATABASE" \
    --command 'SELECT count(*) AS n FROM blobs' \
    "$project_dir" \
    --bucket "$bucket" --endpoint "$S3_ENDPOINT" --region "$AWS_REGION" \
    >"$out" 2>&1; then
    echo -1
    return
  fi
  grep -Eo '[0-9]+' "$out" | tail -1 || echo -1
}

d1_branch_cli_ready() {
  celld d1 branch -h >/dev/null 2>&1
}

if ! d1_branch_cli_ready; then
  fail "celld d1 branch not available"
fi
if ! celld d1 import --help >/dev/null 2>&1; then
  fail "celld d1 import not available"
fi

mb_log "project=${PROJECT} parent=${PARENT} size=${SIZE_MB}MB branches=${BRANCH_COUNT}"

ensure_project "$PROJECT"
cleanup_e2e_versions "$PROJECT"

PARENT_DIR="${ARTIFACTS_DIR}/${PROJECT}/${PARENT}"
EXPORT="${PARENT_DIR}/seed.db"
SEED_WORK="${E2E_ROOT}/dev/data/d1-branch-multi/${PARENT}"
mkdir -p "$EVIDENCE_DIR" "$PARENT_DIR" "$SEED_WORK"

copy_d1_seed_bundle "$PARENT_DIR"

# --- 100 MB incompressible blobs in offshoot (parent export source) ---
python3 - "$SIZE_MB" "$SEED_WORK/seed.db" <<'PY'
import os, sqlite3, sys
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
for i in range(mb):
    con.execute("INSERT INTO blobs(data) VALUES (?)", (blob,))
con.execute("COMMIT")
con.close()
print(os.path.getsize(dest))
PY
seed_bytes="$(python3 -c 'import os,sys; print(os.path.getsize(sys.argv[1]))' "$SEED_WORK/seed.db")"
mb_record "seed_bytes" "$seed_bytes" "$(jq -nc --argjson mb "$SIZE_MB" '{mb:$mb}')"

offshoot -store "$OFFSHOOT_STORE" init 2>/dev/null || true
offshoot -store "$OFFSHOOT_STORE" create "$PROJECT" 2>/dev/null || true
CHECKOUT=$(offshoot -store "$OFFSHOOT_STORE" checkout "${PROJECT}@main")
if [[ -z "$CHECKOUT" || ! -f "$CHECKOUT" ]]; then
  fail "offshoot checkout ${PROJECT}@main failed"
fi
python3 - "$SEED_WORK/seed.db" "$CHECKOUT" <<'PY'
import os, shutil, sqlite3, sys
src, dst = sys.argv[1], sys.argv[2]
for suffix in ("", "-wal", "-shm"):
    p = dst + suffix
    if suffix:
        try:
            os.remove(p)
        except FileNotFoundError:
            pass
shutil.copyfile(src, dst)
con = sqlite3.connect(dst)
con.execute("PRAGMA journal_mode=OFF")
con.close()
PY
offshoot -store "$OFFSHOOT_STORE" checkpoint "${PROJECT}@main" "d1-multi-${PARENT}" \
  >>"${EVIDENCE_DIR}/d1-branch-multi-offshoot.log" 2>&1 || fail "offshoot checkpoint"

if ! offshoot -store "$OFFSHOOT_STORE" export "${PROJECT}@main" "$EXPORT" --force \
  >>"${EVIDENCE_DIR}/d1-branch-multi-offshoot.log" 2>&1; then
  fail "offshoot export"
fi
rm -f "${EXPORT}-wal" "${EXPORT}-shm"
mb_record "export_bytes" "$(python3 -c 'import os,sys; print(os.path.getsize(sys.argv[1]))' "$EXPORT")" "{}"

# --- parent via cellp orchestrator ---
t0=$(date +%s)
create_version "$PROJECT" "$PARENT" | jq -r .id >/dev/null
poll_version "$PROJECT" "$PARENT" ready 900 >/dev/null
parent_ready_s=$(( $(date +%s) - t0 ))
mb_record "parent_ready_s" "$parent_ready_s" "{}"

PARENT_BUCKET="s3://cellp-celld/${PROJECT}/${PARENT}"
read -r parent_all parent_cells < <(s3_prefix_bytes "$PARENT_BUCKET")
mb_record "parent_s3_all_bytes" "$parent_all" "{}"
mb_record "parent_s3_cells_bytes" "$parent_cells" "{}"

PARENT_WATCH="${E2E_ROOT}/dev/data/celld-watch/${PROJECT}/${PARENT}"
parent_watch=$(du_bytes "$PARENT_WATCH")
mb_record "parent_watch_bytes" "$parent_watch" "{}"

parent_blobs=$(d1_blob_count "$PROJECT" "$PARENT" "$PARENT_DIR")
if [[ "$parent_blobs" != "$SIZE_MB" ]]; then
  mb_fail "parent blob count=${parent_blobs} expected ${SIZE_MB}"
else
  mb_log "parent blobs OK=${parent_blobs}"
fi

# --- sibling branches ---
branch_s3_all=()
branch_s3_cells=()
branch_watch=()
branch_counts=()

for i in $(seq 0 $((BRANCH_COUNT - 1))); do
  label="${CHILD_LABELS[$i]}"
  child="$(unique_id)"
  CHILD_IDS+=("$child")
  child_dir="${ARTIFACTS_DIR}/${PROJECT}/${child}"
  mkdir -p "$child_dir"
  copy_d1_seed_bundle "$child_dir"

  t0=$(date +%s)
  create_version "$PROJECT" "$child" "$PARENT" | jq -r .id >/dev/null
  poll_version "$PROJECT" "$child" ready 600 >/dev/null
  mb_record "branch_${label}_ready_s" "$(( $(date +%s) - t0 ))" "$(jq -nc --arg label "$label" '{label:$label}')"

  child_bucket="s3://cellp-celld/${PROJECT}/${child}"
  read -r allb cellsb < <(s3_prefix_bytes "$child_bucket")
  branch_s3_all+=("$allb")
  branch_s3_cells+=("$cellsb")
  mb_record "branch_${label}_s3_all_bytes" "$allb" '{"phase":"post-branch"}'
  mb_record "branch_${label}_s3_cells_bytes" "$cellsb" '{"phase":"post-branch"}'

  watch_path="${E2E_ROOT}/dev/data/celld-watch/${PROJECT}/${child}"
  wb=$(du_bytes "$watch_path")
  branch_watch+=("$wb")
  mb_record "branch_${label}_watch_bytes" "$wb" "{}"

  base_count=$(d1_blob_count "$PROJECT" "$child" "$child_dir")
  if [[ "$base_count" != "$SIZE_MB" ]]; then
    mb_fail "branch ${label} baseline blobs=${base_count} expected ${SIZE_MB}"
    branch_counts+=("$base_count")
    continue
  fi

  # Each branch adds one 1 MB row (distinct id).
  new_id=$((SIZE_MB + i + 1))
  celld d1 execute "$DATABASE" \
    --command "INSERT INTO blobs(id, data) SELECT ${new_id}, data FROM blobs WHERE id=1" \
    "$child_dir" \
    --bucket "$child_bucket" --endpoint "$S3_ENDPOINT" --region "$AWS_REGION" \
    >>"${EVIDENCE_DIR}/d1-branch-multi-insert-${label}.log" 2>&1 \
    || mb_fail "branch ${label} INSERT"

  after=$(d1_blob_count "$PROJECT" "$child" "$child_dir")
  branch_counts+=("$after")
  mb_record "branch_${label}_blob_count" "$after" "$(jq -nc --argjson expected $((SIZE_MB + 1)) '{expected:$expected}')"
  if [[ "$after" != "$((SIZE_MB + 1))" ]]; then
    mb_fail "branch ${label} after INSERT=${after}"
  else
    mb_log "branch ${label} INSERT OK count=${after}"
  fi

  read -r allb cellsb < <(s3_prefix_bytes "$child_bucket")
  mb_record "branch_${label}_s3_cells_post_insert" "$cellsb" '{"phase":"post-insert"}'
done

# --- isolation: parent unchanged; siblings cannot see each other's rows ---
parent_after=$(d1_blob_count "$PROJECT" "$PARENT" "$PARENT_DIR")
if [[ "$parent_after" != "$SIZE_MB" ]]; then
  mb_fail "parent after branch writes=${parent_after} expected ${SIZE_MB}"
else
  mb_log "parent isolation OK blobs=${parent_after}"
fi

for i in $(seq 0 $((BRANCH_COUNT - 1))); do
  label="${CHILD_LABELS[$i]}"
  child="${CHILD_IDS[$i]}"
  child_dir="${ARTIFACTS_DIR}/${PROJECT}/${child}"
  child_bucket="s3://cellp-celld/${PROJECT}/${child}"
  for j in $(seq 0 $((BRANCH_COUNT - 1))); do
    [[ "$i" == "$j" ]] && continue
    other_label="${CHILD_LABELS[$j]}"
    other_id=$((SIZE_MB + j + 1))
    out="${EVIDENCE_DIR}/d1-multi-iso-${label}-vs-${other_label}.out"
    celld d1 execute "$DATABASE" \
      --command "SELECT count(*) AS n FROM blobs WHERE id=${other_id}" \
      "$child_dir" \
      --bucket "$child_bucket" --endpoint "$S3_ENDPOINT" --region "$AWS_REGION" \
      >"$out" 2>&1 || true
    seen=$(grep -Eo '[0-9]+' "$out" | tail -1 || echo 1)
    if [[ "$seen" != "0" ]]; then
      mb_fail "branch ${label} sees branch ${other_label} row id=${other_id} (count=${seen})"
    fi
  done
done
mb_log "sibling isolation matrix OK"

# --- totals ---
s3_total=$parent_all
for b in "${branch_s3_all[@]}"; do s3_total=$((s3_total + b)); done
naive_s3=$(( (SIZE_MB * 1024 * 1024) * (1 + BRANCH_COUNT) ))
watch_total=$parent_watch
for w in "${branch_watch[@]}"; do watch_total=$((watch_total + w)); done
naive_watch=$(( (SIZE_MB * 1024 * 1024) * (1 + BRANCH_COUNT) ))

mb_record "s3_total_bytes" "$s3_total" "$(jq -nc --argjson naive "$naive_s3" '{naive_full_copy:$naive}')"
mb_record "watch_total_bytes" "$watch_total" "$(jq -nc --argjson naive "$naive_watch" '{naive_per_version:$naive}')"

result="PASS"
if (( FAILS > 0 )); then result="FAIL"; fi

jq -n \
  --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg result "$result" \
  --arg project "$PROJECT" \
  --arg parent "$PARENT" \
  --argjson size_mb "$SIZE_MB" \
  --argjson branch_count "$BRANCH_COUNT" \
  --argjson fails "$FAILS" \
  --argjson parent_s3_all "$parent_all" \
  --argjson parent_s3_cells "$parent_cells" \
  --argjson parent_watch "$parent_watch" \
  --argjson parent_blobs "$parent_blobs" \
  --argjson s3_total "$s3_total" \
  --argjson naive_s3 "$naive_s3" \
  --argjson watch_total "$watch_total" \
  --argjson naive_watch "$naive_watch" \
  --argjson children "$(printf '%s\n' "${CHILD_IDS[@]}" | jq -R . | jq -s .)" \
  --argjson branch_s3_all "$(printf '%s\n' "${branch_s3_all[@]}" | jq -R 'tonumber' | jq -s .)" \
  --argjson branch_s3_cells "$(printf '%s\n' "${branch_s3_cells[@]}" | jq -R 'tonumber' | jq -s .)" \
  --argjson branch_watch "$(printf '%s\n' "${branch_watch[@]}" | jq -R 'tonumber' | jq -s .)" \
  --argjson branch_counts "$(printf '%s\n' "${branch_counts[@]}" | jq -R 'tonumber' | jq -s .)" \
  '{
    ts: $ts,
    result: $result,
    project: $project,
    size_mb: $size_mb,
    branch_count: $branch_count,
    fails: $fails,
    parent: {
      version: $parent,
      s3_all_bytes: $parent_s3_all,
      s3_cells_bytes: $parent_s3_cells,
      watch_bytes: $parent_watch,
      blob_rows: $parent_blobs
    },
    branches: [
      range(0; $branch_count) as $i |
      {
        label: (["A","B","C","D","E","F","G","H"][$i]),
        version: $children[$i],
        s3_all_bytes: $branch_s3_all[$i],
        s3_cells_bytes: $branch_s3_cells[$i],
        watch_bytes: $branch_watch[$i],
        blob_rows: $branch_counts[$i]
      }
    ],
    totals: {
      s3_all_bytes: $s3_total,
      naive_full_copy_s3_bytes: $naive_s3,
      s3_savings_pct: (if $naive_s3 > 0 then (1 - ($s3_total / $naive_s3)) * 100 else 0 end),
      watch_bytes: $watch_total,
      naive_per_version_watch_bytes: $naive_watch
    }
  }' >"$REPORT_JSON"

mb_log "report ${REPORT_JSON}"
if (( FAILS > 0 )); then
  echo "FAIL: d1 branch multi (${FAILS} checks)" >&2
  exit 1
fi
pass "d1 branch multi ${SIZE_MB}MB parent=${PARENT} branches=${BRANCH_COUNT} s3_total=${s3_total}"
