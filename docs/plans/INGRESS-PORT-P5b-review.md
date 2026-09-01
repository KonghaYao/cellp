# Ingress Port P5b 审查报告

> **日期：** 2026-09-01  
> **范围：** [INGRESS-PORT-DEPLOYMENT.md](./INGRESS-PORT-DEPLOYMENT.md)（**R-PROD-PORT-STABLE**、**R-ARCHIVE-TEARDOWN**）· [INGRESS-PORT-P5b-impl-plan.md](./INGRESS-PORT-P5b-impl-plan.md) · 仓库实现  
> **前置：** [INGRESS-PORT-P5-review.md](./INGRESS-PORT-P5-review.md)（P5a **pass**）

---

## 总评

| 维度 | 结论 |
|------|------|
| **P5b 实现 vs 实施计划 §1.1 / §实施记录** | **pass** |
| **R-PROD-PORT-STABLE（编排层）** | **pass** |
| **R-ARCHIVE-TEARDOWN（编排 + 台账）** | **pass（registry/Detach）**；**部分 defer（Gateway 不可达）** |
| **设计文档与代码一致性** | **pass**，少量文档滞后 |
| **测试矩阵 §6.1** | **未闭环**（T3–T5、Archive 集成） |
| **e2e §6.2 / 设计 §9 P5b 验收** | **defer**（无 `v1-ingress-port-preview.sh`，符合计划） |

**合并建议：** **可合并**（P5b 编排/registry 目标已落地）；合并前/后 **建议** 补 §6.1 中 T3–T5，不视为阻塞项。

---

## 1. 权威设计：R-PROD-PORT-STABLE

### 1.1 规范（INGRESS-PORT-DEPLOYMENT §4.2、§5.4、§8）

| 要点 | 评价 |
|------|------|
| prod binding 一旦绑定 `listen_port`，promote / rollback / 多次 deploy **不得改口** | 表述清晰，与 AD-12 Host 同构对照完整 |
| CAS 仅切 upstream；stable 台账仅在删项目或 admin 迁移时释放 | 与 P5a `ReserveStablePort` / adopt 衔接正确 |
| blocking 表 §8 将 R-PROD-PORT-STABLE 标为 promote 责任 | 与 P5b 分期一致 |

**文档：** **pass**

### 1.2 实现对照

| 检查项 | 位置 | 结果 |
|--------|------|------|
| 已有 prod `listen_port` 时 merge，不重新 Attach | `orch/ingress.go` `ensureProdIngress` L132–137 | **pass** |
| 无口时 stable：reserve adopt 或 allocate | `AdoptStableIngressPortForBinding` | **pass** |
| Promote 路径调用 `ensureProdIngress`，不 Release prod stable | `orchestrator.go` Promote | **pass** |
| 单元：两次 `ensureProdIngress` 同口 | `TestEnsureProdIngressPreservesListenPort` | **pass** |
| 单元：**完整 Promote saga** 两次 promote 口不变 | — | **缺失（T5）** |

**规则 R-PROD-PORT-STABLE：** **pass**（实现 + 最小单测；Promote 端到端单测为缺口）。

---

## 2. 权威设计：R-ARCHIVE-TEARDOWN

### 2.1 规范（§5.3、§8）

| 子要求 | P5b 责任 | P5c 责任 |
|--------|----------|----------|
| ephemeral **ReleasePort** / 台账 `released_at` | **是** | — |
| preview binding `active=false`、清 `listen_port`（host 双写策略） | **是** | — |
| 关闭 dedicated listener → **port 不可达** | 部分（Stop celld + Detach 台账） | **ReconcileListeners** 关 Listen |

规范将「不可达」与 archive 生命周期绑在一起；P5b impl-plan §1.2 已声明无 listener 时 curl 可能失败，**合理拆分**。

**文档：** **pass**（需在审查中显式标注 P5c 才能完成「TCP 不可达」全义）。

### 2.2 实现对照

| 检查项 | 位置 | 结果 |
|--------|------|------|
| Archive 调用 `teardownPreviewIngress` | `archive.go` L113 | **pass** |
| compensate 同样 Detach | `compensateDeploy` | **pass** |
| Detach：Release + 有 Host 时清 `listen_port` | `port_ledger.go` `DetachIngressListenPort` | **pass** |
| Wake 重新 `ensurePreviewIngress`（新 ephemeral） | `archive.go` Wake | **pass** |
| `TestTeardownPreviewIngressDetach` 验台账释放 | `ingress_test.go`（审查中补强） | **pass** |
| `TestArchiveReadyVersion` 在 `dedicated_port` 下验 Detach | `archive_test.go` | **缺失（T4）** |

**规则 R-ARCHIVE-TEARDOWN（P5b 范围）：** **pass**；**Gateway 侧不可达** = **N/A / P5c**。

---

## 3. P5b 实施计划审查

### 3.1 交付清单（§1.1、§实施记录）vs 代码

| 计划项 | 状态 |
|--------|------|
| `projects.ingress_tier_b` / `prod_listen_port` + Create Reserve | **已落地** |
| `EffectiveIngressTierB` / Validate + cellpd fail-fast | **已落地** |
| `Attach` / `Detach` / `AdoptStableIngressPortForBinding` | **已落地** |
| deploy 顺序 SetRoute → ensurePreviewIngress；port verify 可选 env | **已落地** |
| API `prod_url` / create 字段 | **已落地**（PATCH/Dashboard **defer P5d**） |
| P5c listener / 默认 port e2e 脚本 | **defer**（与计划一致） |

