# Next.js OpenNext S30 — 根因分析

> **关联：** [NEXT-OPENNEXT-CELLP.md](./NEXT-OPENNEXT-CELLP.md) · **PD-20260903-08** · `docs/evidence/support-S30.log` · support-matrix S30

## 摘要

| 项 | 结论 |
|----|------|
| wrangler / artifact 体积 | dry-run **~8.6 MiB** 可产出；`celld deploy` **可完成上传**（~7.2 MiB 模块 + 静态资产） |
| celld 进程能否起来 | **不能** — `Generation::build` 阶段 **V8 instantiate 失败**，进程在监听前退出 |
| cellpd `celld health timeout` | 表象：60×1s 探针 `/.well-known/celld/health` 无 200；根因是 **Worker 未加载**，非「8.6 MiB 硬限制」 |
| 与 support-nuxt 对比 | Nuxt **~491 KiB**、无 wasm 侧车 → **health 200 @ ~2s** |

---

## 背景（S30）

OpenNext PoC 路径见 [NEXT-OPENNEXT-CELLP.md §3](./NEXT-OPENNEXT-CELLP.md)。`POST /versions` 在 `start celld` 阶段失败（v1/v2 均 `celld health timeout`）。

---

## celld 加载

### 1. Artifact 实测（`support-opennext` 最新 version **v2**）

路径：`dev/data/artifacts/support-opennext/v2/`

| 文件 / 目录 | 大小 | 说明 |
|-------------|------|------|
| `.cellp-bundle/index.js` | **7 363 437 B**（≈ **7.02 MiB**） | wrangler `main`；与 `worker.js` 同字节 |
| `.cellp-bundle/`（含 wasm + map） | **28 M** `du -sh` | 含 `77d9fae…-resvg.wasm`（~1.3 MiB）、`ef4866ec…-yoga.wasm`（~87 KiB）、`worker.js.map`（~9.2 MiB，**不**进 deploy） |
| 整包 `v2/` | **29 M** | 含 `.cellp-assets` |
| wrangler dry-run（证据 log） | **8623.49 KiB**（≈ **8.43 MiB**） | 与 README/矩阵写的 **~8.6 MiB** 一致 |

`wrangler.jsonc`（v2）：

```jsonc
{
  "name": "support-opennext",
  "main": ".cellp-bundle/index.js",
  "compatibility_date": "2025-10-08",
  "compatibility_flags": ["nodejs_compat", "global_fetch_strictly_public"],
  "assets": { "binding": "ASSETS", "directory": ".cellp-assets" },
  "no_bundle": true
}
```

`index.js` 顶层 **静态 import** 两个 wasm 侧车（wrangler dry-run 落在 `.cellp-bundle/`）：

```text
import resvg_wasm from "./77d9faebf7af9e421806970ce10a58e9d83116d7-resvg.wasm";
import yoga_wasm from "./ef4866ecae192fd87727067cf2c0c0cf9fb8b020-yoga.wasm";
```

### 2. celld 源码：health / startup / no_bundle / 体积与超时

#### 2.1 cellpd 如何起 celld（与 `runtime.Manager` 一致）

`cellp/internal/runtime/manager.go`：

- **Deploy：** `celld deploy <bundleDir> --bucket s3://cellp-celld/<project>/<version> --endpoint … --region …`
- **Start：** `celld --bucket … --endpoint … --region … --listen 127.0.0.1:<port>`
- **环境：** `CELLD_WATCH`（临时目录）、`CELLD_READY_FLEET_GATE_MS`（per-version 默认 **5000**，覆盖 celld 默认 120s fleet gate）、`AWS_*`、`CELLD_VAR_*` / `CELLD_VARS_FILE`
- **Health：** GET `http://<host>:<port>/.well-known/celld/health`，**60 次 × 1s** 未 200 → `celld health timeout`

#### 2.2 `/.well-known/celld/health` 何时 200

`celld/crates/celld/main.rs`（`handle_public`）：

- 200 需同时：`!draining` && `app.fleet_ready()` && `app.healthy().await`
- `healthy()` → actor `ready_to_serve()`（`celld/crates/logic/lib.rs`）：**节点 lease 权威**且未 fenced/resuming
- `fleet_ready`：单节点 AD-1 在 `CELLD_READY_FLEET_GATE_MS` 到期后会 **fall open**（`ready_gate_expired`）；**不是** S30 的主阻塞点

#### 2.3 Startup：何时编译 Worker

- 指针 `deploy/current.json` 触发 `adopt_deployment` → `Generation::build`（`spawn_blocking`）
- `runtime.rs`：`Generation::build` **编译并 warm 每个 script 的 stateless pool**；失败则 **reload 失败**，且 broken 版本会被记住避免轮询重试
- `StatelessRuntime::start` → `isolates.warm()`；失败错误串：**`stateless Worker failed to load`**

#### 2.4 `no_bundle`

`celld/crates/celld/deploy.rs`：

