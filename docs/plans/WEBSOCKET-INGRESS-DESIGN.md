# WebSocket Ingress（Gateway → celld → Worker/DO）— 设计 v0.2

> **状态：** v0.2 工程计划 · **不实现代码**（本文件仅规格；辩论已并入）  
> **缺陷：** [platform-defects-log.md § PD-20260903-07](../platform-defects-log.md)  
> **Ingress 权威：** [INGRESS-ROUTING.md](./INGRESS-ROUTING.md) §4.4（已写「透传 `Upgrade` / `Connection`」；实现未兑现）  
> **辩论：** [WEBSOCKET-INGRESS-DEBATE.md](./WEBSOCKET-INGRESS-DEBATE.md)（v0.1 压力测试；结论见本文件 **§0**）  
> **决策：** [decisions.md](../decisions.md) AD-1 · AD-10 · AD-12  
> **Compat：** [celld/docs/cloudflare-compat.md](../../celld/docs/cloudflare-compat.md) WebSockets **Partial**  
> **产品缓解：** A04 overlay `POST /api/prompt`（[support-fx-on-workers README](../../dev/examples/support-fx-on-workers/README.md)）— **不**替代本计划  
> **验收（关闭 PD）：** `support-fx-on-workers` `GET /session` Upgrade → **101**；`e2e/` 或 `celld/examples/wsecho` — **仅 WS-M2**，见 §0 / §5

---

## 0. Debate resolution

来源：[WEBSOCKET-INGRESS-DEBATE.md](./WEBSOCKET-INGRESS-DEBATE.md)。v0.2 **合并辩方胜出的分期与范围**，把检方已接受条目升为 **WS-M1 硬门禁**（不得再压到「Open Q / 实现时再说」）。

### 0.1 辩方胜出（规范不变）

| 主题 | 决议 |
|------|------|
| **H1 第一假设 + 可证伪** | A04 仅 Upgrade **502** + 正文逐字 `bad gateway` + `statusRecorder` 无 `Hijacker` → H1 为第一假设，**不是**终局。§6.1 四格决定改哪一层；**禁止同时改 Gateway 与 celld**。 |
| **M1 只恢复代理层** | WS-M1 **只改** `cellp/internal/gateway`（+ 单测 / 证据 / 文档同步）。门禁是 **Gateway → `wsecho` 101 + echo**，用最小 DO 控制组证伪通用代理，避免把 fx 头/Cookie/子协议当成 Gateway bug。 |
| **PD-07 不在 M1 关闭** | frontmatter「关闭 PD」= **WS-M2**（A04 `GET /session` Upgrade → 101 + acceptance 证据）。M1 绿 **不得**把 PD 标 `fixed`，**不得**对外宣称「DO WebSocket 生产可用 / ingress WS done」。 |
| **方案 A 默认** | 先恢复标准库 `ReverseProxy` + `Hijacker`；**不**默认改 `go.mod` / 引入 `gorilla/websocket`。方案 B 是对照后标准库仍吞 101 时的 **同里程碑 fallback**，不是另开项目。 |
| **Partial compat 诚实** | G3 关闭条件是可观测 **101 + 泵一帧**，不是 CF 全矩阵。`getTags` / hibernation / heap 90% / DO 迁节点 WS → **P1 / WS-M3**，不作为关 PD 条件。 |
| **长连接运维后置** | 握手后不被 HTTP 栈误杀（Server 超时）属 M1；**排空 / Close 帧 / archive 交互 / `active_websocket_connections` / 并发配额 / 外层 `wss`** → **WS-M3**。G4 M1 仅要求 101 后节流 `touchLastAccess` **仍触发一次**。 |
| **范围硬边界** | 不改 Nitro isolate 内 `fetch` Upgrade；不拿 A03 SSE 关本 PD；CORS 跨站策略 → **P1**（M1 同源 `lvh.me` 不拦 `Origin`）；G6 多节点 splice OUT OF SCOPE M1–M2。 |
| **overlay 保留** | `POST /api/prompt` **始终**保留；WS「部分绿」**不得**删 overlay 测试步骤（WS-M2 PR 模板显式禁止）。 |
| **`run-all.sh` 时机** | M1 **不**把依赖 `wsecho` fixture 的脚本默认塞进 `run-all.sh`（无 fixture 则全红）。Hijack 行为由 Go 单测锁定；**脚本必须存在且可手工/CI 可选跑**（检方 13 的「有脚本」半接受，见 §0.2）。 |

### 0.2 检方已接受 → **WS-M1 硬要求**

下列条目在辩论中被辩方接受为「M1 checklist / 交付物」，或检方结论明确要求 **M1 合并前闭合**。实现 PR **缺任一项不得标 WS-M1 done**。

