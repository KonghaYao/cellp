#!/usr/bin/env bash
# Deploy a community Workers repo onto local cellp for support validation.
# Usage: deploy-support-app.sh <S-id|A-id>   e.g. S01 or A01
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

SUPPORT_MD="${ROOT}/docs/support-todos.md"
CORPUS="${ROOT}/dev/support-corpus"
EVIDENCE="${ROOT}/docs/evidence"

# shellcheck disable=SC1091
source dev/.env 2>/dev/null || { echo "FAIL: dev/.env"; exit 1; }
# shellcheck disable=SC1091
source e2e/scripts/lib.sh

# npm 源（国内默认 npmmirror；可在 dev/.env 覆盖 NPM_CONFIG_REGISTRY）
export NPM_CONFIG_REGISTRY="${NPM_CONFIG_REGISTRY:-https://registry.npmmirror.com}"
export npm_config_registry="${npm_config_registry:-$NPM_CONFIG_REGISTRY}"

SID="${1:?S-id or A-id e.g. S01 or A01}"
LOG="${EVIDENCE}/support-${SID}.log"
mkdir -p "$EVIDENCE" "$CORPUS"

exec > >(tee -a "$LOG") 2>&1
echo "=== deploy-support-app ${SID} $(date -Iseconds) ==="

