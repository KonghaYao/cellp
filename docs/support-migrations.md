# 社区项目 → cellp 部署改造记录

> 供后续 **Cloudflare → cellp 迁移文档** 引用。每个 `S-id` 一节：原项目假设、cellp 差异、改造清单、验证 URL。  
> 部署命令：`./dev/scripts/deploy-support-app.sh <S-id>`

---

## 通用改造（多数项目）

| 项 | CF 默认 | cellp 做法 |
|----|---------|------------|
| 公网 URL | `*.workers.dev` 或自定义域 **根路径** | Gateway **`http://{host}:8787/`**（AD-12 **Host** 选 version；见 `preview_url` / `prod_url`） |
| 前端 API | `fetch('/api/…')` | 需 `apiUrl()` 或 `<base>` / 构建时 `BASE_URL`（**E 类**） |
| wrangler | `observability`、`routes`、`preview_urls`、`email`… | overlay 或 `strip_wrangler_for_celld`（**C 类**） |
| GitHub | 直连 | `GITHUB_CLONE_MIRROR=https://ghfast.top/https://github.com/` |
| npm | postinstall workerd | `NPM_CONFIG_IGNORE_SCRIPTS=true` |
| 依赖 | 不含 node_modules | 有 `npm install` 时 artifact **含 node_modules** |
| **RustFS 上传** | `sync_artifact_to_rustfs`（`lib.sh`） | `artifact_uri` 为 `s3://` 时 orch 从 RustFS 拉取；**仅写本地目录不够** |
| D1 | `wrangler d1 execute` | `seed.db` 放入 artifact 目录 |

---

## 端口与路由（**必记 · E 类**）

cellp **不是**「Worker 占一个固定端口」。浏览器只认 **Gateway**；每个 ready version 的 celld 在 **动态端口**（如 `8808`、`8809`…），由 cellpd **反代**，用户**不应**直连。

| 入口 | 默认端口 | 用途 | 浏览器 / 前端 |
|------|----------|------|----------------|
| **Gateway** | **8787** | 用户访问 Worker + 静态资源 | ✅ **唯一**对外入口；URL 见 API **`preview_url` / `prod_url`**（含 Host + `:8787` dev） |
| Platform API | 8790 | 注册 version、Dashboard 后端 | ❌ 不是业务 API |
| celld 探针 | 8792 | `/.well-known/celld/health` | ❌ 诊断用 |
| **每 version celld** | **8808+ 动态** | Gateway 反代目标 | ❌ **禁止**写进前端；健康 JSON 里若出现 `:8809` 是内部 upstream |

**Gateway 行为（AD-12，2026-09）：**

1. 选路键为 **HTTP Host**（`{version}.{project}.{base}` / `{project}.{base}`），**不**再用 `/{project}/{version}/` path 选 version（已废弃）。
2. 业务 path **从 `/` 起、不 strip**；Worker 内 `url.pathname` 与用户 URL 一致（与 CF 根域一致）。
3. 反代设 **`Host: synthetic_host`** + **`X-Forwarded-Host`**（浏览器 authority）；`PUBLIC_BASE_URL` 注入对外 origin。Dev 配置：[dev/INGRESS-HOST.md](../dev/INGRESS-HOST.md)。
4. **prod** 为 prod Host（`prod_url`）；promote 切 upstream，**Host 不变**。

**迁移对策（记录到各项目改造表）：**

| 问题 | 做法 |
|------|------|
| 前端 API | 相对路径 `/api/…`（推荐）；或 `PUBLIC_BASE_URL` / env 前缀 |
| 前端静态资源 | Vite `base: './'`（见 S17） |
| Clash + lvh.me | [dev/clash/README.md](../dev/clash/README.md) 直连规则 |
| Worker 生成绝对链接 | 配置 **`PUBLIC_BASE_URL`**（wrangler `vars`）= Gateway 完整前缀；或读 `X-Forwarded-*`（**cellp 若未注入则只能 vars**） |
| 误连 `:8792` / `:8809` | 视为部署/配置错误，不是「改端口」能永久解决 |

**待产品化（cellp 机制，非社区改造）：** Gateway 注入 `X-Forwarded-Prefix` / `X-Forwarded-Host`（用户 Host + 8787），Worker 按 RFC 7239 拼公开 URL。在此之前，**不要**在社区项目里堆 `PUBLIC_BASE_URL` / Referer 补丁——属于 **平台机制债务**，记入 cellp backlog，不算 S-id 兼容项。

---

## S01 Relay

**状态：** ready · `seed-support-relay-demo.sh`

| 改造 | 说明 |
|------|------|
| wrangler | 无 jsonc 时合成；完整部署用 `dev/examples/support-relay/wrangler.cellp.jsonc`（D1 + ASSETS + ADMIN_TOKEN vars） |
| worker.js | 根路径 `Accept: text/html` 时 `ASSETS` 提供 `index.html` |
| static/index.html | 预填 `relay_api_base`、`relay_token` |
| D1 | `dev/examples/support-relay/seed.sh` → 示例 slug demo/cellp/relay |
| 文档 | `dev/scripts/seed-support-relay-demo.sh` |

---

## S03 Tempik

**状态：** ready 但 **blocked**（收信不可用）

| 改造 | 说明 |
|------|------|
| wrangler overlay | 去掉 `[email]`；`dev/examples/support-tempik/wrangler.cellp.jsonc` |
| patch-web | `apiUrl()` + 相对 `styles.css`/`app.js` |
| D1 | `schema.sql` → seed.db |
| 脚本 | `seed-support-tempik-demo.sh` |

---

## S17 r2filebox

