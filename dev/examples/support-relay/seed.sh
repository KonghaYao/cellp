#!/usr/bin/env bash
# Build Relay D1 seed.db (schema + demo short links).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
RELAY="${ROOT}/dev/support-corpus/support-relay"
OUT="${1:?output seed.db path}"

sqlite3 "$OUT" <<'SQL'
PRAGMA journal_mode = DELETE;
SQL

sqlite3 "$OUT" < "${RELAY}/schema.sql"

sqlite3 "$OUT" <<'SQL'
INSERT INTO links (slug, title, mode, target_url, redirect_type, active, created_at, updated_at) VALUES
  ('demo', 'Example.com', 'simple', 'https://example.com/', 302, 1, datetime('now'), datetime('now')),
  ('cellp', 'cellp 文档', 'simple', 'https://konghayao.github.io/cellp/', 302, 1, datetime('now'), datetime('now')),
  ('relay', 'Relay 上游仓库', 'simple', 'https://github.com/YuriCrystal/relay', 302, 1, datetime('now'), datetime('now'));
SQL

echo "seed.db links=$(sqlite3 "$OUT" 'SELECT count(*) FROM links;')"
