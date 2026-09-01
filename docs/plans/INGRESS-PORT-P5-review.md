# Ingress Port P5a 实现审查

> **审查日期：** 2026-09-01  
> **依据：** [INGRESS-PORT-DEPLOYMENT.md](./INGRESS-PORT-DEPLOYMENT.md) · [INGRESS-PORT-P5-impl-plan.md](./INGRESS-PORT-P5-impl-plan.md)  
> **代码范围：** `cellp/internal/registry/`（`sqlite.go` · `store.go` · `port_alloc.go` · `port_ledger.go` · `store_port_alloc_test.go`）

---

## 1. 结论摘要

| 维度 | 结论 |
|------|------|
| P5a 范围交付 | **基本达标** — schema、Store API、Attach/Detach、单元测试与 plan §1–§5 对齐 |
| Plan 验收命令 | **pass** — `go test ./internal/registry/...` 与 `go test ./...` 全绿 |
| 合并建议 | **可合并**（审查中已修 1 处 Detach 事务 bug）；P5b 前建议补 1–2 条测试 |

---

## 2. R-PORT-* 与 blocking 规则（P5a 适用性）

| ID | P5a 是否需落地 | 审查结果 | 说明 |
|----|----------------|----------|------|
| **R-PORT-UNIQUE** | 是 | **pass** | `idx_port_alloc_port_active` + Allocate 遇 UNIQUE 换口；T2 并发 12 goroutine 无重复 |
| **R-PORT-LEDGER** | 写路径 | **pass** | `AttachIngressListenPort` 单事务：台账 `owner_id=binding_id` + `listen_port`；T9 覆盖 |
| **R-PORT-LEDGER-REVERSE** | 测试/辅助 | **部分 pass** | Attach/Detach 正常路径一致；无 orphan 台账 / reconcile 测试（plan 允许 defer P5c） |
| **R-PORT-DISJOINT** | 否（P5b 探针） | **N/A** | 池边界 `DefaultIngressPortMin/Max` + `OpenWithOptions`；无 bind 探针符合 plan §1.2 |
| **R-PORT-OWNER** | 否 | **N/A** | Gateway `ReconcileListeners` 属 P5c |
| **R-STABLE-RESERVE** | API 层 | **pass** | `ReserveStablePort` + T3/T6/T7；PATCH project 409 属 P5b |
| **R-PROD-PORT-STABLE** | 否 | **N/A** | orchestrator promote 属 P5b |
| **R-BIND-LOOPBACK** | 否 | **N/A** | Gateway |
| **R-ARCHIVE-TEARDOWN** | 否 | **N/A** | orch archive 属 P5b |

---

## 3. P5 实现计划验收（逐条）

### 3.1 §1 P5a 交付

| 项 | 结果 | 证据 |
|----|------|------|
| Schema `port_allocations` + 索引 | **pass** | `migratePortAllocations()` 与设计 §3.1 DDL 一致 |
| `AllocateIngressListenPort` / `ReleasePort` / `ReserveStablePort` | **pass** | `port_alloc.go` |
| `GetActivePortAllocationByOwner` / `ListActivePortAllocations` | **pass** | List 对 `celld_upstream` 返回 `ErrPortPurposeNotSupported` |
| 辅助 `AttachIngressListenPort` / `DetachIngressListenPort` | **pass** | `port_ledger.go`（等价 plan 中 Sync 辅助） |
| `go test ./internal/registry/...` | **pass** | 审查时执行 `-count=1` ok |

### 3.2 §3 详细范围

| 项 | 结果 | 备注 |
|----|------|------|
| 3.1 迁移链顺序（ingress 之后） | **pass** | `migrate()` → `migrateIngress` → `migratePortAllocations` |
| 3.1 仅 `ingress_listen` + `ingress_binding` 写入 | **pass** | 无公开 API 写 `celld_upstream` |
| 3.2 领域类型 / 哨兵错误 | **pass** | `store.go`；含 `ProdPortReserveOwnerID` |
| 3.3 Store 接口签名 | **pass** | 与 plan 冻结列表一致 |
| 3.4 Allocate 算法（幂等 owner、扫池、UNIQUE 重试、池耗尽） | **pass** | T5/T8 |
| 3.5 ReserveStablePort（冲突、同 owner 漂移） | **pass** | T3/T6/T7 |
| 3.6 ReleasePort 不自动改 binding | **pass** | Detach 负责 binding 侧 |
| 3.6 stable 释放策略 defer P5b | **pass** | Release 不区分 stability |

### 3.3 §5 单元测试清单

