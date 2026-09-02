# Nitro Loopback 设计 — workerd Parity / 运行时怀疑论审查

> **审查角色：** CF workerd parity / runtime skeptic  
> **现行设计：** [NITRO-CELLD-LOOPBACK-DESIGN.md](./NITRO-CELLD-LOOPBACK-DESIGN.md) **v0.2**（已并入本审查：E1/E7 门禁、isolate selfFetch、SC-5a）  
> **审查对象：** [NITRO-CELLD-LOOPBACK-DESIGN.md](./NITRO-CELLD-LOOPBACK-DESIGN.md) v0.1 · [NITRO-CELLD-COMPAT.md](./NITRO-CELLD-COMPAT.md)  
> **结论先行：** Option A（`op_fetch` 同源 loopback）是 **workerd 对齐所必需**，但对 **S25 `GET /` 挂起很可能不够**；在投入完整 loopback 实现前，应先用廉价实验 **证伪 H1**，否则有较高概率「loopback 合入后 Nuxt 仍挂」。

---

## 1. 审查问题

| 问题 | 立场 |
|------|------|
| Option A 能否修复 S25？ | **条件性否**：仅当挂起主因是 SSR 链上的 **`globalThis.fetch` → `op_fetch` → ingress 再入** 时成立 |
| H2（`localFetch` / h3 `b2` / microtask·ALS）是否才是真 blocker？ | **证据倾向是**：compat 已证明 **主路径不经 `op_fetch`**；loopback 与 S25 症状 **解耦** |

---

## 2. 与 workerd `fetch` 语义的缺口

以下按「Option A 若按设计落地，仍与 workerd/Miniflare 心智模型不一致或未定」列出。

### 2.1 子请求调度模型

| 维度 | workerd / CF（典型） | celld 今日 | Option A 草案 | 缺口 |
|------|----------------------|------------|---------------|------|
| Worker 对自身 origin 的 `fetch` | 运行时 **内部 dispatch**，不占公网连接、不依赖第二 listener 占满池 | `globalThis.fetch` → **`op_fetch` → reqwest** | 同源 → `selfFetch` / loopback | 方向正确 |
| 同源 fast path 所在层 | 多在 **同一 isolate / 同一 JS 堆** 内重入 handler | Service binding 已有 **isolate 内** `__cell.selfFetch`（`harness.js` ~2236） | §6 写「`runtime.rs` loopback 走 **`fetch_worker` 同池路径**」 | **严重不一致**：若 loopback 走 **池线程 + 第二 isolate 租约**，语义上接近「再入 ingress」，**H1 类死锁可能从 HTTP 形态变成池形态**，与 harness 已有 selfFetch 语义分裂 |
| `selfFetch` 注册 | N/A（运行时内置） | **bootstrap 已设** `__cell.selfFetch = defaultExport.fetch`（`js.rs` ~4690） | §5.4 强调注册 | H5 对 **无状态 Worker** 可能已是 satisfied；P0 应是 **wired `globalThis.fetch`**，而非重复注册 |

**建议（parity）：** loopback **必须**与 service binding 同源路径 **同构**：isolate 内 `__beginEvent` → `selfFetch` → `__endEvent`，**禁止** loopback 默认走 `fetch_worker` 再占池（除非文档明确这是 celld 有意偏离 workerd 且证明无死锁）。

### 2.2 URL / Host / 转发头

- **Relative URL**（`/path`）：workerd 以 **当前 inbound Request 的 URL** 为 base；草案 §5.2 一致。
- **Synthetic dev Host**（`support-nuxt.lvh.me:8787` vs `127.0.0.1:8787`）：workerd/Miniflare 在 local 常与 **请求 URL 的 host** 一致；草案 §5.2(4) 的 127.0.0.1 特例需与 **`CELLD_TRUST_FORWARDED_HEADERS`**、ingress 合成 Host **单测表** 钉死，否则 loopback 与真实客户端路径 **分叉**（一条走 selfFetch，一条仍 HTTP）。
- **`Request.url` / `response.url`**：子请求后 URL 字符串是否与 workerd 一致（含 trailing slash、default port）——草案未写；Nitro 路由匹配可能敏感。

### 2.3 请求/响应语义（子请求）

| 能力 | 审查备注 |
|------|----------|
| **AbortSignal** | harness service binding 已对 self 路径做 caller/target signal 镜像（~2210–2229）；`op_fetch` loopback 若绕开 harness，易丢 parity |
| **Body / stream** | `op_fetch` 走 JSON + `streamId`；selfFetch 走 in-process `Request`；loopback 须 **同一套** body 与 backpressure 行为 |
| **Redirect** | `globalThis.fetch` 的 redirect 由 `op_fetch` 传 `req.redirect`；内部 selfFetch 是否 follow、是否暴露 `opaqueredirect` — 未定（§9 未列） |
| **`waitUntil` 隔离** | workerd 子请求常见 **独立 execution context**；selfFetch 已 `__beginEvent`；若 loopback 走池路径，`waitUntil` 挂到错误 event → **永不 `end_event`**，表现仍为挂起（与 H2 症状重叠） |
| **WebSocket** | `globalThis.fetch` 对 upgrade 走 `__fetchWebSocketUpgrade`（`harness.js` ~569）；§9 Q3 未决；与 Nitro SSR 无关但影响「fetch parity」完整性 |
| **`cf` / trace** | §9 Q2 未决；Nuxt `_platform.cloudflare` 注入依赖 inbound `cf`；子请求是否继承影响可观测性，一般不挡 SSR，但影响 SC-5「心智模型」 |