| ID | 检方 MUST-FIX | M1 要求（normative） |
|----|---------------|----------------------|
| **P-1** | §6.1 四格证据先于任何 `gateway` 补丁 | 合并代码前仓库须有 `docs/evidence/websocket-ingress-h1h2.md`（或 `.json`）：直连 wsecho / 直连 fx / GW wsecho / GW fx。无表 **禁止** 合入 Hijacker 补丁。 |
| **P-4** | 默认 `Transport` + 统一 502 不可归因 | `ErrorHandler` 对 Upgrade **structured log** 区分 **dial / hijack / 其它**；单测：**上游 dial 失败 → 502 `bad gateway`**；**假上游 426 → 正文透传、非 502**；**假上游 101 → 不进 ErrorHandler**。Upgrade 检测见 §4.2.3。可不引入独立对外错误码（避免泄 upstream）。 |
| **P-7** | Tier B / 外层绿 ≠ 产品 URL 绿 | M1 **前**确认 `:8787` 与 `dedicated_port` 是否同一 `gw.Handler()` / `proxyIngress`。同源则一次修复须 **两入口** 单测或手工 101；若第二套 proxy，纳入本里程碑，**禁止**只修 Host 路径。`external_map`：一眼确认「内层 Handler 覆盖」并写入证据；**不**把外层 stream 当 M1 产品关门（G1 M1 = Gateway 入口，见 §1.1 注）。 |
| **P-13** | 仅 `httptest` + 不进 `run-all` 会静默回归 | （1）Go 单测必须用 **可 Hijack 的 `net.Conn` stub**（包装后 `ReverseProxy` 能 `Hijack()`），不能只断言接口存在；（2）落地 `e2e/scripts/v1-websocket-ingress.sh`（Host → Gateway → wsecho：101 + 至少一文本帧）；**默认不**加入 `run-all.sh`；（3）文档写明如何部署 `celld/examples/wsecho` fixture。无（1）（2）不得关 M1。 |
| **P-14** | INGRESS-ROUTING 与实现分叉 | **同一 PR** 更新 [INGRESS-ROUTING.md](./INGRESS-ROUTING.md) §4.4：Gateway 中间件必须 **Hijacker-safe**（`statusRecorder` 委托 `Hijack`/`Flush`/`Unwrap`）；透传 `Upgrade`/`Connection`/`Sec-WebSocket-*`。禁止「文档说行、代码不行」。 |
| **P-15** | PD 关闭门槛与里程碑脱节 | M1 changelog / PR / release note **禁止**写「WebSocket 已修复 / PD-07 fixed」。PD 保持 `mitigated`（overlay）直至 WS-M2。严重度 **不**因 wsecho 101 下调。 |

**检方结论中「方案 B 同 PR 可切换」：** 接受为 **书面回滚**，不强制未证伪就合入泵代码。M1 PR 必须写清：方案 A 对照失败 → 同里程碑切 §4.2 方案 B（独立 `proxyWebSocket` 或等价 flag）。A 的单测绿且 §6.1 GW wsecho=101 时，**不必**预合并 B 实现。

### 0.3 检方提出、明确 **不**进 M1（defer）

| ID | 主题 | 归入 |
|----|------|------|
| P-2 | Hijack 后 `Shutdown` 无排空 / Close 帧 | WS-M3（503 draining vs 1001） |
| P-3 | `gateway_requests` 在 handler 返回后才记账；缺 active gauge | M1：握手路径须能记到 **101**（`ModifyResponse` 与/或 recorder 在 Hijack 前记 status）；**`active_websocket_connections` / 字节泵** → WS-M3 |
| P-5 | hop-by-hop 全矩阵 + header smuggling 形式化 | M1 **最小规范**见 §4.2.3 / §4.2.7；双栈 fuzz / CF 对齐 → P1 |
| P-6 | 101 带 CORS 头 | M1 不拦 Origin、不因 CORS 拒绝 Upgrade；**剥离 101 上 `Access-Control-*`** → P1 |
| P-8 | `X-Forwarded-Host` 用 `r.Host` 而非 `effectiveHost` | 与 HTTP 同路径；H2-fx / `idFromName` 误判 → **WS-M2 检查清单**，不单开 WS 转发语义 |
| P-9 | 长会话不再 `touchLastAccess` → 误 archive | WS-M3；M1 只保证 101 **一次** touch |
| P-10 | 101 ≠ workerd / DO session 全修好 | 规格诚实（§1.2 / §2.4）；发布话术见 P-15 |
| P-11 | 双跳 TCP 无 per-version/IP 上限 | WS-M3 配额 / 告警 |
| P-12 | 方案 B 必须与 A 同 PR 落地代码 | 降为 §0.2 书面回滚（辩方胜） |

---

## 1. Goals / Non-goals

### 1.1 Goals（P0）

| ID | 目标 | 关闭条件 |
|----|------|----------|
| G1 | **First-class ingress WebSocket** 经 cellp Gateway（AD-12 Host 选 version，path 不 rewrite） | 浏览器 / `curl` / 任意 RFC6455 客户端对 **Gateway 产品入口**（`:8787` Host；若启用 dedicated_port 则同 Handler）做 Upgrade，得到 **101**，帧可双向泵。**注：** `external_map` 外层 stream **不是** G1/M1 关门条件（P-7）。 |
| G2 | 区分 **H1 Gateway** vs **H2 celld**（见 §3）；先证伪再改两层 | 对比实验表（§6.1）写入 `docs/evidence/`；502 正文可归因（P-1、P-4） |
| G3 | Durable Object session 通路：Worker `fetch` → `stub.fetch` → `WebSocketPair` + `acceptWebSocket` | **WS-M1：** `wsecho` 经 Gateway echo。**WS-M2：** A04 `GET /session` **101**（不再 `bad gateway`）。G3 **产品**关闭 = M2，**不是** M1。 |
| G4 | 可观测：Upgrade 成功/失败与普通 HTTP 分计数；长连接不饿死 `touchLastAccess` | M1：101 可计数（recorder 与/或 `ModifyResponse`）+ 节流 last-access 在 101 后仍触发一次。存活条数 gauge → M3。 |

### 1.2 Goals（P1，本文件不关闭）

| ID | 目标 | 备注 |
|----|------|------|
| G5 | Hibernatable WebSockets（`getTags` / auto-response / wake）经 Gateway 不退化 | celld 已有 `ws_auto_response`；compat 仍 Partial |
| G6 | 跨节点 owner / peer_tunnel splice 经 Gateway 稳定 | AD-1 单 version 单 celld 下 dev **不需要**；多节点 OUT OF SCOPE for M1–M2 |
| G7 | 外层 `wss` + `X-Forwarded-Proto: https`（INGRESS-ROUTING §4.4 Tier A） | AD-10：cellp **不**终止 TLS |

