# NITRO-CELLD-LOOPBACK Option A — 敌对安全 / 平台审查

> **现行设计：** [NITRO-CELLD-LOOPBACK-DESIGN.md](./NITRO-CELLD-LOOPBACK-DESIGN.md) **v0.2**（已并入本审查 must-fix）  
> **审查对象：** [NITRO-CELLD-LOOPBACK-DESIGN.md](./NITRO-CELLD-LOOPBACK-DESIGN.md) v0.1 §4 Option A + §5 推荐方案  
> **架构约束：** [DESIGN.md](../../DESIGN.md) AD-1 · [decisions.md](../decisions.md) AD-1 / AD-12  
> **审查立场：** 假定 Worker 代码恶意或可被利用；Gateway / 多 version 同机部署；`CELLD_TRUST_FORWARDED_HEADERS=1` 为 cellp ready 门禁（[INGRESS-ROUTING.md](./INGRESS-ROUTING.md) §4.2）  
> **审查日期：** 2026-09-02  
> **结论摘要：** **APPROVE WITH CONDITIONS**（实现前必须闭合 Gateway 再入与 origin 绑定；否则 REJECT）

---

## 1. 审查范围与威胁模型

| 资产 | 说明 |
|------|------|
| **AD-1 隔离** | 每 ready version = 独立 celld 进程 + 独立 `s3://cellp-celld/{project}/{version}` |
| **SC-3** | 不跨 version / 跨 project 误 loopback |
| **节点邻域** | 同主机上多 celld（`8792+N`）、Gateway `:8787`、celld internal operator（loopback） |
| **信任边界** | celld 文档明确：**非** hostile multi-tenant（[celld/docs/security.md](../../celld/docs/security.md)）；cellp 仍须在 **单 version 内** 防止 fetch 成为 **横向移动**（Gateway → 他租户 upstream） |

**不在范围：** Nitro `localFetch`（h3 `b2`）路径——Option A **不修复** H2；本节仅标注可用性/审查盲区，不作为 Option A 否决项。

---

## 2. 攻击场景

### 2.1 SSRF：同源误判 → Gateway 再入（**Critical**）

**现状：** `op_fetch`（`celld/crates/celld/js.rs`）对任意 URL 直接 `reqwest`，**无** URL 策略；挂起根因即 SSR 链可能 `fetch` 到 `http://127.0.0.1:8787/` 或对外 origin。

**Option A 意图：** 同源 → `__cell.selfFetch`，**禁止** loopback 内再 HTTP 到 Gateway（设计 §8）。

**攻击：**

1. Worker（或依赖链）执行  
   `fetch("http://127.0.0.1:8787/admin", { headers: { Host: "victim.other.lvh.me" } })`  
2. 若 URL **未**命中 loopback（设计 §5.2 rule 2 排除「仅 127.0.0.1:celldPort」而无 origin 绑定），请求仍走 **reqwest** → 打到 **cellp Gateway**。  
3. Gateway 按 **请求 Host** 选 `ingress_bindings`（AD-12）→ **跨 project / 跨 version** 访问他租户 celld 的 HTTP 面（在共享 `DEPLOY_TOKEN` / 无 app 级 RBAC 的 cellp 模型下，这是 **平台级 SSRF**）。

**变体：**

- `fetch("http://127.0.0.1:8793/...")` 直连他 version 的 celld 公网 listener（若端口可从 Worker 视角到达）。  
- `fetch("http://[::1]:8787/")`、十进制/八进制 IP 混淆、userinfo `http://evil@127.0.0.1:8787/`、尾点 hostname `http://127.0.0.1.`（若解析器与判定逻辑不一致）。

**设计 §5.2 rule 4（dev 特例）：**  
「`127.0.0.1:{upstream}` **且** Host 与 ingress 合成 Host **一致** 时视为同源」→ loopback 到 **本 Worker**。  
但若实现用 **URL authority** 判同源、却 **原样转发** Worker 提供的 `Host` 进入 `selfFetch`，应用内基于 `Host` 的路由/鉴权可能被 **自指混淆**（非跨租户，但可绕过 app 层 hostname 检查）。更严重的是 **判同源失败** 时的 reqwest 路径（上一条）。

**Must-fix：** loopback 判定 **fail-closed**；凡 `127.0.0.1` / `localhost` / RFC1918 / link-local / metadata 端点 **一律不得** 走未鉴权的出网 reqwest 到 cellp 监听地址（应拒绝或仅允许 **进程内** selfFetch 且 **重写** Host 为 generation 绑定的 `synthetic_host`）。

