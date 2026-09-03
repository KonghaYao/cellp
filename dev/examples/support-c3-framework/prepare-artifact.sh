#!/usr/bin/env bash
# C3 full-stack frameworks (solid | qwik | waku) on Workers — non-interactive scaffold for cellp.
set -euo pipefail
export SUPPORT_RSYNC_NO_NODE=1

APP_DIR="${1:?app dir}"
FRAMEWORK="${CELP_C3_FRAMEWORK:?set CELP_C3_FRAMEWORK (solid|qwik|waku)}"
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
SCAFFOLD="${APP_DIR}/cellp-${FRAMEWORK}-app"
OVERLAY="${ROOT}/dev/examples/support-c3-framework/wrangler.${FRAMEWORK}.cellp.jsonc"
[[ -f "$OVERLAY" ]] || { echo "missing overlay ${OVERLAY}" >&2; exit 1; }

log() { echo "prepare-artifact: $*"; }

# workers-sdk clone: APP_DIR may be hello-world ts stub or templates/<framework>
c3_templates_root() {
  if [[ -f "${APP_DIR}/c3.ts" && -d "${APP_DIR}/templates" ]]; then
    cd "${APP_DIR}/.." && pwd
  else
    cd "${APP_DIR}/../.." && pwd
  fi
}
C3_TEMPLATES="$(c3_templates_root)"

parse_jsonc() {
  node -e '
const fs = require("fs");
const p = process.argv[1];
let raw = fs.readFileSync(p, "utf8");
raw = raw.replace(/\/\*[\s\S]*?\*\//g, "");
raw = raw.replace(/^\s*\/\/.*$/gm, "");
raw = raw.replace(/,\s*([}\]])/g, "$1");
process.stdout.write(JSON.stringify(JSON.parse(raw)));
' "$1"
}

scaffold_solid() {
  local solid_tpl="${C3_TEMPLATES}/solid/templates"
  log "solid: create-solid (SolidStart v2, basic) + C3 template overlay"
  rm -rf "$SCAFFOLD"
  (
    cd "$APP_DIR"
    CI=1 DEBIAN_FRONTEND=noninteractive npx --yes create-solid@latest \
      -p "${SCAFFOLD##*/}" \
      -s --v2 --ts -t basic
  )
  [[ -d "$solid_tpl" ]] || {
    echo "prepare-artifact: missing ${solid_tpl} (clone cloudflare/workers-sdk for S27)" >&2
    exit 1
  }
  rsync -a "${solid_tpl}/" "${SCAFFOLD}/"
  cd "$SCAFFOLD"
  local wname
  wname="$(parse_jsonc "$OVERLAY" | node -e 'let j="";process.stdin.on("data",d=>j+=d);process.stdin.on("end",()=>console.log(JSON.parse(j).name))')"
  node -e "
const fs = require('fs');
const overlay = JSON.parse(fs.readFileSync(process.argv[1], 'utf8'));
let raw = fs.readFileSync('wrangler.jsonc', 'utf8');
raw = raw.replace(/<WORKER_NAME>/g, overlay.name || process.argv[2]);
raw = raw.replace(/<COMPATIBILITY_DATE>/g, overlay.compatibility_date || '2026-09-03');
fs.writeFileSync('wrangler.jsonc', raw);
" "$OVERLAY" "$wname"
  node <<'NODE'
const fs = require('fs');
const path = require('path');
const candidates = ['vite.config.ts', 'vite.config.js'];
const file = candidates.find((c) => fs.existsSync(c));
if (!file) process.exit(0);
let src = fs.readFileSync(file, 'utf8');
if (src.includes('cloudflare-module')) process.exit(0);
if (!/nitro\s*\(/.test(src)) {
  console.error('prepare-artifact: vite.config missing nitro() call');
  process.exit(1);
}
src = src.replace(
  /nitro\s*\(\s*\{([^}]*)\}\s*\)/,
  (m, inner) => {
    if (/preset\s*:/.test(inner)) {
      return m.replace(/preset\s*:\s*['"][^'"]+['"]/, "preset: 'cloudflare-module'");
    }
    const trimmed = inner.trim();
    const prefix = trimmed ? `${trimmed}, ` : '';
    return `nitro({ ${prefix}preset: 'cloudflare-module' })`;
  }
);
if (!src.includes('cloudflare-module')) {
  src = src.replace(/nitro\s*\(\s*\)/, "nitro({ preset: 'cloudflare-module' })");
}
fs.writeFileSync(file, src);
NODE
  npm install
  npm install -D wrangler
}

