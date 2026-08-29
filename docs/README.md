# cellp 文档库

> **设计入口：** [DESIGN.md](../DESIGN.md)  
> **决策摘要：** [decisions.md](./decisions.md)  
> **功能验收：** [test-plan.md](./test-plan.md)  
> **Agent 总则：** [../AGENTS.md](../AGENTS.md)

---

## 快速导航

| 我想… | 读这个 |
|--------|--------|
| 理解整体架构 | [DESIGN.md](../DESIGN.md) |
| 查当前有效决策（AD-1..7、D1、Bindings、存储 tier） | [decisions.md](./decisions.md) |
| 跑验收 / 看门禁 | [test-plan.md](./test-plan.md) |
| 本地起栈 | [../dev/README.md](../dev/README.md) · [../dev/AGENTS.md](../dev/AGENTS.md) |
| 改 Dashboard | [../web/AGENTS.md](../web/AGENTS.md) |
| 跑 E2E | [../e2e/README.md](../e2e/README.md) |
| 跑压测 | [../stress/README.md](../stress/README.md) · [phase6/README.md](../stress/phase6/README.md) |
| 查历史 VALIDATION 编号 | [../VALIDATION.md](../VALIDATION.md)（仅索引，执行以 test-plan 为准） |

---

## 契约（冻结 · 修改需对抗审查）

| 文件 | 范围 | 状态 |
|------|------|------|
| [plans/D1-IMPORT-RPC.md](./plans/D1-IMPORT-RPC.md) | 根 version：`celld d1 import` CLI + HTTP | ✅ 已落地 |
| [plans/D1-BRANCH-RPC.md](./plans/D1-BRANCH-RPC.md) | 子 version：`celld d1 branch` CLI + HTTP | ✅ 已落地 |
| [../cellp/api/openapi.yaml](../cellp/api/openapi.yaml) | REST API | ✅ TP-API-7 |

---

## 架构与审查

| 文件 | 内容 |
|------|------|
| [decisions.md](./decisions.md) | **当前有效**决策摘要（推荐首读） |
| [plans/REVIEW.md](./plans/REVIEW.md) | AD-1..5 对抗审查原文 |
| [plans/REVIEW-celld-d1-branch.md](./plans/REVIEW-celld-d1-branch.md) | D1 branch 对抗审查（APPROVE-WITH-CHANGES） |

---

## 实施计划（按 Phase）

| Phase | 文件 | Gate | 状态 |
|-------|------|------|------|
| 0 | [phase-0-storage-gates.md](./plans/phase-0-storage-gates.md) | RustFS 探针 | V0a✅ V0b✅ |
| 1 | [phase-1-backend-core.md](./plans/phase-1-backend-core.md) | schema freeze | ✅ |
| 2 | [phase-2-orchestrator.md](./plans/phase-2-orchestrator.md) | AD-1 spike | ✅ |
| 3 | [phase-3-e2e.md](./plans/phase-3-e2e.md) | run-all | ✅ |
| 4 | [phase-4-dashboard.md](./plans/phase-4-dashboard.md) | M1 TP-VE-ALL | ✅ |
| 5 | [phase-5-stress.md](./plans/phase-5-stress.md) | M2 | 见 phase2 压测 |
| 6 | [phase-6-scale-10m-master.md](./plans/phase-6-scale-10m-master.md) | M3 / 6A | **6A 实现完成**（SQLite scope；6B–6F OUT OF SCOPE） |
| 7 | [phase-7-bindings.md](./plans/phase-7-bindings.md) | AD-6 · AD-7 | **计划中** — celld 0.4.0 KV / Queue / Workflow / Cron |

### v1 收尾（2026-08-29）

| 文件 | 说明 |
|------|------|
| [plans/v1-v0b-phase6-plan.md](./plans/v1-v0b-phase6-plan.md) | V0b PASS + Phase 6A 诚实交付范围 |

### D1 数据面（已完成 · 2026-08-29）

| 文件 | 说明 |
|------|------|
| [plans/celld-d1-branch.md](./plans/celld-d1-branch.md) | D1 branch 实施计划（T1–T5 完成） |
| [plans/D1-IMPORT-RPC.md](./plans/D1-IMPORT-RPC.md) | import 契约 |
| [plans/D1-BRANCH-RPC.md](./plans/D1-BRANCH-RPC.md) | branch 契约 |

> 已删除的过时计划：`celld-d1-binary-import.md`、`REVIEW-celld-d1-import.md`（import 已完成，以契约 + 证据为准）。