---

### 2.2 跨租户 / 跨 version Host 与 origin 混淆（**High**）

**机制：** AD-12 下 `request.url` 的 origin 来自 Gateway 注入的 synthetic `Host` + `PUBLIC_BASE_URL`（[INGRESS-ROUTING.md](./INGRESS-ROUTING.md) §5.1）。同源判定草案（§5.2）使用：

- `request.url` 的 scheme+host+port，或  
- `PUBLIC_BASE_URL` / synthetic host  

**攻击：**

1. **环境变量篡改**（compromised deploy、错误 orchestrator）：`PUBLIC_BASE_URL` 指向 **父 version** 或 **兄弟 project** 的 URL → Worker 内 `fetch('/api')` 或 `fetch(PUBLIC_BASE_URL + '/x')` 的「同源」语义与 **实际 celld 进程**（AD-1 单 bucket）不一致；数据仍来自本进程，但 **应用逻辑**（回调 URL、OAuth redirect、签名串）可能指向错误租户。  
2. **`fetch("https://other.preview.lvh.me/secret")`**：若与当前 `request.url` host **字符串相等**失败，走 reqwest——正确；若 wildcard / 后缀匹配 bug（`*.lvh.me` 误配），则 **误 loopback** 或 **误出网**。  
3. **相对 URL** `/path` 以 inbound `request.url` 为 base：若 inbound URL 被 `X-Forwarded-*` 污染（`CELLD_TRUST_FORWARDED_HEADERS=1`），则 **子请求 base 偏移** → 同源集合错误。设计 Open Q §5 已列，无闭合答案。

**AD-1 视角：** 误 loopback **不会**直接读他 version 的 bucket（不同进程）。**真正 AD-1 破坏**是 **经 Gateway 的 HTTP SSRF**（§2.1）或 **长期错误 `PUBLIC_BASE_URL`** 导致运维上 version 间逻辑耦合。

**Must-fix：**

- Origin 白名单 = **本 generation 不可变三元组** `(synthetic_host, public_scheme, gateway_port|dedicated_port)`，由 Rust 在进程启动/deploy 时注入 op 层，**禁止**单独信任 Worker 可见的 `request.url` 字符串而不与配置交叉校验。  
- 文档化：`CELLD_TRUST_FORWARDED_HEADERS=1` 时 loopback base URL **必须**来自 celld 侧合成的 canonical URL，而非最后一次 `X-Forwarded-Host`。

---

### 2.3 递归 / 资源耗尽 DoS（**Medium–High**）

| 向量 | 说明 |
|------|------|
| **深度** | 设计复用 `svcDepth` 上限 8（`harness.js` ~2236–2248）；与 service binding **共享**计数器 → 8 层 `fetch`+binding 混合即可耗尽，行为可接受但需 **明确文档与测试**。 |
| **广度** | 单层 `Promise.all` 发起数百同源 `fetch`：每层若仍占 isolate/池槽位，可 **CPU/内存** 压垮单 celld（AD-1 仅进程级隔离）。设计 **未** 限制并发 loopback 数。 |
| **waitUntil** | §5.3：「禁止无界 waitUntil 链导致永不 `end_event`」——**无机制**，仅愿望。恶意或 buggy Worker 可挂住 generation。 |
| **死锁** | 设计图：isolate 内 `selfFetch`；§6 表：`runtime.rs`「loopback 走 `fetch_worker` **同池**路径」——**两处语义冲突**。若实现为 **池上嵌套等待**（父请求占槽、子请求也要槽），仍可复现 PD 描述的 **单池死锁**。 |

**Must-fix：** 实现规格 **唯一**：loopback **必须**走与 service binding 相同的 **同 isolate `selfFetch` + `__beginEvent`/`__endEvent`**，**不得**在 loopback 路径调用 `fetch_worker_pool` 等待第二个池槽。§6 表需改稿以免实现者误读。

**Nice-to-have：** 独立 `loopbackDepth`；并发 loopback 信号量；`waitUntil` 与 `end_event` 联动的上限或超时。

---

### 2.4 Header 走私与请求语义漂移（**Medium**）

**入口：** `op_fetch` 接受 JSON 化 header 列表（`js.rs`），完整交给下游。

