# 社区 Support 项目 — 支持 / 不支持（唯一口径）

> **判定日期：** 2026-09-02  
> **环境：** 本地 dev · `lvh.me:8787` Host ingress · 单 Worker / 单 version / 单 celld（**无** `[[services]]` 多 Worker）  
> **只两档：** **支持** | **不支持**（无 ready / partial / warn / pass）

## 判定规则

| 档位 | 含义 |
|------|------|
| **支持** | `deploy-support-app.sh` 可部署到 ready；约定 URL 可打开主界面或**一次**预期重定向（302/307）且**不出现** `ingress_unknown`、502、主流程 **500** |
| **不支持** | 无法部署、运行时/框架 blocked、依赖**多 service Worker**、或主入口不可用 |

多 Worker / service binding：**平台不支持** → 依赖它的仓库一律 **不支持**（见 [MULTI-WORKER-DEPLOY.md](./plans/MULTI-WORKER-DEPLOY.md)）。

---

## 总表（S01–S21）

| ID | 项目 | **支持？** | 验证 URL（支持时） | 不支持原因 |
|----|------|:--------:|-------------------|------------|
| S01 | Relay | **支持** | http://support-relay.lvh.me:8787/ | — |
| S02 | ni-mail | **不支持** | — | 未验证 / blocked（队列外） |
| S03 | Tempik | **不支持** | — | 曾可部署，**不纳入产品支持范围** |
| S04 | Kukuroo | **不支持** | — | 同上 |
| S05 | FlareMo | **支持** | http://support-flaremo.lvh.me:8787/ | — |
| S06 | Memos | **支持** | http://support-memos.lvh.me:8787/ | — |
| S07 | Monolith | **支持** | http://support-monolith.lvh.me:8787/ | — |
| S08 | EdgeEver | **支持** | http://support-edgeever.lvh.me:8787/ | — |
| S09 | SonicJS | **支持** | http://support-sonicjs.lvh.me:8787/ | — |
| S10 | NodeWarden | **支持** | http://support-nodewarden.lvh.me:8787/ | — |
| S11 | Sink | **支持** | http://support-sink.lvh.me:8787/ | — |
| S12 | Inkstone | **不支持** | — | 同上 |
| S13 | SaaSMail | **不支持** | — | 同上 |
| S14 | cloudflarebase | **不支持** | — | 需 **AUTH_AGENT 等多 Worker**；cellp **不编排 service** |
| S15 | workflows-starter | **支持** | http://support-workflows.lvh.me:8787/ | — |
| S16 | pastebin-worker | **不支持** | — | celld 二次 esbuild / SSR·Tailwind 管线（A 类） |
| S17 | r2filebox | **支持** | http://support-r2filebox.lvh.me:8787/ | — |
| S18 | webhookflare | **支持** | http://support-webhookflare.lvh.me:8787/ | — |
| S19 | request-bin | **不支持** | — | 根路径无应用；仅 `/new` 可用，**不符合「打开即主界面」** |
| S20 | R2-Explorer | **支持** | http://support-r2explorer.lvh.me:8787/ | — |
| S21 | FileWorker | **支持** | http://support-fileworker.lvh.me:8787/ | — |

---

## 汇总

| | 数量 |
|---|------|
| **支持** | **14** |
| **不支持** | **7** |
| **合计** | **21** |

**支持率（本清单）：** 14/21 ≈ **67%**

---

## 框架验证（S22–S25 · AD-13）

| ID | 框架 | **支持？** | 验证 URL（支持时） | 备注 |
|----|------|:--------:|-------------------|------|
| S22 | **Astro** | **支持** | http://support-astro.lvh.me:8787/ | `@astrojs/cloudflare` · `dev/examples/support-astro/` |
| S23 | **SvelteKit** | **支持** | http://support-sveltekit.lvh.me:8787/ | C3 `adapter-cloudflare` · `dev/examples/support-sveltekit/`；prod 验收 2026-09-03（v1） |
| S24 | **Remix** | **支持** | http://support-remix.lvh.me:8787/ | `@remix-run/cloudflare` · `dev/examples/support-remix/`（wrangler bundle + `.cellp-assets`，剔除 `.assetsignore`） |
| S25 | **Nuxt** | **支持** | http://support-nuxt.lvh.me:8787/ | Nitro `cloudflare_module` · `dev/examples/support-nuxt/`；prod 验收 2026-09-03（v1，含 `/_nuxt/builds/latest.json`）· celld `node:timers` |