### 1.3 Non-goals

- **不**用 A04 HTTP overlay 替代上游 fx TUI；overlay 保留作自动化门禁。
- **不**做账号 / DNS / CDN / TLS / WAF（AD-10）。
- **不**把浏览器 URL 指到 celld 上游口（8803+）；直连仅作 **根因实验**，产品入口仍是 Gateway。
- **不**在 M1 承诺 Cloudflare Agents SDK / hibernation 全矩阵 / `getTags()`。
- **不**在本计划修 Nitro 同源 fetch loopback（见 [NITRO-CELLD-LOOPBACK-DESIGN.md](./NITRO-CELLD-LOOPBACK-DESIGN.md) Open Q3）；ingress WS ≠ isolate 内 `fetch` Upgrade。
- **不**改冻结 D1 RPC 契约；**不**引入 Caddy / 外部云对象存储。
- **不**在 Dashboard（`web/`）直连 `:8792` 或 offshoot。
- **不**因 M1 `wsecho` 101 宣称 PD-07 已修或下调严重度。

---

## 2. Current architecture

### 2.1 路径（应有 vs 今日）

```
Browser / curl
    │  Host: support-fx-on-workers.lvh.me
    │  GET /session  Upgrade: websocket
    ▼
cellp Gateway :8787          AD-12 Host → ingress_bindings → routes
    corsMiddleware
    MetricsMiddleware        ← statusRecorder 包装 ResponseWriter（见 H1）
    handleIngress            Host 解析、route.active
    proxyIngress             httputil.NewSingleHostReverseProxy(http://upstream:port)
    │  applyUpstreamHeaders  Host=synthetic_host；X-Forwarded-*
    ▼
celld public listener        AD-1 每 ready version 一进程
    is_upgrade_request?      celld/crates/celld/main.rs ~2608
        yes → handle_websocket (websocket.rs)
        no  → handle_ingress
    ▼
isolate Worker fetch
    optional stub.fetch(DO)
    WebSocketPair + accept() / state.acceptWebSocket()
    Response 101 + webSocket
    ▼
celld pump                    worker_websocket_task | websocket_task | peer_tunnel::splice
```

A04 已观测：

| 请求 | 结果 | 含义 |
|------|------|------|
| `GET /?key=` | **200** HTML | Host 路由、Worker、静态/TUI 页 OK |
| `GET /session` **无** Upgrade | **426** `expected websocket` | Worker/`FxSession` 路由 OK；HTTP 反代 OK |
| `GET /session` **有** Upgrade | **502** `bad gateway` | 与 Gateway `ErrorHandler` 正文逐字一致（`gateway.go`） |

缓解：`POST /api/prompt` overlay。目标：同一 Host 上 WS 成为一等公民，供 DO / agents（fx TUI、CF Agents 实时通道）。

### 2.2 Gateway（`cellp/internal/gateway/gateway.go`）

`proxyIngress`（约 L190–218）：

- `httputil.NewSingleHostReverseProxy(target)`，`target = http://{UpstreamHost}:{UpstreamPort}`。
- `Director`：保留 path/query；`applyUpstreamHeaders`（`proxy_headers.go`）重写 `Host` → `synthetic_host`，注入 `X-Forwarded-Host/Proto/For`、`Forwarded`。
- `ModifyResponse`：记 `RecordGatewayUpstream`；2xx–4xx 节流 `TouchLastAccess`。
- `ErrorHandler`：一律 **`502` + 正文 `bad gateway`**（与 PD / A04 对齐）。
- **未**设置 `FlushInterval`；**未**自定义 `Transport`；**无** Upgrade 专用分支。

Go 标准库 `ReverseProxy` **可以**在 `ResponseWriter` 实现 `http.Hijacker` 时透传 101 并 splice 连接。今日中间件破坏了该前提。

`Handler()` 链：

```
corsMiddleware(MetricsMiddleware(router))
```

- `cors.go`：只在 `OPTIONS` 短路；**不**包装 `ResponseWriter`（对 Hijack 无害）。`Allow-Methods: GET, OPTIONS` 对浏览器 **非**简单跨域 WS 可能不够（P1 / Open Q）。非 OPTIONS 会预先 `Set` `Access-Control-*`（101 可能带上无关 CORS 头 — P-6，P1 清理）。
- `metrics_middleware.go`：`statusRecorder` **只 embed** `http.ResponseWriter`，实现 `WriteHeader` / `Write`。  
  **未**实现 `http.Hijacker`、`http.Flusher`、`http.Pusher`。  
  `ReverseProxy` 对 101 会 `Hijack()`；包装类型无该方法 → 代理失败 → `ErrorHandler` → **502 `bad gateway`**。

这是 H1 的最强静态证据。INGRESS-ROUTING §4.4 的「透传 Upgrade」在代码层未落地。

`cellp/internal/api` 另有 `statusRecorder`（`last_access.go`）同样无 Hijacker — **非** ingress 路径，但说明包装丢接口是仓库模式。M1 在 gateway 包内修复（推荐 `Unwrap()`）；**不**强制本里程碑抽共享 `hijackRecorder`（避免范围膨胀）。后续包装 Writer 须复制 Hijack 委托。

### 2.3 celld ingress WS（已存在，Partial）

