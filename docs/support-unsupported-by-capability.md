# Support「不支持」— 按 cellp / celld 能力缺口

> **口径：** [support-matrix.md](./support-matrix.md) 只标 **支持 | 不支持**。本文把 **不支持** 归到 **我们缺什么能力**（或 **产品明确不做**），便于排期而不是逐 repo 翻备注。  
> **对照：** [decisions.md](./decisions.md) · [MULTI-WORKER-DEPLOY.md](./plans/MULTI-WORKER-DEPLOY.md) · [NEXT-OPENNEXT-CELLP.md](./plans/NEXT-OPENNEXT-CELLP.md)

---

## 1. 运行时 / 平台 binding（celld 相对 Cloudflare Workers）

| 能力缺口 | Cloudflare 上典型形态 | cellp / celld 现状 | 受影响示例 | 备注 |
|----------|----------------------|-------------------|--------------|------|
| **`wrangler [[services]]` 多 Worker 编排** | 主 Worker 绑定 `AUTH_AGENT` 等 **其它 Worker 名** | **不支持**。单 version = 单 celld = **一个** 主 Worker + manifest 内 D1/KV/R2/Queue/DO 等 | **S14** cloudflarebase；凡硬依赖多 service 的 BaaS 模板 | 产品 **Deferred** · [MULTI-WORKER-DEPLOY.md](./plans/MULTI-WORKER-DEPLOY.md) · AD-10 |
| **Analytics Engine** | `analytics_engine_datasets` · 写入/查询计数 | **无 AE binding**；含 AE 的 wrangler 配置 **deploy 失败** 或去掉后核心不可用 | **S38** Counterscale（`/` 登录 200 · `/dashboard` 501） | 非 RustFS；需 celld + 控制面建模 |
| **Cloudflare Images (`IMAGES`)** | 边缘图片变换 API | **Partial**：`input` + `transform` + `output` + `info` 本地变换（Rust `image`，非 CF 付费 API）。`draw`/部分选项 fail-closed | **S31** ImgBed（变换可走 binding；overlay 已加 `images.binding`） | 见 celld `docs/cloudflare-compat.md` |
| **Workers AI** | `@cf/...` 模型推理 | **部分**；HTTP/WS 可通，**多轮 AI 回合**未验收 | **A01** agents-starter（矩阵 **部分支持**） | celld Workers AI 与 CF  parity 缺口 |
| **Email / Email Workers 路由** | `send_email` 等 | **未作为 cellp 目标**（非 celld 单点缺失时仍标队列外） | 邮件类 OSS（**S02–S04、S12–S13** 等多为 **产品范围**） | 见 §2 |

---

## 2. 框架 / 构建形态（AD-13 · 单部署单元）

| 能力缺口 | 典型需求 | cellp / celld 现状 | 受影响示例 | 备注 |
|----------|----------|-------------------|--------------|------|
| **Next.js / OpenNext tier-1** | App Router SSR · OpenNext 单 Worker · `request.url` / `node:http` / 图片路由 | **非一等公民**；S30 根页和 S40 基础 App Router 的固定单 Worker artifact 已通过，但尚无跨版本/功能矩阵与官方模板，**未达 tier-1 门禁** | **S30/S40** OpenNext · **S33** UptimeFlare（Next/Pages）· Supermemory SaaS stack | 固定 artifact 可实验；任意 Next/OpenNext 组合不在承诺内 · [NEXT-OPENNEXT-CELLP.md](./plans/NEXT-OPENNEXT-CELLP.md) · AD-13 |
| **Node SSR 在 Worker 内二次打包** | Worker 内完整 SSR + Tailwind/MD 图 · celld **再 esbuild** | **A 类 blocked**；宜 **wrangler dry-run → `.cellp-bundle` + `no_bundle`** | **S16** pastebin-worker | 与「预打包 Astro/Nuxt」对比 |
| **Pages + Worker 双部署（未合并）** | 静态 Pages 与 API Worker 分离 | cellp 只认 **单 artifact 单 celld**；需 **unified SPA** 或单 Worker 入口 | 原 **CloudPaste** 叙述；**S39 unified SPA → 支持** | 缺口是 **部署模型**，不是 R2 |
| **flareact webpack SW IIFE** | `addEventListener('fetch')` 无 ESM default | 需 **cellp-entry 包装**（已解决一类） | **S32**（已 **支持**） | 曾属缺口；保留作模式参考 |

