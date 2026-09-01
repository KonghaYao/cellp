# Ingress Port P5c 实现计划（Gateway Listeners + prod_port + 最小 e2e / web）

> **状态：** 实施计划 · 只读产出（本文件不写产品代码）  
> **权威设计：** [INGRESS-PORT-DEPLOYMENT.md](./INGRESS-PORT-DEPLOYMENT.md) §5–§6、§8–§9 · [INGRESS-ROUTING.md](./INGRESS-ROUTING.md) §4.3  
> **前置：** [INGRESS-PORT-P5b-impl-plan.md](./INGRESS-PORT-P5b-impl-plan.md) 已落地（registry/orch 台账与 URL）；[INGRESS-PORT-P5b-review.md](./INGRESS-PORT-P5b-review.md) **pass**（Gateway Listen **defer → P5c**）  
> **代码入口：** `cellp/internal/gateway/` · `cellp/internal/serve/serve.go` · `cellp/internal/orch/ingress.go` · `web/src/lib/format.ts` · `e2e/scripts/`

---

## 1. 目标与边界

### 1.1 P5c 要交付什么

补齐 Tier B **物理 listener** 与 **prod_port 混合模式** 的运行时闭环，使 P5b 已写入的 `listen_port` / 台账在 TCP 层可达，并完成 R-ARCHIVE-TEARDOWN 的「port 不可达」全义（与 P5b 台账 Detach 衔接）。

| 能力 | 说明 |
|------|------|
| **ReconcileListeners** | cellpd 启动时 + binding/台账变更后，对本机 `gateway_id` 匹配的 **active** `ingress_listen` 分配执行 `Listen 127.0.0.1:<port>`，共享现有 `Gateway.Handler()` |
| **关 listener** | Detach / archive / binding `active=false` 或台账释放后，reconcile **关闭** 对应 `http.Server`，使 `curl 127.0.0.1:port` 连接失败（**R-ARCHIVE-TEARDOWN** Listen 半部） |
| **§5.5 启动 reconcile** | 孤儿台账（binding 非 active）→ **ReleasePort** + log；**R-PORT-OWNER**：非本机 `gateway_id` **不得** Listen |
| **prod_port** | preview 仍 **Host @ `:GATEWAY_PORT`**（P5b 已如此）；prod **stable** `127.0.0.1:<prod_listen_port>` listener + 解析 |
| **解析修正** | 请求落在 **非主 Gateway 口** 时，按 `LookupIngressByListenPort(localPort, selfGatewayID)` 选 binding（覆盖 **prod_port** 在全局 `CELLP_INGRESS_TIER_B=host` 时仍有 prod 专用口的情况） |
| **e2e** | `dedicated_port`：`preview_url` → `curl` **200**；**可选** promote 前后 prod port 不变 |
| **web（最小）** | `format.ts`：API `preview_url` / `prod_url` 若 port ≠ `gatewayPublicPort()`，**原样使用绝对 URL**，禁止重拼为 `:8787` Host 代理 |

### 1.2 明确不在 P5c（defer）

| 阶段 | 内容 | 原因 |
|------|------|------|
| **P5d** | OpenAPI `PATCH` project、`prod_host` 列、Dashboard 全量 TP-UI | 设计 §7 / §9 |
| **P5e** | `celld_upstream` 迁入同一台账、统一 bind 探针 | 设计 §3.3 / §9 |
| **P5c 非必** | `GET /v1/platform/ports`、多 cellpd 跨实例 listener 迁移 | 设计 §7 / §5.5 多实例仅「不 Listen」 |
| **P5c 非必** | `run-all.sh` 默认启用 port e2e | 仍 Host 回归为主；port 脚本 **opt-in**（`INGRESS_PORT_E2E=1` 或单独点名） |
| **P5c 非必** | promote prod port e2e | 设计 §9 P5c 验收第二行；可文档 + 单测代替 |

**P5c 仍应：** Host 默认路径与现有 `run-all.sh` **零行为变化**（无 `listen_port` 时不增 listener）。

---

## 2. 现状对照（P5b 之后）

