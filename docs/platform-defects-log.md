# cellp / celld 平台缺陷日志

> **用途：** 社区 support、框架验证（S22+）中暴露的**运行时 / 控制面缺口**，供排期修 celld 或 cellp，**不是**应用侧长期 patch 清单。  
> **证据目录：** `docs/evidence/`（`support-*.log`、celld stderr 见 macOS `TMPDIR/celld-{project}-{version}.log`）  
> **关联：** [support-validation-lessons.md](./support-validation-lessons.md) · [framework-coverage-cellp.md](./framework-coverage-cellp.md)

---

## 记录格式

| 字段 | 说明 |
|------|------|
| **ID** | `PD-YYYYMMDD-NN` |
| **层级** | `celld` / `cellp` / `dev 栈` |
| **严重度** | `blocker`（无法 ready）/ `major`（主路径 5xx）/ `minor` |
| **状态** | `open` / `mitigated`（overlay 绕过）/ `fixed` |
| **修复证据** | celld commit / `cargo test -p celld`（可选 `docs/evidence/`） |

---

## PD-20260902-01 — `cloudflare:workers` 多模块 ESM 未注册 stub（celld）

| | |
|--|--|
| **层级** | celld |
| **严重度** | blocker（Worker `instantiate` 失败 → health timeout） |
| **状态** | `fixed`（`resolve_external` → `ensure_external_stub`；`scan_external_imports` 副作用 import） |

### `cloudflare:workers` 是什么？

Cloudflare / workerd 的 **内置虚拟模块**（非 npm 包），由运行时注入，典型导出包括：

- `DurableObject`、`WorkerEntrypoint`、`RpcTarget`
- `waitUntil`、`env`、`exports`（与 DO / 多导出 Worker 相关）

在 **celld** 中，语义由 `harness.js` 里的 `globalThis.__cf` 实现；`modules.rs` 通过 **预编译 stub 模块** 把 `import { … } from "cloudflare:workers"` 接到 `__cf`（见 `builtin_source` / `stub_source`）。

这与 **npm polyfill** 不同：正确做法是 **celld 模块解析器**在加载 Worker 图之前注册 stub，而不是在应用里 `npm install` 某个包。

### 现象

- **项目：** S22 Astro（`@astrojs/cloudflare`），`dist/_worker.js/chunks/*.mjs` 含 `import 'cloudflare:workers'`（副作用 import）。
- **日志：** `resolve: no stub for specifier spec=cloudflare:workers` → `Error: stateless Worker failed to load`（`/tmp/celld-support-astro-v3.log` 等）。
- **对比：** 同目录 `celld deploy` 单文件 bundle 可成功上传；**fleet 启动**仍可能走多文件 / 未扫描到 sibling import 的路径。

### 根因（推断）

`register_stubs` 仅根据 **主模块** `config.main_imports` 注册；`register_loader_modules` 应为 sibling 补 stub，但 Astro 产物图或 deploy 落盘形态下，**副作用 import 出现在 chunk 中且未在注册前被解析**时，`resolve_external` 只打 warn、**不按需生成** `cloudflare:workers` stub（`modules.rs` 注释假定「celld bundles are single-file」）。

### 应有修复（celld，非应用 patch）

1. **`resolve_external` 回退：** 对 `cloudflare:workers` / `cloudflare:workflows` / 已知 `node:*`，若 registry 无条目，**同步** `full_surface_source` + `compile_module` 并缓存（与 `register_loader_modules` 逻辑复用）。
2. **Deploy 产物：** 保证 `celld deploy` 后 S3 manifest 的入口与运行时加载路径一致（单 bundle vs 多模块）并覆盖全图 import 扫描。
3. **测试：** 增加 fixture：`import 'cloudflare:workers'` 仅出现在 **非 main** chunk 的多文件 Worker。

### 临时缓解（已废弃）

~~`prepare-artifact.sh` strip `cloudflare:workers`~~ — **已删除**；依赖 celld `ensure_external_stub` + 副作用 import 扫描（见 `cargo test -p celld`）。

---

## PD-20260902-02 — 全局 `caches` API 缺失（celld）