**计划正文：** **pass**

### 3.2 计划内不一致 / 滞后（非阻塞）

| 项 | 说明 |
|----|------|
| 文首 *「orch Host-only 快照」*（§ 末 *2026-09-01* 段） | 与 § **实施记录** 矛盾；应改为「P5b 已落地」或删除旧脚注 |
| §5.2 默认 **host+port 双写** vs 实现 `preview_url` 纯 `127.0.0.1:port` | binding 仍写 `Host`；**API URL** 走 loopback — 与 §4.7 / 实施记录一致，设计 §5.2 易误读，建议在 DEPLOYMENT 加一句「`preview_url` authority 以 orch 写入为准」 |
| INGRESS-PORT-DEPLOYMENT 文首 **「待实现 P5」** | 应更新为 P5a✓ / P5b✓（编排）/ P5c 待办 |

### 3.3 决策点（§8）

| 决策 | 实现 |
|------|------|
| reserve → prod **UPDATE owner_id** | **是** |
| bind 探针 §3.3 | **defer**（与 P5a/P5b 文档一致） |
| `external_map` no-op | 未专门测；tier 分支未写台账 — **可接受** |

---

## 4. 强制规则自检（P5b 完成后 §9）

| ID | 结论 | 说明 |
|----|------|------|
| R-PORT-LEDGER | **pass** | 生产路径经 Attach/Adopt/Detach |
| R-PROD-PORT-STABLE | **pass** | 见 §1 |
| R-PORT-UNIQUE | **pass** | P5a registry |
| R-STABLE-RESERVE | **pass** | CreateProject + `store_project_ingress_test` |
| R-ARCHIVE-TEARDOWN | **pass（台账）** / **P5c（Listen）** | 见 §2 |
| R-BIND-LOOPBACK | **N/A** | Gateway P5c |

---

## 5. 测试缺口（相对 impl-plan §6.1）

| ID | 场景 | 现状 | 阻塞？ |
|----|------|------|--------|
| T1 | effective tier | `ingress_tier_test.go` | 否 |
| T2 | dedicated_port Attach + ledger | `ingress_test.go` | 否 |
| T3 | compensate Detach + 口可再分配 | 仅 route inactive | **建议补** |
| T4 | **Archive** ephemeral Release | Host-only archive 测 | **建议补** |
| T5 | **Promote 两次** prod 口不变 | 仅 double `ensureProdIngress` | **建议补** |
| T6 | Create reserve + adopt | registry + orch 部分 | 否 |
| T7 | ProdURL / FormatPreviewURL loopback | `ingress_port_test.go` | 否 |
| e2e | `127.0.0.1:port` HTTP 200 | 无脚本；`INGRESS_PORT_E2E` 未接线 | **defer P5c**（计划允许） |

**验收命令（审查时）：**

```text
cd cellp && go test ./internal/orch/... ./internal/registry/... ./internal/config/... -count=1  # ok
```

---

## 6. 阻塞项

**无代码级阻塞项。**

若将 **设计 §9 P5b 验收行**（e2e dedicated_port 200）视为硬门禁，则 **阻塞在 P5c**（listener），与 impl-plan §1.2 一致，**不应判 P5b 实现 fail**。

---

## 7. 审查期小修

| 变更 | 说明 |
|------|------|
| `orch/ingress_test.go` `TestTeardownPreviewIngressDetach` | 增加 `GetActivePortAllocationByOwner` 断言，对齐 R-ARCHIVE-TEARDOWN 台账释放 |

---

## 8. 建议后续（非 P5b 阻塞）

1. 补 `TestArchiveDedicatedPortReleasesLedger`、`TestCompensateDeployReleasesEphemeralPort`、`TestPromotePreservesProdListenPort`（full Promote）。  
2. 更新 INGRESS-PORT-DEPLOYMENT 状态行 + P5b impl-plan 文首脚注。  
3. P5c 落地后：`v1-ingress-port-preview.sh` + `docs/evidence/ingress-port-p5b.md` 手动/CI 可选跑。  

---

**审查结论：P5b — pass（测试矩阵与顶层设计状态文案未完全同步；e2e 按分期 defer）。**

---

## Fix 阶段

**日期：** 2026-09-01

| 项 | 结果 |
|----|------|
| 剩余阻塞 | **无** — §6 无代码级阻塞项；T3–T5 / e2e 为建议补测或 P5c defer，Fix 阶段未扩大 scope |
| orch / registry 代码 | **无需改动** |
| P5c Gateway listener | **未启动**（按指令） |
| `cd cellp && go test ./internal/orch/... ./internal/registry/...` | **pass**（Fix 阶段复验） |

§8 建议补测（`TestArchiveDedicatedPortReleasesLedger`、`TestCompensateDeployReleasesEphemeralPort`、`TestPromotePreservesProdListenPort`）与 DEPLOYMENT 状态文案更新仍为 **非阻塞**，Fix 阶段未做。
