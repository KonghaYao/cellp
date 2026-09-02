# Support 批次部署与验收汇总

> **状态：已完成**（2026-09-02）  
> **克隆：** `GITHUB_CLONE_DIRECT=1` · **栈：** `./dev/scripts/up.sh`  
> **证据：** `docs/evidence/support-<ID>.log`（本地 gitignore）

| ID | 项目 | 部署 | 验收 | Prod URL | HTTP | 备注 |
|----|------|------|------|----------|------|------|
| S06 | support-memos | ok v1 | pass | http://support-memos.lvh.me:8787/ | 200 | |
| S20 | support-r2explorer | ok v2 | pass | http://support-r2explorer.lvh.me:8787/ | 200 | |
| S21 | support-fileworker | ok v1 | pass | http://support-fileworker.lvh.me:8787/ | 200 | |
| S18 | support-webhookflare | ok v1 | pass | http://support-webhookflare.lvh.me:8787/ | 200 | |
| S07 | support-monolith | ok v3 | pass | http://support-monolith.lvh.me:8787/ | 200 | |
| S09 | support-sonicjs | ok v2 | pass | http://support-sonicjs.lvh.me:8787/ | 302 | |
| S10 | support-nodewarden | ok v1 | pass | http://support-nodewarden.lvh.me:8787/ | 200 | |
| S19 | support-requestbin | ok v2 | partial | http://support-requestbin.lvh.me:8787/ | `/` 404 · `/new` 302 | 无根路由；入口 `/new` |
| S15 | support-workflows | **ok v2** | **pass** | http://support-workflows.lvh.me:8787/ | **200** | ASSETS + worker patch |
| S14 | support-cfbase | **ok v6** | **pass** | http://support-cfbase.lvh.me:8787/ | **307** → `/dashboard` | caches polyfill + slim bundle |

**本批次其余：** S01/S05/S08/S17 已 ready · S16 blocked · S02–S04 blocked

---

## S19 / S14 / S15（子 agent 收尾）

### S19 request-bin
- `dev/examples/support-requestbin/prepare-artifact.sh`（wrangler dry-run，`no_bundle`）
- 验收：Worker 正常；`GET /new` → 302

### S15 workflows（v2）
- `prepare-artifact.sh`：patch `worker/index.ts` 走 `ASSETS.fetch`；`.cellp-assets`；排除 `.assetsignore`
- `wrangler.cellp.jsonc`：`ASSETS`、`run_worker_first`
- 验收：**根路径 200**（SPA HTML）

### S14 cloudflarebase（v6）
- `prepare-artifact.sh`：wrangler dry-run bundle + **caches polyfill**；Svelte 静态 → `.cellp-assets`
- `stage-artifact-extra.sh`：slim artifact
- 验收：**307** → `/dashboard`（SvelteKit 预期）

---

## 新增/变更的 dev 示例与脚本

- `dev/examples/support-requestbin/prepare-artifact.sh`
- `dev/examples/support-memos/wrangler.cellp.jsonc`
- `dev/examples/support-monolith/wrangler.cellp.jsonc`
- `dev/examples/support-workflows/prepare-artifact.sh` + `stage-artifact-extra.sh` + `wrangler.cellp.jsonc`
- `dev/examples/support-cfbase/prepare-artifact.sh` + `stage-artifact-extra.sh`
- `dev/scripts/deploy-support-app.sh`（S07 `server/`、prebuilt slim stage 等）
