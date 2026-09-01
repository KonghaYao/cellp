# Ingress 路由方案（修订版）

> **状态：** 权威方案 · 已对抗审查（2026-08-31）· 实现中  
> **决策：** [decisions.md §17 AD-12](../decisions.md#17-ad-12--hostname--port-ingress废弃-path-选-version)  
> **说明：** 本文档为 Ingress 唯一设计入口（含对抗审查结论，见 §10）。

## 1. 问题陈述

| 现状 | 后果 |
|------|------|
| Gateway `/{project}/{version}/*` strip 后反代 | 破坏 SPA、`fetch('/api')`、OAuth、wrangler 根路径语义 |
| upstream `Host = 127.0.0.1:<celldPort>` | `request.url` origin 错误（`docs/support-migrations.md` §端口与路由） |
| path 区分 prod/preview | chi 隐式解析、绝对 URL、Cookie scope 与 CF 不一致 |

**决策：** **废弃 path 作为 version 选择器**；业务 path 从 `/` 起；选路键为 **Host（Tier A）** 或 **可选 listen port（Tier B，仅 dev 附录）**。

---

## 2. 设计原则

1. **Path 不 rewrite**：Gateway 将请求 path/query 原样转发至 celld。
2. **Baseline（默认）：单 Gateway listener + Host 路由**；dev 无 DNS 时用 **synthetic FQDN + `/etc/hosts` 一行**，而非默认「每 preview 动态 `Listen`」。
3. **Tier B 端口模式为 opt-in**：`CELLP_INGRESS_TIER_B=host|dedicated_port|external_map`（默认 **`host`**）。
4. **禁止**将浏览器/产品 URL 指向 celld upstream 口（8803+）；仅 Gateway（或外层 `external_map`）入口。
5. **AD-10 不变**：cellp 不写 DNS API、不签证书；TLS/WAF/Zero Trust 在外层。
6. **威胁模型（R-THREAT-GATEWAY）**：Gateway HTTP = 应用暴露面；`DEPLOY_TOKEN` **不**保护 Worker URL；生产须 Gateway 仅 LB 后端可达。

---

## 3. Registry

### 3.1 保留 `routes`

每 version：`upstream_host`、`upstream_port`、`active`（AD-1 不变）。

### 3.2 新增 `ingress_bindings`

| 列 | 说明 |
|----|------|
| `binding_id` | PK |
| `project_id` | |
| `version_id` | preview 必填；prod 可为 NULL（解析 `prod_version_id`） |
| `role` | `preview` \| `prod` |
| `host` | Tier A；规范化小写 FQDN；**全局唯一（active）** |
| `listen_port` | Tier B opt-in；**仅 `127.0.0.1`**；与 celld 端口池 **互斥** |
| `synthetic_host` | 发往 celld 的 `Host`；**全局唯一（active）** |
| `owner_gateway_id` | dedicated_port 模式：持有 listener 的 cellpd 实例 ID |
| `active` | bool |

约束：每条 binding 至少 `host` 或 `listen_port` 之一；`(host)`、`(synthetic_host)`、`(listen_port WHERE active)` 唯一。

**Projects：** 可选 `prod_host`；默认 `{project_id}.{CELLP_INGRESS_BASE_DOMAIN}`。

### 3.3 端口分配（互斥）

| purpose | 默认区间 | 说明 |
|---------|----------|------|
| `celld_upstream` | 8803–8999（相对 `CELLD_PORT`） | 现有 allocator |
| `ingress_listen` | **19080–19999** | Tier B dedicated_port；**禁止**与 RustFS 9000、celld 池重叠 |

启动时 **port conflict check**；冲突则 version 不得 `ready`。

### 3.4 `preview_url` 与 env

| 优先级 | 生成 |
|--------|------|
| 1 | 有 `host` → `{CELLP_PUBLIC_SCHEME_PREVIEW}://{host}/` |
| 2 | 仅 `listen_port` → `http://127.0.0.1:{port}/` |
| 3 | 模板覆盖 | `CELLP_PREVIEW_URL_TEMPLATE` |

**version `ready` 时** orchestrator 注入 deploy env：**`PUBLIC_BASE_URL` = `preview_url`**（与 support-migrations 命名一致）。prod 同理 **`PUBLIC_BASE_URL` = prod 对外 URL**。

---

## 4. Gateway 行为

### 4.1 解析顺序（normative）

```
localAddr := listener local port
if CELLP_INGRESS_TIER_B == dedicated_port && localAddr != GATEWAY_PORT:
    b := LookupByListenPort(localAddr)  // 仅 owner_gateway_id == self
else:
    h := normalize(Host)   // TRUST=0 时忽略客户端 X-Forwarded-Host
    if GATEWAY_TRUST_FORWARDED_HEADERS==1 && RemoteAddr in TRUSTED_CIDRS:
        h := normalize(last X-Forwarded-Host) ?? h
    b := LookupByHost(h)
if b == nil: 404 ingress_unknown
if b.role == prod: version := project.prod_version_id
else: version := b.version_id
route := GetRoute(project, version); if !route.active → 503
proxy(route.upstream)
```

**R-G-LOCALHOST：** `Host` 为 `localhost` / `127.0.0.1` 且无 Tier B 口时，**仅**匹配显式 `host` 绑定；preview **不得**仅靠裸 localhost 挂在 `:8787`。

### 4.2 反代头（硬契约）

| 头 | 值 |
|----|-----|
| 发往 celld `Host` | **`synthetic_host`**（禁止客户端 Host 直传） |
| `X-Forwarded-Host` | **客户端 authority**（Tier B：`127.0.0.1:<listen_port>` 含端口，直至 celld 支持 Port 头） |
| `X-Forwarded-Proto` | 对外 scheme（Tier A prod 默认 https；dev 可 http） |
| `Forwarded` | 可选；Gateway 重写，不信任客户端 |

celld：**`CELLD_TRUST_FORWARDED_HEADERS=1` 为 ready 门禁**；public listener 仅 loopback。

### 4.3 Listeners

| 模式 | 行为 |
|------|------|
| **host（默认）** | 仅 `:GATEWAY_PORT`；dev：`cellp dev ingress hosts-print` 输出 `/etc/hosts` 行 |
| **dedicated_port** | preview ready 时 `Listen 127.0.0.1:port`；archive 关 listener；**cellpd 启动 reconcile** active bindings |
| **external_map** | cellp 只写 Registry；外层 socat/nginx stream 映射端口并注入 Host |

### 4.4 WebSocket

Gateway 透传 `Upgrade` / `Connection`；Tier A 依赖外层 `wss` + `X-Forwarded-Proto: https`。

---

## 5. 生命周期与 AD-5 / AD-11

### 5.1 ready（事务内）

1. `SetRoute(active=true)`
2. `UpsertIngressBinding(preview)` + 分配 port（若 dedicated_port）
3. deploy celld + trust forwarded
4. 注入 `PUBLIC_BASE_URL`
5. `VerifyGatewayRoute` 使用 **API `preview_url` + 正确 Host**，非 path

### 5.2 archive

1. route draining / `active=false`
2. `binding.active=false`
3. 关 dedicated_port listener
4. stop celld

### 5.3 promote

- prod **Host 不变**；`CAS_prod` + upstream 切换 + **`InvalidateProd`**
- **preview `listen_port` / host 不变**；promote 不自动关闭旧 version preview URL
- AD-11 cron redeploy **在** prod 路由/cache 刷新 **之后** 对外完成

---

## 6. 配置

| 变量 | 默认 | 说明 |
|------|------|------|
| `GATEWAY_PORT` | 8787 | 单 listener baseline |
| `CELLP_INGRESS_TIER_B` | `host` | `host` \| `dedicated_port` \| `external_map` |
| `INGRESS_PORT_MIN/MAX` | 19080 / 19999 | dedicated_port 池 |
| `CELLP_INGRESS_BASE_DOMAIN` | `ingress.local` | synthetic / 默认 prod host |
| `CELLP_PUBLIC_SCHEME_PREVIEW` | `http` | |
| `CELLP_PUBLIC_SCHEME_PROD` | `https` | dev 可全站 http override |
| `GATEWAY_TRUST_FORWARDED_HEADERS` | `0` | |
| `GATEWAY_TRUSTED_PROXY_CIDRS` | 空 | TRUST=1 时必填 |

---

## 7. 外层设施（Tier A）

Nginx / HAProxy / K8s Ingress：`proxy_pass` → Gateway **无 path 前缀**；`proxy_set_header Host $host`；TLS 在外层。

---

## 8. 实施分期

| 阶段 | 内容 | 验收 |
|------|------|------|
| **P0** | Host 路由、Forwarded 契约、trust celld、`PUBLIC_BASE_URL`、去 upstream 假 Host | `request.url` origin 与 preview_url；`fetch('/api')` 200 |
| **P1** | prod_host、promote host 切流、VerifyGatewayRoute 改 API URL | 替代 `v4-promote-cutover` path 版 |
| **P2** | 删 path 路由；e2e/site；可选 dedicated_port | `run-all.sh` — **e2e MANIFEST 已 Host 化**；dev `INGRESS_HOST_ONLY=1`；**mock Gateway** 已按 `*.{base}` / `*.*.{base}` 反代 celld（path 不 strip；path 路由仅 `INGRESS_HOST_ONLY!=1` deprecated） |
| **P3** | `external_map` 文档、deep health listener 指标 | |

**过渡：** path 路由可保留 **一个 major** 并标 `Deprecated`，与 P0 同步改 `VerifyGatewayRoute`，避免 ready 门禁与 URL 分裂。

---

## 9. 强制规则清单（对抗审查 blocking）

| ID | 规则 |
|----|------|
| R-TRUST-0 | TRUST=0 时 binding 仅看 `Host`，不读 X-Forwarded-Host |
| R-TRUST-1 | TRUST=1 时须 TRUSTED_CIDRS；Gateway 重写 Forwarded，直连伪造无效 |
| R-BIND-LOOPBACK | listen_port 必须 bind 127.0.0.1；0.0.0.0 启动失败 |
| R-UPSTREAM-HOST | upstream Host 必须为 synthetic_host |
| R-SYNTH-UNIQUE | synthetic_host / host 全局唯一 |
| R-PORT-DISJOINT | ingress 池与 celld/RustFS 默认口不相交 |
| R-NO-HOST-AUTH | 文档引用 celld：禁止 Host 鉴权 |
| R-ARCHIVE-TEARDOWN | archived 后 port 不可达 |
| R-THREAT-GATEWAY | 威胁模型章节必选 |
| R-CELld-TRUST-CHAIN | trust forwarded + 仅 Gateway 连 celld |

---

## 10. 对抗审查摘要（2026-08-31）

三角色并行审查：**SRE** · **安全/AD-10** · **CF 迁移兼容**。

| 原草案 v0 问题 | 修订 |
|----------------|------|
| Tier B 默认每 preview `Listen` + 9000–9999 | **默认 Tier B=host**；端口池 **19080+**；dedicated_port opt-in |
| 9000 与 RustFS/celld 冲突 | 统一 **port_allocations purpose** |
| 多 cellpd `(listen_port)` 全局锁 | `owner_gateway_id` + 仅本机 reconcile |
| celld 不读 X-Forwarded-Port | **X-Forwarded-Host 带 :port** |
| 无 PUBLIC_BASE_URL | ready 时注入 env |
| Gateway 无 WS 契约 | §4.4 |
| AD-2 path 与 AD-10 §15.5 未更新 | **AD-12** + decisions 同步 |

**保留：** path 不 strip、Host/synthetic origin、promote 不换 prod Host。

---

## 11. 明确不做

- cellp 不 emulate wrangler 多 `routes` pattern（仍 strip/文档化）
- Tier B dedicated_port **非**多租户共享 CI 默认（需 network namespace 或 Tier A）
- 不宣称 preview/prod 同 cookie jar（不同 origin 为预期）
