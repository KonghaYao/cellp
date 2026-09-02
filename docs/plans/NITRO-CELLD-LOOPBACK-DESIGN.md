# Nitro / Worker 同源 Fetch Loopback — 设计文档

> **状态：** Draft v0.2（对抗审查已并入；实现前须完成 Phase 0）  
> **作者：** opus 任务（主会话起草）；v0.2 合并安全 / parity 审查  
> **依据：** [NITRO-CELLD-COMPAT.md](./NITRO-CELLD-COMPAT.md) · [PD-20260902-06](../platform-defects-log.md)  
> **架构约束：** [DESIGN.md](../../DESIGN.md) AD-1 · [decisions.md](../decisions.md) AD-10 · AD-12  
> **对抗审查：** [安全](./NITRO-CELLD-LOOPBACK-DESIGN-REVIEW-SECURITY.md) · [parity](./NITRO-CELLD-LOOPBACK-DESIGN-REVIEW-PARITY.md)

---

## Adversarial review summary

| 审查 | Verdict | 对 v0.2 的约束 |
|------|---------|----------------|
| **Security** | **APPROVE WITH CONDITIONS** | loopback **fail-closed**；禁止对 cellp Gateway / 邻 celld 端口 **reqwest 再入**；canonical origin 绑定 generation；**唯一**实现路径 = isolate 内 `__cell.selfFetch`（禁止 `fetch_worker` 占第二池槽） |
| **Parity** | **有条件批准 Option A 作 workerd fetch parity**；**拒绝**「仅 loopback」作为 S25 关闭条件 | 先做 **E1/E7** 证伪 H1；`localFetch('/')` **不经** `op_fetch`；S25（SC-1）与同源 fetch 集成测（SC-5a）**分开验收** |

**已闭合的设计矛盾（v0.1 §5 图 vs §6 表）：** loopback **MUST** 与 service binding 同构——同 isolate `__beginEvent` → `__cell.selfFetch` → `__endEvent`。**禁止** loopback 默认走 `fetch_worker` 池再入，也 **禁止** HTTP 打回 `:8787` / `879x`。否则 H1 死锁从「HTTP 形态」变成「池形态」，并打开跨 Host Gateway SSRF。

---

## 1. Problem statement

### 1.1 现象

Nuxt 3 + Nitro `preset: cloudflare_module`（S25 `support-nuxt`）在 celld 上：

- **SSR** `GET /`：客户端挂起直至超时（Promise 不 settle）
- **静态** `GET /robots.txt`、`/_nuxt/*`、`/favicon.ico`：**200**

静态 200 **只说明** Worker 入口与 `op_asset_fetch` 正常，**不能**证伪 H2（h3 SSR 链）。

### 1.2 Success criteria

| ID | 标准 | 关闭条件 |
|----|------|----------|
| SC-1 | S25 prod Host `GET /` 在 **≤30s** 内返回 **200** + HTML（含 `<!DOCTYPE` 或 Nuxt 标记） | **独立于** loopback 合入；须 Phase 0 判定 H1 vs H2 |
| SC-2 | 现有 **S22 Astro / S23 SvelteKit / S24 Remix** 回归无退化（同 Host 验收脚本） | loopback 不得改变 **非同源** `fetch` |
| SC-3 | 不引入 **跨 version** 或 **跨 project** 请求误 loopback（安全） | Must-fix URL 表 + 安全回归（见 §5.2 / §8） |
| SC-4 | 同源子请求深度有界（与 service binding `svcDepth=8` 同级或更严） | 同 isolate 重入；**不得**池嵌套等待 |
| SC-5 | 行为可对照 **workerd** 文档/Miniflare 心智模型（不要求字节级一致） | 心智核心 = **同一 isolate 内重入**，不可用「不字节级」掩盖池路径 |
| **SC-5a** | 裁剪 Worker：`fetch(req) { return await fetch(new URL('/', req.url)) }` 同源 **200**（parity E4） | **loopback 合入的验收**；**不**等于关闭 SC-1 / PD |

### 1.3 非目标

- 不支持多 Worker `[[services]]`（cellp 产品边界，见 MULTI-WORKER-DEPLOY）
- 不在 `support-nuxt/prepare-artifact.sh` 长期改写 Nitro `localFetch`
- 不承诺 Node.js 全量 `node:http` 与 Nitro 开发服务器 parity
- Phase 1 **不**修补 h3 `b2` / `localFetch('/')`（属 Phase 2 / H2）