lookup() {
  case "$SID" in
    S01) PROJECT=support-relay; REPO_URL=https://github.com/YuriCrystal/relay.git; WORKDIR_SUB=.; BUILD_STEPS= ;;
    S02) PROJECT=support-nimail; REPO_URL=https://github.com/mskatoni/ni-mail.git; WORKDIR_SUB=.; BUILD_STEPS= ;;
    S03) PROJECT=support-tempik; REPO_URL=https://github.com/hirotomasato/tempik.git; WORKDIR_SUB=.; BUILD_STEPS="npm ci" ;;
    S04) PROJECT=support-kukuroo; REPO_URL=https://github.com/saiday/kukuroo.git; WORKDIR_SUB=templates/standalone; BUILD_STEPS="npm install" ;;
    S05) PROJECT=support-flaremo; REPO_URL=https://github.com/realchendahuang/FlareMo.git; WORKDIR_SUB=.; BUILD_STEPS="corepack enable 2>/dev/null || true; pnpm install --ignore-scripts && pnpm --filter @flaremo/web build" ;;
    S06) PROJECT=support-memos; REPO_URL=https://github.com/souvenp/memos-worker.git; WORKDIR_SUB=.; BUILD_STEPS= ;;
    S07) PROJECT=support-monolith; REPO_URL=https://github.com/one-ea/Monolith.git; WORKDIR_SUB=server; BUILD_STEPS="(cd .. && npm ci && npm run build) && rm -rf client-dist node_modules && mkdir -p client-dist node_modules && rsync -a ../client/dist/ client-dist/ && rsync -a --exclude monolith-server --exclude monolith-client ../node_modules/ node_modules/" ;;
    S08) PROJECT=support-edgeever; REPO_URL=https://github.com/tianma-if/edgeever.git; WORKDIR_SUB=.; BUILD_STEPS="bun install && bun run build:web && bun run build:worker" ;;
    S09) PROJECT=support-sonicjs; REPO_URL=https://github.com/SonicJs-Org/sonicjs.git; WORKDIR_SUB=my-sonicjs-app; BUILD_STEPS="(cd .. && npm install && npm run build:core)" ;;
    S10) PROJECT=support-nodewarden; REPO_URL=https://github.com/shuaiplus/nodewarden.git; WORKDIR_SUB=.; BUILD_STEPS="npm ci && npm run build" ;;
    S11) PROJECT=support-sink; REPO_URL=https://github.com/miantiao-me/Sink.git; WORKDIR_SUB=.; BUILD_STEPS= ;;
    S12) PROJECT=support-inkstone; REPO_URL=https://github.com/shuaiplus/inkstone.git; WORKDIR_SUB=.; BUILD_STEPS="npm ci && npm run build" ;;
    S13) PROJECT=support-saasmail; REPO_URL=https://github.com/choyiny/saasmail.git; WORKDIR_SUB=.; BUILD_STEPS="npm ci && npm run build" ;;
    S14) PROJECT=support-cfbase; REPO_URL=https://github.com/cloudflarebase/cloudflarebase.git; WORKDIR_SUB=.; BUILD_STEPS="npm ci && npm run build" ;;
    S15) PROJECT=support-workflows; REPO_URL=https://github.com/cloudflare/templates.git; WORKDIR_SUB=workflows-starter-template; BUILD_STEPS="npm ci && npm run build" ;;
    S16) PROJECT=support-pastebin; REPO_URL=https://github.com/SharzyL/pastebin-worker.git; WORKDIR_SUB=.; BUILD_STEPS="npm install && npm run build:frontend" ;;
    S17) PROJECT=support-r2filebox; REPO_URL=https://github.com/workHMZ/r2filebox.git; WORKDIR_SUB=.; BUILD_STEPS="npm install" ;;
    S18) PROJECT=support-webhookflare; REPO_URL=https://github.com/fayazara/webhookflare.git; WORKDIR_SUB=.; BUILD_STEPS="npm install" ;;
    S19) PROJECT=support-requestbin; REPO_URL=https://github.com/ghostdevv/request-bin.git; WORKDIR_SUB=.; BUILD_STEPS="npm install" ;;
    S20) PROJECT=support-r2explorer; REPO_URL=https://github.com/G4brym/R2-Explorer.git; WORKDIR_SUB=.; BUILD_STEPS="npm install" ;;
    S21) PROJECT=support-fileworker; REPO_URL=https://github.com/woaiqjj/FileWorker.git; WORKDIR_SUB=.; BUILD_STEPS="npm install" ;;
    S22) PROJECT=support-astro; REPO_URL=https://github.com/cloudflare/templates.git; WORKDIR_SUB=astro-blog-starter-template; BUILD_STEPS="npm install" ;;
    S23) PROJECT=support-sveltekit; REPO_URL=https://github.com/cloudflare/workers-sdk.git; WORKDIR_SUB=packages/create-cloudflare/templates/svelte; BUILD_STEPS= ;;
    S24) PROJECT=support-remix; REPO_URL=https://github.com/cloudflare/templates.git; WORKDIR_SUB=remix-starter-template; BUILD_STEPS="npm install" ;;
    S25) PROJECT=support-nuxt; REPO_URL=https://github.com/cloudflare/workers-sdk.git; WORKDIR_SUB=packages/create-cloudflare/templates/nuxt; BUILD_STEPS= ;;
    S26) PROJECT=support-hono; REPO_URL=https://github.com/cloudflare/workers-sdk.git; WORKDIR_SUB=packages/create-cloudflare/templates/hello-world-with-assets/ts; BUILD_STEPS= ;;
    S27) PROJECT=support-solidstart; REPO_URL=https://github.com/cloudflare/workers-sdk.git; WORKDIR_SUB=packages/create-cloudflare/templates/solid; BUILD_STEPS= ;;
    S28) PROJECT=support-qwik; REPO_URL=https://github.com/cloudflare/workers-sdk.git; WORKDIR_SUB=packages/create-cloudflare/templates/qwik/workers; BUILD_STEPS= ;;
    S29) PROJECT=support-waku; REPO_URL=https://github.com/cloudflare/workers-sdk.git; WORKDIR_SUB=packages/create-cloudflare/templates/hello-world-with-assets/ts; BUILD_STEPS= ;;
    S30) PROJECT=support-opennext; REPO_URL=https://github.com/cloudflare/templates.git; WORKDIR_SUB=next-starter-template; BUILD_STEPS= ;;
    S31) PROJECT=support-imgbed; REPO_URL=https://github.com/MarSeventh/CloudFlare-ImgBed.git; WORKDIR_SUB=deploy/worker; BUILD_STEPS="(cd ../.. && npm install)" ;;
    S32) PROJECT=support-status-page; REPO_URL=https://github.com/eidam/cf-workers-status-page.git; WORKDIR_SUB=.; BUILD_STEPS= ;;
    S33) PROJECT=support-uptimeflare; REPO_URL=https://github.com/lyc8503/UptimeFlare.git; WORKDIR_SUB=.; BUILD_STEPS= ;;
    S34) PROJECT=support-microfeed; REPO_URL=https://github.com/microfeed/microfeed.git; WORKDIR_SUB=.; BUILD_STEPS= ;;
    S35) PROJECT=support-openstatus; REPO_URL=https://github.com/openstatusHQ/openstatus.git; WORKDIR_SUB=.; BUILD_STEPS= ;;
    S36) PROJECT=support-triplit; REPO_URL=https://github.com/aspen-cloud/triplit.git; WORKDIR_SUB=packages/cf-worker-server; BUILD_STEPS= ;;
    S37) PROJECT=support-serverless-dns; REPO_URL=https://github.com/serverless-dns/serverless-dns.git; WORKDIR_SUB=.; BUILD_STEPS= ;;
    S38) PROJECT=support-counterscale; REPO_URL=https://github.com/jeffysl/counterscale.git; WORKDIR_SUB=packages/server; BUILD_STEPS= ;;
    S39) PROJECT=support-cloudpaste; REPO_URL=https://github.com/ling-drag0n/CloudPaste.git; WORKDIR_SUB=backend; BUILD_STEPS= ;;
    A01) PROJECT=support-agents-starter; REPO_URL=https://github.com/cloudflare/agents-starter.git; WORKDIR_SUB=.; BUILD_STEPS="npm install && npx vite build" ;;
    A02) PROJECT=support-pi-worker; REPO_URL=https://github.com/qaml-ai/pi-worker.git; WORKDIR_SUB=examples/hello-agent; BUILD_STEPS= ;;
    A03) PROJECT=support-opencode-do; REPO_URL=https://github.com/southpolesteve/opencode-do.git; WORKDIR_SUB=.; BUILD_STEPS="npm install" ;;
    A04) PROJECT=support-fx-on-workers; REPO_URL=https://github.com/codingstark-dev/fx-on-workers.git; WORKDIR_SUB=.; BUILD_STEPS="npm install" ;;
    *) echo "Unknown ${SID}"; exit 1 ;;
  esac
}
lookup

