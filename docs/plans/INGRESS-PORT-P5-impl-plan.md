# Ingress Port P5 首期实现计划（P5a + 最小可测闭环）

> **状态：** 实施计划 · 只读产出（不改产品代码）  
> **权威设计：** [INGRESS-PORT-DEPLOYMENT.md](./INGRESS-PORT-DEPLOYMENT.md) · [INGRESS-ROUTING.md §3](./INGRESS-ROUTING.md#3-registry)  
> **代码现状快照：** `ingress_bindings` + `listen_port` 已落地；**无** `port_allocations` 表；`Store` 无端口台账 API；orchestrator / Gateway 仅 Host 路径 wired，Port 选路依赖 binding 上已有 `listen_port`（无分配器）。

---

## 1. 目标与边界

### 1.1 P5a 要交付什么

在 **registry 层** 实现 Tier B **ingress_listen** 端口的权威台账与分配 API，并附带 **单元测试可闭环** 的最小同步辅助（见 §4），使 P5b 可直接在事务/编排里调用，而无需再改表结构。

**不在 P5a：** orchestrator ready/archive、Gateway `ReconcileListeners`、projects 列扩展、OpenAPI、Dashboard、e2e port 模式（均 defer，见 §7）。

### 1.2 最小可测闭环（P5a 内）

| 环节 | P5a 范围 |
|------|----------|
| Schema + 迁移 | `port_allocations` 表 + 索引（与设计 §3.1 一致） |
| 台账 CRUD | `AllocateIngressListenPort` / `ReleasePort` / `ReserveStablePort` |
| 与 binding 冗余字段 | **辅助函数** `SyncIngressListenPortLedger`（或等价命名）在 **单事务** 内完成「台账 + `ingress_bindings.listen_port`」，供测试与 P5b 复用 |
| 校验 | `go test ./internal/registry/...` 全绿 |

P5a **不要求** 本机 `bind(127.0.0.1:port)` 探针（设计 §3.3 完整门禁留给 P5b ready 路径）；registry 层仅保证 **DB 内 R-PORT-UNIQUE**（活跃台账每 port 一条）。

---

## 2. 现状对照

| 组件 | 文件 | 与 Port 相关现状 |
|------|------|------------------|
| Schema | `cellp/internal/registry/sqlite.go` | `migrateIngress()` 已有 `ingress_bindings`；`idx_ingress_listen_active` 对 active `listen_port` 唯一 |
| 类型 / 接口 | `cellp/internal/registry/store.go` | `DefaultIngressPortMin/Max`；`IngressBinding.ListenPort`；**无** `PortAllocation`、**无** 分配接口 |
| Ingress CRUD | `cellp/internal/registry/ingress.go` | `UpsertIngressBinding` 可写 `listen_port`，**不**写台账 → 违反 R-PORT-LEDGER |
| Orchestrator | `cellp/internal/orch/ingress.go` | 仅 Host：`ensurePreviewIngress` / `ensureProdIngress`，无 port |
| Gateway | `cellp/internal/gateway/ingress_resolve.go` | `dedicated_port` 时 `LookupIngressByListenPort(localPort, gatewayID)` 已存在 |
| celld 上游口 | `cellp/internal/runtime/manager.go` | 内存 `AllocatePort`（8803+），**非** registry 台账（P5e） |

**结论：** P5a 只补 registry「ingress_listen 台账 + 分配语义」；现有测试 `store_ingress_test.go` 可直接 Upsert `listen_port` 而无台账——P5a 后应 **新增** port 测试文件，并 **禁止** 生产路径在 P5b 之前绕过台账写 `listen_port`（本计划仅文档约束，代码变更在 P5a PR）。

---

## 3. P5a 范围（详细）

### 3.1 表迁移 `port_allocations`

在 `SQLiteStore.migrate()` 链中新增 `migratePortAllocations()`（在 `migrateIngress()` 之后调用），DDL **对齐** [INGRESS-PORT-DEPLOYMENT §3.1](./INGRESS-PORT-DEPLOYMENT.md#31-表结构)：

- 列：`allocation_id`, `port`, `purpose`, `stability`, `owner_kind`, `owner_id`, `project_id`, `gateway_id`, `created_at`, `released_at`, `release_reason`
- CHECK：`purpose IN ('ingress_listen', 'celld_upstream')`；`stability IN ('ephemeral', 'stable')`；`owner_kind IN ('ingress_binding', 'celld_route')`
- 索引：`idx_port_alloc_port_active`（`port` WHERE `released_at IS NULL` UNIQUE）；`idx_port_alloc_owner_active`（`owner_kind`, `owner_id` WHERE `released_at IS NULL`）

**P5a 写入约束：** 实现层 **仅允许** `purpose=ingress_listen` + `owner_kind=ingress_binding`；`celld_upstream` 枚举保留给 P5e，调用返回 `ErrPortPurposeNotSupported` 或等价。

### 3.2 领域类型与错误（`store.go`）

建议新增：

```go
type PortAllocation struct {
    AllocationID  string
    Port          int
    Purpose       string // ingress_listen | celld_upstream (P5a 只实现前者)
    Stability     string // ephemeral | stable
    OwnerKind     string
    OwnerID       string
    ProjectID     *string
    GatewayID     *string
    CreatedAt     time.Time
    ReleasedAt    *time.Time
    ReleaseReason *string
}

const (
    PortPurposeIngressListen = "ingress_listen"
    PortStabilityEphemeral   = "ephemeral"
    PortStabilityStable      = "stable"
    PortOwnerIngressBinding  = "ingress_binding"
)

// 稳定 prod 预占 owner_id 后缀（设计 §4.1）
func ProdPortReserveOwnerID(projectID string) string { return projectID + "-prod-reserve" }
```

哨兵错误（命名可微调，需在 package 内稳定）：

- `ErrPortConflict` — 端口已被其他 **活跃** 台账占用（含 stable 跨项目）
- `ErrPortPoolExhausted` — `[portMin, portMax]` 无可用口
- `ErrPortAllocationNotFound` — release / get 目标不存在或已释放
- `ErrPortInvalid` — 端口超出 ingress 池或参数非法

**池边界：** 默认 `DefaultIngressPortMin` / `DefaultIngressPortMax`（19080–19999）；可选 `IngressPortPool` 结构体或 `Open` 后配置字段供测试缩小池（避免单测扫 900 口过慢）。

### 3.3 `Store` 接口扩展

在 `Store` 上增加（P5a 冻结签名，P5b 直接依赖）：

```go
// AllocateIngressListenPort 在 ingress 池内选取当前未被活跃台账占用的端口并插入记录。
// stability=ephemeral：preview 等可 archive 释放；stable：prod 预留/绑定，Release 需显式 reason（P5b 政策）。
AllocateIngressListenPort(ctx context.Context, in AllocateIngressListenPortInput) (*PortAllocation, error)

// ReserveStablePort 在指定 port 上插入 stability=stable 的 ingress_listen 台账（设计 §4.1 prod_listen_port 预占）。
// owner_id 通常为 ProdPortReserveOwnerID(project) 或 prod binding_id；冲突返回 ErrPortConflict。
ReserveStablePort(ctx context.Context, in ReserveStablePortInput) (*PortAllocation, error)

// ReleasePort 将活跃台账标记 released_at=now（按 allocation_id，或 owner_kind+owner_id 唯一活跃条）。
ReleasePort(ctx context.Context, in ReleasePortInput) error

// GetActivePortAllocationByOwner 运维/测试：查 owner 当前活跃 ingress_listen。
GetActivePortAllocationByOwner(ctx context.Context, ownerKind, ownerID string) (*PortAllocation, error)

// ListActivePortAllocations 可选；P5a 至少供测试断言，filter purpose=ingress_listen。
ListActivePortAllocations(ctx context.Context, purpose string) ([]PortAllocation, error)
```

**Input 结构体字段（normative 最小集）：**

| 方法 | 关键字段 |
|------|----------|
| `AllocateIngressListenPort` | `Stability`, `OwnerKind`, `OwnerID`, `ProjectID`, `GatewayID`（均可选除 Owner*） |
| `ReserveStablePort` | `Port`, `OwnerKind`, `OwnerID`, `ProjectID`, `GatewayID` |
| `ReleasePort` | `AllocationID` **或** `(OwnerKind, OwnerID)`；`ReleaseReason` string |

**实现文件建议：** 新建 `cellp/internal/registry/port_alloc.go`（实现 + 私有 scan/helpers），迁移 SQL 可放在 `sqlite.go` 的 `migratePortAllocations` 或同文件 `const portAllocSchema`。

### 3.4 `AllocateIngressListenPort` 算法

1. 校验 `owner_kind=ingress_binding`，`stability` ∈ {ephemeral, stable}。
2. 若该 `(owner_kind, owner_id)` 已有活跃 `ingress_listen` 台账：**幂等**返回现有记录（与 celld `AllocatePort` 同项目内 version 行为一致，便于重试）。
3. 在 `[portMin, portMax]` 顺序扫描（或 `SELECT MIN(candidate)` 子查询）：候选 `p` 满足 **不存在** `released_at IS NULL AND port=p` 的任意 purpose 行（P5a 仅 ingress 写入，但 UNIQUE 索引已覆盖全 purpose）。
4. `INSERT` 新行 `allocation_id=uuid`，`created_at=RFC3339`；遇 UNIQUE → **换下一 port 重试**（并发安全核心）。
5. 池耗尽 → `ErrPortPoolExhausted`。

**并发：** SQLite `MaxOpenConns(1)` 已序列化连接；单元测试仍应用 **多 goroutine + 多 Store 不可能**——应使用 **单 Store + 多 goroutine 并发 Allocate** 验证 UNIQUE + 重试；或临时测试用 `:memory:` 多连接若未来改 pool（当前保持 1 连接，并发测试验证逻辑正确即可）。

### 3.5 `ReserveStablePort`

1. 校验 `port` ∈ ingress 池。
2. 若 `port` 已有活跃台账且 `owner_id` 不同 → `ErrPortConflict`（R-STABLE-RESERVE）。
3. 若同 `owner_id` 已占用 **同一** port → 幂等返回。
4. 若同 `owner_id` 已占用 **不同** port → `ErrPortConflict`（stable 不可漂移，对齐 R-PROD-PORT-STABLE  spirit）。
5. `INSERT` stability=stable，`purpose=ingress_listen`。

**设计 §4.1：** 预占 `owner_id={project}-prod-reserve`；P5a 实现 **不解析** prod binding，仅保证 API；`ensureProdIngress` 转 binding 属 P5b（可能 `Release` reserve + 新 owner 同 port 或 UPDATE owner_id——P5b 设计二选一，P5a 文档预留 **TransferStablePortOwner** 若需要，可 defer 到 P5b 首 PR）。

### 3.6 `ReleasePort`

1. 定位唯一活跃行（by id 或 owner）。
2. `UPDATE SET released_at=?, release_reason=? WHERE allocation_id=? AND released_at IS NULL`。
3. **不**自动修改 `ingress_bindings`（调用方或 §4 辅助函数负责），避免 stable prod 误清。

**ephemeral vs stable：** P5a 实现 **不**在 `ReleasePort` 内拒绝 stable；P5b orchestrator 对 prod stable 禁止调用 Release。可选：P5a 加 `AllowStableRelease bool` 测试旗标，默认 false 时 stable 需 `ReleaseReason=="admin"`——**建议 P5a 保持简单**，stable 释放策略写在 P5b。

---

## 4. R-PORT-LEDGER 与 `ingress_bindings.listen_port` 同步

权威规则（[INGRESS-PORT-DEPLOYMENT §3.2](./INGRESS-PORT-DEPLOYMENT.md#32-与-ingress_bindings-的关系)）：

| ID | 规则 | P5a 落地 |
|----|------|----------|
| **R-PORT-LEDGER** | `active=1` 且 `listen_port IS NOT NULL` 的 binding → 唯一活跃台账 `purpose=ingress_listen`, `owner_id=binding_id` | 辅助函数 **写路径** 强制 |
| **R-PORT-LEDGER-REVERSE** | 活跃 ingress_listen 台账 → `active=1` binding（或 ≤30s reconcile） | P5a **测试** 辅助函数；全量 reconcile 属 P5c |
| **R-PORT-UNIQUE** | 活跃台账全局每 port 一条 | UNIQUE 索引 + Allocate 算法 |

### 4.1 推荐辅助 API（P5a 同 PR，registry 包内）

```go
// AttachIngressListenPort 单事务：Allocate 或绑定已有 stable 台账 → Upsert binding.listen_port (+ owner_gateway_id)。
AttachIngressListenPort(ctx context.Context, binding IngressBinding, alloc AllocateIngressListenPortInput) error

// DetachIngressListenPort 单事务：ReleasePort(owner=binding_id) + 将 binding listen_port 置 NULL（inactive 时）。
DetachIngressListenPort(ctx context.Context, bindingID, releaseReason string) error
```

**顺序（写路径 normative）：**

1. 台账：`AllocateIngressListenPort` / 已有 stable 行 / `ReserveStablePort` 已存在的 port  attach 到 `owner_id=binding_id`。
2. `UpsertIngressBinding`：`ListenPort = &port`，`OwnerGatewayID` 与台账 `gateway_id` 一致。
3. **禁止** 单独 `UpsertIngressBinding` 设置 `listen_port` 而不写台账（P5b 起 orchestrator 遵守；P5a 可用 lint 注释或 `_test` 仅）。

**Detach（ephemeral 预览 teardown 预览）：**

1. `ReleasePort`（owner=`binding_id`）。
2. `UpsertIngressBinding` 清 `listen_port`（或 `SetIngressBindingActive(false)` 由 P5b 定序）。

**与现有 `UpsertIngressBinding`：** P5a **不**改 Upsert 签名；Ledger 一致性由 **Attach/Detach** 与 P5b 调用点保证。可选：Upsert 时若 `listen_port != nil` 且非测试 build tag 打 log warn——**非必须**。

### 4.2 `ReserveStablePort` 与 prod 预占

- 预占：`owner_id = {project}-prod-reserve`，`project_id` 填 project。
- 真正 prod binding `{project}-prod` attach 同一 port：**P5b** 实现「reserve → prod binding」迁移；P5a 测试覆盖 reserve 与第二项目同 port 冲突即可。

---

## 5. 单元测试清单（`store_port_alloc_test.go`）

与 [INGRESS-PORT-DEPLOYMENT §9 P5a](./INGRESS-PORT-DEPLOYMENT.md#9-实施分期) 对齐，并补充 ledger 辅助：

| # | 用例 | 断言 |
|---|------|------|
| T1 | `AllocateIngressListenPort` ephemeral ×2 不同 owner | 两端口不同，均在池内；`ListActive` count=2 |
| T2 | **并发分配**：N goroutine（如 32）各 Allocate 不同 owner | 无 duplicate port；N 条活跃台账 |
| T3 | `ReserveStablePort` 指定 19100 | 再 Allocate 不得返回 19100；其他 owner Reserve 19100 → `ErrPortConflict` |
| T4 | `ReleasePort` 后同 port 再 Allocate | 成功；旧 `allocation_id` 已 released |
| T5 | 同 owner 二次 `AllocateIngressListenPort` | 幂等同一 port / 同一 allocation |
| T6 | `ReserveStablePort` 同 owner 同 port 重复 | 幂等 |
| T7 | `ReserveStablePort` 同 owner **不同** port | `ErrPortConflict` |
| T8 | 缩小 portMin=portMax=单口，占满后 Allocate | `ErrPortPoolExhausted` |
| T9 | `AttachIngressListenPort` | binding 上 `listen_port` 与台账一致；GetActiveByOwner 命中 |
| T10 | `DetachIngressListenPort` | 台账 released；binding `listen_port` 清除或 inactive |
| T11 | `Attach` 后违反 R-PORT-LEDGER 的手动 Upsert 不同 port（可选文档测试） | 说明 P5b 不应这么做；或测 `GetIngressBinding` vs ledger 不一致供 future reconcile |

**测试池：** 使用临时目录 DB + 测试专用 `IngressPortPool{Min: 19080, Max: 19095}` 缩短扫描（通过 `Open` 选项或 package-level test hook，实现时在 `Open` 增加 `Options` 或 unexported test setter）。

**现有测试：** `TestIngressBindingListenPortOwner` 继续有效（无台账的 legacy 写法的 **文档债**）；P5a PR **不强制** 改旧测试，P5b 切换 orchestrator 后改为 Attach 路径。

---

## 6. 实现顺序（建议 PR 内 commit 顺序）

1. 类型 + `Store` 接口 + 错误常量（`store.go`）
2. `migratePortAllocations` + `Open` 验证迁移 idempotent
3. `port_alloc.go`：Allocate / Reserve / Release / Get / List
4. `AttachIngressListenPort` / `DetachIngressListenPort`（事务：`BeginTx` 模式参考 `ClaimJob` / `PurgeDestroyedVersions`）
5. `store_port_alloc_test.go` 全清单
6. （可选）`docs/evidence/ingress-port-p5a.md` 粘贴测试输出——非阻塞

**Mock：** 若其他包需接口，P5a 不必改 mockgen；仅 registry 包内测试。

---

## 7. 明确 defer 的后续 PR

| 阶段 | 内容 | 依赖 P5a |
|------|------|----------|
| **P5b** | `projects.ingress_tier_b` / `prod_listen_port` 迁移；orchestrator ready/archive/promote 调用 Attach/Detach/Reserve；`FormatPreviewURL` + port；ready 失败 rollback ephemeral；e2e `CELLP_INGRESS_TIER_B=dedicated_port` | Store API + Attach |
| **P5c** | Gateway `ReconcileListeners`；`prod_port` 混合模式；启动 orphan 台账清理（§5.5）；e2e promote prod port 不变 | P5b + ledger 列表 |
| **P5d** | OpenAPI `prod_listen_port` / `ingress_tier_b`；Dashboard `format.ts` 信任 API port URL | P5b/c |
| **P5e** | `celld_upstream` 写入同一 `port_allocations`；与 `runtime.Manager.AllocatePort` 统一；启动 port conflict check 含台账 | 可选，独立 |

**P5a 不触碰：** `orch/ingress.go`、`gateway/*` listener、`web/`、`e2e/`、`projects` ALTER、本机 bind 探针。

---

## 8. 验收命令

```bash
cd cellp && go test ./internal/registry/... -count=1
```

**门禁：** 上式全绿；新增测试覆盖 §5 核心用例（T2/T3/T4/T9 为 P5a 最小 bar）。

**回归：**

```bash
cd cellp && go test ./...
```

P5a 不应破坏现有 `store_ingress_test.go` 与其它 registry 测试。

---

## 9. 风险与决策点（P5a PR 前可确认）

| 项 | 建议 |
|----|------|
| stable 预占 → prod binding 迁移 | P5b 首 PR 定：`UPDATE owner_id` vs release+reserve；P5a 只保证 Reserve/Allocate 语义 |
| 幂等 Allocate 同 owner | **采用**（对齐 runtime manager） |
| `Open` 配置 ingress 池 | 测试 hook 优先，环境变量 `INGRESS_PORT_MIN/MAX` 解析放 **P5b** config 层 |
| 事务与 `withRetry` | Attach/Detach 用 `BeginTx` + 内层不重试整事务；UNIQUE 失败在 Allocate 内换 port 重试 |

---

## 10. 参考索引

- 设计台账与生命周期：[INGRESS-PORT-DEPLOYMENT.md](./INGRESS-PORT-DEPLOYMENT.md) §3–§5、§8–§9  
- Registry 概念与端口池：[INGRESS-ROUTING.md §3.3](./INGRESS-ROUTING.md#33-端口分配互斥)  
- Gateway 解析（P5c）：[INGRESS-ROUTING.md §4.1](./INGRESS-ROUTING.md#41-解析顺序normative) · `gateway/ingress_resolve.go`  
- 现有 ingress 测试模式：`cellp/internal/registry/store_ingress_test.go`

---

*文档版本：2026-09-01 · 对应仓库 ingress 实现快照（无 `port_allocations`）。*

---

## 实施记录

**日期：** 2026-09-01 · **范围：** P5a（registry 层）

### 交付项

| 项 | 位置 |
|----|------|
| `port_allocations` 迁移 | `cellp/internal/registry/sqlite.go` → `migratePortAllocations()` |
| 领域类型 / 错误 / `Store` 接口 | `cellp/internal/registry/store.go` |
| Allocate / Reserve / Release / Get / List | `cellp/internal/registry/port_alloc.go` |
| `AttachIngressListenPort` / `DetachIngressListenPort`（单事务，R-PORT-LEDGER 写路径） | `cellp/internal/registry/port_ledger.go` |
| 测试池 `OpenWithOptions` | `OpenOptions{IngressPortMin, IngressPortMax}` |
| 单元测试 T1–T10 | `cellp/internal/registry/store_port_alloc_test.go` |

### 未改（P5b）

- `orch/` ready/archive 路径仍仅用 Host ingress；**未**调用 Attach/Detach。
- `UpsertIngressBinding` 仍可单独写 `listen_port`（遗留测试兼容）；生产路径应在 P5b 切换台账。

### 验收

```bash
cd cellp && go test ./internal/registry/... -count=1
# ok github.com/cellp/cellp/internal/registry
```

### 审查与 Fix 阶段

| 文档 | 结论 |
|------|------|
| [INGRESS-PORT-P5-review.md](./INGRESS-PORT-P5-review.md) | P5a 基本达标；plan 验收命令全绿；**可合并** |
| Fix 阶段（同 review §Fix 阶段） | 无剩余阻塞；registry **未再改代码**；复验 `go test ./internal/registry/...` pass |

审查期已合入：`port_ledger.go` Detach 在 binding 缺失但台账释放成功时 `Commit`；`store.go` 错误常量 cosmetic。

