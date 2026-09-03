#!/usr/bin/env bash
# Cloudflare templates next-starter-template (@opennextjs/cloudflare): prebuild + wrangler bundle + slim assets.
set -euo pipefail
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
cd "$APP_DIR"

log() { echo "prepare-artifact: $*"; }

export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-false}"
export npm_config_ignore_scripts="${npm_config_ignore_scripts:-false}"

if [[ ! -d node_modules ]]; then
  log "npm ci"
  npm ci
fi

if [[ ! -f .open-next/worker.js ]]; then
  log "next build"
  npm run build
  log "opennextjs-cloudflare build"
  npx opennextjs-cloudflare build
fi
[[ -f .open-next/worker.js ]] || { echo "missing .open-next/worker.js" >&2; exit 1; }

log "wrangler dry-run bundle"
rm -rf .cellp-bundle
# wrangler.jsonc may already point at .cellp-bundle from a prior run; dry-run needs .open-next/worker.js.
node <<'NODE'
const fs = require('fs');
const src = 'wrangler.jsonc';
let raw = fs.readFileSync(src, 'utf8');
let j;
try {
  j = JSON.parse(raw);
} catch {
  raw = raw.replace(/^\s*\/\/.*$/gm, '').replace(/\/\*[\s\S]*?\*\//g, '').replace(/,\s*([}\]])/g, '$1');
  j = JSON.parse(raw);
}
j.main = '.open-next/worker.js';
j.assets = j.assets || { binding: 'ASSETS', directory: '.open-next/assets' };
if (!j.assets.directory) j.assets.directory = '.open-next/assets';
if (!j.assets.binding) j.assets.binding = 'ASSETS';
delete j.no_bundle;
fs.writeFileSync('wrangler.cellp-dry-run.jsonc', JSON.stringify(j, null, 2) + '\n');
NODE
npx --yes wrangler@4 deploy --config wrangler.cellp-dry-run.jsonc --dry-run --outdir .cellp-bundle
rm -f wrangler.cellp-dry-run.jsonc
if [[ -f .cellp-bundle/worker.js && ! -f .cellp-bundle/index.js ]]; then
  cp .cellp-bundle/worker.js .cellp-bundle/index.js
fi
[[ -f .cellp-bundle/index.js ]] || { echo "missing .cellp-bundle/index.js" >&2; exit 1; }

log "patch Next slash redirect + Location: ? (celld absolute request.url)"
node <<'NODE'
const fs = require('fs');
const p = '.cellp-bundle/index.js';
let s = fs.readFileSync(p, 'utf8');
let patched = 0;

if (!s.includes('__cellpSlashPath')) {
  const needleA = `            let urlNoQuery = (req.url || "").split("?", 1)[0];
            if (urlNoQuery?.match(/(\\\\|\\/\\/)/)) {`;
  const needleB = `            let urlNoQuery = (req.url || "").split("?", 1)[0];
            if (urlNoQuery?.match(/(^\\/\\/|^\\\\\\\\)/)) {`;
  const slashRepl = `            let urlNoQuery = (req.url || "").split("?", 1)[0];
            let __cellpSlashPath = urlNoQuery;
            try { if (/^https?:\\/\\//i.test(urlNoQuery)) __cellpSlashPath = new URL(urlNoQuery).pathname; } catch {}
            if (__cellpSlashPath?.match(/(\\\\|\\/\\/)/)) {`;
  if (s.includes(needleA)) {
    s = s.replace(needleA, slashRepl);
    patched++;
  } else if (s.includes(needleB)) {
    s = s.replace(needleB, slashRepl);
    patched++;
  } else {
    console.error('prepare-artifact: handleRequestImpl slash-redirect needle not found');
    process.exit(1);
  }
}

const normLocRe = /function normalizeLocationHeader\(location2, baseUrl, encodeQuery = false\) \{[\s\S]*?\n\}/;
const normLocRepl = `function normalizeLocationHeader(location2, baseUrl, encodeQuery = false) {
  if (!URL.canParse(location2)) {
    return location2 === "?" || location2 === "//" || location2 === "" ? "/" : location2;
  }
  const locationURL = new URL(location2);
  const origin = new URL(baseUrl).origin;
  let search = locationURL.search;
  if (encodeQuery && search) {
    search = \`?\${stringifyQs(parseQs(search.slice(1)))}\`;
  }
  const href = \`\${locationURL.origin}\${locationURL.pathname}\${search}\${locationURL.hash}\`;
  if (locationURL.origin === origin) {
    let rel = href.slice(origin.length);
    if (!rel || rel === "?" || rel.startsWith("//")) rel = "/";
    return rel;
  }
  return href;
}`;
if (!normLocRe.test(s)) {
  console.error('prepare-artifact: normalizeLocationHeader function not found');
  process.exit(1);
}
if (!s.includes('rel.startsWith("//")')) {
  s = s.replace(normLocRe, normLocRepl);
  patched++;
}

const relParseRe =
  /href: href\.slice\(origin\.length\), slashes: void 0 \}/;