## README 框架扩展（S26–S30）

| ID | 框架 | **支持？** | 验证 URL | 备注 |
|----|------|:--------:|----------|------|
| S26 | **Hono** | **支持** | http://support-hono.lvh.me:8787/ | C3 `create-hono` + Workers Assets · `dev/examples/support-hono/` · **v3** · `GET /message` → `Hello Hono!` · `/` → `public/index.html` |
| S27 | **SolidStart** | **不支持** | — | `create-solid -s --v2 -t basic` + rsync C3 `templates/solid/` · slim artifact OK（~324 KiB）· deploy **celld `signal: killed`**（与当日其它 job 同）· `docs/evidence/support-S27-20260903-deploy.log` · prod `grep 'Hello world'` 未验 |
| S28 | **Qwik City** | **不支持** | — | `create-qwik` + cloudflare-workers · slim artifact OK · **v4** deploy `celld health timeout` **8833** · load `process.stdin`（unenv）· prod `ingress_unknown` · 见 `docs/evidence/verify-full-20260903.log` |
| S29 | **Waku** | **支持** | http://support-waku.lvh.me:8787/ | `create-waku` + C3 overlay · **v9** ready + promote · prod/preview **200** · grep `Waku` / `An internet website!` · celld: sibling `.js`→`EsModule` + relative resolve · `docs/evidence/support-S29.log` |
| S30 | **Next.js (OpenNext)** | **不支持** | — | `prepare-artifact` + `no_bundle` · **v10** ready + promote · prod `GET /` 仍 **308** `Location: ?`（celld `445569a` · 无 `global_fetch_strictly_public`）· `verify-full-20260903.log` · [NEXT-OPENNEXT-CELLP.md](./plans/NEXT-OPENNEXT-CELLP.md) |

---

## 高 Star 生态（验证队列）

**口径：** 尚未写入 S01–S21 总表 verdict 的明星 OSS；状态 **测试中** / **排队** / **难**。明细与 GitHub 链接：**[support-star-queue.md](./support-star-queue.md)**。

| 项目 | 队列状态 | 备注 |
|------|----------|------|
| Sink | **已纳入 S11** | **支持** · `dev/examples/support-sink/` · prod **v2**（2026-09-03） |
| Counterscale | **不支持** | turbo **pnpm** monorepo，无单 wrangler 入口 |
| CloudPaste | **不支持** | 多组件 Pages + Worker 拆分，未纳入单 Worker 槽位 |
| cf-workers-status-page | **不支持** | 旧 **flareact/webpack** `[site]` 栈，未适配 cellp slim artifact |
| CloudFlare-ImgBed · UptimeFlare · microfeed · … | **排队** | 见 star-queue |
| VibeSDK | **难** | WFP + 多 DO（仅 star-queue 备注，非 Agent 主线） |
| Cloudflare OS | **🔜 计划支持** | 见 CODING-AGENT 计划 |

**排除：** 各类 **email Worker** 不入队。

---

## Coding Agent on cellp（前沿）

