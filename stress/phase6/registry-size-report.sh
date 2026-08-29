#!/usr/bin/env bash
# Phase 6A — registry size + row counts (+ optional ListProjects query plan)
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib/common.sh"

scale_source_env
stress_need sqlite3
scale_require_sqlite

SCALE_EXPLAIN="${SCALE_EXPLAIN:-0}"

db_bytes="$(stress_sqlite_bytes)"
db_mb="$(awk -v b="$db_bytes" 'BEGIN { printf "%.2f", b/1024/1024 }')"

count_rows() {
  local table="$1"
  sqlite3 "$REGISTRY_DB" "SELECT count(*) FROM ${table};" 2>/dev/null || echo "0"
}

projects="$(count_rows projects)"
versions="$(count_rows versions)"
jobs="$(count_rows jobs)"
routes="$(count_rows routes)"
routes_active="$(sqlite3 "$REGISTRY_DB" "SELECT count(*) FROM routes WHERE active=1;" 2>/dev/null || echo "0")"

explain_list_projects() {
  local limit="${SCALE_EXPLAIN_LIMIT:-51}"
  local cursor_at cursor_id
  cursor_at="$(sqlite3 "$REGISTRY_DB" \
    "SELECT created_at FROM projects ORDER BY created_at ASC, id ASC LIMIT 1 OFFSET 50;" 2>/dev/null || true)"
  cursor_id="$(sqlite3 "$REGISTRY_DB" \
    "SELECT id FROM projects ORDER BY created_at ASC, id ASC LIMIT 1 OFFSET 50;" 2>/dev/null || true)"

  scale_log "EXPLAIN QUERY PLAN — ListProjects (first page, limit=${limit})"
  sqlite3 "$REGISTRY_DB" "EXPLAIN QUERY PLAN
SELECT p.id, p.git_remote, p.prod_version_id, p.created_at
FROM projects p
ORDER BY p.created_at ASC, p.id ASC
LIMIT ${limit};" 2>/dev/null || true

  if [[ -n "$cursor_at" && -n "$cursor_id" ]]; then
    scale_log "EXPLAIN QUERY PLAN — ListProjects (cursor page, limit=${limit})"
    sqlite3 "$REGISTRY_DB" "EXPLAIN QUERY PLAN
SELECT p.id, p.git_remote, p.prod_version_id, p.created_at
FROM projects p
WHERE p.created_at > '${cursor_at}' OR (p.created_at = '${cursor_at}' AND p.id > '${cursor_id}')
ORDER BY p.created_at ASC, p.id ASC
LIMIT ${limit};" 2>/dev/null || true
  fi
}

scale_log "registry-size-report: ${REGISTRY_DB}"

cat <<EOF

=== TP6-A5 Registry Size Report ===
path:           ${REGISTRY_DB}
size_bytes:     ${db_bytes}
size_mb:        ${db_mb}
projects:       ${projects}
versions:       ${versions}
jobs:           ${jobs}
routes:         ${routes}
routes_active:  ${routes_active}
EOF

if [[ "$SCALE_EXPLAIN" == "1" ]]; then
  explain_list_projects
fi

scale_record_metric "TP6-A5-report" "registry_snapshot" "$db_bytes" \
  "{\"db_mb\":${db_mb},\"projects\":${projects},\"versions\":${versions},\"jobs\":${jobs},\"routes\":${routes},\"routes_active\":${routes_active}}"

scale_log "metric appended -> ${SCALE_METRICS}"
exit 0