---

## 验收计划

| 文件 | 里程碑 | 用途 |
|------|--------|------|
| [test-plan.md](./test-plan.md) | **M1**（VE 门禁）· **M2**（全功能） | 一期主验收 |
| [test-plan-phase2.md](./test-plan-phase2.md) | **M3** | 生产压测 |
| [test-plan-phase6.md](./test-plan-phase6.md) | M6–M7 | 6A 完成（SQLite）；6B–6F OUT OF SCOPE |
| [test-plan-offshoot-branch-scale.md](./test-plan-offshoot-branch-scale.md) | TP-OB | offshoot 大库 CoW |

---

## 证据目录（`evidence/`）

运行前：`mkdir -p docs/evidence`  
临时文件 `*.log` / `*.out` 已 gitignore。

### D1 数据面

| 证据 | 脚本 |
|------|------|
| [d1-import-scale-report.md](./evidence/d1-import-scale-report.md) | `stress/phase6/d1-import-scale.sh` |
| [d1-branch-e2e-report.md](./evidence/d1-branch-e2e-report.md) | `e2e/scripts/v1-d1-branch.sh` |
| [d1-branch-scale-report.md](./evidence/d1-branch-scale-report.md) | `stress/phase6/d1-branch-scale.sh` |
| [d1-branch-multi-100mb.json](./evidence/d1-branch-multi-100mb.json) | `e2e/scripts/v1-d1-branch-multi-100mb.sh` |

### 存储 / 架构

| 证据 | 说明 |
|------|------|
| [celld-multi-fleet-spike.md](./evidence/celld-multi-fleet-spike.md) | AD-1 多 upstream |
| [v0b-pass-report.md](./evidence/v0b-pass-report.md) | V0b offshoot × RustFS 全序列 PASS（2026-08-29） |
| [v0c-skip.md](./evidence/v0c-skip.md) | 多节点探针跳过 |
| [offshoot-branch-scale-report.md](./evidence/offshoot-branch-scale-report.md) | 大库 CoW（local） |
| [scale-report-6A.md](./evidence/scale-report-6A.md) | Phase 6A SQLite 基线 |
| [scale-env.json](./evidence/scale-env.json) | 压测环境拓扑 |

### 指标（append-only JSONL）

| 文件 | 来源 |
|------|------|
| [d1-import-metrics.jsonl](./evidence/d1-import-metrics.jsonl) | `d1-import-scale.sh` |
| [d1-branch-metrics.jsonl](./evidence/d1-branch-metrics.jsonl) | `d1-branch-scale.sh` |
| [d1-branch-multi-metrics.jsonl](./evidence/d1-branch-multi-metrics.jsonl) | multi-100mb |
| [scale-metrics.jsonl](./evidence/scale-metrics.jsonl) | phase6 聚合 |
| [stress-metrics.jsonl](./evidence/stress-metrics.jsonl) | phase5 压测 |

---

## 里程碑

| ID | 含义 | 解锁 |
|----|------|------|
| **M1** | 后端 + TP-VE-ALL | Dashboard 开工 |
| **M2** | test-plan 全绿 | 压测 |
| **M3** | test-plan-phase2 全绿 | 生产 sign-off |
| **M6–M7** | phase6 Gateway / 千万 | 6A 完成（SQLite）；6F **未 sign-off** |

```mermaid
flowchart LR
  P0[Phase 0 存储] --> P1[Phase 1 后端]
  P1 --> P2[Phase 2 Orchestrator]
  P2 --> P3[Phase 3 E2E]
  P3 --> M1[M1 VE 门禁]
  M1 --> P4[Phase 4 Dashboard]
  P4 --> M2[M2 功能完成]
  M2 --> P5[Phase 5 压测]
  P5 --> M3[M3 压测全绿]
  M3 --> P6[Phase 6 千万扩展]
```

---

## Subagent 派发约定

**Module root：** `cellp/` — 所有 Go 命令 `cd cellp && …`

**任务描述必含：**

1. Track ID（如 `P1-T2`）
2. 必读：[decisions.md](./decisions.md) AD-* + 对应 phase + test-plan TP 列表
3. Deliverables（路径 + 验证命令）
4. **禁止：** 改 `go.mod`（除非 deps owner）· PostgreSQL · Caddy · Forgejo · 修改冻结契约

**Dashboard 仅 `web/`** — Vite SPA，禁止引入 Next.js / SSR。

---

*文档库 v1 · 2026-08-29 · cellp 首版交付定型*
