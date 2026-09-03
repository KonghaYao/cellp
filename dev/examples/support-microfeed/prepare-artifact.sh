#!/usr/bin/env bash
# microfeed: Astro @astrojs/cloudflare → dist/server/entry.mjs + dist/client assets; D1 + Queues + R2.
set -euo pipefail
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
cd "$APP_DIR"

log() { echo "prepare-artifact: $*"; }

export NPM_CONFIG_REGISTRY="${NPM_CONFIG_REGISTRY:-https://registry.npmmirror.com}"
export npm_config_registry="${npm_config_registry:-$NPM_CONFIG_REGISTRY}"
export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-false}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-false}"

WORKER_ENTRY="dist/server/entry.mjs"

if [[ ! -f "$WORKER_ENTRY" ]]; then
  log "yarn install + astro build"
  corepack enable 2>/dev/null || true
  yarn install --immutable 2>/dev/null || yarn install
  node <<'NODE'
const fs = require('fs');
const p = 'wrangler.jsonc';
let raw = fs.readFileSync(p, 'utf8');
raw = raw.replace(/\/\/[^\n]*/g, '').replace(/,\s*([}\]])/g, '$1');
const j = JSON.parse(raw);
j.main = './src/worker.ts';
fs.writeFileSync(p, JSON.stringify(j, null, 2) + '\n');
NODE
  yarn build
fi

[[ -f "$WORKER_ENTRY" ]] || { echo "missing ${WORKER_ENTRY}" >&2; exit 1; }

log "stage static assets → .cellp-assets"
rm -rf .cellp-assets .cellp-bundle
mkdir -p .cellp-assets
rsync -a dist/client/ .cellp-assets/
rm -f .cellp-assets/.assetsignore

node <<'NODE'
const fs = require('fs');
const p = 'wrangler.jsonc';
let raw = fs.readFileSync(p, 'utf8');
raw = raw.replace(/\/\/[^\n]*/g, '').replace(/,\s*([}\]])/g, '$1');
const j = JSON.parse(raw);
j.main = 'dist/server/entry.mjs';
delete j.no_bundle;
delete j.cache;
delete j.observability;
delete j.version_metadata;
delete j.secrets;
delete j.routes;
delete j.triggers;
j.assets = j.assets || {};
j.assets.directory = '.cellp-assets';
j.assets.binding = j.assets.binding || 'ASSETS';
j.assets.html_handling = j.assets.html_handling || 'auto-trailing-slash';
if (!j.d1_databases?.length) {
  j.d1_databases = [
    {
      binding: 'FEED_DB',
      database_name: 'microfeed-feed',
      database_id: '00000000-0000-0000-0000-00000000000e',
      migrations_dir: './migrations',
    },
  ];
} else {
  for (const db of j.d1_databases) {
    db.migrations_dir = './migrations';
  }
}
if (!j.r2_buckets?.length) {
  j.r2_buckets = [
    { binding: 'MEDIA_BUCKET', bucket_name: 'support-microfeed-media' },
  ];
}
if (!j.queues) {
  j.queues = {
    producers: [
      { binding: 'WEBHOOK_QUEUE', queue: 'support-microfeed-webhooks' },
    ],
    consumers: [
      {
        queue: 'support-microfeed-webhooks',
        max_batch_size: 1,
        max_batch_timeout: 1,
        max_retries: 5,
      },
    ],
  };
}
j.vars = {
  ...(j.vars || {}),
  UPLOAD_SIGNING_KEY:
    j.vars?.UPLOAD_SIGNING_KEY || 'cellp-microfeed-upload-signing-key-32b',
  BETTER_AUTH_SECRET:
    j.vars?.BETTER_AUTH_SECRET || 'cellp-microfeed-better-auth-secret-32',
  WEBHOOK_SECRET_KEY:
    j.vars?.WEBHOOK_SECRET_KEY || 'cellp-microfeed-webhook-secret-key-32',
};
raw = JSON.stringify(j, null, 2) + '\n';
raw = raw.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync(p, raw);
NODE

log "wrangler dry-run bundle"
rm -rf .cellp-bundle
npx --yes wrangler@4 deploy --config wrangler.jsonc --dry-run --outdir .cellp-bundle
if [[ -f .cellp-bundle/entry.js && ! -f .cellp-bundle/index.js ]]; then
  cp .cellp-bundle/entry.js .cellp-bundle/index.js
fi
[[ -f .cellp-bundle/index.js ]] || { echo "missing .cellp-bundle/index.js" >&2; exit 1; }

node <<'NODE'
const fs = require('fs');
const p = 'wrangler.jsonc';
let raw = fs.readFileSync(p, 'utf8');
raw = raw.replace(/\/\/[^\n]*/g, '').replace(/,\s*([}\]])/g, '$1');
const j = JSON.parse(raw);
j.main = '.cellp-bundle/index.js';
j.no_bundle = true;
raw = JSON.stringify(j, null, 2) + '\n';
raw = raw.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync(p, raw);
NODE

log "ok: microfeed .cellp-bundle/index.js ($(wc -c < .cellp-bundle/index.js | tr -d ' ') bytes)"
