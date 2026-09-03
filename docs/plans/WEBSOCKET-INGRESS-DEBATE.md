# WebSocket Ingress 设计辩论

> **设计权威：** [WEBSOCKET-INGRESS-DESIGN.md](./WEBSOCKET-INGRESS-DESIGN.md) **v0.2**（§0 Debate resolution 已并入）  
> **缺陷：** PD-20260903-07  
> **说明：** 原文针对 v0.1 压力测试，保留备查。决议：辩方分期/范围胜出；检方 MUST-FIX **1、4、7、13、14、15** 升为 WS-M1 硬门禁。

---

## Prosecutor

**立场：** Opus 计划（H1 → 方案 A → WS-M1 `wsecho` 关门）在**未满足下列 MUST-FIX 前不可视为安全、可运维、可关闭 PD-07 的实施规格**。静态代码与 `serve.go`/`listeners.go` 表明：即使 Hijacker 委托正确，仍有多条独立失败模式会被计划压到「M2/M3/Open Q」。

### MUST-FIX（实现前阻断，≤15）

1. **§6.1 四格证据必须先于任何 `gateway` 补丁** — 设计写「禁止同时改两层」，但仓库无 `docs/evidence/websocket-ingress-h1h2.md`；在直连 celld `wsecho`/`fx` 与 Gateway 对照完成前，H1 仍是假设而非结论。
2. **cellpd 优雅关闭与 Hijack 长连接** — `serve.go` 对 Gateway 使用裸 `http.Server` + `Shutdown`；`listeners.go` 专用口仅 5s shutdown。Hijack 后连接不受 `Handler` 返回约束，**无**「排空 WS / 发 Close 帧」策略；重启或 deploy 会硬断 DO 会话，与 CF 行为不一致且难归因。
3. **`MetricsMiddleware` 与 101 的语义** — 中间件在 `next.ServeHTTP` **返回后**才 `RecordGatewayRequest`；Hijack 成功后 handler 可能长期阻塞，**Prometheus 在握手完成前不记账**，且 `gateway_requests{code=101}` 与「仍存活的 WS 条数」脱钩；无 `active_websocket_connections` 则 FD/泄漏不可运维。
4. **每请求 `NewSingleHostReverseProxy` + 默认 `Transport`** — `proxyIngress` 每次新建 proxy，无 Upgrade 专用 `Transport`（`ResponseHeaderTimeout`、连接复用、TLS）。握手失败与上游慢响应仍统一 `502 bad gateway`，**ErrorHandler 不区分 dial / hijack / 上游 426**，与 PD 要求的可归因证据矛盾（§4.2.6 仅日志、无契约测试）。
5. **hop-by-hop 头透传未规格化** — 计划写「透传 `Upgrade`/`Connection`」，但未定义：客户端 `Connection: keep-alive, Upgrade`、重复 `Sec-WebSocket-Protocol`、非法 `Transfer-Encoding` 与 celld `forwards_worker_websocket_header` 过滤链的交互；**header smuggling / 双解析**风险在 Gateway→celld 两段 HTTP 栈之间，单测假上游 101 覆盖不了。
6. **`corsMiddleware` 污染 Upgrade 响应** — `cors.go` 对**所有**非 OPTIONS 请求预先 `Set` `Access-Control-*`；101 响应会带上与 WebSocket 无关的 CORS 头。M1 写「不拦 Origin」，但未要求 Upgrade 路径跳过 CORS 写头；跨站 + 子协议场景可能通过，但**规格与浏览器/CF 行为未验证**。
7. **Tier B / 外层 `external_map` 仍可能 502** — `ListenerManager` 复用同一 `gw.Handler()`（Hijacker 修复可覆盖 dedicated_port），但 **external_map** 由外层 stream 代理注入 Host；内层 Gateway 绿了不等于产品 URL 绿。Open Q3 标「一眼确认」，**不是** M1 交付物 — 与 G1「preview/prod Host」表述冲突。
8. **`clientAuthority` 与双层转发** — `applyUpstreamHeaders` 用 `r.Host` 写 `X-Forwarded-Host`，**不**走 `effectiveHost` 的 trusted-proxy 链；外层 LB 终止 TLS 且直连 Gateway 时，DO/Worker 看到的 `request.url` authority 可能与浏览器栏不一致，**H2-fx**（`idFromName` / session key）在 M1 后被误判为「已修 Gateway」。
9. **AD-9 与长 WS 生命周期** — `ModifyResponse` 在 101 时触发节流 `touchLastAccess` **一次**；会话持续数小时不再 touch，version 可被 archive 策略判空闲，**celld 仍在泵 WS**。计划把 draining/archive 交互推到 WS-M3，但 M2 关 PD-07 会对外宣称 agent 路径可用 — **运维与产品承诺错位**。
10. **workerd parity 不能靠 101 冒充「DO session 修好」** — compat **Partial**：子请求 WS 须 `accept()`、V8 heap >90% `acceptWebSocket` throw、outbound WS 在 `waitUntil` 后关闭、DO 迁节点 WS 不续。A04/fx 在 101 后仍可能随机断；G3 文案若进 release note 构成**过度承诺**。
11. **Gateway 连接放大（SSRF 类）** — 路由来自 registry `UpstreamHost:UpstreamPort`（loopback celld）；WS 使每次 Upgrade 占 **两条** TCP（客户端↔Gateway、Gateway↔celld）且无 per-version/per-IP 上限。恶意或故障客户端可占满 cellpd FD，**HTTP 无此驻留时间**；M1 无并发/速率门禁。
12. **方案 B 未与方案 A 同 PR 就绪** — 设计把专用 Hijack 泵标为 fallback，但若 M1 仅合并 A 并在生产/长会话暴露标准库边界 bug，**无回滚开关**。「对照实验后再 B」在组织上常变成永不 B。
13. **WS-M1 不进 `run-all.sh` + 无默认 `wsecho` fixture** — 单测 `httptest` 不能替代真实 `net.Conn` Hijack；合并后 CI 不部署 example 即**静默回归 502**，与「first-class ingress」目标不符。
14. **INGRESS-ROUTING §4.4 与实现分叉未随 M1 强制闭合** — 规格写「透传 Upgrade」，今日 502；Open Q8 把「补一句 Hijacker-safe」标为可选同步。**不更新 normative 文档就改代码**会再次制造「文档说行、代码不行」。
15. **PD-07 关闭门槛与里程碑脱节** — PD 严重度 **major**、用户路径是 A04 浏览器 TUI；计划允许 M1 后 PD 仍为 `mitigated`、仅 overlay。若沟通上把「wsecho 101」说成「WebSocket 已修复」，**违反** platform-defects-log 的「应有修复」语义。

