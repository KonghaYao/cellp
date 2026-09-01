#!/usr/bin/env bash
# Build FlareMo D1 seed.db from drizzle migrations/*.sql for cellp D1Execute.
set -euo pipefail
OUT="${1:?output seed.db path}"
CORPUS="${2:-$(cd "$(dirname "$0")/../../support-corpus/support-flaremo" && pwd)}"
MIG="${CORPUS}/migrations"

rm -f "$OUT" "${OUT}"-wal "${OUT}"-shm
sqlite3 "$OUT" "PRAGMA journal_mode=DELETE;"

for f in $(ls "$MIG"/[0-9]*.sql 2>/dev/null | sort); do
  sed 's/--> statement-breakpoint//g' "$f" | sqlite3 "$OUT"
done

echo "seed.db tables=$(sqlite3 "$OUT" "SELECT count(*) FROM sqlite_master WHERE type='table';")"
