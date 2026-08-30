# Phase 6F — 生产路径 Sign-off（v1 诚实范围）

> **日期：** 2026-08-30  
> **结论：** **v1 生产数据面 + Gateway 基线路径 SIGN-OFF**；**完整 6F 千万洪峰 OUT OF SCOPE**（见 [test-plan-phase6.md](../test-plan-phase6.md) §F）。

---

## 1. 范围说明

[phase-6-scale-10m-master.md](../plans/phase-6-scale-10m-master.md) 将 **6B–6F**（百万 project、50k–500k RPS、deploy storm、PG）标为 **OUT OF SCOPE**。

本报告 **不** 宣称达成 TP6-F1..F5（500k RPS、100 万 project 等）。  
本报告 **签收** cellp v1 在 SQLite + 单节点拓扑下，**生产可部署**所需证据链：

| 维度 | 签收项 | 证据 |
|------|--------|------|
| **6A 实现** | 分页 · Gateway cache · GC · Dashboard | [scale-report-6A.md](./scale-report-6A.md) |
| **M2 功能** | test-plan 全 TP-* 绿 | [test-plan.md](../test-plan.md) · `m2-run-all-*.log` |
| **M3 压测** | 单节点 CD / Gateway 基线 | [test-plan-phase2.md](../test-plan-phase2.md) 已绿 |
| **V0b prod offshoot** | RustFS offshoot 全序列 | [v0b-pass-report.md](./v0b-pass-report.md) · [runbooks/prod-offshoot-rustfs.md](../runbooks/prod-offshoot-rustfs.md) |
| **Gateway dev 基线** | `gateway-scale.sh` 维度 D1 | [scale-env.json](./scale-env.json) · phase6 README |

---

## 2. 未签收（明确 OUT OF SCOPE）

| ID | 场景 | 状态 |
|----|------|------|
| TP6-F1 | 500k RPS 混合读写 | **未测** — 需 6E fleet |
| TP6-F2 | 100 万 project 随机查询 | **未测** — 需 6B PG |
| TP6-F3 | 5k deploy/min 风暴 | **未测** — 需 6C |
| TP6-F4 | 混沌 kill 节点 | **未测** |
| TP6-F5 | 24h soak @F1 10% | **未测** |
| TP6-E1 | 50k RPS 单项目 prod | **未测** — 6E OUT OF SCOPE |

**M7（6F 千万 sign-off）** 在 master plan 中仍为 **未达成**；v1 以 **M2 + M3 + V0b prod runbook** 作为生产入口。

---

## 3. SQLite waiver（6A 已知）

TP6-A5 ListProjects @10k：**p99 238–262ms** vs gate 200ms — **documented waiver**（见 scale-report-6A）。  
千万 project 规模 **不** 在 v1 承诺内。

---

## 4. 生产部署检查清单

- [ ] `./e2e/scripts/run-all.sh` exit 0
- [ ] `stress/scripts/*` phase2 绿（或等价环境复现）
- [ ] V0b 序列在目标 RustFS 上复跑
- [ ] `offshoot_tier=rustfs` 写入 `stress-env.json` / 运维文档
- [ ] 外层 TLS / DNS / WAF（AD-10 外层项目）
- [ ] Prometheus 抓取 `:8790/metrics`（[observability.md](../observability.md)）

---

## 5. 里程碑更新（文档）

| 里程碑 | v1 状态 |
|--------|---------|
| **M6** 6A | ✅ 实现完成 |
| **M7** 6F 千万 | ❌ OUT OF SCOPE；**v1 prod path** 见上表签收 |
| **Prod offshoot** | ✅ V0b PASS + runbook |

---

*scale-report-6F v1 · 2026-08-30 · 诚实生产路径签收，非千万洪峰签收*
