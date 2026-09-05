#!/usr/bin/env bash
# vercel/next.js examples/hello-world + @opennextjs/cloudflare (pinned like S30).
set -euo pipefail
APP_DIR="${1:?app dir}"
OVERLAY="$(cd "$(dirname "$0")" && pwd)"
CORE="${OVERLAY}/../support-opennext/prepare-artifact.sh"

log() { echo "prepare-artifact: $*"; }

cp "${OVERLAY}/open-next.config.ts" "${APP_DIR}/"
node "${OVERLAY}/pin-package.cjs" "${APP_DIR}/package.json"
rm -f "${APP_DIR}/package-lock.json"

FIXTURE_SRC="${OVERLAY}/cellp-app"
if [[ -d "${FIXTURE_SRC}" ]]; then
  log "copy cellp overlay fixtures → ${APP_DIR}"
  rsync -a "${FIXTURE_SRC}/" "${APP_DIR}/"
  rm -rf "${APP_DIR}/pages"
  STAMP="$(find "${FIXTURE_SRC}" -type f -print0 | sort -z | xargs -0 shasum 2>/dev/null | shasum | awk '{print $1}')"
  OLD=""
  [[ -f "${APP_DIR}/.cellp-fixture-stamp" ]] && OLD="$(cat "${APP_DIR}/.cellp-fixture-stamp")"
  if [[ "${STAMP}" != "${OLD}" ]]; then
    log "fixture stamp changed (${OLD:-∅} → ${STAMP}); clean build outputs"
    rm -rf "${APP_DIR}/.next" "${APP_DIR}/.open-next" "${APP_DIR}/.cellp-bundle" "${APP_DIR}/.cellp-assets"
    echo "${STAMP}" > "${APP_DIR}/.cellp-fixture-stamp"
  fi
fi

exec bash "${CORE}" "${APP_DIR}"