| 风险 | 详情 |
|------|------|
| **Hop-by-hop** | `Connection` / `Keep-Alive` / `Transfer-Encoding` 进入 loopback 子请求 → 与 ingress 行为不一致或异常。 |
| **双重 Host** | Fetch API 通常禁止用户设 `Host`；若 harness 允许或未来 loader 路径不同，**URL 判同源 + Host 指向他域** 组合见 §2.1。 |
| **Cookie / Authorization** | 同源 loopback 默认 **继承** 子请求 headers → SSR 中间件 `fetch('/api', { credentials: 'include' })` 可能放大为 **内部 API 滥用**（同租户内，属 app 责任；平台应文档说明与 CF 差异）。 |
| **Trace 头** | `traceparent` 在 `op_fetch` 自动注入；loopback 子事件应 **子 span** 还是 **复用**？Open Q §9.2 未决；错误实现可污染审计。 |

**Must-fix（平台）：** loopback 子请求 **强制** `Host`（及可选 `X-Forwarded-*`）为 canonical synthetic 值；剥离或拒绝 hop-by-hop / 明显走私组合（与 ingress 规则对齐）。

**Nice-to-have：** 与 workerd 对齐的 forbidden header 列表单测。

---

### 2.5 与现有出网 SSRF 策略的裂缝（**Medium**）

设计 §5.2 rule 3：metadata / RFC1918 等「仍走出网或拒绝（与现有 SSRF 策略一致）」。

**事实：** 当前 `op_fetch` **未见** 对等 CF 的 blocklist 实现（审查时仅 `reqwest` 直发）。Option A 若只对「同源」特殊处理，**非同源** 内网扫描行为 **不变**——cellp 单节点上仍可打 `http://10.x/celld-internal`（若网络可达）。

**对 cellp：** AD-10 不做 WAF；但 **Operator 期望** Worker 不能随意扫 RFC1918。loopback 工作不应 **削弱** 未来 SSRF 加固。

**Must-fix：** 在设计/实现任务中 **单列**：同源 loopback 与 **出网 denylist** 共用 URL 规范化（IDNA、IPv6、zone id、default port）；内网 URL **不得**因「像同源」而 loopback。

---

### 2.6 WebSocket / 非 GET 子请求（**Low–Medium，规格缺口**）

Open Q §9.3：同源 WebSocket 是否同一 loopback？若 `op_fetch` 仅处理 HTTP，而 `Upgrade` 走别路径，可能出现 **HTTP loopback + WS 仍出网** 的不一致。Nuxt SSR 主路径不依赖 WS，但 **安全一致性** 需要决策。

**Nice-to-have：** 明确拒绝或实现 WS loopback；安全测试列入 backlog。

---

### 2.7 Feature flag 与 Option B 双路径（**Low**）

推荐「Option A + harness 薄封装」（§5）：若 harness 与 Rust 判定 **不一致**，攻击者可针对 **较弱路径**（仅 harness 或仅 `op_fetch`）构造 URL。`CELLD_FETCH_LOOPBACK=0` 回退 **恢复** Gateway 再入死锁/SSRF 面，仅适合 bisect，**不得** production 默认。

**Must-fix：** 单一 canonical 判定函数（Rust 为准，JS 仅调用 op 或共享常量表）；E2E 覆盖 flag on/off 时 **同源 URL 行为一致或 off 时明确拒绝同源**。

---

## 3. AD-1 隔离违规清单

| 违规类型 | Option A 是否引入 | 条件 |
|----------|-------------------|------|
| 跨 version **存储** 读写在单 fetch 内完成 | **否**（同进程单 bucket） | — |
| 跨 version **HTTP** 经 Gateway/celld 端口 | **是** | §2.1 reqwest 再入未禁止 |
| 跨 project **路由** 选错 upstream | **是** | 恶意 `Host` + `127.0.0.1:8787` |
| 同进程多 tenant（违反 celld 1 fleet） | **否** | AD-1 已 1 version / celld |
| **错误 `PUBLIC_BASE_URL`** 导致逻辑串租户 | **边缘** | 运维/注入问题，需 generation 绑定 |

**结论：** Option A **本身不违反** AD-1 的进程/bucket 模型；**违反 SC-3 / AD-1 安全意图** 的主要路径是 **loopback 失败后的 HTTP 再入** 与 **origin 配置漂移**，不是 selfFetch 原语。

---

## 4. Must-fix vs Nice-to-have

### Must-fix（合并前门禁）