| 组件 | 现状 | P5c 缺口 |
|------|------|----------|
| `serve.go` | 单 `http.Server` @ `0.0.0.0:GATEWAY_PORT` | 无 per-port `127.0.0.1` server；启动无 ReconcileListeners |
| `gateway/ingress_resolve.go` | Tier B 仅当 **全局** `IngressTierB == dedicated_port` 且 `localPort != GatewayPort` | **prod_port** / 项目级 tier 与全局不一致时，专用口请求可能误走 Host 解析 |
| `listenerLocalPort` | 测试用 `WithLocalListenPort`；生产请求依赖 `r.Context` 注入 | 专用 listener 须在 accept 链注入 **真实 local port**（见 §4.2） |
| `registry` | `ListActivePortAllocations(ingress_listen)`、`LookupIngressByListenPort` | 无 gateway 侧消费 |
| `orch` | Attach/Detach 后无通知 Gateway | 需 **Reconcile 钩子** 或等价机制（§4.4） |
| `invalidating_store` | 缓存失效 on Upsert/SetRoute | **不**关 listener |
| `runtime.VerifyGatewayRoutePreviewURL` | 已用 API authority | P5c 后应对 dedicated 口 **200** |
| `web/format.ts` | `previewBrowseUrl` / `prodBrowseUrl` 走 `gatewayBrowseUrl(host)` | loopback `:19xxx` 会被拼成 Host @ 8787 |
| e2e | P5b 计划中的 `v1-ingress-port-preview.sh` **未实现** | P5c 实现并 opt-in |

---

## 3. 设计对齐（§5–§9 摘要）

### 3.1 Reconcile  desired set（normative）

对 `gateway_id == cfg.GatewayID` 且 `purpose=ingress_listen` 且 `released_at IS NULL` 的每条台账 `pa`：

1. `b := GetIngressBinding(ctx, pa.owner_id)`（`owner_kind=ingress_binding`）。
2. **若** `b != nil && b.active && b.listen_port == pa.port` → desired listeners 包含 `127.0.0.1:pa.port`。
3. **否则**（binding 缺失 / inactive / 口不一致）→ **不 Listen**；若台账仍 active → §5.5 **ReleasePort**（`release_reason=orphan_reconcile`）+ 结构化 log。

**禁止：** 对 `pa.gateway_id != self` 的台账在本机 `Listen`（**R-PORT-OWNER**）。

### 3.2 生命周期挂钩（§5.2 step 5、§5.3 step 3）

| 事件 | Orchestrator / Registry | Gateway |
|------|-------------------------|---------|
| Version **ready**（Attach ephemeral / prod stable） | 台账 + binding 已写（P5b） | **ReconcileListeners** → 新开 preview/prod 口 |
| **Archive** / compensate **Detach** | Detach + `active=false`（P5b） | Reconcile → **Close** 该口 |
| **Promote** | prod `listen_port` 不变（P5b） | Reconcile **无新口**；仅 cache/upstream 变 |
| **cellpd 启动** | — | Reconcile **before** 或 **紧接** 主 Gateway Listen（见 §4.1） |
| **Wake** | 新 ephemeral Attach | Reconcile 开新 preview 口 |

### 3.3 prod_port 模式（设计 §2、§4.1、§5.2）

| 流量 | 选路 | Listener |
|------|------|----------|
| Preview | Host @ `:GATEWAY_PORT`（与 `host` 相同） | **仅**主 Gateway 口 |
| Prod | `127.0.0.1:<stable_prod_port>` | **Dedicated** `127.0.0.1:prod_listen_port` |

**API：** `preview_url` = Host 形态；`prod_url` = `http://127.0.0.1:{port}/`（P5b）。**e2e：** preview 用 Host/`__cellp_host` 或 Gateway Host；prod 用 loopback port。

### 3.4 强制规则（P5c 责任）

| ID | P5c |
|----|-----|
| R-PORT-OWNER | Reconcile 过滤 `gateway_id` |
| R-BIND-LOOPBACK | `Listen("127.0.0.1:port")` only |
| R-ARCHIVE-TEARDOWN | TCP 不可达 + P5b 台账释放 |
| R-PROD-PORT-STABLE | Reconcile **不**因 promote 改 prod 口（无 orch 变更则 desired set 不变） |

---

## 4. Gateway 实现要点

### 4.1 组件：`ListenerManager`（建议 `gateway/listeners.go`）

```
type ListenerManager struct {
    mu       sync.Mutex
    gw       *Gateway
    servers  map[int]*http.Server   // key = listen port
    store    registry.Store
    cfg      GatewayConfig
}
```

| 方法 | 行为 |
|------|------|
| `ReconcileListeners(ctx context.Context) error` | 计算 desired 集合 vs `servers`；**open** 缺失口；**Shutdown** 多余口；处理 orphan 台账 |
| `CloseAll(ctx context.Context)` | cellpd 退出时与 `serve.shutdownAll` 一并调用 |

**启动顺序（`serve.Run`）：**