- `no_bundle: true` 时 **只 `read` 入口 JS**，**不跑 esbuild**，且注释明确：**预打包入口不带 sibling wasm**（`wasm: Vec::new()`）
- esbuild 路径才会把 bundle 引用的 wasm **复制为并列 module**

因此：OpenNext 用 wrangler dry-run 产出 **JS + wasm 文件**，但 cellp 标准 **`no_bundle` + `celld deploy`** 只上传 **单个 `index.js` module** → 运行时 `register_wasm_modules` 无对应模块 → **`instantiate_module` 失败**。

#### 2.5 「大文件 / isolate 超时」

在 celld 0.4.0 submodule 内 **未找到**「Worker 脚本最大 MiB」或「compile  wall-clock 超时」类硬编码。

相关但 **非本次直接命中** 的限制：

| 机制 | 位置 | 说明 |
|------|------|------|
| V8 heap / isolate | `CELLD_V8_HEAP_LIMIT_MB`（默认 **128 MB**） | README；影响 heap/WebSocket/SQL 物化，**非**脚本字节上限 |
| 请求体 | `CELLD_MAX_REQUEST_BODY_BYTES` | `docs/security.md` |
| RSS / resident cells | `CELLD_MAX_RSS_MB`、`CELLD_MAX_RESIDENT_CELLS` | 节点级 |
| cellpd health 等待 | `manager.go` StartOnPort | **60s** 固定，与 celld 内部无单独「加载超时」配置 |

**结论：** S30 失败 **不能** 归因于「8.6 MiB 超过 celld 上传上限」；`celld deploy` 在本地 **0.01s** 读完 bundle 并 **0.13s** 上传成功。

### 3. 与 `support-nuxt` 体积对比

| 项目 | `du -sh` 整 version | `.cellp-bundle/` | `index.js` | `celld deploy` Total Upload |
|------|---------------------|------------------|------------|-----------------------------|
| **support-opennext v2** | 29 M | 28 M | 7.02 MiB | **7190.86 KiB** |
| **support-nuxt v1** | 1.4 M | 1.1 M | ~491 KiB | **490.99 KiB** |

Nuxt artifact 同样 `no_bundle: true`，但 **无 `.wasm` import** → celld 单模块即可 instantiate。

### 4. 本地复现（与 cellpd 同型 celld 命令）

环境：`source dev/.env`（RustFS `S3_ENDPOINT`、密钥）。**未**改生产 bucket；使用独立测试 bucket。

```bash
# 1) deploy（与 orchestrator 相同参数形状）
celld deploy dev/data/artifacts/support-opennext/v2 \
  --bucket s3://cellp-celld/support-opennext/s30-manual-test \
  --endpoint "$S3_ENDPOINT" --region "$AWS_REGION"
# → Total Upload: 7190.86 KiB，成功

# 2) start（对齐 cellpd：CELLD_READY_FLEET_GATE_MS=5000 + CELLD_WATCH + AWS_*）
celld --bucket s3://cellp-celld/support-opennext/s30-manual-test \
  --endpoint "$S3_ENDPOINT" --region "$AWS_REGION" \
  --listen 127.0.0.1:18992
```

**结果（2026-09-03 本地）：**

- 进程 **未** 进入 `celld listening on …`（90s 内 curl health 均为连接失败）
- **stderr 全文仅：**

```text
2026-09-03T01:18:43.483213Z  WARN celld::memory: the allocator will not run a background thread ...
Error: stateless Worker failed to load

Caused by:
    instantiate: <none>
```

对照：**support-nuxt v1** 同流程 **~2s** 获得 `/.well-known/celld/health` **200**，日志含 `isolate started`、`ready_gate_open`。

### 5. § celld 加载 — 判定

| 问题 | 答案 |
|------|------|
| **8.6 MiB worker 在 celld 上能否「加载」？** | **当前 pipeline 下不能。** 体积本身未触发 deploy 拒绝；**加载阶段**在 warm/instantiate 失败并 **进程退出**。 |
| **与 S30 `health timeout` 关系** | cellpd 探针等的是已监听且 health 200 的 celld；OpenNext 进程在 listen 前即失败 → **必然 timeout**。 |
| **首要技术缺口** | **`no_bundle` 与 OpenNext/wrangler 输出的 wasm 侧车不兼容**（`deploy.rs` 不上传 wasm modules；`index.js` 又硬依赖 wasm import）。次要风险：7 MiB 单文件 compile/warm **耗时**（未在本次失败中暴露，因 instantiate 先失败）。 |
| **建议验证方向（文档 only，未改代码）** | ① celld 扩展 `no_bundle` 上传 `.cellp-bundle/*.wasm`；或 ② 构建侧去掉 OG/image wasm 路径；或 ③ 允许一次 esbuild 打包 wasm（需测 OpenNext 图是否可二次 bundle）。 |

---

## 待补章节

- [ ] Gateway / `GET /` 行为（celld 起来之后）
- [ ] `nodejs_compat` / OpenNext API 缺口矩阵