---

## 2. workerd / CF vs celld 今日

| 能力 | workerd / CF Workers | celld 今日 |
|------|----------------------|------------|
| 对外 `fetch(url)` | 运行时调度；同源常 **内部 dispatch** | `op_fetch` → **reqwest HTTP** 出网（无 URL 策略） |
| Service binding 同源 | `selfFetch` 类路径 | `__cell.selfFetch` + `svcDepth`（**仅** binding 入口；bootstrap **已**设 `selfFetch = defaultExport.fetch`） |
| `globalThis.fetch` 指向自己 | 内部 loopback / 不占用公网连接 | **无**；可能再入 `:8787` 或外网 |
| ASSETS | `env.ASSETS.fetch` | `op_asset_fetch` ✅ |
| Nitro `localFetch` | h3 进程内 listener | **`/` 前缀不经过** `op_fetch`；依赖 V8 事件循环 + ALS |
| 每 version 进程 | 1 fleet / 1 deploy（CF 账号内） | **AD-1** 1 celld / version ✅ |
| `waitUntil` / `ctx.waitUntil` | 支持 | 部分支持；与 `end_event` 交互待验 |

---

## 3. Root cause hypotheses（排序）

| 秩 | 假设 | 证据 | 若成立，loopback 是否足够 |
|----|------|------|---------------------------|
| H1 | SSR 链中 **`globalThis.fetch` 打同源 URL**，`op_fetch` 再入 gateway/celld，**单 isolate 池死锁** | compat；`createFetch` 默认 `globalThis.fetch` | **是**（Phase 1）；**未证明**为 `GET /` 热路径 |
| H2 | Nitro **`localFetch`（h3 `b2`）** 在 celld 下 microtask/ALS 不推进 | bundle：`Vr.fetch` → `localFetch(pathname)` → `b2`，**不经** `op_fetch` | **否**（Phase 2：事件循环 / ALS） |
| H3 | **`nodejs_als`** / `compatibility_flags` 与 Nitro 上下文不兼容 | wrangler 含 `nodejs_als` | 可能需标志或 harness 补丁（并入 Phase 2） |
| H4 | 部署 `_routes` / `run_worker_first` 误配 | artifact 无 `_routes.json` | **否**（已排除） |
| H5 | Worker 未注册 `selfFetch` 于无状态入口 | bootstrap **已**注册；缺口是 **`globalThis.fetch` 未接到 selfFetch** | 重复注册非 P0；P0 是 **wired default fetch** |

**设计前提（v0.2）：**

- Option A 是 **workerd 同源 fetch 平台债**（无论 S25 是否因此痊愈）。
- **S25 根因未定**：先假定 **H2 可能才是 blocker**；Phase 0（E1/E7）之后再决定 S25 是否以 loopback 为 merge 条件。
- 实现对象：**H1 的调度裂缝**（`globalThis.fetch` → 出网/池再入）为 Phase 1；**H2/H3** 为 Phase 2 并行工作包，不降为「日后 issue 即可合 loopback 关 PD」。

---

## 4. Design options

### Option A — `op_fetch` / harness 同源 → isolate `selfFetch`（推荐）

**行为：** 判定 URL 是否属于 **本 generation 的 canonical origin**。若是 → **同 isolate** `__cell.selfFetch(req, env, ctx)`（新 `__beginEvent` / `__endEvent`，递增 depth）。**不得**再 HTTP、**不得** `fetch_worker` 租第二 isolate。

**Pros：** 复用 service binding 同源语义；对齐 workerd「同一 JS 堆重入」；不改 Nitro bundle。  
**Cons：** 须 fail-closed URL 判定；不修复 H2 `localFetch`。

### Option B — 仅改 harness `globalThis.fetch`（不经 Rust）

同源则 `selfFetch`，否则 `op_fetch`。

**Pros：** 迭代快；适合 **E2 证伪**。  
**Cons：** 绕过 Rust 策略；与 `op_fetch` 其他调用方不一致。**不宜长期替代**；若与 Rust 判定分叉，攻击者打较弱路径。

### Option C — 独立 internal listener（`--internal-listen`）

子请求走 loopback TCP。

**Pros：** 与「真实 HTTP」表面一致。  
**Cons：** **不消除**单池死锁；延迟与复杂度高。**非** S25 / Phase 1 首选。

### Option D — 产品：Nuxt SSR 标不支持，仅静态 export