const relParseRepl = `href: (() => {
        const rel = href.slice(origin.length);
        return !rel || rel === "?" || rel.startsWith("//") ? "/" : rel;
      })(), slashes: void 0 }`;
if (relParseRe.test(s) && !s.includes('__cellpParseRel')) {
  s = s.replace(relParseRe, relParseRepl.replace('(() => {', '/* __cellpParseRel */ (() => {'));
  patched++;
}

const repeatedSlashFrom =
  'Location: normalizeRepeatedSlashes(new URL(event.url))';
const repeatedSlashTo =
  'Location: normalizeLocationHeader(normalizeRepeatedSlashes(new URL(event.url)), event.url)';
if (s.includes(repeatedSlashFrom) && !s.includes(repeatedSlashTo)) {
  s = s.replaceAll(repeatedSlashFrom, repeatedSlashTo);
  patched++;
}

const ifSlashFrom =
  'if (__cellpSlashPath?.match(/(\\\\|\\/\\/)/)) {';
const ifSlashTo =
  'if (__cellpSlashPath && __cellpSlashPath !== "/" && /(?:\\\\|\\/\\/)/.test(__cellpSlashPath)) {';
if (s.includes(ifSlashFrom) && !s.includes('__cellpSlashPath !== "/"')) {
  s = s.replace(ifSlashFrom, ifSlashTo);
  patched++;
}

const cleanUrlFrom = `let cleanUrl = (0, _utils.normalizeRepeatedSlashes)(req.url);
              res.redirect(cleanUrl, 308).body(cleanUrl).send();`;
const cleanUrlTo = `let cleanUrl = (__cellpSlashPath || "/").replace(/\\\\/g, "/").replace(/\\/\\/+/g, "/") || "/";
              if (!cleanUrl.startsWith("/")) cleanUrl = "/" + cleanUrl.replace(/^\\/+/, "");
              res.redirect(cleanUrl, 308).body(cleanUrl).send();`;
if (s.includes(cleanUrlFrom) && !s.includes('__cellpSlashPath || "/"')) {
  s = s.replace(cleanUrlFrom, cleanUrlTo);
  patched++;
}

const nrs2From = `      function normalizeRepeatedSlashes2(url) {
        let urlParts = url.split("?");
        return urlParts[0].replace(/\\\\/g, "/").replace(/\\/\\/+/g, "/") + (urlParts[1] ? \`?\${urlParts.slice(1).join("?")}\` : "");
      }`;
const nrs2To = `      function normalizeRepeatedSlashes2(url) {
        let urlParts = url.split("?");
        let pathPart = urlParts[0];
        if (/^https?:\\/\\//i.test(pathPart)) {
          try {
            const u = new URL(pathPart);
            u.pathname = (u.pathname || "/").replace(/\\\\/g, "/").replace(/\\/\\/+/g, "/") || "/";
            pathPart = u.origin + u.pathname;
          } catch {}
        } else {
          pathPart = pathPart.replace(/\\\\/g, "/").replace(/\\/\\/+/g, "/");
        }
        return pathPart + (urlParts[1] ? \`?\${urlParts.slice(1).join("?")}\` : "");
      }`;
if (s.includes(nrs2From) && !s.includes('u.origin + u.pathname')) {
  s = s.replace(nrs2From, nrs2To);
  patched++;
}

const reqUrlFrom =
  'req.url = initialURL.pathname + convertToQueryString2(routingResult.internalEvent.query), await requestHandler(requestMetadata)(req, res);';
const reqUrlTo =
  'req.url = (initialURL.pathname || "/") + convertToQueryString2(routingResult.internalEvent.query), await requestHandler(requestMetadata)(req, res);';