# GitHub clone: direct often fails in CN; default ghfast mirror (override in dev/.env).
#   GITHUB_CLONE_MIRROR=https://ghfast.top/https://github.com/   (default)
#   GITHUB_CLONE_DIRECT=1   skip mirror, use REPO_URL as-is
#   http_proxy / https_proxy   also applied to git when set
clone_git_url() {
  local url="$1"
  if [[ "${GITHUB_CLONE_DIRECT:-}" == "1" ]]; then
    echo "$url"
    return
  fi
  local mirror="${GITHUB_CLONE_MIRROR:-https://ghfast.top/https://github.com/}"
  if [[ "$url" =~ ^https://github.com/(.+)$ ]]; then
    echo "${mirror}${BASH_REMATCH[1]}"
  else
    echo "$url"
  fi
}

VERSION="${SUPPORT_VERSION:-v1}"
pick_support_version() {
  local n v st
  for n in 1 2 3 4 5 6 7 8 9 10; do
    v="v${n}"
    st="$(api_get "/v1/projects/${PROJECT}/versions/${v}" 2>/dev/null | jq -r .status 2>/dev/null || echo absent)"
    case "$st" in
      absent|null|gone) VERSION="$v"; return ;;
      ready) VERSION="$v"; return ;;
      deploying|pending|starting) VERSION="$v"; return ;;
      failed|destroyed|draining) continue ;;
      *) VERSION="$v"; return ;;
    esac
  done
  VERSION="v10"
}
if [[ -z "${SUPPORT_SKIP_VERSION_PICK:-}" ]]; then
  pick_support_version
fi
log "deploy version ${VERSION}"

CLONE_DIR="${CORPUS}/${PROJECT}"

require_platform
require_celld

log "clone/update ${REPO_URL}"
CLONE_URL="$(clone_git_url "$REPO_URL")"
[[ "$CLONE_URL" != "$REPO_URL" ]] && log "mirror clone: ${CLONE_URL}"
if [[ -d "${CLONE_DIR}/.git" ]]; then
  if [[ "${SUPPORT_SKIP_GIT_FETCH:-}" == "1" ]]; then
    log "skip git fetch (SUPPORT_SKIP_GIT_FETCH=1, use existing corpus)"
  else
    git -C "$CLONE_DIR" fetch --depth 1 origin 2>/dev/null || true
    git -C "$CLONE_DIR" pull --ff-only 2>/dev/null || true
  fi