违背 AD-13；已失败 S25。否决。

### Option E — Nitro 构建期强制 `localFetch` only + 禁用 outbound fetch

违反「非应用长期 patch」。否决为产品路径（E7 一次性探针除外）。

---

## 5. Recommended: Option A（canonical 判定在 Rust）+ harness 薄封装

判定函数 **唯一、Rust 为准**；JS 只调用 op 或共享常量表。`CELLD_FETCH_LOOPBACK=0` 仅 bisect，**不得**作为 production 默认（回退恢复 Gateway 再入面）。

### 5.1 组件

`localFetch('/')` **不**汇入 `op_fetch` loopback。两条管道：

```
┌─────────────┐     GET /      ┌──────────────┐
│   Client    │ ──────────────►│ cellp Gateway│
└─────────────┘                └──────┬───────┘
                                      │ Host + synthetic
                                      ▼
                               ┌──────────────┐
                               │ celld ingress│
                               └──────┬───────┘
                    asset miss       │ fetch_worker（仅 inbound）
                                      ▼
                               ┌──────────────┐
                               │ isolate      │
                               │ Worker.fetch │
                               └──┬────────┬──┘
                                  │        │
                    localFetch('/')│        │ globalThis.fetch(same-origin)
                    h3 b2 (H2)    │        ▼
                                  │  ┌─────────────────────────────┐
                                  │  │ Phase 1 loopback（同 isolate）│
                                  │  │ __beginEvent → selfFetch     │
                                  │  │ → __endEvent；depth ≤ 8      │
                                  │  │ 禁止 fetch_worker / HTTP GW  │
                                  │  └─────────────────────────────┘
                                  ▼
                            Phase 2：泵 / ALS（若 E1=0 仍挂）
```

### 5.2 URL 判定规则（fail-closed）

**Origin 白名单** = 本 generation **不可变**三元组，由 Rust 在进程启动 / deploy 注入 op 层：

`(synthetic_host, public_scheme, gateway_port | dedicated_port)`

与 `PUBLIC_BASE_URL` **交叉校验**。**禁止**单独信任 Worker 可见的 `request.url` 或最后一次 `X-Forwarded-Host` 成 origin。`CELLD_TRUST_FORWARDED_HEADERS=1` 时，loopback **base URL 必须**来自 celld 合成的 **canonical URL**。

规范化（同源判定与出网 denylist **共用**）：IDNA、IPv6（含 `[::1]`）、zone id、default port、去 userinfo、尾点 hostname。**禁止**后缀 / wildcard 匹配（`*.lvh.me`）。

| # | 输入 | 动作 |
|---|------|------|
| 1 | **Relative** `/path` | 以 **canonical inbound URL**（非原始 `X-Forwarded-*`）为 base 解析，再走绝对规则 |
| 2 | **Absolute** 且 scheme+host+port **精确等于** canonical origin | **loopback** → in-isolate `selfFetch`；**重写** `Host`（及选用的 `X-Forwarded-*`）为 synthetic canonical；剥离 hop-by-hop（`Connection` / `Keep-Alive` / `Transfer-Encoding` 等，对齐 ingress） |
| 3 | `127.0.0.1` / `localhost` / `[::1]` / `127.0.0.1.` / RFC1918 / link-local / metadata（含 `metadata.google.internal`）指向 **cellp Gateway 或任意 celld 公网/operator 口** | **拒绝**（或仅当 **同时** 命中本 generation canonical **且** 可无歧义映射为 self 时 **仅** in-process selfFetch）。**一律不得** `reqwest` 到这些地址（含 Worker 自带 **外国 Host**） |
| 4 | 其他内网 / metadata（非本 origin） | **拒绝** 或走未来出网 denylist；**不得**因「像同源」而 loopback；**不得** silent HTTP fallback |
| 5 | 外网、他 project / 他 version host（字符串不等） | 出网 `reqwest`（现行为）；**不得**误 loopback |
| 6 | 判定失败 / 解析歧义（八进制 IP、`evil@127.0.0.1`、双 Host） | **fail-closed：拒绝**。禁止「判不成同源就 reqwest」打到 Gateway |

**废除 v0.1 rule 4 的宽松 dev 特例：** 不得仅凭「`127.0.0.1:{upstream}` + Host 碰巧一致」走 reqwest 或半套 loopback。dev 栈只允许 **canonical 命中 → selfFetch + Host 重写**。

