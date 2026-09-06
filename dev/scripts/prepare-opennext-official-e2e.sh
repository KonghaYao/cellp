# shellcheck shell=bash
# Shared helpers for @opennextjs/cloudflare official Playwright e2e (cellp preview harness).
# Source from repo root after e2e/scripts/lib.sh.

: "${ONCF_REPO_URL:=https://github.com/opennextjs/opennextjs-cloudflare.git}"
: "${ONCF_TAG:=@opennextjs/cloudflare@1.14.0}"
: "${ONCF_COMMIT:=a644ee1597de577632a29af9a005684554607b2f}"
: "${ONCF_CLONE_DIR:=${E2E_ROOT:-}/dev/support-corpus/opennextjs-cloudflare}"

ONCF_FIXTURES=(app-router pages-router app-pages-router)

# Baseline declarations in worker mode (*next.spec.ts ignored).
# Playwright --list includes the 5 explicit test.skip declarations; runnable = listed - skip.
oncf_expect_runnable() {
  case "$1" in
    app-router) echo 58 ;;
    pages-router) echo 36 ;;
    app-pages-router) echo 20 ;;
    *) echo 0 ;;
  esac
}
oncf_expect_listed() {
  case "$1" in
    app-router) echo 61 ;;
    pages-router) echo 37 ;;
    app-pages-router) echo 21 ;;
    *) echo 0 ;;
  esac
}
ONCF_EXPECT_SKIP_APP_ROUTER=3
ONCF_EXPECT_SKIP_PAGES_ROUTER=1
ONCF_EXPECT_SKIP_APP_PAGES_ROUTER=1
ONCF_EXPECT_RUNNABLE_TOTAL=114
ONCF_EXPECT_LISTED_TOTAL=119
ONCF_EXPECT_SKIP_TOTAL=5

oncf_log() { echo "==> opennext-official: $*"; }

oncf_fixture_dir() {
  local fixture="$1"
  echo "${ONCF_CLONE_DIR}/examples/e2e/${fixture}"
}

oncf_project_id() {
  case "$1" in
    app-router) echo "${ONCF_PROJECT_APP_ROUTER:-opennext-e2e-app-router}" ;;
    pages-router) echo "${ONCF_PROJECT_PAGES_ROUTER:-opennext-e2e-pages-router}" ;;
    app-pages-router) echo "${ONCF_PROJECT_APP_PAGES_ROUTER:-opennext-e2e-app-pages-router}" ;;
    *) return 1 ;;
  esac
}

oncf_require_existing_prod() {
  local project="$1"
  local prod
  prod="$(api_get "/v1/projects/${project}" "$ADMIN_TOKEN" 2>/dev/null \
    | jq -r '.prod_version_id // empty' 2>/dev/null || true)"
  if [[ -z "$prod" ]]; then
    echo "FAIL: official preview acceptance requires an existing production for ${project}; refusing first-ready bootstrap" >&2
    return 1
  fi
  oncf_log "preview safety: preserve existing production ${project}/${prod}"
}

oncf_valid_fixture() {
  local f="$1"
  local x
  for x in "${ONCF_FIXTURES[@]}"; do
    [[ "$x" == "$f" ]] && return 0
  done
  return 1
}