| | |
|--|--|
| **层级** | celld |
| **严重度** | major（SSR 路由 500，静态页可 200） |
| **状态** | `fixed`（`harness.js` `globalThis.caches`） |

### 现象

- **项目：** S22 Astro v8 prod Host。
- **路径：** `/` → 200；`/blog/`、`/about/` → **500**。
- **响应：** `ReferenceError: caches is not defined`（Astro SSR / adapter 使用 Cache API）。

### 与 CF 的差距

Workers 运行时提供 **Cache Storage**（`caches.default` 等）。celld 若未在 harness 暴露等价全局，**框架 SSR**（Astro、部分 SvelteKit）会在动态路由失败。

### 应有修复（celld）

- 在 isolate 启动或 `__cf` 初始化时提供 **workerd 对齐的 `caches` 全局**（至少 default cache：match/put/delete/keys 的 dev 实现或 no-op + 内存后端）。
- 在 `celld/docs/cloudflare-compat.md` 标明 **Partial/No** 并链到本缺陷。

### 临时缓解

prepare 在 `dist/_worker.js/index.js` 头部注入最小 `globalThis.caches` 对象（与 S14 cfbase 同类手法）。

---

## PD-20260902-03 — Astro 部署需 slim artifact + 专用 stage（cellp dev 脚本）

| | |
|--|--|
| **层级** | cellp（`deploy-support-app.sh`） |
| **严重度** | minor（无 overlay 时脚本挂死 / 上传巨包） |
| **状态** | fixed（astro slim 分支 + `support-astro/*`） |

### 现象

- 未走 `prepare-artifact` 时全量 `rsync node_modules`，单次 deploy **>5min 无输出**（用户感知「shell 卡住」）。
- `SUPPORT_RSYNC_NO_NODE` 仅在存在 `CELLP_PREPARE` 时由 deploy 脚本设置；prepare 子进程内 `export` **不会**回传父 shell（已靠父脚本 `SUPPORT_RSYNC_NO_NODE=1` 修复）。

### 修复

- `elif` 分支：`dist/_worker.js/index.js` + wrangler → 只 stage worker 树与 `.cellp-assets`。

---

## PD-20260902-04 — RustFS 时钟漂移导致 celld 自毁（dev 栈）

| | |
|--|--|
| **层级** | dev 栈 / 环境 |
| **严重度** | major（ready 后 Gateway **502**） |
| **状态** | open |

### 现象

`RequestTimeTooSkewed`（S3 PUT）→ `node_lease_watchdog_fence` → celld 进程退出；`support-astro` v8 复测 502。

### 缓解

校准 macOS 系统时间 / NTP；**`./dev/scripts/fix-rustfs-skew.sh`**（重启 RustFS + cellpd）；再 deploy 或 `POST …/wake`。

---

## Polyfill 策略总结（给产品 / 实现）

| 能力 | 能否用 npm polyfill？ | cellp 推荐 |
|------|----------------------|------------|
| `cloudflare:workers` | **否**（虚拟模块） | celld `resolve_external` 按需 stub → `globalThis.__cf` |
| `caches` | 可在入口 **注入** `globalThis.caches` | celld harness 一等实现 |
| `node:crypto` / Web Crypto | 部分可；PBKDF2 等已在 celld 补 | 运行时对齐 CF（已做一轮） |
| `[[services]]` 多 Worker | **否** | cellp 编排（当前 **不支持**，见 MULTI-WORKER-DEPLOY） |

**原则：** 一等框架（AD-13）不应依赖 `dev/examples/*/prepare-artifact.sh` 长期 strip；缺陷应 **关闭 PD 条目** 并删 overlay。

---

## PD-20260902-05 — Astro 静态路由 `_routes.json` + 尾斜杠（celld assets）

| | |
|--|--|
| **层级** | celld（assets 路由）/ wrangler 配置 |
| **严重度** | major（`/blog/`、`/about/` **404**；`/` **200**） |
| **状态** | fixed |

### 现象

S22 v8：`.cellp-assets` 含 `blog/index.html`、`_routes.json`（`exclude` 含 `/blog/*`），但 prod Host 下 `/blog/`、`/about/` 仍 404；已试 `html_handling: auto-trailing-slash`。

### 修复