1. `gw := gateway.New(...)`（现有）
2. `lm := gateway.NewListenerManager(gw, store, gw.Config())`
3. `lm.ReconcileListeners(ctx)` — 失败则 **fail-fast** 或 log+继续（**建议 fail-fast** 若 bind 冲突）
4. 启动主 `gwServer` @ `GATEWAY_PORT`（现有）
5. Reconcile 已为每个 dedicated 口启动 goroutine `ListenAndServe`（`127.0.0.1:port`）

**Bind 冲突：** 若 `Listen` 失败且非「已在 map 中」，log `port` + `allocation_id`；ready 路径应已被 P5b 分配逻辑避免重复（§3.3 探针仍 **defer P5e**）。

### 4.2 Handler 与 local port 注入

专用口 server 使用 **同一** `gw.Handler()`，但在最外层 middleware（或 `http.Server` 的 `BaseContext` / 自定义 `ConnContext`）设置：

- `r = r.WithContext(WithLocalListenPort(ctx, port))`

使 `listenerLocalPort(r)` 在生产环境返回 **该 dedicated 口的 port**（现有测试 helper 复用）。

**主 Gateway 口：** 可不注入（`localPort==0` 或 `== GatewayPort`）→ 走 Host 解析。

**解析逻辑调整（`ingress_resolve.go`）：**

```
localPort := listenerLocalPort(r)
if localPort != 0 && localPort != g.cfg.GatewayPort {
    return LookupIngressByListenPort(ctx, localPort, g.cfg.GatewayID)
}
// else Host path (existing)
```