if (s.includes(reqUrlFrom) && !s.includes('initialURL.pathname || "/"')) {
  s = s.replace(reqUrlFrom, reqUrlTo);
  patched++;
}

const hrsIfFrom = 'if (event.rawPath.match(/(\\\\|\\/\\/)/)) {';
const hrsIfTo =
  'if (event.rawPath && event.rawPath !== "/" && /(?:\\\\|\\/\\/)/.test(String(event.rawPath).replace(/\\\\/g, "/"))) {';
if (s.includes(hrsIfFrom) && !s.includes('event.rawPath !== "/"')) {
  s = s.replace(hrsIfFrom, hrsIfTo);
  patched++;
}

const imageUrlFrom = `  const url = urls[0];
  if (url.length > 3072) {`;
const imageUrlTo = `  let url = urls[0];
  if (typeof url === "string" && url.startsWith("//")) {
    url = "/" + url.replace(/^\\/+/, "") || "/";
  }
  /* __cellpImageUrl */ if (url.length > 3072) {`;
if (s.includes(imageUrlFrom) && !s.includes('__cellpImageUrl')) {
  s = s.replace(imageUrlFrom, imageUrlTo);
  patched++;
}

const cookieParserRe =
  /function getCookieParser\(headers\) \{\s*return function\(\) \{\s*let \{ cookie \} = headers;\s*if \(!cookie\) return \{\};\s*let \{ parse: parseCookieFn \} = require_cookie\(\);\s*return parseCookieFn\(Array\.isArray\(cookie\) \? cookie\.join\("; "\) : cookie\);\s*\};\s*\}/;
const cookieParserRepl = `function getCookieParser(headers) {
        return function() {
          let cookie = headers == null ? void 0 : headers.cookie;
          if (cookie == null || cookie === "") return {};
          if (typeof cookie !== "string") {
            if (Array.isArray(cookie)) cookie = cookie.join("; ");
            else return {};
          }
          let { parse: parseCookieFn } = require_cookie();
          return parseCookieFn(cookie);
        };
      }`;
if (cookieParserRe.test(s) && !s.includes('typeof cookie !== "string"')) {
  s = s.replace(cookieParserRe, cookieParserRepl);
  patched++;
}

if (patched === 0 && s.includes('__cellpSlashPath') && s.includes('rel.startsWith("//")') && s.includes('u.origin + u.pathname') && s.includes('typeof cookie !== "string"') && s.includes('__cellpImageUrl')) {
  console.log('prepare-artifact: bundle already patched');
} else if (patched === 0) {
  console.error('prepare-artifact: no patches applied');
  process.exit(1);
} else {
  console.log('prepare-artifact: applied', patched, 'patch(es)');
}

fs.writeFileSync(p, s);
NODE

log "stage .open-next/assets → .cellp-assets"
rm -rf .cellp-assets
mkdir -p .cellp-assets
rsync -a .open-next/assets/ .cellp-assets/

node <<'NODE'
const fs = require('fs');
const p = 'wrangler.jsonc';
let raw = fs.readFileSync(p, 'utf8');
let j;
try {
  j = JSON.parse(raw);
} catch {
  raw = raw.replace(/^\s*\/\/.*$/gm, '').replace(/\/\*[\s\S]*?\*\//g, '').replace(/,\s*([}\]])/g, '$1');
  j = JSON.parse(raw);
}
j.name = 'support-opennext';
j.main = '.cellp-bundle/index.js';
j.no_bundle = true;
j.assets = j.assets || {};
j.assets.directory = '.cellp-assets';
j.assets.binding = j.assets.binding || 'ASSETS';
j.compatibility_flags = (j.compatibility_flags || []).filter(
  (f) => f !== 'global_fetch_strictly_public'
);
if (!j.compatibility_flags.includes('nodejs_compat')) {
  j.compatibility_flags.push('nodejs_compat');
}
delete j.observability;
delete j.upload_source_maps;
delete j.$schema;
raw = JSON.stringify(j, null, 2) + '\n';
raw = raw.replace(/:\/\//g, ':\\u002f\\u002f');
fs.writeFileSync(p, raw);
NODE

log "ok: OpenNext bundled worker + .cellp-assets (no celld re-bundle of .open-next tree)"