1. **禁止 loopback 路径上的 Gateway/celld 公网 listener 再入**（reqwest 到 `127.0.0.1:8787`、`127.0.0.1:879x` 等）；同源失败时 **拒绝** 或 **仅** in-process selfFetch，不得 silent fallback HTTP。  
2. **Canonical origin** 绑定当前 generation（synthetic host + scheme + port），相对 URL 仅对该 origin 解析；与 `PUBLIC_BASE_URL` 交叉校验。  
3. **实现唯一路径：** loopback = isolate 内 `selfFetch`（与现有 service binding 同源分支一致），修正设计 §6 `fetch_worker` 同池表述。  
4. **URL 规范化单测表：** SC-3 用例（他 project host、他 version 专用口、127.0.0.1+错 Host、metadata、RFC1918、IPv6 loopback）。  
5. **Loopback 子请求：** 强制 synthetic `Host`；hop-by-hop 处理策略成文。  
6. **`selfFetch` 注册：** §5.4 无状态入口必须注册——否则部分入口仍只走出网（与 H5 一致，属功能+安全）。  
7. **安全回归：** 最小 Worker「fetch 同源 + fetch gateway」集成测试进 `celld/tests/` 或 cellp e2e。

### Nice-to-have

- 独立 `loopbackDepth` 与 `svcDepth` 分计  
- 并发 loopback 上限  
- `waitUntil` / `end_event` 资源上限  
- WebSocket loopback 策略  
- 出网 RFC1918 全面 blocklist（与 loopback 共用解析器）  
- OpenTelemetry 子 span 规范  
- 压测：loopback vs reqwest 延迟与池占用  

---

## 5. 与设计 Success Criteria 对照

| ID | 审查意见 |
|----|----------|
| SC-1 | Option A 针对 H1/H5；**不保证** H2 `localFetch`——审查不阻塞，但须在 PD 中保留 H2 跟踪 |
| SC-2 | 回归依赖 Host e2e；loopback 不得改变 Astro 等 **非同源** fetch 行为 |
| SC-3 | **当前草案不足**；须 §4 Must-fix 1–4 闭合 |
| SC-4 | `svcDepth=8` 可接受；须防池死锁（Must-fix 3） |
| SC-5 | 对齐 workerd 心智模型在 **Gateway SSRF 禁止** 上必须更保守（cellp 多租户同机） |

---

## 6. 对抗审查 Open Questions（§9）立场

| # | 立场 |
|----|------|
| 1 H2 localFetch | 安全审查：**不阻塞 Option A**；可用性风险单独 issue |
| 2 cf / trace | Must-fix 子 span；禁止子请求冒充 inbound `cf` |
| 3 WebSocket | Nice-to-have 决策 |
| 4 多 ready version Host 边界 | **Must-fix** 归入 Gateway SSRF / canonical origin |
| 5 Forwarded 交互 | **Must-fix** canonical URL 不依赖不可信 forwarded 单独成 origin |

---

## 7. Verdict

### **APPROVE WITH CONDITIONS**

**理由：**

- Option A 方向正确：用 **进程内 `selfFetch`** 对齐 workerd 同源子请求，是修复 PD-20260902-06（H1/H5）的合理 P0，且 **不破坏** AD-1 进程/bucket 隔离。  
- 草案对 **最危险裂缝**（同源判定失败 → reqwest 打 Gateway、§5.2 rule 4 dev 特例、§6 与 §5 池路径矛盾）描述不足，**在未闭合前等同于接受平台 SSRF**。

**拒绝（REJECT）条件（若实现阶段出现任一即应停发）：**

- loopback 判定后仍可能对 `127.0.0.1`/RFC1918 的 cellp/celld 地址做 reqwest；或  
- loopback 经 `fetch_worker_pool` 同步等待第二池槽；或  
- 无 SC-3 自动化测试即默认 `CELLD_FETCH_LOOPBACK=1`。

**批准条件：**

- 完成 §4 Must-fix 1–7；设计文档 v0.2 修正 §5.2/§6 矛盾；PD 条目增加「安全门禁」勾选。

---

## 8. 参考实现触点（审查时快照）

| 组件 | 位置 | 备注 |
|------|------|------|
| `op_fetch` | `celld/crates/celld/js.rs` ~6502 | 无 URL 策略 |
| service binding 同源 | `celld/crates/celld/js/harness.js` ~2236–2250 | `svcDepth`、selfFetch 范本 |
| Ingress 契约 | `docs/plans/INGRESS-ROUTING.md` §4 | synthetic Host、trust forwarded |
| AD-1 | `docs/decisions.md` §2 | 1 celld / version |
| celld 威胁模型 | `celld/docs/security.md` | 非 hostile multi-tenant |

---

| 日期 | 变更 |
|------|------|
| 2026-09-02 | 初版敌对安全审查（Option A） |