| # | 用例 | 结果 |
|---|------|------|
| T1 | 两 owner ephemeral | **pass** `TestAllocateIngressListenPortEphemeralTwoOwners` |
| T2 | 并发分配 | **pass** `TestAllocateIngressListenPortConcurrent`（N=12，plan 示例 32 — 见建议） |
| T3 | Reserve 占位 + 冲突 | **pass** `TestReserveStablePortConflict`（口 19090，非 19100 — 可接受） |
| T4 | Release 后再 Allocate | **pass** `TestReleasePortThenReallocate` |
| T5 | 同 owner 幂等 Allocate | **pass** |
| T6 | Reserve 同 owner 同 port 幂等 | **pass** |
| T7 | 同 owner 不同 port → Conflict | **pass** |
| T8 | 单口池耗尽 | **pass** |
| T9 | Attach + ledger 一致 | **pass** |
| T10 | Detach | **pass** |
| T11 | 手动 Upsert 破坏 ledger | **未实现** | plan 标注可选 |

### 3.4 §8 验收命令

| 命令 | 结果 |
|------|------|
| `cd cellp && go test ./internal/registry/... -count=1` | **pass** |
| `cd cellp && go test ./...` | **pass** |

---

## 4. 阻塞问题 vs 建议

### 4.1 阻塞（审查中已修）

| 问题 | 严重性 | 状态 |
|------|--------|------|
| **`DetachIngressListenPort`：binding 不存在但台账释放成功时 `return nil` 未 `Commit`，defer 回滚导致释放无效** | 高（边缘：仅台账 / binding 已删） | **已修** — `b == nil && releaseErr == nil` 时 `tx.Commit()` |

### 4.2 非阻塞建议

| 项 | 类型 | 说明 |
|----|------|------|
| 补测 `Detach` binding 缺失、台账存在 | 测试 | 覆盖上述 bug 回归 |
| 补测 `ReleasePort` 按 `(OwnerKind, OwnerID)` | 测试 | plan §3.3 双路径释放 |
| 补测 `ListActivePortAllocations("celld_upstream")` → `ErrPortPurposeNotSupported` | 测试 | 文档化 P5e defer |
| 并发测试 N=32 | 测试 | 与 plan 示例一致（当前 12 已够验证 UNIQUE） |
| `UpsertIngressBinding` 仍可单独写 `listen_port` | 技术债 | plan 已知；P5b orch 改 Attach |
| `allocateIngressListenPortInTx` 与 `AllocateIngressListenPort` 逻辑重复 | 维护 | 可接受，事务路径需内联 |
| `ReserveStablePort` 在 `onPort != nil && same OwnerID` 时仍 INSERT | 边缘 | 幂等路径应先查 byOwner；同 owner 同 port 走 byOwner 已覆盖 |
| 迁移 idempotent 专项测试 | 可选 | plan §6 可选 |
| `docs/evidence/ingress-port-p5a.md` | 可选 | plan 非阻塞 |

---

## 5. 是否缺测试

**核心 bar（plan §8：T2/T3/T4/T9）：已覆盖。**

**建议补充（非合并阻塞）：**

1. `TestDetachIngressListenPortLedgerOnly` — 无 binding 行、仅台账时 Detach 应 commit 释放。  
2. `TestReleasePortByOwner` — `ReleasePortInput{OwnerKind, OwnerID}`。  
3. （可选）`TestListActivePortAllocationsCelldUpstreamUnsupported`。

---

## 6. 与设计文档分期对齐

| 阶段 | P5a 预期 | 现状 |
|------|----------|------|
| P5a | registry 台账 + unit 并发无重复 | **满足** |
| P5b+ | orch / Gateway / projects 列 | **未动**（符合 plan §7） |

---

## 7. 审查时修改

| 文件 | 变更 |
|------|------|
| `cellp/internal/registry/port_ledger.go` | Detach：binding 缺失时成功释放台账须 Commit |
| `cellp/internal/registry/store.go` | 错误常量对齐空格（cosmetic） |

---

*审查人：Composer · 基于工作区 registry 实现与 2026-09-01 测试输出。*

---

## Fix 阶段

**日期：** 2026-09-01

| 项 | 结果 |
|----|------|
| 剩余阻塞 | **无** — §4.1 唯一阻塞（`DetachIngressListenPort` 无 binding 时未 Commit）已在审查阶段合入 `port_ledger.go` |
| registry 代码/测试 | **无需改动** |
| `cd cellp && go test ./internal/registry/...` | **pass**（Fix 阶段复验） |

§4.2 / §5 建议补测（Detach 仅台账、`ReleasePort` by owner、`ListActive` celld_upstream）为 **P5b 前非阻塞** 项，Fix 阶段未扩大 scope。

## Verify

| 命令 | 结果 |
|------|------|
| `cd cellp && go test ./... -count=1` | **pass** |

- **Exit code:** `0`
- **失败包:** 无（全部 `ok` 或 `[no test files]`）