### 2.4 与 Nitro 实际用法的错位（非 workerd 缺口，但影响「修 loopback = 修 S25」）

S25 bundle 事实（compat §4、artifact `index.js`）：

```
Vr.fetch → localFetch(pathname + search)   // ~7856
localFetch("/…") → h3 b2(toNodeListener) // ~7809–7812，不经 globalThis.fetch
localFetch(非 "/…") → globalThis.fetch     // ~7818
```

因此：

- **workerd 子 fetch parity** 与 **Nitro SSR 主路径** 在 celld 上 **不是同一条管道**。
- Option A 只修补 **`globalThis.fetch` / `op_fetch`**；**不修补** `b2` 进程内 listener。
- 设计图 §5.1 将 `localFetch (h3)` 与 `loopback (op_fetch)` 画成并列汇入 selfFetch — **与当前 bundle 不符**（`localFetch` 的 `/` 分支 **不经过** `op_fetch`）。

### 2.5 SC-5 的诚实边界

草案 SC-5：「不要求字节级一致」。可接受。但若 loopback 实现选 **池路径** 而非 **isolate selfFetch**，则连「同一 isolate 内重入」这一 workerd **核心心智** 都对不齐，SC-5 不应被用作掩盖调度模型分歧的挡箭牌。

---

## 3. 为何 loopback 合入后 Nuxt 仍可能挂起

按 **可能性排序**（与 compat §5.2 一致，并对照设计前提 H1+H5）。

### 3.1 主路径仍在 H2：`localFetch` → `b2` → Vue SSR

1. 客户端 `GET /` 进入 `Vr.fetch`，对 `/` 调用 **`localFetch`**，**零次** 必经 `op_fetch`。
2. `b2` 在 **同一 isolate、同一事件帧** 内驱动 h3；依赖 **microtask、Promise、`nodejs_als`、Nitro `context` ALS** 与 celld **`__beginEvent` / `__endEvent` / `drive` 泵** 协同。
3. 若某 Promise（`renderToString`、`useAsyncData`、lazy route、内部 `localFetch('/__nuxt_error')` 等）**永远 pending**，外层 Worker `fetch` 的 Promise **永不 settle** → 与现网症状 **完全一致**。
4. **Option A 不改变此路径**；除非 SSR 内部某步 **额外** 触发 `globalThis.fetch`（绝对 URL 或 `localFetch` 非 `/` 前缀）。

### 3.2 H1 仅为「叠加层」而非主路径

H1 成立场景：SSR 或 Nitro 中间件调用 **`$fetch.native` / 未 patch 的 `createFetch({ fetch: globalThis.fetch })` / 绝对 URL 的 `localFetch` 回落** → `op_fetch` → 单 isolate 占满 + 再入等待。

- 对 **最小** `GET /` 是否触发：**未证明**；需在挂起请求上 **计数 `op_fetch`**（compat P2）。
- 若挂起期间 **`op_fetch` 调用次数为 0**，H1 可排除，**loopback 对 S25 无直接疗效**。

### 3.3 Loopback 实现不当会 **制造** 新挂起

- loopback 若走 **`fetch_worker` 同池** 且仍 **单 isolate 池深度为 1**：父 handler 占 isolate 等子请求，子请求等池 → **与 H1 同型死锁**。
- loopback 若未正确 **`__endEvent`** / `waitUntil` 门闸：handler budget 与客户端超时表现与 H2 难区分。

### 3.4 H3（`nodejs_als`）与 H2 未分清

- wrangler overlay 含 `nodejs_als`；Nitro h3 与 Vue SSR 大量使用 **AsyncLocalStorage**。
- workerd 上由运行时集成；celld 上 **flag + 泵顺序** 任一不匹配 → **仅修 fetch loopback 无效**。

### 3.5 静态 200 不能证伪 H2