- celld：`decode_path` 接受合法尾斜杠；deploy 解析 `_routes.json` → `AssetConfig.routes` + `run_worker_first` 编译；ingress `exclude` 路径资产独占、miss 不回落 Worker。
- 证据：`docs/evidence/integration-verify-astro-s22-routes.md`

---

## PD-20260902-06 — Nitro `localFetch` SSR 挂起（celld）

| | |
|--|--|
| **层级** | celld（`fetch` / 子请求 / `waitUntil`） |
| **严重度** | major（SSR `/` 无响应直至客户端超时；静态经 `ASSETS` 可 200） |
| **状态** | `fixed` |

### 现象

- **项目：** S25 Nuxt（Nitro `cloudflare_module`，`dev/examples/support-nuxt/` wrangler slim bundle）。
- **路径：** `GET /` → 网关/celld **挂起**（smoke 曾见 500；curl `-m 8` → 超时）；`GET /robots.txt`、`/favicon.ico`、`/_nuxt/*` → **200**。
- **产物：** `.cellp-bundle/index.mjs` 内 SSR 走 `useNitroApp().localFetch(pathname+search, …)`（非仅 `ASSETS.fetch` 白名单路径）。

### 根因

**已钉死（Phase 2）：** Nitro `h3.send()` 把 `res.end` 排到 `setImmediate`（bundle `Xt2 = setImmediate`）。S25 从 `node:timers` 具名导入 `setImmediate` / `clearImmediate`；celld 原先把 `node:timers` 接到 **`__nodeStub`**（可调用、**永不执行回调**），`await e6(r6,s2)` / `localFetch` 永不 settle。`op_fetch` 不在热路径上（Phase 0 E1=0）。修复：`node:timers` 懒模块导出真实 `setImmediate`（`setTimeout(cb, 0)`）。详见 **[NITRO-CELLD-COMPAT.md](./plans/NITRO-CELLD-COMPAT.md)**。

### 应有修复（celld，非应用 patch）

1. `op_fetch` / harness `fetch`：同源 Worker URL → `__cell.selfFetch`（对齐 service binding 同源路径）。
2. **测试：** 最小 Nitro `cloudflare_module` bundle（或 S25 裁剪），`GET /` 返回 HTML。

### 缓解

- **勿**在 `prepare-artifact.sh` 长期 polyfill `cloudflare:workers` / `caches`。
- 仅静态验收可依赖 `routeRules` 预渲染 `/`（**不**视为 S25 全栈通过）。

---

## PD-20260903-07 — Gateway → DO WebSocket 升级 502（fx-on-workers 等）

| | |
|--|--|
| **层级** | cellp Gateway / celld（DO `fetch` + `Upgrade: websocket`） |
| **严重度** | major（浏览器 TUI / 长会话 agent 主路径不可用） |
| **状态** | **`fixed`**（WS-M2：2026-09-03 verification — Gateway `GET /session` Upgrade → **101**；证据 `docs/evidence/ws-ingress-verify.log`、`websocket-ingress-h1h2.md`） |

### 现象

- `support-fx-on-workers`：`GET /?key=` **200**（静态页）；`GET /session` + WebSocket Upgrade → **502** `bad gateway`。
- `FxSession` 非 WS 请求返回 **426** `expected websocket`（Worker 路由正常）。

### 影响

- 依赖 **WebSocket + Durable Object** 的 agent（fx TUI、部分 CF Agents 实时通道）在本地 dev 栈无法做浏览器级验收。
- **不**影响纯 HTTP agent（如 Pi `POST /`、opencode-do `POST /session/.../message`）。

### 缓解

- fx：**HTTP `/api/prompt`**（cellp overlay，不替代上游 WS 设计）。
- 文档：`dev/README.md`、`dev/examples/support-fx-on-workers/README.md`。

### 应有修复

- Gateway 将 WebSocket 升级正确代理到 celld / workerd DO；回归：`curl -i` Upgrade 期望 **101**。

---

## PD-20260903-08 — `no_bundle` 预构建 Worker 丢失 sibling wasm（celld）