与 [INGRESS-ROUTING §4.1](./INGRESS-ROUTING.md#41-解析顺序normative) 对齐：**物理 listener 口** 优先于 Host；不再仅依赖全局 `CELLP_INGRESS_TIER_B=dedicated_port`（否则 **prod_port** 在全局 `host` 时 prod 专用口无法解析）。

**Tier B 请求 Host：** `X-Forwarded-Host` = `127.0.0.1:<listen_port>`（§4.2 已有 `proxy_headers` 方向；P5c 实现时核对 dedicated 路径）。

### 4.3 Reconcile 算法（伪代码）

```
active := ListActivePortAllocations(ingress_listen)
desired := map[int]struct{}{}

for _, pa := range active {
    if pa.GatewayID == nil || *pa.GatewayID != cfg.GatewayID { continue }
    b := GetIngressBinding(pa.OwnerID)
    if b != nil && b.Active && b.ListenPort != nil && *b.ListenPort == pa.Port {
        desired[pa.Port] = struct{}{}
    } else if pa 仍 active {
        ReleasePort(..., reason=orphan_reconcile)
    }
}

for port in desired \ servers.keys(): startServer(127.0.0.1:port)
for port in servers.keys \ desired: shutdownServer(port)
```

**中间态：** 设计 **R-PORT-LEDGER-REVERSE** 允许 reconcile 中间态 ≤30s；orch 在 **同事务** Attach 后调用 Reconcile 可缩短窗口。

### 4.4 Orchestrator → Gateway 钩子

**推荐（最小耦合）：**

1. 定义小接口 `type IngressListenerReconciler interface { ReconcileIngressListeners(ctx context.Context) error }`，由 `ListenerManager` 实现。
2. `serve.Run`：`o.SetIngressListenerReconciler(lm)`（或 `orch.New(..., lm)` 可选依赖）。
3. 在以下路径 **defer-safe** 调用（错误 log，**不** rollback 已提交 DB；下次 reconcile 修复）：
   - `runDeploy`：`ensurePreviewIngress` 成功且 preview 使用 port 之后、`VerifyGatewayRoute*` **之前**
   - `teardownPreviewIngress` / `compensateDeploy`：Detach 之后
   - `ensureProdIngress`：新 Attach stable 之后
   - `archive` Wake：`ensurePreviewIngress` 之后

**备选（若避免 orch→gateway 依赖）：** `invalidating_store` 在 `AttachIngressListenPort` / `DetachIngressListenPort` / `UpsertIngressBinding` 带 `listen_port` 变更时回调；**缺点** registry 层感知 Gateway。**不推荐** 除非 orch 注入不可行。

**兜底：** 可选 env `CELLP_INGRESS_LISTENER_RECONCILE_INTERVAL`（默认 **0**=仅事件驱动）；非 0 时 ticker Reconcile（与 fleet reconciler 分离）。P5c **可仅事件+启动**，periodic 为 **nice-to-have**。

### 4.5 Shutdown

`serve.shutdownAll`：`lm.CloseAll(ctx)` **先于** 主 `gwServer.Shutdown`，避免 dedicated 口仍接受连接。

---

## 5. prod_port 实现清单

| 项 | 位置 | 动作 |
|----|------|------|
| 项目 tier `prod_port` | 已有 `orch/ingress.go` | **无** preview Attach；prod **Attach stable**（P5b） |
| Listener | Reconcile | 仅 prod binding 的 `listen_port` 出现在 desired |
| Preview verify | `orchestrator.go` | 继续 `VerifyGatewayRouteHost`（非 preview URL loopback） |
| Prod verify | 可选 P5c | `curl prod_url` 200（promote 后）；可放 e2e optional |
| Gateway 解析 | §4.2 | prod 请求走 dedicated listener + `LookupIngressByListenPort` |
| 文档 | `docs/evidence/ingress-port-p5c.md` | 记录 `ingress_tier_b=prod_port` 手动矩阵 |

**Create project + `prod_listen_port`：** Reconcile 在 `ensureProdIngress` 后应已 Listen stable 口（项目创建路径需确认调用 reconciler）。

---

## 6. web：`format.ts`（最小，设计 §6）

**规则：** 若 API 提供的 `preview_url` / `prod_url` 解析成功，且 `URL.port` 非空，且 `parseInt(port) !== gatewayPublicPort()` → **返回 trim 后的绝对 URL**（pathname 来自 API 或 `pathOverride`），**不**调用 `gatewayBrowseUrl` / `withGatewayPortIfNeeded`。

**建议改动点：**

| 函数 | 行为 |
|------|------|
| 新增 `absoluteIngressUrlIfDedicated(apiUrl, pathOverride?)` | 上述判定 |
| `previewBrowseUrl` | API URL 为 loopback 专用口时 early return |
| `prodBrowseUrl` | 同上（已有部分 `prodUrl` 分支，扩展 port 判定） |
| `ingressDisplayUrl` | 展示字符串与 browse 一致（用户复制 URL） |

**测试：** `web/src/lib/format.test.ts` 增加：

- `previewBrowseUrl(..., "http://127.0.0.1:19081/")` → 仍为 `19081`，不含 `:8787`
- Host @ gateway 行为 **不变**

**范围：** 不改动 commerce iframe / 非 ingress 逻辑；**不**引入 P5d 的 PATCH UI。

---

## 7. 测试矩阵

### 7.1 单元 / 包测试（P5c 必做）

| ID | 场景 | 位置建议 |
|----|------|----------|
| L1 | Reconcile：一条 active 台账 + active binding → `Listen` 被调用 / 测试用 `httptest` 或 mock net | `gateway/listeners_test.go` |
| L2 | Reconcile：Detach 后台账释放 / binding inactive → server 从 map 移除 | 同上 |
| L3 | Reconcile：orphan 台账（无 active binding）→ `ReleasePort` | 同上 + registry mock |
| L4 | **R-PORT-OWNER**：`gateway_id=other` 不 Listen | 同上 |
| L5 | `resolveIngressBinding`：`localPort=19081` + prod binding → prod 路由 | `ingress_host_test.go` / 新用例 |
| L6 | `WithLocalListenPort` + 集成 handler 200 | 现有模式扩展 |
| W1 | `format.ts` dedicated port URL | `format.test.ts` |

### 7.2 e2e（P5c）

**脚本：** `e2e/scripts/v1-ingress-port-preview.sh`

```bash
# 前置：./dev/scripts/health.sh
# 必需：CELLPD_INSTANCE_ID 与 dev 栈 cellpd 一致（见 dev/.env 或 health 输出）
export CELLP_INGRESS_TIER_B=dedicated_port
# 或项目级：创建时 ingress_tier_b=dedicated_port

# 1. 部署版本（复用 ve-cd-loop 片段或 API）
# 2. GET /v1/.../versions/{id} → PREVIEW_URL
# 3. port=$(python/node 解析 URL port)
# 4. curl -sf "http://127.0.0.1:${port}/"   # 期望 200（无需 Host 头，Tier B 口选 binding）
# 5. archive → curl 同一 port 期望 **连接失败** 或 非 200（与 v15 语义一致）
```

**lib-ingress.sh 增量：**

- `preview_url_port "$json_or_url"`  
- `curl_preview_url "$preview_url" [path]`  
- `wait_http_200_preview_url "$preview_url"`

**门禁策略：**

| 入口 | 行为 |
|------|------|
| `./e2e/scripts/run-all.sh` | **默认不加入**（或 `INGRESS_PORT_E2E=1` 才跑） |
| CI / 手动 | `INGRESS_PORT_E2E=1 bash e2e/scripts/v1-ingress-port-preview.sh` |
| 证据 | `docs/evidence/ingress-port-p5c.md` |

**可选脚本：** `e2e/scripts/v1-ingress-port-promote.sh`

- 项目 `ingress_tier_b=prod_port` + `prod_listen_port`（或 dedicated_port 全 port）
- promote 前 `PROD_PORT=$(jq prod_url)`，promote 后再读，**断言 port 相同**；`curl prod_url` 200 且 body/version 切换

---

## 8. 文件级任务清单

| 文件 | 变更 |
|------|------|
| `cellp/internal/gateway/listeners.go` | **新建** ListenerManager + Reconcile |
| `cellp/internal/gateway/ingress_resolve.go` | localPort 优先 Listen 解析 |
| `cellp/internal/gateway/gateway.go` | 暴露 `Config()`、可选 `SetListenerManager` |
| `cellp/internal/serve/serve.go` | 启动/关闭 Reconcile；注入 orch |
| `cellp/internal/orch/orchestrator.go` | 可选 reconciler 字段 + 调用点 |
| `cellp/internal/orch/ingress.go` | （若调用点集中于此）Detach/Attach 后 reconcile |
| `web/src/lib/format.ts` | 绝对 URL 信任 |
| `web/src/lib/format.test.ts` | W1 |
| `e2e/scripts/v1-ingress-port-preview.sh` | **新建** |
| `e2e/scripts/lib-ingress.sh` | helpers |
| `docs/evidence/ingress-port-p5c.md` | **新建**（跑通后填） |

**不改（除非 bug）：** `docs/plans/D1-*-RPC.md` 冻结契约；`celld/` submodule。

---

## 9. 验收命令

```bash
# 1. Gateway 包
cd cellp && go test ./internal/gateway/... -count=1

# 2. 全量 Go（回归）
cd cellp && go test ./...

# 3. 本地栈
./dev/scripts/up.sh && ./dev/scripts/health.sh

# 4. Port e2e（P5c 点名）
export INGRESS_PORT_E2E=1
export CELLP_INGRESS_TIER_B=dedicated_port
bash e2e/scripts/v1-ingress-port-preview.sh

# 5. Host 回归（必须仍绿）
./e2e/scripts/run-all.sh

# 6. Web
cd web && npm test -- src/lib/format.test.ts
```

**设计 §9 P5c 验收行对照：**

| 验收项 | 命令 / 证据 |
|--------|-------------|
| dedicated_port preview `127.0.0.1:port` 200 | `v1-ingress-port-preview.sh` |
| promote 前后 prod port 不变、upstream 变 | 可选 `v1-ingress-port-promote.sh` 或 manual + evidence |
| archive 后 port 不可达 | preview 脚本 step 5 或 `v15-archive` + port 项目 |

---

## 10. 风险与决策点（P5c PR 前确认）

| 项 | 建议 |
|----|------|
| Reconcile 失败 vs ready | ready 路径：**Reconcile 失败 → verify 失败 → compensate**（与 P5b Detach 一致） |
| 双 goroutine Listen 同一口 | `ListenerManager` mutex + reconcile 幂等；已存在则 skip |
| `ingress_resolve` 与 INGRESS-ROUTING 文档 | 实现 **localPort ≠ GATEWAY_PORT** 分支后，在 DEPLOYMENT 或 ROUTING 加 footnote：**prod_port 依赖物理 listener，不依赖全局 tier 字符串** |
| orch 循环依赖 | reconciler 接口在 `orch` 包定义，`gateway` 实现，**serve**  wiring |
| Windows dev | `127.0.0.1` listen 与 Linux/mac 一致；e2e 仍 bash |
| bind 探针 | 仍 defer P5e；P5c 依赖 OS `Listen` 错误暴露冲突 |

---

## 11. 参考索引

- 生命周期 normative：[INGRESS-PORT-DEPLOYMENT.md §5](./INGRESS-PORT-DEPLOYMENT.md#5-生命周期orchestrator--gateway) · [§5.5](./INGRESS-PORT-DEPLOYMENT.md#55-cellpd-重启reconcile)  
- Listeners：[INGRESS-ROUTING.md §4.3](./INGRESS-ROUTING.md#43-listeners)  
- P5b 审查 defer：[INGRESS-PORT-P5b-review.md §2、§5](./INGRESS-PORT-P5b-review.md)  
- 实现入口：`cellp/internal/gateway/` · `cellp/internal/serve/serve.go` · `web/src/lib/format.ts`

---

*本计划为 P5c 专用；不在此阶段编写产品代码。*