LLM → build → `POST /versions` → preview Host → promote；对齐 [Agent Cloud](https://agents.cloudflare.com/) · Cloudflare OS · Eve · Pi · Deep Agents。

| 目标 | 状态 |
|------|------|
| **[Cloudflare OS](https://github.com/cloudflare/cloudflare-os)** | **🔜 计划支持** |
| **P0 验证** | **A01–A03** → [AGENT-SUPPORT.md §P0](./AGENT-SUPPORT.md#p0--cellp-deploy-support-validation-ordered) |
| **P1 fx** | **[fx-on-workers](https://github.com/codingstark-dev/fx-on-workers)** ([fx.sh](https://fx.sh)) · **A04** · **支持** · v9 |
| **[Cloudflare Agents](https://agents.cloudflare.com/)** | **🔜 对齐 Agent Cloud**（SDK + DO + Workflows；见 P0） |
| **[Eve](https://github.com/vercel/eve)** (Vercel) | **🔜 研究** |
| **[Pi](https://github.com/earendil-works/pi)** · **[Deep Agents](https://github.com/langchain-ai/deepagents)** | **🔜 研究** |


### P0 Agent 验证（A01–A03）

| ID | 项目 | **支持？** | 验证 URL（支持时） | 备注 |
|----|------|:--------:|-------------------|------|
| A01 | **agents-starter** | **支持（部分）** | http://support-agents-starter.lvh.me:8787/ | **v10** · prod `GET /` **200**（Agent Starter SPA + `/assets/*`）· `prepare-artifact.sh` + overlay 无 `AI` · 多轮推理依赖 **Workers AI**（celld 缺口） |
| A02 | **pi-worker** (`hello-agent`) | **支持** | http://support-pi-worker.lvh.me:8787/ | OpenAI 兼容 **OpenCode Zen**（`OPENAI_*`）+ R2 工具多轮；overlay `hello-agent.src` |
| A03 | **opencode-do** | **支持（部分）** | http://support-opencode-do.lvh.me:8787/ | **v1** · prod `GET /` **200** · `POST /session` + `POST/GET .../message` **JSON 多轮持久化** · assistant 文案为 Workers AI 不可用占位 · `GET /event` SSE **长连接超时** |
| A04 | **fx-on-workers** | **支持** | http://support-fx-on-workers.lvh.me:8787/?key=cellp-dev-fx-on-workers | **v9** · **WebSocket `/session` 101** + TUI（`fx-websocket-smoke.sh`）· **`AI_GATEWAY_API_KEY`** + `FX_MODEL`（Vercel Gateway，非 OpenCode） |

**计划：** [plans/CODING-AGENT-ON-CELLP.md](./plans/CODING-AGENT-ON-CELLP.md) · **Vercel OSS（后续）：** [VERCEL-SUPPORT.md](./VERCEL-SUPPORT.md) · [AGENT-SUPPORT.md](./AGENT-SUPPORT.md)

---

## Vercel framework on cellp（后续专题）

**口径：** Vercel 托管能力 ❌；仅 wrangler 可投递产物。**[VERCEL-SUPPORT.md](./VERCEL-SUPPORT.md)**

| 组件 | 状态 |
|------|------|
| Next.js / OpenNext | **不支持**（S30 实验）· [NEXT-OPENNEXT-CELLP.md](./plans/NEXT-OPENNEXT-CELLP.md) · `docs/evidence/support-S30.log` |
| AI SDK (`vercel/ai`) | ⚠️ 打进 Worker 包 |
| Workflow SDK | 🔜 对照 CF Workflows |
| fx → fx-on-workers | **支持** · A04 · v9 |
| Eve · open-agents | 🔜 研究 |

---

## 平台能力边界（与上表一致）

| 能力 | |
|------|---|
| 单 Worker + D1/KV/R2/Queue/Workflow（一份 wrangler） | **支持** |
| `wrangler [[services]]` 多 Worker 编排 | **不支持** |
| Host `*.lvh.me` prod ingress | **支持**（需 promote / `ingress-repromote-support.sh`） |

---

## 其它文档

- **Support 文档索引：** **[support/README.md](./support/README.md)**
- curl 明细：**[support-curl-user-acceptance.md](./support-curl-user-acceptance.md)**
- 待办队列排序：**[support-todos.md](./support-todos.md)**
- 高 Star 验证队列：**[support-star-queue.md](./support-star-queue.md)**
- Coding Agent 前沿：**[plans/CODING-AGENT-ON-CELLP.md](./plans/CODING-AGENT-ON-CELLP.md)**
