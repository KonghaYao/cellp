#!/usr/bin/env bash
# CloudFlare-ImgBed Workers path (deploy/worker): generate routes + wrangler dry-run slim bundle.
set -euo pipefail
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
cd "$APP_DIR"
REPO_ROOT="$(cd ../.. && pwd)"

log() { echo "prepare-artifact: $*"; }

if [[ ! -d "${REPO_ROOT}/functions" ]]; then
  echo "prepare-artifact: missing repo functions/ (expected ${REPO_ROOT})" >&2
  exit 1
fi

if [[ ! -f wrangler.jsonc ]]; then
  echo "prepare-artifact: missing wrangler.jsonc" >&2
  exit 1
fi

log "generate worker routes"
node "${REPO_ROOT}/deploy/worker/generate-routes.js"

[[ -d "${REPO_ROOT}/frontend-dist" ]] || {
  echo "prepare-artifact: missing frontend-dist" >&2
  exit 1
}

log "wrangler dry-run bundle"
rm -rf .cellp-bundle
npx --yes wrangler@4 deploy --config wrangler.jsonc --dry-run --outdir .cellp-bundle
if [[ -f .cellp-bundle/_worker.js && ! -f .cellp-bundle/index.js ]]; then
  cp .cellp-bundle/_worker.js .cellp-bundle/index.js
fi
[[ -f .cellp-bundle/index.js ]] || { echo "prepare-artifact: missing .cellp-bundle/index.js" >&2; exit 1; }

log "stage static assets"
rm -rf .cellp-assets
mkdir -p .cellp-assets
rsync -a "${REPO_ROOT}/frontend-dist/" .cellp-assets/

node <<'NODE'
const fs = require('fs');

const routesPath = '.cellp-assets/_routes.json';
// celld treats missing _routes.json as worker-first `/*`, forcing SPA `/` through
// env.ASSETS.fetch inside the worker (loopback stall). Limit worker-first paths.
fs.writeFileSync(
  routesPath,
  JSON.stringify(
    {
      version: 1,
      include: [
        '/api/*',
        '/file/*',
        '/upload',
        '/upload/*',
        '/random',
        '/random/*',
        '/dav/*',
      ],
    },
    null,
    2,
  ) + '\n',
);

const p = 'wrangler.jsonc';
let raw = fs.readFileSync(p, 'utf8');
let j;
try {
  j = JSON.parse(raw);
} catch {
  raw = raw.replace(/^\s*\/\/.*$/gm, '').replace(/,\s*([}\]])/g, '$1');
  j = JSON.parse(raw);
}
j.main = '.cellp-bundle/index.js';
j.no_bundle = true;
j.assets = j.assets || {};
j.assets.directory = '.cellp-assets';
j.assets.binding = j.assets.binding || 'ASSETS';
j.assets.not_found_handling = j.assets.not_found_handling || 'single-page-application';
j.vars = j.vars || {};
j.vars.disable_telemetry = 'true';
// Keep `images` binding for celld FEATURE_IMAGES_V1 (prepare used to strip it).
raw = JSON.stringify(j, null, 2) + '\n';
raw = raw.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync(p, raw);

const bundlePath = '.cellp-bundle/index.js';
let code = fs.readFileSync(bundlePath, 'utf8');
let patched = 0;

const sentryEh =
  'async function errorHandling(context) {\n  const othersConfig3 = await fetchOthersConfig(context.env);';
if (code.includes(sentryEh)) {
  code = code.replace(
    sentryEh,
    'async function errorHandling(context) {\n  return context.next();\n  const othersConfig3 = await fetchOthersConfig(context.env);',
  );
  patched++;
}

const sentryTd =
  'async function telemetryData(context) {\n  const othersConfig3 = await fetchOthersConfig(context.env);';
if (code.includes(sentryTd)) {
  code = code.replace(
    sentryTd,
    'async function telemetryData(context) {\n  return context.next();\n  const othersConfig3 = await fetchOthersConfig(context.env);',
  );
  patched++;
}

const sampleNeedle = 'const url = "https://frozen-sentinel.pages.dev/signal/sampleRate.json";';
if (code.includes(sampleNeedle)) {
  code = code.replace(
    /async function fetchSampleRate\(context\) \{[\s\S]*?\n\}/,
    'async function fetchSampleRate(context) {\n  return null;\n}',
  );
  patched++;
}

const kvMiss =
  'if (!imgRecord) {\n    return new Response("Error: Image Not Found", { status: 404 });';
if (code.includes(kvMiss)) {
  code = code.replace(
    kvMiss,
    'if (!imgRecord || imgRecord.value == null && imgRecord.metadata == null) {\n    return new Response("Error: Image Not Found", { status: 404 });',
  );
  patched++;
}

const blockFetch =
  'const blockImg = await fetch(url.origin + "/static/media/BlockImg.png");';
if (code.includes(blockFetch)) {
  code = code.replace(
    blockFetch,
    'return new Response(null, { status: 302, headers: { Location: url.origin + "/blockimg", "Cache-Control": "public, max-age=86400" } });\n  const blockImg = await fetch(url.origin + "/static/media/BlockImg.png");',
  );
  patched++;
}

const whiteFetch =
  'const WhiteListImg = await fetch(url.origin + "/static/media/WhiteListOn.png");';
if (code.includes(whiteFetch)) {
  code = code.replace(
    whiteFetch,
    'return new Response(null, { status: 302, headers: { Location: url.origin + "/whiteliston", "Cache-Control": "public, max-age=86400" } });\n  const WhiteListImg = await fetch(url.origin + "/static/media/WhiteListOn.png");',
  );
  patched++;
}

const img404Fetch = 'const Img404 = await fetch(url.origin + "/static/media/404.png");';
if (code.includes(img404Fetch)) {
  code = code.replace(
    img404Fetch,
    'return new Response("Error: Image Not Found", { status: 404, headers: { "Cache-Control": "public, max-age=86400" } });\n  const Img404 = await fetch(url.origin + "/static/media/404.png");',
  );
  patched++;
}

const cacheFn = 'async function maybeServeFromCache(request, ctx, producer) {';
if (code.includes(cacheFn)) {
  code = code.replace(
    cacheFn,
    'async function maybeServeFromCache(request, ctx, producer) {\n  try {\n    const __p = new URL(request.url).pathname;\n    if (__p.startsWith("/file/")) return await producer();\n  } catch (_) {}\n',
  );
  patched++;
}

const fileHandler = 'async function onRequest44(context) {\n  const {';
if (code.includes(fileHandler)) {
  code = code.replace(
    fileHandler,
    'async function onRequest44(context) {\n  const __fileUrl = new URL(context.request.url);\n  let __fileId = __fileUrl.pathname.replace(/^\\/file\\/?/, "");\n  try { __fileId = decodeURIComponent(__fileId.split(",").join("/")); } catch { return new Response("Error: Decode Image ID Failed", { status: 400 }); }\n  if (!__fileId) return new Response("Error: Image Not Found", { status: 404 });\n  const __quick = await context.env.img_url.get(__fileId);\n  if (__quick == null || __quick === "") return new Response("Error: Image Not Found", { status: 404 });\n  const {',
  );
  patched++;
}

if (patched === 0) {
  console.error('prepare-artifact: bundle celld patches did not apply');
  process.exit(1);
}
fs.writeFileSync(bundlePath, code);
console.log(`prepare-artifact: applied ${patched} celld patch(es) to bundle`);
NODE

log "bundled $(wc -c < .cellp-bundle/index.js | tr -d ' ') bytes → wrangler no_bundle"