入口（`celld/crates/celld/main.rs` ~2606–2610）：runtime 在且 `fastwebsockets::upgrade::is_upgrade_request` → `handle_websocket`。

`handle_websocket`（`websocket.rs` ~779+）：

1. `fastwebsockets::upgrade::upgrade` 失败 → **400** `ws upgrade: …`
2. `request_payload`（含 `trust_forwarded_headers`、body 上限）失败 → 对应 HTTP 错
3. `runtime.fetch_worker_pool(...)` 失败 → **500** `Worker failed`
4. Worker 响应 **无** `websocket` → 原样返回 Worker 状态（fx 无 Upgrade 时的 **426**）
5. 有 socket 但 `status != 101` → **502** `unsupported WebSocket route`（celld 正文，**不是** Gateway 的 `bad gateway`）
6. Worker 应用头经 `forwards_worker_websocket_header` 过滤后并入 101（剥离 `Connection`/`Upgrade`/`Sec-WebSocket-*` 等，对齐 kj `acceptWebSocket`）
7. 后台 task：`Worker` → `worker_websocket_task`；本地 DO cell → `websocket_task`；`target.tunnel` → `peer_tunnel::splice`

另有：outbound WS、hibernation 短路径 `ws_auto_response`、拒绝已 `accept` 但未升级时的 `reject_accepted_websocket`（1006）。

参考实现：`celld/examples/wsecho` — DO `W` + `WebSocketPair` + `state.acceptWebSocket` + echo/`count` storage。与 fx `FxSession` 同构、表面更小。

### 2.4 Compat 缺口（WebSockets = Partial）

摘自 `celld/docs/cloudflare-compat.md`：

- `getTags()` 不可用
- 子请求 Upgrade 的 socket **必须** `accept()`
- outbound Worker socket 在 response + `waitUntil` 结束后关闭
- 非 101 拒绝升级
- 升级响应去掉 Worker 提供的 protocol / connection 头
- outbound 升级合并同名重复头
- `acceptWebSocket()` 在 isolate V8 heap **>90%** 时 throw
- DO：outbound WS **不**在 object 迁到另一节点后继续

这些是 **H2 / M3** 清单，不是 A04 502 的第一解释。**不得**用 Gateway 101 冒充上述矩阵已齐（P-10）。

---

## 3. Root-cause hypotheses

先做对比实验（§6.1），**禁止**同时改 Gateway 与 celld。

### H1 — Gateway 未正确代理 Upgrade（优先）

| | |
|--|--|
| **命题** | 客户端 → `:8787` 的 Upgrade 在到达 celld 之前失败；或 101 无法 Hijack，落入 `ErrorHandler`。 |
| **机制** | `statusRecorder` 丢失 `Hijacker`（主因）。次因：默认 `Transport` / 缓冲、`FlushInterval==0`、中间件抢写 header、超时切断。 |
| **预测** | Gateway 正文 **`bad gateway`**；**直连 celld 上游端口** 同一 `Host`/`synthetic_host` Upgrade → **101**。 |
| **已有证据** | A04：HTTP 200/426 通、仅 Upgrade 502；`ErrorHandler` 文案匹配；metrics 包装类型无 Hijack。 |
| **证伪** | 去掉包装或实现 `Hijacker` 后 Gateway 仍 502，且 celld access/timing 日志无 `websocket_connection_timing accepted`。 |

### H2 — celld Worker / DO WS 缺口

| | |
|--|--|
| **命题** | Upgrade 已进 celld，但 Worker 未交出 `webSocket`、状态非 101、或 DO `acceptWebSocket` / pair 失败。 |
| **机制** | `handle_websocket` 分支 4–5；fx 只认浏览器式头；`synthetic_host` / forwarded 导致 `request.url` 与 DO id 不一致；heap 90% throw。 |
| **预测** | **直连 celld 也失败**；正文为 **426** / **`unsupported WebSocket route`** / **400 `ws upgrade`** / **500**，而非 Gateway `bad gateway`。 |
| **已有证据** | 弱：无 Upgrade 时 426 说明路由在；compat Partial；fx 比 `wsecho` 更重。 |
| **证伪** | 直连 celld `wsecho` **与** fx `/session` 均为 101，仅经 Gateway 502。 |

### 组合与排序

```
1. 直连 celld wsecho Upgrade
2. 直连 celld fx /session Upgrade
3. Gateway → 同 version wsecho
4. Gateway → fx /session
```

| 直连 wsecho | 直连 fx | GW wsecho | GW fx | 结论 |
|-------------|---------|-----------|-------|------|
| 101 | 101 | 502 | 502 | **H1**；M1 只改 Gateway |
| 101 | 非 101 | 101 | 非 101 | **H2-fx**；celld/app 缺口，Gateway 已通 |
| 非 101 | 非 101 | 502/非 101 | 同 | **H2-platform**；先修 celld |
| 101 | 101 | 101 | 502 | 头/cookie/key/CORS 或 fx 专用；非通用代理 |

**工作假设（待 P-1 证据表确认或证伪）：** A04 = **H1**。H2 用 `wsecho` 作控制组，避免把 fx 产品问题当成运行时缺口。

---

## 4. Proposed design

### 4.1 原则

1. Path 不 rewrite；选路仍 AD-12 Host / opt-in port。
2. 发往 celld 的 `Host` 仍是 **`synthetic_host`**；客户端 authority 只走 `X-Forwarded-*`（现有硬契约）。
3. Gateway **透传** hop-by-hop 升级头，**不**自己当 WS 端点（不解码帧、不 `acceptWebSocket`）。
4. 产品 URL 只暴露 Gateway（或 AD-10 外层 `wss`）。
5. 最小改动优先：先恢复标准库 Hijack 能力；专用代理仅在 H1 证伪「包装不够」之后。

