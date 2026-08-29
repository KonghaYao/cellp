#!/usr/bin/env bash
# Phase 6A-T5 — seed M metadata versions per scale-seed project (SQLite bulk insert)
#
# Full POST /versions triggers deploy orchestration — too slow for 10k+ rows.
# This script inserts destroyed metadata rows directly for registry scale tests.
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib/common.sh"

scale_source_env
stress_require_tools
scale_require_sqlite

SCALE_SEED_VERSIONS="${SCALE_SEED_VERSIONS:-100}"
SCALE_SEED_PROJECTS="${SCALE_SEED_PROJECTS:-10}"
SCALE_SEED_PROJECT_START="${SCALE_SEED_PROJECT_START:-1}"
SCALE_VERSION_BATCH="${SCALE_VERSION_BATCH:-500}"
SCALE_VERSION_STATUS="${SCALE_VERSION_STATUS:-destroyed}"

usage() {
  cat <<EOF
Usage: $(basename "$0") [options]

  Seed metadata-only version rows for scale-seed-* projects.

Options:
  -n N    Versions per project (default: SCALE_SEED_VERSIONS=${SCALE_SEED_VERSIONS})
  -p P    Number of projects (default: SCALE_SEED_PROJECTS=${SCALE_SEED_PROJECTS})
  -s S    Starting project index (default: ${SCALE_SEED_PROJECT_START})
  -h      Show this help

Env: SCALE_SEED_VERSIONS, SCALE_SEED_PROJECTS, STRESS_RUN_ID, REGISTRY_DB
Safety: only projects matching prefix '${STRESS_PROJECT_PREFIX}' are touched.
EOF
}

while getopts ":n:p:s:h" opt; do
  case "$opt" in
    n) SCALE_SEED_VERSIONS="$OPTARG" ;;
    p) SCALE_SEED_PROJECTS="$OPTARG" ;;
    s) SCALE_SEED_PROJECT_START="$OPTARG" ;;
    h) usage; exit 0 ;;
    *) usage; exit 1 ;;
  esac
done

scale_log "seed-versions: ${SCALE_SEED_PROJECTS} projects × ${SCALE_SEED_VERSIONS} versions status=${SCALE_VERSION_STATUS}"

total_inserted=0
project_end=$((SCALE_SEED_PROJECT_START + SCALE_SEED_PROJECTS - 1))

for ((pidx = SCALE_SEED_PROJECT_START; pidx <= project_end; pidx++)); do
  project_id="$(scale_project_id "$pidx")"
  stress_assert_prefix "$project_id"

  # Ensure project row exists
  sqlite3 "$REGISTRY_DB" \
    "INSERT OR IGNORE INTO projects (id, created_at) VALUES ('${project_id}', datetime('now'));" 2>/dev/null || true

  inserted=0
  for ((vstart = 1; vstart <= SCALE_SEED_VERSIONS; vstart += SCALE_VERSION_BATCH)); do
    vend=$((vstart + SCALE_VERSION_BATCH - 1))
    if (( vend > SCALE_SEED_VERSIONS )); then vend=$SCALE_SEED_VERSIONS; fi

    sql="BEGIN;"
    for ((vidx = vstart; vidx <= vend; vidx++)); do
      vid="$(scale_version_id "$pidx" "$vidx")"
      ts="$(date -u +%Y-%m-%dT%H:%M:%S.000000000Z)"
      sql+="INSERT OR IGNORE INTO versions (id, project_id, git_ref, git_sha, status, created_at, updated_at)"
      sql+=" VALUES ('${vid}', '${project_id}', 'scale-seed', 'scale-seed', '${SCALE_VERSION_STATUS}', '${ts}', '${ts}');"
    done
    sql+="COMMIT;"
    sqlite3 "$REGISTRY_DB" "$sql"
    inserted=$((inserted + vend - vstart + 1))
  done

  total_inserted=$((total_inserted + inserted))
  scale_log "  ${project_id}: ${inserted} versions"
done

scale_record_metric "TP6-A5-seed" "versions_inserted" "$total_inserted" \
  "{\"projects\":$SCALE_SEED_PROJECTS,\"per_project\":$SCALE_SEED_VERSIONS,\"run_id\":\"${STRESS_RUN_ID}\"}"

stress_pass "seed-versions: ${total_inserted} rows (${SCALE_SEED_PROJECTS}×${SCALE_SEED_VERSIONS}) status=${SCALE_VERSION_STATUS}"