oncf_ensure_clone() {
  need git
  if [[ ! -d "${ONCF_CLONE_DIR}/.git" ]]; then
    oncf_log "clone ${ONCF_REPO_URL} → ${ONCF_CLONE_DIR}"
    mkdir -p "$(dirname "${ONCF_CLONE_DIR}")"
    git clone "${ONCF_REPO_URL}" "${ONCF_CLONE_DIR}"
  fi
  local head
  if ! git -C "${ONCF_CLONE_DIR}" diff --quiet -- \
    || ! git -C "${ONCF_CLONE_DIR}" diff --cached --quiet --; then
    echo "FAIL: pinned OpenNext corpus has tracked modifications; refusing contaminated run" >&2
    git -C "${ONCF_CLONE_DIR}" status --short --untracked-files=no >&2
    exit 1
  fi
  head="$(git -C "${ONCF_CLONE_DIR}" rev-parse HEAD 2>/dev/null || echo "")"
  if [[ "$head" == "$ONCF_COMMIT" ]]; then
    oncf_log "clone already at ${ONCF_COMMIT} (${ONCF_TAG})"
  else
    oncf_log "verify checkout ${ONCF_COMMIT} (${ONCF_TAG})"
    git -C "${ONCF_CLONE_DIR}" fetch --tags --force origin 2>/dev/null || git -C "${ONCF_CLONE_DIR}" fetch --tags --force 2>/dev/null || true
    git -C "${ONCF_CLONE_DIR}" checkout --detach "${ONCF_COMMIT}"
    head="$(git -C "${ONCF_CLONE_DIR}" rev-parse HEAD)"
    if [[ "$head" != "$ONCF_COMMIT" ]]; then
      echo "FAIL: opennext clone HEAD ${head} != ${ONCF_COMMIT}" >&2
      exit 1
    fi
  fi
  if ! git -C "${ONCF_CLONE_DIR}" diff --quiet -- \
    || ! git -C "${ONCF_CLONE_DIR}" diff --cached --quiet --; then
    echo "FAIL: pinned OpenNext corpus has tracked modifications; refusing contaminated run" >&2
    git -C "${ONCF_CLONE_DIR}" status --short --untracked-files=no >&2
    exit 1
  fi
  local branch
  branch="$(git -C "${ONCF_CLONE_DIR}" rev-parse --abbrev-ref HEAD 2>/dev/null || echo HEAD)"
  if [[ "$branch" == "main" || "$branch" == "master" ]]; then
    echo "FAIL: refuse to run on main/master branch" >&2
    exit 1
  fi
}

oncf_install_workspace() {
  local fast="${1:-0}"
  cellp_ensure_pnpm || return 1
  cd "${ONCF_CLONE_DIR}"
  if [[ "$fast" == "1" && -x node_modules/.bin/playwright \
    && -x examples/e2e/app-router/node_modules/.bin/opennextjs-cloudflare \
    && -x examples/e2e/pages-router/node_modules/.bin/opennextjs-cloudflare \
    && -x examples/e2e/app-pages-router/node_modules/.bin/opennextjs-cloudflare ]]; then
    oncf_log "fast: reuse filtered workspace dependencies"
    return 0
  fi
  oncf_log "pnpm install (adapter + three official fixtures)"
  CI=true pnpm install --frozen-lockfile \
    --filter cloudflare... \
    --filter app-router... \
    --filter pages-router... \
    --filter app-pages-router...
  oncf_log "build @opennextjs/cloudflare package"
  pnpm --filter cloudflare build
  # pnpm may try to create workspace CLI links before the adapter postinstall build.
  # A second filtered install is cheap and repairs those links deterministically.
  CI=true pnpm install --frozen-lockfile \
    --filter cloudflare... \
    --filter app-router... \
    --filter pages-router... \
    --filter app-pages-router...
}

oncf_ensure_browser() {
  oncf_log "ensure Chromium for pinned Playwright"
  (
    cd "${ONCF_CLONE_DIR}"
    pnpm exec playwright install chromium
  )
}

oncf_build_fixture() {
  local fixture="$1"
  local skip_build="${2:-0}"
  local app_dir
  app_dir="$(oncf_fixture_dir "$fixture")"
  [[ -d "$app_dir" ]] || { echo "FAIL: missing fixture dir ${app_dir}" >&2; return 1; }
  if [[ "$skip_build" == "1" ]]; then
    [[ -f "${app_dir}/.open-next/worker.js" ]] || { echo "FAIL: --skip-build but missing .open-next/worker.js in ${app_dir}" >&2; return 1; }
    oncf_log "${fixture}: skip build (reuse .open-next)"
    return 0
  fi
  oncf_log "${fixture}: official build:worker (clean Next + OpenNext build)"
  rm -rf "${app_dir}/.open-next" "${app_dir}/.next" || return $?
  (
    cd "${ONCF_CLONE_DIR}" || exit $?
    pnpm --filter "${fixture}" run build:worker
  ) || return $?
  [[ -f "${app_dir}/.open-next/worker.js" ]] || { echo "FAIL: missing .open-next/worker.js after build" >&2; return 1; }
}