### 4.2 Gateway WS 代理

#### 方案 A（M1 默认）— 修复 `ReverseProxy` + `Hijacker`

在现有 `proxyIngress` 上：

1. **`statusRecorder` 实现 `http.Hijacker` / `http.Flusher`**（可选 `http.Pusher`）：  
   `Hijack()` 委托给内层；内层不是 Hijacker → 明确 error（便于日志，仍可能 502，但应在修对后不再触发）。  
   推荐 `Unwrap() http.ResponseWriter` 以便 `http.NewResponseController`。
2. **`proxy.FlushInterval = -1`**：立即刷 101 与后续帧（SSE/A03 长连接可同受益，但不在本 PD 关闭条件）。
3. **Upgrade 检测**（normative）：  
   `Connection` 含 `upgrade`（comma-list、case-insensitive；允许 `keep-alive, Upgrade`）且 `Upgrade` 含 `websocket`（case-insensitive）。  
   仅打日志 / metrics 标签，**不**另开 listener。
4. **`ModifyResponse`：** 101 计一次 upstream 101 + `touchLastAccessThrottled`（今日条件是 `< 500`，101 已满足；确认 metrics 不把 101 当错误）。Hijack 后 `MetricsMiddleware` 可能在连接关闭才 `RecordGatewayRequest`：M1 允许「记账偏晚」，但 **101 必须最终可观测**（upstream 计数与/或握手时点一次 status）。
5. **超时：** `ReverseProxy` 默认用 `http.DefaultTransport` 的 `ResponseHeaderTimeout` 等。Upgrade 成功后连接已 Hijack，与 HTTP client timeout 脱钩。**M1 阻塞：** 读 cellpd 外层 `http.Server`（`serve.go`、`listeners.go` 含 TLS 第二 listener）。若存在握手后砍长连接的全局 `WriteTimeout`，必须对 Upgrade 豁免或改为 **`ReadHeaderTimeout` only**。今日若未设 Read/WriteTimeout，记录进证据，并注释「未来加 WriteTimeout 须豁免 WS」。
6. **错误正文（P-4）：** `ErrorHandler` 对 Upgrade 打 structured log（`method`/`path`/`host` + 错误类：`dial` \| `hijack` \| `other`），对外仍 502 `bad gateway`，避免泄漏 upstream。上游 **已返回非 101**（如 426）**不得**经 ErrorHandler 改写成 502。
7. **头透传（P-5 最小集）：** Director **不得** `Del` `Upgrade`、`Connection`、`Sec-WebSocket-Key`、`Sec-WebSocket-Version`、`Sec-WebSocket-Protocol`、`Sec-WebSocket-Extensions`。`Connection: keep-alive, Upgrade` 原样或规范为含 `upgrade` 的 token 列表（两者 celld `is_upgrade_request` 须仍为真）。重复 `Sec-WebSocket-Protocol`：原样转发。非法 `Transfer-Encoding`：不在 M1 发明第二套过滤器；依赖 Go 服务端解析。Worker→客户端 101 头仍由 celld `forwards_worker_websocket_header` 过滤。

**不**在 M1 引入 `gorilla/websocket`，除非方案 A 在对照实验后仍失败。

#### 方案 B（M1 fallback）— 专用 Hijack 泵

若标准库在自定义 `Director` 下仍吞 101：

1. `proxyIngress` 对 Upgrade 走 `proxyWebSocket`：`http.NewRequest` 复制头（保留 `Sec-WebSocket-Key/Version/Protocol/Extensions`、`Connection`、`Upgrade`），`applyUpstreamHeaders`，`net.Dial` 上游 HTTP/1.1。
2. 写请求、读响应；若 101：`Hijack` 客户端，`io.Copy` 双向 + idle deadline。
3. 非 101：把状态与 body 原样写回（426/404/500 不得变成 Gateway 502）。

依赖：标准库 `net/http` Hijack 即可；`gorilla/websocket` 仅当需要严格扩展协商时再引入（**避免无必要改 `go.mod`**）。

M1 PR 描述须含「A 失败 → B」回滚段（§0.2）。**不必**在 A 已绿时预合并 B。

#### CORS / 浏览器

同源 `ws://support-*.lvh.me:8787` **不**走 CORS。跨站页面连 preview Host 时，浏览器 WS 握手会带 `Origin`。M1：不拦 `Origin`、不因 CORS 中间件拒绝 Upgrade。P1：与 `cors.go` 对齐；评估 101 是否应跳过预写 `Access-Control-*`（P-6）。今日 `Allow-Headers` 无 `Sec-WebSocket-*`，一般无妨（浏览器不把这些当自定义头）。

### 4.3 celld 缺口（仅当 H2 或 M2/M3）

对照 compat + `handle_websocket`：

| 缺口 | 层 | 何时做 |
|------|----|--------|
| ingress 已实现 101 + DO pump | — | M1 验证，不重写 |
| Gateway 头被 `is_upgrade_request` 拒（缺 Key/Version） | 客户端或 Gateway 剥头 | M1 回归：Director **不得**剥升级头（§4.2.7） |
| `unsupported WebSocket route`（有 socket 但 status≠101） | Worker 契约 | 文档 + fx 若返回 200+pair |
| `getTags` / hibernation 标签 | compat Partial | M3 |
| 子请求 Upgrade 必须 `accept()` | 应用 | 文档；agents SDK |
| heap >90% `acceptWebSocket` throw | 运行时 | M3 压测 |
| outbound WS + DO 迁移 | 多节点 | OUT OF SCOPE |
| 同源 `fetch` Upgrade loopback | Nitro 计划 Open Q | **本计划不做** |