---

### 论据展开（按攻击面）

#### 安全（SSRF、头走私、信任边界）

- **SSRF（受限但可放大）：** Gateway 仅 dial registry 中的上游，攻击面主要是**错误/被篡改路由**与**长时间占满连接**，而非任意 URL。WS 将风险从「短 HTTP」变为「小时级双跳 TCP」，计划无连接配额与告警（§4.4 metrics 仅有计数器类设想，无 MUST）。
- **Header smuggling / 双栈解析：** 客户端 `Connection`/`Upgrade`/`TE` 经 `ReverseProxy` 原样进 celld；celld 在 101 路径上另走 `fastwebsockets::upgrade` + Worker 头过滤（`websocket.rs`）。Gateway 不 strip、不规范化，与 Cloudflare 边缘行为不对齐；**任一层的宽松解析**都可能产生「Gateway 200/101、Worker 行为异常」类漏洞或会话固定问题。
- **转发头重写：** `proxy_headers.go` 删除客户端 `X-Forwarded-*` 再注入 — 对 celld `CELLD_TRUST_FORWARDED_HEADERS=1` 正确。但 `X-Forwarded-For` 仅来自 `r.RemoteAddr`（Gateway 所见 TCP），外层 `external_map`/LB 后**真实客户端 IP 丢失**；对依赖 IP 的 WS 鉴权（若 Worker 未来使用）是隐性缺口。
- **Ingress Host 信任：** `effectiveHost` 在 `TrustForwardedHeaders` + CIDR 下信任 `X-Forwarded-Host`；**WS Upgrade 与 HTTP 共用选路**。不可信客户端若直达 Gateway（非 AD-10 生产形态），与 HTTP 相同攻击面；计划未要求 WS 握手速率限制或 binding 级熔断。

#### 可运维性

- **502 不可区分：** 今日 A04 正文 `bad gateway` 与 `gateway.go` `ErrorHandler` 一致；修 Hijacker 后，上游 celld `unsupported WebSocket route`、dial 拒绝、握手超时仍可能统一 502（取决于失败点）。无 **结构化对外错误码** 时，on-call 仍靠猜。
- **Gateway/cellpd 重启：** Hijack 连接脱离正常 `Handler` 生命周期；`Shutdown` 不等待 WS 泵结束。专用口 `ListenerManager.shutdownEntryLocked` 5s 超时后强关 — 与 WS-M3「archive 时 1001」均未在 M1/M2 定义。
- **指标与 SLO：** `ModifyResponse` 记 `RecordGatewayUpstream(101)`，但无长连接存活、字节泵、异常断开原因；**G4**「Upgrade 成功/失败分计数」在 Hijack 路径上依赖未实现的 statusRecorder 行为，未写入 MUST-FIX 测试矩阵。
- **CORS / 浏览器运维：** P1 才处理跨站 `Origin`；M1 假设同源 `lvh.me`。Dashboard 或第三方页面连 preview Host 的失败会落在 WS-M2「查 Origin」，**无默认 deny/allow 列表**。