else
  rm -rf "$CLONE_DIR"
  GIT_TERMINAL_PROMPT=0 git clone --depth 1 "$CLONE_URL" "$CLONE_DIR"
fi

APP_DIR="${CLONE_DIR}"
[[ "$WORKDIR_SUB" != "." ]] && APP_DIR="${CLONE_DIR}/${WORKDIR_SUB}"
cd "$APP_DIR"

if [[ -n "$BUILD_STEPS" && "${SUPPORT_SKIP_BUILD:-}" != "1" ]]; then
  log "build: ${BUILD_STEPS}"
  export NPM_CONFIG_IGNORE_SCRIPTS="${NPM_CONFIG_IGNORE_SCRIPTS:-true}"
  export npm_config_ignore_scripts="${npm_config_ignore_scripts:-true}"
  bash -lc "$BUILD_STEPS" || { echo "FAIL: build (${SID})"; exit 1; }
fi

CELLP_WRANGLER_OVERLAY="${ROOT}/dev/examples/${PROJECT}/wrangler.cellp.jsonc"
if [[ ! -f wrangler.jsonc && ! -f wrangler.json && ! -f wrangler.toml ]]; then
  if [[ -f worker.js ]]; then
    log "synthesize wrangler.jsonc for worker.js"
    cat > wrangler.jsonc <<EOF
{
  "name": "${PROJECT}",
  "main": "worker.js",
  "compatibility_date": "2026-01-01"
}
EOF
  elif [[ -f "$CELLP_WRANGLER_OVERLAY" ]]; then
    log "no upstream wrangler; will apply cellp overlay"
  else
    echo "FAIL: no wrangler config in ${APP_DIR}"
    exit 1
  fi
fi