| | |
|--|--|
| **层级** | celld（`deploy.rs` · `no_bundle`） |
| **严重度** | major（OpenNext 等 wrangler 产物含 wasm 时 version 无法 `ready`） |
| **状态** | `fixed`（celld `no_bundle` 收录入口目录 sibling `*.wasm`；S30 **v5** `ready`，但 prod `GET /` 仍为 **308** `Location: ?`，矩阵仍 **不支持**） |

### 现象

- **S30** OpenNext：`celld health timeout`；`$TMPDIR/celld-support-opennext-*.log` → `stateless Worker failed to load` / `instantiate: <none>`。
- Artifact 含 `.cellp-bundle/*-yoga.wasm`、`*-resvg.wasm`，`wrangler.jsonc` 设 `no_bundle: true`。

### 根因

`no_bundle` 路径只上传 `main` JS，`wasm: Vec::new()`。与 A04 注释「celld no_bundle drops wasm」一致。

### 应有修复

celld 收录 sibling wasm，或 artifact 改用 A04 式 `CompiledWasm` + celld esbuild（见 **[NEXT-OPENNEXT-S30-ROOT-CAUSE.md](./plans/NEXT-OPENNEXT-S30-ROOT-CAUSE.md)**）。

---

## PD-20260903-09 — 多 celld 常驻 + 并发 deploy → OOM SIGKILL（cellp）

| | |
|--|--|
| **层级** | cellp（`runtime/manager.go` · dev AD-1 fleet） |
| **严重度** | blocker（version `failed`: `celld deploy: signal: killed`） |
| **状态** | `fixed`（`CELLP_CELLD_DEPLOY_CONCURRENCY` 默认 1 + deploy 槽位；SIGKILL 一次重试） |

### 现象

- **S27** SolidStart：artifact ~324 KiB `no_bundle` OK；orch 子进程 **`celld deploy: signal: killed`**（无 stderr），`docs/evidence/support-S27-20260903-deploy.log`。
- **S29** 等同日 batch 多次 `signal: killed`；**v9** 在内存压力降低后 deploy **ready**（`docs/evidence/support-S29.log`）。
- **对比：** artifact 目录手工 `celld deploy --bucket s3://cellp-celld/...` **可成功**（无并发 deploy、无额外 orch 峰值）。

### 根因

AD-1 为每个 ready route 常驻 **celld**（dev 可达 ~28 进程）。Orchestrator / promote reconcile 再 fork **`celld deploy`**（esbuild 读盘 + S3 上传）时，与 `context.WithoutCancel` 无关，**macOS 内存压力**对 deploy 子进程发 **SIGKILL**；`exec` 表现为 `signal: killed`、CombinedOutput 为空。

### 修复 / 缓解

1. **cellp：** `withCelldDeploySlot` 限制并发 deploy（`CELLP_CELLD_DEPLOY_CONCURRENCY`，默认 **1**）；`signal: killed` **重试 1 次**（间隔 3s）。
2. **运维：** 批量 support 验证前 `./dev/scripts/health.sh`；仍 OOM 时 archive 非必要 ready preview 或增大主机内存；大 bundle 用 `prepare-artifact` wrangler dry-run + `no_bundle`（见 S27/S28）。

---

## 变更 log

| 日期 | 变更 |
|------|------|
| 2026-09-03 | PD-09 **fixed**：`CELLP_CELLD_DEPLOY_CONCURRENCY` + deploy slot/retry；见 `cellp/internal/runtime/deploy_limit.go` |
| 2026-09-03 | PD-06 **fixed**：`node:timers` `setImmediate`；S25 `GET /` 200 HTML（~11ms）；证据见 user-acceptance 复验 |
| 2026-09-01 | PD-06：根因分析见 [NITRO-CELLD-COMPAT.md](./plans/NITRO-CELLD-COMPAT.md)（修正 localFetch 机制描述） |
| 2026-09-02 | PD-05 fixed：`_routes.json` + 尾斜杠；S22 全路径 200（见 integration-verify-astro-s22-routes.md） |
| 2026-09-02 | celld：`ensure_external_stub` + `caches`；`check-s3-clock-skew.sh`；S22 v8 全路径验收 |
| 2026-09-02 | 初版：S22 Astro 暴露 PD-01～04 |
| 2026-09-01 | PD-01/02 fixed：celld `ensure_external_stub` + harness `caches`；`scan_external_imports` 副作用 import |