---

## 3. 控制面 / Gateway / 运维（cellp）

| 能力缺口 | 表现 | 现状 | 受影响示例 | 备注 |
|----------|------|------|--------------|------|
| **Ingress 长连接 / SSE** | `/event` 等 **chunked 长响应** | **已通**：gateway `FlushInterval=-1` + `statusRecorder.Flush`；`text/event-stream` 设 `X-Accel-Buffering: no`。`http.Server` **无** `WriteTimeout`。celld 默认 `CELLD_HANDLER_BUDGET_S=300`，流式 body 走 waitUntil pump（对标 CF Worker 持连流式）。验收勿用「整连接必须在 3s 结束」——SSE **不结束**，看 **首字节**。 | **A03** `GET /event` **PASS**（`server.connected`）。仍 **部分支持** 因 Workers AI 占位 | 长连接 hang 是协议；不是 gateway 超时 |
| **单 Host 多 Worker 控制台** | 同域名下多后端 service | 与 **services 编排** 同源 | **S14** | 见 §1 多 Worker |
| **Deploy 并发 / 内存** | 大 bundle `celld deploy` **SIGKILL** | **PD-09** `CELLP_CELLD_DEPLOY_CONCURRENCY` · 非功能缺口但阻塞验证 | 历史 S27/S28/S30 部署 | [platform-defects-log.md](./platform-defects-log.md) |

---

## 4. Worker 运行时行为 / 资源（celld · workerd）

| 能力缺口 | 表现 | 现状 | 受影响示例 | 备注 |
|----------|------|------|--------------|------|
| **长 CPU / 冷启动数据面** | 首请求拉 **blocklist trie** 等重计算 | 提高 `WORKER_TIMEOUT` 仍 **408** | **S37** Serverless DNS | Worker **能 deploy**；主路径 **超时** |
| **API-only Worker（无 Web UI）** | `GET /` 返回 JSON 401 | 运行正常，**不符合** matrix「打开即主界面」 | **S36** Triplit | **验收策略**，不是 celld 崩溃 |
| **根路径无应用** | `/` 404/空 · 仅 `/new` 等子路径 | 策略：**主界面在 /** | **S19** request-bin | 同上 |

---

## 5. 产品边界（非「补 binding 就能支持」）

| 类型 | 说明 | 示例 |
|------|------|------|
| **不做账号 / DNS / CDN / 全球边缘** | AD-10 | 与 support 矩阵独立 |
| **邮件 /  disposable-mail 类 OSS** | **不纳入** support 产品范围 | **S03、S04、S12、S13** 等 |
| **未验证 / 队列外** | 尚未跑 deploy 门禁 | **S02** ni-mail |

---

## 6. 能力 → 不支持项速查

| 若缺… | 典型不支持 / 部分 |
|--------|-------------------|
| 多 Worker `services` | S14、多 service BaaS |
| Analytics Engine | S38 |
| Next/OpenNext tier-1 | S30、S33（暂缓）、Next-on-Pages 队列 |
| Worker 内 A 类 SSR 二次 bundle | S16 |
| 长冷启动 / CPU | S37 |
| 仅 API 无 Web `/` | S36 |
| `/` 非主界面 | S19 |
| Workers AI 全链路 | A01 部分 |
| SSE 长连接 | **已通**（A03 `/event` 首字节）；A03 部分因 Workers AI |
| `IMAGES` 变换 | S31：MVP 已接；`draw`/部分选项仍缺 |

---

## 7. 维护

- 新增 **不支持** 时：在 [support-matrix.md](./support-matrix.md) 写 **原因**，并 **补一行** 到上表对应能力（或新增能力行）。
- **支持** 但 **功能降级**（如 ImgBed 尚未用满 IMAGES `draw`）：写在矩阵备注，**不要** 误标整 app 不支持。

**索引：** [support/README.md](./support/README.md) · [support-star-queue.md](./support-star-queue.md)