`ASSETS.fetch` 旁路 Nitro SSR（compat §3）。静态正常 **只说明** Worker 入口与 `op_asset_fetch 正常**，**不能** 证明 h3 SSR 链健康。

---

## 4. 最小实验：证伪 H1 vs H2

目标：**在实现完整 Option A 之前**，用 1–2 天可完成的实验决定 P0 顺序。

| ID | 实验 | 若 H1 真 | 若 H2 真 | 成本 |
|----|------|----------|----------|------|
| **E1** | `GET /` 挂起期间统计 **`__op_fetch` / Rust `op_fetch` 调用**（计数 + URL 日志，compat P2） | **≥1** 次同源或 ingress URL | **0** 次 | 低 |
| **E2** | 临时 **仅 harness** Option B：同源 `globalThis.fetch` → `selfFetch`（不改 Rust），重跑 S25 | **200** | 仍挂 | 低 |
| **E3** | 裁剪 Worker：**无 Nitro**，仅 `export default { fetch() { return new Response('ok') } }` 同 Host | 200 | N/A | 低（基线） |
| **E4** | 裁剪 Worker：`fetch(req) { return await fetch(new URL('/', req.url)) }`（强制走 **globalThis.fetch** 同源） | 无 loopback 时挂/慢；有 E2 后 **200** | 有 E2 仍挂（不应发生除非 pool bug） | 低 |
| **E5** | 裁剪 **最小 h3**：单路由 `return html`，**禁止**任何 `fetch`；同 preset | 200 → H2 在更深 Nitro 层 | 仍挂 → **celld 事件循环 / ALS** | 中 |
| **E6** | `CELLD_HANDLER_BUDGET_S=30` + isolate **「handler is waiting on nothing」** / pending op 日志（compat §7） | budget 内 504/500 且日志有 **fetch wait** | budget 内仍 **客户端超时** 或 **microtask 停滞** | 低 |
| **E7** | 一次性 bundle 探针：`localFetch` 首行 `throw new Error('localFetch reached')`（**非** 长期 patch） | `GET /` **500** 且见错误 → 主路径确为 localFetch | 仍挂 → 未进 localFetch（路由/资产问题） | 低（脏） |
| **E8** | `nodejs_als` flag A/B：`compatibility_flags` 仅 `nodejs_compat` vs 含 `nodejs_als` | 无关 | 一侧 **恢复 200** → H3 并入 H2 | 中 |

**判定规则（建议写进 issue 门禁）：**

- **E1=0 且 E7 命中 localFetch 且 E2 后仍挂** → **H2（±H3）为 S25 P0**；Option A 降为 **通用 parity / 防回归**，不挡 S25。
- **E1>0 或 E4 在无 loopback 时复现挂起、E2 修复** → **H1 为 S25 必要条件**；Option A（须 isolate selfFetch）与 H2 调查 **并行**。
- **E5 挂、E3 不挂** → 问题在 **h3/ALS/泵**，与 loopback 无关。

---

## 5. 对推荐设计的裁决

### 5.1 Option A

| 维度 | 裁决 |
|------|------|
| **是否应做** | **是** — celld 缺少与 workerd 等价的同源 `fetch`，属平台债（compat §5.2、PD-20260902-06） |
| **是否 S25 的充分修复** | **否（默认）** — 除非 E1/E4 证 H1 在 `GET /` 热路径上 |
| **实现约束** | **必须** 与现有 **`__cell.selfFetch`** 同路径（isolate 内 + depth + `__beginEvent`）；**修订** §6 中「`fetch_worker` 同池」表述，除非附带 **无死锁** 证明与测试 |
| **Option B** | 可作为 **E2 验证** 与短期 bisect，不宜长期替代 Rust 策略层 |
| **Option C** | skeptic 同意草案：TCP loopback **不消除** 单池死锁，复杂度高，**非 S25 首选** |

### 5.2 设计文档应修改的要点

1. **分离成功标准：** SC-1（S25）与 **SC-5a（同源 fetch 集成测 E4）** 分开验收；避免「loopback 合并 = 关闭 PD」。
2. **更正 §5.1 数据流：** `localFetch('/')` **不** 经 `op_fetch` loopback。
3. **将 §9 Q1 升为 blocking：** 在 S25 e2e 前回答「挂起时 `op_fetch` 是否被调用」。
4. **H2 从 follow-up 升为并行 P0 工作包：** 事件循环泵、`nodejs_als`、h3 `b2` 与 `end_event` 交互 — 含 E5/E6/E8。

### 5.3 推荐执行顺序

```
E1 + E7 + E3  →  分支
  ├─ H1 信号 → E2/E4 → Option A（isolate selfFetch）+ S25
  └─ H1 无信号 → E5/E6/E8 优先 → Option A 与 Nitro 修复并行，S25 不以 loopback alone 为 merge 条件
```

### 5.4 总评

| 项 | 评分 / 说明 |
|----|-------------|
| 问题定义 | **强** — compat 对 `localFetch` 机制的修正可信 |
| Option A 作为 **通用设计** | **可接受**，需收紧 loopback 调度与 harness 一致 |
| Option A 作为 **S25 根因修复** | **证据不足** — **怀疑 H2 才是真 blocker** |
| 测试计划 | 缺 **H1/H2 分叉实验**；§7 直接上 full `support-nuxt` 易 **假阴性**（loopback 合入仍挂） |

**Verdict：** **有条件批准 Option A 作为 workerd fetch parity 的 P0 平台能力**；**拒绝** 将「仅 Option A」作为 S25 关闭条件。在 **E1/E7** 结果出来之前，主会话应假定：**修 loopback 可能修不好 Nuxt SSR**。

---

## 6. 变更 log

| 日期 | 变更 |
|------|------|
| 2026-09-01 | 初版：parity 缺口、H2 优先论证、证伪实验、Option A 条件裁决 |
