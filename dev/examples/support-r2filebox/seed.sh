#!/usr/bin/env bash
# D1 from worker/migrations/*.sql (order by name).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
CORPUS="${ROOT}/dev/support-corpus/support-r2filebox"
OUT="${1:?seed.db path}"
rm -f "$OUT"
sqlite3 "$OUT" 'PRAGMA journal_mode=DELETE;'
for f in "${CORPUS}"/worker/migrations/*.sql; do
  [[ -f "$f" ]] || continue
  sqlite3 "$OUT" < "$f"
done
echo "seed.db tables=$(sqlite3 "$OUT" ".tables" | wc -w | tr -d ' ')"
