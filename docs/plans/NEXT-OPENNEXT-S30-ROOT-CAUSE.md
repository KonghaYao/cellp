# S30 OpenNext 不支持 — 根因分析

> **状态：** 实验路径 · support-matrix **不支持**  
> **证据：** `docs/evidence/support-S30.log`、`docs/evidence/support-S30-opennext-20260903.md`（本地）  
> **日期：** 2026-09-03  
> **关联计划：** [NEXT-OPENNEXT-CELLP.md](./NEXT-OPENNEXT-CELLP.md) · **PD-20260903-08**（见 [platform-defects-log.md](../platform-defects-log.md)）

---

## 1. 现象时间线

| 时间 (UTC+8) | 事件 |
|--------------|------|
| 2026-09-03 09:07 | `deploy-support-app.sh S30` v1：wrangler dry-run **8623.49 KiB** 通过 |
| 09:07:29 | `POST /versions` v1 创建 |
| 09:08:31 | v1 `failed`：`start celld: celld health timeout on 127.0.0.1:8827`（~62s） |
| 09:09 | v2 部署，poll timeout **300s** |
| 09:10:05 | v2 `failed`：`celld health timeout on 127.0.0.1:8830` |
| 验收 | `Host: support-opennext.lvh.me` → **404**（无 `ready` version / 未 promote） |

---

## 2. cellp health probe（表象）

| 项 | 行为 |
|----|------|
| **URL** | `GET http://{host}:{port}/.well-known/celld/health` |
| **成功** | HTTP **200** |
| **Start 等待** | `StartOnPort`：最多 **60×1s**（`cellp/internal/runtime/manager.go`） |
| **日志** | `$TMPDIR/celld-{project}-{version}.log`（`dev/scripts/logs.sh` **不含** 此文件） |

版本 `failed` 后 gateway prod Host 无路由 → **404**。这是 **后果**，不是独立根因。

---

## 3. 根因（决定性）

**主因：`no_bundle: true` + OpenNext wrangler 产物含 sibling `.wasm`，celld deploy 的 `no_bundle` 路径只上传 `main` 的 `index.js`，丢弃 wasm 模块。**

| 类别 | 是否主因 | 说明 |
|------|----------|------|
| **celld `no_bundle` + wasm** | **是** | `deploy.rs`：`no_bundle` 时 `wasm: Vec::new()`，注释写明 pre-bundled entry **不携带 sibling wasm** |
| **artifact / prepare** | 合格 | `.cellp-bundle/index.js` + `*-yoga.wasm` + `*-resvg.wasm` 均在 artifact 目录 |
| **health 60s 过短** | 否（本次） | Worker **未实例化**；延长 poll 不能单独修复 |
| **8.6 MiB 编译慢 / OOM** | 未观察到 | stderr 为即时 `stateless Worker failed to load` |
| **路由未 promote** | 后果 | version 从未 `ready` |

### 3.1 celld 日志（典型）

```
Error: stateless Worker failed to load
Caused by:
    instantiate: <none>
```

对应 `StatelessRuntime::warm()` / Worker 模块图缺少 wasm。

### 3.2 Artifact 证据

`dev/data/artifacts/support-opennext/v1/`：

- `wrangler.jsonc`：`main: .cellp-bundle/index.js`，`no_bundle: true`
- `.cellp-bundle/`：`index.js`（~7.0 MiB）+ `ef4866ec…-yoga.wasm` + `77d9faeb…-resvg.wasm`
- `index.js` 含 `import … from "./…-resvg.wasm"` 等 ESM 引用

### 3.3 对比成功槽位

| 槽位 | wrangler 体积 | `no_bundle` | bundle 内 `.wasm` | 结果 |
|------|---------------|-------------|-------------------|------|
| **S25** Nuxt | ~491 KiB | true | **无** | ready · prod 200 |
| **S26** Hono | ~62 KiB | true | **无** | ready · prod 200 |
| **S30** OpenNext | ~8623 KiB | true | **有** | health timeout |

**已有规避范例：** [A04 fx-on-workers](../../dev/examples/support-fx-on-workers/prepare-artifact.sh) — **不用** `no_bundle`，`CompiledWasm` rules + celld esbuild，注释写明「celld no_bundle drops wasm」。

---

## 4. 与「Next 不支持」产品表述的关系

| 层面 | 结论 |
|------|------|
| **AD-13 tier-1** | Next/OpenNext **非**一等公民；S30 为实验槽位 |
| **本次 S30** | 失败在 **celld 部署语义**，非「Next 不能在 Workers 上跑」的笼统结论 |
| **修复后** | 仍可能有 OpenNext 运行时差异（`global_fetch_strictly_public`、SSR 路由等），需单独 prod 验收 |

---

## 5. 可行动项

| 负责方 | 动作 | 优先级 |
|--------|------|--------|
| **celld** | `no_bundle` 时收录 `main` 同目录（或 wrangler `rules`）内 **全部 `.wasm`** 进 manifest | **P0** |
| **artifact** | 或仿 **A04**：去掉 `no_bundle`，`CompiledWasm` + celld 再 bundle（需验证 8.6 MiB 图） | P1 |
| **cellp** | `Start` 失败时把 `$TMPDIR/celld-*.log` 尾部写入 version `error` | P2 |
| **cellp** | 可选：大 bundle 延长 Start health 轮询 | P3（**不能**单独解决 S30） |
| **验收** | wasm 修复后复跑 S30 → prod `GET /` **200** + HTML | P1 |

### 本地复现

```bash
# per-version 日志（决定性）
tail -50 "$TMPDIR/celld-support-opennext-v1.log"

# 存储探针
celld diagnose --bucket s3://cellp-celld/support-opennext/v2/ --endpoint http://127.0.0.1:19000 …
```

---

## 6. 结论（一句话）

S30 **artifact 流水线合格**；失败因 **celld 以 `no_bundle` 部署 OpenNext 预构建 Worker 时丢失 wasm**，导致 **instantiate 失败** → cellp 报 **celld health timeout** → prod **404**。**不得**标为已支持，直至 wasm 收录或改用 A04 式 bundle 策略并通过 prod 验收。
