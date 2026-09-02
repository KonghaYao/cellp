#!/usr/bin/env bash
set -euo pipefail
APP_DIR="${1:?app dir}"
DEST="${2:?dest dir}"
if [[ -d "${APP_DIR}/.cellp-assets" ]]; then
  rsync -a "${APP_DIR}/.cellp-assets/" "${DEST}/.cellp-assets/"
fi
