#!/usr/bin/env bash
# Bundle official @opennextjs/cloudflare e2e fixture for cellp preview (no app source edits).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
APP_DIR="${1:?app dir}"
: "${CELLP_ONCF_PROJECT:?set CELLP_ONCF_PROJECT}"
: "${CELLP_ONCF_DEPLOY_URL:?set CELLP_ONCF_DEPLOY_URL}"

cd "$APP_DIR"
[[ -f wrangler.jsonc ]] || { echo "missing wrangler.jsonc" >&2; exit 1; }
GIT_ROOT="$(git rev-parse --show-toplevel)"
GIT_PREFIX="$(git rev-parse --show-prefix)"
WRANGLER_BACKUP="$(mktemp)"
git -C "$GIT_ROOT" show "HEAD:${GIT_PREFIX}wrangler.jsonc" > "$WRANGLER_BACKUP"
cp "$WRANGLER_BACKUP" wrangler.jsonc
restore_wrangler() {
  cp "$WRANGLER_BACKUP" wrangler.jsonc
  rm -f "$WRANGLER_BACKUP"
}
trap restore_wrangler EXIT

# Reuse only OpenNext's wrangler dry-run and cellp staging. The official suite
# must not receive S30/S40 generated-bundle patches or image configuration
# workarounds: those would change the artifact under test and contaminate the
# upstream assertions — unless CELLP_ONCF_COMPAT_PATCH=1 (lab compat tier).
PATCH_ARGS=(CELLP_OPENNEXT_SKIP_PATCH=1 CELLP_OPENNEXT_SKIP_NEXT_CONFIG_PATCH=1)
if [[ "${CELLP_ONCF_COMPAT_PATCH:-0}" == "1" ]]; then
  echo "prepare-opennext-official: compat patch tier (S30 bundle patches on; next.config patch still skipped)" >&2
  PATCH_ARGS=(CELLP_OPENNEXT_SKIP_NEXT_CONFIG_PATCH=1)
fi
env "${PATCH_ARGS[@]}" bash "${ROOT}/dev/examples/support-opennext/prepare-artifact.sh" "$APP_DIR"

cd "$APP_DIR"
export CELLP_ONCF_PROJECT CELLP_ONCF_DEPLOY_URL
log() { echo "prepare-opennext-official: $*"; }

log "rewrite wrangler for cellp project ${CELLP_ONCF_PROJECT} (preserve bindings)"
node <<'NODE'
const fs = require('fs');
const project = process.env.CELLP_ONCF_PROJECT;
const deployUrl = process.env.CELLP_ONCF_DEPLOY_URL;
const p = 'wrangler.jsonc';
let raw = fs.readFileSync(p, 'utf8');
let j;
try {
  j = JSON.parse(raw);
} catch {
  raw = raw.replace(/^\s*\/\/.*$/gm, '').replace(/\/\*[\s\S]*?\*\//g, '').replace(/,\s*([}\]])/g, '$1');
  j = JSON.parse(raw);
}
j.name = project;
if (Array.isArray(j.services)) {
  for (const s of j.services) {
    if (s && typeof s.service === 'string') s.service = project;
  }
}
j.vars = j.vars || {};
j.vars.DEPLOY_URL = deployUrl;
j.vars.PUBLIC_BASE_URL = deployUrl;
if (Array.isArray(j.compatibility_flags)) {
  j.compatibility_flags = j.compatibility_flags.filter((f) => f !== 'global_fetch_strictly_public');
}
if (!j.compatibility_flags.includes('nodejs_compat')) {
  j.compatibility_flags.push('nodejs_compat');
}
delete j.$schema;
delete j.observability;
delete j.upload_source_maps;
let out = JSON.stringify(j, null, 2) + '\n';
out = out.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync('.cellp-wrangler.jsonc', out);
NODE

log "ok: ${CELLP_ONCF_PROJECT} artifact (.cellp-bundle + .cellp-assets + wrangler bindings)"