#### workerd / celld parity 缺口（握手后仍失败）

- 即使 Gateway **101**，A04 仍可能死于：**426/502 celld 正文**（Worker 未交 `webSocket`）、heap 90%、hibernation/`ws_auto_response` 路径、fx 专用头或 `?key=`。设计 §3 表最后一行（GW wsecho 101、GW fx 502）要求 M2，但**组织上 M1 合并即可能宣布「ingress WS done」** — 检方反对的是这种实施顺序，不是反对修 Hijacker。
- **子请求 Upgrade**（Nitro loopback Open Q3）与 ingress WS **故意分离**正确，但用户会把「站点 WS 不通」混为一谈；文档若不醒目，support 成本暴涨。

#### Gateway 连接泄漏与资源

- Hijack 成功后 `MetricsMiddleware` 在连接关闭前可能一直阻塞在 `proxy.ServeHTTP` — 这是预期，但 **goroutine + FD 无上限**。
- 每请求新建 `ReverseProxy`：无共享 `MaxIdleConns` 调优文档；对 WS 虽多为单请求单连接，但**异常路径**（hijack 失败、仅关闭一端）依赖标准库 defer；无 cellp 层集成测试。
- **双 listener：** `:8787` 与 dedicated_port 各一套 `http.Server`（`listeners.go:112`），同一 `Handler`；修复须验证**两入口** Hijack，否则 P5c 用户仅 port 模式仍挂。

#### Durable Object WebSocket 生命周期

- celld pump：`worker_websocket_task` / `websocket_task` / `peer_tunnel::splice`（设计 §2.3）。AD-1 单 celld 下 dev 无 peer_tunnel，但 **archive route → stop celld** 时 Gateway 可能仍持有客户端 WS；客户端见 RST，DO 状态未定义。
- **touchLastAccess** 仅握手时一次：长会话不刷新 last_access → **误 archive** 与「会话仍活跃」矛盾。
- compat：**DO outbound WS 不随 object 迁节点** — 非 dev 问题，但若 AD-10 分布式控制面延伸，Gateway 101 不能暗示 CF 语义。

#### 对「方案 A 足够」的反驳

- `cellp/internal/serve/serve.go` 中 Gateway `http.Server` **未设置** `ReadTimeout`/`WriteTimeout`（当前对长连接「看似友好」），但 **TLS 第二 listener** 同样裸配置；任何未来加 `WriteTimeout` 会无声砍 WS — Open Q1 标「实现前读」，**未绑定到 M1 PR 验收**。
- `api` 包另有 `statusRecorder`（`last_access.go`）也无 Hijacker — 虽非 ingress，说明**包装 Writer 丢接口是仓库模式**；只修 `metrics_middleware.go` 而不引入共享 `hijackRecorder` 工具，易再犯。
- **FlushInterval=-1** 解决缓冲观感，不解决 hijack 失败、不解决 cors 写头、不解决 shutdown。

#### 门禁与 PD-07

- WS-M1 以 `wsecho` 关门**不能**降低 PD-20260903-07 严重度；platform-defects-log 明确要求 **fx `GET /session` Upgrade → 101**。
- overlay `POST /api/prompt` 在 WS「部分绿」后易被删；设计写保留但**无 CI 断言** — 检方要求 WS-M2 前不得移除 overlay 测试步骤。

---

### 检方结论（对 Opus 计划）

Opus 计划的**正确部分**是：A04 现象与 `statusRecorder` 缺 `Hijacker`、`ErrorHandler` 文案与静态 `ReverseProxy` 行为一致，**值得作为 H1 第一假设**。但把「委托 Hijack + FlushInterval + wsecho 单测」标成可安全落地的 M1，**低估了**：(1) 证据与文档闭合的前置条件；(2) Hijack 后连接与 cellpd 生命周期的运维空洞；(3) celld Partial compat 与 PD 用户路径的差距；(4) Tier B / 外层代理 / 转发 authority 导致的 H2 误判；(5) 无 WS 连接治理时的 DoS 放大。

