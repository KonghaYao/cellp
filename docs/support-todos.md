# 社区 Workers 项目 — cellp 支持验证待办

> **排序原则：** **有 Web 前端、打开能点** 的优先；API-only / 需另配客户端的往后。  
> **是否支持（唯一口径）：** **[support-matrix.md](./support-matrix.md)** — 每项仅 **支持** 或 **不支持**。  
> **框架与 CF 对照：** **[framework-coverage-cellp.md](./framework-coverage-cellp.md)**（**AD-13** 一等公民 · S22–S25）
> **高 Star 队列：** **[support-star-queue.md](./support-star-queue.md)**（Sink、Counterscale、ImgBed…）  
> **Coding Agent 前沿：** **[plans/CODING-AGENT-ON-CELLP.md](./plans/CODING-AGENT-ON-CELLP.md)** · Pages [research/coding-agent-on-cellp](https://konghayao.github.io/cellp/research/coding-agent-on-cellp)  
> **证据：** `docs/evidence/support-<id>.log` · **踩坑：** [support-validation-lessons.md](./support-validation-lessons.md) · **平台缺陷：** [platform-defects-log.md](./platform-defects-log.md)

---

## 优先级：前端界面

| 档位 | 含义 | 部署后你怎么验 |
|------|------|----------------|
| **P0** | 完整 Web UI，主流程在浏览器 | 打开 Gateway URL，点按钮走通 |
| **P1** | 有 UI，可能需登录/seed | 同上，或 Dashboard + 预览 |
| **P2** | 轻量页面或管理台为主 | 能打开页面，功能可能不全 |
| **P3** | **几乎无 UI**（API/BaaS 模板/CLI） | 仅 curl/Postman，**排最后** |

---

## P0–P1：有前端，优先部署（推荐队列）

| 序 | ID | 项目 | 用户有什么用 | UI | 类型 |
|----|-----|------|--------------|-----|------|
| ✓ | S01 | Relay | 短链 + 统计后台 | `index.html` 管理台 | 工具 |
| 1 | S17 | [r2filebox](https://github.com/workHMZ/r2filebox) | 临时文件/文本分享 | Vue 全站 | 工具 |
| 2 | S16 | [pastebin-worker](https://github.com/SharzyL/pastebin-worker) | 贴文本/小文件分享 | Web 粘贴页 | 工具 |
| 3 | S05 | [FlareMo](https://github.com/realchendahuang/FlareMo) | 自建小 SaaS | 完整 App UI | SaaS |
| 4 | S08 | [EdgeEver](https://github.com/tianma-if/edgeever) | 笔记 + 附件 | 笔记 Web App | 内容 |
| 5 | S06 | [Memos Worker](https://github.com/souvenp/memos-worker) | 碎片笔记 | Memos 风格 UI | 内容 |
| 6 | S20 | [R2-Explorer](https://github.com/G4brym/R2-Explorer) | R2 网盘浏览上传 | 文件管理界面 | 工具 |
| 7 | S21 | [FileWorker](https://github.com/woaiqjj/FileWorker) | 剪贴板/小文件外链 | Web 上传页 | 工具 |
| 8 | S18 | [webhookflare](https://github.com/fayazara/webhookflare) | Webhook 调试 | 请求流 Dashboard | 工具 |
| 9 | S07 | [Monolith](https://github.com/one-ea/Monolith) | 博客/CMS | 管理后台 + 站点 | 内容 |
| 10 | S09 | [SonicJS](https://github.com/SonicJs-Org/sonicjs) | Headless CMS | **Admin 控制台**（非仅 API） | 内容 |
| 11 | S15 | [workflows-starter](https://github.com/cloudflare/templates/tree/main/workflows-starter-template) | 异步任务 | SPA 进度页 | 平台 |

**仓库内（必有 UI）：** `commerce-store`  storefront — `./dev/scripts/seed-commerce-store.sh`

---

## P2–P3：无完整前端，往后

| ID | 项目 | 说明 |
|----|------|------|
| S19 | request-bin | 极简列表页，偏开发者工具 |
| S10 | NodeWarden | 密码库 **客户端** 为主，Worker 偏 API |
| S14 | cloudflarebase | **不支持** — [support-matrix.md](./support-matrix.md) |

---

## blocked（不排批次）

S02 ni-mail · S03 Tempik · S04 Kukuroo · S11–S13 Sink/Inkstone/SaaSMail 等（见前版原因）。

---

## 执行状态

**见 [support-matrix.md](./support-matrix.md)**。**2026-09-03：** P2 **S34 microfeed ✅**（Astro · D1+Queue+R2 · prod **v4** `support-microfeed.lvh.me` **200**）；P1 高 Star **S31 ImgBed ✅ · S32 status-page ✅ · S35 OpenStatus ✅**；**S36 Triplit** prod v1 ready · **不支持**（API-only）；**S30 OpenNext / S33 UptimeFlare（Next 栈）⏸️ 后排**。框架 **S26–S29 ✅**。

**框架一等公民（S22–S25）** 见下表。

---

## 框架验证（S22–S25 · AD-13）

| ID | 框架 | 来源 | 命令 |
|----|------|------|------|
| S22 | **Astro** | `cloudflare/templates` · `astro-blog-starter-template` | `./dev/scripts/deploy-support-app.sh S22` |
| S23 | **SvelteKit** | `cloudflare/workers-sdk` · `packages/create-cloudflare/templates/svelte` + `dev/examples/support-sveltekit/` | `./dev/scripts/deploy-support-app.sh S23` · **已验 v1 · 支持** |
| S24 | **Remix** | `cloudflare/templates` · `remix-starter-template` + `dev/examples/support-remix/` | `./dev/scripts/deploy-support-app.sh S24` · **已验 v2 · 支持** |
| S25 | **Nuxt** | `cloudflare/workers-sdk` · `packages/create-cloudflare/templates/nuxt` + `dev/examples/support-nuxt/` | `./dev/scripts/deploy-support-app.sh S25` · **已验 v1 · 支持** |

**Next.js：** 非一等；见 [plans/NEXT-OPENNEXT-CELLP.md](./plans/NEXT-OPENNEXT-CELLP.md)。

---

## 批次计划（**UI 优先**）

| 批次 | 项目 | 打开即验 |
|------|------|----------|
| **3** | S17 r2filebox, S16 pastebin-worker | 上传/粘贴页 |
| **4** | S05 FlareMo, S08 EdgeEver | 登录/笔记 |
| **5** | S06 Memos, S20 R2-Explorer | 笔记/网盘 |
| **6** | S21 FileWorker, S18 webhookflare | 文件/Webhook 面板 |
| **7** | S07 Monolith, S09 SonicJS | CMS 后台 |
| **8** | S15 workflows-starter | Workflow SPA |
| **9** | S19 request-bin, S10 NodeWarden | 弱 UI / API |
| **10** | S14 cloudflarebase | 几乎无 UI |

---

## 部署

```bash
./dev/scripts/deploy-support-app.sh S17   # lookup 已含 S16–S21
./dev/scripts/seed-support-relay-demo.sh
```

GitHub 镜像：`ghfast.top`（见 `deploy-support-app.sh`）。

**npm 源：** `deploy-support-app.sh` 默认 **`https://registry.npmmirror.com`**（`dev/.env` 可设 `NPM_CONFIG_REGISTRY`）。

**框架用户行为验收：** [support-framework-user-acceptance.md](./support-framework-user-acceptance.md)（verification，非仅 HTTP 码）。

**端口 / Gateway（E 类）：** 用户仅 **:8787**；每 version celld 为 **8808+ 内部端口**；Worker `origin` 会偏移 → 见 **`docs/support-migrations.md` §端口与路由**。

---

## 失败 taxonomy

A 运行时 · B Binding · C wrangler · D 框架 SSR · E Gateway/控制面 · **F 多 service Worker（不支持）**

---

## 变更 log

| 日期 | 变更 |
|------|------|
| 2026-09-03 | **S34 microfeed**：`support-microfeed` · `deploy-support-app.sh S34` · overlay `dev/examples/support-microfeed/` · prod **v4** Host **200** · **支持** |
| 2026-09-03 | **S36 Triplit**：`support-triplit` · prod **v1** · overlay `dev/examples/support-triplit/` · **不支持**（无 Web 主界面 · `/health` 200 · `/` 401） |
| 2026-09-02 | **A04 fx-on-workers**：v5 ready · prod `/?key=cellp-dev-fx-on-workers` **200** · overlay + `prepare-artifact.sh`（celld `no_bundle` 不含 wasm → wrangler dry-run + celld 再 esbuild） |
| 2026-09-02 | **A02 pi-worker**：`hello-agent` v1 ready · GET/POST `/` 200/400 · **支持**（见 support-matrix §P0 Agent） |
| 2026-09-02 | **P0 agent deploy:** A01 agents-starter · A02 pi-worker · A03 opencode-do（`deploy-support-app.sh`） |
| 2026-09-02 | **platform-defects-log.md**：S22 暴露 `cloudflare:workers` / `caches` |
| 2026-09-02 | **S25 Nuxt**：`support-nuxt` overlay · prod `/robots.txt` 200、`/` SSR 挂起 → **不支持**（PD-20260902-06） |
| 2026-09-02 | **AD-13**：一等公民 Astro/SvelteKit/Remix/Nuxt；S22–S25 lookup；Next/OpenNext 实验文档 |
| 2026-09-02 | 批次 5–10 部署；多 **service** 标 **不支持**；S14 partial；ingress repromote；celld crypto |
| 2026-08-31 | UI 优先排序；批次 3 起改为 r2filebox + pastebin |