scaffold_qwik() {
  log "qwik: create-qwik playground + cloudflare-workers integration"
  rm -rf "$SCAFFOLD"
  (
    cd "$APP_DIR"
    CI=1 npm create qwik@latest playground "${SCAFFOLD##*/}" -f
  )
  cd "$SCAFFOLD"
  npm install
  npx qwik add cloudflare-workers --skipConfirmation=true
}

scaffold_waku() {
  local waku_tpl="${C3_TEMPLATES}/waku/templates"
  log "waku: create-waku + workers-sdk C3 template overlay"
  rm -rf "$SCAFFOLD"
  (
    cd "$APP_DIR"
    npx --yes create-waku@latest --project-name "${SCAFFOLD##*/}" --skip-install
  )
  [[ -d "$waku_tpl" ]] || {
    echo "prepare-artifact: missing ${waku_tpl} (clone cloudflare/workers-sdk for S29)" >&2
    exit 1
  }
  rsync -a "${waku_tpl}/" "${SCAFFOLD}/"
  cd "$SCAFFOLD"
  local wname
  wname="$(parse_jsonc "$OVERLAY" | node -e 'let j="";process.stdin.on("data",d=>j+=d);process.stdin.on("end",()=>console.log(JSON.parse(j).name))')"
  node -e "
const fs = require('fs');
const overlay = JSON.parse(fs.readFileSync(process.argv[1], 'utf8'));
let raw = fs.readFileSync('wrangler.jsonc', 'utf8');
raw = raw.replace(/<WORKER_NAME>/g, overlay.name || process.argv[2]);
raw = raw.replace(/<COMPATIBILITY_DATE>/g, overlay.compatibility_date || '2026-09-03');
fs.writeFileSync('wrangler.jsonc', raw);
" "$OVERLAY" "$wname"
  npm install
  npm install -D @cloudflare/vite-plugin miniflare @types/node wrangler
}

mkdir -p "$APP_DIR"

if [[ ! -f "${SCAFFOLD}/package.json" ]]; then
  case "$FRAMEWORK" in
    solid) scaffold_solid ;;
    qwik) scaffold_qwik ;;
    waku) scaffold_waku ;;
    *) echo "unknown CELP_C3_FRAMEWORK=${FRAMEWORK}" >&2; exit 1 ;;
  esac
fi

cd "$SCAFFOLD"

if [[ "$FRAMEWORK" == "qwik" ]] && [[ -f wrangler.jsonc ]] && grep -q '.cellp-bundle' wrangler.jsonc 2>/dev/null; then
  log "qwik: reset wrangler.jsonc for upstream build + dry-run"
  npx qwik add cloudflare-workers --skipConfirmation=true
fi

apply_qwik_wrangler_overlay() {
  [[ "$FRAMEWORK" == "qwik" ]] || return 0
  [[ -f wrangler.jsonc || -f wrangler.toml ]] || return 0
  export OVERLAY_PATH="$OVERLAY"
  node <<'NODE'
const fs = require('fs');
function parseJsonc(raw) {
  raw = raw.replace(/\/\*[\s\S]*?\*\//g, '');
  raw = raw.replace(/^\s*\/\/.*$/gm, '');
  raw = raw.replace(/,\s*([}\]])/g, '$1');
  return JSON.parse(raw);
}
const overlay = JSON.parse(fs.readFileSync(process.env.OVERLAY_PATH, 'utf8'));
const p = fs.existsSync('wrangler.jsonc') ? 'wrangler.jsonc' : 'wrangler.toml';
const j = parseJsonc(fs.readFileSync(p, 'utf8'));
j.name = overlay.name || j.name;
j.compatibility_date = overlay.compatibility_date || j.compatibility_date;
j.compatibility_flags = overlay.compatibility_flags || ['global_fetch_strictly_public'];
delete j.no_bundle;
fs.writeFileSync('wrangler.jsonc', JSON.stringify(j, null, 2) + '\n');
NODE
  log "qwik: wrangler compat (no nodejs_compat — avoids unenv vs celld Process on load)"
}

apply_qwik_wrangler_overlay

export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-false}"
log "build"
if grep -q '"build"' package.json; then
  npm run build
fi

