#!/usr/bin/env bash
set -euo pipefail
APP_DIR="${1:?app dir}"
DEST="${2:?dest}"
[[ -d "${APP_DIR}/.cellp-assets" ]] && rsync -a "${APP_DIR}/.cellp-assets/" "${DEST}/.cellp-assets/"
[[ -d "${APP_DIR}/dist/support_workflows" ]] && mkdir -p "${DEST}/dist/support_workflows" && rsync -a "${APP_DIR}/dist/support_workflows/" "${DEST}/dist/support_workflows/"