M2（fx / agents）检查清单（**先实验**）：

- `FxSession` 是否要求特定 `Sec-WebSocket-Protocol`
- `?key=` 是否必须出现在 Upgrade URL（A04 426 路径已带 key）
- Cookie / `Authorization` 是否被 CORS 中间件或 Director 丢掉（今日 Director 不应删）
- DO `idFromName` 是否依赖 `request.url` origin（`synthetic_host` vs `X-Forwarded-Host`）（P-8）

### 4.4 指标与日志

| 信号 | 来源 | 用途 |
|------|------|------|
| `gateway_requests{code=101}` | 修复后的 `statusRecorder`（或握手时点补记） | M1 gate |
| `gateway_upstream{code=101\|502}` | `ModifyResponse` / `ErrorHandler` | 对照 |
| ErrorHandler `class=dial\|hijack\|other` | 新增 log | P-4 |
| celld `websocket_connection_timing` `outcome=accepted\|rejected\|worker_error` | 已有 | H1/H2 |
| `active_websocket_connections` | 未做 | WS-M3（P-3） |

---

## 5. Phased milestones

本计划的 M1–M3 **≠** 仓库 test-plan 全局 M1/M2/M3。下文称 **WS-M1..3**。

### WS-M1 — Gateway 101（H1）

**范围：** 只改 `cellp/internal/gateway`（+ 单测 + §6.1 证据 + INGRESS-ROUTING §4.4 + e2e 脚本文件）。不改 celld submodule，除非对照证明 H2-platform。

**做：**

1. **P-1：** §6.1 对照实验，证据写入 `docs/evidence/websocket-ingress-h1h2.md`（或 `.json`）。**先于** gateway 补丁合并。
2. **P-7：** 确认 `:8787` / dedicated_port / `external_map` 与 `proxyIngress` 关系；写入同一证据文件。
3. 读 `http.Server` 超时（`serve.go`、`listeners.go`）；按 §4.2.5 豁免或记录「当前无 WriteTimeout」。
4. 方案 A：`Hijacker`/`Flusher`/`Unwrap` + `FlushInterval=-1` + Upgrade 日志 + ErrorHandler 分类（P-4）。
5. 单测：见 §6.2（含可 Hijack `net.Conn`）。
6. **P-13：** 新增 `e2e/scripts/v1-websocket-ingress.sh`；文档化 `wsecho` fixture。脚本 **不**默认进 `run-all.sh`。
7. **P-14：** 同步 INGRESS-ROUTING §4.4 Hijacker-safe 一句。
8. 手工或脚本：`wsecho` 经 Gateway **101 + 一帧 echo**（`:8787`；dedicated_port 若启用则同测）。
9. PR 话术遵守 P-15；写明方案 B 回滚条件。

**Gate（全部必须）：**

| # | 断言 |
|---|------|
| M1-T1 | `docs/evidence/websocket-ingress-h1h2.md` 四格 + Tier B 结论存在 |
| M1-T2 | Gateway → `wsecho`：**101** + 至少一文本帧 echo |
| M1-T3 | 非 Upgrade `GET /session` 仍 **426**（无回归） |
| M1-T4 | HTTP TUI `GET /?key=` 仍 **200** |
| M1-T5 | `cd cellp && go test ./internal/gateway/...`（§6.2 全表） |
| M1-T6 | `e2e/scripts/v1-websocket-ingress.sh` 存在且在有 fixture 时对 wsecho 绿（**未**加入 `run-all.sh`） |
| M1-T7 | INGRESS-ROUTING §4.4 已写 Hijacker-safe / 头透传 |
| M1-T8 | Upgrade 失败路径：dial → 502 `bad gateway` + log class；上游 426 正文透传 |
| M1-T9 | PD-07 仍为 `mitigated`；PR 未宣称 fixed |

**不做：** fx 浏览器 TUI 关闭；hibernation；改 overlay；关 PD-07；`active_websocket` gauge；Shutdown 排空；改 `go.mod`（除非已切 B 且标准库不够）。

### WS-M2 — DO session / agents（H2-fx 或纯产品）

**范围：** 在 WS-M1 绿之后。

**做：**

1. A04 `GET /session?key=…` Upgrade → **101**；xterm 不再永续 `connecting…`。
2. 若直连 101、Gateway 101、仅浏览器失败：查 `Origin` / 子协议 / 混合内容（`ws:` vs `https:` 页）。
3. 若直连 fx 非 101：对照 `wsecho` 与 `FxSession`；必要时 celld 补 DO fetch Upgrade 路径（保持 compat 注释诚实）。
4. e2e：`v1-websocket-ingress.sh` + 可选 A04（无模型也可只验 101）；**此时**可加入 `run-all.sh`（须默认能拿到 wsecho 或跳过策略，且 `run-all` 不退化）。
5. 更新 `support-framework-user-acceptance.md` A04、`support-matrix` 诚实句；PD-07 → `fixed`（需证据）。
6. overlay `POST /api/prompt` **保留**；acceptance **HTTP overlay 行保持 PASS**；PR 检查「不得删除 `/api/prompt`」。

**Gate：**

| # | 断言 |
|---|------|
| M2-T1 | A04 表「终端 WebSocket」**PASS**（`GET /session` Upgrade **101**） |
| M2-T2 | A04 `POST /api/prompt` 仍 200（overlay 未删） |
| M2-T3 | `e2e/scripts/run-all.sh` 不退化 |
| M2-T4 | PD-07 → `fixed` 且证据在 `docs/evidence/` |
| M2-T5 | 不要求真实模型回合（§8 Q7）：101 + 至少一帧或 xterm open |

