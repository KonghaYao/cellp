# Ingress Port 部署策略（权威设计）

> **状态：** 权威设计 · **待实现**（P5）  
> **上位决策：** [decisions.md §17 AD-12](../decisions.md#17-ad-12--hostname--port-ingress废弃-path-选-version)  
> **Host 路由与 Gateway 行为：** [INGRESS-ROUTING.md](./INGRESS-ROUTING.md)（本文 **不重复** §4 反代头 / §9 blocking 规则）  
> **Dev 操作：** [../dev/INGRESS-HOST.md](../dev/INGRESS-HOST.md)

---

## 1. 目标

在 AD-12 已废弃 path 选 version 的前提下，补齐 **Tier B Port 部署** 的完整策略，满足：

| 需求 | 设计回应 |
|------|----------|
| **Port 写入数据库** | 独立台账表 `port_allocations` + `ingress_bindings.listen_port`（冗余便于查询，以台账为准） |
| **每个对外实例唯一 Port** | 全局 `(port)` 在 **未释放** 分配上唯一；active binding 上 `listen_port` 唯一（已有 partial index） |
| **可指定稳定 Port** | 项目级 `prod_listen_port`（及可选 preview 预留）；`stability=stable` 分配 **不随 promote 回收** |
| **生产稳定指向同一 Port** | **prod** `ingress_binding` 的 `listen_port` **终身不变**；promote 只切 `prod_version_id` + upstream，与 Host 模式「prod Host 不变」同构 |

**禁止：** 浏览器 / Dashboard **生产链接** 指向 celld upstream 口（8803+）。Port 模式对外口仅为 **Gateway ingress listen**（19080+）或外层 `external_map`。

---

## 2. 部署模式矩阵

`CELLP_INGRESS_TIER_B`（全局默认 `host`）与 **项目级覆盖** `projects.ingress_tier_b`（NULL = 继承全局）：

| 模式 | 值 | Preview 选路 | Prod 选路 | 典型场景 |
|------|-----|--------------|-----------|----------|
| **Hostname（默认）** | `host` | Host @ `:GATEWAY_PORT` | Host @ `:GATEWAY_PORT` | 生产 / dev 标准栈 |
| **全 Port** | `dedicated_port` | `127.0.0.1:<preview_port>` | `127.0.0.1:<prod_port>` | 无 DNS、CI 本机多 version |
| **Prod 稳定 Port** | `prod_port` | 同 `host`（Host @ Gateway） | `127.0.0.1:<stable_prod_port>` | **生产固定口** + preview 仍用 Host |
| **仅 Registry** | `external_map` | 外层 stream 映射 | 外层 stream 映射 | cellp 不写 listener |

**Normative：** 任一模式只要存在 `listen_port`，**必须**有 §3 台账记录；Gateway 解析顺序仍见 [INGRESS-ROUTING.md §4.1](./INGRESS-ROUTING.md#41-解析顺序normative)。

---

## 3. Registry：`port_allocations`（权威台账）

> **现状：** `ingress_bindings.listen_port` 已存在；**`port_allocations` 表尚未实现**（INGRESS-ROUTING §3.3 仅概念提及）。

### 3.1 表结构

```sql
CREATE TABLE port_allocations (
  allocation_id   TEXT PRIMARY KEY,
  port            INTEGER NOT NULL,
  purpose         TEXT NOT NULL CHECK (purpose IN ('ingress_listen', 'celld_upstream')),
  stability       TEXT NOT NULL DEFAULT 'ephemeral'
                  CHECK (stability IN ('ephemeral', 'stable')),
  owner_kind      TEXT NOT NULL
                  CHECK (owner_kind IN ('ingress_binding', 'celld_route')),
  owner_id        TEXT NOT NULL,
  project_id      TEXT,
  gateway_id      TEXT,
  created_at      TEXT NOT NULL,
  released_at     TEXT,
  release_reason  TEXT
);

CREATE UNIQUE INDEX idx_port_alloc_port_active
  ON port_allocations(port) WHERE released_at IS NULL;

CREATE INDEX idx_port_alloc_owner_active
  ON port_allocations(owner_kind, owner_id) WHERE released_at IS NULL;
```

| 列 | 说明 |
|----|------|
| `port` | TCP 端口；**ingress_listen** 必须在 `[INGRESS_PORT_MIN, INGRESS_PORT_MAX]`（默认 19080–19999） |
| `purpose` | `ingress_listen` = Gateway 127.0.0.1 listener；`celld_upstream` = 与现有 routes 口统一台账（**P5b** 迁移，可先只写 ingress） |
| `stability` | `stable`：项目/ prod 预留，**archive promote 不释放**；`ephemeral`：preview 绑定，archive 释放 |
| `owner_kind` + `owner_id` | `ingress_binding` + `binding_id`（如 `{project}-prod`、`{project}-{version}-preview`） |
| `gateway_id` | 持有 listener 的 cellpd 实例（与 `ingress_bindings.owner_gateway_id` 一致） |
| `released_at` | NULL = 占用中；非 NULL = 已释放，端口可再分配 |

### 3.2 与 `ingress_bindings` 的关系

```
port_allocations (1 active) ──► ingress_bindings.listen_port (denormalized)
                              ingress_bindings.owner_gateway_id
```

**R-PORT-LEDGER：** `active=1` 且 `listen_port IS NOT NULL` 的 binding，**必须**存在唯一一条 `released_at IS NULL` 且 `purpose=ingress_listen`、`owner_id=binding_id` 的台账。

**R-PORT-LEDGER-REVERSE：** 台账中 `purpose=ingress_listen` 且未释放的记录，**必须**对应一条 `active=1` 的 binding（或 reconcile 中间态 ≤30s）。

### 3.3 端口池互斥（继承 R-PORT-DISJOINT）

| purpose | 区间 | 分配器 |
|---------|------|--------|
| `celld_upstream` | 8803–8999（相对 `CELLD_PORT`） | 现有 orchestrator（**P5b** 写入台账） |
| `ingress_listen` | 19080–19999 | **新** `AllocateIngressListenPort` |
| **禁止** | 9000 RustFS、8787 Gateway 主 listener | 启动 conflict check |

分配前：`SELECT` 活跃台账 + 本机 `bind(127.0.0.1:port)` 探针（与 ready 门禁一致）。

---

## 4. 稳定 Port 与项目配置

### 4.1 `projects` 扩展列

```sql
ALTER TABLE projects ADD COLUMN ingress_tier_b TEXT;          -- NULL | host | dedicated_port | prod_port | external_map
ALTER TABLE projects ADD COLUMN prod_listen_port INTEGER;   -- 可选；stable prod 指定口
ALTER TABLE projects ADD COLUMN prod_host TEXT;             -- INGRESS-ROUTING 已有概念，P5 落地
```

| 字段 | 行为 |
|------|------|
| `prod_listen_port` | 用户在创建/更新项目时指定；须在 ingress 池内且 **未被其他项目 stable 占用**；设置时 **预占** `stability=stable` 台账（`owner_id={project}-prod-reserve`），`ensureProdIngress` 时转为 prod binding |
| `ingress_tier_b` | 覆盖全局；`prod_port` 时 preview 仍走 Host binding（仅 prod binding 带 `listen_port`） |

**API（normative）：**

- `PATCH /v1/projects/{id}` body 可选 `prod_listen_port`（admin）、`ingress_tier_b`
- `GET /v1/projects/{id}` 响应增加 `prod_listen_port`、`ingress_tier_b`、**解析后的** `prod_url`（含 port 形态时 `http://127.0.0.1:{port}/`）

### 4.2 生产「稳定指向一个 Port」

**不变量 R-PROD-PORT-STABLE：**

1. 项目 `{project}-prod` binding 一旦获得 `listen_port`，**promote / rollback / 多次 deploy prod version** 均 **不得**更改该口。
2. `prod_version_id` CAS 切换时，Gateway 在 **同一 listener** 上解析 `role=prod` → 新 upstream。
3. `prod_listen_port` 仅在 **删除项目** 或 **显式 admin 迁移**（未实现则禁止改）时释放 stable 台账。

**与 Host 模式对照：**

| | Host 模式 | Port 模式（prod） |
|--|-----------|-------------------|
| 对外键 | `Host: {project}.{domain}` | `127.0.0.1:{prod_listen_port}` |
| promote 变什么 | upstream only | upstream only |
| 不变什么 | prod Host | prod **listen_port** |

---

## 5. 生命周期（Orchestrator + Gateway）

### 5.1 项目创建

1. 解析 effective `ingress_tier_b`。
2. 若设置了 `prod_listen_port`：校验池范围 → **ReserveStablePort**(project, port) 写入台账。
3. `ensureProdIngress`：
   - `host` / `prod_port`（preview 部分）：Upsert prod binding **带 host**（与现网一致）。
   - `dedicated_port` / `prod_port`（prod 部分）：AttachStableOrAllocatePort → 写 `listen_port` + 台账 + `owner_gateway_id`。

### 5.2 Version ready（preview）

事务内顺序（与 [INGRESS-ROUTING §5.1](./INGRESS-ROUTING.md#51-ready事务内) 对齐）：

1. `SetRoute(active=true)`（celld upstream 口写入 routes；P5b 同步 `celld_upstream` 台账）。
2. **若** effective tier 要求 preview 使用 port：`AllocateIngressListenPort(ephemeral, owner=preview binding_id)` → Upsert preview binding（`host` 可 NULL **仅当** tier=dedicated_port 且策略允许 **仅 port**；默认 **host+port 双写** 便于 Dashboard 展示 Host，prod_port 模式 preview **仅 host**）。
3. `preview_url = FormatPreviewURL(host, listenPort)`（已有 [config/ingress.go](../../cellp/internal/config/ingress.go) 优先级）。
4. 注入 `PUBLIC_BASE_URL`。
5. Gateway **ReconcileListeners**（本机 `Listen 127.0.0.1:port`）。
6. `VerifyGatewayRoute` 使用 **API `preview_url` 的 authority**（含 port），禁止 path 路由。

**ready 失败：** 释放本次 ephemeral 台账 + 回滚 binding；stable prod 口 **不**释放。

### 5.3 Archive / destroy preview

1. `binding.active=false`
2. **ReleasePort**（ephemeral）→ `released_at=now`
3. 关闭 dedicated listener
4. stop celld

### 5.4 Promote

- **禁止**修改 prod binding 的 `listen_port` / stable 台账。
- `mergeProdPublicBaseURL`：`prod_url` 在 port 模式下为 `http://127.0.0.1:{prod_listen_port}/`（或模板覆盖）。
- Preview binding **保留**（AD-12 不变）。

### 5.5 cellpd 重启（Reconcile）

启动时（[INGRESS-ROUTING §4.3 dedicated_port](./INGRESS-ROUTING.md#43-listeners)）：

```
for each port_allocations where purpose=ingress_listen AND released_at IS NULL AND gateway_id = self:
    if binding.active: Listen 127.0.0.1:port
    else: ReleasePort + log orphan
```

多 cellpd：`owner_gateway_id` 不一致的 active 台账 **不得** 在本机 Listen（R-PORT-OWNER）。

---

## 6. 对外 URL 与 Dashboard

| 来源 | 规则 |
|------|------|
| `versions.preview_url` | 台账 + binding 的 **唯一真相**；含 `http://127.0.0.1:PORT/` 时 Dashboard **原样打开**，禁止重拼为 Gateway 8787 |
| `projects.prod_url` | cellpd 按 prod binding 计算；port 模式含固定 port |
| 复制 / curl | 始终 API URL |

**Dashboard 变更（P5）：** `web/src/lib/format.ts` — 若 `preview_url`/`prod_url` 的 port ≠ `gatewayPublicPort()`，**直接使用绝对 URL**（见前序审查）。

---

## 7. API / OpenAPI 增量（P5 冻结前）

| 字段 | 位置 | 说明 |
|------|------|------|
| `prod_listen_port` | Project | optional, stable |
| `ingress_tier_b` | Project | optional override |
| `listen_port` | IngressBinding（若暴露） | 只读，来自 registry |
| `preview_url` | Version | 已有；port 模式必填 port |

可选：`GET /v1/platform/ports` 只读列出活跃 `port_allocations`（运维）。

---

## 8. 强制规则（Port 专用 blocking）

| ID | 规则 |
|----|------|
| R-PORT-LEDGER | §3.2 双向一致 |
| R-PROD-PORT-STABLE | §4.2 promote 不改 prod 口 |
| R-PORT-UNIQUE | 活跃台账全局每 port 一条 |
| R-STABLE-RESERVE | `prod_listen_port` 冲突则 PATCH project 409 |
| R-PORT-OWNER | listener 仅 owner_gateway_id 匹配实例 |
| R-BIND-LOOPBACK | 继承 INGRESS-ROUTING |
| R-ARCHIVE-TEARDOWN | archive 后 port 不可达且 ephemeral 台账释放 |

---

## 9. 实施分期

| 阶段 | 内容 | 验收 |
|------|------|------|
| **P5a** | `port_allocations` schema + Allocate/Release/Reserve API（registry） | unit：并发分配无重复口 |
| **P5b** | orchestrator ready/archive/promote + prod_listen_port | e2e：`dedicated_port` preview 127.0.0.1:port 200 |
| **P5c** | Gateway ReconcileListeners + `prod_port` 混合模式 | e2e：promote 前后 prod port 不变、upstream 变 |
| **P5d** | OpenAPI + Dashboard URL 信任 API | TP-UI ingress port 链接 |
| **P5e** | `celld_upstream` 迁入同一台账（可选） | 文档 + conflict check 统一 |

**证据目录：** `docs/evidence/ingress-port-p5*.md`（待跑）。

---

## 10. 配置摘要

| 变量 | 默认 | Port 相关 |
|------|------|-----------|
| `CELLP_INGRESS_TIER_B` | `host` | 全局默认；可被 project 覆盖 |
| `INGRESS_PORT_MIN/MAX` | 19080 / 19999 | ingress_listen 池 |
| `GATEWAY_ID` | 实例 UUID | 写入 `owner_gateway_id` / 台账 |

---

## 11. 与现有文档关系

- **Host 默认路径不变**；未设 `ingress_tier_b` / 未分配 `listen_port` 时，行为与当前 P3 Host-only Gateway **完全一致**。
- 本文 **冻结** 后，修改 `port_allocations` 语义或 R-PROD-PORT-STABLE 须走对抗审查（同 D1 RPC）。

**索引：** [docs/README.md](../README.md) · 实现入口 `cellp/internal/registry/` · `cellp/internal/orch/ingress.go` · `cellp/internal/gateway/`