**SC-3 单测最少集：** 他 project host、他 version 专用口、`127.0.0.1:8787` + 错 Host、`[::1]:8787`、metadata、RFC1918、IPv6 loopback、相对 URL + 污染 `X-Forwarded-Host`。

### 5.3 深度与重入

- 复用 **`svcDepth`**（与 binding **共享**计数器，8 层混合即耗尽——可接受，须测）。独立 `loopbackDepth` / 并发信号量为 nice-to-have。
- 超限：`Error: loopback recursion limit exceeded`。
- **同一 isolate** 内允许重入（Nitro SSR / 子 fetch 常见）。
- **禁止** loopback 路径调用 `fetch_worker` / `fetch_worker_pool` **等待第二个池槽**（父占槽、子要槽 = 原死锁）。
- `waitUntil` 挂到 **本子事件**；错误挂到父 event → 永不 `end_event`。硬上限仍为 nice-to-have；实现须保证 `__endEvent` 成对。

### 5.4 `selfFetch` 与 `globalThis.fetch`

- bootstrap **已** `__cell.selfFetch = defaultExport.fetch`。Phase 1 焦点是把 **`globalThis.fetch` / `op_fetch` 同源分支接到该路径**，不是再注册一遍。
- 仍须审查：无 `selfFetch` 时 **拒绝** loopback（勿 silently 出网）。
- AbortSignal、body / `streamId` 与 binding 同源分支 **同一套**（harness 已有 caller/target signal 镜像）。

### 5.5 Feature flag

- `CELLD_FETCH_LOOPBACK=1`：bake-in 后默认 on；**须先有 SC-3 自动化** 才可默认开。
- `=0`：bisect only；off 时同源行为须 **文档为「仍出网 / 或明确拒绝」** 并 E2E 覆盖，避免半开半关。

### 5.6 子请求头与可观测性（平台）

- 强制 synthetic `Host`；不把 Worker 提供的 Host 原样送入 `selfFetch`（防自指混淆）。
- 子请求 **不得**冒充 inbound `cf`；trace 用 **子 span**（`traceparent` 不复用父 span 冒充入口）。
- Cookie / `Authorization` 随子请求 headers 走——同租户 app 责任；文档化与 CF 差异。

---

## 6. API / 代码触点

| 层 | 文件 | 变更 |
|----|------|------|
| Rust | `js.rs` `op_fetch` | **唯一** URL 判定（§5.2）；同源 → **调度 isolate 内 selfFetch**（非 HTTP）；歧义/内网 GW → **拒绝** |
| JS | `harness.js` | `globalThis.fetch` 薄封装 **调用同一判定/同一 selfFetch**；复用 binding 的 depth / signal / `__beginEvent` |
| Rust | `runtime.rs` | **不**为 loopback 走 `fetch_worker` 同池路径。inbound 仍 `fetch_worker`；子请求 **不**再租池 |
| Ingress | `main.rs` | 文档 + 测试：loopback **永不**经公网 Gateway / 邻 celld listener |
| 测试 | `celld/tests/` | 最小 Worker：`fetch(new URL('/', request.url))`；**另** `fetch(http://127.0.0.1:8787, Host: victim)` **必须失败** |

---

## 7. Phased plan

```
Phase 0（廉价证伪，1–2 天，挡完整 P0 实现）
  E1  GET / 挂起期间统计 op_fetch 次数 + URL
  E7  一次性 localFetch 首行 throw（非长期 patch）
  （建议顺带 E3 基线：无 Nitro 的 default fetch 200）
        │
        ▼
Phase 1  loopback：globalThis.fetch / op_fetch → isolate selfFetch
         验收 = SC-5a + SC-3 安全表；默认不关 SC-1
        │
        ▼
Phase 2  若 S25 仍挂（或 E1=0 且 E7 命中 localFetch）
         H2 localFetch / h3 b2 / 事件循环泵 / nodejs_als（E5/E6/E8）
```

**判定（写入 issue 门禁）：**

| Phase 0 结果 | 后续 |
|--------------|------|
| **E1=0** 且 **E7 命中 localFetch** | H2（±H3）为 S25 P0；Option A 仍做（parity / 防回归），**S25 不以 loopback alone 为 merge 条件** |
| **E1≥1** 或无 loopback 时 E4 复现挂起 | H1 为 S25 **必要**条件；Phase 1 与 H2 调查并行 |
| E5 挂、E3 不挂 | 问题在 h3/ALS/泵，与 loopback 无关 |

