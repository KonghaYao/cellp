#!/usr/bin/env bash
# Stage Drizzle SQL migrations for apply-version-d1-migrations.sh (slim artifact).
set -euo pipefail
APP_DIR="${1:?app dir}"
DEST="${2:?dest}"
if [[ -d "${APP_DIR}/drizzle" ]]; then
  rsync -a "${APP_DIR}/drizzle/" "${DEST}/drizzle/"
fi