**建议（检方）：** 允许 M1 **仅**在 MUST-FIX 1、4、7、13、14 闭合后做最小 Hijacker 修复；**禁止**在 WS-M2（A04 101 + e2e）完成前将 PD-07 标为 `fixed` 或对外宣称 DO WebSocket 生产可用；方案 B 的代码骨架或与 A 同 feature flag 应在 M1 PR 中可切换。

---

## Defender

### 对 B1（归因未证）

- **设计已把 §6.1 设为任何代码改动的前置条件**（§3、§5 WS-M1 第 1 步、§9 实施顺序 1）。M1 实现与对照实验的合法顺序是：先填四格表（直连 wsecho / 直连 fx / GW wsecho / GW fx），再动单层。
- **静态证据已足够支撑 H1 为「第一假设」而非终局结论：** A04 仅 Upgrade **502** 且正文与 `gateway.go` `ErrorHandler` **逐字 `bad gateway`**；无 Upgrade **426** 说明 celld Worker 路由与 HTTP 反代正常；`metrics_middleware` 包装类型**未实现** `http.Hijacker`（§2.2）与 Go `ReverseProxy` 对 101 的要求直接冲突。
- **证伪路径写进设计：** 若去掉包装或正确委托后 Gateway 仍 502，且 celld 无 `websocket_connection_timing accepted`，才升级 **H2-platform**（§3 H1 证伪行）。这不是跳过实验，而是**实验表决定改 Gateway 还是 celld**，禁止同时改两层（§7 风险表、§3）。

### 对 B2（方案 A 过脆）

- **分层缓解：** M1 默认方案 A（§4.2）；**方案 B（专用 Hijack 泵）** 明确为「对照实验后标准库仍吞 101」的 fallback，不先上 `gorilla`、不默认改 `go.mod`（§4.2、§7）。
- **Open Q1 已列为 WS-M1 阻塞项：** 实现前读 cellpd `http.Server` 超时；若存在握手后砍连接的 `WriteTimeout`，M1 **必须**改为 `ReadHeaderTimeout` 等（§4.2.5、§8 Q1）——这不是 M3 才管的事。
- **半包装接口：** 单测强制 `statusRecorder` 在假上游 101 下可 Hijack（§6.2）；推荐 `Unwrap()` + `gateway_requests{code=101}`（§4.4）；`ErrorHandler` 对 Upgrade 打 structured log 区分 dial vs hijack（§4.2.6）。
- **`FlushInterval=-1`：** 针对缓冲/交互延迟（§7）；若仍不足，方案 B 的 `io.Copy` 双向泵是同一里程碑内的加固，不拖到「另开项目」。

### 对 B3（门禁错位 / PD-07）

- **里程碑故意拆分，不是搪塞：** WS-M1 范围**只改** `cellp/internal/gateway`，门禁是 **Gateway → wsecho 101**（§5 WS-M1）。目的：用最小 DO 控制组证明 **ingress 代理层**恢复 RFC6455，避免把 fx 产品头/Cookie/子协议问题误当成 Gateway bug（§3 组合表最后一行、§3 工作假设脚注）。
- **PD-07 关闭条件在设计 frontmatter 与 WS-M2 写死：** A04 `GET /session` Upgrade → **101**；更新 acceptance + 证据后 PD → `fixed`（§1 G3、§6.4、WS-M2 Gate）。M1 绿 **不**声称关闭 PD；G1/G2 与 G3 的 DO session **产品验收**在 WS-M2。
- **M2 清单已覆盖 fx 专用失败模式：** `Sec-WebSocket-Protocol`、`?key=`、`Origin`/混合内容、`idFromName` 与 `synthetic_host`（§4.3 M2 检查清单）——若「GW wsecho 101、GW fx 502」则走该行，**不是**回滚 M1。

### 对 B4（Compat 过度承诺）

- **G3 关闭条件是可观测契约，非 CF 全矩阵：** `wsecho` echo + A04 **101**（§1 G3）；**G5/G6** 与 hibernation、`getTags` 标为 **P1 / 本文件不关闭**（§1.2）；M1 **不**承诺 hibernation 全矩阵（§1.3）。
- **Partial 诚实留在 celld compat + WS-M3：** heap 90%、`getTags`、DO 迁移 WS → **M3 压测与文档**（§4.3 表、WS-M3）；**不作为**关闭 PD-07 条件（§5 WS-M3 Gate）。
- **A04 关闭不要求真实模型回合**（§8 Q7）：101 + 至少一帧或 xterm open 即可；模型仍依赖 `AI_GATEWAY_API_KEY`，与 WS 握手正交。

### 对 B5（运维 / archived）

