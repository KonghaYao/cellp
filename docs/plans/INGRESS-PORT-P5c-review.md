# Ingress Port P5c 审查报告

> **日期：** 2026-09-01  
> **范围：** [INGRESS-PORT-DEPLOYMENT.md](./INGRESS-PORT-DEPLOYMENT.md) §5.3 / §8 **R-ARCHIVE-TEARDOWN** · [INGRESS-PORT-P5c-impl-plan.md](./INGRESS-PORT-P5c-impl-plan.md) · 仓库实现  
> **前置：** [INGRESS-PORT-P5b-review.md](./INGRESS-PORT-P5b-review.md)（台账 Detach **pass**；TCP 不可达 **defer → P5c**）

---

## 总评

| 维度 | 结论 |
|------|------|
| **P5c 核心（ListenerManager + Reconcile）** | **pass** |
| **R-ARCHIVE-TEARDOWN（全义：台账 + TCP 不可达）** | **pass（实现 + 单测）** |
| **设计 §9 P5c 验收（e2e / promote / web）** | **部分 defer**（与 impl-plan §1.2 一致） |
| **Go 单测（gateway + orch）** | **pass**（本审查复跑） |

**合并建议（相对 R-ARCHIVE-TEARDOWN）：** **可合并** — Listen 半部已闭环；**无代码级阻塞**。若将 impl-plan §7.2 的 opt-in e2e / `format.ts` 视为 P5c 完整交付，则 **非阻塞缺口** 见 §5。

---

## 1. 权威设计：R-ARCHIVE-TEARDOWN

### 1.1 规范（INGRESS-PORT-DEPLOYMENT §5.3、§8）

| 子要求 | 规范表述 | 责任分期 |
|--------|----------|----------|
| ephemeral **ReleasePort** / `released_at` | archive / destroy preview | P5b（registry + orch Detach） |
| preview binding **`active=false`**、清 `listen_port`（host 双写策略） | 同上 | P5b |
| **关闭 dedicated listener** → `127.0.0.1:port` **不可达** | §5.3 步骤 3 | **P5c** |
| stop celld | §5.3 步骤 4 | P5b（archive 路径已有） |

**§8 合并表述：**「archive 后 port 不可达 **且** ephemeral 台账释放」— 两项须同时成立（stable prod 口不在 archive preview 范围）。

**文档：** **pass**（P5b 审查已拆分责任；P5c 补齐 Listen 半部后全义可验收）。

### 1.2 实现对照

| 检查项 | 位置 | 结果 |
|--------|------|------|
| Archive 调用 `teardownPreviewIngress(..., "archive")` | `orch/archive.go` L113 | **pass** |
| Teardown：`DetachIngressListenPort` + `SetIngressBindingActive(false)` | `orch/ingress.go` L163–167 | **pass** |
| Teardown 后触发 Gateway reconcile | `reconcileIngressListenersLog` @ L167 | **pass**（`serve.Run` 注入 `ListenerManager`） |
| Reconcile：台账不在 desired → **Shutdown** 专用 `http.Server` | `gateway/listeners.go` L89–94, L124–132 | **pass** |
| Reconcile：仅 `127.0.0.1` bind | `listeners.go` L107 | **pass**（**R-BIND-LOOPBACK**） |
| 启动 boot reconcile + §5.5 orphan `ReleasePort` | `serve/serve.go` L78–79；`listeners.go` L71–78 | **pass** |
| Ready 路径 reconcile **失败即 deploy 失败** | `orchestrator.go` L304–306 | **pass** |
| Archive 路径 reconcile **仅 log 错误** | `reconcileIngressListenersLog` | **warn**（见 §4 非阻塞） |
| 单测：Detach + reconcile 后 **curl 连接失败** | `gateway/listeners_test.go` `TestListenerManagerReconcileClosesAfterDetach` | **pass** |
| 单测：ephemeral 台账释放 | `orch/ingress_test.go` `TestTeardownPreviewIngressDetach` | **pass**（P5b 延续） |
| 单测：**Archive 全流程** + ListenerManager 同进程 | `orch/archive_ingress_listener_test.go` `TestArchiveReadyVersionClosesDedicatedListener` | **pass** |

