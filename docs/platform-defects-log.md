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

## PD-20260903-01 — Mastra wrangler bundle：`util.promisify(undefined)` 导致 Worker 无法 instantiate（celld）

| | |
|--|--|
| **ID** | PD-20260903-01 |
| **层级** | celld / nodejs_compat |
| **严重度** | blocker（`stateless Worker failed to load` → health timeout） |
| **状态** | `fixed`（celld `4b3a3bf`：callback `gzip` / `gunzip` / `deflate` / `inflate`） |
| **项目** | A05 `support-mastra`（`mastra build` + wrangler dry-run，~19 MiB bundle，gzip ~3.5 MiB） |

### 现象

- deploy / artifact staging **成功**；version 卡在 `start celld: celld health timeout`
- `$TMPDIR/celld-support-mastra-v2.log`：

```text
Error: stateless Worker failed to load
Caused by:
    top-level rejected: TypeError [ERR_INVALID_ARG_TYPE]: The "original" argument must be of type function. Received undefined
    … at promisify … at mastra.mjs (worker.js:279855:17)
```

### 与 A05 打包策略

- 已改为保留 deployer 自带 `alias` + **wrangler `--dry-run`** → `.cellp-bundle/index.js`（非裸 `.mastra/output` 多文件）。
- 根因仍在 **bundled `mastra.mjs` 内 Node 垫片 / promisify**，非 wrangler 缺 D1/R2。

### 根因（2026-09-03 细化）

- **`node:util.promisify` 存在**；失败因 **`import { gzip } from "node:zlib"` → `undefined`**（当时 celld 仅有 `gzipSync`，没有 callback `gzip`）。
- 触发链：`@mastra/core` → `posthog-node` 模块顶层 `promisify(gzip)`。

### 修复（celld）

celld `4b3a3bf` 在 `__zlibModule` 上补齐 Node 风格 callback `gzip` / `gunzip` / `deflate` / `inflate`。压缩工作同步完成后通过 `process.nextTick`（无 `process` 时使用 microtask）调用 error-first callback，因此兼容 `util.promisify`。

### A05 修复证据

- 最终 `.cellp-bundle/index.js` **仍含** `posthog-node` 的 `promisify(gzip)`，说明 Mastra 第一阶段 bundle 已内联该依赖，后续 Wrangler alias/stub 没有参与修复。
- 使用已修复 celld 的 A05 **v14** 通过 strict `acceptance.sh`（Agent · Tool · Workflow · D1 · R2）。
- `MASTRA_TELEMETRY_DISABLED=1` 只负责关闭 demo 的运行时 telemetry；无效 alias/stub 已删除。

Mastra upstream 若将 `posthog-node` 改为完全可选依赖，可缩小 bundle，但不再是 celld 兼容性 blocker。

**证据：** celld `4b3a3bf` · `docs/evidence/support-A05.log` · `/tmp/celld-support-mastra-v2.log`

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

## PD-20260905-01 — OpenNext no-bundle `*.wasm?module` 未进入 manifest（celld）

| | |
|--|--|
| **层级** | celld / deploy module publication |
| **严重度** | blocker（App Router version 无法 ready，58 runnable 未执行） |
| **状态** | `fixed`（celld 子模块；cellp 指针与子模块发布待对齐） |
| **置信度** | 高；`deploy::no_bundle_wasm_tests`（含 `no_bundle_preserves_wasm_module_query_suffix`）与 2026-09-07 官方 App Router deploy/og 用例支持「侧车已收录」 |
| **Owner** | celld deploy/module owner |

### 证据与责任边界

- 固定 OpenNext `1.14.0` no-patch artifact 的 `index.js` 静态 import `77d9…-resvg.wasm?module` 与 `ef48…-yoga.wasm?module`，同目录还有动态使用的 `*.ttf.bin`。
- **历史根因：** 旧 no-bundle 收集用 `Path::extension() == "wasm"`，漏掉 `*.wasm?module`。**当前代码：** `collect_no_bundle_siblings` + `is_wasm_module_name` 按 import specifier 全名收录（见 `celld/crates/celld/deploy.rs`）。
- no-patch version `v-oncf-app-router-1788616744-24413` 的 OpenNext build、Wrangler dry-run 与 cellp staging 成功；celld warm isolate 报 `stateless Worker failed to load` / `instantiate: <none>`，最终 health timeout。该阶段尚未进入 binding/DO 实例或 HTTP assertion，不能写成 cache DO 失败。