### WS-M3 — 压测 / hibernation / 运维

**范围：** 长连接寿命，非「能否握手」。

**做：**

- 并发 N 条 WS（`stress/` 新脚本或 phase 附录）；idle 心跳；Gateway 重启 / version archive 时连接行为（503 draining vs 1001）（P-2、P-9）。
- `active_websocket_connections` 与异常断开原因（P-3）；可选 per-version 连接上限（P-11）。
- hibernation：`wsecho` 式 `webSocketMessage` + storage；auto-response 不 wake（celld 已有）经 Gateway 仍成立。
- `getTags` 等 compat 仍可 Partial；不作为关闭 PD 的条件。
- 外层 `wss` 文档（dev：`ws://*.lvh.me:8787`；prod：LB 终止 TLS）。
- hop-by-hop / CORS-on-101 形式化（P-5、P-6）若仍有客户。

**Gate：** 证据报告 + 无 FD 泄漏；不要求进 `run-all.sh`。

---

## 6. Test plan

### 6.1 对照实验（任何代码改动前 · P-1）

前置：`./dev/scripts/health.sh`；记下 fx / 临时 `wsecho` version 的 `upstream_port`（Registry / orchestrator 日志；**不要**手改 `dev/data/`）。

**curl 握手（期望 101 时看状态行与 `Sec-WebSocket-Accept`）：**

```bash
# Gateway（产品入口）
curl -i -N \
  -H "Host: support-fx-on-workers.lvh.me" \
  -H "Connection: Upgrade" \
  -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" \
  -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
  "http://127.0.0.1:8787/session?key=cellp-dev-fx-on-workers"

# 直连 celld（仅诊断；Host 必须是 synthetic_host，与 applyUpstreamHeaders 一致）
curl -i -N \
  -H "Host: <synthetic_host>" \
  -H "Connection: Upgrade" \
  -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" \
  -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
  "http://127.0.0.1:<upstream_port>/session?key=cellp-dev-fx-on-workers"
```

记录：HTTP 状态、**正文**（`bad gateway` vs `unsupported WebSocket route` vs `expected websocket` vs `ws upgrade:`）、celld 是否打 `websocket_connection_timing`。

`wsecho`：部署 example 到本地栈（或 `celld` 单进程），对 `/` 做同样两组 curl；再用 websocat / 小 JS 发一文本帧，期望 JSON `{echo, count}`。

**证据文件必须含：** §3 四格表填值 + P-7（Handler 是否共用）+ Server 超时现状（§4.2.5）。

### 6.2 单元（WS-M1）

| 测 | 断言 |
|----|------|
| `statusRecorder` Hijack | **可 Hijack 的 `net.Conn` stub**（非仅类型断言）；内层可被 `ReverseProxy` 调用 `Hijack()` |
| `Unwrap` / Flusher | 包装后 `http.NewResponseController` 或不panic Flush |
| 假上游 101 | `proxyIngress` 返回 101，**不**走 `ErrorHandler` |
| 假上游 426 | 正文透传，非 502 |
| 上游 dial 失败 | 502 `bad gateway`；log/测试可区分 class=`dial`（或等价） |
| Hijack 失败（内层非 Hijacker） | 不静默；structured class=`hijack` |
| `Connection: keep-alive, Upgrade` | 仍识别为 Upgrade；上游仍收到升级头 |
| Director 不剥 `Sec-WebSocket-*` | 假上游看到 Key/Version/Protocol |
| 非 Upgrade GET | 行为与现网单测一致 |
| metrics | 101 路径最终可观测为成功而非 5xx（允许记账在连接结束） |

`cd cellp && go test ./internal/gateway/...`

**两入口（P-7）：** 若 `ListenerManager` 复用 `gw.Handler()`，单测 Handler 一次即可，证据写「同源」。若否，为第二入口补等价用例。

### 6.3 e2e / example

| 套件 | 作用 | 阶段 |
|------|------|------|
| `celld/examples/wsecho` | DO + `acceptWebSocket` 控制组 | WS-M1 fixture（须文档化部署） |
| `e2e/scripts/v1-websocket-ingress.sh` | Host → Gateway → wsecho：101 + echo | **WS-M1 必须存在**；**WS-M2** 再考虑进 `run-all.sh` |
| A04 | 见下 | WS-M2 关 PD |

### 6.4 A04（`support-fx-on-workers`）

| 步骤 | 期望 | 阶段 |
|------|------|------|
| `GET /?key=` | 200 HTML | 回归（已绿）；M1-T4 |
| `GET /session?key=` 无 Upgrade | 426 `expected websocket` | 回归；M1-T3 |
| `GET /session?key=` Upgrade | **101** | **WS-M2 关闭 PD-07**（M1 不要求） |
| `POST /api/prompt` | 200 overlay | **始终保留**（M2-T2） |
| 浏览器 TUI | xterm 非永续 connecting | WS-M2 手工 |

证据：更新 [support-framework-user-acceptance.md](../support-framework-user-acceptance.md) A04 行；`mkdir -p docs/evidence`。

### 6.5 验证顺序（落地后，与仓库惯例对齐）

```bash
./dev/scripts/up.sh && ./dev/scripts/health.sh
cd cellp && go test ./internal/gateway/...
# WS-M1：有 wsecho fixture 时
# bash e2e/scripts/v1-websocket-ingress.sh
# WS-M2：脚本可加入 run-all 后再跑
./e2e/scripts/run-all.sh
```

不跑无关 D1 专项，除非误改 orchestrator。

---