**规则 R-ARCHIVE-TEARDOWN：** **pass** — 与 P5b 台账 Detach 衔接；TCP 不可达由 `ListenerManager` + teardown 后 reconcile 实现，并由 **L2** 单测直接断言。

### 1.3 生命周期顺序注记（非 fail）

设计 §5.3 文本顺序为：inactive → ReleasePort → **关 listener** → stop celld。  
当前 `ArchiveReadyVersion`：**先** `Stop` celld，**再** `teardownPreviewIngress`（含关 listener）。  
对「archive 完成后 port 不可达」的 normative 结果 **无冲突**；仅存在 teardown 前极短窗口仍可能 Accept（upstream 已停）。**不列为 blocking**。

---

## 2. P5c 计划矩阵（相对 impl-plan §7.1）

| ID | 场景 | 结果 |
|----|------|------|
| L1 | active 台账 + binding → Listen + HTTP 200 | **pass** `TestListenerManagerReconcileOpensDedicatedPort` |
| L2 | Detach / inactive → 关 listener、连接失败 | **pass** `TestListenerManagerReconcileClosesAfterDetach` |
| L3 | orphan 台账 → `ReleasePort(orphan_reconcile)` | **pass** `TestListenerManagerOrphanRelease` |
| L4 | **R-PORT-OWNER** 非本机 `gateway_id` 不 Listen | **pass** `TestListenerManagerSkipsOtherGatewayID` |
| L5 | `localPort` → `LookupIngressByListenPort`（prod_port） | **pass**（evidence：`ingress_listen_resolve_test.go`） |
| W1 | `format.ts` dedicated URL | **defer** |
| orch Attach 后 reconcile | `ensurePreviewIngress` / `ensureProdIngress` | **pass** |

---

## 3. e2e 状态

| 项 | 设计 / 计划期望 | 仓库现状 | 结论 |
|----|-----------------|----------|------|
| `e2e/scripts/v1-ingress-port-preview.sh` | P5c 新建；preview `curl` 200 | **不存在** | **未跑** |
| archive 后同一 port 连接失败（plan §7.2 step 5） | 与 R-ARCHIVE-TEARDOWN e2e 对齐 | 无脚本 | **defer** |
| `run-all.sh` / `INGRESS_PORT_E2E` | 默认 **不** 加入 | `e2e/` 无 `INGRESS_PORT` 引用 | **符合 defer 策略** |
| `v1-ingress-port-promote.sh` | 可选 | 不存在 | **defer** |
| Host 回归 `run-all.sh` | P5c 零行为变化 | **未在本审查会话执行** | 需合并前常规跑 |

**e2e 总评：** **未实现、未门禁** — 对 **R-ARCHIVE-TEARDOWN** 由 **L2 + orch Detach 单测** 替代；对 **设计 §9 P5c 整行验收** 仍为 **缺口**（impl-plan 允许 opt-in defer）。

---

## 4. 阻塞项

| 级别 | 项 | 说明 |
|------|-----|------|
| **Blocking** | — | **无**（就 R-ARCHIVE-TEARDOWN 与已声明 P5c 核心范围而言） |
| **建议（非阻塞）** | Archive reconcile 仅 `Log` | listener 关失败时 archive 仍成功；可考虑与 ready 路径一致返回 error 或 metrics |
| **建议（非阻塞）** | 补 `TestArchiveReadyVersion` + `ListenerManager` 集成 | **已补** `TestArchiveReadyVersionClosesDedicatedListener` |
| **Defer（计划内）** | `web/format.ts` + W1 | `docs/evidence/ingress-port-p5c.md` 已列 |
| **Defer（计划内）** | port e2e / promote e2e | §7.2、§9 第二验收行 |