### 补救与复验门禁

按 import specifier 原名发布 `*.wasm?module`，增加 no-bundle module-closure 校验以及 `.bin` data module策略；补 manifest + instantiate 回归测试。门禁：App Router version `ready`、无 module instantiate 错误、58 runnable 全部实际进入 Playwright，且不得删除官方 DO/R2/service binding。

**证据：** `docs/evidence/opennext-official-e2e-20260905-215903-47835.log` · `$TMPDIR/celld-opennext-e2e-app-router-v-oncf-app-router-1788616744-24413.log`

---

## PD-20260905-02 — OpenNext converter 将 public forwarded authority 覆盖为 synthetic Host（集成边界）

| | |
|--|--|
| **层级** | cellp Gateway → celld → OpenNext request conversion |
| **严重度** | major（Host assertion 与 Server Actions 失败） |
| **状态** | `open` |
| **置信度** | 高 |
| **Owner** | OpenNext integration owner；cellp ingress owner协同 |

### 证据与责任边界

- Gateway 测试与源码证明上游请求最初携带 public `X-Forwarded-Host`（含 `:8787`），managed celld 进程的定向环境核对也证明 `CELLD_TRUST_FORWARDED_HEADERS=1` 已实际生效；因此不是 manager 漏设变量或旧进程。
- A/B：Gateway 与 direct celld + forwarded 均让 `/api/host` 返回 synthetic URL；direct celld + public `Host` 返回正确 public URL。
- OpenNext 使用的 `@opennextjs/aws` edge converter 在 middleware handoff 创建新 `Request` 时无条件设置 `"x-forwarded-host": result.internalEvent.headers.host`。cellp 为路由使用 synthetic upstream Host，因此 public authority 被覆盖。
- 同一 no-patch celld 日志明确记录 synthetic `x-forwarded-host` 与 public `Origin` 不匹配，随后 `Invalid Server Actions request`。Next 的 CSRF/Origin 校验按设计 fail-closed；无证据指向 cache/DO。
- 同 commit、同 build 的官方 Wrangler `http://localhost` baseline 中 Host 与 Server Actions 均通过；这排除 upstream fixture/assertion 在官方 runtime 上的独立失败。

### 补救与复验门禁

适配层应保留可信 public forwarded authority，或 cellp 定义不会向应用暴露 synthetic authority 的内部 ingress 契约。门禁：`/api/host` 的 `request.url` 与 public preview URL 完全相等；Server Actions 用例通过；日志不再出现 forwarded-host/Origin mismatch。

**证据：** `docs/evidence/opennext-official-app-pages-router-v-oncf-app-pages-router-1788614744-1071.log` · `docs/evidence/opennext-official-wrangler-mixed-baseline-localhost-20260905.log` · `$TMPDIR/celld-opennext-e2e-app-pages-router-v-oncf-app-pages-router-1788614744-1071.log` · `node_modules/.pnpm/@opennextjs+aws@3.9.0/.../overrides/converters/edge.js`

---

## PD-20260905-03 — 官方 middleware 假设 HTTPS，而 dev preview 仅 HTTP（验收环境）

| | |
|--|--|
| **层级** | dev 验收环境 / 外层 ingress |
| **严重度** | major（middleware redirect 浏览器连接失败） |
| **状态** | `open` |
| **置信度** | 高 |
| **Owner** | 外层 TLS / dev acceptance environment owner；OpenNext integration owner协同修正 synthetic authority |

### 证据与责任边界

官方 middleware 对非 `localhost` Host 固定选择 `https`。A/B 均返回 307：Gateway/direct-forwarded 为 `https://synthetic...`，direct-public 为 `https://public...:8787`；本地 Gateway `:8787` 只有 HTTP，因此 Playwright 报 `ERR_CONNECTION_CLOSED`。即使先修复 synthetic Host，`*.lvh.me` 仍触发 HTTPS。同 commit、同 build 的官方 Wrangler `http://localhost` baseline 中该用例通过，符合 middleware 源码的 localhost HTTP 分支。

AD-10 明确 cellp 不负责 TLS 终止，故补救应由外层 TLS preview origin 提供；不得修改官方 middleware 或加应用特判制造 PASS。门禁：在真实 HTTPS preview 下，307 `Location` 使用 public authority并成功导航至 `/redirect-destination`。

**证据：** `docs/evidence/opennext-official-wrangler-mixed-baseline-localhost-20260905.log`

---

## PD-20260905-04 — OpenNext Pages rewrite/trailing 路径丢 query（celld 兼容路径）

