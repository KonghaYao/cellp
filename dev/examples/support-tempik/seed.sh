#!/usr/bin/env bash
# D1 schema for Tempik (empty inboxes; Create will populate).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
OUT="${1:?seed.db path}"
sqlite3 "$OUT" <<'SQL'
PRAGMA journal_mode = DELETE;
SQL
sqlite3 "$OUT" < "${ROOT}/dev/support-corpus/support-tempik/src/db/schema.sql"
echo "seed.db ok (schema only)"