- **M1 仍覆盖「握手后不被 HTTP 栈误杀」**（Server 超时，见上）；**连接寿命、并发 N、Gateway 重启、archive 时 503/1001** 明确归入 **WS-M3**（§5 WS-M3、G4 仅要求 101 后 **仍触发一次** 节流 `touchLastAccess`，§1 G4）。
- **AD-1 单 version 单 celld：** dev 不依赖 peer_tunnel 多节点 splice（§1.2 G6 OUT OF SCOPE M1–M2）；勿把 tunnel 失败误判为 Gateway（§8 Q6）。
- **Deferral 有 Gate：** WS-M3 要证据报告 + 无 FD 泄漏（§5）；不要求进 `run-all`，但与「能否握手」分离，避免用运维议题阻塞 **H1 502** 这一 major 根因。

### 对 B6（范围蠕变）

- **Non-goals 硬边界：** 不改 Nitro loopback（§1.3）；不拿 A03 SSE 当 PD 关闭条件（§7）；CORS 跨站 → **P1**，M1 同源 `lvh.me`（§4.2 CORS、§7）；外层 `wss` → **G7 / WS-M3 文档**（§1.2）。
- **INGRESS-ROUTING 分叉：** Open Q8 要求实现落地时补一句「Hijacker-safe 中间件」——辩方支持**随 M1 合并同步规格**，把技术债变成 M1 交付物之一，而非拒绝修 Gateway。
- **SSE 捎带：** `FlushInterval` 修复可能惠及长 HTTP；设计明确 **不**把 A03 并入本 PD 关闭条件（§7），控制 blast radius。

### 对 B7（Tier B 第二路径）

- **Open Q3 要求 M1 前一眼确认** dedicated_port / external_map 是否同 `proxyIngress`（§8 Q3）。若同源，Hijacker 修复**一次覆盖**；若是第二套 proxy，纳入 WS-M1 checklist（与 §6.1 同级），**不**允许只修 Host 路径——这是实现纪律，不是设计缺口。

### 对 B8（run-all 回归洞）

- **WS-M1：** `cd cellp && go test ./internal/gateway/...` + 手工 wsecho（§5、§6.5）——因 fixture 未默认进栈，**故意**不进 `run-all` 以免无 wsecho 时全红（§8 Q5）。
- **WS-M2：** `v1-websocket-ingress.sh` **可选进** `run-all`（§6.3、§5 WS-M2 Gate：`run-all` 不退化）。辩方：**M1 靠 Go 单测锁 Hijack 行为；M2 把 e2e 升为门禁**——时间换稳定性，与 PD 用户路径对齐。

### 对 B9（overlay）

- **设计多处 normative 保留：** §1.3、WS-M2 第 6 点、§6.4 表、`POST /api/prompt` **始终 200**；§7 风险表「overlay 被删」→ 缓解是**明确保留** + acceptance 表双轨（WS + HTTP）。
- **Enforcement 建议（不扩大 M1）：** WS-M2 更新 `support-framework-user-acceptance.md` A04 行时 **HTTP overlay 行保持 PASS**；`run-all` 若已有 A04 HTTP 步骤则继续跑。辩方接受检方诉求：可在 WS-M2 PR 模板加「不得删除 `/api/prompt`」——属于流程加固，不改变 M1 技术范围。

---

### 辩方总结

| 检方主题 | M1 内缓解 | defer M2/M3 |
|----------|-----------|-------------|
| H1 vs H2 | §6.1 四格 + 单层改动 | H2-fx / celld 表 §4.3 |
| 方案 A 不足 | 单测 + 超时审计 + 方案 B fallback | — |
| PD-07 vs wsecho | wsecho = 代理层控制组 | A04 101 + e2e = WS-M2 |
| Partial compat | 规格写清不关闭项 | hibernation/heap/tags = WS-M3 |
| 运维 / 长连接 | 101 + touchLastAccess | 压测、draining、wss 文档 = WS-M3 |
| 范围 / 双 proxy | Non-goals + Q3/Q8 | CORS / G7 = P1/M3 |
| 回归 / overlay | gateway 单测 | run-all e2e + acceptance 双轨 = M2 |

**结论：** Opus 计划的可辩护性在于 **证据优先、H1/H2 可证伪、M1 只恢复 Gateway Hijack 能力并以 wsecho 证伪通用代理，PD-07 与 fx 浏览器在 WS-M2 关门**；检方提出的脆性与运维缺口多数已在 §7 风险表与 WS-M3 中有对应 deferral，不应成为「不做 M1」的 blocker，而应成为 **M1 实现 checklist 与 M2/M3 Gate 的显式条目**。