完整实验表（E2–E8）见 [parity 审查 §4](./NITRO-CELLD-LOOPBACK-DESIGN-REVIEW-PARITY.md)。E2（仅 harness Option B）只作验证，不替代 Phase 1。

---

## 8. Test plan

| 层级 | 内容 |
|------|------|
| Phase 0 | E1 计数；E7 探针；记录是否进入 `localFetch` |
| Unit | §5.2 表（relative、canonical、synthetic host、127.0.0.1+错 Host、IPv6、metadata、RFC1918、X-Forwarded 污染） |
| Integration | 内联 Worker 同源 fetch **200**（SC-5a）；Gateway / 邻端口 + 外国 Host **拒绝** |
| E2E | `support-nuxt` S25：**仅当** Phase 0 显示 H1 或 Phase 2 完成后作为 SC-1；**禁止**「loopback 合入仍挂」当假阴性关单 |
| Regression | S22–S24；`cargo test -p celld`；flag on/off 行为成文 |
| 安全回归 | 最小 Worker「fetch 同源 + fetch gateway」进 `celld/tests/` 或 cellp e2e |

---

## 9. Rollout & risk

| 风险 | 缓解 |
|------|------|
| SSRF：判失败 → reqwest Gateway + 外国 Host | **fail-closed**；内网/loopback 地址 **无** HTTP 到 cellp 监听口 |
| 死锁（loopback 再要池） | **仅**进程内 selfFetch；§6 已删同池路径 |
| Origin 配置漂移（错误 `PUBLIC_BASE_URL`） | generation 绑定三元组 + 交叉校验 |
| 双路径判定分叉（harness vs Rust） | Rust canonical；JS 不自写第二套规则 |
| loopback 合入后 Nuxt 仍挂 | 预期可能（H2）；SC-1 ≠ SC-5a |
| Astro `_routes` 回归 | 全量 e2e Host |
| flag=0 扩大攻击面 | 非生产默认；须 SC-3 测试才默认 on |
| 性能 | loopback 应快于 reqwest；压测可选 |

**实现阶段停发（安全 REJECT）：**

- 仍可能对 `127.0.0.1` / RFC1918 的 cellp/celld 地址 `reqwest`；或  
- loopback 经 `fetch_worker_pool` 同步等第二槽；或  
- 无 SC-3 自动化即默认 `CELLD_FETCH_LOOPBACK=1`。

---

## 10. Open questions

| # | 问题 | 状态 | 决议 / 残留 |
|---|------|------|-------------|
| 1 | Nitro 主路径 `localFetch` 不经 `op_fetch` 时，H2 是否仍需单独修复？ | **部分决议** | **是，按 Phase 0 分支**：E1=0∧E7 命中 → H2 为 S25 P0。**仍开：** 挂起时 `op_fetch` 是否被调用（E1 未跑） |
| 2 | loopback 是否继承原请求 `cf` / trace？ | **部分决议** | **不**冒充 inbound `cf`；trace = **子 span**。**仍开：** 字段级清单 |
| 3 | WebSocket 同源 upgrade 是否同一 loopback？ | **仍开**（nice-to-have） | SSR 不依赖；须避免 HTTP loopback + WS 仍出网的长期分叉；可先拒绝 WS loopback |
| 4 | 多 ready version 时误把 Host 指到别 project？ | **已决议** | canonical origin + 禁止 GW reqwest（§5.2）；AD-1 存储不跨进程，**HTTP 再入才是破坏面** |
| 5 | `CELLD_TRUST_FORWARDED_HEADERS` 与 URL 判定？ | **已决议** | base **必须** canonical，不单独信 `X-Forwarded-Host` |
| 6 | `redirect` / `opaqueredirect` 在 selfFetch 上是否 follow？ | **仍开** | 须对齐 `op_fetch` 的 `req.redirect`，写入单测 |
| 7 | `Request.url` / `response.url` 字符串（slash、default port） | **仍开** | Nitro 路由可能敏感；与 workerd 对照表 |

---

## 11. 变更 log

| 日期 | 变更 |
|------|------|
| 2026-09-02 | v0.1 初稿（opus 流中断，主会话起草） |
| 2026-09-02 | **v0.2** 并入安全 / parity 对抗审查：审查摘要表；闭合 selfFetch vs `fetch_worker` 矛盾；§5.2 fail-closed；Phase 0→1→2；SC-5a；Open Q 分已决议 / 仍开 |