---

## 5. P5c 其余交付（简要）

| 能力 | 结论 |
|------|------|
| `serve.Run` wiring + shutdown `CloseAll` | **pass** |
| `ingress_resolve` 专用口优先 | **pass** |
| `prod_port` 混合模式运行时 | **pass**（解析 + listener；无 promote e2e） |
| Dashboard URL 信任 API port | **defer**（P5c plan 含最小 web，evidence 标 defer） |

---

## 6. 测试证据（审查复跑）

```bash
cd cellp && go test ./internal/gateway/... ./internal/orch/... -count=1
```

```
ok  	github.com/cellp/cellp/internal/gateway	2.131s
ok  	github.com/cellp/cellp/internal/orch	3.799s
```

详见 [docs/evidence/ingress-port-p5c.md](../evidence/ingress-port-p5c.md)。

---

## 7. 规则对照摘要

| 规则 ID | P5c 审查 |
|---------|----------|
| **R-ARCHIVE-TEARDOWN** | **pass**（台账 P5b + Listen P5c） |
| R-BIND-LOOPBACK | **pass** |
| R-PORT-OWNER | **pass**（L4） |
| R-PORT-LEDGER / orphan §5.5 | **pass**（L3 + reconcile） |
| R-PROD-PORT-STABLE | **N/A 本审查焦点**（P5b **pass**；promote e2e defer） |

---

## 8. 后续建议

1. 实现 `e2e/scripts/v1-ingress-port-preview.sh`（含 archive step 5），`INGRESS_PORT_E2E=1` opt-in。  
2. 补 orch 集成测或 archive 单测 + mock reconciler 断言 `ReconcileIngressListeners` 调用次数。  
3. 落地 `format.ts` W1 或明确挪 P5d。  
4. 更新 [INGRESS-PORT-DEPLOYMENT.md](./INGRESS-PORT-DEPLOYMENT.md) 文首状态：P5c listener **已实现**（待 e2e 证据补全）。

---

## Verify

**日期：** 2026-09-01  
**命令：**

```bash
cd cellp && go test ./... -count=1
```

**结果：** **pass**（全包；`cmd/cellpd`、`cmd/gc-once`、`internal/version` 无测试文件）

```
ok  	github.com/cellp/cellp/cmd/cellp	0.500s
?   	github.com/cellp/cellp/cmd/cellpd	[no test files]
?   	github.com/cellp/cellp/cmd/gc-once	[no test files]
ok  	github.com/cellp/cellp/internal/api	20.325s
ok  	github.com/cellp/cellp/internal/artifact	3.116s
ok  	github.com/cellp/cellp/internal/branch	2.144s
ok  	github.com/cellp/cellp/internal/config	1.279s
ok  	github.com/cellp/cellp/internal/gateway	1.789s
ok  	github.com/cellp/cellp/internal/gc	3.624s
ok  	github.com/cellp/cellp/internal/health	2.585s
ok  	github.com/cellp/cellp/internal/job	4.021s
ok  	github.com/cellp/cellp/internal/locals3	4.587s
ok  	github.com/cellp/cellp/internal/metrics	4.744s
ok  	github.com/cellp/cellp/internal/orch	8.157s
ok  	github.com/cellp/cellp/internal/registry	4.664s
ok  	github.com/cellp/cellp/internal/runtime	87.760s
ok  	github.com/cellp/cellp/internal/serve	14.817s
?   	github.com/cellp/cellp/internal/version	[no test files]
```

与 §6 的 gateway/orch 子集复跑一致；本 Verify 为合并前 **cellp 全量** 门禁记录。

---

*审查对象：P5c 相对 INGRESS-PORT-DEPLOYMENT R-ARCHIVE-TEARDOWN（TCP unreachable）及 impl-plan 声明范围。*