strip_wrangler_toml() {
  local dir="$1"
  local f="${dir}/wrangler.toml"
  [[ -f "$f" ]] || return 0
  log "strip wrangler.toml (routes, email, observability)"
  perl -0777 -i -pe '
    s/\n\[\[routes\]\][^\[]*//g;
    s/\n\[observability\][^\[]*//g;
    s/\n\[email\][^\[]*//g;
    s/workers_dev\s*=\s*false/workers_dev = true/g;
    s/database_id\s*=\s*""/database_id = "00000000-0000-0000-0000-00000000000b"/g;
  ' "$f"
}

strip_wrangler_for_celld() {
  local dir="$1"
  strip_wrangler_toml "$dir"
  if ! command -v node >/dev/null; then
    return 0
  fi
  log "strip wrangler.json(c) keys unsupported by celld"
  STRIP_DIR="$dir" node <<'NODE'
const fs = require('fs');
const path = require('path');
const dir = process.env.STRIP_DIR;
const drop = new Set(['observability', 'upload_source_maps', 'dispatch_namespaces', 'placement', 'limits', 'routes', 'preview_urls', 'email', 'workers_dev', '$schema']);
for (const f of ['wrangler.jsonc', 'wrangler.json']) {
  const p = path.join(dir, f);
  if (!fs.existsSync(p)) continue;
  let raw = fs.readFileSync(p, 'utf8');
  raw = raw.replace(/\/\/[^\n]*/g, '').replace(/\/\*[\s\S]*?\*\//g, '');
  raw = raw.replace(/,(\s*[}\]])/g, '$1');
  raw = raw.replace(/"preview_urls"\s*:\s*[^,\n]+,?\s*/g, '');
  raw = raw.replace(/"workers_dev"\s*:\s*[^,\n]+,?\s*/g, '');
  raw = raw.replace(/"routes"\s*:\s*\[[^\]]*\],?\s*/g, '');
  let j;
  try { j = JSON.parse(raw); } catch (e) { console.error('wrangler parse failed', e.message); continue; }
  for (const k of drop) delete j[k];
  j.name = j.name || 'worker';
  fs.writeFileSync(p.replace(/\.json$/, '.jsonc'), JSON.stringify(j, null, 2) + '\n');
  if (p.endsWith('.json')) fs.unlinkSync(p);
  break;
}
NODE
}

OVERLAY="${ROOT}/dev/examples/${PROJECT}/wrangler.cellp.jsonc"
if [[ -f "$OVERLAY" ]]; then
  log "apply cellp wrangler overlay ${OVERLAY}"
  cp "$OVERLAY" "$APP_DIR/wrangler.jsonc"
  rm -f "$APP_DIR/wrangler.toml"
  if grep -q '__CELLP_DEPLOY_URL__' "$APP_DIR/wrangler.jsonc" 2>/dev/null || grep -q '__CELLP_DEPLOY_URL__' "$APP_DIR/wrangler.jsonc" 2>/dev/null; then
    gw_port="${GATEWAY_URL##*:}"
    gw_port="${gw_port%%/*}"
    gw_port="${gw_port:-8787}"
    scheme="${CELLP_PUBLIC_SCHEME_PREVIEW:-http}"
    if [[ "$PROJECT" == "support-opennext" ]]; then
      deploy_url="${scheme}://$(prod_host "$PROJECT"):${gw_port}/"
    else
      deploy_url="${scheme}://$(preview_host "$PROJECT" "$VERSION"):${gw_port}"
    fi
    log "inject DEPLOY_URL=${deploy_url}"
    node -e "
const fs = require('fs');
const p = process.argv[1];
const url = process.argv[2];
let j = JSON.parse(fs.readFileSync(p, 'utf8'));
const keys = ['DEPLOY_URL', 'FLAREMO_PUBLIC_URL', 'PUBLIC_BASE_URL'];
for (const k of keys) {
  if (j.vars && k in j.vars) j.vars[k] = url;
}
let raw = JSON.stringify(j, null, 2) + '\n';
raw = raw.replace(/:\\/\\//g, ':\\\\u002f\\\\u002f');
fs.writeFileSync(p, raw);
" "$APP_DIR/wrangler.jsonc" "$deploy_url"
  fi
else
  strip_wrangler_for_celld "$APP_DIR"
  if [[ -f "$APP_DIR/wrangler.toml" && ! -f "$APP_DIR/wrangler.jsonc" ]]; then
    echo "FAIL: ${SID} has wrangler.toml only; add dev/examples/${PROJECT}/wrangler.cellp.jsonc or wrangler.json(c)"
    exit 1
  fi
fi

CELLP_PREPARE="${ROOT}/dev/examples/${PROJECT}/prepare-artifact.sh"
PATCH_BUILD="${ROOT}/dev/examples/${PROJECT}/patch-build-worker-node-externals.sh"
if [[ -f "$PATCH_BUILD" ]]; then
  log "patch worker build externals: ${PATCH_BUILD}"
  bash "$PATCH_BUILD" "$APP_DIR"
fi
PATCH_CRYPTO="${ROOT}/dev/examples/${PROJECT}/patch-auth-crypto-pbkdf2.sh"
if [[ -f "$PATCH_CRYPTO" ]]; then
  log "patch auth crypto: ${PATCH_CRYPTO}"
  bash "$PATCH_CRYPTO" "$APP_DIR"
  if [[ -f "${APP_DIR}/package.json" ]] && grep -q '"build:worker"' "${APP_DIR}/package.json" 2>/dev/null; then
    log "rebuild worker after auth patch"
    (cd "$APP_DIR" && bun run build:worker)
  fi
fi
if [[ -f "$CELLP_PREPARE" ]]; then
  log "prepare artifact: ${CELLP_PREPARE}"
  if [[ -f "${ROOT}/dev/examples/${PROJECT}/patch-local-dev-hosts.sh" && -f "${APP_DIR}/apps/worker/src/auth.ts" ]]; then
    bash "${ROOT}/dev/examples/${PROJECT}/patch-local-dev-hosts.sh" "${APP_DIR}/apps/worker/src/auth.ts"
  fi
  bash "$CELLP_PREPARE" "$APP_DIR"
  CELLP_PREPARE_ENV="${ROOT}/dev/examples/${PROJECT}/cellp-prepare.env"
  if [[ -f "$CELLP_PREPARE_ENV" ]]; then
    # shellcheck disable=SC1090
    source "$CELLP_PREPARE_ENV"
  fi
  if [[ "${CELLP_RSYNC_NODE_MODULES:-}" != "1" ]]; then
    SUPPORT_RSYNC_NO_NODE=1
  fi
fi

ensure_project "$PROJECT"
# 仅回收 ready/failed 的同 id 重部署；destroyed 由 pick_support_version 换 id
if [[ "${SUPPORT_DESTROY_FIRST:-}" == "1" ]]; then
  curl -sf -X DELETE "${PLATFORM_URL}/v1/projects/${PROJECT}/versions/${VERSION}" \
    -H "$(api_auth "$ADMIN_TOKEN")" >/dev/null 2>&1 || true
  sleep 1
fi
DEST="${ARTIFACTS_DIR}/${PROJECT}/${VERSION}"
rm -rf "$DEST"
mkdir -p "$DEST"

log "stage artifact → ${DEST}"
if [[ "${SUPPORT_RSYNC_NO_NODE:-}" == "1" && -f ./.cellp-bundle/index.js && -f ./wrangler.jsonc ]]; then
  log "stage slim artifact (wrangler + bundle + static + migrations; no node_modules)"
  cp ./wrangler.jsonc "$DEST/"
  rsync -a ./.cellp-bundle/ "$DEST/.cellp-bundle/"
  if [[ -d ./.cellp-assets ]]; then
    rsync -a ./.cellp-assets/ "$DEST/.cellp-assets/"
  fi
  if [[ -d ./.output/server ]]; then
    mkdir -p "$DEST/.output/server"
    rsync -a ./.output/server/ "$DEST/.output/server/"
  fi
  if [[ -d ./apps/web/dist ]]; then
    mkdir -p "$DEST/apps/web"
    rsync -a ./apps/web/dist/ "$DEST/apps/web/dist/"
  fi
  if [[ -d ./migrations ]]; then
    rsync -a ./migrations/ "$DEST/migrations/"
  fi
  STAGE_HOOK="${ROOT}/dev/examples/${PROJECT}/stage-artifact-extra.sh"
  if [[ -f "$STAGE_HOOK" ]]; then
    log "stage extra: ${STAGE_HOOK}"
    bash "$STAGE_HOOK" "$APP_DIR" "$DEST"
  fi
elif [[ "${SUPPORT_RSYNC_NO_NODE:-}" == "1" && -f ./wrangler.jsonc ]] && {
  [[ -f ./dist/support_workflows/index.js ]] || [[ -f ./.svelte-kit/cloudflare/_worker.js ]];
}; then
  log "stage slim artifact (prebuilt worker path + assets; no node_modules)"
  cp ./wrangler.jsonc "$DEST/"
  if [[ -d ./.cellp-assets ]]; then
    rsync -a ./.cellp-assets/ "$DEST/.cellp-assets/"
  fi
  if [[ -d ./.cellp-bundle ]]; then
    rsync -a ./.cellp-bundle/ "$DEST/.cellp-bundle/"
  fi
  if [[ -d ./.svelte-kit/cloudflare ]]; then
    mkdir -p "$DEST/.svelte-kit"
    rsync -a ./.svelte-kit/cloudflare/ "$DEST/.svelte-kit/cloudflare/"
  fi
  if [[ -d ./dist/support_workflows ]]; then
    mkdir -p "$DEST/dist/support_workflows"
    rsync -a ./dist/support_workflows/ "$DEST/dist/support_workflows/"
  fi
  if [[ -d ./migrations ]]; then
    rsync -a ./migrations/ "$DEST/migrations/"
  fi
  STAGE_HOOK="${ROOT}/dev/examples/${PROJECT}/stage-artifact-extra.sh"
  if [[ -f "$STAGE_HOOK" ]]; then
    log "stage extra: ${STAGE_HOOK}"
    bash "$STAGE_HOOK" "$APP_DIR" "$DEST"
  fi
elif [[ "${SUPPORT_RSYNC_NO_NODE:-}" == "1" && -f ./wrangler.jsonc && -f ./dist/server/entry.mjs ]]; then
  log "stage slim artifact (astro dist/server + assets; no node_modules)"
  cp ./wrangler.jsonc "$DEST/"
  if [[ -d ./.cellp-assets ]]; then
    rsync -a ./.cellp-assets/ "$DEST/.cellp-assets/"
  fi
  mkdir -p "$DEST/dist"
  rsync -a ./dist/server/ "$DEST/dist/server/"
  if [[ -d ./migrations ]]; then
    rsync -a ./migrations/ "$DEST/migrations/"
  fi
  STAGE_HOOK="${ROOT}/dev/examples/${PROJECT}/stage-artifact-extra.sh"
  if [[ -f "$STAGE_HOOK" ]]; then
    log "stage extra: ${STAGE_HOOK}"
    bash "$STAGE_HOOK" "$APP_DIR" "$DEST"
  fi
elif [[ "${SUPPORT_RSYNC_NO_NODE:-}" == "1" && -f ./wrangler.jsonc && -f ./dist/_worker.js/index.js ]]; then
  log "stage slim artifact (astro dist/_worker.js + assets; no node_modules)"
  cp ./wrangler.jsonc "$DEST/"
  if [[ -d ./.cellp-assets ]]; then
    rsync -a ./.cellp-assets/ "$DEST/.cellp-assets/"
  fi
  mkdir -p "$DEST/dist"
  rsync -a ./dist/_worker.js/ "$DEST/dist/_worker.js/"
  if [[ -d ./migrations ]]; then
    rsync -a ./migrations/ "$DEST/migrations/"
  fi
  STAGE_HOOK="${ROOT}/dev/examples/${PROJECT}/stage-artifact-extra.sh"
  if [[ -f "$STAGE_HOOK" ]]; then
    log "stage extra: ${STAGE_HOOK}"
    bash "$STAGE_HOOK" "$APP_DIR" "$DEST"
  fi
elif [[ "${SUPPORT_RSYNC_NO_NODE:-}" == "1" && -f ./.wrangler/edgeever-worker/index.js && -f ./wrangler.jsonc ]]; then
  log "stage slim artifact (edgeever worker + web dist + migrations)"
  cp ./wrangler.jsonc "$DEST/"
  mkdir -p "$DEST/.wrangler"
  rsync -a ./.wrangler/edgeever-worker/ "$DEST/.wrangler/edgeever-worker/"
  mkdir -p "$DEST/apps/web"
  rsync -a ./apps/web/dist/ "$DEST/apps/web/dist/"
  rsync -a ./migrations/ "$DEST/migrations/"
else
  RSYNC_EX=(--exclude .git)
  [[ -z "$BUILD_STEPS" ]] && RSYNC_EX+=(--exclude node_modules)
  [[ "${SUPPORT_RSYNC_NO_NODE:-}" == "1" ]] && RSYNC_EX+=(--exclude node_modules --exclude packages --exclude tests --exclude docs --exclude scripts --exclude .github --exclude apps/worker --exclude apps/site --exclude apps/telegram-bot)
  if command -v rsync >/dev/null; then
    rsync -a "${RSYNC_EX[@]}" ./ "$DEST/"
  else
    tar -cf - --exclude .git ${BUILD_STEPS:+} . | tar -xf - -C "$DEST"
  fi
fi

SEED_SCRIPT="${ROOT}/dev/examples/${PROJECT}/seed.sh"
if [[ -f "$SEED_SCRIPT" && -d "${APP_DIR}/migrations" ]]; then
  log "D1 seed.db via ${SEED_SCRIPT}"
  bash "$SEED_SCRIPT" "${DEST}/seed.db" "${APP_DIR}"
fi

sync_artifact_to_rustfs "$PROJECT" "$VERSION"

create_version "$PROJECT" "$VERSION" "" "{\"artifact_uri\":\"s3://cellp-artifacts/${PROJECT}/${VERSION}/\"}" \
  | jq -r .preview_url >/tmp/support-preview-url.txt 2>/dev/null || true

log "poll ready (timeout ${SUPPORT_POLL_SECS:-120}s)"
if ! poll_version "$PROJECT" "$VERSION" ready "${SUPPORT_POLL_SECS:-120}" >/dev/null; then
  api_get "/v1/projects/${PROJECT}/versions/${VERSION}" | jq -r '.status,.error' 2>/dev/null || true
  echo "FAIL: version ${VERSION} failed (wanted ready)"
  exit 1
fi

if [[ "$SID" == "A04" && -n "${AI_GATEWAY_API_KEY:-}" ]]; then
  FX_MODEL="${FX_MODEL:-minimax/minimax-m3-free}"
  log "A04: worker env AI_GATEWAY_API_KEY + FX_MODEL=${FX_MODEL}"
  if ! api_put "/v1/projects/${PROJECT}/versions/${VERSION}/env" \
    "{\"vars\":{\"AI_GATEWAY_API_KEY\":\"${AI_GATEWAY_API_KEY}\",\"FX_MODEL\":\"${FX_MODEL}\"}}" \
    "$ADMIN_TOKEN" >/dev/null; then
    echo "WARN: PUT version env failed (set AI_GATEWAY_API_KEY in dev/.env)"
  fi
  sleep 2
fi

if [[ -f "${DEST}/wrangler.jsonc" ]] && { [[ -d "${DEST}/migrations" ]] || [[ -d "${DEST}/drizzle" ]]; }; then
  MIG_SCRIPT="${ROOT}/dev/scripts/apply-version-d1-migrations.sh"
  if [[ -f "$MIG_SCRIPT" ]]; then
    log "apply D1 migrations"
    bash "$MIG_SCRIPT" "$PROJECT" "$VERSION" || echo "WARN: d1 migrations apply failed (may already be applied)"
  fi
fi

PREVIEW="$(version_preview_url "$PROJECT" "$VERSION" 2>/dev/null || true)"
if [[ -z "$PREVIEW" ]]; then
  PREVIEW="${GATEWAY_URL}/${PROJECT}/${VERSION}/"
  [[ -f /tmp/support-preview-url.txt ]] && PREVIEW=$(cat /tmp/support-preview-url.txt)
fi
PREVIEW="${PREVIEW%/}/"

if [[ "${INGRESS_HOST_ONLY:-1}" != "0" ]]; then
  CODE=$(http_code_version "$PROJECT" "$VERSION" "/")
  log "smoke GET Host=$(preview_host "$PROJECT" "$VERSION") ${GATEWAY_URL}/ → HTTP ${CODE}"
else
  CODE=$(http_code "$PREVIEW")
  log "smoke GET ${PREVIEW} → HTTP ${CODE}"
fi

if ! promote_out="$(curl -sf -X POST "${PLATFORM_URL}/v1/projects/${PROJECT}/versions/${VERSION}/promote" \
  -H "$(api_auth "$ADMIN_TOKEN")" -H "Content-Type: application/json" -d '{}' 2>&1)"; then
  log "WARN: promote failed for ${PROJECT}/${VERSION} (ingress prod Host may stay stale): ${promote_out}"
else
  log "promote ok: $(echo "$promote_out" | jq -r '.prod_url // .prod_version_id // ok' 2>/dev/null || echo "$promote_out")"
fi
if [[ "${INGRESS_HOST_ONLY:-1}" != "0" ]]; then
  PROD_CODE=$(http_code_prod "$PROJECT" "/")
  gw_port="${GATEWAY_URL##*:}"
  gw_port="${gw_port%%/*}"
  PROD="${CELLP_PUBLIC_SCHEME_PREVIEW:-http}://$(prod_host "$PROJECT"):${gw_port:-8787}/"
else
  PROD="${GATEWAY_URL}/${PROJECT}/"
  PROD_CODE=$(http_code "$PROD")
fi

echo ""
echo "OK ${SID} project=${PROJECT} version=${VERSION}"
echo "PREVIEW_URL=${PREVIEW}"
echo "PROD_URL=${PROD}"
echo "PREVIEW_HTTP=${CODE}"
echo "PROD_HTTP=${PROD_CODE}"
echo "DASHBOARD=http://127.0.0.1:${DASHBOARD_PORT:-5190}/projects/${PROJECT}"
echo "LOG=${LOG}"

exit 0
