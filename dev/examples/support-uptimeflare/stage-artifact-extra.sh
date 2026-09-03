#!/usr/bin/env bash
set -euo pipefail
APP_DIR="${1:?app dir}"
DEST="${2:?dest dir}"
if [[ -d "${APP_DIR}/.cellp-assets" ]]; then
  rsync -a "${APP_DIR}/.cellp-assets/" "${DEST}/.cellp-assets/"
fi
if [[ -d "${APP_DIR}/dist/_worker.js" ]]; then
  mkdir -p "${DEST}/dist"
  rsync -a "${APP_DIR}/dist/_worker.js/" "${DEST}/dist/_worker.js/"
fi
if [[ -d "${APP_DIR}/migrations" ]]; then
  rsync -a "${APP_DIR}/migrations/" "${DEST}/migrations/"
fi