log "wrangler dry-run bundle"
rm -rf .cellp-bundle
CFG="wrangler.jsonc"
[[ -f wrangler.toml ]] && CFG="wrangler.toml"
if [[ "$FRAMEWORK" == "waku" && -f dist/server/wrangler.json ]]; then
  npx wrangler deploy --config dist/server/wrangler.json --dry-run --outdir .cellp-bundle
elif [[ "$FRAMEWORK" == "solid" && -f .output/server/wrangler.json ]]; then
  npx wrangler deploy --config .output/server/wrangler.json --dry-run --outdir .cellp-bundle
else
  npx wrangler deploy --config "$CFG" --dry-run --outdir .cellp-bundle
fi
if [[ -f .cellp-bundle/_worker.js && ! -f .cellp-bundle/index.js ]]; then
  cp .cellp-bundle/_worker.js .cellp-bundle/index.js
fi
if [[ ! -f .cellp-bundle/index.js && -f .cellp-bundle/index.mjs ]]; then
  cp .cellp-bundle/index.mjs .cellp-bundle/index.js
fi
[[ -f .cellp-bundle/index.js ]] || { echo "missing .cellp-bundle/index.js" >&2; exit 1; }

rm -rf .cellp-assets
mkdir -p .cellp-assets
if [[ -d .output/public ]]; then
  log "stage assets from .output/public"
  rsync -a --exclude '.assetsignore' .output/public/ .cellp-assets/
elif [[ -d dist/public ]]; then
  log "stage assets from dist/public"
  rsync -a --exclude '.assetsignore' dist/public/ .cellp-assets/
elif [[ -d dist ]]; then
  log "stage assets from dist (exclude worker stubs)"
  rsync -a \
    --exclude '_worker.js' \
    --exclude '_worker.js.map' \
    --exclude '.assetsignore' \
    --exclude 'README.md' \
    dist/ .cellp-assets/
elif [[ -d dist/client ]]; then
  log "stage assets from dist/client"
  rsync -a dist/client/ .cellp-assets/
elif [[ -d public ]]; then
  log "stage assets from public"
  rsync -a public/ .cellp-assets/
fi

export OVERLAY_PATH="$OVERLAY"
export CELP_C3_FRAMEWORK="$FRAMEWORK"
node <<'NODE'
const fs = require('fs');
function parseJsonc(raw) {
  raw = raw.replace(/\/\*[\s\S]*?\*\//g, '');
  raw = raw.replace(/^\s*\/\/.*$/gm, '');
  raw = raw.replace(/,\s*([}\]])/g, '$1');
  return JSON.parse(raw);
}
const overlay = JSON.parse(fs.readFileSync(process.env.OVERLAY_PATH, 'utf8'));
const candidates = ['wrangler.jsonc', 'wrangler.toml'];
const p = candidates.find((c) => fs.existsSync(c));
if (!p) throw new Error('no wrangler config');
const j = parseJsonc(fs.readFileSync(p, 'utf8'));
j.name = overlay.name || j.name;
j.compatibility_date = overlay.compatibility_date || j.compatibility_date;
if (process.env.CELP_C3_FRAMEWORK === 'qwik') {
  j.compatibility_flags = overlay.compatibility_flags || ['global_fetch_strictly_public'];
} else {
  j.compatibility_flags = overlay.compatibility_flags || j.compatibility_flags || ['nodejs_compat'];
}
if (process.env.CELP_C3_FRAMEWORK === 'waku' && !j.compatibility_flags.includes('nodejs_als')) {
  j.compatibility_flags.push('nodejs_als');
}
j.main = '.cellp-bundle/index.js';
j.no_bundle = true;
j.assets = j.assets || {};
j.assets.binding = overlay.assets?.binding || j.assets.binding || 'ASSETS';
j.assets.directory = '.cellp-assets';
delete j.observability;
delete j.rules;
delete j.vars;
fs.writeFileSync('wrangler.jsonc', JSON.stringify(j, null, 2) + '\n');
NODE

log "publish slim artifact paths → ${APP_DIR}"
cp wrangler.jsonc "$APP_DIR/"
rm -rf "${APP_DIR}/.cellp-bundle" "${APP_DIR}/.cellp-assets"
rsync -a .cellp-bundle/ "${APP_DIR}/.cellp-bundle/"
rsync -a .cellp-assets/ "${APP_DIR}/.cellp-assets/"

log "ok: ${FRAMEWORK} bundle + .cellp-assets"