## 7. Risks

| 风险 | 影响 | 缓解 |
|------|------|------|
| 只修 fx、不修 Hijacker | 下一 agent 再 502 | WS-M1 以 `wsecho` 为门禁 |
| 同时改 Gateway + celld | 无法归因 | 分里程碑；先证据（P-1） |
| 全局 `WriteTimeout` 砍 TUI | 101 后数秒断 | Server 用 `ReadHeaderTimeout`；idle 在泵上设 |
| `ReverseProxy` 缓冲二进制帧 | 交互延迟 / 粘包观感 | `FlushInterval=-1`；方案 B splice |
| 101 不触发 last-access | archived 误判空闲 | 确认 `<500` 含 101；长会话 touch → M3 |
| 包装类型半实现 Hijack | 偶发 502 | 单测强制可 Hijack conn；`Unwrap` |
| 直连 celld 被当成产品入口 | 绕过 AD-12 / 错误 `request.url` | 文档标明诊断-only |
| CORS 过窄 / 101 带 CORS 头 | 跨站 dashboard；与 CF 观感差 | P1；M1 同源 lvh.me |
| hibernation / 90% heap | M2 绿、生产掉线 | WS-M3；compat 保持 Partial；话术 P-15 |
| 改 `go.mod` 拉 gorilla | 违反默认禁止 | 方案 A 不引入；方案 B 先标准库 |
| A03 SSE 与本 PD 耦合 | 范围膨胀 | Flush 修复可能捎带 SSE；**不**把 A03 当关闭条件 |
| overlay 被删 | 无模型/无 WS 时失去自动化 | 明确保留 `/api/prompt`（M2-T2） |
| 只修 `:8787`、dedicated_port 仍 502 | P5c 用户仍挂 | P-7 两入口 |
| M1 被宣传为 PD 已修 | 违反 defects-log「应有修复」 | P-15 |
| Hijack 后无上限双跳 TCP | FD 耗尽 | WS-M3（P-11）；M1 不阻塞握手修复 |
| `Shutdown` 硬断 DO 会话 | 重启难归因 | WS-M3（P-2） |

---

## 8. Open questions

| # | 问题 | v0.2 状态 |
|---|------|-----------|
| 1 | cellpd `http.Server` 超时现状？ | **M1 阻塞审计**（实现时读代码并写入 P-1 证据）。若有 `WriteTimeout`，M1 必须改。 |
| 2 | `Sec-WebSocket-Protocol` 是否原样转发？ | **已决议（M1）：** 方案 A **必须**转发。fx / Agents 选用记入 M2 证据。 |
| 3 | dedicated_port / external_map 是否同 `proxyIngress`？ | **已决议（P-7）：** M1 前一眼确认；同源一次覆盖；第二套 proxy 进 M1；external_map 外层不关 G1/M1。 |
| 4 | 同源 isolate `fetch` Upgrade 是否共享夹具？ | **否**（保持）。避免与 Nitro 计划互相阻塞。 |
| 5 | e2e 是否进 `run-all.sh`？ | **已决议：** M1 必须有脚本 + 单测，**不**进 `run-all`；WS-M2 再进（有 fixture/跳过策略）。 |
| 6 | 多节点 peer_tunnel WS 是否出现在本地单 celld？ | 保持：AD-1 下一 version 一进程，dev 应为 local `websocket_task`。勿把 tunnel 失败误判为 Gateway。 |
| 7 | A04 关闭是否要求真实模型回合？ | **否：** 101 + 至少一帧（或 xterm open）即可关 PD-07；模型仍依赖 `AI_GATEWAY_API_KEY`。 |
| 8 | 是否更新 INGRESS-ROUTING §4.4？ | **已决议（P-14）：** M1 **同一 PR** 同步 Hijacker-safe。 |
| 9 | Gateway 对外 `ws:` vs 文档站 `wss:`？ | 产品文档（`site/`）在 **WS-M2** 浏览器绿之后再改，避免先承诺。 |

---

## 9. 建议实施顺序（实现阶段，非本规格 PR）

1. 跑 §6.1 + P-7 + 超时审计，填 `docs/evidence/websocket-ingress-h1h2.md`。  
2. 若 H1：方案 A（`metrics_middleware.go` + `FlushInterval` + ErrorHandler class + §6.2）。  
3. 加 `v1-websocket-ingress.sh`；同步 INGRESS-ROUTING §4.4。  
4. Gateway `wsecho` 101 + M1-T1..T9 → **WS-M1 关闭**（PD 仍 mitigated）。  
5. A04 curl 101 → 浏览器 TUI → overlay 仍绿 → **WS-M2**；更新 PD / acceptance。  
6. 再对抗审查本文件后才动 celld；禁止未经审查改冻结契约。

---

## 10. 变更（v0.1 → v0.2）

| 项 | 变化 |
|----|------|
| §0 | 新增 Debate resolution：辩方胜出表 + 检方 P-1/4/7/13/14/15 为 M1 硬门禁 + defer 表 |
| G1/G3/G4 | 收窄 M1 vs M2 关闭语义；external_map 不关 M1 |
| §4.2 | ErrorHandler class；头透传最小集；方案 B = 书面回滚非强制同 PR 代码 |
| §5–§6 | 里程碑验收改为 M1-T1..T9 / M2-T1..T5；e2e 脚本升为 M1 交付、默认仍不进 `run-all` |
| §8 | Q2/Q3/Q5/Q8 标为已决议 |
| 话术 | 明确禁止 M1 关闭 PD-07 |

*v0.2 · 2026-09-02 · 计划 only · 辩论并入 · 对应 PD-20260903-07*
)