| | |
|--|--|
| **层级** | celld HTTP/self-fetch 与 OpenNext routing 兼容路径 |
| **严重度** | major（3 个官方 assertion 稳定失败） |
| **状态** | `open`（根因轨道已收窄） |
| **置信度** | 高：**官方 e2e 强制 `CELLP_OPENNEXT_SKIP_PATCH=1`**，与 S30 `prepare-artifact.sh` 中针对 celld 的 Location/query/slash 补丁互斥；症状与未补丁 bundle 一致 |
| **Owner** | OpenNext integration owner（compat patch tier 或 celld 语义下沉）；celld HTTP owner 协查 |

### 证据与排除项

no-patch Pages Router 新 preview稳定复现：rewrite 页面无 `SSR`；`/rewriteWithQuery?b=2` 结果只有 `q=1`；`/ssr?happy=true` 的最终 URL 为 `/ssr/?`。Gateway、direct celld + forwarded、direct celld + public Host 三条路径结果一致；直接 `/api/query?b=2&q=1` control 三条均保留两个参数。同 commit、同 build 的官方 Wrangler localhost baseline 对 rewrite/trailing 文件 **7/7 通过**。

已排除 Gateway、celld 的**一般性** query 截断。**2026-09-07 排查：** `dev/examples/support-opennext/prepare-artifact.sh` 在 `SKIP_PATCH=1` 时跳过 `normalizeLocationHeader` / `normalizeRepeatedSlashes2` / trailing-slash 等补丁（官方套件经 `support-opennext-official/prepare-artifact.sh` 固定 skip）。待验证：Pages 在 **允许 patch** 的 lab 路径是否 3/3 绿。

### 补救与复验门禁

优先：**Pages 去 SKIP_PATCH 对照 run**；长期：补丁语义迁入 celld 或文档化 compat tier。门禁：rewrite merge query + trailing search 全绿，control 保留 query。

**2026-09-07 对照：** `run-opennext-official-e2e.sh --only pages-router --compat-patch`（S30 bundle 补丁 + stage `.next/*.json` + celld no_bundle `*.json` data module）→ **36 passed / 1 skipped**，rewrite/trailing 全绿。日志 `docs/evidence/opennext-official-e2e-20260907-143952-70936.log`。