**状态：** ready · **验证 URL：** 使用 API `preview_url`（Host + `:8787`），非 path `/<project>/<version>/`
**脚本：** `./dev/scripts/seed-support-r2filebox-demo.sh`（`SUPPORT_VERSION=v8` 可指定）

| 改造 | 文件 | 说明 |
|------|------|------|
| Monorepo 构建 | `seed-support-r2filebox-demo.sh` | `frontend`: `npm install && npm run build`；根目录 `npm install`（`NPM_CONFIG_IGNORE_SCRIPTS=true`） |
| wrangler overlay | `dev/examples/support-r2filebox/wrangler.cellp.jsonc` | 去掉 CF `ratelimits` / `analytics_engine` / `observability` / `version_metadata` |
| **禁止** `run_worker_first` 含 `/*` | 同上 | cellp `stripJSONC` 误删 `"/api/*"` 后整文件 → **parse wrangler: unexpected end**（**cellp bug**，待修） |
| 去掉 `triggers.crons` | overlay | 非 prod 时走 cron strip 拷贝；也可直接不写 cron |
| D1 | `dev/examples/support-r2filebox/seed.sh` | 合并 `worker/migrations/*.sql` → `seed.db` |
| API 前缀 | `patch-frontend.sh` | `CELLp_API_PREFIX` + axios `baseURL`（Gateway `/support-r2filebox/vN`） |
| **分片校验** | `patch-crypto-fallback.sh` | celld **R2 multipart 曾返回 `etag: ""`** → 前端勿要求非空 etag；有 **signed receipt** 即可；**celld** 现已返回 S3 风格 part MD5 etag（需重建 celld + 重启 cellpd） |
| **端口 / 公开 URL** | `public-origin.ts` | 分享链用 Referer/Origin；**勿**在 `wrangler.jsonc` 写 `http://…` 的 `PUBLIC_BASE_URL`（`stripJSONC` 会截断 → D1 seed 失败 → `/api/config` **503**） |
| artifact 体积 | stage | **不** rsync 全量 `node_modules`；只带 `node_modules/hono` + `package.json` |
| **RustFS 上传** | `e2e/scripts/lib.sh` → `sync_artifact_to_rustfs` | `POST /versions` 前 **必须** `s3 sync`（本机无 `aws` 时用 **docker amazon/aws-cli**） |
| deploy 脚本 | `deploy-support-app.sh` | 已调用 `sync_artifact_to_rustfs` + 正确 `artifact_uri` |

**管理后台：** `/admin` · 默认 `admin` / `cellp-dev-r2filebox`

**未解决：** overlay 去掉 `run_worker_first` 后，静态页可能先命中 Worker（需点 `/api` 或后续修 cellp `stripJSONC`）；Turnstile CSP 在本地无 CF 挑战可忽略。

---

## S16 pastebin-worker

**状态：** blocked（**A 类** · celld 打包链）

| 改造 | 说明 |
|------|------|
| overlay | `dev/examples/support-pastebin/wrangler.cellp.jsonc`（KV/R2/ASSETS、`DEPLOY_URL` 占位） |
| build | `npm install && npm run build:frontend`（勿删 corpus 内 `wrangler.toml`，overlay 在 stage 前执行） |
| DEPLOY_URL | 注入时用 `\u002f` 转义，避免 cellp `stripJSONC` 把 `http://` 当注释 |
| vars | celld 要求 **字符串**（如 `"7200"` 非数字） |

**未解决：** Worker 依赖 wrangler 规则 + `.md` import + 前端 SSR（`worker/pages/index.ts` → `PasteBin.tsx` → `tailwindcss`）。`celld deploy` 默认 esbuild 二次打包失败；`no_bundle` 仅单文件，wrangler `--dry-run` 的 `dist/index.js` 仍引用 sibling `.md`。

**证据：** `docs/evidence/support-S16.log`（v6 典型错误：`No loader for .md` / `Could not resolve tailwindcss`）

---

## S05 FlareMo

**状态：** ready · **验证 URL：** `https://v3.support-flaremo.<CELLP_INGRESS_BASE_DOMAIN>:8788/`（promote 后 `https://support-flaremo.<domain>:8788/`）  
**脚本：** `./dev/scripts/deploy-support-app.sh S05`（`SUPPORT_SKIP_BUILD=1` 可跳过 pnpm 若 corpus 已构建）

| 改造 | 文件/脚本 | 说明 |
|------|-----------|------|
| pnpm monorepo | `deploy-support-app.sh` | `pnpm install --ignore-scripts` + `@flaremo/web` build（勿 `npm ci`） |
| wrangler bundle | `prepare-artifact.sh` | `wrangler deploy --dry-run --outdir .cellp-bundle` → `main` + `no_bundle: true` |
| overlay | `wrangler.cellp.jsonc` | D1/R2/assets；去掉 Vectorize/AI；`FLAREMO_EMBEDDING_PROVIDER=none` |
| **禁止** `run_worker_first` 含 `/*` | overlay | cellp `stripJSONC` 会把 `"/api/*"` 当块注释 → **parse wrangler: unexpected end** |
| slim artifact | `deploy-support-app.sh` | `SUPPORT_RSYNC_NO_NODE=1` 时只 stage：`wrangler.jsonc`、`.cellp-bundle/`、`apps/web/dist/`、`migrations/` |
| DEPLOY_URL | inject | `\u002f` 转义 `https://` |

**浏览器：** 单用户模式默认 `owner@flaremo.local`（见 vars）；需在 UI 走登录/初始化流程。

---

## 模板（新 ID 复制）

### S?? project-name

**仓库：**  
**状态：**  
**验证 URL：**

| 改造 | 文件/脚本 | 说明 |
|------|-----------|------|
| | | |

**未解决 / 产品限制：**