oncf_write_playwright_config() {
  local app_dir="$1"
  local base_url="$2"
  local cfg="${app_dir}/e2e/playwright.cellp.config.ts"
  # Generated under ignored clone; official e2e/*.test.ts unchanged.
  cat >"$cfg" <<'EOF'
import { defineConfig, devices } from "@playwright/test";

/** cellp preview harness — no webServer; baseURL uses lvh.me preview host. */
export default defineConfig({
  testDir: "./",
  testIgnore: "*next.spec.ts",
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: [
    ["list"],
    ["html", { open: "never", outputFolder: `${process.env.CELLP_ONCF_OUTPUT_DIR || ".cellp-test-results"}-html` }],
  ],
  outputDir: process.env.CELLP_ONCF_OUTPUT_DIR || ".cellp-test-results",
  use: {
    baseURL: "http://placeholder.lvh.me:8787",
    trace: "retain-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
EOF
  # Fix baseURL to use env when set at runtime
  node -e "
const fs = require('fs');
const p = process.argv[1];
const url = process.argv[2];
let s = fs.readFileSync(p, 'utf8');
s = s.replace(
  /baseURL: .*/,
  'baseURL: process.env.CELLP_ONCF_BASE_URL || ' + JSON.stringify(url) + ','
);
fs.writeFileSync(p, s);
" "$cfg" "$base_url"
}

oncf_declared_skip_count() {
  local fixture="$1"
  local app_dir
  app_dir="$(oncf_fixture_dir "$fixture")"
  local n
  n="$(grep -R --include='*.test.ts' -E 'test\.skip\(' "${app_dir}/e2e" 2>/dev/null | wc -l | tr -d ' ')"
  echo "${n:-0}"
}

oncf_playwright_list_file() {
  local fixture="$1"
  local app_dir base_url list_out err_out rc
  app_dir="$(oncf_fixture_dir "$fixture")"
  base_url="http://preview.example.lvh.me:8787"
  oncf_write_playwright_config "$app_dir" "$base_url" || return $?
  list_out="$(mktemp)"
  err_out="$(mktemp)"
  set +e
  (
    cd "${ONCF_CLONE_DIR}"
    CELLP_ONCF_BASE_URL="$base_url" pnpm --filter "${fixture}" exec playwright test -c e2e/playwright.cellp.config.ts --list >"$list_out" 2>"$err_out"
  )
  rc=$?
  set -e
  if [[ "$rc" != "0" ]] || ! grep -qE '\.test\.ts' "$list_out" 2>/dev/null; then
    echo "FAIL: playwright --list failed for ${fixture} (exit ${rc})" >&2
    tail -20 "$err_out" >&2 || true
    rm -f "$list_out" "$err_out"
    return 1
  fi
  rm -f "$err_out"
  echo "$list_out"
}

# Parse Playwright --list output. The list includes explicit test.skip declarations,
# so runnable is derived from the listed total minus the source-declared skips.
oncf_parse_list_count() {
  local list_file="$1"
  grep -cE '\.test\.ts' "$list_file" 2>/dev/null || echo 0
}

oncf_collect_fixture_stats() {
  local fixture="$1"
  local list_file listed decl_skip runnable
  list_file="$(oncf_playwright_list_file "$fixture")"
  listed="$(oncf_parse_list_count "$list_file")"
  decl_skip="$(oncf_declared_skip_count "$fixture")"
  runnable=$((listed - decl_skip))
  rm -f "$list_file"
  echo "${runnable} ${listed} ${decl_skip}"
}

oncf_validate_collect_counts() {
  local failed=0
  local sum_runnable=0
  local sum_listed=0
  local sum_decl_skip=0
  local sum_expect_runnable=0
  local fixture runnable listed decl_skip expect_runnable expect_listed expect_decl
  local -a targets
  if [[ -n "${ONCF_COLLECT_FIXTURES:-}" ]]; then
    targets=()
    for _f in ${ONCF_COLLECT_FIXTURES}; do
      targets+=("$_f")
    done
  else
    targets=("${ONCF_FIXTURES[@]}")
  fi
  for fixture in "${targets[@]}"; do
    read -r runnable listed decl_skip <<<"$(oncf_collect_fixture_stats "$fixture")"
    expect_runnable="$(oncf_expect_runnable "$fixture")"
    expect_listed="$(oncf_expect_listed "$fixture")"
    sum_expect_runnable=$((sum_expect_runnable + expect_runnable))
    case "$fixture" in
      app-router) expect_decl=$ONCF_EXPECT_SKIP_APP_ROUTER ;;
      pages-router) expect_decl=$ONCF_EXPECT_SKIP_PAGES_ROUTER ;;
      app-pages-router) expect_decl=$ONCF_EXPECT_SKIP_APP_PAGES_ROUTER ;;
      *) expect_decl=0 ;;
    esac
    oncf_log "collect ${fixture}: listed=${listed} declared test.skip=${decl_skip} runnable=${runnable} (expect ${expect_listed}/${expect_decl}/${expect_runnable})"
    if [[ "$listed" != "$expect_listed" ]]; then
      echo "FAIL: ${fixture} listed count ${listed} != expected ${expect_listed}" >&2
      failed=1
    fi
    if [[ "$runnable" != "$expect_runnable" ]]; then
      echo "FAIL: ${fixture} runnable count ${runnable} != expected ${expect_runnable}" >&2
      failed=1
    fi
    if [[ "$decl_skip" != "$expect_decl" ]]; then
      echo "FAIL: ${fixture} declared test.skip ${decl_skip} != expected ${expect_decl}" >&2
      failed=1
    fi
    sum_runnable=$((sum_runnable + runnable))
    sum_listed=$((sum_listed + listed))
    sum_decl_skip=$((sum_decl_skip + decl_skip))
  done
  oncf_log "collect total listed=${sum_listed} declared skip=${sum_decl_skip} runnable=${sum_runnable}"
  if [[ "$sum_runnable" != "$sum_expect_runnable" ]]; then
    echo "FAIL: total runnable ${sum_runnable} != ${sum_expect_runnable}" >&2
    failed=1
  fi
  if [[ ${#targets[@]} -eq ${#ONCF_FIXTURES[@]} && "$sum_listed" != "$ONCF_EXPECT_LISTED_TOTAL" ]]; then
    echo "FAIL: total listed ${sum_listed} != ${ONCF_EXPECT_LISTED_TOTAL}" >&2
    failed=1
  fi
  if [[ ${#targets[@]} -eq ${#ONCF_FIXTURES[@]} && "$sum_decl_skip" != "$ONCF_EXPECT_SKIP_TOTAL" ]]; then
    echo "FAIL: total declared skip ${sum_decl_skip} != ${ONCF_EXPECT_SKIP_TOTAL}" >&2
    failed=1
  fi
  if [[ "$failed" -ne 0 ]]; then
    return 1
  fi
  return 0
}

oncf_gateway_port() {
  local gw_port="${GATEWAY_URL##*:}"
  gw_port="${gw_port%%/*}"
  echo "${gw_port:-8787}"
}

oncf_preview_base_url() {
  local project="$1"
  local version="$2"
  local scheme="${CELLP_PUBLIC_SCHEME_PREVIEW:-http}"
  local port
  port="$(oncf_gateway_port)"
  echo "${scheme}://$(preview_host "$project" "$version"):${port}"
}

oncf_inject_deploy_url() {
  local wrangler_path="$1"
  local deploy_url="$2"
  node -e "
const fs = require('fs');
const p = process.argv[1];
const url = process.argv[2];
let raw = fs.readFileSync(p, 'utf8');
let j;
try { j = JSON.parse(raw); } catch {
  raw = raw.replace(/^\\s*\\/\\/.*\$/gm, '').replace(/\\/\\*[\\s\\S]*?\\*\\//g, '').replace(/,\\s*([}\\]])/g, '\$1');
  j = JSON.parse(raw);
}
j.vars = j.vars || {};
j.vars.DEPLOY_URL = url;
j.vars.PUBLIC_BASE_URL = url;
let out = JSON.stringify(j, null, 2) + '\\n';
out = out.replace(/:\\/\\//g, ':\\\\u002f\\\\u002f');
fs.writeFileSync(p, out);
" "$wrangler_path" "$deploy_url"
}

# Generate the exact {key,file} objects used by the pinned OpenNext
# populateCache command, then import them into this version's real fleet bucket.
oncf_populate_r2_cache() (
  local app_dir="$1"
  local project="$2"
  local version="$3"
  local bucket_name manifest_dir manifest objects

  bucket_name="$(jq -er '
    [.r2_buckets[]? | select(.binding == "NEXT_INC_CACHE_R2_BUCKET") | .bucket_name]
    | if length == 1 and (.[0] | type == "string") and (.[0] | length > 0)
      then .[0]
      else error("expected exactly one NEXT_INC_CACHE_R2_BUCKET binding")
      end
  ' "${app_dir}/.cellp-wrangler.jsonc")" || exit $?
  manifest_dir="$(mktemp -d "${TMPDIR:-/tmp}/cellp-oncf-r2-cache.XXXXXX")" || exit $?
  trap 'rm -rf "$manifest_dir"' EXIT
  manifest="${manifest_dir}/r2-bulk-list.json"

  objects="$({
    cd "$ONCF_CLONE_DIR" || exit $?
    node --input-type=module - "$ONCF_CLONE_DIR" "$app_dir" "$manifest" <<'NODE'
import fs from "node:fs";
import path from "node:path";
import { pathToFileURL } from "node:url";

const [cloneDir, appDir, manifest] = process.argv.slice(2);
const populateUrl = pathToFileURL(
  path.join(cloneDir, "packages/cloudflare/dist/cli/commands/populate-cache.js")
).href;
const internalUrl = pathToFileURL(
  path.join(cloneDir, "packages/cloudflare/dist/api/overrides/internal.js")
).href;
const { getCacheAssets } = await import(populateUrl);
const { computeCacheKey } = await import(internalUrl);
const assets = getCacheAssets({ outputDir: path.join(appDir, ".open-next") });
if (assets.length === 0) throw new Error("OpenNext emitted no incremental cache assets");
const objects = assets.map(({ fullPath, key, buildId, isFetch }) => ({
  key: computeCacheKey(key, {
    prefix: undefined,
    buildId,
    cacheType: isFetch ? "fetch" : "cache",
  }),
  file: fullPath,
}));
fs.writeFileSync(manifest, JSON.stringify(objects));
process.stdout.write(String(objects.length));
NODE
  })" || exit $?

  rustfs_s3_env || exit $?
  celld r2 bulk put "$bucket_name" --filename "$manifest" \
    --bucket "s3://cellp-celld/${project}/${version}" \
    --endpoint "${S3_ENDPOINT:-http://127.0.0.1:19000}" \
    --region "${AWS_REGION:-us-east-1}" --json >/dev/null || exit $?
  oncf_log "official R2 cache manifest imported ${objects} object(s) into ${project}/${version}/${bucket_name}"
)

oncf_celld_log_path() {
  local project="$1"
  local version="$2"
  local tmp_dir="${TMPDIR:-/tmp}"
  echo "${tmp_dir%/}/celld-${project}-${version}.log"
}

# Unlike e2e/scripts/lib.sh poll_version, this helper never exits the caller.
# A failed fixture must be recorded while the remaining official fixtures continue.
oncf_poll_preview() {
  local project="$1"
  local version="$2"
  local timeout="${3:-300}"
  local elapsed=0
  local status=""
  local body=""
  local log_path
  log_path="$(oncf_celld_log_path "$project" "$version")"

  while [[ "$elapsed" -lt "$timeout" ]]; do
    api_status GET "/v1/projects/${project}/versions/${version}"
    body="$API_BODY"
    status="$(printf '%s' "$body" | jq -r '.status // empty' 2>/dev/null || true)"
    if [[ "$API_STATUS" == "200" && "$status" == "ready" ]]; then
      oncf_log "deploy state project=${project} version=${version} status=${status} elapsed=${elapsed}s"
      return 0
    fi
    if [[ "$status" == "failed" ]]; then
      break
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done

  echo "FAIL: deploy state project=${project} version=${version} http=${API_STATUS:-000} status=${status:-unknown} elapsed=${elapsed}s" >&2
  if [[ -n "$body" ]]; then
    printf '%s' "$body" | jq '{id, project_id, status, error, preview_url, upstream_host, upstream_port}' >&2 2>/dev/null \
      || echo "WARN: version response was not valid JSON" >&2
  fi
  echo "celld_log=${log_path}" >&2
  if [[ ! -f "$log_path" ]]; then
    echo "WARN: celld log not found" >&2
  fi
  return 1
}

oncf_stage_and_register_preview() {
  local fixture="$1"
  local version="$2"
  local project app_dir dest deploy_url
  project="$(oncf_project_id "$fixture")" || return $?
  app_dir="$(oncf_fixture_dir "$fixture")" || return $?
  dest="${ARTIFACTS_DIR}/${project}/${version}"
  deploy_url="$(oncf_preview_base_url "$project" "$version")" || return $?

  # The caller intentionally disables errexit to collect per-fixture results.
  # Check every safety precondition before staging and propagate every required
  # command failure explicitly so a failed preview can never bootstrap prod.
  oncf_require_existing_prod "$project" || return $?
  if [[ -e "$dest" ]]; then
    echo "FAIL: refuse to overwrite existing version artifact ${dest}" >&2
    return 1
  fi

  export CELLP_ONCF_PROJECT="$project"
  export CELLP_ONCF_DEPLOY_URL="$deploy_url"
  export SUPPORT_RSYNC_NO_NODE=1

  oncf_log "prepare cellp artifact ${project}/${version}"
  bash "${E2E_ROOT}/dev/examples/support-opennext-official/prepare-artifact.sh" "$app_dir" || return $?

  cd "$app_dir" || return $?
  oncf_inject_deploy_url ./.cellp-wrangler.jsonc "$deploy_url" || return $?

  mkdir -p "$dest" || return $?
  cp ./.cellp-wrangler.jsonc "$dest/wrangler.jsonc" || return $?
  rsync -a ./.cellp-bundle/ "$dest/.cellp-bundle/" || return $?
  rsync -a ./.cellp-assets/ "$dest/.cellp-assets/" || return $?

  sync_artifact_to_rustfs "$project" "$version" || return $?
  oncf_populate_r2_cache "$app_dir" "$project" "$version" || return $?
  create_version "$project" "$version" >/dev/null || return $?

  if ! oncf_poll_preview "$project" "$version" "${ONCF_POLL_SECS:-300}"; then
    echo "FAIL: deploy ${project}/${version} not ready; version retained" >&2
    return 1
  fi
  oncf_log "preview ready Host=$(preview_host "$project" "$version") base=${deploy_url}"
}

oncf_run_playwright() {
  local fixture="$1"
  local version="$2"
  local project app_dir base_url pw_log result_dir rc
  project="$(oncf_project_id "$fixture")" || return $?
  app_dir="$(oncf_fixture_dir "$fixture")" || return $?
  base_url="$(oncf_preview_base_url "$project" "$version")" || return $?
  result_dir="${app_dir}/e2e/.cellp-runs/${version}"
  mkdir -p "$result_dir" || return $?
  oncf_write_playwright_config "$app_dir" "$base_url" || return $?
  pw_log="${EVIDENCE_DIR}/opennext-official-${fixture}-${version}.log"
  oncf_log "playwright ${fixture} → ${pw_log}; traces=${result_dir}"
  (
    cd "${ONCF_CLONE_DIR}" || exit $?
    CELLP_ONCF_BASE_URL="$base_url" CELLP_ONCF_OUTPUT_DIR="$result_dir" CI=1 \
      pnpm --filter "${fixture}" exec playwright test -c e2e/playwright.cellp.config.ts --workers=1 2>&1 | tee "$pw_log"
  )
  rc=$?
  return "$rc"
}