**2026-09-07 celld 补丁下沉（`opennext_compat` + ingress `Location` / 绝对 `request.url` slash）：** 同一官方 no-patch harness → **11 passed / 25 failed**（trailing `happy=true`、rewrite merge 等仍红）。说明 S30 中 **Worker 内** `req.url = pathname + query`、`normalizeRepeatedSlashes2` 于 edge handler、rewrite 子 fetch 等仍须 bundle 补丁或更深 runtime hook；celld 边界下沉 alone 不足。逐项对照见 [NEXT-OPENNEXT-CELLP.md §补丁下沉矩阵](./plans/NEXT-OPENNEXT-CELLP.md#补丁下沉矩阵s30--celld)。日志 `docs/evidence/opennext-official-e2e-20260907-145334-74571.log` · celld `805bc50`。

**证据：** `docs/evidence/opennext-official-pages-router-v-oncf-pages-router-1788614929-19670.log` · `docs/evidence/opennext-official-wrangler-pages-baseline-20260905.log`

---

## PD-20260905-05 — Mixed fixture 的 App Router ISR flaky 与动态 JSON import 缺口相关但未定唯一根因（celld/OpenNext)

| | |
|--|--|
| **层级** | celld dynamic import / OpenNext incremental cache |
| **严重度** | major（缓存时序不稳定） |
| **状态** | `open` |
| **置信度** | 中；平台责任边界已确定，唯一根因待实验 |
| **Owner** | celld dynamic-import/cache owner；OpenNext cache owner协查 |

较早 App + Pages preview 的 ISR 首次失败后 retry 通过；no-patch 主套件首轮通过，旧失败证据必须继续保留。随后在同一 cellp preview 以 `--retries=0 --repeat-each=3` 复验：App Router ISR **1/3 失败**，Pages ISR **3/3 通过**；同 commit、同 build 的 Wrangler localhost baseline 两类 ISR **6/6 通过**。因此已排除仅由 upstream fixture 稳定性造成，问题属于 celld/OpenNext cache 兼容路径。

失败轮启动时 celld 同时出现 `dynamic import of "./.next/prerender-manifest.json" is not supported`；这强化了相关性，但仍不能证明其是唯一根因，R2/DO/cache 状态与 OpenNext revalidation 时序仍需隔离。

补救实验：在多份全新 preview 串行运行相同无 retry gate，分别补齐/禁用候选动态 import 路径做 A/B，并关联每轮 Playwright 与 celld 日志。门禁：多份 fresh preview 连续多轮两类 ISR 全绿，且相关 dynamic import 错误消失；未达到前不得改写为稳定 PASS。

**证据：** `docs/evidence/opennext-official-cellp-mixed-isr-repeat3-20260905.log` · `docs/evidence/opennext-official-wrangler-mixed-isr-repeat3-20260905.log`

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

celld 收录 sibling wasm，或 artifact 改用 A04 式 `CompiledWasm` + celld esbuild（见 [NEXT-OPENNEXT-CELLP.md](./plans/NEXT-OPENNEXT-CELLP.md) · PD-08）。

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

## PD-20260904-10 — OpenNext SSR：`process.setImmediate` 缺失导致 `GET /` 0-byte hang（celld）

| | |
|--|--|
| **层级** | celld（harness / unenv `process`） |
| **严重度** | major（OpenNext preview `GET /` 无响应直至客户端超时；`/_next/static` 可 200） |
| **状态** | `fixed`（lab 二进制；需 kill/wake 或重 deploy 换进程） |

### 现象

- **项目：** S30 `support-opennext`（`@opennextjs/cloudflare` 单 Worker bundle）。
- **路径：** preview v38–v42 `GET /` → **0 byte**、curl 28；探针 `hasProcessSetImmediate: undefined`，卡在 Next `requestHandler` / `waitTillReady` 之后。
- **对照：** prod v22 **400** proto-rel `//`（快失败，**另一 bug**）。

### 根因

全局 `setImmediate` 已由 `set_immediate.js` 提供；OpenNext/unenv 用 **独立 `process` 对象**（`import process` / `globalThis.process = process`），**不含** `process.setImmediate`。Next 16 调度走 `process.setImmediate`，回调不跑，HTTP 响应永不 settle。

### 修复（celld）

1. `harness.js`：`__celldPatchProcessTimers` + `globalThis.process` setter。
2. `patch_process_timers`：worker 模块 evaluate 后、每次 `start_fetch` 前再 patch。
3. 单测：`harness_patches_process_set_immediate`。

实录：[S30-OPENNEXT-HARD-PROBLEM.md](./plans/S30-OPENNEXT-HARD-PROBLEM.md) §4.2。

---

## 变更 log

| 日期 | 变更 |
|------|------|
| 2026-09-04 | **PD-20260903-01 fixed**：celld `4b3a3bf` 补 callback zlib API；A05 `v13` 在最终 bundle 仍含 `promisify(gzip)` 时可正常 instantiate；删除无效 alias/stub |
| 2026-09-04 | **PD-20260904-10 fixed**：`process.setImmediate` on unenv `process`；S30 hang **可 settle**（prod v22 仍 400 proto-rel） |
| 2026-09-03 | PD-10 **垫片落地、S30 未过**：loopback 重入 + `CelldHttpBodyStream` BYOB；v42 `GET /` 仍 0-byte hang。实录 [S30-OPENNEXT-HARD-PROBLEM.md](./plans/S30-OPENNEXT-HARD-PROBLEM.md) |
| 2026-09-03 | PD-09 **fixed**：`CELLP_CELLD_DEPLOY_CONCURRENCY` + deploy slot/retry；见 `cellp/internal/runtime/deploy_limit.go` |
| 2026-09-03 | PD-06 **fixed**：`node:timers` `setImmediate`；S25 `GET /` 200 HTML（~11ms）；证据见 user-acceptance 复验 |
| 2026-09-01 | PD-06：根因分析见 [NITRO-CELLD-COMPAT.md](./plans/NITRO-CELLD-COMPAT.md)（修正 localFetch 机制描述） |
| 2026-09-02 | PD-05 fixed：`_routes.json` + 尾斜杠；S22 全路径 200（见 integration-verify-astro-s22-routes.md） |
| 2026-09-02 | celld：`ensure_external_stub` + `caches`；`check-s3-clock-skew.sh`；S22 v8 全路径验收 |
| 2026-09-02 | 初版：S22 Astro 暴露 PD-01～04 |
| 2026-09-01 | PD-01/02 fixed：celld `ensure_external_stub` + harness `caches`；`scan_external_imports` 副作用 import |
